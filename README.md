# metadata-retrieval

Current `examples/cmd/` contains an example of how to use the library, implementing a `ghsync` subcmd that mimics the `src-d/ghsync` deep subcmd.

The example cmd can print to sdtout or save to a postgres DB. To help even further with the development, use the options `--log-level=debug --log-http`.

To use, create a personal GitHub token with the scopes **read:org**, **repo**.

```shell
export GITHUB_TOKEN=xxxx

# Info for individual repositories
go run examples/cmd/*.go repo --version v0 --owner=src-d --name=metadata-retrieval

# Info for individual organization and its users (not including its repositories)
go run examples/cmd/*.go org --version v0 --name=src-d

# Info for organization and all its repositories (similar to ghsync deep)
go run examples/cmd/*.go ghsync --version v0 --name=src-d --no-forks
```

To use a postgres DB:

```shell
docker-compose up -d

go run examples/cmd/*.go repo --version v0 --owner=src-d --name=metadata-retrieval --db=postgres://user:password@127.0.0.1:5432/ghsync?sslmode=disable

docker-compose exec postgres psql postgres://user:password@127.0.0.1:5432/ghsync?sslmode=disable -c "select * from pull_request_reviews"
```

The file [doc/1560510971_initial_schema.up.sql](./doc/1560510971_initial_schema.up.sql) contains the src-d/ghsync schema file at v0.2.0 ([link](https://github.com/src-d/ghsync/blob/v0.2.0/models/sql/1560510971_initial_schema.up.sql)). The schema is the same, but the tables and columns have been reordered and reformatted.

You can see the diff between the current DB schema and the ghsync schema here:

<details><summary>diff</summary>

```diff
--- doc/1560510971_initial_schema.up.sql	2019-09-25 11:15:06.037702240 +0100
+++ database/migrations/000001_init.up.sql	2019-09-25 12:01:21.829358293 +0100
@@ -1,267 +1,251 @@
 BEGIN;
 
-CREATE TABLE organizations (
-  kallax_id serial NOT NULL PRIMARY KEY,
+CREATE TABLE IF NOT EXISTS organizations_versioned (
+  sum256 character varying(64) PRIMARY KEY,
+  versions text ARRAY,
 
   avatar_url text,
   billing_email text,
-  blog text,
   collaborators bigint,
-  company text,
   created_at timestamptz,
   description text,
-  disk_usage bigint,
   email text,
-  followers bigint,
-  following bigint,
   htmlurl text,
   id bigint,
   location text,
   login text,
   name text,
   node_id text,
   owned_private_repos bigint,
-  private_gists bigint,
-  public_gists bigint,
   public_repos bigint,
   total_private_repos bigint,
   two_factor_requirement_enabled boolean,
-  type text,
   updated_at timestamptz
 );
 
-CREATE TABLE users (
-  kallax_id serial NOT NULL PRIMARY KEY,
+CREATE INDEX IF NOT EXISTS organizations_versions ON organizations_versioned (versions);
+
+CREATE TABLE IF NOT EXISTS users_versioned (
+  sum256 character varying(64) PRIMARY KEY,
+  versions text ARRAY,
 
   avatar_url text,
   bio text,
-  blog text,
-  collaborators bigint,
   company text,
   created_at timestamptz,
-  disk_usage bigint,
   email text,
   followers bigint,
   following bigint,
-  gravatar_id text,
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
-  suspended_at timestamptz,
   total_private_repos bigint,
-  two_factor_authentication boolean,
-  type text,
   updated_at timestamptz
 );
 
-CREATE TABLE repositories (
-  kallax_id serial NOT NULL PRIMARY KEY,
+CREATE INDEX IF NOT EXISTS users_versions ON users_versioned (versions);
 
+CREATE TABLE IF NOT EXISTS repositories_versioned (
+  sum256 character varying(64) PRIMARY KEY,
+  versions text ARRAY,
+  
   allow_merge_commit boolean,
   allow_rebase_merge boolean,
   allow_squash_merge boolean,
   archived boolean,
-  auto_init boolean,
   clone_url text,
-  code_of_conduct jsonb,
   created_at timestamptz,
   default_branch text,
   description text,
   disabled boolean,
   fork boolean,
   forks_count bigint,
   full_name text,
-  git_url text,
-  gitignore_template text,
-  has_downloads boolean,
   has_issues boolean,
-  has_pages boolean,
-  has_projects boolean,
   has_wiki boolean,
   homepage text,
   htmlurl text,
   id bigint,
   language text,
-  license jsonb,
-  license_template text,
-  master_branch text,
   mirror_url text,
   name text,
-  network_count bigint,
   node_id text,
   open_issues_count bigint,
-  organization_id bigint NOT NULL,
-  organization_name text NOT NULL,
   owner_id bigint NOT NULL,
   owner_login text NOT NULL,
   owner_type text NOT NULL,
-  parent jsonb,
-  permissions jsonb,
   private boolean,
   pushed_at timestamptz,
-  size bigint,
-  source jsonb,
   sshurl text,
   stargazers_count bigint,
-  subscribers_count bigint,
-  svnurl text,
-  team_id bigint,
   topics text[] NOT NULL,
   updated_at timestamptz,
   watchers_count bigint
 );
 
-CREATE TABLE issues (
-  kallax_id serial NOT NULL PRIMARY KEY,
+CREATE INDEX IF NOT EXISTS repositories_versions ON repositories_versioned (versions);
 
-  assignee_id bigint NOT NULL,
-  assignee_login text NOT NULL,
-  assignees jsonb NOT NULL,
+CREATE TABLE IF NOT EXISTS issues_versioned (
+  sum256 character varying(64) PRIMARY KEY,
+  versions text ARRAY,
+
+  assignees text[] NOT NULL,
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
-  milestone_id bigint NOT NULL,
+  milestone_id text NOT NULL,
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
 
-CREATE TABLE issue_comments (
-  kallax_id serial NOT NULL PRIMARY KEY,
+CREATE INDEX IF NOT EXISTS issues_versions ON issues_versioned (versions);
+
+CREATE TABLE IF NOT EXISTS issue_comments_versioned (
+  sum256 character varying(64) PRIMARY KEY,
+  versions text ARRAY,
 
   author_association text,
   body text,
   created_at timestamptz,
   htmlurl text,
   id bigint,
   issue_number bigint NOT NULL,
   node_id text,
-  reactions jsonb,
   repository_name text NOT NULL,
   repository_owner text NOT NULL,
   updated_at timestamptz,
   user_id bigint NOT NULL,
   user_login text NOT NULL
 );
 
-CREATE TABLE pull_requests (
-  kallax_id serial NOT NULL PRIMARY KEY,
+CREATE INDEX IF NOT EXISTS issue_comments_versions ON issue_comments_versioned (versions);
+
+CREATE TABLE IF NOT EXISTS pull_requests_versioned (
+  sum256 character varying(64) PRIMARY KEY,
+  versions text ARRAY,
 
   additions bigint,
-  assignee_id bigint NOT NULL,
-  assignee_login text NOT NULL,
-  assignees jsonb NOT NULL,
+  assignees text[] NOT NULL,
   author_association text,
-  base_label text NOT NULL,
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
-  draft boolean,
-  head_label text NOT NULL,
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
-  mergeable_state text,
   merged boolean,
   merged_at timestamptz,
   merged_by_id bigint NOT NULL,
   merged_by_login text NOT NULL,
-  milestone_id bigint NOT NULL,
+  milestone_id text NOT NULL,
   milestone_title text NOT NULL,
   node_id text,
   number bigint,
   repository_name text NOT NULL,
   repository_owner text NOT NULL,
-  requested_reviewers jsonb NOT NULL,
   review_comments bigint,
   state text,
   title text,
   updated_at timestamptz,
   user_id bigint NOT NULL,
   user_login text NOT NULL
 );
 
-CREATE TABLE pull_request_reviews (
-  kallax_id serial NOT NULL PRIMARY KEY,
+CREATE INDEX IF NOT EXISTS pull_requests_versions ON pull_requests_versioned (versions);
+
+CREATE TABLE IF NOT EXISTS pull_request_reviews_versioned (
+  sum256 character varying(64) PRIMARY KEY,
+  versions text ARRAY,
 
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
 
-CREATE TABLE pull_request_comments (
-  kallax_id serial NOT NULL PRIMARY KEY,
+CREATE INDEX IF NOT EXISTS pull_request_reviews_versions ON pull_request_reviews_versioned (versions);
+
+/*
+The name is used for compatiblity with ghsync, but pull_request_comments
+does not store the IssueComment's of PullRequest's.
+Instead it stores the PullRequestReviewComment, so a better name would be
+pull_request_review_comments
+*/
+CREATE TABLE IF NOT EXISTS pull_request_comments_versioned (
+  sum256 character varying(64) PRIMARY KEY,
+  versions text ARRAY,
 
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
-  reactions jsonb,
   repository_name text NOT NULL,
   repository_owner text NOT NULL,
   updated_at timestamptz,
   user_id bigint NOT NULL,
   user_login text NOT NULL
 );
 
+CREATE INDEX IF NOT EXISTS pull_request_comments_versions ON pull_request_comments_versioned (versions);
+
 COMMIT;
```

</details>
