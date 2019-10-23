package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"fmt"

	"github.com/src-d/metadata-retrieval/github/graphql"

	"github.com/lib/pq"
)

type DB struct {
	*sql.DB
	tx *sql.Tx
	v  int
}

func NewDB(db *sql.DB) *DB {
	return &DB{DB: db}
}

func (s *DB) Begin() error {
	var err error
	s.tx, err = s.DB.Begin()
	return err
}

func (s *DB) Commit() error {
	return s.tx.Commit()
}

func (s *DB) Rollback() error {
	return s.tx.Rollback()
}

func (s *DB) Version(v int) {
	s.v = v
}

const (
	organizationsCols             = "avatar_url, collaborators, created_at, description, email, htmlurl, id, login, name, node_id, owned_private_repos, public_repos, total_private_repos, updated_at"
	usersCols                     = "avatar_url, bio, company, created_at, email, followers, following, hireable, htmlurl, id, location, login, name, node_id, organization_id, organization_login, owned_private_repos, private_gists, public_gists, public_repos, total_private_repos, updated_at"
	repositoriesCols              = "allow_merge_commit, allow_rebase_merge, allow_squash_merge, archived, created_at, default_branch, description, disabled, fork, forks_count, full_name, has_issues, has_wiki, homepage, htmlurl, id, language, name, node_id, open_issues_count, owner_id, owner_login, owner_type, private, pushed_at, sshurl, stargazers_count, topics, updated_at, watchers_count"
	issuesCols                    = "assignees, body, closed_at, closed_by_id, closed_by_login, comments, created_at, htmlurl, id, labels, locked, milestone_id, milestone_title, node_id, number, repository_name, repository_owner, state, title, updated_at, user_id, user_login"
	issueCommentsCols             = "author_association, body, created_at, htmlurl, id, issue_number, node_id, repository_name, repository_owner, updated_at, user_id, user_login"
	pullRequestsCol               = "additions, assignees, author_association, base_ref, base_repository_name, base_repository_owner, base_sha, base_user, body, changed_files, closed_at, comments, commits, created_at, deletions, head_ref, head_repository_name, head_repository_owner, head_sha, head_user, htmlurl, id, labels, maintainer_can_modify, merge_commit_sha, mergeable, merged, merged_at, merged_by_id, merged_by_login, milestone_id, milestone_title, node_id, number, repository_name, repository_owner, review_comments, state, title, updated_at, user_id, user_login"
	pullRequestReviewsCols        = "body, commit_id, htmlurl, id, node_id, pull_request_number, repository_name, repository_owner, state, submitted_at, user_id, user_login"
	pullRequestReviewCommentsCols = "author_association, body, commit_id, created_at, diff_hunk, htmlurl, id, in_reply_to, node_id, original_commit_id, original_position, path, position, pull_request_number, pull_request_review_id, repository_name, repository_owner, updated_at, user_id, user_login"
)

var tables = []string{
	"organizations_versioned",
	"users_versioned",
	"repositories_versioned",
	"issues_versioned",
	"issue_comments_versioned",
	"pull_requests_versioned",
	"pull_request_reviews_versioned",
	"pull_request_comments_versioned",
}

