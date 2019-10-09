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

const (
	assigneesPage                 = 2
	issueCommentsPage             = 10
	issuesPage                    = 50
	labelsPage                    = 2
	membersWithRolePage           = 100
	pullRequestReviewCommentsPage = 5
	pullRequestReviewsPage        = 5
	pullRequestsPage              = 50
	repositoryTopicsPage          = 10

	// to track progress of sub-resources only each N page to avoid log flooding
	logEachPageN = 3
)

// getPerPage calculates how many resources to request based on total number and number of already downloaded
func getPerPage(total, count, fallback int) githubv4.Int {
	return getPerPageLimited(total, count, fallback, 100)
}

// getPerPageLimited same as getPerPage but accepts non-maximum limit
// should be used for heavy requests like pull requests to avoid 502 from github
func getPerPageLimited(total, count, fallback, limit int) githubv4.Int {
	perPage := total - count
	if perPage > limit {
		perPage = limit
	}
	// in case entities appeared during downloading process
	if perPage <= 0 {
		perPage = fallback
	}

	return githubv4.Int(perPage)
}

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

		"assigneesPage":                 githubv4.Int(assigneesPage),
		"issueCommentsPage":             githubv4.Int(issueCommentsPage),
		"issuesPage":                    githubv4.Int(issuesPage),
		"labelsPage":                    githubv4.Int(labelsPage),
		"pullRequestReviewCommentsPage": githubv4.Int(pullRequestReviewCommentsPage),
		"pullRequestReviewsPage":        githubv4.Int(pullRequestReviewsPage),
		"pullRequestsPage":              githubv4.Int(pullRequestsPage),
		"repositoryTopicsPage":          githubv4.Int(repositoryTopicsPage),

		"assigneesCursor":                 (*githubv4.String)(nil),
		"issueCommentsCursor":             (*githubv4.String)(nil),
		"issuesCursor":                    (*githubv4.String)(nil),
		"labelsCursor":                    (*githubv4.String)(nil),
		"pullRequestReviewCommentsCursor": (*githubv4.String)(nil),
		"pullRequestReviewsCursor":        (*githubv4.String)(nil),
		"pullRequestsCursor":              (*githubv4.String)(nil),
		"repositoryTopicsCursor":          (*githubv4.String)(nil),
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

func (d Downloader) downloadTopics(ctx context.Context, repository *graphql.Repository) ([]string, error) {
	logger := ctxlog.Get(ctx)
	logger.Infof("start downloading topics")
	defer logger.Infof("finished downloading topics")

	topics := repository.RepositoryTopics
	names := []string{}

	// Topics included in the first page
	for _, topicNode := range topics.Nodes {
		names = append(names, topicNode.Topic.Name)
	}

	variables := map[string]interface{}{
		"id": githubv4.ID(repository.ID),
	}

	// if there are more topics, loop over all the pages
	hasNextPage := topics.PageInfo.HasNextPage
	endCursor := topics.PageInfo.EndCursor

	var count int
	for hasNextPage {
		count += len(topics.Nodes)

		// get only repository topics
		var q struct {
			Node struct {
				Repository struct {
					RepositoryTopics graphql.RepositoryTopicsConnection `graphql:"repositoryTopics(first: $repositoryTopicsPage, after: $repositoryTopicsCursor)"`
				} `graphql:"... on Repository"`
			} `graphql:"node(id:$id)"`
		}

		variables["repositoryTopicsPage"] = getPerPage(topics.TotalCount, count, repositoryTopicsPage)
		variables["repositoryTopicsCursor"] = githubv4.String(endCursor)

		err := d.client.Query(ctx, &q, variables)
		if err != nil {
			return nil, fmt.Errorf("RepositoryTopics query failed: %v", err)
		}

		topics = q.Node.Repository.RepositoryTopics
		for _, topicNode := range q.Node.Repository.RepositoryTopics.Nodes {
			names = append(names, topicNode.Topic.Name)
		}

		hasNextPage = topics.PageInfo.HasNextPage
		endCursor = topics.PageInfo.EndCursor
	}

	return names, nil
}

