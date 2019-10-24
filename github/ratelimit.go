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
)

var (
	// defaultAbuseRetryAfter is used when an Abuse is returned, but without 'Retry-After' header
	// this value of 60s is usually returned by GitHub when they trigger the Abuse mechanism
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
	sync.Mutex

	transport         http.RoundTripper
	lockedUntil       time.Time
	logger            log.Logger
	defaultAbuseSleep time.Duration
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
		transport:         rt,
		logger:            logger,
		defaultAbuseSleep: defaultAbuseRetryAfter,
	}
}

// RoundTrip executes a single HTTP transaction, returning a Response for the provided Request.
// If the request hitted an API RateLimit or Abuse, it will return an ErrorRateLimit
// and it no longer process any further Requests until the Limit has been expired.
func (rt *RateLimitTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Make Requests serially
	rt.Lock()
	defer rt.Unlock()

	if time.Now().Before(rt.lockedUntil) {
		rt.logger.Infof("rate limit reached, sleeping until %s", rt.lockedUntil)
		time.Sleep(rt.lockedUntil.Sub(time.Now()))
	}

	resp, err := rt.transport.RoundTrip(req)
	if err != nil {
		return resp, err
	}

	if errUnauth := checkResponseUnauth(resp); errUnauth != nil {
		return resp, errUnauth
	}

	if errRateLimit := checkResponseRateLimit(resp, rt.logger, rt.defaultAbuseSleep); errRateLimit != nil {
		rt.lockedUntil = errRateLimit.when()
		return resp, errRateLimit
	}

	return resp, nil
}

// checkResponseUnauth checks whether the request is authenticated
func checkResponseUnauth(resp *http.Response) error {
	if resp.StatusCode == http.StatusUnauthorized {
		errorResponse := &apiErrorResponse{}
		err := readAPIErrorResponse(resp, errorResponse)
		if err != nil {
			return err
		}

		return &ErrUnauthorized{message: errorResponse.Message}
	}

	return nil
}

// checkRateLimit checks the API response and returns a whener error if a rate limit was found:
// - *ErrRateLimit is returned when the request failed because of a RateLimit
//    https://developer.github.com/v4/guides/resource-limitations/#rate-limit
// - *ErrAbuseRateLimit is returned when the request triggered a GitHub abuse detection mechanism
//    https://developer.github.com/v3/#abuse-rate-limits
func checkResponseRateLimit(resp *http.Response, logger log.Logger, defaultAbuseSleep time.Duration) whener {
	if err := asErrRateLimit(resp); err != nil {
		return err
	}

	if err := asErrAbuseRateLimit(resp, logger, defaultAbuseSleep); err != nil {
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

// ErrUnauthorized is returned when a response returns 401
type ErrUnauthorized struct {
	message string
}

func (e *ErrUnauthorized) Error() string {
	return fmt.Sprintf("unauthorized: %s", e.message)
}

type whener interface {
	error
	when() time.Time
}

/*
An apiErrorResponse reports one or more errors caused by an API request.
GitHub API docs: https://developer.github.com/v3/#client-errors
Certain errors in graphql can come up in the body of the message with a 200 OK status
https://graphql.github.io/graphql-spec/June2018/#sec-Errors
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

// asErrAbuseRateLimit returns ErrAbuseRateLimit when abuse was detected
// it sets default value sleeping time if github didn't return any
func asErrAbuseRateLimit(resp *http.Response, logger log.Logger, defaultSleep time.Duration) *ErrAbuseRateLimit {
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
			defaultSleep,
			err,
		)

		return &ErrAbuseRateLimit{
			errRetryLater{time.Now().Add(defaultSleep)},
		}
	}

	logger.Warningf("403 Forbidden response got, but could not be read as an Abuse Rate Limit response")
	return nil
}

// asErrRateLimit will return an ErrRateLimit if the Response 'X-RateLimit-Remaining' header is 0,
// and the X-RateLimit-Reset' header contains a valid reset info
func asErrRateLimit(resp *http.Response) *ErrRateLimit {
	rateLimitResetHeader := resp.Header.Get("X-RateLimit-Reset")
	rateLimitReset, err := strconv.Atoi(rateLimitResetHeader)
	if err != nil {
		return nil
	}

	rateLimitRemainingHeader := resp.Header.Get("X-RateLimit-Remaining")
	rateLimitRemaining, err := strconv.Atoi(rateLimitRemainingHeader)
	if err != nil {
		return nil
	}

	if rateLimitRemaining == 0 {
		return &ErrRateLimit{
			errRetryLater{time.Unix(int64(rateLimitReset)+1, 0)},
		}
	}

	return nil
}

// readAPIErrorResponse reads the response.Body into the passed errorResponse
func readAPIErrorResponse(resp *http.Response, errorResponse *apiErrorResponse) error {
	bodyContent, err := readResponseAndRestore(resp)
	if err != nil {
		return err
	}

	return json.Unmarshal(bodyContent, errorResponse)
}