func (s *DB) SetActiveVersion(ctx context.Context, v int) error {
	// TODO: for some reason the normal parameter interpolation $1 fails with
	// pq: got 1 parameters but the statement requires 0

	_, err := s.DB.ExecContext(ctx, fmt.Sprintf(`CREATE OR REPLACE VIEW organizations AS
	SELECT %s
	FROM organizations_versioned WHERE %v = ANY(versions)`, organizationsCols, v))
	if err != nil {
		return fmt.Errorf("failed to create VIEW organizations: %v", err)
	}

	_, err = s.DB.ExecContext(ctx, fmt.Sprintf(`CREATE OR REPLACE VIEW users AS
	SELECT %s
	FROM users_versioned WHERE %v = ANY(versions)`, usersCols, v))
	if err != nil {
		return fmt.Errorf("failed to create VIEW users: %v", err)
	}

	_, err = s.DB.ExecContext(ctx, fmt.Sprintf(`CREATE OR REPLACE VIEW repositories AS
	SELECT %s
	FROM repositories_versioned WHERE %v = ANY(versions)`, repositoriesCols, v))
	if err != nil {
		return fmt.Errorf("failed to create VIEW repositories: %v", err)
	}

	_, err = s.DB.ExecContext(ctx, fmt.Sprintf(`CREATE OR REPLACE VIEW issues AS
	SELECT %s
	FROM issues_versioned WHERE %v = ANY(versions)`, issuesCols, v))
	if err != nil {
		return fmt.Errorf("failed to create VIEW issues: %v", err)
	}

	_, err = s.DB.ExecContext(ctx, fmt.Sprintf(`CREATE OR REPLACE VIEW issue_comments AS
	SELECT %s
	FROM issue_comments_versioned WHERE %v = ANY(versions)`, issueCommentsCols, v))
	if err != nil {
		return fmt.Errorf("failed to create VIEW issue_comments: %v", err)
	}

	_, err = s.DB.ExecContext(ctx, fmt.Sprintf(`CREATE OR REPLACE VIEW pull_requests AS
	SELECT %s
	FROM pull_requests_versioned WHERE %v = ANY(versions)`, pullRequestsCol, v))
	if err != nil {
		return fmt.Errorf("failed to create VIEW pull_requests: %v", err)
	}

	_, err = s.DB.ExecContext(ctx, fmt.Sprintf(`CREATE OR REPLACE VIEW pull_request_reviews AS
	SELECT %s
	FROM pull_request_reviews_versioned WHERE %v = ANY(versions)`, pullRequestReviewsCols, v))
	if err != nil {
		return fmt.Errorf("failed to create VIEW pull_request_reviews: %v", err)
	}

	_, err = s.DB.ExecContext(ctx, fmt.Sprintf(`CREATE OR REPLACE VIEW pull_request_comments AS
	SELECT %s
	FROM pull_request_comments_versioned WHERE %v = ANY(versions)`, pullRequestReviewCommentsCols, v))
	if err != nil {
		return fmt.Errorf("failed to create VIEW pull_request_comments: %v", err)
	}

	return nil
}

func (s *DB) Cleanup(ctx context.Context, currentVersion int) error {
	for _, table := range tables {
		// Delete all entries that do not belong to currentVersion
		_, err := s.DB.ExecContext(ctx, fmt.Sprintf(`DELETE FROM %s WHERE %v <> ALL(versions)`, table, currentVersion))
		if err != nil {
			return fmt.Errorf("failed in cleanup method, delete: %v", err)
		}

		// All remaining entries belong to currentVersion, replace the list of versions
		// with an array of 1 entry
		_, err = s.DB.ExecContext(ctx, fmt.Sprintf(`UPDATE %s SET versions = array[%v]`, table, currentVersion))
		if err != nil {
			return fmt.Errorf("failed in cleanup method, update: %v", err)
		}
	}

	return nil
}

func (s *DB) SaveOrganization(ctx context.Context, organization *graphql.Organization) error {
	statement := fmt.Sprintf(
		`INSERT INTO organizations_versioned
		(sum256, versions, %s)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14,
			$15, $16)
		ON CONFLICT (sum256)
		DO UPDATE
		SET versions = array_append(organizations_versioned.versions, $17)`,
		organizationsCols)

	st := fmt.Sprintf("%+v", organization)
	hash := sha256.Sum256([]byte(st))
	hashString := fmt.Sprintf("%x", hash)

	_, err := s.tx.ExecContext(ctx, statement,
		hashString,
		pq.Array([]int{s.v}),

		organization.AvatarURL,                    // avatar_url text,
		organization.MembersWithRole.TotalCount,   // collaborators bigint,
		organization.CreatedAt,                    // created_at timestamptz,
		organization.Description,                  // description text,
		organization.Email,                        // email text,
		organization.URL,                          // htmlurl text,
		organization.DatabaseID,                   // id bigint,
		organization.Login,                        // login text,
		organization.Name,                         // name text,
		organization.ID,                           // node_id text,
		organization.OwnedPrivateRepos.TotalCount, // owned_private_repos bigint,
		organization.PublicRepos.TotalCount,       // public_repos bigint,
		organization.TotalPrivateRepos.TotalCount, // total_private_repos bigint,
		organization.UpdatedAt,                    // updated_at timestamptz,

		s.v,
	)

	if err != nil {
		return fmt.Errorf("SaveOrganization: %v", err)
	}
	return nil
}

