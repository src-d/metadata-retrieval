package github

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	"gopkg.in/src-d/go-log.v1"
)

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

		if r.StatusCode != http.StatusOK {
			body, _ := ioutil.ReadAll(r.Body)

			// Restore the io.ReadCloser
			r.Body = ioutil.NopCloser(bytes.NewBuffer(body))

			return fmt.Errorf("non-200 OK status code: %v body: %q", r.Status, body)
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

func retry(f func() error) error {
	d := delay
	var i uint

	for ; ; i++ {
		err := f()
		if err == nil {
			return nil
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
