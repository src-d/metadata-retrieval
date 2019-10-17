package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/gob"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/src-d/metadata-retrieval/github"
	"github.com/src-d/metadata-retrieval/testutils"

	"github.com/motemen/go-loghttp"
	"golang.org/x/oauth2"
	"gopkg.in/src-d/go-log.v1"
)

// This script will use the downloader to crawl using the graphql API a repository (default: src-d/gitbase)
// and an organization (default: src-d) and will record requests (graphql queries) and responses (json body data)
// into a map stored in a gob file.
// NB: script is assumed to run from the project root (for correct file storage)
func main() {
	var (
		org  string
		repo string
	)
	flag.StringVar(&org, "org", "src-d", "a GitHub organization")
	flag.StringVar(&repo, "repo", "gitbase", "a GitHub repository")
	flag.Parse()

	// Variables to hold graphql queries (keys) and responses (values)
	var (
		reqResp map[string]string = make(map[string]string)
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

	client.Transport = &loghttp.Transport{
		Transport: client.Transport,
		LogRequest: func(req *http.Request) {
			query = cloneRequest(req)
			// https://stackoverflow.com/a/19006050/869151
			req.Close = true
		},
		LogResponse: func(resp *http.Response) {
			// TODO(@kyrcha): also record other types of responses
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
	reqResp = make(map[string]string)

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

func cloneRequest(req *http.Request) string {
	savecl := req.ContentLength
	bodyBytes, _ := ioutil.ReadAll(req.Body)
	defer req.Body.Close()
	// recreate request body
	req.Body = ioutil.NopCloser(bytes.NewBuffer(bodyBytes))
	req.ContentLength = savecl
	return string(bodyBytes)
}

func cloneResponse(resp *http.Response) string {
	// consume response body
	savecl := resp.ContentLength
	bodyBytes, _ := ioutil.ReadAll(resp.Body)
	defer resp.Body.Close()
	// recreate response body
	resp.Body = ioutil.NopCloser(bytes.NewBuffer(bodyBytes))
	resp.ContentLength = savecl
	// save response body
	return string(bodyBytes)
}

func encodeAndStore(filename string, reqResp map[string]string) error {
	filepath := filepath.Join("testdata", filename)
	encodeFile, err := os.Create(filepath)
	if err != nil {
		return err
	}
	defer encodeFile.Close()
	zw := gzip.NewWriter(encodeFile)
	defer zw.Close()
	return gob.NewEncoder(zw).Encode(reqResp)
}

func encodeAndStoreTests(filename string, tests testutils.TestOracles) error {
	filepath := filepath.Join("testdata", filename)
	data, err := json.MarshalIndent(tests, "", "\t")
	if err != nil {
		return err
	}
	err = ioutil.WriteFile(filepath, data, 0644)
	if err != nil {
		return err
	}
	return nil
}