func (s *DB) SaveUser(ctx context.Context, orgID int, orgLogin string, user *graphql.UserExtended) error {
	statement := fmt.Sprintf(
		`INSERT INTO users_versioned
		(sum256, versions, %s)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14,
			$15, $16, $17, $18, $19, $20, $21, $22, $23, $24)
		ON CONFLICT (sum256)
		DO UPDATE
		SET versions = array_append(users_versioned.versions, $25)`,
		usersCols)

	st := fmt.Sprintf("%+v", user)
	hash := sha256.Sum256([]byte(st))
	hashString := fmt.Sprintf("%x", hash)

	_, err := s.tx.ExecContext(ctx, statement,
		hashString,
		pq.Array([]int{s.v}),

		user.AvatarURL, // avatar_url text,
		user.Bio,       // bio text,
		user.Company,   // company text,
		user.CreatedAt, // created_at timestamptz,
		// TODO
		"",                                // user.Email, // email text,
		user.Followers.TotalCount,         // followers bigint,
		user.Following.TotalCount,         // following bigint,
		user.IsHireable,                   // hireable boolean,
		user.URL,                          // htmlurl text,
		user.DatabaseID,                   // id bigint,
		user.Location,                     // location text,
		user.Login,                        // login text,
		user.Name,                         // name text,
		user.ID,                           // node_id text,
		orgID,                             // organization_id bigint NOT NULL
		orgLogin,                          // organization_login text NOT NULL
		user.OwnedPrivateRepos.TotalCount, // owned_private_repos bigint,
		// TODO: gists makes the server return: You don't have permission to see gists.
		0,                                 // private_gists bigint,
		0,                                 // public_gists bigint,
		user.PublicRepos.TotalCount,       // public_repos bigint,
		user.TotalPrivateRepos.TotalCount, // total_private_repos bigint,
		user.UpdatedAt,                    // updated_at timestamptz,

		s.v,
	)

	if err != nil {
		return fmt.Errorf("saveUser: %v", err)
	}
	return nil
}

func (s *DB) SaveRepository(ctx context.Context, repository *graphql.RepositoryFields, topics []string) error {
	statement := fmt.Sprintf(
		`INSERT INTO repositories_versioned
		(sum256, versions, %s)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14,
			$15, $16, $17, $18, $19, $20, $21, $22, $23, $24, $25, $26, $27, $28, $29,
			$30, $31, $32)
		ON CONFLICT (sum256)
		DO UPDATE
		SET versions = array_append(repositories_versioned.versions, $33)`,
		repositoriesCols)

	st := fmt.Sprintf("%+v %v", repository, topics)
	hash := sha256.Sum256([]byte(st))
	hashString := fmt.Sprintf("%x", hash)

	_, err := s.tx.ExecContext(ctx, statement,
		hashString,
		pq.Array([]int{s.v}),

		repository.MergeCommitAllowed,    // allow_merge_commit boolean
		repository.RebaseMergeAllowed,    // allow_rebase_merge boolean
		repository.SquashMergeAllowed,    // allow_squash_merge boolean
		repository.IsArchived,            // archived boolean
		repository.CreatedAt,             // created_at timestamptz
		repository.DefaultBranchRef.Name, // default_branch text
		repository.Description,           // description text
		repository.IsDisabled,            // disabled boolean
		repository.IsFork,                // fork boolean
		repository.ForkCount,             // forks_count bigint
		repository.NameWithOwner,         // full_name text
		repository.HasIssuesEnabled,      // has_issues boolean
		repository.HasWikiEnabled,        // has_wiki boolean
		repository.HomepageURL,           // homepage text
		repository.URL,                   // htmlurl text
		repository.DatabaseID,            // id bigint,
		repository.PrimaryLanguage.Name,  // language text
		repository.Name,                  // name text
		repository.ID,                    // node_id text
		repository.OpenIssues.TotalCount, // open_issues_count bigint
		repoOwnerID(repository),          // owner_id bigint NOT NULL,
		repository.Owner.Login,           // owner_login text NOT NULL,
		repository.Owner.Typename,        // owner_type text NOT NULL
		repository.IsPrivate,             // private boolean
		repository.PushedAt,              // pushed_at timestamptz
		repository.SSHURL,                // sshurl text
		repository.Stargazers.TotalCount, // stargazers_count bigint
		pq.Array(topics),                 // topics text[] NOT NULL
		repository.UpdatedAt,             // updated_at timestamptz
		repository.Watchers.TotalCount,   // watchers_count bigint

		s.v,
	)

	if err != nil {
		return fmt.Errorf("saveRepository: %v", err)
	}
	return nil
}