func (d Downloader) downloadIssues(ctx context.Context, owner string, name string, repository *graphql.Repository) error {
	logger := ctxlog.Get(ctx)
	logger.Infof("start downloading issues")
	defer logger.Infof("finished downloading issues")

	issues := repository.Issues
	process := func(issue *graphql.Issue) error {
		assignees, err := d.downloadIssueAssignees(ctx, issue)
		if err != nil {
			return err
		}

		labels, err := d.downloadIssueLabels(ctx, issue)
		if err != nil {
			return err
		}

		err = d.storer.SaveIssue(ctx, owner, name, issue, assignees, labels)
		if err != nil {
			return err
		}
		return d.downloadIssueComments(ctx, owner, name, issue)
	}

	// Save issues included in the first page
	for _, issue := range issues.Nodes {
		err := process(&issue)
		if err != nil {
			return fmt.Errorf("failed to process issue %v/%v #%v: %v", owner, name, issue.Number, err)
		}
	}

	variables := map[string]interface{}{
		"id": githubv4.ID(repository.ID),

		"assigneesPage":     githubv4.Int(assigneesPage),
		"issueCommentsPage": githubv4.Int(issueCommentsPage),
		"labelsPage":        githubv4.Int(labelsPage),

		"assigneesCursor":     (*githubv4.String)(nil),
		"issueCommentsCursor": (*githubv4.String)(nil),
		"labelsCursor":        (*githubv4.String)(nil),
	}

	// if there are more issues, loop over all the pages
	hasNextPage := issues.PageInfo.HasNextPage
	endCursor := issues.PageInfo.EndCursor

	var count int
	for hasNextPage {
		count += len(issues.Nodes)
		if count%(issuesPage*logEachPageN) == 0 {
			logger.Infof("%d/%d issues downloaded", count, issues.TotalCount)
		}

		// get only issues
		var q struct {
			Node struct {
				Repository struct {
					Issues graphql.IssueConnection `graphql:"issues(first: $issuesPage, after: $issuesCursor)"`
				} `graphql:"... on Repository"`
			} `graphql:"node(id:$id)"`
		}

		variables["issuesPage"] = getPerPage(issues.TotalCount, count, issuesPage)
		variables["issuesCursor"] = githubv4.String(endCursor)

		err := d.client.Query(ctx, &q, variables)
		if err != nil {
			return fmt.Errorf("failed to query issues for repository %v: %v", repository.NameWithOwner, err)
		}

		issues = q.Node.Repository.Issues
		for _, issue := range issues.Nodes {
			err := process(&issue)
			if err != nil {
				return fmt.Errorf("failed to process issue %v #%v: %v", repository.NameWithOwner, issue.Number, err)
			}
		}

		hasNextPage = issues.PageInfo.HasNextPage
		endCursor = issues.PageInfo.EndCursor
	}

	return nil
}

func (d Downloader) downloadIssueAssignees(ctx context.Context, issue *graphql.Issue) ([]string, error) {
	assignees := issue.Assignees
	logins := []string{}

	// Assignees included in the first page
	for _, node := range assignees.Nodes {
		logins = append(logins, node.Login)
	}

	variables := map[string]interface{}{
		"id": githubv4.ID(issue.ID),
	}

	// if there are more assignees, loop over all the pages
	hasNextPage := assignees.PageInfo.HasNextPage
	endCursor := assignees.PageInfo.EndCursor

	var count int
	for hasNextPage {
		count += len(assignees.Nodes)

		// get only issue assignees
		var q struct {
			Node struct {
				Issue struct {
					Assignees graphql.UserConnection `graphql:"assignees(first: $assigneesPage, after: $assigneesCursor)"`
				} `graphql:"... on Issue"`
			} `graphql:"node(id:$id)"`
		}

		variables["assigneesPage"] = getPerPage(assignees.TotalCount, count, assigneesPage)
		variables["assigneesCursor"] = githubv4.String(endCursor)

		err := d.client.Query(ctx, &q, variables)
		if err != nil {
			return nil, fmt.Errorf("failed to query issue assignees for issue #%v: %v", issue.Number, err)
		}

		assignees = q.Node.Issue.Assignees
		for _, node := range assignees.Nodes {
			logins = append(logins, node.Login)
		}

		hasNextPage = assignees.PageInfo.HasNextPage
		endCursor = assignees.PageInfo.EndCursor
	}

	return logins, nil
}

