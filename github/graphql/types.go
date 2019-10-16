package graphql

import "time"

// Connection represents common fields for paginated the connections
type Connection struct {
	PageInfo   PageInfo
	TotalCount int
}

func (c Connection) GetPageInfo() PageInfo {
	return c.PageInfo
}

func (c Connection) GetTotalCount() int {
	return c.TotalCount
}

// PageInfo represents https://developer.github.com/v4/object/pageinfo/
type PageInfo struct {
	HasNextPage bool
	EndCursor   string
}

// Organization represents https://developer.github.com/v4/object/organization/
type Organization struct {
	OrganizationFields
	MembersWithRole OrganizationMemberConnection `graphql:"membersWithRole(first: $membersWithRolePage, after: $membersWithRoleCursor)"`
} // `graphql:"organization(login: $organizationLogin)"`

// OrganizationFields defines the fields for Organization
// https://developer.github.com/v4/object/organization/
type OrganizationFields struct {
	AvatarURL         string    // avatar_url text,
	CreatedAt         time.Time // created_at timestamptz,
	Description       string    // description text,
	Email             string    // email text,
	URL               string    // htmlurl text,
	DatabaseID        int       // id bigint,
	Login             string    // login text,
	Name              string    // name text,
	ID                string    // node_id text,
	OwnedPrivateRepos struct {
		TotalCount int // owned_private_repos bigint,
	} `graphql:"owned_private_repos: repositories(privacy:PRIVATE, ownerAffiliations:OWNER)"`
	PublicRepos struct {
		TotalCount int // public_repos bigint,
	} `graphql:"public_repos: repositories(privacy:PUBLIC)"`
	TotalPrivateRepos struct {
		TotalCount int // total_private_repos bigint,
	} `graphql:"total_private_repos: repositories(privacy:PRIVATE)"`
	UpdatedAt string // updated_at timestamptz,
}

// OrganizationMemberConnection represents https://developer.github.com/v4/object/organizationmemberconnection/
type OrganizationMemberConnection struct {
	Connection
	Nodes []UserExtended
} // `graphql:"membersWithRole(first: $membersWithRolePage, after: $membersWithRoleCursor)"`

func (c OrganizationMemberConnection) Len() int { return len(c.Nodes) }

// UserExtended is the same type as User, but requesting more fields.
// Represents https://developer.github.com/v4/object/user/
type UserExtended struct {
	AvatarURL string    // avatar_url text,
	Bio       string    // bio text,
	Company   string    // company text,
	CreatedAt time.Time // created_at timestamptz,
	// TODO requires ['user:email', 'read:user'] scopes
	//Email     string // email text,
	Followers struct {
		TotalCount int // followers bigint,
	}
	Following struct {
		TotalCount int // following bigint,
	}
	IsHireable        bool   // hireable boolean,
	URL               string // htmlurl text,
	DatabaseID        int    // id bigint,
	Location          string // location text,
	Login             string // login text,
	Name              string // name text,
	ID                string // node_id text,
	OwnedPrivateRepos struct {
		TotalCount int // owned_private_repos bigint,
	} `graphql:"owned_private_repos: repositories(privacy:PRIVATE, ownerAffiliations:OWNER)"`
	/*
		TODO: call returns: You don't have permission to see gists.
			PrivateGists struct {
				TotalCount int // private_gists bigint,
			} `graphql:"private_gists: gists(privacy:SECRET)"`
			PublicGists struct {
				TotalCount int // public_gists bigint,
			} `graphql:"public_gists: gists(privacy:PUBLIC)"`
	*/
	PublicRepos struct {
		TotalCount int // public_repos bigint,
	} `graphql:"public_repos: repositories(privacy:PUBLIC)"`
	TotalPrivateRepos struct {
		TotalCount int // total_private_repos bigint,
	} `graphql:"total_private_repos: repositories(privacy:PRIVATE)"`
	UpdatedAt time.Time // updated_at timestamptz,
}

// Repository represents https://developer.github.com/v4/object/repository/
type Repository struct {
	RepositoryFields
	RepositoryTopics RepositoryTopicsConnection `graphql:"repositoryTopics(first: $repositoryTopicsPage, after: $repositoryTopicsCursor)"`
	Issues           IssueConnection            `graphql:"issues(first: $issuesPage, after: $issuesCursor)"`
	PullRequests     PullRequestConnection      `graphql:"pullRequests(first: $pullRequestsPage, after: $pullRequestsCursor)"`
} // `graphql:"repository(owner: $owner, name: $name)"`