func repoOwnerID(repository *graphql.RepositoryFields) int {
	switch repository.Owner.Typename {
	case "Orgazation":
		return repository.Owner.Organization.DatabaseID
	case "User":
		return repository.Owner.User.DatabaseID
	default:
		return 0
	}
}

func (s *DB) SaveIssue(ctx context.Context, repositoryOwner, repositoryName string, issue *graphql.Issue, assignees []string, labels []string) error {
	statement := fmt.Sprintf(
		`INSERT INTO issues_versioned
		(sum256, versions, %s)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14,
			$15, $16, $17, $18, $19, $20, $21, $22, $23, $24)
		ON CONFLICT (sum256)
		DO UPDATE
		SET versions = array_append(issues_versioned.versions, $25)`,
		issuesCols)

	st := fmt.Sprintf("%v %v %+v %v %v", repositoryOwner, repositoryName, issue, assignees, labels)
	hash := sha256.Sum256([]byte(st))
	hashString := fmt.Sprintf("%x", hash)

	closedByID := 0
	closedByLogin := ""

	if len(issue.ClosedBy.Nodes) > 0 {
		closedByID = issue.ClosedBy.Nodes[0].ClosedEvent.Actor.DatabaseID
		closedByLogin = issue.ClosedBy.Nodes[0].ClosedEvent.Actor.Login
	}

	_, err := s.tx.ExecContext(ctx, statement,
		hashString,
		pq.Array([]int{s.v}),

		pq.Array(assignees),          // assignees text[] NOT NULL,
		issue.Body,                   // body text,
		issue.ClosedAt,               // closed_at timestamptz,
		closedByID,                   // closed_by_id bigint NOT NULL
		closedByLogin,                // closed_by_login text NOT NULL,
		issue.Comments.TotalCount,    // comments bigint,
		issue.CreatedAt,              // created_at timestamptz,
		issue.URL,                    // htmlurl text,
		issue.DatabaseID,             // id bigint,
		pq.Array(labels),             // labels text[] NOT NULL,
		issue.Locked,                 // locked boolean,
		issue.Milestone.ID,           // milestone_id text NOT NULL,
		issue.Milestone.Title,        // milestone_title text NOT NULL,
		issue.ID,                     // node_id text,
		issue.Number,                 // number bigint,
		repositoryName,               // repository_name text NOT NULL,
		repositoryOwner,              // repository_owner text NOT NULL,
		issue.State,                  // state text,
		issue.Title,                  // title text,
		issue.UpdatedAt,              // updated_at timestamptz,
		issue.Author.User.DatabaseID, // user_id bigint NOT NULL,
		issue.Author.Login,           // user_login text NOT NULL,

		s.v,
	)

	if err != nil {
		return fmt.Errorf("saveIssue: %v", err)
	}
	return nil
}

func (s *DB) SaveIssueComment(ctx context.Context, repositoryOwner, repositoryName string, issueNumber int, comment *graphql.IssueComment) error {
	statement := fmt.Sprintf(`INSERT INTO issue_comments_versioned
		(sum256, versions, %s)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
		ON CONFLICT (sum256)
		DO UPDATE
		SET versions = array_append(issue_comments_versioned.versions, $15)`,
		issueCommentsCols)

	st := fmt.Sprintf("%v %v %v %+v", repositoryOwner, repositoryName, issueNumber, comment)
	hash := sha256.Sum256([]byte(st))
	hashString := fmt.Sprintf("%x", hash)

	_, err := s.tx.ExecContext(ctx, statement,
		hashString,
		pq.Array([]int{s.v}),

		comment.AuthorAssociation,      // author_association text,
		comment.Body,                   // body text,
		comment.CreatedAt,              // created_at timestamptz,
		comment.URL,                    // htmlurl text,
		comment.DatabaseID,             // id bigint,
		issueNumber,                    // issue_number bigint NOT NULL,
		comment.ID,                     // node_id text,
		repositoryName,                 // repository_name text NOT NULL,
		repositoryOwner,                // repository_owner text NOT NULL,
		comment.UpdatedAt,              // updated_at timestamptz,
		comment.Author.User.DatabaseID, // user_id bigint NOT NULL,
		comment.Author.Login,           // user_login text NOT NULL,

		s.v,
	)

	if err != nil {
		return fmt.Errorf("saveIssueComment: %v", err)
	}
	return nil
}