func (d Downloader) downloadIssueLabels(ctx context.Context, issue *graphql.Issue) ([]string, error) {
	labels := issue.Labels
	names := []string{}

	// Labels included in the first page
	for _, node := range labels.Nodes {
		names = append(names, node.Name)
	}

	variables := map[string]interface{}{
		"id": githubv4.ID(issue.ID),
	}

	// if there are more labels, loop over all the pages
	hasNextPage := labels.PageInfo.HasNextPage
	endCursor := labels.PageInfo.EndCursor

	var count int
	for hasNextPage {
		count += len(labels.Nodes)

		// get only issue labels
		var q struct {
			Node struct {
				Issue struct {
					Labels graphql.LabelConnection `graphql:"labels(first: $labelsPage, after: $labelsCursor)"`
				} `graphql:"... on Issue"`
			} `graphql:"node(id:$id)"`
		}

		variables["labelsPage"] = getPerPage(labels.TotalCount, count, labelsPage)
		variables["labelsCursor"] = githubv4.String(endCursor)

		err := d.client.Query(ctx, &q, variables)
		if err != nil {
			return nil, fmt.Errorf("failed to query issue labels for issue #%v: %v", issue.Number, err)
		}

		labels = q.Node.Issue.Labels
		for _, node := range labels.Nodes {
			names = append(names, node.Name)
		}

		hasNextPage = labels.PageInfo.HasNextPage
		endCursor = labels.PageInfo.EndCursor
	}

	return names, nil
}

func (d Downloader) downloadIssueComments(ctx context.Context, owner string, name string, issue *graphql.Issue) error {
	comments := issue.Comments

	// save first page of comments
	for _, comment := range comments.Nodes {
		err := d.storer.SaveIssueComment(ctx, owner, name, issue.Number, &comment)
		if err != nil {
			return err
		}
	}

	variables := map[string]interface{}{
		"id": githubv4.ID(issue.ID),
	}

	// if there are more issue comments, loop over all the pages
	hasNextPage := comments.PageInfo.HasNextPage
	endCursor := comments.PageInfo.EndCursor

	var count int
	for hasNextPage {
		count += len(comments.Nodes)

		// get only issue comments
		var q struct {
			Node struct {
				Issue struct {
					Comments graphql.IssueCommentsConnection `graphql:"comments(first: $issueCommentsPage, after: $issueCommentsCursor)"`
				} `graphql:"... on Issue"`
			} `graphql:"node(id:$id)"`
		}

		variables["issueCommentsPage"] = getPerPage(comments.TotalCount, count, issueCommentsPage)
		variables["issueCommentsCursor"] = githubv4.String(endCursor)

		err := d.client.Query(ctx, &q, variables)
		if err != nil {
			return fmt.Errorf("failed to query issue comments for issue #%v: %v", issue.Number, err)
		}

		comments = q.Node.Issue.Comments
		for _, comment := range comments.Nodes {
			err := d.storer.SaveIssueComment(ctx, owner, name, issue.Number, &comment)
			if err != nil {
				return fmt.Errorf("failed to save issue comments for issue #%v: %v", issue.Number, err)
			}
		}

		hasNextPage = comments.PageInfo.HasNextPage
		endCursor = comments.PageInfo.EndCursor
	}

	return nil
}

