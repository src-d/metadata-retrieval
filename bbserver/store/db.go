package store

import (
	"crypto/sha256"
	"database/sql"
	"fmt"
	"time"

	bitbucketv1 "github.com/gfleury/go-bitbucket-v1"
	"github.com/src-d/metadata-retrieval/bbserver/types"

	"github.com/lib/pq"
)

type DB struct {
	*sql.DB
	tx *sql.Tx
	v  int
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
	repositoriesCols              = "allow_merge_commit, allow_rebase_merge, allow_squash_merge, archived, clone_url, created_at, default_branch, description, disabled, fork, forks_count, full_name, has_issues, has_wiki, homepage, htmlurl, id, language, name, node_id, open_issues_count, owner_id, owner_login, owner_type, private, pushed_at, sshurl, stargazers_count, topics, updated_at, watchers_count"
	issueCommentsCols             = "author_association, body, created_at, htmlurl, id, issue_number, node_id, repository_name, repository_owner, updated_at, user_id, user_login"
	pullRequestsCol               = "additions, assignees, author_association, base_ref, base_repository_name, base_repository_owner, base_sha, base_user, body, changed_files, closed_at, comments, commits, created_at, deletions, head_ref, head_repository_name, head_repository_owner, head_sha, head_user, htmlurl, id, labels, maintainer_can_modify, merge_commit_sha, mergeable, merged, merged_at, merged_by_id, merged_by_login, milestone_id, milestone_title, node_id, number, repository_name, repository_owner, review_comments, state, title, updated_at, user_id, user_login"
	pullRequestReviewsCols        = "body, commit_id, htmlurl, id, node_id, pull_request_number, repository_name, repository_owner, state, submitted_at, user_id, user_login"
	pullRequestReviewCommentsCols = "author_association, body, commit_id, created_at, diff_hunk, htmlurl, id, in_reply_to, node_id, original_commit_id, original_position, path, position, pull_request_number, pull_request_review_id, repository_name, repository_owner, updated_at, user_id, user_login"
)

var tables = []string{
	"organizations_versioned",
	"users_versioned",
	"repositories_versioned",
	"issue_comments_versioned",
	"pull_requests_versioned",
	"pull_request_reviews_versioned",
	"pull_request_comments_versioned",
}

func (s *DB) SetActiveVersion(v int) error {
	// TODO: for some reason the normal parameter interpolation $1 fails with
	// pq: got 1 parameters but the statement requires 0

	_, err := s.DB.Exec(fmt.Sprintf(`CREATE OR REPLACE VIEW organizations AS
	SELECT %s
	FROM organizations_versioned WHERE %v = ANY(versions)`, organizationsCols, v))
	if err != nil {
		return fmt.Errorf("failed to create VIEW organizations: %v", err)
	}

	_, err = s.DB.Exec(fmt.Sprintf(`CREATE OR REPLACE VIEW users AS
	SELECT %s
	FROM users_versioned WHERE %v = ANY(versions)`, usersCols, v))
	if err != nil {
		return fmt.Errorf("failed to create VIEW users: %v", err)
	}

	_, err = s.DB.Exec(fmt.Sprintf(`CREATE OR REPLACE VIEW repositories AS
	SELECT %s
	FROM repositories_versioned WHERE %v = ANY(versions)`, repositoriesCols, v))
	if err != nil {
		return fmt.Errorf("failed to create VIEW repositories: %v", err)
	}

	_, err = s.DB.Exec(fmt.Sprintf(`CREATE OR REPLACE VIEW issue_comments AS
	SELECT %s
	FROM issue_comments_versioned WHERE %v = ANY(versions)`, issueCommentsCols, v))
	if err != nil {
		return fmt.Errorf("failed to create VIEW issue_comments: %v", err)
	}

	_, err = s.DB.Exec(fmt.Sprintf(`CREATE OR REPLACE VIEW pull_requests AS
	SELECT %s
	FROM pull_requests_versioned WHERE %v = ANY(versions)`, pullRequestsCol, v))
	if err != nil {
		return fmt.Errorf("failed to create VIEW pull_requests: %v", err)
	}

	_, err = s.DB.Exec(fmt.Sprintf(`CREATE OR REPLACE VIEW pull_request_reviews AS
	SELECT %s
	FROM pull_request_reviews_versioned WHERE %v = ANY(versions)`, pullRequestReviewsCols, v))
	if err != nil {
		return fmt.Errorf("failed to create VIEW pull_request_reviews: %v", err)
	}

	_, err = s.DB.Exec(fmt.Sprintf(`CREATE OR REPLACE VIEW pull_request_comments AS
	SELECT %s
	FROM pull_request_comments_versioned WHERE %v = ANY(versions)`, pullRequestReviewCommentsCols, v))
	if err != nil {
		return fmt.Errorf("failed to create VIEW pull_request_comments: %v", err)
	}

	return nil
}

