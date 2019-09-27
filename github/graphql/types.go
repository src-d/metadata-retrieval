package graphql

import "time"

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
	AvatarUrl string // avatar_url text,
	// TODO: requires admin:org scope
	//OrganizationBillingEmail string    // billing_email text,
	CreatedAt         time.Time // created_at timestamptz,
	Description       string    // description text,
	Email             string    // email text,
	Url               string    // htmlurl text,
	DatabaseId        int       // id bigint,
	Location          string    // location text,
	Login             string    // login text,
	Name              string    // name text,
	Id                string    // node_id text,
	OwnedPrivateRepos struct {
		TotalCount int // owned_private_repos bigint,
	} `graphql:"owned_private_repos: repositories(privacy:PRIVATE, ownerAffiliations:OWNER)"`
	PublicRepos struct {
		TotalCount int // public_repos bigint,
	} `graphql:"public_repos: repositories(privacy:PUBLIC)"`
	TotalPrivateRepos struct {
		TotalCount int // total_private_repos bigint,
	} `graphql:"total_private_repos: repositories(privacy:PRIVATE)"`
	// TODO: requires admin:org scope
	//RequiresTwoFactorAuthentication bool   // two_factor_requirement_enabled boolean,
	UpdatedAt string // updated_at timestamptz,
}

// OrganizationMemberConnection represents https://developer.github.com/v4/object/organizationmemberconnection/
type OrganizationMemberConnection struct {
	TotalCount int
	PageInfo   PageInfo
	Nodes      []UserExtended
} // `graphql:"membersWithRole(first: $membersWithRolePage, after: $membersWithRoleCursor)"`

// UserExtended is the same type as User, but requesting more fields.
// Represents https://developer.github.com/v4/object/user/
type UserExtended struct {
	AvatarUrl string    // avatar_url text,
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
	Url               string // htmlurl text,
	DatabaseId        int    // id bigint,
	Location          string // location text,
	Login             string // login text,
	Name              string // name text,
	Id                string // node_id text,
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
	IsSiteAdmin       bool // site_admin boolean,
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
	Url                string    // clone_url text
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
	HomepageUrl      string // homepage text
	//Url              string // htmlurl text
	DatabaseId      int // id bigint,
	PrimaryLanguage struct {
		Name string // language text
	}
	MirrorUrl  string // mirror_url text
	Name       string // name text
	Id         string // node_id text
	OpenIssues struct {
		TotalCount int // open_issues_count bigint
	} `graphql:"openIssues: issues(states:[OPEN])"`
	Owner struct {
		Organization struct {
			DatabaseId int // owner_id bigint NOT NULL,
		} `graphql:"... on Organization"`
		User struct {
			DatabaseId int // owner_id bigint NOT NULL,
		} `graphql:"... on User"`
		Login    string // owner_login text NOT NULL,
		Typename string `graphql:"__typename"` // owner_type text NOT NULL
	}

	IsPrivate  bool      // private boolean
	PushedAt   time.Time // pushed_at timestamptz
	SshUrl     string    // sshurl text
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
	PageInfo PageInfo
	Nodes    []struct {
		Topic struct {
			Name string
		}
	}
} //`graphql:"repositoryTopics(first: $repositoryTopicsPage, after: $repositoryTopicsCursor)"`

// IssueConnection represents https://developer.github.com/v4/object/issueconnection/
type IssueConnection struct {
	PageInfo PageInfo
	Nodes    []Issue
} //`graphql:"issues(first: $issuesPage, after: $issuesCursor)"`

type IssueCommentsConnection struct {
	TotalCount int
	PageInfo   PageInfo
	Nodes      []IssueComment
} // `graphql:"comments(first: $issueCommentsPage, after: $issueCommentsCursor)"`

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
	DatabaseId int
	Id         string
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
	Url        string    // htmlurl text,
	DatabaseId int       // id bigint,
	Locked     bool      // locked boolean,
	Milestone  struct {
		Id    string // milestone_id text NOT NULL,
		Title string // milestone_title text NOT NULL,
	}
	Id        string    // node_id text,
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
	PageInfo PageInfo
	Nodes    []User
} //`graphql:"assignees(first: $assigneesPage, after: $assigneesCursor)"`

