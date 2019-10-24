package github

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/cenkalti/backoff"
	"gopkg.in/src-d/go-log.v1"
)

// SetRetryTransport wraps the passed client.Transport with a RetryTransport
func SetRetryTransport(client *http.Client) {
	client.Transport = &retryTransport{client.Transport}
}

// retryTransport retries a http.Request if it fails when processing, or if
// its http.Response has StatusCode in 5xx range (server errors)
type retryTransport struct {
	T http.RoundTripper
}

func (t *retryTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	var response *http.Response
	requestBodyContent, err := ioutil.ReadAll(req.Body)
	if err != nil {
		return nil, fmt.Errorf("could not backup the response before sending it through the retry loop: %s", err)
	}

	do := func() error {
		var err error
		req.Body = ioutil.NopCloser(bytes.NewReader(requestBodyContent))
		response, err = t.T.RoundTrip(req)
		if err == context.Canceled {
			return backoff.Permanent(err)
		}

		if err, ok := err.(*ErrUnauthorized); ok {
			return backoff.Permanent(err)
		}

		if err != nil {
			return err
		}

		if response.StatusCode >= 500 {
			responseBody, err := readResponseAndRestore(response)
			if err != nil {
				return err
			}

			return fmt.Errorf("%s: %s", response.Status, responseBody)
		}

		return nil
	}

	return response, retry(do)
}

const (
	maxRetries      = 10
	initialInterval = 10 * time.Millisecond
	maxInterval     = 10 * time.Second
	multiplier      = 6 // this multiplier, with these defaults will cause kind of: 10ms, 60ms, 360ms, 2.2s, 10s, 10s ...
)

// retry retries the passed operation until it returns no err or a permanent one
// or until it reaches the passed max number of attempts.
// If returns either the first backoff.PermanentError it gets, or the last obtained error
// when reaching the max number of attempts
func retry(operation backoff.Operation) error {
	retryCount := 0

	onError := func(reason error, nextSlep time.Duration) {
		retryCount++
		log.Warningf("retrying in %s; got %s", nextSlep, reason)
	}

	backoffPolicy := backoff.NewExponentialBackOff()
	backoffPolicy.InitialInterval = initialInterval
	backoffPolicy.MaxInterval = maxInterval
	backoffPolicy.Multiplier = multiplier

	err := backoff.RetryNotify(operation, backoff.WithMaxRetries(backoffPolicy, maxRetries), onError)

	if err != nil {
		elapsed := backoffPolicy.GetElapsedTime().Seconds()
		log.Errorf(err, "retry was aborted after %d attempts and %fs", retryCount, elapsed)
	}

	return err
}