func (s *DB) Cleanup(currentVersion int) error {
	for _, table := range tables {
		// Delete all entries that do not belong to currentVersion
		_, err := s.DB.Exec(fmt.Sprintf(`DELETE FROM %s WHERE %v <> ALL(versions)`, table, currentVersion))
		if err != nil {
			return fmt.Errorf("failed in cleanup method, delete: %v", err)
		}

		// All remaining entries belong to currentVersion, replace the list of versions
		// with an array of 1 entry
		_, err = s.DB.Exec(fmt.Sprintf(`UPDATE %s SET versions = array[%v]`, table, currentVersion))
		if err != nil {
			return fmt.Errorf("failed in cleanup method, update: %v", err)
		}
	}

	return nil
}

func (s *DB) SaveOrganization(project bitbucketv1.Project) error {
	statement := fmt.Sprintf(
		`INSERT INTO organizations_versioned
		(sum256, versions, %s)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14,
			$15, $16)
		ON CONFLICT (sum256)
		DO UPDATE
		SET versions = array_append(organizations_versioned.versions, $17)`,
		organizationsCols)

	st := fmt.Sprintf("%+v", project)
	hash := sha256.Sum256([]byte(st))
	hashString := fmt.Sprintf("%x", hash)

	_, err := s.tx.Exec(statement,
		hashString,
		pq.Array([]int{s.v}),

		"",                         // avatar_url text,
		0,                          // collaborators bigint,
		nil,                        // created_at timestamptz,
		project.Description,        // description text,
		"",                         // email text,
		project.Links.Self[0].Href, // htmlurl text,
		project.ID,                 // id bigint,
		project.Key,                // login text,
		project.Name,               // name text,
		"",                         // node_id text,
		0,                          // owned_private_repos bigint,
		0,                          // public_repos bigint,
		0,                          // total_private_repos bigint,
		nil,                        // updated_at timestamptz,

		s.v,
	)

	if err != nil {
		return fmt.Errorf("SaveOrganization: %v", err)
	}
	return nil
}

func (s *DB) SaveUser(orgID int, orgLogin string, user bitbucketv1.User) error {
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

	_, err := s.tx.Exec(statement,
		hashString,
		pq.Array([]int{s.v}),

		"",  // avatar_url text,
		"",  // bio text,
		"",  // company text,
		nil, // created_at timestamptz,
		// TODO
		user.Email, // email text,
		0,          // followers bigint,
		0,          // following bigint,
		false,      // hireable boolean,
		"",         // htmlurl text,
		user.ID,    // id bigint,
		"",         // location text,
		user.Slug,  // login text,
		user.Name,  // name text,
		"",         // node_id text,
		orgID,      // organization_id bigint NOT NULL
		orgLogin,   // organization_login text NOT NULL
		0,          // owned_private_repos bigint,
		0,          // private_gists bigint,
		0,          // public_gists bigint,
		0,          // public_repos bigint,
		0,          // total_private_repos bigint,
		nil,        // updated_at timestamptz,

		s.v,
	)

	if err != nil {
		return fmt.Errorf("saveUser: %v", err)
	}
	return nil
}