func (d Downloader) downloadPullRequests(ctx context.Context, owner string, name string, repository *graphql.Repository) error {
	logger := ctxlog.Get(ctx)
	logger.Infof("start downloading pull requests")
	defer logger.Infof("finished downloading pull requests")

	prs := repository.PullRequests
	process := func(pr *graphql.PullRequest) error {
		assignees, err := d.downloadPullRequestAssignees(ctx, pr)
		if err != nil {
			return err
		}

		labels, err := d.downloadPullRequestLabels(ctx, pr)
		if err != nil {
			return err
		}

		err = d.storer.SavePullRequest(ctx, owner, name, pr, assignees, labels)
		if err != nil {
			return err
		}
		err = d.downloadPullRequestComments(ctx, owner, name, pr)
		if err != nil {
			return err
		}
		err = d.downloadPullRequestReviews(ctx, owner, name, pr)
		if err != nil {
			return err
		}

		return nil
	}

	// Save PRs included in the first page
	for _, pr := range prs.Nodes {
		err := process(&pr)
		if err != nil {
			return fmt.Errorf("failed to process PR %v/%v #%v: %v", owner, name, pr.Number, err)
		}
	}

	variables := map[string]interface{}{
		"id": githubv4.ID(repository.ID),

		"assigneesPage":                 githubv4.Int(assigneesPage),
		"issueCommentsPage":             githubv4.Int(issueCommentsPage),
		"labelsPage":                    githubv4.Int(labelsPage),
		"pullRequestReviewCommentsPage": githubv4.Int(pullRequestReviewCommentsPage),
		"pullRequestReviewsPage":        githubv4.Int(pullRequestReviewsPage),

		"assigneesCursor":                 (*githubv4.String)(nil),
		"issueCommentsCursor":             (*githubv4.String)(nil),
		"labelsCursor":                    (*githubv4.String)(nil),
		"pullRequestReviewCommentsCursor": (*githubv4.String)(nil),
		"pullRequestReviewsCursor":        (*githubv4.String)(nil),
	}

	// if there are more PRs, loop over all the pages
	hasNextPage := prs.PageInfo.HasNextPage
	endCursor := prs.PageInfo.EndCursor

	var count int
	for hasNextPage {
		count += len(prs.Nodes)
		if count%(pullRequestsPage*logEachPageN) == 0 {
			logger.Infof("%d/%d pull requests downloaded", count, prs.TotalCount)
		}

		// get only PRs
		var q struct {
			Node struct {
				Repository struct {
					PullRequests graphql.PullRequestConnection `graphql:"pullRequests(first: $pullRequestsPage, after: $pullRequestsCursor)"`
				} `graphql:"... on Repository"`
			} `graphql:"node(id:$id)"`
		}

		variables["pullRequestsPage"] = getPerPageLimited(prs.TotalCount, count, pullRequestsPage, pullRequestsPage)
		variables["pullRequestsCursor"] = githubv4.String(endCursor)

		err := d.client.Query(ctx, &q, variables)
		if err != nil {
			return fmt.Errorf("failed to query PRs for repository %v/%v: %v", owner, name, err)
		}

		prs = q.Node.Repository.PullRequests
		for _, pr := range prs.Nodes {
			err := process(&pr)
			if err != nil {
				return fmt.Errorf("failed to process PR %v/%v #%v: %v", owner, name, pr.Number, err)
			}
		}

		hasNextPage = prs.PageInfo.HasNextPage
		endCursor = prs.PageInfo.EndCursor
	}

	return nil
}