// Label represents https://developer.github.com/v4/object/label/
type Label struct {
	Name string
}

// LabelConnection represents https://developer.github.com/v4/object/labelconnection/
type LabelConnection struct {
	PageInfo PageInfo
	Nodes    []Label
} //`graphql:"labels(first: $labelsPage, after: $labelsCursor)"`

type IssueComment struct {
	AuthorAssociation string    // author_association text,
	Body              string    // body text,
	CreatedAt         time.Time // created_at timestamptz,
	Url               string    // htmlurl text,
	DatabaseId        int       // id bigint,
	Id                string    // node_id text,
	UpdatedAt         string    // updated_at timestamptz,
	Author            Actor     // user_id bigint NOT NULL, user_login text NOT NULL,
}

type PullRequestConnection struct {
	PageInfo PageInfo
	Nodes    []PullRequest
} //`graphql:"pullRequests(first: $pullRequestsPage, after: $pullRequestsCursor)"`

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
	Url                 string    // htmlurl text,
	DatabaseId          int       // id bigint,
	MaintainerCanModify bool      // maintainer_can_modify boolean,
	MergeCommit         struct {
		Oid string // merge_commit_sha text,
	}
	Mergeable string    // mergeable boolean,
	Merged    bool      // merged boolean,
	MergedAt  time.Time // merged_at timestamptz,
	MergedBy  Actor     // merged_by_id bigint NOT NULL, merged_by_login text NOT NULL,
	Milestone struct {
		Id    string // milestone_id text NOT NULL,
		Title string // milestone_title text NOT NULL,
	}
	Id            string // node_id text,
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
	//TotalCount int
	PageInfo PageInfo
	Nodes    []PullRequestReview
} // `graphql:"reviews(first: $pullRequestReviewsPage, after: $pullRequestReviewsCursor)"`

type PullRequestReview struct {
	PullRequestReviewFields
	Comments PullRequestReviewCommentConnection `graphql:"comments(first: $pullRequestReviewCommentsPage, after: $pullRequestReviewCommentsCursor)"`
}

type PullRequestReviewFields struct {
	Body   string // body text,
	Commit struct {
		Oid string // commit_id text,
	}
	Url         string    // htmlurl text,
	DatabaseId  int       // id bigint,
	Id          string    // node_id text,
	State       string    // state text,
	SubmittedAt time.Time // submitted_at timestamptz,
	Author      Actor     // user_id bigint NOT NULL, user_login text NOT NULL,

	Comments PullRequestReviewCommentConnection `graphql:"comments(first: $pullRequestReviewCommentsPage, after: $pullRequestReviewCommentsCursor)"`
}

type PullRequestReviewCommentConnection struct {
	//TotalCount int
	PageInfo PageInfo
	Nodes    []PullRequestReviewComment
}

type PullRequestReviewComment struct {
	AuthorAssociation string // author_association text,
	Body              string // body text,
	Commit            struct {
		Oid string // commit_id text,
	}
	CreatedAt  time.Time // created_at timestamptz,
	DiffHunk   string    // diff_hunk text,
	Url        string    // htmlurl text,
	DatabaseId int       // id bigint,
	//in_reply_to            string    // in_reply_to bigint,
	Id             string // node_id text,
	OriginalCommit struct {
		Oid string // original_commit_id text,
	}
	OriginalPosition int       // original_position bigint,
	Path             string    // path text,
	Position         int       // position bigint,
	UpdatedAt        time.Time // updated_at timestamptz,
	Author           Actor     // user_id bigint NOT NULL, user_login text NOT NULL,
}
