// Integration tests of the metadata graphql crawler:
// - with and without the DB
// - online and offline tests

package github

import (
	"bytes"
	"compress/gzip"
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

	"github.com/cenkalti/backoff"
	"github.com/lib/pq"
	"github.com/shurcooL/githubv4"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"golang.org/x/oauth2"
)

const (
	orgPrefix        = "../testdata/organization_src-d_2019-10-15"
	repoPrefix       = "../testdata/repository_src-d_gitbase_2019-10-15"
	orgRecFile       = orgPrefix + ".gob.gz"
	repoRecFile      = repoPrefix + ".gob.gz"
	offlineRepoTests = orgPrefix + ".json"
	offlineOrgTests  = repoPrefix + ".json"
	onlineRepoTests  = "../testdata/online-repository-tests.json"
	onlineOrgTests   = "../testdata/online-organization-tests.json"
)

// loads requests-response data from a gob file
func loadReqResp(filepath string, reqResp map[string]string) error {
	// Open a file
	decodeFile, err := os.Open(filepath)
	if err != nil {
		return err
	}
	defer decodeFile.Close()
	reader, err := gzip.NewReader(decodeFile)
	if err != nil {
		return err
	}
	// Create a decoder and decode
	return gob.NewDecoder(reader).Decode(&reqResp)
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
	}
}

func isOSXOnTravis() bool {
	// docker service is not supported on osx in Travis: https://docs.travis-ci.com/user/docker/
	// but maybe in a local osx dev env so we will skip only in travis
	if runtime.GOOS == "darwin" && os.Getenv("TRAVIS") == "true" {
		return true
	}
	return false
}

