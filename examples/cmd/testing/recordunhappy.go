package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/src-d/metadata-retrieval/github"
	"github.com/src-d/metadata-retrieval/testutils"

	"github.com/motemen/go-loghttp"
	"golang.org/x/oauth2"
	"gopkg.in/src-d/go-log.v1"
)

// recordUnhappy recording an unhappy path with various errors
func recordUnhappy(org, repo string) {

	// Variables to hold graphql queries (keys) and responses (values)
	var (
		reqResp map[string]testutils.Response = make(map[string]testutils.Response)
		query   string
		ctx     context.Context = context.Background()
	)
	client := oauth2.NewClient(
		ctx,
		oauth2.StaticTokenSource(
			&oauth2.Token{AccessToken: os.Getenv("GITHUB_TOKEN")},
		))
	github.SetRateLimitTransport(client, log.New(nil))
	github.SetRetryTransport(client)
	client.Transport = &loghttp.Transport{
		Transport: client.Transport,
		LogRequest: func(req *http.Request) {
			query = cloneRequest(req)
		},
		LogResponse: func(resp *http.Response) {
			reqResp[query] = cloneResponse(resp)
		},
	}

	log.Infof("Start recording errors")

	dt := time.Now()
	// 200 OK {"errors":[{"message":"Unexpected end of document"}]}
	log.Infof("Record a malformed query string")
	filename := fmt.Sprintf("200_malformed_%s", dt.Format("2006-01-02"))
	recordAndStoreErrors(client, filename, testutils.BasicMalformedQuery, reqResp)
	reqResp = make(map[string]testutils.Response)

	// 200 OK Exceed limit of nodes query
	log.Infof("Record an exceeded nodes query")
	filename = fmt.Sprintf("200_exceed_%s", dt.Format("2006-01-02"))
	recordAndStoreErrors(client, filename, testutils.ReallyReallyBigQuery, reqResp)
	reqResp = make(map[string]testutils.Response)

	// 200 OK but unrelated query
	log.Infof("Record an exceeded nodes query")
	filename = fmt.Sprintf("200_unmarshable_%s", dt.Format("2006-01-02"))
	recordAndStoreErrors(client, filename, testutils.BasicQuery, reqResp)
	reqResp = make(map[string]testutils.Response)

}

func recordAndStoreErrors(client *http.Client, filename, query string, reqResp map[string]testutils.Response) {
	gqlMarshalled, _ := json.Marshal(testutils.GQLRequest{Query: query})
	resp, err := client.Post(testutils.Endpoint, "application/json", strings.NewReader(string(gqlMarshalled)))
	if err != nil {
		log.Errorf(err, "200?")
	}
	log.Infof("Recorded %d", resp.StatusCode)

	err = encodeAndStore(filename+".gob.gz", reqResp)
	if err != nil {
		panic(err)
	}
}
