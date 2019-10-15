package bbserver

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"

	bitbucketv1 "github.com/gfleury/go-bitbucket-v1"
	"github.com/mitchellh/mapstructure"
	"github.com/src-d/metadata-retrieval/bbserver/store"
	"github.com/src-d/metadata-retrieval/bbserver/types"
)

const defaultLimit = 1000

// Downloader fetches BitBucket Server (Stash) data using REST API
type Downloader struct {
	client *bitbucketv1.APIClient
	storer *store.DB
}

// NewDownloader creates a new Downloader that will store the Bitbucket Server metadata
// in the given DB. The HTTP client is expected to have the proper
// authentication setup
func NewDownloader(ctx context.Context, basePath string, httpClient *http.Client, db *sql.DB) (*Downloader, error) {
	cfg := bitbucketv1.NewConfiguration(basePath)
	cfg.HTTPClient = httpClient

	return &Downloader{
		storer: &store.DB{DB: db},
		client: bitbucketv1.NewAPIClient(ctx, cfg),
	}, nil
}

// ContextWithBasicAuth add bitbucket basic auto to ctx
func ContextWithBasicAuth(ctx context.Context, login, pass string) context.Context {
	basicAuth := bitbucketv1.BasicAuth{UserName: login, Password: pass}
	return context.WithValue(context.Background(), bitbucketv1.ContextBasicAuth, basicAuth)
}

// ListProjects returns all available project keys
func (d Downloader) ListProjects() ([]string, error) {
	projects, err := d.fetchProjects()
	if err != nil {
		return nil, err
	}

	keys := make([]string, len(projects))
	for i, project := range projects {
		keys[i] = project.Key
	}

	return keys, nil
}

// ListRepositories returns list of repositories slugs belong to the project
func (d Downloader) ListRepositories(ctx context.Context, project string) ([]string, error) {
	repos, err := d.fetchRepositories(project)
	if err != nil {
		return nil, err
	}

	slugs := make([]string, len(repos))
	for i, repo := range repos {
		slugs[i] = repo.Slug
	}

	return slugs, nil
}

// DownloadRepository downloads the metadata for the given repository and all
// its resources (PRs, comments, reviews)
func (d Downloader) DownloadRepository(ctx context.Context, project string, slug string, version int) error {
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

	resp, err := d.client.DefaultApi.GetRepository(project, slug)
	if err != nil {
		return err
	}

	repo, err := bitbucketv1.GetRepositoryResponse(resp)
	if err != nil {
		return err
	}

	if err := d.storer.SaveRepository(repo); err != nil {
		return err
	}

	prs, err := d.fetchPullRequests(project, slug)
	if err != nil {
		return err
	}

	for _, pr := range prs {
		epr, err := d.enrichPullRequest(project, slug, pr)
		if err != nil {
			return err
		}

		comments, diffComments, reviews, stateUpdate, err := d.fetchPRActivity(project, slug, pr.ID)
		if err != nil {
			return err
		}

		epr.Comments = len(comments)
		epr.ReviewComments = len(reviews)
		if stateUpdate != nil {
			if stateUpdate.State == "MERGED" {
				epr.MergedAt = stateUpdate.Date
				epr.MergedBy = stateUpdate.User
			} else if stateUpdate.State == "CLOSED" {
				epr.ClosedAt = stateUpdate.Date
			}
		}

		if err := d.storer.SavePullRequest(project, slug, *epr); err != nil {
			return err
		}

		for _, comment := range comments {
			if err := d.storer.SavePullRequestComment(project, slug, pr.ID, comment); err != nil {
				return err
			}
		}

		for _, comment := range diffComments {
			if err := d.storer.SavePullRequestReviewComment(project, slug, pr.ID, comment); err != nil {
				return err
			}
		}

		for _, review := range reviews {
			if err := d.storer.SavePullRequestReview(project, slug, pr.ID, review); err != nil {
				return err
			}
		}
	}

	return nil
}

// DownloadProject downloads the metadata for the given project and its member users
func (d Downloader) DownloadProject(ctx context.Context, name string, version int) error {
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

	resp, err := d.client.DefaultApi.GetProject(ctx, name)
	if err != nil {
		return err
	}

	project, err := GetProjectResponse(resp)
	if err != nil {
		return err
	}

	if err := d.storer.SaveOrganization(project); err != nil {
		return err
	}

	users, err := d.fetchUsers()
	if err != nil {
		return err
	}

	for _, user := range users {
		if err := d.storer.SaveUser(project.ID, project.Key, user); err != nil {
			return err
		}
	}

	return nil
}

// // SetCurrent enables the given version as the current one accessible in the DB
// func (d Downloader) SetCurrent(ctx context.Context, version int) error {
// 	err := d.storer.SetActiveVersion(ctx, version)
// 	if err != nil {
// 		return fmt.Errorf("failed to set current DB version to %v: %v", version, err)
// 	}
// 	return nil
// }

