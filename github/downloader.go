package github

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"

	"github.com/src-d/metadata-retrieval/github/graphql"
	"github.com/src-d/metadata-retrieval/github/store"
	"github.com/src-d/metadata-retrieval/utils/ctxlog"

	"github.com/shurcooL/githubv4"
	"gopkg.in/src-d/go-log.v1"
)

type connectionType struct {
	Name     string
	PageSize githubv4.Int
}

func (c connectionType) Page() string   { return fmt.Sprintf("%sPage", c.Name) }
func (c connectionType) Cursor() string { return fmt.Sprintf("%sCursor", c.Name) }

var (
	topicsType                    = connectionType{"repositoryTopics", 10}
	assigneesType                 = connectionType{"assignees", 2}
	issuesType                    = connectionType{"issues", 50}
	issueCommentsType             = connectionType{"issueComments", 10}
	pullRequestsType              = connectionType{"pullRequests", 50}
	pullRequestReviewsType        = connectionType{"pullRequestReviews", 5}
	pullRequestReviewCommentsType = connectionType{"pullRequestReviewComments", 5}
	labelsType                    = connectionType{"labels", 2}
	membersWithRole               = connectionType{"membersWithRole", 100}
)

type storer interface {
	SaveOrganization(ctx context.Context, organization *graphql.Organization) error
	SaveUser(ctx context.Context, orgID int, orgLogin string, user *graphql.UserExtended) error
	SaveRepository(ctx context.Context, repository *graphql.RepositoryFields, topics []string) error
	SaveIssue(ctx context.Context, repositoryOwner, repositoryName string, issue *graphql.Issue, assignees []string, labels []string) error
	SaveIssueComment(ctx context.Context, repositoryOwner, repositoryName string, issueNumber int, comment *graphql.IssueComment) error
	SavePullRequest(ctx context.Context, repositoryOwner, repositoryName string, pr *graphql.PullRequest, assignees []string, labels []string) error
	SavePullRequestComment(ctx context.Context, repositoryOwner, repositoryName string, pullRequestNumber int, comment *graphql.IssueComment) error
	SavePullRequestReview(ctx context.Context, repositoryOwner, repositoryName string, pullRequestNumber int, review *graphql.PullRequestReview) error
	SavePullRequestReviewComment(ctx context.Context, repositoryOwner, repositoryName string, pullRequestNumber int, pullRequestReviewID int, comment *graphql.PullRequestReviewComment) error

	Begin() error
	Commit() error
	Rollback() error
	Version(v int)
	SetActiveVersion(ctx context.Context, v int) error
	Cleanup(ctx context.Context, currentVersion int) error
}

// Downloader fetches GitHub data using the v4 API
type Downloader struct {
	storer
	client *githubv4.Client
}

// NewDownloader creates a new Downloader that will store the GitHub metadata
// in the given DB. The HTTP client is expected to have the proper
// authentication setup
func NewDownloader(httpClient *http.Client, db *sql.DB) (*Downloader, error) {
	// TODO: is the ghsync rate limited client needed?

	t := &retryTransport{httpClient.Transport}
	httpClient.Transport = t

	return &Downloader{
		storer: &store.DB{DB: db},
		client: githubv4.NewClient(httpClient),
	}, nil
}

// NewStdoutDownloader creates a new Downloader that will print the GitHub
// metadata to stdout. The HTTP client is expected to have the proper
// authentication setup
func NewStdoutDownloader(httpClient *http.Client) (*Downloader, error) {
	// TODO: is the ghsync rate limited client needed?

	t := &retryTransport{httpClient.Transport}
	httpClient.Transport = t

	return &Downloader{
		storer: &store.Stdout{},
		client: githubv4.NewClient(httpClient),
	}, nil
}

