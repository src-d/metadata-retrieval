// Integration tests of the metadata graphql crawler:
// - with and without the DB
// - online and offline tests

package github

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/gob"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/src-d/metadata-retrieval/database"
	"github.com/src-d/metadata-retrieval/github/store"
	"github.com/src-d/metadata-retrieval/testutils"

	"github.com/golang-migrate/migrate/v4"
	"github.com/lib/pq"
	"github.com/shurcooL/githubv4"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"golang.org/x/oauth2"
)

const orgRecFile = "../testdata/organization_src-d_2019-10-10.gob"
const repoRecFile = "../testdata/repository_src-d_gitbase_2019-10-10.gob"

// loads requests-response data from a gob file
func loadReqResp(filepath string, reqResp map[string]string) error {
	// Open a file
	decodeFile, err := os.Open(filepath)
	if err != nil {
		return err
	}
	defer decodeFile.Close()
	// Create a decoder and decode
	return gob.NewDecoder(decodeFile).Decode(&reqResp)
}

// loads tests from a json file
func loadTests(filepath string) (testutils.Tests, error) {
	var (
		err   error
		tests testutils.Tests
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

// checks whether a token exists as an env var, if not it skips the test
func checkToken(t *testing.T) {
	if os.Getenv("GITHUB_TOKEN") == "" {
		t.Skip("GITHUB_TOKEN is not set")
		return
	}
}

func isOSX() bool {
	// docker service is not supported on osx in Travis: https://docs.travis-ci.com/user/docker/
	if runtime.GOOS == "darwin" {
		return true
	}
	return false
}

// Testing connection documentation, docker-compose and Migrate method
func getDB(t *testing.T) (db *sql.DB) {
	const DBURL = "postgres://user:password@localhost:5432/ghsync?sslmode=disable"
	db, err := sql.Open("postgres", DBURL)
	require.NoError(t, err, "DB URL is not working")
	if err = db.Ping(); err != nil {
		require.Nil(t, err, "DB connection is not working")
	}
	if err = database.Migrate(DBURL); err != nil && err != migrate.ErrNoChange {
		require.Nil(t, err, "Cannot migrate the DB")
	}
	return db
}

// RoundTripFunc a function type that gets a request and returns a response
type RoundTripFunc func(req *http.Request) *http.Response

// RoundTrip function to implement the interface of a RoundTripper Transport
func (f RoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req), nil
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

// Tests:
// 1. Online (live) download with token for git-fixtures org and memory cache
// 2. Online (live) download with token for git-fixtures repos and memory cache
// 4. Offline (recorded) download without token for src-d/gitbase repo and memory cache
// 3. Offline (recorded) download without token for src-d org and memory cache
// 5. Online (live) download with token for git-fixtures org and DB
// 6. Online (live) download with token for git-fixtures repos and DB
// 7. Offline (recorded) download without token for src-d org and DB
// 8. Offline (recorded) download without token for src-d/gitbase repo and DB

type DownloaderTestSuite struct {
	suite.Suite
	db         *sql.DB
	downloader *Downloader
}

func (suite *DownloaderTestSuite) SetupSuite() {
	if !isOSX() {
		suite.db = getDB(suite.T())
		suite.NotNil(suite.db)
	}
}

// TestOnlineRepositoryDownload Tests the download of known and fixed GitHub repositories
func (suite *DownloaderTestSuite) TestOnlineRepositoryDownload() {
	t := suite.T()
	checkToken(t)
	tests, err := loadTests("../testdata/online-repository-tests.json")
	suite.NoError(err, "Failed to read the testcases")
	downloader, storer, err := getDownloader()
	suite.NoError(err, "Failed to instantiate downloader")
	for _, test := range tests.RepositoryTests {
		test := test // pinned, see scopelint for more info
		t.Run(fmt.Sprintf("Repo: %s/%s", test.Owner, test.Repository), func(t *testing.T) {
			testRepo(t, test, downloader, storer)
		})
	}
}

func testRepo(t *testing.T, oracle testutils.RepositoryTest, d *Downloader, storer *testutils.Memory) {
	err := d.DownloadRepository(context.TODO(), oracle.Owner, oracle.Repository, oracle.Version)
	require := require.New(t) // Make a new require object for the specified test, so no need to pass it around
	require.Nil(err)
	require.Equal(oracle.URL, storer.Repository.Url)
	require.Equal(oracle.CreatedAt, storer.Repository.CreatedAt.String())
	require.Equal(oracle.IsPrivate, storer.Repository.IsPrivate)
	require.Equal(oracle.IsArchived, storer.Repository.IsArchived)
	require.Equal(oracle.HasWiki, storer.Repository.HasWikiEnabled)
	require.ElementsMatch(oracle.Topics, storer.Topics)
	require.Len(storer.PRs, oracle.NumOfPRs)
	require.Len(storer.PRComments, oracle.NumOfPRComments)
	require.Len(storer.Issues, oracle.NumOfIssues)
	require.Len(storer.IssueComments, oracle.NumOfIssueComments)
	numOfPRReviews, numOfPRReviewComments := storer.CountPRReviewsAndReviewComments()
	require.Equal(oracle.NumOfPRReviews, numOfPRReviews)
	require.Equal(oracle.NumOfPRReviewComments, numOfPRReviewComments)
}

// TestOnlineOrganizationDownload Tests the download of known and fixed GitHub organization
func (suite *DownloaderTestSuite) TestOnlineOrganizationDownload() {
	t := suite.T()
	checkToken(t)
	tests, err := loadTests("../testdata/online-organization-tests.json")
	suite.NoError(err, "Failed to read the testcases")
	downloader, storer, err := getDownloader()
	suite.NoError(err, "Failed to instantiate downloader")
	for _, test := range tests.OrganizationsTests {
		test := test
		t.Run(fmt.Sprintf("Org: %s", test.Org), func(t *testing.T) {
			testOrg(t, test, downloader, storer)
		})
	}
}

func testOrg(t *testing.T, oracle testutils.OrganizationTest, d *Downloader, storer *testutils.Memory) {
	err := d.DownloadOrganization(context.TODO(), oracle.Org, oracle.Version)
	require := require.New(t)
	require.Nil(err, "DownloadOrganization(%s) failed", oracle.Org)
	require.Equal(oracle.Org, storer.Organization.Login)
	require.Equal(oracle.URL, storer.Organization.Url)
	require.Equal(oracle.CreatedAt, storer.Organization.CreatedAt.String())
	require.Equal(oracle.PublicRepos, storer.Organization.PublicRepos.TotalCount)
	require.Equal(oracle.TotalPrivateRepos, storer.Organization.TotalPrivateRepos.TotalCount)
	require.Len(storer.Users, oracle.NumOfUsers)
}

// TestOfflineOrganizationDownload Tests a large organization by replaying recorded responses
func (suite *DownloaderTestSuite) TestOfflineOrganizationDownload() {
	t := suite.T()
	reqResp := make(map[string]string)
	// Load the recording
	suite.NoError(loadReqResp(orgRecFile, reqResp), "Failed to read the offline recordings")
	// Setup the downloader with RoundTrip functionality.
	// Not using the NewStdoutDownloader initialization because it overides the transport
	storer := &testutils.Memory{}
	downloader := &Downloader{
		storer: storer,
		client: githubv4.NewClient(&http.Client{
			Transport: RoundTripFunc(func(req *http.Request) *http.Response {
				// consume request body
				savecl := req.ContentLength
				bodyBytes, _ := ioutil.ReadAll(req.Body)
				defer req.Body.Close()
				// recreate request body
				req.Body = ioutil.NopCloser(bytes.NewBuffer(bodyBytes))
				req.ContentLength = savecl
				data := reqResp[string(bodyBytes)]
				return &http.Response{
					StatusCode: 200,
					Body:       ioutil.NopCloser(bytes.NewBufferString(data)),
					Header:     make(http.Header),
				}
			})}),
	}
	tests, err := loadTests("../testdata/offline-organization-tests.json")
	suite.NoError(err, "Failed to read the offline tests")
	for _, test := range tests.OrganizationsTests {
		test := test
		t.Run(fmt.Sprintf("Org: %s", test.Org), func(t *testing.T) {
			testOrg(t, test, downloader, storer)
		})
	}
}

// TestOfflineRepositoryDownload Tests a large repository by replaying recorded responses
func (suite *DownloaderTestSuite) TestOfflineRepositoryDownload() {
	t := suite.T()
	reqResp := make(map[string]string)
	suite.NoError(loadReqResp(repoRecFile, reqResp), "Failed to read the offline recordings")
	storer := &testutils.Memory{}
	downloader := &Downloader{
		storer: storer,
		client: githubv4.NewClient(&http.Client{
			Transport: RoundTripFunc(func(req *http.Request) *http.Response {
				// consume request body
				savecl := req.ContentLength
				bodyBytes, _ := ioutil.ReadAll(req.Body)
				defer req.Body.Close()
				// recreate request body
				req.Body = ioutil.NopCloser(bytes.NewBuffer(bodyBytes))
				req.ContentLength = savecl
				data := reqResp[string(bodyBytes)]
				return &http.Response{
					StatusCode: 200,
					Body:       ioutil.NopCloser(bytes.NewBufferString(data)),
					Header:     make(http.Header),
				}
			})}),
	}
	tests, err := loadTests("../testdata/offline-repository-tests.json")
	suite.NoError(err, "Failed to read the offline tests")
	for _, test := range tests.RepositoryTests {
		test := test
		t.Run(fmt.Sprintf("Repo: %s/%s", test.Owner, test.Repository), func(t *testing.T) {
			testRepo(t, test, downloader, storer)
		})
	}
}

func testOrgWithDB(t *testing.T, oracle testutils.OrganizationTest, d *Downloader, db *sql.DB) {
	err := d.DownloadOrganization(context.TODO(), oracle.Org, oracle.Version)
	require := require.New(t) // Make a new require object for the specified test, so no need to pass it around
	require.NoError(err, "Error in downloading")
	var (
		htmlurl           string
		createdAt         time.Time
		numOfPublicRepos  int
		numOfPrivateRepos int
		numOfUsers        int
	)
	// Retrieve data
	err = db.QueryRow("select htmlurl, created_at, public_repos, total_private_repos from organizations where login = $1", oracle.Org).Scan(&htmlurl, &createdAt, &numOfPublicRepos, &numOfPrivateRepos)
	require.NoError(err, "Error in retrieving orgs")
	// TODO(@kyrcha): when schema is updated add query: select count(*) from users where owner = "src-d" for example
	err = db.QueryRow("select count(*) from users").Scan(&numOfUsers)
	require.NoError(err, "Error in retrieving users")
	// Checks
	require.Equal(oracle.URL, htmlurl)
	require.Equal(oracle.CreatedAt, createdAt.String())
	require.Equal(oracle.PublicRepos, numOfPublicRepos)
	require.Equal(oracle.TotalPrivateRepos, numOfPrivateRepos)
	require.Equal(oracle.NumOfUsers, numOfUsers)
}

// TestOnlineOrganizationDownloadWithDB Tests the download of known and fixed GitHub repositories and stores them in a Postgresql DB
func (suite *DownloaderTestSuite) TestOnlineOrganizationDownloadWithDB() {
	t := suite.T()
	checkToken(t)
	tests, err := loadTests("../testdata/online-organization-tests.json")
	suite.NoError(err, "Failed to read the online tests")
	downloader, err := NewDownloader(oauth2.NewClient(
		context.TODO(),
		oauth2.StaticTokenSource(
			&oauth2.Token{AccessToken: os.Getenv("GITHUB_TOKEN")},
		)), suite.db)
	suite.NoError(err, "Failed to init the downloader")
	suite.downloader = downloader
	for _, test := range tests.OrganizationsTests {
		test := test
		t.Run(fmt.Sprintf("Org %s with DB", test.Org), func(t *testing.T) {
			testOrgWithDB(t, test, downloader, suite.db)
		})
	}
}

func testRepoWithDB(t *testing.T, oracle testutils.RepositoryTest, d *Downloader, db *sql.DB) {
	err := d.DownloadRepository(context.TODO(), oracle.Owner, oracle.Repository, oracle.Version)
	require := require.New(t) // Make a new require object for the specified test, so no need to pass it around
	require.Nil(err)
	var (
		htmlurl               string
		createdAt             time.Time
		private               bool
		archived              bool
		hasWiki               bool
		topics                []string
		numOfIssues           int
		numOfIssueComments    int
		numOfPRs              int
		numOfPRReviewComments int
	)
	err = db.QueryRow("select htmlurl, created_at, private, archived, has_wiki, topics from repositories where owner_login = $1 and name = $2", oracle.Owner, oracle.Repository).Scan(&htmlurl, &createdAt, &private, &archived, &hasWiki, pq.Array(&topics))
	require.NoError(err, "Error in retrieving repo")
	err = db.QueryRow("select count(*) from issues where repository_owner = $1 and repository_name = $2", oracle.Owner, oracle.Repository).Scan(&numOfIssues)
	require.NoError(err, "Error in retrieving issues")
	err = db.QueryRow("select count(*) from issue_comments where repository_owner = $1 and repository_name = $2", oracle.Owner, oracle.Repository).Scan(&numOfIssueComments)
	require.NoError(err, "Error in retrieving issue comments")
	err = db.QueryRow("select count(*) from pull_requests where repository_owner = $1 and repository_name = $2", oracle.Owner, oracle.Repository).Scan(&numOfPRs)
	require.NoError(err, "Error in retrieving pull requests")
	err = db.QueryRow("select count(*) from pull_request_comments where repository_owner = $1 and repository_name = $2", oracle.Owner, oracle.Repository).Scan(&numOfPRReviewComments)
	require.NoError(err, "Error in retrieving pull request comments")
	require.Equal(oracle.URL, htmlurl)
	require.Equal(oracle.CreatedAt, createdAt.String())
	require.Equal(oracle.IsPrivate, private)
	require.Equal(oracle.IsArchived, archived)
	require.Equal(oracle.HasWiki, hasWiki)
	require.ElementsMatch(oracle.Topics, topics)
	require.Equal(numOfIssues, oracle.NumOfIssues, "Issues")
	require.Equal(oracle.NumOfPRs, numOfPRs, "PRs")
	// NB: ghsync saves both Issue and PRs comments in the same table, issue_comments => See store/db.go comment
	require.Equal((oracle.NumOfPRComments + oracle.NumOfIssueComments), numOfIssueComments, "Issue and PR Comments")
	require.Equal(oracle.NumOfPRReviewComments, numOfPRReviewComments, "PR Review Comments")
}

// TestOnlineRepositoryDownloadWithDB Tests the download of known and fixed GitHub organization and stores it in a Postgresql DB
func (suite *DownloaderTestSuite) TestOnlineRepositoryDownloadWithDB() {
	t := suite.T()
	checkToken(t)
	tests, err := loadTests("../testdata/online-repository-tests.json")
	suite.NoError(err, "Failed to read the online tests")
	downloader, err := NewDownloader(oauth2.NewClient(
		context.TODO(),
		oauth2.StaticTokenSource(
			&oauth2.Token{AccessToken: os.Getenv("GITHUB_TOKEN")},
		)), suite.db)
	suite.NoError(err, "Failed to init the downloader")
	suite.downloader = downloader
	for _, test := range tests.RepositoryTests {
		test := test
		t.Run(fmt.Sprintf("Repo %s/%s with DB", test.Owner, test.Repository), func(t *testing.T) {
			testRepoWithDB(t, test, downloader, suite.db)
		})
	}
}

// TestOfflineOrganizationDownloadWithDB Tests a large organization by replaying recorded responses and storing the results in Postgresql
func (suite *DownloaderTestSuite) TestOfflineOrganizationDownloadWithDB() {
	t := suite.T()
	reqResp := make(map[string]string)
	// Load the recording
	suite.NoError(loadReqResp(orgRecFile, reqResp), "Failed to read the recordings")
	// Setup the downloader with RoundTrip functionality.
	// Not using the NewStdoutDownloader initialization because it overides the transport
	downloader := &Downloader{
		storer: &store.DB{DB: suite.db},
		client: githubv4.NewClient(&http.Client{
			Transport: RoundTripFunc(func(req *http.Request) *http.Response {
				// consume request body
				savecl := req.ContentLength
				bodyBytes, _ := ioutil.ReadAll(req.Body)
				defer req.Body.Close()
				// recreate request body
				req.Body = ioutil.NopCloser(bytes.NewBuffer(bodyBytes))
				req.ContentLength = savecl
				data := reqResp[string(bodyBytes)]
				return &http.Response{
					StatusCode: 200,
					Body:       ioutil.NopCloser(bytes.NewBufferString(data)),
					Header:     make(http.Header),
				}
			})}),
	}
	suite.downloader = downloader
	tests, err := loadTests("../testdata/offline-organization-tests.json")
	suite.NoError(err, "Failed to read the offline tests")
	for _, test := range tests.OrganizationsTests {
		test := test
		t.Run(fmt.Sprintf("%s", test.Org), func(t *testing.T) {
			testOrgWithDB(t, test, downloader, suite.db)
		})
	}
}

// TestOfflineRepositoryDownload Tests a large repository by replaying recorded responses and stores the results in postgresql
func (suite *DownloaderTestSuite) TestOfflineRepositoryDownloadWithDB() {
	t := suite.T()
	reqResp := make(map[string]string)
	// Load the recording
	suite.NoError(loadReqResp(repoRecFile, reqResp), "Failed to read the recordings")
	downloader := &Downloader{
		storer: &store.DB{DB: suite.db},
		client: githubv4.NewClient(&http.Client{
			Transport: RoundTripFunc(func(req *http.Request) *http.Response {
				// consume request body
				savecl := req.ContentLength
				bodyBytes, _ := ioutil.ReadAll(req.Body)
				defer req.Body.Close()
				// recreate request body
				req.Body = ioutil.NopCloser(bytes.NewBuffer(bodyBytes))
				req.ContentLength = savecl
				data := reqResp[string(bodyBytes)]
				return &http.Response{
					StatusCode: 200,
					Body:       ioutil.NopCloser(bytes.NewBufferString(data)),
					Header:     make(http.Header),
				}
			})}),
	}
	tests, err := loadTests("../testdata/offline-repository-tests.json")
	suite.NoError(err, "Failed to read the offline tests")
	for _, test := range tests.RepositoryTests {
		test := test
		t.Run(fmt.Sprintf("%s/%s", test.Owner, test.Repository), func(t *testing.T) {
			testRepoWithDB(t, test, downloader, suite.db)
		})
	}
}

func (suite *DownloaderTestSuite) BeforeTest(suiteName, testName string) {
	if strings.HasSuffix(testName, "WithDB") && isOSX() {
		suite.T().Skip("Don't test OSX with docker psql")
	}
}

// AfterTest after specific tests that use the DB, cleans up the DB for later tests
func (suite *DownloaderTestSuite) AfterTest(suiteName, testName string) {
	if testName == "TestOnlineOrganizationDownloadWithDB" || testName == "TestOfflineOrganizationDownloadWithDB" {
		// I cleanup with a different version (1 vs. 0), so to clean all the data from the DB
		suite.downloader.Cleanup(1)
		// Check
		var countOrgs int
		err := suite.db.QueryRow("select count(*) from organizations_versioned").Scan(&countOrgs)
		suite.NoError(err, "Failed to count the orgs")
		suite.Equal(0, countOrgs)
	} else if testName == "TestOnlineRepositoryDownloadWithDB" || testName == "TestOfflineRepositoryDownloadWithDB" {
		// I cleanup with a different version (1 vs. 0), so to clean all the data from the DB
		suite.downloader.Cleanup(1)
		// Check
		var countRepos int
		err := suite.db.QueryRow("select count(*) from repositories_versioned").Scan(&countRepos)
		suite.NoError(err, "Failed to count the repos")
		suite.Equal(0, countRepos)
	}
}

// Run the suite
func TestDownloaderTestSuite(t *testing.T) {
	suite.Run(t, new(DownloaderTestSuite))
}