func (s *DB) SavePullRequest(ctx context.Context, repositoryOwner, repositoryName string, pr *graphql.PullRequest, assignees []string, labels []string) error {
	statement := fmt.Sprintf(
		`INSERT INTO pull_requests_versioned
		(sum256, versions, %s)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14,
			$15, $16, $17, $18, $19, $20, $21, $22, $23, $24, $25, $26, $27, $28, $29,
			$30, $31, $32, $33, $34, $35, $36, $37, $38, $39, $40, $41, $42, $43, $44)
		ON CONFLICT (sum256)
		DO UPDATE
		SET versions = array_append(pull_requests_versioned.versions, $45)`,
		pullRequestsCol)

	st := fmt.Sprintf("%v %v %+v %v %v", repositoryOwner, repositoryName, pr, assignees, labels)
	hash := sha256.Sum256([]byte(st))
	hashString := fmt.Sprintf("%x", hash)

	_, err := s.tx.ExecContext(ctx, statement,
		hashString,
		pq.Array([]int{s.v}),

		pr.Additions,                               // additions bigint,
		pq.Array(assignees),                        // assignees text[] NOT NULL,
		pr.AuthorAssociation,                       // author_association text,
		pr.BaseRef.Name,                            // base_ref text NOT NULL,
		pr.BaseRef.Repository.Name,                 // base_repository_name text NOT NULL,
		pr.BaseRef.Repository.Owner.Login,          // base_repository_owner text NOT NULL,
		pr.BaseRef.Target.Oid,                      // base_sha text NOT NULL,
		pr.BaseRef.Target.Commit.Author.User.Login, // base_user text NOT NULL,
		pr.Body,                           // body text,
		pr.ChangedFiles,                   // changed_files bigint,
		pr.ClosedAt,                       // closed_at timestamptz,
		pr.Comments.TotalCount,            // comments bigint,
		pr.Commits.TotalCount,             // commits bigint,
		pr.CreatedAt,                      // created_at timestamptz,
		pr.Deletions,                      // deletions bigint,
		pr.HeadRef.Name,                   // head_ref text NOT NULL,
		pr.HeadRef.Repository.Name,        // head_repository_name text NOT NULL,
		pr.HeadRef.Repository.Owner.Login, // head_repository_owner text NOT NULL,
		pr.HeadRef.Target.Oid,             // head_sha text NOT NULL,
		pr.HeadRef.Target.Commit.Author.User.Login, // head_user text NOT NULL,
		pr.URL,                      // htmlurl text,
		pr.DatabaseID,               // id bigint,
		pq.Array(labels),            // labels text[] NOT NULL,
		pr.MaintainerCanModify,      // maintainer_can_modify boolean,
		pr.MergeCommit.Oid,          // merge_commit_sha text,
		pr.Mergeable == "MERGEABLE", // mergeable boolean,
		pr.Merged,                   // merged boolean,
		pr.MergedAt,                 // merged_at timestamptz,
		pr.MergedBy.DatabaseID,      // merged_by_id bigint NOT NULL,
		pr.MergedBy.Login,           // merged_by_login text NOT NULL,
		pr.Milestone.ID,             // milestone_id text NOT NULL,
		pr.Milestone.Title,          // milestone_title text NOT NULL,
		pr.ID,                       // node_id text,
		pr.Number,                   // number bigint,
		repositoryName,              // repository_name text NOT NULL,
		repositoryOwner,             // repository_owner text NOT NULL,
		pr.ReviewThreads.TotalCount, // review_comments bigint,
		pr.State,                    // state text,
		pr.Title,                    // title text,
		pr.UpdatedAt,                // updated_at timestamptz,
		pr.Author.DatabaseID,        // user_id bigint NOT NULL,
		pr.Author.Login,             // user_login text NOT NULL,

		s.v,
	)

	if err != nil {
		return fmt.Errorf("savePullRequest: %v", err)
	}
	return nil
}