// DownloadRepository downloads the metadata for the given repository and all
// its resources (issues, PRs, comments, reviews)
func (d Downloader) DownloadRepository(ctx context.Context, owner string, name string, version int) error {
	ctx, _ = ctxlog.WithLogFields(ctx, log.Fields{"owner": owner, "repo": name})

	d.storer.Version(version)

	var err error
	err = d.storer.Begin()
	if err != nil {
		return fmt.Errorf("could not call Begin(): %v", err)
	}

	defer func() {
		if err != nil {
			d.storer.Rollback()
			return
		}

		d.storer.Commit()
	}()

	var q struct {
		graphql.Repository `graphql:"repository(owner: $owner, name: $name)"`
	}

	// Some variables are repeated in the query, like assigneesCursor for Issues
	// and PullRequests. It's ok to reuse because in this top level Repository
	// query the cursors are set to nil, and when the pagination occurs, the
	// queries only request either Issues or PullRequests
	variables := map[string]interface{}{
		"owner": githubv4.String(owner),
		"name":  githubv4.String(name),
	}
	connections := []connectionType{
		assigneesType, issueCommentsType, issuesType, labelsType, topicsType,
		pullRequestReviewCommentsType, pullRequestReviewsType, pullRequestsType,
	}
	for _, c := range connections {
		variables[c.Page()] = c.PageSize
		variables[c.Cursor()] = (*githubv4.String)(nil)
	}

	err = d.client.Query(ctx, &q, variables)
	if err != nil {
		return fmt.Errorf("first query failed: %v", err)
	}

	// repository topics
	topics, err := d.downloadTopics(ctx, &q.Repository)
	if err != nil {
		return err
	}

	err = d.storer.SaveRepository(ctx, &q.Repository.RepositoryFields, topics)
	if err != nil {
		return fmt.Errorf("failed to save repository %v: %v", q.Repository.NameWithOwner, err)
	}

	// issues and comments
	err = d.downloadIssues(ctx, owner, name, &q.Repository)
	if err != nil {
		return err
	}

	// PRs and comments
	err = d.downloadPullRequests(ctx, owner, name, &q.Repository)
	if err != nil {
		return err
	}

	return nil
}

func (d Downloader) ListRepositories(ctx context.Context, name string, noForks bool) ([]string, error) {
	repos := []string{}

	hasNextPage := true

	variables := map[string]interface{}{
		"login": githubv4.String(name),

		"repositoriesPage":   githubv4.Int(100),
		"repositoriesCursor": (*githubv4.String)(nil),
	}

	if noForks {
		variables["isFork"] = githubv4.Boolean(false)
	} else {
		variables["isFork"] = (*githubv4.Boolean)(nil)
	}

	for hasNextPage {
		var q struct {
			Organization struct {
				Repositories struct {
					PageInfo graphql.PageInfo
					Nodes    []struct {
						Name string
					}
				} `graphql:"repositories(first:$repositoriesPage, after: $repositoriesCursor, isFork: $isFork)"`
			} `graphql:"organization(login: $login)"`
		}

		err := d.client.Query(ctx, &q, variables)
		if err != nil {
			return nil, fmt.Errorf("failed to query organization %v repositories: %v", name, err)
		}

		for _, node := range q.Organization.Repositories.Nodes {
			repos = append(repos, node.Name)
		}

		hasNextPage = q.Organization.Repositories.PageInfo.HasNextPage
		variables["repositoriesCursor"] = githubv4.String(q.Organization.Repositories.PageInfo.EndCursor)
	}

	return repos, nil
}

// RateRemaining returns the remaining rate limit for the v4 GitHub API
func (d Downloader) RateRemaining(ctx context.Context) (int, error) {
	var q struct {
		RateLimit struct {
			Remaining int
		}
	}

	err := d.client.Query(ctx, &q, nil)
	if err != nil {
		return 0, fmt.Errorf("failed to query remaining rate limit: %v", err)
	}

	return q.RateLimit.Remaining, nil
}

// Connection is a unified interface for GraphQL connections
type Connection interface {
	Len() int
	GetTotalCount() int
	GetPageInfo() graphql.PageInfo
}

// Query is a GraphQL query that returns Connection
type Query interface {
	Connection() Connection
}

// getPerPage calculates how many resources to request based on total number and number of already downloaded
func getPerPage(total, count int, fallback, limit githubv4.Int) githubv4.Int {
	perPage := githubv4.Int(total - count)
	if perPage > limit {
		perPage = limit
	}
	// in case entities appeared during downloading process
	if perPage <= 0 {
		perPage = fallback
	}

	return perPage
}

