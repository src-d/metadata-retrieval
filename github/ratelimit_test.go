package github

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/src-d/metadata-retrieval/testutils"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

const defaultRateLimitReset = 1 * time.Second
const defaultAbuseReset = 1 * time.Second
const default403Reset = 2 * time.Second

const defaultRequestBody = `{"content":"data"}`

var testCases = map[string]response{
	// regular answer
	"/normal": response{
		statusCode: http.StatusOK,
		headers:    nil,
		response:   apiResponse{Data: "success"},
		err:        nil,
	},
	// proper ratelimit reset in defaultRateLimitReset
	"/ratelimit_sleep": response{
		statusCode: http.StatusOK,
		headers:    rateLimitHeaders(defaultRateLimitReset),
		response:   apiResponse{Data: "whatever"},
		err:        nil,
	},
	// proper abuse reset in defaultRateLimitReset
	"/abuse_sleep": response{
		statusCode: http.StatusForbidden,
		headers:    abuseHeaders(defaultAbuseReset),
		response:   apiResponse{Data: "whatever"},
		err:        nil,
	},
	// unauthorized answer
	"/unauthorized": response{
		statusCode: http.StatusUnauthorized,
		headers:    nil,
		response: apiResponse{
			apiErrorResponse: apiErrorResponse{Message: "Bad credentials"},
		},
		err: nil,
	},
	// 403 with proper AbuseRateLimit body but without headers
	"/abuse_no_headder": response{
		statusCode: http.StatusForbidden,
		headers:    nil,
		response: apiResponse{
			apiErrorResponse: apiErrorResponse{Message: "Found an abuse detection mechanism"},
		},
		err: nil,
	},
	// 403 without proper AbuseRateLimit body nor headers
	"/forbidden_403": response{
		statusCode: http.StatusForbidden,
		headers:    nil,
		response:   apiResponse{Data: "forbidden"},
		err:        nil,
	},
	// returns a network error
	"/network_error": response{
		statusCode: http.StatusTeapot,
		headers:    nil,
		response:   apiResponse{},
		err:        fmt.Errorf("network error"),
	},
}

type response struct {
	statusCode int
	headers    func(time.Time) map[string]string
	response   apiResponse
	err        error
}

type apiResponse struct {
	apiErrorResponse
	Data string `json:"data,omitempty"`
}

func rateLimitHeaders(wait time.Duration) func(time.Time) map[string]string {
	return func(when time.Time) map[string]string {
		return map[string]string{
			"X-RateLimit-Reset":     strconv.FormatInt(when.Add(wait).Unix(), 10),
			"X-RateLimit-Remaining": "0",
		}
	}
}

func abuseHeaders(wait time.Duration) func(time.Time) map[string]string {
	return func(when time.Time) map[string]string {
		return map[string]string{
			"Retry-After": strconv.FormatInt(int64(wait.Seconds()), 10),
		}
	}
}

func newResponse(content apiResponse, headers func(time.Time) map[string]string, statusCode int) (*http.Response, error) {
	data, err := json.Marshal(content)
	if err != nil {
		return nil, err
	}

	respWriter := httptest.NewRecorder()
	respWriter.WriteHeader(statusCode)
	_, err = respWriter.Write(data)
	if err != nil {
		return nil, err
	}

	response := respWriter.Result()

	if headers != nil {
		for k, v := range headers(time.Now()) {
			response.Header.Set(k, v)
		}
	}

	return response, err
}

func newRequest(url string) *http.Request {
	req, _ := http.NewRequest("POST", url, bytes.NewBuffer([]byte(defaultRequestBody)))
	return req
}

// gitHubTransportResponseMock avoids calling the network to get the response directly from testcases
type gitHubTransportResponseMock struct {
	lastRequest *http.Request
}

func (gh *gitHubTransportResponseMock) RoundTrip(req *http.Request) (*http.Response, error) {
	gh.lastRequest = req
	url := req.URL.String()
	tc, ok := testCases[url]
	if !ok {
		return newResponse(apiResponse{}, nil, http.StatusOK)
	}

	if tc.err != nil {
		return &http.Response{}, tc.err
	}

	return newResponse(tc.response, tc.headers, tc.statusCode)
}
func TestRateLimitSuite(t *testing.T) {
	suite.Run(t, new(RateLimitSuite))
}

type RateLimitSuite struct {
	suite.Suite
	transport        *RateLimitTransport
	loggerMock       *testutils.LoggerMock
	ghResponseMocker *gitHubTransportResponseMock
	require          *require.Assertions
}

func (s *RateLimitSuite) SetupSuite() {
}

func (s *RateLimitSuite) SetupTest() {
	s.require = s.Require()
	s.loggerMock = &testutils.LoggerMock{}
	s.ghResponseMocker = &gitHubTransportResponseMock{}
	s.transport = NewRateLimitTransport(s.ghResponseMocker, s.loggerMock)
	s.transport.defaultAbuseSleep = default403Reset
}

func (s *RateLimitSuite) TearDownSuite() {
}

