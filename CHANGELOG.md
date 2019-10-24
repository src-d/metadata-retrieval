# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

The changes listed under `Unreleased` section have landed in master but are not yet released.


## [Unreleased]


## [v0.1.1](https://github.com/src-d/metadata-retrieval/releases/tag/v0.1.1) - 2019-10-24

### Fixed

- Missing values for nullable `timestamptz` where being inserted as `0001-01-01 00:00:00+00:00` instead of `null`. The affected fields where `issues_versioned.closed_at`, `pull_requests_versioned.closed_at` and `pull_requests_versioned.merged_at` ([#74](https://github.com/src-d/metadata-retrieval/issues/74)).
- Avoid retrying requests with bad credentials ([#23](https://github.com/src-d/metadata-retrieval/issues/23)).


## [v0.1.0](https://github.com/src-d/metadata-retrieval/releases/tag/v0.1.0) - 2019-10-23

Initial release for downloading metadata from git hostings. Available commands:

- Download organization
- Download repository
- Download an organization with all the repositories

Supported providers:

- Github