func (d Downloader) downloadConnection(
	ctx context.Context,
	t connectionType,
	res Connection,
	q Query,
	variables map[string]interface{},
	process func(Connection) error,
) error {
	logger := ctxlog.Get(ctx)
	// logging only top-level resources
	isLoggable := t == issuesType || t == pullRequestsType || t == membersWithRole
	if isLoggable {
		logger.Infof("start downloading %s", t.Name)
		defer logger.Infof("finished downloading %s", t.Name)
	}

	// Save resources included in the first page
	if err := process(res); err != nil {
		return fmt.Errorf("can not process %s: %s", t.Name, err)
	}

	var count int
	limit := githubv4.Int(100)
	// github timeouts and returns 502 too often with 100 limit
	if t == pullRequestsType {
		limit = t.PageSize
	}
	for res.GetPageInfo().HasNextPage {
		count += res.Len()
		variables[t.Page()] = getPerPage(res.GetTotalCount(), count, t.PageSize, limit)
		variables[t.Cursor()] = githubv4.String(res.GetPageInfo().EndCursor)

		if isLoggable && count%int(t.PageSize) == 0 {
			logger.Infof("%d/%d %s downloaded", count, res.GetTotalCount(), t.Name)
		}

		if err := d.client.Query(ctx, q, variables); err != nil {
			return fmt.Errorf("query to %s failed: %s", t.Name, err)
		}

		res = q.Connection()
		if err := process(res); err != nil {
			return fmt.Errorf("can not process %s: %s", t.Name, err)
		}
	}

	return nil
}

type repositoryTopicsQ struct {
	Node struct {
		Repository struct {
			RepositoryTopics graphql.RepositoryTopicsConnection `graphql:"repositoryTopics(first: $repositoryTopicsPage, after: $repositoryTopicsCursor)"`
		} `graphql:"... on Repository"`
	} `graphql:"node(id:$id)"`
}

func (q *repositoryTopicsQ) Connection() Connection {
	return q.Node.Repository.RepositoryTopics
}

func (d Downloader) downloadTopics(ctx context.Context, repository *graphql.Repository) ([]string, error) {
	var q repositoryTopicsQ
	variables := map[string]interface{}{
		"id": githubv4.ID(repository.ID),
	}

	names := []string{}
	process := func(res Connection) error {
		topics := res.(graphql.RepositoryTopicsConnection)
		for _, topicNode := range topics.Nodes {
			names = append(names, topicNode.Topic.Name)
		}

		return nil
	}

	err := d.downloadConnection(ctx, topicsType, repository.RepositoryTopics, &q, variables, process)
	if err != nil {
		return nil, err
	}
	return names, err
}

type issuesQ struct {
	Node struct {
		Repository struct {
			Issues graphql.IssueConnection `graphql:"issues(first: $issuesPage, after: $issuesCursor)"`
		} `graphql:"... on Repository"`
	} `graphql:"node(id:$id)"`
}

func (q *issuesQ) Connection() Connection {
	return q.Node.Repository.Issues
}

func (d Downloader) downloadIssues(ctx context.Context, owner string, name string, repository *graphql.Repository) error {
	var q issuesQ
	variables := map[string]interface{}{
		"id": githubv4.ID(repository.ID),
	}
	connections := []connectionType{assigneesType, issueCommentsType, labelsType}
	for _, c := range connections {
		variables[c.Page()] = c.PageSize
		variables[c.Cursor()] = (*githubv4.String)(nil)
	}

	process := func(res Connection) error {
		issues := res.(graphql.IssueConnection)
		for _, issue := range issues.Nodes {
			assignees, err := d.downloadIssueAssignees(ctx, &issue)
			if err != nil {
				return err
			}

			labels, err := d.downloadIssueLabels(ctx, &issue)
			if err != nil {
				return err
			}

			if err := d.storer.SaveIssue(ctx, owner, name, &issue, assignees, labels); err != nil {
				return err
			}

			if err := d.downloadIssueComments(ctx, owner, name, &issue); err != nil {
				return err
			}
		}

		return nil
	}

	return d.downloadConnection(ctx, issuesType, repository.Issues, &q, variables, process)
}

type issueAssigneesQ struct {
	Node struct {
		Issue struct {
			Assignees graphql.UserConnection `graphql:"assignees(first: $assigneesPage, after: $assigneesCursor)"`
		} `graphql:"... on Issue"`
	} `graphql:"node(id:$id)"`
}

func (q *issueAssigneesQ) Connection() Connection {
	return q.Node.Issue.Assignees
}

func (d Downloader) downloadIssueAssignees(ctx context.Context, issue *graphql.Issue) ([]string, error) {
	var q issueAssigneesQ
	variables := map[string]interface{}{
		"id": githubv4.ID(issue.ID),
	}

	logins := []string{}
	process := func(res Connection) error {
		assignees := res.(graphql.UserConnection)
		for _, node := range assignees.Nodes {
			logins = append(logins, node.Login)
		}
		return nil
	}

	err := d.downloadConnection(ctx, assigneesType, issue.Assignees, &q, variables, process)
	if err != nil {
		return nil, err
	}

	return logins, err
}