// TestNoLimit ensures that the regular case: no rate limit nor abuse, is not considered a RateLimit case
func (s *RateLimitSuite) TestNoLimit() {
	t0 := time.Now()

	response, err := s.transport.RoundTrip(newRequest("/normal"))
	s.require.NoError(err)

	elapsed := time.Now().Sub(t0)
	s.True(elapsed < 500*time.Millisecond, "request took %s, but it should be almost instant", elapsed)
	s.Equal("", s.loggerMock.Next())

	content, err := ioutil.ReadAll(response.Body)
	s.require.NoError(err)

	var data apiResponse
	err = json.Unmarshal(content, &data)
	s.require.NoError(err)

	s.Equal("success", data.Data)
	s.Len(data.Errors, 0)
}

// TestRateLimitConsecutively ensures that hitting RateLimit twice, causes two waiting periods
// it should log the RateLimitError, and wait till the lock expires
// if the next Request hits the RateLimit again, it should do the same
// if the next Request does not hit the RateLimit, it should return the response inmediately
func (s *RateLimitSuite) TestRateLimitConsecutively() {
	t0 := time.Now()

	response, err := s.transport.RoundTrip(newRequest("/ratelimit_sleep"))
	s.require.Error(err)
	s.require.IsType(&ErrRateLimit{}, err)

	elapsed := time.Now().Sub(t0)
	s.True(elapsed < 500*time.Millisecond, "request took %s, but it should be almost instant", elapsed)
	s.NotNil(response)
	s.Equal("", s.loggerMock.Next())

	response, err = s.transport.RoundTrip(newRequest("/ratelimit_sleep"))
	s.require.Error(err)
	s.require.IsType(&ErrRateLimit{}, err)

	t1 := time.Now()

	elapsed = t1.Sub(t0)
	s.True(elapsed > defaultRateLimitReset, "request took %s, but it should be, at least %s", elapsed, defaultRateLimitReset)
	s.NotNil(response)
	s.Contains(s.loggerMock.Next(), "rate limit reached, sleeping until")
	s.Equal("", s.loggerMock.Next())

	response, err = s.transport.RoundTrip(newRequest("/normal"))
	s.require.NoError(err)

	t2 := time.Now()

	elapsed = t2.Sub(t1)
	s.True(elapsed > defaultRateLimitReset, "request took %s, but it should be, at least %s", elapsed, defaultRateLimitReset)
	s.Contains(s.loggerMock.Next(), "rate limit reached, sleeping until")
	s.Equal("", s.loggerMock.Next())

	response, err = s.transport.RoundTrip(newRequest("/normal"))
	s.require.NoError(err)

	elapsed = time.Now().Sub(t2)
	s.True(elapsed < 500*time.Millisecond, "request took %s, but it should be almost instant", elapsed)
	s.Equal("", s.loggerMock.Next())
}

// TestRateLimitButWaitInsteadOfRetry ensures that RateLimitTransport does not block requests once the
// previous RateLimit expired
func (s *RateLimitSuite) TestRateLimitButWaitInsteadOfRetry() {
	t0 := time.Now()

	response, err := s.transport.RoundTrip(newRequest("/ratelimit_sleep"))
	s.require.Error(err)
	s.IsType(&ErrRateLimit{}, err)

	elapsed := time.Now().Sub(t0)
	s.True(elapsed < 500*time.Millisecond, "request took %s, but it should be almost instant", elapsed)
	s.NotNil(response)
	s.Equal("", s.loggerMock.Next())

	// The next Request is going to wait for more time than the previous RateLimit, so it should not be blocked by RateLimitTransport
	time.Sleep(defaultRateLimitReset + time.Second)

	t1 := time.Now()

	response, err = s.transport.RoundTrip(newRequest("/normal"))
	s.require.NoError(err)

	elapsed = time.Now().Sub(t1)
	s.True(elapsed < 500*time.Millisecond, "request took %s, but it should be almost instant", elapsed)
	s.Equal("", s.loggerMock.Next())
}

// TestAbuse ensures that hitting AbuseRateLimit, causes a wait period
// it should log the AbuseRateLimit, and wait till the lock expires
// if the next Request does not hit a RateLimit, it should return the response inmediately
func (s *RateLimitSuite) TestAbuse() {
	t0 := time.Now()

	response, err := s.transport.RoundTrip(newRequest("/abuse_sleep"))
	s.require.Error(err)
	s.IsType(&ErrAbuseRateLimit{}, err)

	elapsed := time.Now().Sub(t0)
	s.True(elapsed < 500*time.Millisecond, "request took %s, but it should be almost instant", elapsed)
	s.NotNil(response)
	s.Equal("", s.loggerMock.Next())

	response, err = s.transport.RoundTrip(newRequest("/normal"))
	s.require.NoError(err)

	elapsed = time.Now().Sub(t0)
	s.True(elapsed > defaultAbuseReset, "request took %s, but it should be, at least %s", elapsed, defaultAbuseReset)
	s.Contains(s.loggerMock.Next(), "rate limit reached, sleeping until")
	s.Equal("", s.loggerMock.Next())
}