func (s *DB) SavePullRequestComment(ctx context.Context, repositoryOwner, repositoryName string, pullRequestNumber int, comment *graphql.IssueComment) error {
	// ghsync saves both Issue and PRs comments in the same table, issue_comments
	return s.SaveIssueComment(ctx, repositoryOwner, repositoryName, pullRequestNumber, comment)
}

func (s *DB) SavePullRequestReview(ctx context.Context, repositoryOwner, repositoryName string, pullRequestNumber int, review *graphql.PullRequestReview) error {
	statement := fmt.Sprintf(`INSERT INTO pull_request_reviews_versioned
		(sum256, versions, %s)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
		ON CONFLICT (sum256)
		DO UPDATE
		SET versions = array_append(pull_request_reviews_versioned.versions, $15)`,
		pullRequestReviewsCols)

	st := fmt.Sprintf("%v %v %v %+v", repositoryOwner, repositoryName, pullRequestNumber, review)
	hash := sha256.Sum256([]byte(st))
	hashString := fmt.Sprintf("%x", hash)

	_, err := s.tx.ExecContext(ctx, statement,
		hashString,
		pq.Array([]int{s.v}),

		review.Body,                   // body text,
		review.Commit.Oid,             // commit_id text,
		review.URL,                    // htmlurl text,
		review.DatabaseID,             // id bigint,
		review.ID,                     // node_id text,
		pullRequestNumber,             // pull_request_number bigint NOT NULL,
		repositoryName,                // repository_name text NOT NULL,
		repositoryOwner,               // repository_owner text NOT NULL,
		review.State,                  // state text,
		review.SubmittedAt,            // submitted_at timestamptz,
		review.Author.User.DatabaseID, // user_id bigint NOT NULL,
		review.Author.Login,           // user_login text NOT NULL,

		s.v,
	)

	if err != nil {
		return fmt.Errorf("savePullRequestComment: %v", err)
	}
	return nil
}

func (s *DB) SavePullRequestReviewComment(ctx context.Context, repositoryOwner, repositoryName string, pullRequestNumber int, pullRequestReviewId int, comment *graphql.PullRequestReviewComment) error {
	statement := fmt.Sprintf(`INSERT INTO pull_request_comments_versioned
		(sum256, versions, %s)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14,
			$15, $16, $17, $18, $19, $20, $21, $22)
		ON CONFLICT (sum256)
		DO UPDATE
		SET versions = array_append(pull_request_comments_versioned.versions, $23)`,
		pullRequestReviewCommentsCols)

	st := fmt.Sprintf("%v %v %v %v %+v", repositoryOwner, repositoryName, pullRequestNumber, pullRequestReviewId, comment)
	hash := sha256.Sum256([]byte(st))
	hashString := fmt.Sprintf("%x", hash)

	_, err := s.tx.ExecContext(ctx, statement,
		hashString,
		pq.Array([]int{s.v}),

		comment.AuthorAssociation, // author_association text,
		comment.Body,              // body text,
		comment.Commit.Oid,        // commit_id text,
		comment.CreatedAt,         // created_at timestamptz,
		comment.DiffHunk,          // diff_hunk text,
		comment.URL,               // htmlurl text,
		comment.DatabaseID,        // id bigint,
		// TODO
		0,                          // in_reply_to bigint,
		comment.ID,                 // node_id text,
		comment.OriginalCommit.Oid, // original_commit_id text,
		comment.OriginalPosition,   // original_position bigint,
		comment.Path,               // path text,
		comment.Position,           // position bigint,
		pullRequestNumber,          // pull_request_number bigint NOT NULL,
		pullRequestReviewId,        // pull_request_review_id bigint,
		repositoryName,             // repository_name text NOT NULL,
		repositoryOwner,            // repository_owner text NOT NULL,
		comment.UpdatedAt,          // updated_at timestamptz,
		comment.Author.DatabaseID,  // user_id bigint NOT NULL,
		comment.Author.Login,       // user_login text NOT NULL,

		s.v,
	)

	if err != nil {
		return fmt.Errorf("savePullRequestReviewComment: %v", err)
	}
	return nil
}