// RepositoryFields defines the fields for Repository
// https://developer.github.com/v4/object/repository/
type RepositoryFields struct {
	MergeCommitAllowed bool      // allow_merge_commit boolean
	RebaseMergeAllowed bool      // allow_rebase_merge boolean
	SquashMergeAllowed bool      // allow_squash_merge boolean
	IsArchived         bool      // archived boolean
	CreatedAt          time.Time // created_at timestamptz
	DefaultBranchRef   struct {
		Name string // default_branch text
	}
	Description      string // description text
	IsDisabled       bool   // disabled boolean
	IsFork           bool   // fork boolean
	ForkCount        int    // forks_count bigint
	NameWithOwner    string // full_name text
	HasIssuesEnabled bool   // has_issues boolean
	HasWikiEnabled   bool   // has_wiki boolean
	HomepageURL      string // homepage text
	URL              string // htmlurl text
	DatabaseID       int    // id bigint,
	PrimaryLanguage  struct {
		Name string // language text
	}
	Name       string // name text
	ID         string // node_id text
	OpenIssues struct {
		TotalCount int // open_issues_count bigint
	} `graphql:"openIssues: issues(states:[OPEN])"`
	Owner struct {
		Organization struct {
			DatabaseID int // owner_id bigint NOT NULL,
		} `graphql:"... on Organization"`
		User struct {
			DatabaseID int // owner_id bigint NOT NULL,
		} `graphql:"... on User"`
		Login    string // owner_login text NOT NULL,
		Typename string `graphql:"__typename"` // owner_type text NOT NULL
	}
	IsPrivate  bool      // private boolean
	PushedAt   time.Time // pushed_at timestamptz
	SSHURL     string    // sshurl text
	Stargazers struct {
		TotalCount int // stargazers_count bigint
	}
	UpdatedAt time.Time // updated_at timestamptz
	Watchers  struct {
		TotalCount int // watchers_count bigint
	}
}

// RepositoryTopicsConnection represents https://developer.github.com/v4/object/repositorytopicconnection/
type RepositoryTopicsConnection struct {
	Connection
	Nodes []struct {
		Topic struct {
			Name string
		}
	}
} //`graphql:"repositoryTopics(first: $repositoryTopicsPage, after: $repositoryTopicsCursor)"`

func (c RepositoryTopicsConnection) Len() int { return len(c.Nodes) }

// IssueConnection represents https://developer.github.com/v4/object/issueconnection/
type IssueConnection struct {
	Connection
	Nodes []Issue
} //`graphql:"issues(first: $issuesPage, after: $issuesCursor)"`

func (c IssueConnection) Len() int { return len(c.Nodes) }

type IssueCommentsConnection struct {
	Connection
	Nodes []IssueComment
} // `graphql:"comments(first: $issueCommentsPage, after: $issueCommentsCursor)"`

func (c IssueCommentsConnection) Len() int { return len(c.Nodes) }

// Issue represents https://developer.github.com/v4/object/issue/
type Issue struct {
	IssueFields
	Assignees UserConnection          `graphql:"assignees(first: $assigneesPage, after: $assigneesCursor)"`
	Labels    LabelConnection         `graphql:"labels(first: $labelsPage, after: $labelsCursor)"`
	Comments  IssueCommentsConnection `graphql:"comments(first: $issueCommentsPage, after: $issueCommentsCursor)"`
	ClosedBy  ClosedByConnection      `graphql:"timelineItems(last:1, itemTypes:CLOSED_EVENT)"`
} // `graphql:"issue(number: $issueNumber)"`

// User represents https://developer.github.com/v4/object/user/
type User struct {
	DatabaseID int
	ID         string
	Login      string
}

type Actor struct {
	Login    string
	Typename string `graphql:"__typename"`
	User     `graphql:"... on User"`
}

type IssueFields struct {
	Body       string    // body text,
	ClosedAt   time.Time // closed_at timestamptz,
	CreatedAt  time.Time // created_at timestamptz,
	URL        string    // htmlurl text,
	DatabaseID int       // id bigint,
	Locked     bool      // locked boolean,
	Milestone  struct {
		ID    string // milestone_id text NOT NULL,
		Title string // milestone_title text NOT NULL,
	}
	ID        string    // node_id text,
	Number    int       // number bigint,
	State     string    // state text,
	Title     string    // title text,
	UpdatedAt time.Time // updated_at timestamptz,
	Author    Actor     // user_id bigint NOT NULL, user_login text NOT NULL,
}

type ClosedByConnection struct {
	Nodes []struct {
		ClosedEvent struct {
			Actor Actor // closed_by_id bigint NOT NULL, closed_by_login text NOT NULL,
		} `graphql:"... on ClosedEvent"`
	}
} // `graphql:"timelineItems(last:1, itemTypes:CLOSED_EVENT)"`

// UserConnection represents https://developer.github.com/v4/object/userconnection/
type UserConnection struct {
	Connection
	Nodes []User
} //`graphql:"assignees(first: $assigneesPage, after: $assigneesCursor)"`

func (c UserConnection) Len() int { return len(c.Nodes) }

// Label represents https://developer.github.com/v4/object/label/
type Label struct {
	Name string
}

// LabelConnection represents https://developer.github.com/v4/object/labelconnection/
type LabelConnection struct {
	Connection
	Nodes []Label
} //`graphql:"labels(first: $labelsPage, after: $labelsCursor)"`

func (c LabelConnection) Len() int { return len(c.Nodes) }

