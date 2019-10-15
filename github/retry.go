package github

import (
	"context"
	"net/http"
	"time"

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
	var r *http.Response
	var err error
	retry(func() error {
		r, err = t.T.RoundTrip(req)
		if err == context.Canceled {
			return nil
		}

		if err != nil || r.StatusCode >= 500 {
			return err
		}

		return nil
	})

	return r, err
}

const (
	retries  = 10
	delay    = 10 * time.Millisecond
	truncate = 10 * time.Second
)

func retry(f func() error) {
	d := delay
	var i uint

	var err error
	for ; ; i++ {
		err = f()
		if err == nil {
			return
		}

		if i == retries {
			break
		}

		log.Errorf(err, "retrying in %v", d)
		time.Sleep(d)

		d = d * (1<<i + 1)
		if d > truncate {
			d = truncate
		}
	}

	log.Errorf(err, "retried %v times", retries)
}