// // Cleanup deletes from the DB all records that do not belong to the currentVersion
// func (d Downloader) Cleanup(ctx context.Context, currentVersion int) error {
// 	err := d.storer.Cleanup(ctx, currentVersion)
// 	if err != nil {
// 		return fmt.Errorf("failed to do cleanup for DB version %v: %v", currentVersion, err)
// 	}
// 	return nil
// }

func (d Downloader) fetchProjects() ([]bitbucketv1.Project, error) {
	var projects []bitbucketv1.Project

	start := 0
	for {
		resp, err := d.client.DefaultApi.GetProjects(map[string]interface{}{
			"limit": defaultLimit, "start": start})
		if err != nil {
			return nil, fmt.Errorf("projects req failed: %v", err)
		}
		projectsPerPage, err := GetProjectsResponse(resp)
		if err != nil {
			return nil, fmt.Errorf("projects decoding failed: %v", err)
		}
		projects = append(projects, projectsPerPage...)

		isLastPage := resp.Values["isLastPage"].(bool)
		if isLastPage {
			break
		}

		start = int(resp.Values["nextPageStart"].(float64))
	}

	return projects, nil
}

func (d Downloader) fetchRepositories(projectKey string) ([]bitbucketv1.Repository, error) {
	var repositories []bitbucketv1.Repository

	start := 0
	for {
		resp, err := d.client.DefaultApi.GetRepositoriesWithOptions(projectKey, map[string]interface{}{
			"limit": defaultLimit, "start": start})
		if err != nil {
			return nil, fmt.Errorf("repos req failed: %v", err)
		}
		pageRepos, err := bitbucketv1.GetRepositoriesResponse(resp)
		if err != nil {
			return nil, fmt.Errorf("repos decoding failed: %v", err)
		}
		repositories = append(repositories, pageRepos...)

		isLastPage := resp.Values["isLastPage"].(bool)
		if isLastPage {
			break
		}

		start = int(resp.Values["nextPageStart"].(float64))
	}

	return repositories, nil
}

func (d Downloader) fetchPullRequests(projectKey, repositorySlug string) ([]bitbucketv1.PullRequest, error) {
	var prs []bitbucketv1.PullRequest

	start := 0
	for {
		resp, err := d.client.DefaultApi.GetPullRequestsPage(projectKey, repositorySlug, map[string]interface{}{
			"limit": defaultLimit, "start": start, "state": "ALL"})
		if err != nil {
			return nil, fmt.Errorf("prs req failed: %v", err)
		}
		pagePRs, err := GetPullRequestsResponse(resp)
		if err != nil {
			return nil, fmt.Errorf("prs decoding failed: %v", err)
		}
		prs = append(prs, pagePRs...)

		isLastPage := resp.Values["isLastPage"].(bool)
		if isLastPage {
			break
		}

		start = int(resp.Values["nextPageStart"].(float64))
	}

	return prs, nil
}

func (d Downloader) enrichPullRequest(projectKey, repositorySlug string, pr bitbucketv1.PullRequest) (*types.PullRequest, error) {
	var commits []types.Commit
	start := 0
	for {
		resp, err := d.client.DefaultApi.GetPullRequestCommitsWithOptions(projectKey, repositorySlug, pr.ID, map[string]interface{}{
			"limit": defaultLimit, "start": start})
		if err != nil {
			return nil, fmt.Errorf("prs commits req failed: %v", err)
		}

		var pageCommits []types.Commit
		err = mapstructure.Decode(resp.Values["values"], &pageCommits)
		if err != nil {
			return nil, fmt.Errorf("prs commits decoding failed: %v", err)
		}
		commits = append(commits, pageCommits...)

		isLastPage := resp.Values["isLastPage"].(bool)
		if isLastPage {
			break
		}

		start = int(resp.Values["nextPageStart"].(float64))
	}

	resp, err := d.client.DefaultApi.GetPullRequestDiff(projectKey, repositorySlug, pr.ID, nil)
	if err != nil {
		return nil, fmt.Errorf("prs commits req failed: %v", err)
	}

	var diffResp types.DiffResp
	err = mapstructure.Decode(resp.Values, &diffResp)
	if err != nil {
		return nil, fmt.Errorf("prs diff decoding failed: %v", err)
	}

	var additions, deletions int
	for _, d := range diffResp.Diffs {
		for _, h := range d.Hunks {
			for _, s := range h.Segments {
				if s.Type == "ADDED" {
					additions += len(s.Lines)
				}
				if s.Type == "REMOVED" {
					deletions += len(s.Lines)
				}
			}
		}
	}

	return &types.PullRequest{
		PullRequest:  pr,
		Commits:      len(commits),
		ChangedFiles: len(diffResp.Diffs),
		Additions:    additions,
		Deletions:    deletions,
	}, nil
}

