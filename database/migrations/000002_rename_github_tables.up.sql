BEGIN;

-- drop views to be able to create materialized views
DROP VIEW IF EXISTS organizations;
DROP VIEW IF EXISTS users;
DROP VIEW IF EXISTS repositories;
DROP VIEW IF EXISTS issues;
DROP VIEW IF EXISTS issue_comments;
DROP VIEW IF EXISTS pull_requests;
DROP VIEW IF EXISTS pull_request_reviews;
DROP VIEW IF EXISTS pull_request_comments;

-- rename tables to specify provider
ALTER TABLE organizations_versioned RENAME TO github_organizations_versioned;
ALTER TABLE users_versioned RENAME TO github_users_versioned;
ALTER TABLE repositories_versioned RENAME TO github_repositories_versioned;
ALTER TABLE issues_versioned RENAME TO github_issues_versioned;
ALTER TABLE issue_comments_versioned RENAME TO github_issue_comments_versioned;
ALTER TABLE pull_requests_versioned RENAME TO github_pull_requests_versioned;
ALTER TABLE pull_request_reviews_versioned RENAME TO github_pull_request_reviews_versioned;
ALTER TABLE pull_request_comments_versioned RENAME TO github_pull_request_comments_versioned;

COMMIT;
