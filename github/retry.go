package github

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	"gopkg.in/src-d/go-log.v1"
)

// errUnretriable wraps an error to stop retry
type errUnretriable struct {
	Err error
}

func (e *errUnretriable) Error() string {
	return e.Err.Error()
}

type retryTransport struct {
	T http.RoundTripper
}

func (t *retryTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	var r *http.Response
	var err error
	retry(func() error {
		r, err = t.T.RoundTrip(req)
		if err != nil {
			return err
		}

		if r.StatusCode == http.StatusOK {
			return nil
		}

		body, _ := ioutil.ReadAll(r.Body)

		// Restore the io.ReadCloser
		r.Body = ioutil.NopCloser(bytes.NewBuffer(body))

		err = fmt.Errorf("non-200 OK status code: %v body: %q", r.Status, body)
		if r.StatusCode > 500 {
			return err
		}
		return &errUnretriable{Err: err}
	})

	return r, err
}

const (
	retries  = 10
	delay    = 10 * time.Millisecond
	truncate = 10 * time.Second
)

func retry(f func() error) error {
	d := delay
	var i uint

	for ; ; i++ {
		err := f()
		if err == nil {
			return nil
		}
		if errU, ok := err.(*errUnretriable); ok {
			return errU.Err
		}

		if i == retries {
			return err
		}

		log.Errorf(err, "retrying in %v", d)
		time.Sleep(d)

		d = d * (1<<i + 1)
		if d > truncate {
			d = truncate
		}
	}
}
