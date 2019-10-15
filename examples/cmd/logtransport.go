package main

import (
	"net/http"
	"time"

	"gopkg.in/src-d/go-log.v1"

	"github.com/src-d/metadata-retrieval/utils"
)

func setLogTransport(client *http.Client, logger log.Logger) {
	client.Transport = &logTransport{client.Transport, logger}
}

type logTransport struct {
	T      http.RoundTripper
	Logger log.Logger
}

func (t *logTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	t0 := time.Now()

	// Since the underlaying Transport.RoundTrip might consume the Request, and it's also needed later to log the process,
	// it is needed to make an snapshot of the Request.Body in advance; otherwise, its will be no longer accessible.
	bakupReqBody := utils.BackupRequestBody(r)
	resp, err := t.T.RoundTrip(r)
	restoreRequestBody(r, bakupReqBody)

	// Since the outter Transport.RoundTrip calling this one might also consume the Request, and it is also needed here to log
	// the process, it will be done over a snapshot of the Request.Body; otherwise, its will be no longer accessible by outer Transports.
	reqBody, _ := utils.ReadRequestAndRestore(r)

	if err != nil {
		return resp, err
	}

	// Since the Response has been read, it is needed to restore it
	// before reusing it; otherwise, its body will be unreadable again in other Transports.
	respBody, err := utils.ReadResponseAndRestore(resp)
	if err != nil {
		return resp, err
	}

	t.Logger.With(
		log.Fields{"elapsed": time.Since(t0), "response-code": resp.StatusCode, "url": r.URL, "request-body": string(reqBody), "response-body": string(respBody)},
	).Debugf("HTTP response")

	return resp, err
}