func (s *DB) SaveRepository(repository bitbucketv1.Repository) error {
	statement := fmt.Sprintf(
		`INSERT INTO repositories_versioned
		(sum256, versions, %s)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14,
			$15, $16, $17, $18, $19, $20, $21, $22, $23, $24, $25, $26, $27, $28, $29,
			$30, $31, $32, $33)
		ON CONFLICT (sum256)
		DO UPDATE
		SET versions = array_append(repositories_versioned.versions, $34)`,
		repositoriesCols)

	st := fmt.Sprintf("%+v", repository)
	hash := sha256.Sum256([]byte(st))
	hashString := fmt.Sprintf("%x", hash)

	_, err := s.tx.Exec(statement,
		hashString,
		pq.Array([]int{s.v}),

		false,                          // allow_merge_commit boolean
		false,                          // allow_rebase_merge boolean
		false,                          // allow_squash_merge boolean
		false,                          // archived boolean
		repository.Links.Clone[0].Href, // clone_url text
		nil,                            // created_at timestamptz
		"",                             // default_branch text
		"",                             // description text
		false,                          // disabled boolean
		false,                          // fork boolean
		0,                              // forks_count bigint
		repository.Slug,                // full_name text
		false,                          // has_issues boolean
		false,                          // has_wiki boolean
		"",                             // homepage text
		repository.Links.Self[0].Href,  // htmlurl text
		repository.ID,                  // id bigint,
		"",                             // language text
		repository.Name,                // name text
		"",                             // node_id text
		0,                              // open_issues_count bigint
		repository.Project.ID,          // owner_id bigint NOT NULL,
		repository.Project.Key,         // owner_login text NOT NULL,
		"",                             // owner_type text NOT NULL
		!repository.Public,             // private boolean
		nil,                            // pushed_at timestamptz
		repository.Links.Clone[1].Href, // sshurl text
		0,                              // stargazers_count bigint
		pq.Array([]string{}),           // topics text[] NOT NULL
		nil,                            // updated_at timestamptz
		0,                              // watchers_count bigint

		s.v,
	)

	if err != nil {
		return fmt.Errorf("saveRepository: %v", err)
	}
	return nil
}

func (s *DB) SavePullRequest(repositoryOwner, repositoryName string, pr types.PullRequest) error {
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

	st := fmt.Sprintf("%v %v %+v", repositoryOwner, repositoryName, pr)
	hash := sha256.Sum256([]byte(st))
	hashString := fmt.Sprintf("%x", hash)

	var closedAt *time.Time
	if pr.ClosedAt > 0 {
		t := time.Unix(pr.ClosedAt/1000, 0)
		closedAt = &t
	}
	var mergedAt *time.Time
	if pr.MergedAt > 0 {
		t := time.Unix(pr.MergedAt/1000, 0)
		mergedAt = &t
	}

	_, err := s.tx.Exec(statement,
		hashString,
		pq.Array([]int{s.v}),

		pr.Additions,                             // additions bigint,
		pq.Array([]string{}),                     // assignees text[] NOT NULL,
		"",                                       // author_association text,
		pr.ToRef.ID,                              // base_ref text NOT NULL,
		pr.ToRef.Repository.Name,                 // base_repository_name text NOT NULL,
		pr.ToRef.Repository.Project.Key,          // base_repository_owner text NOT NULL,
		pr.ToRef.LatestCommit,                    // base_sha text NOT NULL,
		"",                                       // base_user text NOT NULL,
		pr.Description,                           // body text,
		pr.ChangedFiles,                          // changed_files bigint,
		closedAt,                                 // closed_at timestamptz,
		pr.Comments,                              // comments bigint,
		pr.Commits,                               // commits bigint,
		time.Unix(int64(pr.CreatedDate/1000), 0), // created_at timestamptz,
		pr.Deletions,                             // deletions bigint,
		pr.FromRef.ID,                            // head_ref text NOT NULL,
		pr.FromRef.Repository.Name,               // head_repository_name text NOT NULL,
		pr.FromRef.Repository.Project.Key,        // head_repository_owner text NOT NULL,
		pr.FromRef.LatestCommit,                  // head_sha text NOT NULL,
		"",                                       // head_user text NOT NULL,
		pr.Links.Self[0].Href,                    // htmlurl text,
		pr.ID,                                    // id bigint,
		pq.Array([]string{}),                     // labels text[] NOT NULL,
		false,                                    // maintainer_can_modify boolean,
		"",                                       // merge_commit_sha text,
		false,                                    // mergeable boolean,
		pr.State == "MERGED",                     // merged boolean,
		mergedAt,                                 // merged_at timestamptz,
		pr.MergedBy.ID,                           // merged_by_id bigint NOT NULL,
		pr.MergedBy.Name,                         // merged_by_login text NOT NULL,
		"",                                       // milestone_id text NOT NULL,
		"",                                       // milestone_title text NOT NULL,
		"",                                       // node_id text,
		pr.ID,                                    // number bigint,
		repositoryName,                           // repository_name text NOT NULL,
		repositoryOwner,                          // repository_owner text NOT NULL,
		pr.ReviewComments,                        // review_comments bigint,
		pr.State,                                 // state text,
		pr.Title,                                 // title text,
		time.Unix(int64(pr.UpdatedDate/1000), 0), // updated_at timestamptz,
		pr.Author.User.ID,                        // user_id bigint NOT NULL,
		pr.Author.User.Slug,                      // user_login text NOT NULL,

		s.v,
	)

	if err != nil {
		return fmt.Errorf("savePullRequest: %v", err)
	}
	return nil
}