type issueLabelsQ struct {
	Node struct {
		Issue struct {
			Labels graphql.LabelConnection `graphql:"labels(first: $labelsPage, after: $labelsCursor)"`
		} `graphql:"... on Issue"`
	} `graphql:"node(id:$id)"`
}

func (q *issueLabelsQ) Connection() Connection {
	return q.Node.Issue.Labels
}

func (d Downloader) downloadIssueLabels(ctx context.Context, issue *graphql.Issue) ([]string, error) {
	var q issueLabelsQ
	variables := map[string]interface{}{
		"id": githubv4.ID(issue.ID),
	}

	names := []string{}
	process := func(res Connection) error {
		labels := res.(graphql.LabelConnection)
		for _, node := range labels.Nodes {
			names = append(names, node.Name)
		}
		return nil
	}

	err := d.downloadConnection(ctx, labelsType, issue.Labels, &q, variables, process)
	if err != nil {
		return nil, err
	}

	return names, err
}

type issueCommentsQ struct {
	Node struct {
		Issue struct {
			Comments graphql.IssueCommentsConnection `graphql:"comments(first: $issueCommentsPage, after: $issueCommentsCursor)"`
		} `graphql:"... on Issue"`
	} `graphql:"node(id:$id)"`
}

func (q *issueCommentsQ) Connection() Connection {
	return q.Node.Issue.Comments
}

func (d Downloader) downloadIssueComments(ctx context.Context, owner string, name string, issue *graphql.Issue) error {
	var q issueCommentsQ
	variables := map[string]interface{}{
		"id": githubv4.ID(issue.ID),
	}

	process := func(res Connection) error {
		comments := res.(graphql.IssueCommentsConnection)
		for _, comment := range comments.Nodes {
			err := d.storer.SaveIssueComment(ctx, owner, name, issue.Number, &comment)
			if err != nil {
				return err
			}
		}
		return nil
	}

	return d.downloadConnection(ctx, issueCommentsType, issue.Comments, &q, variables, process)
}

type pullRequestsQ struct {
	Node struct {
		Repository struct {
			PullRequests graphql.PullRequestConnection `graphql:"pullRequests(first: $pullRequestsPage, after: $pullRequestsCursor)"`
		} `graphql:"... on Repository"`
	} `graphql:"node(id:$id)"`
}

func (q *pullRequestsQ) Connection() Connection {
	return q.Node.Repository.PullRequests
}

func (d Downloader) downloadPullRequests(ctx context.Context, owner string, name string, repository *graphql.Repository) error {
	var q pullRequestsQ
	variables := map[string]interface{}{
		"id": githubv4.ID(repository.ID),
	}
	connections := []connectionType{
		assigneesType, issueCommentsType, labelsType,
		pullRequestReviewCommentsType, pullRequestReviewsType}
	for _, c := range connections {
		variables[c.Page()] = c.PageSize
		variables[c.Cursor()] = (*githubv4.String)(nil)
	}

	process := func(res Connection) error {
		prs := res.(graphql.PullRequestConnection)
		for _, pr := range prs.Nodes {
			assignees, err := d.downloadPullRequestAssignees(ctx, &pr)
			if err != nil {
				return err
			}

			labels, err := d.downloadPullRequestLabels(ctx, &pr)
			if err != nil {
				return err
			}

			if err := d.storer.SavePullRequest(ctx, owner, name, &pr, assignees, labels); err != nil {
				return err
			}

			if err := d.downloadPullRequestComments(ctx, owner, name, &pr); err != nil {
				return err
			}
			if err := d.downloadPullRequestReviews(ctx, owner, name, &pr); err != nil {
				return err
			}
		}

		return nil
	}

	return d.downloadConnection(ctx, pullRequestsType, repository.PullRequests, &q, variables, process)
}

type pullRequestAssigneesQ struct {
	Node struct {
		PullRequest struct {
			Assignees graphql.UserConnection `graphql:"assignees(first: $assigneesPage, after: $assigneesCursor)"`
		} `graphql:"... on PullRequest"`
	} `graphql:"node(id:$id)"`
}

func (q *pullRequestAssigneesQ) Connection() Connection {
	return q.Node.PullRequest.Assignees
}