func expandComment(c types.Comment) []types.Comment {
	comments := []types.Comment{c}
	for _, cc := range c.Comments {
		comments = append(comments, expandComment(cc)...)
	}

	return comments
}

func expandDiffComment(c types.Comment, a types.CommentAnchor) []types.DiffComment {
	comments := []types.DiffComment{types.DiffComment{
		Comment:       c,
		CommentAnchor: a,
	}}
	for _, cc := range c.Comments {
		comments = append(comments, expandDiffComment(cc, a)...)
	}

	return comments
}

func (d Downloader) fetchPRActivity(projectKey, repositorySlug string, pullRequestID int) ([]types.Comment, []types.DiffComment, []types.Review, *types.PRStateUpdate, error) {
	var comments []types.Comment
	var diffComments []types.DiffComment
	var reviews []types.Review
	var state *types.PRStateUpdate

	start := 0
	for {
		resp, err := d.client.DefaultApi.GetPullRequestActivity(projectKey, repositorySlug, pullRequestID, map[string]interface{}{
			"limit": defaultLimit, "start": start,
		})
		if err != nil {
			return nil, nil, nil, nil, fmt.Errorf("activities req failed: %v", err)
		}

		pageActivities, err := GetActivitiesResponse(resp)
		if err != nil {
			return nil, nil, nil, nil, fmt.Errorf("activities decoding failed: %v", err)
		}

		for _, a := range pageActivities {
			switch a.Action {
			case "COMMENTED":
				if a.CommentAction != "ADDED" {
					continue
				}
				if a.CommentAnchor != nil {
					diffComments = append(diffComments, expandDiffComment(a.Comment, *a.CommentAnchor)...)
				} else {
					comments = append(comments, expandComment(a.Comment)...)
				}

			case "APPROVED":
				reviews = append(reviews, types.Review{
					ID:          a.ID,
					State:       "APPROVED",
					User:        a.User,
					CreatedDate: a.CreatedDate,
				})
			case "REVIEWED":
				reviews = append(reviews, types.Review{
					ID:          a.ID,
					State:       "CHANGES_REQUESTED",
					User:        a.User,
					CreatedDate: a.CreatedDate,
				})
			case "MERGED":
				state = &types.PRStateUpdate{
					State: "MERGED",
					User:  a.User,
					Date:  a.CreatedDate,
				}
			case "DECLINED":
				state = &types.PRStateUpdate{
					State: "CLOSED",
					User:  a.User,
					Date:  a.CreatedDate,
				}
			}

		}
		isLastPage := resp.Values["isLastPage"].(bool)
		if isLastPage {
			break
		}

		start = int(resp.Values["nextPageStart"].(float64))
	}

	return comments, diffComments, reviews, state, nil
}

func (d Downloader) fetchUsers() ([]bitbucketv1.User, error) {
	var users []bitbucketv1.User

	start := 0
	for {
		resp, err := d.client.DefaultApi.GetUsers(map[string]interface{}{
			"limit": defaultLimit, "start": start})
		if err != nil {
			return nil, fmt.Errorf("users req failed: %v", err)
		}
		pageUsers, err := GetUsersResponse(resp)
		if err != nil {
			return nil, fmt.Errorf("users decoding failed: %v", err)
		}
		users = append(users, pageUsers...)

		isLastPage := resp.Values["isLastPage"].(bool)
		if isLastPage {
			break
		}

		start = int(resp.Values["nextPageStart"].(float64))
	}
	return users, nil
}

// TODO: move it to the lib

func GetProjectResponse(r *bitbucketv1.APIResponse) (bitbucketv1.Project, error) {
	var m bitbucketv1.Project
	err := mapstructure.Decode(r.Values, &m)
	return m, err
}

func GetProjectsResponse(r *bitbucketv1.APIResponse) ([]bitbucketv1.Project, error) {
	var m []bitbucketv1.Project
	err := mapstructure.Decode(r.Values["values"], &m)
	return m, err
}

func GetPullRequestsResponse(r *bitbucketv1.APIResponse) ([]bitbucketv1.PullRequest, error) {
	var m []bitbucketv1.PullRequest
	err := mapstructure.Decode(r.Values["values"], &m)
	return m, err
}

func GetUsersResponse(r *bitbucketv1.APIResponse) ([]bitbucketv1.User, error) {
	var m []bitbucketv1.User
	err := mapstructure.Decode(r.Values["values"], &m)
	return m, err
}

func GetActivitiesResponse(r *bitbucketv1.APIResponse) ([]types.Activity, error) {
	var m []types.Activity
	err := mapstructure.Decode(r.Values["values"], &m)
	return m, err
}
