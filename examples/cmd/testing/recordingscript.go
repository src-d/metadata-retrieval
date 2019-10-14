package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/gob"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/src-d/metadata-retrieval/github"

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
		},
		LogResponse: func(resp *http.Response) {
			// TODO(@kyrcha): also record other types of responses
			if resp.StatusCode == http.StatusOK {
				reqResp[query] = cloneResponse(resp)
			}
		},
	}

	downloader, err := github.NewStdoutDownloader(client)
	if err != nil {
		panic(err)
	}

	// record a repo crawl
	log.Infof("Start recording a repo")
	downloader.DownloadRepository(ctx, org, repo, 0)
	log.Infof("End recording a repo")

	// store the results
	dt := time.Now()
	filename := fmt.Sprintf("repository_%s_%s_%s.gob.gz", org, repo, dt.Format("2006-01-02"))
	err = encodeAndStore(filename, reqResp)
	if err != nil {
		panic(err)
	}

	// reset map
	reqResp = make(map[string]string)

	// record an org crawl
	log.Infof("Start recording an org")
	downloader.DownloadOrganization(ctx, org, 0)
	log.Infof("End recording an org")

	// store the results
	filename = fmt.Sprintf("organization_%s_%s.gob.gz", org, dt.Format("2006-01-02"))
	err = encodeAndStore(filename, reqResp)
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