// Testing connection documentation, docker-compose and Migrate method
func getDB(t *testing.T) (db *sql.DB) {
	require.NotEmpty(t, os.Getenv("PSQL_USER"), "PSQL_USER env var not set")
	require.NotEmpty(t, os.Getenv("PSQL_DB"), "PSQL_DB env var not set")
	// PSQL_PWD is not required in case someone wants to run the tests in a default local config
	DBURL := fmt.Sprintf("postgres://%s:%s@localhost:5432/%s?sslmode=disable", os.Getenv("PSQL_USER"), os.Getenv("PSQL_PWD"), os.Getenv("PSQL_DB"))
	err := backoff.Retry(func() error {
		var err error
		db, err = sql.Open("postgres", DBURL)
		return err
	}, backoff.NewExponentialBackOff())
	require.NoError(t, err, "DB URL is not working")
	if err = db.Ping(); err != nil {
		require.Nil(t, err, "DB connection is not working")
	}
	if err = database.Migrate(DBURL); err != nil {
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
// 1. Online (live) list repositories for git-fixtures org
// 2. Online (live) download with token for git-fixtures org and memory cache
// 3. Online (live) download with token for git-fixtures repos and memory cache
// 4. Offline (recorded) download without token for src-d/gitbase repo and memory cache
// 5. Offline (recorded) download without token for src-d org and memory cache
// 6. Online (live) download with token for git-fixtures org and DB
// 7. Online (live) download with token for git-fixtures repos and DB
// 8. Offline (recorded) download without token for src-d org and DB
// 9. Offline (recorded) download without token for src-d/gitbase repo and DB

type DownloaderTestSuite struct {
	suite.Suite
	db         *sql.DB
	downloader *Downloader
}

func (suite *DownloaderTestSuite) SetupSuite() {
	if !isOSXOnTravis() {
		suite.db = getDB(suite.T())
		suite.NotNil(suite.db)
	}
}

// TestOnlineRepositoryDownload Tests the listing of repositories of a known and fixed GitHub organization
func (suite *DownloaderTestSuite) TestOnlineListRepositories() {
	t := suite.T()
	checkToken(t)
	tests, err := loadTests(onlineReposListTests)
	suite.NoError(err, "Failed to read the testcases")
	downloader, _, err := getDownloader()
	suite.NoError(err, "Failed to instantiate downloader")

	var expectedRepos []string
	for _, test := range tests.RepositoryTests {
		expectedRepos = append(expectedRepos, test.Repository)
	}

	test := tests.OrganizationsTests[0]
	repos, err := downloader.ListRepositories(context.TODO(), test.Org, true)
	suite.NoError(err, "Error while listing repositories")

	suite.ElementsMatch(expectedRepos, repos)
}

// TestOnlineRepositoryDownload Tests the download of known and fixed GitHub repositories
func (suite *DownloaderTestSuite) TestOnlineRepositoryDownload() {
	t := suite.T()
	checkToken(t)
	tests, err := loadTests(onlineRepoTests)
	suite.NoError(err, "Failed to read the testcases")
	downloader, storer, err := getDownloader()
	suite.NoError(err, "Failed to instantiate downloader")
	for _, test := range tests.RepositoryTests {
		test := test // pinned, see scopelint for more info
		t.Run(fmt.Sprintf("Repo: %s/%s", test.Owner, test.Repository), func(t *testing.T) {
			testRepo(t, test, downloader, storer, false)
		})
	}
}

func testRepo(t *testing.T, oracle testutils.RepositoryTest, d *Downloader, storer *testutils.Memory, strict bool) {
	err := d.DownloadRepository(context.TODO(), oracle.Owner, oracle.Repository, oracle.Version)
	require := require.New(t) // Make a new require object for the specified test, so no need to pass it around
	require.Nil(err)
	// Sample some properties that will not change, no topics available in git-fixtures
	require.Equal(oracle.URL, storer.Repository.URL)
	require.Equal(oracle.CreatedAt, storer.Repository.CreatedAt.UTC().String())
	require.Equal(oracle.IsPrivate, storer.Repository.IsPrivate)
	require.Equal(oracle.IsArchived, storer.Repository.IsArchived)
	require.Equal(oracle.HasWiki, storer.Repository.HasWikiEnabled)
	require.ElementsMatch(oracle.Topics, storer.Topics)
	numOfPRReviews, numOfPRReviewComments := storer.CountPRReviewsAndReviewComments()
	if strict {
		require.Len(storer.PRs, oracle.NumOfPRs)
		require.Len(storer.PRComments, oracle.NumOfPRComments)
		require.Len(storer.Issues, oracle.NumOfIssues)
		require.Len(storer.IssueComments, oracle.NumOfIssueComments)
		require.Equal(oracle.NumOfPRReviews, numOfPRReviews)
		require.Equal(oracle.NumOfPRReviewComments, numOfPRReviewComments)
	} else {
		require.GreaterOrEqual(oracle.NumOfPRs, len(storer.PRs))
		require.GreaterOrEqual(oracle.NumOfPRComments, len(storer.PRComments))
		require.GreaterOrEqual(oracle.NumOfIssues, len(storer.Issues))
		require.GreaterOrEqual(oracle.NumOfIssueComments, len(storer.IssueComments))
		require.GreaterOrEqual(oracle.NumOfPRReviews, numOfPRReviews)
		require.GreaterOrEqual(oracle.NumOfPRReviewComments, numOfPRReviewComments)
	}
}

// TestOnlineOrganizationDownload Tests the download of known and fixed GitHub organization
func (suite *DownloaderTestSuite) TestOnlineOrganizationDownload() {
	t := suite.T()
	checkToken(t)
	tests, err := loadTests(onlineOrgTests)
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
	// Sample some properties that will not change, no topics available in git-fixtures
	require.Equal(oracle.Org, storer.Organization.Login)
	require.Equal(oracle.URL, storer.Organization.URL)
	require.Equal(oracle.CreatedAt, storer.Organization.CreatedAt.UTC().String())
	require.Equal(oracle.PublicRepos, storer.Organization.PublicRepos.TotalCount)
	require.Equal(oracle.TotalPrivateRepos, storer.Organization.TotalPrivateRepos.TotalCount)
	require.Len(storer.Users, oracle.NumOfUsers)
}

func getRoundTripDownloader(reqResp map[string]string, storer storer) *Downloader {
	return &Downloader{
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
	downloader := getRoundTripDownloader(reqResp, storer)
	tests, err := loadTests(offlineOrgTests)
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
	downloader := getRoundTripDownloader(reqResp, storer)
	tests, err := loadTests(offlineRepoTests)
	suite.NoError(err, "Failed to read the offline tests")
	for _, test := range tests.RepositoryTests {
		test := test
		t.Run(fmt.Sprintf("Repo: %s/%s", test.Owner, test.Repository), func(t *testing.T) {
			testRepo(t, test, downloader, storer, true)
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
	require.Equal(oracle.CreatedAt, createdAt.UTC().String())
	require.Equal(oracle.PublicRepos, numOfPublicRepos)
	require.Equal(oracle.TotalPrivateRepos, numOfPrivateRepos)
	require.Equal(oracle.NumOfUsers, numOfUsers)
}

// TestOnlineOrganizationDownloadWithDB Tests the download of known and fixed GitHub repositories and stores them in a Postgresql DB
func (suite *DownloaderTestSuite) TestOnlineOrganizationDownloadWithDB() {
	t := suite.T()
	checkToken(t)
	tests, err := loadTests(onlineOrgTests)
	suite.NoError(err, "Failed to read the online tests")
	downloader, err := NewDownloader(oauth2.NewClient(
		context.TODO(),
		oauth2.StaticTokenSource(
			&oauth2.Token{AccessToken: os.Getenv("GITHUB_TOKEN")},
		)), suite.db)
	suite.NoError(err, "Failed to init the downloader")
	downloader.SetActiveVersion(context.TODO(), 0)
	suite.downloader = downloader
	for _, test := range tests.OrganizationsTests {
		test := test
		t.Run(fmt.Sprintf("Org %s with DB", test.Org), func(t *testing.T) {
			testOrgWithDB(t, test, downloader, suite.db)
		})
	}
}

func testRepoWithDB(t *testing.T, oracle testutils.RepositoryTest, d *Downloader, db *sql.DB, strict bool) {
	err := d.DownloadRepository(context.TODO(), oracle.Owner, oracle.Repository, oracle.Version)
	require := require.New(t) // Make a new require object for the specified test, so no need to pass it around
	require.Nil(err)
	checkRepo(require, db, oracle, strict)
	checkIssues(require, db, oracle, strict)
	checkIssuePRComments(require, db, oracle, strict)
	checkPRs(require, db, oracle, strict)
	checkPRReviewComments(require, db, oracle, strict)
}

func checkIssues(require *require.Assertions, db *sql.DB, oracle testutils.RepositoryTest, strict bool) {
	var numOfIssues int
	err := db.QueryRow("select count(*) from issues where repository_owner = $1 and repository_name = $2", oracle.Owner, oracle.Repository).Scan(&numOfIssues)
	require.NoError(err, "Error in retrieving issues")
	if strict {
		require.Equal(oracle.NumOfIssues, numOfIssues, "Issues")
	} else {
		require.GreaterOrEqual(oracle.NumOfIssues, numOfIssues, "Issues")
	}

}

func checkIssuePRComments(require *require.Assertions, db *sql.DB, oracle testutils.RepositoryTest, strict bool) {
	var numOfComments int
	err := db.QueryRow("select count(*) from issue_comments where repository_owner = $1 and repository_name = $2", oracle.Owner, oracle.Repository).Scan(&numOfComments)
	require.NoError(err, "Error in retrieving issue comments")
	// NB: ghsync saves both Issue and PRs comments in the same table, issue_comments => See store/db.go comment
	if strict {
		require.Equal(oracle.NumOfPRComments+oracle.NumOfIssueComments, numOfComments, "Issue and PR Comments")
	} else {
		require.GreaterOrEqual(oracle.NumOfPRComments+oracle.NumOfIssueComments, numOfComments, "Issue and PR Comments")
	}
}

func checkPRs(require *require.Assertions, db *sql.DB, oracle testutils.RepositoryTest, strict bool) {
	var numOfPRs int
	err := db.QueryRow("select count(*) from pull_requests where repository_owner = $1 and repository_name = $2", oracle.Owner, oracle.Repository).Scan(&numOfPRs)
	require.NoError(err, "Error in retrieving pull requests")
	if strict {
		require.Equal(oracle.NumOfPRs, numOfPRs, "PRs")
	} else {
		require.GreaterOrEqual(oracle.NumOfPRs, numOfPRs, "PRs")
	}
}

func checkPRReviewComments(require *require.Assertions, db *sql.DB, oracle testutils.RepositoryTest, strict bool) {
	var numOfPRReviewComments int
	err := db.QueryRow("select count(*) from pull_request_comments where repository_owner = $1 and repository_name = $2", oracle.Owner, oracle.Repository).Scan(&numOfPRReviewComments)
	require.NoError(err, "Error in retrieving pull request comments")
	if strict {
		require.Equal(oracle.NumOfPRReviewComments, numOfPRReviewComments, "PR Review Comments")
	} else {
		require.GreaterOrEqual(oracle.NumOfPRReviewComments, numOfPRReviewComments, "PR Review Comments")
	}
}

func checkRepo(require *require.Assertions, db *sql.DB, oracle testutils.RepositoryTest, strict bool) {
	var (
		htmlurl   string
		createdAt time.Time
		private   bool
		archived  bool
		hasWiki   bool
		topics    []string
	)
	err := db.QueryRow("select htmlurl, created_at, private, archived, has_wiki, topics from repositories where owner_login = $1 and name = $2", oracle.Owner, oracle.Repository).Scan(&htmlurl, &createdAt, &private, &archived, &hasWiki, pq.Array(&topics))
	require.NoError(err, "Error in retrieving repo")
	require.Equal(oracle.URL, htmlurl)
	require.Equal(oracle.CreatedAt, createdAt.UTC().String())
	require.Equal(oracle.IsPrivate, private)
	require.Equal(oracle.IsArchived, archived)
	require.Equal(oracle.HasWiki, hasWiki)
	require.ElementsMatch(oracle.Topics, topics)
}

// TestOnlineRepositoryDownloadWithDB Tests the download of known and fixed GitHub organization and stores it in a Postgresql DB
func (suite *DownloaderTestSuite) TestOnlineRepositoryDownloadWithDB() {
	t := suite.T()
	checkToken(t)
	tests, err := loadTests(onlineRepoTests)
	suite.NoError(err, "Failed to read the online tests")
	downloader, err := NewDownloader(oauth2.NewClient(
		context.TODO(),
		oauth2.StaticTokenSource(
			&oauth2.Token{AccessToken: os.Getenv("GITHUB_TOKEN")},
		)), suite.db)
	suite.NoError(err, "Failed to init the downloader")
	downloader.SetActiveVersion(context.TODO(), 0)
	suite.downloader = downloader
	for _, test := range tests.RepositoryTests {
		test := test
		t.Run(fmt.Sprintf("Repo %s/%s with DB", test.Owner, test.Repository), func(t *testing.T) {
			testRepoWithDB(t, test, downloader, suite.db, false)
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
	storer := &store.DB{DB: suite.db}
	downloader := getRoundTripDownloader(reqResp, storer)
	downloader.SetActiveVersion(context.TODO(), 0) // Will create the views
	suite.downloader = downloader
	tests, err := loadTests(offlineOrgTests)
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
	storer := &store.DB{DB: suite.db}
	downloader := getRoundTripDownloader(reqResp, storer)
	downloader.SetActiveVersion(context.TODO(), 0)
	suite.downloader = downloader
	tests, err := loadTests(offlineRepoTests)
	suite.NoError(err, "Failed to read the offline tests")
	for _, test := range tests.RepositoryTests {
		test := test
		t.Run(fmt.Sprintf("%s/%s", test.Owner, test.Repository), func(t *testing.T) {
			testRepoWithDB(t, test, downloader, suite.db, true)
		})
	}
}

func (suite *DownloaderTestSuite) BeforeTest(suiteName, testName string) {
	if strings.HasSuffix(testName, "WithDB") && isOSXOnTravis() {
		suite.T().Skip("Don't test OSX with docker psql")
	}
}

// AfterTest after specific tests that use the DB, cleans up the DB for later tests
func (suite *DownloaderTestSuite) AfterTest(suiteName, testName string) {
	if testName == "TestOnlineOrganizationDownloadWithDB" || testName == "TestOfflineOrganizationDownloadWithDB" {
		// I cleanup with a different version (1 vs. 0), so to clean all the data from the DB
		suite.downloader.Cleanup(context.TODO(), 1)
		// Check
		var countOrgs int
		err := suite.db.QueryRow("select count(*) from organizations").Scan(&countOrgs)
		suite.NoError(err, "Failed to count the orgs")
		suite.Equal(0, countOrgs)
	} else if testName == "TestOnlineRepositoryDownloadWithDB" || testName == "TestOfflineRepositoryDownloadWithDB" {
		// I cleanup with a different version (1 vs. 0), so to clean all the data from the DB
		suite.downloader.Cleanup(context.TODO(), 1)
		// Check
		var countRepos int
		err := suite.db.QueryRow("select count(*) from repositories").Scan(&countRepos)
		suite.NoError(err, "Failed to count the repos")
		suite.Equal(0, countRepos)
	}
}

// Run the suite
func TestDownloaderTestSuite(t *testing.T) {
	suite.Run(t, new(DownloaderTestSuite))
}