func (d Downloader) downloadPullRequestAssignees(ctx context.Context, pr *graphql.PullRequest) ([]string, error) {
	assignees := pr.Assignees
	logins := []string{}

	// Assignees included in the first page
	for _, node := range assignees.Nodes {
		logins = append(logins, node.Login)
	}

	variables := map[string]interface{}{
		"id": githubv4.ID(pr.ID),
	}

	// if there are more assigness, loop over all the pages
	hasNextPage := assignees.PageInfo.HasNextPage
	endCursor := assignees.PageInfo.EndCursor

	var count int
	for hasNextPage {
		count += len(assignees.Nodes)

		// get only PR assignees
		var q struct {
			Node struct {
				PullRequest struct {
					Assignees graphql.UserConnection `graphql:"assignees(first: $assigneesPage, after: $assigneesCursor)"`
				} `graphql:"... on PullRequest"`
			} `graphql:"node(id:$id)"`
		}

		variables["assigneesPage"] = getPerPage(assignees.TotalCount, count, assigneesPage)
		variables["assigneesCursor"] = githubv4.String(endCursor)

		err := d.client.Query(ctx, &q, variables)
		if err != nil {
			return nil, fmt.Errorf("failed to query PR assignees for PR #%v: %v", pr.Number, err)
		}

		assignees = q.Node.PullRequest.Assignees
		for _, node := range assignees.Nodes {
			logins = append(logins, node.Login)
		}

		hasNextPage = assignees.PageInfo.HasNextPage
		endCursor = assignees.PageInfo.EndCursor
	}

	return logins, nil
}

func (d Downloader) downloadPullRequestLabels(ctx context.Context, pr *graphql.PullRequest) ([]string, error) {
	labels := pr.Labels
	names := []string{}

	// Labels included in the first page
	for _, node := range labels.Nodes {
		names = append(names, node.Name)
	}

	variables := map[string]interface{}{
		"id": githubv4.ID(pr.ID),
	}

	// if there are more labels, loop over all the pages
	hasNextPage := labels.PageInfo.HasNextPage
	endCursor := labels.PageInfo.EndCursor

	var count int
	for hasNextPage {
		count += len(labels.Nodes)

		// get only PR labels
		var q struct {
			Node struct {
				PullRequest struct {
					Labels graphql.LabelConnection `graphql:"labels(first: $labelsPage, after: $labelsCursor)"`
				} `graphql:"... on PullRequest"`
			} `graphql:"node(id:$id)"`
		}

		variables["labelsPage"] = getPerPage(labels.TotalCount, count, labelsPage)
		variables["labelsCursor"] = githubv4.String(endCursor)

		err := d.client.Query(ctx, &q, variables)
		if err != nil {
			return nil, fmt.Errorf("failed to query PR labels for PR #%v: %v", pr.Number, err)
		}

		labels = q.Node.PullRequest.Labels
		for _, node := range labels.Nodes {
			names = append(names, node.Name)
		}

		hasNextPage = labels.PageInfo.HasNextPage
		endCursor = labels.PageInfo.EndCursor
	}

	return names, nil
}

func (d Downloader) downloadPullRequestComments(ctx context.Context, owner string, name string, pr *graphql.PullRequest) error {
	comments := pr.Comments

	// save first page of comments
	for _, comment := range comments.Nodes {
		err := d.storer.SavePullRequestComment(ctx, owner, name, pr.Number, &comment)
		if err != nil {
			return fmt.Errorf("failed to save PR comments for PR #%v: %v", pr.Number, err)
		}
	}

	variables := map[string]interface{}{
		"id": githubv4.ID(pr.ID),
	}

	// if there are more issue comments, loop over all the pages
	hasNextPage := comments.PageInfo.HasNextPage
	endCursor := comments.PageInfo.EndCursor

	var count int
	for hasNextPage {
		count += len(comments.Nodes)

		// get only PR comments
		var q struct {
			Node struct {
				PullRequest struct {
					Comments graphql.IssueCommentsConnection `graphql:"comments(first: $issueCommentsPage, after: $issueCommentsCursor)"`
				} `graphql:"... on PullRequest"`
			} `graphql:"node(id:$id)"`
		}

		variables["issueCommentsPage"] = getPerPage(comments.TotalCount, count, issueCommentsPage)
		variables["issueCommentsCursor"] = githubv4.String(endCursor)

		err := d.client.Query(ctx, &q, variables)
		if err != nil {
			return fmt.Errorf("failed to query PR comments for PR #%v: %v", pr.Number, err)
		}

		comments = q.Node.PullRequest.Comments
		for _, comment := range comments.Nodes {
			err := d.storer.SavePullRequestComment(ctx, owner, name, pr.Number, &comment)
			if err != nil {
				return fmt.Errorf("failed to save PR comments for PR #%v: %v", pr.Number, err)
			}
		}

		hasNextPage = comments.PageInfo.HasNextPage
		endCursor = comments.PageInfo.EndCursor
	}

	return nil
}