// TestUnauthorized ensures that hitting unauthroized requests doesn't cause a wait period
func (s *RateLimitSuite) TestUnauthorized() {
	t0 := time.Now()

	response, err := s.transport.RoundTrip(newRequest("/unauthorized"))
	s.require.Error(err)
	s.IsType(&ErrUnauthorized{}, err)

	err = err.(*ErrUnauthorized)
	s.require.Equal(err.Error(), "unauthorized: Bad credentials")

	elapsed := time.Now().Sub(t0)
	s.True(elapsed < 500*time.Millisecond, "request took %s, but it should be almost instant", elapsed)
	s.Equal("", s.loggerMock.Next())

	content, err := ioutil.ReadAll(response.Body)
	s.require.NoError(err)

	var data apiResponse
	err = json.Unmarshal(content, &data)
	s.require.NoError(err)

	s.Equal("Bad credentials", data.apiErrorResponse.Message)
}

// TestAbuseWhithoutHeadersButWithProperBody ensures that a 403 Forbidden Response having a proper Abuse body is handled
// as being an AbuseRateLimit, using defaultAbuseRetryAfter
func (s *RateLimitSuite) TestAbuseWhithoutHeadersButWithProperBody() {
	t0 := time.Now()

	response, err := s.transport.RoundTrip(newRequest("/abuse_no_headder"))
	s.require.Error(err)
	s.IsType(&ErrAbuseRateLimit{}, err)

	elapsed := time.Now().Sub(t0)
	s.True(elapsed < 500*time.Millisecond, "request took %s, but it should be almost instant", elapsed)
	s.NotNil(response)
	s.Contains(s.loggerMock.Next(), "error reading")
	s.Equal("", s.loggerMock.Next())

	response, err = s.transport.RoundTrip(newRequest("/normal"))
	s.require.NoError(err)

	elapsed = time.Now().Sub(t0)
	s.True(elapsed > default403Reset, "request took %s, but it should be, at least %s", elapsed, default403Reset)
	s.Contains(s.loggerMock.Next(), "rate limit reached, sleeping until")
	s.Equal("", s.loggerMock.Next())
}

// TestForbidden403NotHavingHeadersNorBody ensures that a 403 Forbidden Response, not having RateLimit Headers nor
// proper Body, is ignored and not handled as an Abuse
func (s *RateLimitSuite) TestForbidden403NotHavingHeadersNorBody() {
	t0 := time.Now()

	response, err := s.transport.RoundTrip(newRequest("/forbidden_403"))
	s.require.NoError(err)

	elapsed := time.Now().Sub(t0)
	s.True(elapsed < 500*time.Millisecond, "request took %s, but it should be almost instant", elapsed)
	s.Equal(http.StatusForbidden, response.StatusCode)
	s.Contains(s.loggerMock.Next(), "could not be read as an Abuse Rate Limit response")
	s.Equal("", s.loggerMock.Next())

	content, err := ioutil.ReadAll(response.Body)
	s.require.NoError(err)

	var data apiResponse
	err = json.Unmarshal(content, &data)
	s.require.NoError(err)

	s.Equal("forbidden", data.Data)
	s.Len(data.Errors, 0)

	response, err = s.transport.RoundTrip(newRequest("/normal"))
	s.require.NoError(err)

	elapsed = time.Now().Sub(t0)
	s.True(elapsed < 500*time.Millisecond, "request took %s, but it should be almost instant", elapsed)
	s.Equal("", s.loggerMock.Next())
}

// TestFailedRequest ensures that a failed request is not interpreted as a RateLimit,
// and the same error is returned as it was retrieved
func (s *RateLimitSuite) TestFailedRequest() {
	t0 := time.Now()

	_, err := s.transport.RoundTrip(newRequest("/network_error"))
	s.require.Error(err)
	s.Equal("network error", err.Error())

	elapsed := time.Now().Sub(t0)
	s.True(elapsed < 500*time.Millisecond, "request took %s, but it should be almost instant", elapsed)
	s.Equal("", s.loggerMock.Next())

	_, err = s.transport.RoundTrip(newRequest("/normal"))
	s.require.NoError(err)

	elapsed = time.Now().Sub(t0)
	s.True(elapsed < 500*time.Millisecond, "request took %s, but it should be almost instant", elapsed)
	s.Equal("", s.loggerMock.Next())
}

// TestRequestBodyIsKept ensures that the request sent through RateLimitTransport
// is still readable and contains the same body when it calls its inner transport
func (s *RateLimitSuite) TestRequestBodyIsKept() {
	s.Nil(s.ghResponseMocker.lastRequest)

	_, err := s.transport.RoundTrip(newRequest("/normal"))
	s.require.NoError(err)

	s.require.NotNil(s.ghResponseMocker.lastRequest)

	receivedRequestContent, err := ioutil.ReadAll(s.ghResponseMocker.lastRequest.Body)
	s.require.NoError(err)

	s.Equal(defaultRequestBody, string(receivedRequestContent))
}