func (d Downloader) downloadPullRequestAssignees(ctx context.Context, pr *graphql.PullRequest) ([]string, error) {
	var q pullRequestAssigneesQ
	variables := map[string]interface{}{
		"id": githubv4.ID(pr.ID),
	}

	logins := []string{}
	process := func(res Connection) error {
		assignees := res.(graphql.UserConnection)
		for _, node := range assignees.Nodes {
			logins = append(logins, node.Login)
		}
		return nil
	}

	err := d.downloadConnection(ctx, assigneesType, pr.Assignees, &q, variables, process)
	if err != nil {
		return nil, err
	}

	return logins, nil
}

type pullRequestLabelsQ struct {
	Node struct {
		PullRequest struct {
			Labels graphql.LabelConnection `graphql:"labels(first: $labelsPage, after: $labelsCursor)"`
		} `graphql:"... on PullRequest"`
	} `graphql:"node(id:$id)"`
}

func (q *pullRequestLabelsQ) Connection() Connection {
	return q.Node.PullRequest.Labels
}

func (d Downloader) downloadPullRequestLabels(ctx context.Context, pr *graphql.PullRequest) ([]string, error) {
	var q pullRequestLabelsQ
	variables := map[string]interface{}{
		"id": githubv4.ID(pr.ID),
	}

	names := []string{}
	process := func(res Connection) error {
		labels := res.(graphql.LabelConnection)
		for _, node := range labels.Nodes {
			names = append(names, node.Name)
		}
		return nil
	}

	err := d.downloadConnection(ctx, labelsType, pr.Labels, &q, variables, process)
	if err != nil {
		return nil, err
	}

	return names, err
}

type pullRequestCommentsQ struct {
	Node struct {
		PullRequest struct {
			Comments graphql.IssueCommentsConnection `graphql:"comments(first: $issueCommentsPage, after: $issueCommentsCursor)"`
		} `graphql:"... on PullRequest"`
	} `graphql:"node(id:$id)"`
}

func (q *pullRequestCommentsQ) Connection() Connection {
	return q.Node.PullRequest.Comments
}

func (d Downloader) downloadPullRequestComments(ctx context.Context, owner string, name string, pr *graphql.PullRequest) error {
	var q pullRequestCommentsQ
	variables := map[string]interface{}{
		"id": githubv4.ID(pr.ID),
	}

	process := func(res Connection) error {
		comments := res.(graphql.IssueCommentsConnection)
		for _, comment := range comments.Nodes {
			err := d.storer.SavePullRequestComment(ctx, owner, name, pr.Number, &comment)
			if err != nil {
				return fmt.Errorf("failed to save PR comments for PR #%v: %v", pr.Number, err)
			}
		}

		return nil
	}

	return d.downloadConnection(ctx, issueCommentsType, pr.Comments, &q, variables, process)
}

type pullRequestReviewsQ struct {
	Node struct {
		PullRequest struct {
			Reviews graphql.PullRequestReviewConnection `graphql:"reviews(first: $pullRequestReviewsPage, after: $pullRequestReviewsCursor)"`
		} `graphql:"... on PullRequest"`
	} `graphql:"node(id:$id)"`
}

func (q *pullRequestReviewsQ) Connection() Connection {
	return q.Node.PullRequest.Reviews
}

func (d Downloader) downloadPullRequestReviews(ctx context.Context, owner string, name string, pr *graphql.PullRequest) error {
	var q pullRequestReviewsQ
	variables := map[string]interface{}{
		"id": githubv4.ID(pr.ID),
	}
	variables[pullRequestReviewCommentsType.Page()] = pullRequestReviewCommentsType.PageSize
	variables[pullRequestReviewCommentsType.Cursor()] = (*githubv4.String)(nil)

	process := func(res Connection) error {
		reviews := res.(graphql.PullRequestReviewConnection)
		for _, review := range reviews.Nodes {
			err := d.storer.SavePullRequestReview(ctx, owner, name, pr.Number, &review)
			if err != nil {
				return fmt.Errorf("failed to save PR review for PR #%v: %v", pr.Number, err)
			}
			if err := d.downloadReviewComments(ctx, owner, name, pr.Number, &review); err != nil {
				return err
			}
		}

		return nil
	}

	return d.downloadConnection(ctx, pullRequestReviewsType, pr.Reviews, &q, variables, process)
}