func (s *DB) SavePullRequestComment(repositoryOwner, repositoryName string, pullRequestNumber int, comment types.Comment) error {
	// ghsync saves both Issue and PRs comments in the same table, issue_comments
	return s.SaveIssueComment(repositoryOwner, repositoryName, pullRequestNumber, comment)
}

func (s *DB) SaveIssueComment(repositoryOwner, repositoryName string, issueNumber int, comment types.Comment) error {
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

	_, err := s.tx.Exec(statement,
		hashString,
		pq.Array([]int{s.v}),

		"",           // author_association text,
		comment.Text, // body text,
		time.Unix(int64(comment.CreatedDate/1000), 0), // created_at timestamptz,
		"",              // htmlurl text,
		comment.ID,      // id bigint,
		issueNumber,     // issue_number bigint NOT NULL,
		"",              // node_id text,
		repositoryName,  // repository_name text NOT NULL,
		repositoryOwner, // repository_owner text NOT NULL,
		time.Unix(int64(comment.UpdatedDate/1000), 0), // updated_at timestamptz,
		comment.Author.ID,   // user_id bigint NOT NULL,
		comment.Author.Slug, // user_login text NOT NULL,

		s.v,
	)

	if err != nil {
		return fmt.Errorf("saveIssueComment: %v", err)
	}
	return nil
}

func (s *DB) SavePullRequestReview(repositoryOwner, repositoryName string, pullRequestNumber int, review types.Review) error {
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

	_, err := s.tx.Exec(statement,
		hashString,
		pq.Array([]int{s.v}),

		"",                // body text,
		"",                // commit_id text,
		"",                // htmlurl text,
		review.ID,         // id bigint,
		"",                // node_id text,
		pullRequestNumber, // pull_request_number bigint NOT NULL,
		repositoryName,    // repository_name text NOT NULL,
		repositoryOwner,   // repository_owner text NOT NULL,
		review.State,      // state text,
		time.Unix(int64(review.CreatedDate/1000), 0), // submitted_at timestamptz,
		review.User.ID,   // user_id bigint NOT NULL,
		review.User.Slug, // user_login text NOT NULL,

		s.v,
	)

	if err != nil {
		return fmt.Errorf("savePullRequestReview: %v", err)
	}
	return nil
}

func (s *DB) SavePullRequestReviewComment(repositoryOwner, repositoryName string, pullRequestNumber int, comment types.DiffComment) error {
	statement := fmt.Sprintf(`INSERT INTO pull_request_comments_versioned
		(sum256, versions, %s)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14,
			$15, $16, $17, $18, $19, $20, $21, $22)
		ON CONFLICT (sum256)
		DO UPDATE
		SET versions = array_append(pull_request_comments_versioned.versions, $23)`,
		pullRequestReviewCommentsCols)

	st := fmt.Sprintf("%v %v %v %+v", repositoryOwner, repositoryName, pullRequestNumber, comment)
	hash := sha256.Sum256([]byte(st))
	hashString := fmt.Sprintf("%x", hash)

	_, err := s.tx.Exec(statement,
		hashString,
		pq.Array([]int{s.v}),

		"",             // author_association text,
		comment.Text,   // body text,
		comment.ToHash, // commit_id text,
		time.Unix(int64(comment.CreatedDate/1000), 0), // created_at timestamptz,
		// FIXME possible to calculate
		"", // diff_hunk text,
		// possible to calculate like, example url:
		// http://localhost:7990/projects/MY/repos/go-git/pull-requests/1/overview?commentId=2
		"",         // htmlurl text,
		comment.ID, // id bigint,
		// TODO
		0,                          // in_reply_to bigint,
		"",                         // node_id text,
		comment.FromHash,           // original_commit_id text,
		0,                          // original_position bigint,
		comment.CommentAnchor.Path, // path text,
		comment.CommentAnchor.Line, // position bigint,
		pullRequestNumber,          // pull_request_number bigint NOT NULL,
		0,                          // pull_request_review_id bigint,
		repositoryName,             // repository_name text NOT NULL,
		repositoryOwner,            // repository_owner text NOT NULL,
		nil,                        // updated_at timestamptz,
		comment.Author.ID,          // user_id bigint NOT NULL,
		comment.Author.Slug,        // user_login text NOT NULL,

		s.v,
	)

	if err != nil {
		return fmt.Errorf("savePullRequestReviewComment: %v", err)
	}
	return nil
}
