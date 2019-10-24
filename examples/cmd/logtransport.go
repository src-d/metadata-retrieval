package main

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"time"

	"gopkg.in/src-d/go-log.v1"
)

func setLogTransport(client *http.Client, logger log.Logger) {
	t := &logTransport{client.Transport, logger}
	client.Transport = t
}

type logTransport struct {
	T      http.RoundTripper
	Logger log.Logger
}

func (t *logTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	t0 := time.Now()

	reqBody, _ := ioutil.ReadAll(r.Body)
	r.Body = ioutil.NopCloser(bytes.NewBuffer(reqBody))

	resp, err := t.T.RoundTrip(r)
	if err != nil {
		return resp, err
	}

	respBody, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return resp, err
	}

	t.Logger.With(
		log.Fields{
			"elapsed":         time.Since(t0),
			"response-code":   resp.StatusCode,
			"url":             r.URL,
			"request-header":  r.Header,
			"request-body":    string(reqBody),
			"response-header": resp.Header,
			"response-body":   string(respBody),
		},
	).Debugf("HTTP response")

	resp.Body = ioutil.NopCloser(bytes.NewBuffer(respBody))

	return resp, err
}
