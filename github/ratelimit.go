package github

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"gopkg.in/src-d/go-log.v1"

	"github.com/src-d/metadata-retrieval/utils"
)

var (
	defaultAbuseRetryAfter = time.Minute
)

// RateLimitTransport implements GitHub GraphQL API v4 best practices for avoiding rate limits
// https://developer.github.com/v4/guides/resource-limitations/#rate-limit
// https://developer.github.com/v3/#abuse-rate-limits
// RateLimitTransport will process a Request, and if the response could not be fetched
// because of a RateLimit or an AbuseRateLimit, it will return an ErrorRateLimit
// and it no longer process any further Requests until the Limit has been expired.
// RateLimitTransport does not retry; that behaviour must be implemented by another Transport
// Each client (with its own token) should use its own RateLimitTransport
type RateLimitTransport struct {
	transport   http.RoundTripper
	lockedUntil time.Time
	mutex       sync.Mutex
	logger      log.Logger
}

// SetRateLimitTransport wraps the passed client.Transport with a RateLimitTransport
func SetRateLimitTransport(client *http.Client, logger log.Logger) {
	client.Transport = NewRateLimitTransport(client.Transport, logger)
}

// NewRateLimitTransport returns a new NewRateLimitTransport, who will call the passed
// http.RoundTripper to process the http.Request
// Each client (with its own token) should use its own RateLimitTransport
func NewRateLimitTransport(rt http.RoundTripper, logger log.Logger) *RateLimitTransport {
	return &RateLimitTransport{
		transport: rt,
		logger:    logger,
	}
}

// RoundTrip executes a single HTTP transaction, returning a Response for the provided Request.
// If the request hitted an API RateLimit or Abuse, it will return an ErrorRateLimit
// and it no longer process any further Requests until the Limit has been expired.
func (rt *RateLimitTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Make Requests serially
	rt.lock()
	defer rt.unlock()

	if time.Now().Before(rt.lockedUntil) {
		rt.logger.Infof("rate limit reached, sleeping until %s", rt.lockedUntil)
		time.Sleep(rt.lockedUntil.Sub(time.Now()))
	}

	resp, err := rt.transport.RoundTrip(req)
	if err != nil {
		return resp, err
	}

	if errRateLimit := checkResponseRateLimit(resp, rt.logger); errRateLimit != nil {
		rt.lockedUntil = errRateLimit.when()
		rt.logger.Errorf(errRateLimit, "locking transport for %s", errRateLimit.when().Sub(time.Now()))
		return nil, errRateLimit
	}

	return resp, nil
}

func (rt *RateLimitTransport) lock() {
	rt.mutex.Lock()
}

func (rt *RateLimitTransport) unlock() {
	rt.mutex.Unlock()
}

// checkRateLimit checks the API response and returns a whener error if a rate limit was found:
// - *ErrRateLimit is returned when the request failed because of a RateLimit
//    https://developer.github.com/v4/guides/resource-limitations/#rate-limit
// - *ErrAbuseRateLimit is returned when the request triggered a GitHub abuse detection mechanism
//    https://developer.github.com/v3/#abuse-rate-limits
func checkResponseRateLimit(resp *http.Response, logger log.Logger) whener {
	if err := asErrRateLimit(resp); err != nil {
		return err
	}

	if err := asErrAbuseRateLimit(resp, logger); err != nil {
		return err
	}

	return nil
}

// ErrRateLimit is returned when a request failed because of a RateLimit
// https://developer.github.com/v4/guides/resource-limitations/#rate-limit
type ErrRateLimit struct {
	errRetryLater
}

func (e *ErrRateLimit) Error() string {
	return fmt.Sprintf("API rate limit exceeded; %s", e.errRetryLater.Error())
}

// ErrAbuseRateLimit is returned when a request triggers any GitHub abuse detection mechanism
// https://developer.github.com/v3/#abuse-rate-limits
type ErrAbuseRateLimit struct {
	errRetryLater
}

func (e *ErrAbuseRateLimit) Error() string {
	return fmt.Sprintf("abuse detection mechanism triggered; %s", e.errRetryLater.Error())
}

type errRetryLater struct {
	retryAfter time.Time
}

func (e *errRetryLater) Error() string {
	return fmt.Sprintf("retry after %s", e.retryAfter)
}

func (e *errRetryLater) when() time.Time {
	return e.retryAfter
}

type whener interface {
	error
	when() time.Time
}

/*
An apiErrorResponse reports one or more errors caused by an API request.
GitHub API docs: https://developer.github.com/v3/#client-errors
*/
type apiErrorResponse struct {
	Message          string `json:"message,omitempty"`
	DocumentationURL string `json:"documentation_url,omitempty"`
	Errors           []struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"errors,omitempty"`
}

func (aer *apiErrorResponse) isAbuseRateLimit() bool {
	return strings.Contains(aer.DocumentationURL, "abuse") ||
		strings.Contains(aer.Message, "abuse detection")
}

// asErrAbuseRateLimit will return an ErrAbuseRateLimit if the status Response
// is 403 Forbidden and there is a valid 'Retry-After' Response header
// It will also return an ErrAbuseRateLimit with a default RetryAfter if the
// response.Body can be mapped as an apiErrorResponse, and its Message or Documentation
// mentions the abuse error.
func asErrAbuseRateLimit(resp *http.Response, logger log.Logger) *ErrAbuseRateLimit {
	if resp.StatusCode != http.StatusForbidden {
		return nil
	}

	retryInHeader := resp.Header.Get("Retry-After")
	retryIn, err := strconv.Atoi(retryInHeader)
	if err == nil {
		return &ErrAbuseRateLimit{
			errRetryLater{time.Now().Add(time.Duration(retryIn) * time.Second)},
		}
	}

	errorResponse := &apiErrorResponse{}
	err = readAPIErrorResponse(resp, errorResponse)
	if err == nil && errorResponse.isAbuseRateLimit() {
		logger.Warningf("error reading 'Retry-After=%s' header from the '403 Forbidden' response, using default '%s': %s",
			retryInHeader,
			defaultAbuseRetryAfter,
			err,
		)

		return &ErrAbuseRateLimit{
			errRetryLater{time.Now().Add(defaultAbuseRetryAfter)},
		}
	}

	logger.Warningf("403 Forbidden response got, but could not be read as an Abuse Rate Limit response")
	return nil
}

// asErrRateLimit will return an ErrRateLimit if the Response 'X-RateLimit-Remaining' header is 0,
// and the X-RateLimit-Reset' header contains a valid reset info
func asErrRateLimit(resp *http.Response) *ErrRateLimit {
	rateLimitResetHeader := resp.Header.Get("X-RateLimit-Reset")
	rateLimitReset, errReadingReset := strconv.Atoi(rateLimitResetHeader)
	rateLimitRemainingHeader := resp.Header.Get("X-RateLimit-Remaining")
	rateLimitRemaining, errReadingRemaining := strconv.Atoi(rateLimitRemainingHeader)

	if errReadingReset == nil && errReadingRemaining == nil && rateLimitRemaining == 0 {
		return &ErrRateLimit{
			errRetryLater{time.Unix(int64(rateLimitReset)+1, 0)},
		}
	}

	return nil
}

// readAPIErrorResponse reads the response.Body into the passed errorResponse
func readAPIErrorResponse(resp *http.Response, errorResponse *apiErrorResponse) error {
	bodyContent, err := utils.ReadResponseAndRestore(resp)
	if err != nil {
		return err
	}

	return json.Unmarshal(bodyContent, errorResponse)
}
