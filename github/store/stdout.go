package store

import (
	"context"
	"fmt"

	"github.com/src-d/metadata-retrieval/github/graphql"
)

type Stdout struct{}

func (s *Stdout) SaveOrganization(ctx context.Context, organization *graphql.Organization) error {
	fmt.Printf("organization data fetched for %s\n", organization.Login)
	return nil
}

func (s *Stdout) SaveUser(ctx context.Context, orgID int, orgLogin string, user *graphql.UserExtended) error {
	fmt.Printf("user data fetched for %s\n", user.Login)
	return nil
}

func (s *Stdout) SaveRepository(ctx context.Context, repository *graphql.RepositoryFields, topics []string) error {
	fmt.Printf("repository data fetched for %s/%s\n", repository.Owner.Login, repository.Name)
	return nil
}

func (s *Stdout) SaveIssue(ctx context.Context, repositoryOwner, repositoryName string, issue *graphql.Issue, assignees []string, labels []string) error {
	fmt.Printf("issue data fetched for #%v %s\n", issue.Number, issue.Title)
	return nil
}

func (s *Stdout) SaveIssueComment(ctx context.Context, repositoryOwner, repositoryName string, issueNumber int, comment *graphql.IssueComment) error {
	fmt.Printf("  issue comment data fetched by %s at %v: %q\n", comment.Author.Login, comment.CreatedAt, trim(comment.Body))
	return nil
}

func (s *Stdout) SavePullRequest(ctx context.Context, repositoryOwner, repositoryName string, pr *graphql.PullRequest, assignees []string, labels []string) error {
	fmt.Printf("PR data fetched for #%v %s\n", pr.Number, pr.Title)
	return nil
}

func (s *Stdout) SavePullRequestComment(ctx context.Context, repositoryOwner, repositoryName string, pullRequestNumber int, comment *graphql.IssueComment) error {
	fmt.Printf("  pr comment data fetched by %s at %v: %q\n", comment.Author.Login, comment.CreatedAt, trim(comment.Body))
	return nil
}

func (s *Stdout) SavePullRequestReview(ctx context.Context, repositoryOwner, repositoryName string, pullRequestNumber int, review *graphql.PullRequestReview) error {
	fmt.Printf("  PR Review data fetched by %s at %v: %q\n", review.Author.Login, review.SubmittedAt, trim(review.Body))
	return nil
}

func (s *Stdout) SavePullRequestReviewComment(ctx context.Context, repositoryOwner, repositoryName string, pullRequestNumber int, pullRequestReviewId int, comment *graphql.PullRequestReviewComment) error {
	fmt.Printf("    PR review comment data fetched by %s at %v: %q\n", comment.Author.Login, comment.CreatedAt, trim(comment.Body))
	return nil
}

func (s *Stdout) Begin() error {
	return nil
}

func (s *Stdout) Commit() error {
	return nil
}

func (s *Stdout) Rollback() error {
	return nil
}

func (s *Stdout) Version(v int) {
}

func (s *Stdout) SetActiveVersion(ctx context.Context, v int) error {
	return nil
}

func (s *Stdout) Cleanup(ctx context.Context, currentVersion int) error {
	return nil
}

func trim(s string) string {
	if len(s) > 40 {
		return s[0:39] + "..."
	}

	return s
}