func (d Downloader) downloadPullRequestReviews(ctx context.Context, owner string, name string, pr *graphql.PullRequest) error {
	reviews := pr.Reviews

	process := func(review *graphql.PullRequestReview) error {
		err := d.storer.SavePullRequestReview(ctx, owner, name, pr.Number, review)
		if err != nil {
			return fmt.Errorf("failed to save PR review for PR #%v: %v", pr.Number, err)
		}
		return d.downloadReviewComments(ctx, owner, name, pr.Number, review)
	}

	// save first page of reviews
	for _, review := range reviews.Nodes {
		err := process(&review)
		if err != nil {
			return err
		}
	}

	variables := map[string]interface{}{
		"id": githubv4.ID(pr.ID),

		"pullRequestReviewCommentsPage":   githubv4.Int(pullRequestReviewCommentsPage),
		"pullRequestReviewCommentsCursor": (*githubv4.String)(nil),
	}

	// if there are more reviews, loop over all the pages
	hasNextPage := reviews.PageInfo.HasNextPage
	endCursor := reviews.PageInfo.EndCursor

	var count int
	for hasNextPage {
		count += len(reviews.Nodes)

		// get only PR reviews
		var q struct {
			Node struct {
				PullRequest struct {
					Reviews graphql.PullRequestReviewConnection `graphql:"reviews(first: $pullRequestReviewsPage, after: $pullRequestReviewsCursor)"`
				} `graphql:"... on PullRequest"`
			} `graphql:"node(id:$id)"`
		}

		variables["pullRequestReviewsPage"] = getPerPage(reviews.TotalCount, count, pullRequestReviewsPage)
		variables["pullRequestReviewsCursor"] = githubv4.String(endCursor)

		err := d.client.Query(ctx, &q, variables)
		if err != nil {
			return fmt.Errorf("failed to query PR reviews for PR #%v: %v", pr.Number, err)
		}

		reviews = q.Node.PullRequest.Reviews
		for _, review := range q.Node.PullRequest.Reviews.Nodes {
			err := process(&review)
			if err != nil {
				return err
			}
		}

		hasNextPage = reviews.PageInfo.HasNextPage
		endCursor = reviews.PageInfo.EndCursor
	}

	return nil
}

