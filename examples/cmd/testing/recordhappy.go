package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/src-d/metadata-retrieval/github"
	"github.com/src-d/metadata-retrieval/testutils"

	"github.com/motemen/go-loghttp"
	"golang.org/x/oauth2"
	"gopkg.in/src-d/go-log.v1"
)

// recordHappy recording a happy path with all responses 200 OK
func recordHappy(org, repo string) {
	// Variables to hold graphql queries (keys) and responses (values)
	var (
		reqResp map[string]testutils.Response = make(map[string]testutils.Response)
		query   string
		ctx     context.Context
	)
	ctx = context.Background()
	// Create a with authentication and add logging (recording)
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
			// https://stackoverflow.com/a/19006050/869151
			req.Close = true
		},
		LogResponse: func(resp *http.Response) {
			if resp.StatusCode == http.StatusOK {
				reqResp[query] = cloneResponse(resp)
			}
		},
	}

	memory := new(testutils.Memory)
	downloader, err := github.NewMemoryDownloader(client, memory)
	if err != nil {
		panic(err)
	}

	// record a repo crawl
	log.Infof("Start recording a repo")
	downloader.DownloadRepository(ctx, org, repo, 0)
	log.Infof("End recording a repo")

	// create a struct with the oracle
	repoOracles := testutils.TestOracles{
		RepositoryTestOracles: []testutils.RepositoryTestOracle{
			testutils.RepositoryTestOracle{
				Owner:                 org,
				Repository:            repo,
				Version:               0,
				URL:                   memory.Repository.URL,
				Topics:                memory.Topics,
				CreatedAt:             memory.Repository.CreatedAt.UTC().String(),
				IsPrivate:             memory.Repository.IsPrivate,
				IsArchived:            memory.Repository.IsArchived,
				HasWiki:               memory.Repository.HasWikiEnabled,
				NumOfPRs:              len(memory.PRs),
				NumOfPRComments:       len(memory.PRComments),
				NumOfIssues:           len(memory.Issues),
				NumOfIssueComments:    len(memory.IssueComments),
				NumOfPRReviews:        len(memory.PRReviews),
				NumOfPRReviewComments: len(memory.PRReviewComments),
			},
		},
	}

	// store the results
	dt := time.Now()
	filename := fmt.Sprintf("repository_%s_%s_%s", org, repo, dt.Format("2006-01-02"))

	err = encodeAndStore(filename+".gob.gz", reqResp)
	if err != nil {
		panic(err)
	}

	err = encodeAndStoreTests(filename+".json", repoOracles)
	if err != nil {
		panic(err)
	}

	// reset map
	reqResp = make(map[string]testutils.Response)

	// record an org crawl
	log.Infof("Start recording an org")
	downloader.DownloadOrganization(ctx, org, 0)
	log.Infof("End recording an org")

	// create a struct with the oracle
	orgOracles := testutils.TestOracles{
		OrganizationTestOracles: []testutils.OrganizationTestOracle{
			testutils.OrganizationTestOracle{
				Org:               org,
				Version:           0,
				URL:               memory.Organization.URL,
				CreatedAt:         memory.Organization.CreatedAt.UTC().String(),
				PublicRepos:       memory.Organization.PublicRepos.TotalCount,
				TotalPrivateRepos: memory.Organization.TotalPrivateRepos.TotalCount,
				NumOfUsers:        len(memory.Users),
			},
		},
	}

	filename = fmt.Sprintf("organization_%s_%s", org, dt.Format("2006-01-02"))

	// store the results
	err = encodeAndStore(filename+".gob.gz", reqResp)
	if err != nil {
		panic(err)
	}
	err = encodeAndStoreTests(filename+".json", orgOracles)
	if err != nil {
		panic(err)
	}
}