type IssueComment struct {
	AuthorAssociation string    // author_association text,
	Body              string    // body text,
	CreatedAt         time.Time // created_at timestamptz,
	URL               string    // htmlurl text,
	DatabaseID        int       // id bigint,
	ID                string    // node_id text,
	UpdatedAt         string    // updated_at timestamptz,
	Author            Actor     // user_id bigint NOT NULL, user_login text NOT NULL,
}

type PullRequestConnection struct {
	Connection
	Nodes []PullRequest
} //`graphql:"pullRequests(first: $pullRequestsPage, after: $pullRequestsCursor)"`

func (c PullRequestConnection) Len() int { return len(c.Nodes) }

type PullRequest struct {
	PullRequestFields
	Assignees UserConnection              `graphql:"assignees(first: $assigneesPage, after: $assigneesCursor)"`
	Labels    LabelConnection             `graphql:"labels(first: $labelsPage, after: $labelsCursor)"`
	Comments  IssueCommentsConnection     `graphql:"comments(first: $issueCommentsPage, after: $issueCommentsCursor)"`
	Reviews   PullRequestReviewConnection `graphql:"reviews(first: $pullRequestReviewsPage, after: $pullRequestReviewsCursor)"`
} // `graphql:"pullRequest(number: $prNumber)"`

type Ref struct {
	Name       string // _ref text
	Repository struct {
		Name  string // _repository_name text
		Owner struct {
			Login string // _repository_owner text
		}
	}
	Target struct {
		Oid    string //_sha
		Commit struct {
			Author struct {
				User struct {
					Login string // _user
				}
			}
		} `graphql:"... on Commit"`
	}
}

type PullRequestFields struct {
	Additions         int       // additions bigint,
	AuthorAssociation string    // author_association text,
	BaseRef           Ref       // base_*
	Body              string    // body text,
	ChangedFiles      int       // changed_files bigint,
	ClosedAt          time.Time // closed_at timestamptz,
	Commits           struct {
		TotalCount int // commits bigint,
	}
	CreatedAt           time.Time // created_at timestamptz,
	Deletions           int       // deletions bigint,
	HeadRef             Ref       // head_*
	URL                 string    // htmlurl text,
	DatabaseID          int       // id bigint,
	MaintainerCanModify bool      // maintainer_can_modify boolean,
	MergeCommit         struct {
		Oid string // merge_commit_sha text,
	}
	Mergeable string    // mergeable boolean,
	Merged    bool      // merged boolean,
	MergedAt  time.Time // merged_at timestamptz,
	MergedBy  Actor     // merged_by_id bigint NOT NULL, merged_by_login text NOT NULL,
	Milestone struct {
		ID    string // milestone_id text NOT NULL,
		Title string // milestone_title text NOT NULL,
	}
	ID            string // node_id text,
	Number        int    // number bigint,
	ReviewThreads struct {
		TotalCount int // review_comments bigint,
	}
	State     string // state text,
	Title     string // title text,
	UpdatedAt string // updated_at timestamptz,
	Author    Actor  // user_id bigint NOT NULL, user_login text NOT NULL,
}

type PullRequestReviewConnection struct {
	Connection
	Nodes []PullRequestReview
} // `graphql:"reviews(first: $pullRequestReviewsPage, after: $pullRequestReviewsCursor)"`

func (c PullRequestReviewConnection) Len() int { return len(c.Nodes) }

type PullRequestReview struct {
	PullRequestReviewFields
	Comments PullRequestReviewCommentConnection `graphql:"comments(first: $pullRequestReviewCommentsPage, after: $pullRequestReviewCommentsCursor)"`
}

type PullRequestReviewFields struct {
	Body   string // body text,
	Commit struct {
		Oid string // commit_id text,
	}
	URL         string    // htmlurl text,
	DatabaseID  int       // id bigint,
	ID          string    // node_id text,
	State       string    // state text,
	SubmittedAt time.Time // submitted_at timestamptz,
	Author      Actor     // user_id bigint NOT NULL, user_login text NOT NULL,

	Comments PullRequestReviewCommentConnection `graphql:"comments(first: $pullRequestReviewCommentsPage, after: $pullRequestReviewCommentsCursor)"`
}

type PullRequestReviewCommentConnection struct {
	Connection
	Nodes []PullRequestReviewComment
}

func (c PullRequestReviewCommentConnection) Len() int { return len(c.Nodes) }

type PullRequestReviewComment struct {
	AuthorAssociation string // author_association text,
	Body              string // body text,
	Commit            struct {
		Oid string // commit_id text,
	}
	CreatedAt  time.Time // created_at timestamptz,
	DiffHunk   string    // diff_hunk text,
	URL        string    // htmlurl text,
	DatabaseID int       // id bigint,
	//in_reply_to            string    // in_reply_to bigint,
	ID             string // node_id text,
	OriginalCommit struct {
		Oid string // original_commit_id text,
	}
	OriginalPosition int       // original_position bigint,
	Path             string    // path text,
	Position         int       // position bigint,
	UpdatedAt        time.Time // updated_at timestamptz,
	Author           Actor     // user_id bigint NOT NULL, user_login text NOT NULL,
}
