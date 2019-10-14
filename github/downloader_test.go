package github

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"testing"

	"github.com/src-d/metadata-retrieval/testutils"

	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
)

// RepositoryTest struct to hold a test oracle for a repository
type RepositoryTest struct {
	Owner           string `json:"owner"`
	Repository      string `json:"repository"`
	Version         int    `json:"version"`
	URL             string `json:"url"`
	CreatedAt       string `json:"createdAt"`
	IsPrivate       bool   `json:"isPrivate"`
	IsArchived      bool   `json:"isArchived"`
	HasWiki         bool   `json:"hasWiki"`
	NumOfPRs        int    `json:"numOfPrs"`
	NumOfPRComments int    `json:"numOfPrComments"`
}

// OrganizationTest struct to hold a test oracle for an organization
type OrganizationTest struct {
	Org               string `json:"org"`
	Version           int    `json:"version"`
	URL               string `json:"url"`
	CreatedAt         string `json:"createdAt"`
	PublicRepos       int    `json:"publicRepos"`
	TotalPrivateRepos int    `json:"totalPrivateRepos"`
	NumOfUsers        int    `json:"numOfUsers"`
}

// OnlineTests struct to hold the online tests
type OnlineTests struct {
	RepositoryTests    []RepositoryTest   `json:"repositoryTests"`
	OrganizationsTests []OrganizationTest `json:"organizationTests"`
}

func checkToken(t *testing.T) {
	if os.Getenv("GITHUB_TOKEN") == "" {
		t.Skip("GITHUB_TOKEN is not set")
		return
	}
}

func loadOnlineTests(filepath string) (OnlineTests, error) {
	var (
		err   error
		tests OnlineTests
	)
	jsonFile, err := os.Open(filepath)
	if err != nil {
		return tests, fmt.Errorf("Could not open json file: %v", err)
	}
	defer jsonFile.Close()
	byteValue, _ := ioutil.ReadAll(jsonFile)
	err = json.Unmarshal(byteValue, &tests)
	if err != nil {
		return tests, fmt.Errorf("Could not unmarshal json file: %v", err)
	}
	return tests, nil
}

func getDownloader() (*Downloader, *testutils.Memory, error) {
	downloader, err := NewStdoutDownloader(
		oauth2.NewClient(
			context.TODO(),
			oauth2.StaticTokenSource(
				&oauth2.Token{AccessToken: os.Getenv("GITHUB_TOKEN")},
			)))
	if err != nil {
		return nil, nil, err
	}

	storer := new(testutils.Memory)
	downloader.storer = storer
	return downloader, storer, nil
}

func testOnlineRepo(t *testing.T, oracle RepositoryTest, d *Downloader, storer *testutils.Memory) {
	err := d.DownloadRepository(context.TODO(), oracle.Owner, oracle.Repository, oracle.Version)
	require := require.New(t) // Make a new require object for the specified test, so no need to pass it around
	require.Nil(err)
	// Sample some properties that will not change, no topics available in git-fixtures
	require.Equal(oracle.URL, storer.Repository.URL)
	require.Equal(oracle.CreatedAt, storer.Repository.CreatedAt.String())
	require.Equal(oracle.IsPrivate, storer.Repository.IsPrivate)
	require.Equal(oracle.IsArchived, storer.Repository.IsArchived)
	require.Equal(oracle.HasWiki, storer.Repository.HasWikiEnabled)
	require.Len(storer.PRs, oracle.NumOfPRs)
	require.Len(storer.PRComments, oracle.NumOfPRComments)
}

// TestOnlineRepositoryDownload Tests the download of known and fixed GitHub repositories
func TestOnlineRepositoryDownload(t *testing.T) {
	checkToken(t)
	var err error
	tests, err := loadOnlineTests("../testdata/online-repository-tests.json")
	if err != nil {
		t.Errorf("Failed to read the testcases:%s", err)
	}

	downloader, storer, err := getDownloader()
	require.NoError(t, err)

	for _, test := range tests.RepositoryTests {
		t.Run(fmt.Sprintf("%s/%s", test.Owner, test.Repository), func(t *testing.T) {
			testOnlineRepo(t, test, downloader, storer)
		})
	}
}

func testOnlineOrg(t *testing.T, oracle OrganizationTest, d *Downloader, storer *testutils.Memory) {
	err := d.DownloadOrganization(context.TODO(), oracle.Org, oracle.Version)
	require := require.New(t)
	require.Nil(err, "DownloadOrganization(%s) failed", oracle.Org)
	// Sample some properties that will not change, no topics available in git-fixtures
	require.Equal(oracle.Org, storer.Organization.Name)
	require.Equal(oracle.URL, storer.Organization.URL)
	require.Equal(oracle.CreatedAt, storer.Organization.CreatedAt.String())
	require.Equal(oracle.PublicRepos, storer.Organization.PublicRepos.TotalCount)
	require.Equal(oracle.TotalPrivateRepos, storer.Organization.TotalPrivateRepos.TotalCount)
	require.Len(storer.Users, oracle.NumOfUsers)
}

// TestOnlineOrganizationDownload Tests the download of known and fixed GitHub organization
func TestOnlineOrganizationDownload(t *testing.T) {
	checkToken(t)
	var err error
	tests, err := loadOnlineTests("../testdata/online-organization-tests.json")
	if err != nil {
		t.Errorf("Failed to read the testcases:%s", err)
	}

	downloader, storer, err := getDownloader()
	require.NoError(t, err)

	for _, test := range tests.OrganizationsTests {
		t.Run(fmt.Sprintf("%s", test.Org), func(t *testing.T) {
			testOnlineOrg(t, test, downloader, storer)
		})
	}

}
