BEGIN;

CREATE TABLE IF NOT EXISTS organizations_versioned (
  sum256 character varying(64) PRIMARY KEY,
  versions text ARRAY,

  avatar_url text,
  billing_email text,
  collaborators bigint,
  created_at timestamptz,
  description text,
  email text,
  htmlurl text,
  id bigint,
  location text,
  login text,
  name text,
  node_id text,
  owned_private_repos bigint,
  public_repos bigint,
  total_private_repos bigint,
  two_factor_requirement_enabled boolean,
  updated_at timestamptz
);

CREATE INDEX IF NOT EXISTS organizations_versions ON organizations_versioned (versions);

CREATE TABLE IF NOT EXISTS users_versioned (
  sum256 character varying(64) PRIMARY KEY,
  versions text ARRAY,

  avatar_url text,
  bio text,
  company text,
  created_at timestamptz,
  email text,
  followers bigint,
  following bigint,
  hireable boolean,
  htmlurl text,
  id bigint,
  location text,
  login text,
  name text,
  node_id text,
  owned_private_repos bigint,
  private_gists bigint,
  public_gists bigint,
  public_repos bigint,
  site_admin boolean,
  total_private_repos bigint,
  updated_at timestamptz
);

CREATE INDEX IF NOT EXISTS users_versions ON users_versioned (versions);

CREATE TABLE IF NOT EXISTS repositories_versioned (
  sum256 character varying(64) PRIMARY KEY,
  versions text ARRAY,
  
  allow_merge_commit boolean,
  allow_rebase_merge boolean,
  allow_squash_merge boolean,
  archived boolean,
  clone_url text,
  created_at timestamptz,
  default_branch text,
  description text,
  disabled boolean,
  fork boolean,
  forks_count bigint,
  full_name text,
  has_issues boolean,
  has_wiki boolean,
  homepage text,
  htmlurl text,
  id bigint,
  language text,
  mirror_url text,
  name text,
  node_id text,
  open_issues_count bigint,
  owner_id bigint NOT NULL,
  owner_login text NOT NULL,
  owner_type text NOT NULL,
  private boolean,
  pushed_at timestamptz,
  sshurl text,
  stargazers_count bigint,
  topics text[] NOT NULL,
  updated_at timestamptz,
  watchers_count bigint
);

CREATE INDEX IF NOT EXISTS repositories_versions ON repositories_versioned (versions);

CREATE TABLE IF NOT EXISTS issues_versioned (
  sum256 character varying(64) PRIMARY KEY,
  versions text ARRAY,

  assignees text[] NOT NULL,
  body text,
  closed_at timestamptz,
  closed_by_id bigint NOT NULL,
  closed_by_login text NOT NULL,
  comments bigint,
  created_at timestamptz,
  htmlurl text,
  id bigint,
  labels text[] NOT NULL,
  locked boolean,
  milestone_id text NOT NULL,
  milestone_title text NOT NULL,
  node_id text,
  number bigint,
  repository_name text NOT NULL,
  repository_owner text NOT NULL,
  state text,
  title text,
  updated_at timestamptz,
  user_id bigint NOT NULL,
  user_login text NOT NULL
);

CREATE INDEX IF NOT EXISTS issues_versions ON issues_versioned (versions);

CREATE TABLE IF NOT EXISTS issue_comments_versioned (
  sum256 character varying(64) PRIMARY KEY,
  versions text ARRAY,

  author_association text,
  body text,
  created_at timestamptz,
  htmlurl text,
  id bigint,
  issue_number bigint NOT NULL,
  node_id text,
  repository_name text NOT NULL,
  repository_owner text NOT NULL,
  updated_at timestamptz,
  user_id bigint NOT NULL,
  user_login text NOT NULL
);

CREATE INDEX IF NOT EXISTS issue_comments_versions ON issue_comments_versioned (versions);

CREATE TABLE IF NOT EXISTS pull_requests_versioned (
  sum256 character varying(64) PRIMARY KEY,
  versions text ARRAY,

  additions bigint,
  assignees text[] NOT NULL,
  author_association text,
  base_ref text NOT NULL,
  base_repository_name text NOT NULL,
  base_repository_owner text NOT NULL,
  base_sha text NOT NULL,
  base_user text NOT NULL,
  body text,
  changed_files bigint,
  closed_at timestamptz,
  comments bigint,
  commits bigint,
  created_at timestamptz,
  deletions bigint,
  head_ref text NOT NULL,
  head_repository_name text NOT NULL,
  head_repository_owner text NOT NULL,
  head_sha text NOT NULL,
  head_user text NOT NULL,
  htmlurl text,
  id bigint,
  labels text[] NOT NULL,
  maintainer_can_modify boolean,
  merge_commit_sha text,
  mergeable boolean,
  merged boolean,
  merged_at timestamptz,
  merged_by_id bigint NOT NULL,
  merged_by_login text NOT NULL,
  milestone_id text NOT NULL,
  milestone_title text NOT NULL,
  node_id text,
  number bigint,
  repository_name text NOT NULL,
  repository_owner text NOT NULL,
  review_comments bigint,
  state text,
  title text,
  updated_at timestamptz,
  user_id bigint NOT NULL,
  user_login text NOT NULL
);

CREATE INDEX IF NOT EXISTS pull_requests_versions ON pull_requests_versioned (versions);

CREATE TABLE IF NOT EXISTS pull_request_reviews_versioned (
  sum256 character varying(64) PRIMARY KEY,
  versions text ARRAY,

  body text,
  commit_id text,
  htmlurl text,
  id bigint,
  node_id text,
  pull_request_number bigint NOT NULL,
  repository_name text NOT NULL,
  repository_owner text NOT NULL,
  state text,
  submitted_at timestamptz,
  user_id bigint NOT NULL,
  user_login text NOT NULL
);

CREATE INDEX IF NOT EXISTS pull_request_reviews_versions ON pull_request_reviews_versioned (versions);

/*
The name is used for compatiblity with ghsync, but pull_request_comments
does not store the IssueComment's of PullRequest's.
Instead it stores the PullRequestReviewComment, so a better name would be
pull_request_review_comments
*/
CREATE TABLE IF NOT EXISTS pull_request_comments_versioned (
  sum256 character varying(64) PRIMARY KEY,
  versions text ARRAY,

  author_association text,
  body text,
  commit_id text,
  created_at timestamptz,
  diff_hunk text,
  htmlurl text,
  id bigint,
  in_reply_to bigint,
  node_id text,
  original_commit_id text,
  original_position bigint,
  path text,
  position bigint,
  pull_request_number bigint NOT NULL,
  pull_request_review_id bigint,
  repository_name text NOT NULL,
  repository_owner text NOT NULL,
  updated_at timestamptz,
  user_id bigint NOT NULL,
  user_login text NOT NULL
);

CREATE INDEX IF NOT EXISTS pull_request_comments_versions ON pull_request_comments_versioned (versions);

COMMIT;
