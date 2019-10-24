package github

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptrace"
	"os"
	"strings"
	"testing"

	"github.com/src-d/metadata-retrieval/testutils"

	"github.com/stretchr/testify/suite"
	"golang.org/x/oauth2"
	"gopkg.in/src-d/go-log.v1"
)

type RetryTestSuite struct {
	suite.Suite
	client *http.Client
	count  int
}

func (suite *RetryTestSuite) SetupSuite() {
	ctx := context.Background()
	suite.client = oauth2.NewClient(
		ctx,
		oauth2.StaticTokenSource(
			&oauth2.Token{AccessToken: os.Getenv("GITHUB_TOKEN")},
		))
	SetRateLimitTransport(suite.client, log.New(nil))
	SetRetryTransport(suite.client)
}

// TestRetry tests online retries with really big queries that (almost always) return a 502
func (suite *RetryTestSuite) TestRetry() {
	log.Infof("Record 502 server error")
	gqlMarshalled, _ := json.Marshal(testutils.GQLRequest{Query: testutils.ReallyBigQuery})
	req, _ := http.NewRequest("POST", testutils.Endpoint, strings.NewReader(string(gqlMarshalled)))
	trace := &httptrace.ClientTrace{
		GotConn: func(connInfo httptrace.GotConnInfo) {
			suite.count++
		},
	}
	req = req.WithContext(httptrace.WithClientTrace(req.Context(), trace))
	resp, err := suite.client.Do(req)
	if err != nil {
		log.Errorf(err, "502 heavy load")
	} else {
		log.Infof("Recorded %d body: %s", resp.StatusCode)
	}
	suite.Equal(11, suite.count) // 10 retries + the initial one
}

func TestRetryTestSuite(t *testing.T) {
	suite.Run(t, new(RetryTestSuite))
}