type reviewCommentsQ struct {
	Node struct {
		PullRequestReview struct {
			Comments graphql.PullRequestReviewCommentConnection `graphql:"comments(first: $pullRequestReviewCommentsPage, after: $pullRequestReviewCommentsCursor)"`
		} `graphql:"... on PullRequestReview"`
	} `graphql:"node(id:$id)"`
}

func (q *reviewCommentsQ) Connection() Connection {
	return q.Node.PullRequestReview.Comments
}

func (d Downloader) downloadReviewComments(ctx context.Context, repositoryOwner, repositoryName string, pullRequestNumber int, review *graphql.PullRequestReview) error {
	var q reviewCommentsQ
	variables := map[string]interface{}{
		"id": githubv4.ID(review.ID),
	}

	process := func(res Connection) error {
		comments := res.(graphql.PullRequestReviewCommentConnection)
		for _, comment := range comments.Nodes {
			err := d.storer.SavePullRequestReviewComment(ctx, repositoryOwner, repositoryName, pullRequestNumber, review.DatabaseID, &comment)
			if err != nil {
				return fmt.Errorf(
					"failed to save PullRequestReviewComment for PR #%v, review ID %v: %v",
					pullRequestNumber, review.ID, err)
			}
		}

		return nil
	}

	return d.downloadConnection(ctx, pullRequestReviewCommentsType, review.Comments, &q, variables, process)
}

// DownloadOrganization downloads the metadata for the given organization and
// its member users
func (d Downloader) DownloadOrganization(ctx context.Context, name string, version int) error {
	d.storer.Version(version)

	var err error
	err = d.storer.Begin()
	if err != nil {
		return fmt.Errorf("could not call Begin(): %v", err)
	}

	defer func() {
		if err != nil {
			d.storer.Rollback()
			return
		}

		d.storer.Commit()
	}()

	var q struct {
		graphql.Organization `graphql:"organization(login: $organizationLogin)"`
	}

	// Some variables are repeated in the query, like assigneesCursor for Issues
	// and PullRequests. It's ok to reuse because in this top level Repository
	// query the cursors are set to nil, and when the pagination occurs, the
	// queries only request either Issues or PullRequests
	variables := map[string]interface{}{
		"organizationLogin": githubv4.String(name),
	}
	variables[membersWithRole.Page()] = membersWithRole.PageSize
	variables[membersWithRole.Cursor()] = (*githubv4.String)(nil)

	err = d.client.Query(ctx, &q, variables)
	if err != nil {
		return fmt.Errorf("organization query failed: %v", err)
	}

	err = d.storer.SaveOrganization(ctx, &q.Organization)
	if err != nil {
		return fmt.Errorf("failed to save organization %v: %v", name, err)
	}

	err = d.downloadUsers(ctx, name, &q.Organization)
	if err != nil {
		return err
	}

	return nil
}

type usersQ struct {
	Organization struct {
		MembersWithRole graphql.OrganizationMemberConnection `graphql:"membersWithRole(first: $membersWithRolePage, after: $membersWithRoleCursor)"`
	} `graphql:"organization(login: $organizationLogin)"`
}

func (q *usersQ) Connection() Connection {
	return q.Organization.MembersWithRole
}

func (d Downloader) downloadUsers(ctx context.Context, name string, organization *graphql.Organization) error {
	var q usersQ
	variables := map[string]interface{}{
		"organizationLogin": githubv4.String(name),
	}

	process := func(res Connection) error {
		users := res.(graphql.OrganizationMemberConnection)
		for _, user := range users.Nodes {
			err := d.storer.SaveUser(ctx, organization.DatabaseID, organization.Login, &user)
			if err != nil {
				return fmt.Errorf("failed to save UserExtended: %v", err)
			}
		}

		return nil
	}

	return d.downloadConnection(ctx, membersWithRole, organization.MembersWithRole, &q, variables, process)
}

// SetCurrent enables the given version as the current one accessible in the DB
func (d Downloader) SetCurrent(ctx context.Context, version int) error {
	err := d.storer.SetActiveVersion(ctx, version)
	if err != nil {
		return fmt.Errorf("failed to set current DB version to %v: %v", version, err)
	}
	return nil
}

// Cleanup deletes from the DB all records that do not belong to the currentVersion
func (d Downloader) Cleanup(ctx context.Context, currentVersion int) error {
	err := d.storer.Cleanup(ctx, currentVersion)
	if err != nil {
		return fmt.Errorf("failed to do cleanup for DB version %v: %v", currentVersion, err)
	}
	return nil
}