func (d Downloader) downloadReviewComments(ctx context.Context, repositoryOwner, repositoryName string, pullRequestNumber int, review *graphql.PullRequestReview) error {
	comments := review.Comments

	process := func(comment *graphql.PullRequestReviewComment) error {
		err := d.storer.SavePullRequestReviewComment(ctx, repositoryOwner, repositoryName, pullRequestNumber, review.DatabaseID, comment)
		if err != nil {
			return fmt.Errorf(
				"failed to save PullRequestReviewComment for PR #%v, review ID %v: %v",
				pullRequestNumber, review.ID, err)
		}

		return nil
	}

	// save first page of comments
	for _, comment := range comments.Nodes {
		err := process(&comment)
		if err != nil {
			return err
		}
	}

	variables := map[string]interface{}{
		"id": githubv4.ID(review.ID),
	}

	// if there are more review comments, loop over all the pages
	hasNextPage := review.Comments.PageInfo.HasNextPage
	endCursor := review.Comments.PageInfo.EndCursor

	var count int
	for hasNextPage {
		count += len(comments.Nodes)

		var q struct {
			Node struct {
				PullRequestReview struct {
					Comments graphql.PullRequestReviewCommentConnection `graphql:"comments(first: $pullRequestReviewCommentsPage, after: $pullRequestReviewCommentsCursor)"`
				} `graphql:"... on PullRequestReview"`
			} `graphql:"node(id:$id)"`
		}

		variables["pullRequestReviewCommentsPage"] = getPerPage(comments.TotalCount, count, pullRequestReviewCommentsPage)
		variables["pullRequestReviewCommentsCursor"] = githubv4.String(endCursor)

		err := d.client.Query(ctx, &q, variables)
		if err != nil {
			return fmt.Errorf(
				"failed to query PR review comments for PR #%v, review ID %v: %v",
				pullRequestNumber, review.ID, err)
		}

		comments = q.Node.PullRequestReview.Comments
		for _, comment := range comments.Nodes {
			err := process(&comment)
			if err != nil {
				return err
			}
		}

		hasNextPage = comments.PageInfo.HasNextPage
		endCursor = comments.PageInfo.EndCursor
	}

	return nil
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

		"membersWithRolePage":   githubv4.Int(membersWithRolePage),
		"membersWithRoleCursor": (*githubv4.String)(nil),
	}

	err = d.client.Query(ctx, &q, variables)
	if err != nil {
		return fmt.Errorf("organization query failed: %v", err)
	}

	err = d.storer.SaveOrganization(ctx, &q.Organization)
	if err != nil {
		return fmt.Errorf("failed to save organization %v: %v", name, err)
	}

	// issues and comments
	err = d.downloadUsers(ctx, name, &q.Organization)
	if err != nil {
		return err
	}

	return nil
}

func (d Downloader) downloadUsers(ctx context.Context, name string, organization *graphql.Organization) error {
	var logger log.Logger
	ctx, logger = ctxlog.WithLogFields(ctx, log.Fields{"owner": name})
	logger.Infof("start downloading users")
	defer logger.Infof("finished downloading users")

	process := func(user *graphql.UserExtended) error {
		err := d.storer.SaveUser(ctx, organization.DatabaseID, organization.Login, user)
		if err != nil {
			return fmt.Errorf("failed to save UserExtended: %v", err)
		}

		return nil
	}

	// Save users included in the first page
	for _, user := range organization.MembersWithRole.Nodes {
		err := process(&user)
		if err != nil {
			return fmt.Errorf("failed to process user %v: %v", user.Login, err)
		}
	}

	variables := map[string]interface{}{
		"organizationLogin": githubv4.String(name),

		"membersWithRolePage":   githubv4.Int(membersWithRolePage),
		"membersWithRoleCursor": (*githubv4.String)(nil),
	}

	// if there are more users, loop over all the pages
	hasNextPage := organization.MembersWithRole.PageInfo.HasNextPage
	endCursor := organization.MembersWithRole.PageInfo.EndCursor

	for hasNextPage {
		// get only users
		var q struct {
			Organization struct {
				MembersWithRole graphql.OrganizationMemberConnection `graphql:"membersWithRole(first: $membersWithRolePage, after: $membersWithRoleCursor)"`
			} `graphql:"organization(login: $organizationLogin)"`
		}

		variables["membersWithRoleCursor"] = githubv4.String(endCursor)

		err := d.client.Query(ctx, &q, variables)
		if err != nil {
			return fmt.Errorf("failed to organization members for organization %v: %v", name, err)
		}

		for _, user := range q.Organization.MembersWithRole.Nodes {
			err := process(&user)
			if err != nil {
				return fmt.Errorf("failed to process user %v: %v", user.Login, err)
			}
		}

		hasNextPage = q.Organization.MembersWithRole.PageInfo.HasNextPage
		endCursor = q.Organization.MembersWithRole.PageInfo.EndCursor
	}

	return nil
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
