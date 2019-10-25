package testutils

import "net/http"

// Different graphql queries
const (
	Endpoint            = "https://api.github.com/graphql"
	BasicMalformedQuery = `{ viewer { login }`
	BasicQuery          = `query { repository(owner:"octocat", name:"Hello-World") {
		  issues(last:20, states:CLOSED) {
			edges {
			  node {
				title
				url
				labels(first:5) {
				  edges {
					node {
					  name
					}
				  }
				}
			  }
			}
		  }
		}
	  }`
	BigQuery          = `query($assigneesCursor:String$assigneesPage:Int!$issueCommentsCursor:String$issueCommentsPage:Int!$issuesCursor:String$issuesPage:Int!$labelsCursor:String$labelsPage:Int!$name:String!$owner:String!$pullRequestReviewCommentsCursor:String$pullRequestReviewCommentsPage:Int!$pullRequestReviewsCursor:String$pullRequestReviewsPage:Int!$pullRequestsCursor:String$pullRequestsPage:Int!$repositoryTopicsCursor:String$repositoryTopicsPage:Int!){repository(owner: $owner, name: $name){mergeCommitAllowed,rebaseMergeAllowed,squashMergeAllowed,isArchived,url,createdAt,defaultBranchRef{name},description,isDisabled,isFork,forkCount,nameWithOwner,hasIssuesEnabled,hasWikiEnabled,homepageUrl,databaseId,primaryLanguage{name},mirrorUrl,name,id,openIssues: issues(states:[OPEN]){totalCount},owner{... on Organization{databaseId},... on User{databaseId},login,__typename},isPrivate,pushedAt,sshUrl,stargazers{totalCount},updatedAt,watchers{totalCount},repositoryTopics(first: $repositoryTopicsPage, after: $repositoryTopicsCursor){pageInfo{hasNextPage,endCursor},totalCount,nodes{topic{name}}},issues(first: $issuesPage, after: $issuesCursor){pageInfo{hasNextPage,endCursor},totalCount,nodes{body,closedAt,createdAt,url,databaseId,locked,milestone{id,title},id,number,state,title,updatedAt,author{login,__typename,... on User{databaseId,id,login}},assignees(first: $assigneesPage, after: $assigneesCursor){pageInfo{hasNextPage,endCursor},totalCount,nodes{databaseId,id,login}},labels(first: $labelsPage, after: $labelsCursor){pageInfo{hasNextPage,endCursor},totalCount,nodes{name}},comments(first: $issueCommentsPage, after: $issueCommentsCursor){pageInfo{hasNextPage,endCursor},totalCount,nodes{authorAssociation,body,createdAt,url,databaseId,id,updatedAt,author{login,__typename,... on User{databaseId,id,login}}}},timelineItems(last:1, itemTypes:CLOSED_EVENT){nodes{... on ClosedEvent{actor{login,__typename,... on User{databaseId,id,login}}}}}}},pullRequests(first: $pullRequestsPage, after: $pullRequestsCursor){pageInfo{hasNextPage,endCursor},totalCount,nodes{additions,authorAssociation,baseRef{name,repository{name,owner{login}},target{oid,... on Commit{author{user{login}}}}},body,changedFiles,closedAt,commits{totalCount},createdAt,deletions,headRef{name,repository{name,owner{login}},target{oid,... on Commit{author{user{login}}}}},url,databaseId,maintainerCanModify,mergeCommit{oid},mergeable,merged,mergedAt,mergedBy{login,__typename,... on User{databaseId,id,login}},milestone{id,title},id,number,reviewThreads{totalCount},state,title,updatedAt,author{login,__typename,... on User{databaseId,id,login}},assignees(first: $assigneesPage, after: $assigneesCursor){pageInfo{hasNextPage,endCursor},totalCount,nodes{databaseId,id,login}},labels(first: $labelsPage, after: $labelsCursor){pageInfo{hasNextPage,endCursor},totalCount,nodes{name}},comments(first: $issueCommentsPage, after: $issueCommentsCursor){pageInfo{hasNextPage,endCursor},totalCount,nodes{authorAssociation,body,createdAt,url,databaseId,id,updatedAt,author{login,__typename,... on User{databaseId,id,login}}}},reviews(first: $pullRequestReviewsPage, after: $pullRequestReviewsCursor){pageInfo{hasNextPage,endCursor},totalCount,nodes{body,commit{oid},url,databaseId,id,state,submittedAt,author{login,__typename,... on User{databaseId,id,login}},comments(first: $pullRequestReviewCommentsPage, after: $pullRequestReviewCommentsCursor){pageInfo{hasNextPage,endCursor},totalCount,nodes{authorAssociation,body,commit{oid},createdAt,diffHunk,url,databaseId,id,originalCommit{oid},originalPosition,path,position,updatedAt,author{login,__typename,... on User{databaseId,id,login}}}},comments(first: $pullRequestReviewCommentsPage, after: $pullRequestReviewCommentsCursor){pageInfo{hasNextPage,endCursor},totalCount,nodes{authorAssociation,body,commit{oid},createdAt,diffHunk,url,databaseId,id,originalCommit{oid},originalPosition,path,position,updatedAt,author{login,__typename,... on User{databaseId,id,login}}}}}}}}}}`
	BigQueryVariables = `{"assigneesCursor": null, "assigneesPage": 2, "issueCommentsCursor": null, "issueCommentsPage": 50, "issuesCursor": null, "issuesPage": 50, "labelsCursor": null, "labelsPage": 2, "name": "go-git", "owner": "src-d", "pullRequestReviewCommentsCursor": null, "pullRequestReviewCommentsPage": 50, "pullRequestReviewsCursor": null, "pullRequestReviewsPage": 20, "pullRequestsCursor": null, "pullRequestsPage": 50, "repositoryTopicsCursor": null, "repositoryTopicsPage": 10}`
	ReallyBigQuery    = `query {
		organization(login:"src-d") {
		  name
		  repositories(first: 100) {
			edges {
			  node {
				id
				issues(first: 100) {
				  edges {
					node {
					  id
					  comments(first: 30) {
						edges {
						  node {
							id
						  }
						}
					  }
					}
				  }
				}
			  }
			}
		  }
		}
	  }`
	ReallyReallyBigQuery = `query {
		organization(login:"src-d") {
		  name
		  repositories(first: 100) {
			edges {
			  node {
				id
				issues(first: 100) {
				  edges {
					node {
					  id
					  comments(first: 50) {
						edges {
						  node {
							id
						  }
						}
					  }
					}
				  }
				}
			  }
			}
		  }
		}
	  }`
)

// RepositoryTestOracle struct to hold a test oracle for a repository
type RepositoryTestOracle struct {
	Owner                 string   `json:"owner"`
	Repository            string   `json:"repository"`
	Version               int      `json:"version"`
	URL                   string   `json:"url"`
	Topics                []string `json:"topics"`
	CreatedAt             string   `json:"createdAt"`
	IsPrivate             bool     `json:"isPrivate"`
	IsArchived            bool     `json:"isArchived"`
	HasWiki               bool     `json:"hasWiki"`
	NumOfPRs              int      `json:"numOfPrs"`
	NumOfPRComments       int      `json:"numOfPrComments"`
	NumOfIssues           int      `json:"numOfIssues"`
	NumOfIssueComments    int      `json:"numOfIssueComments"`
	NumOfPRReviews        int      `json:"numOfPRReviews"`
	NumOfPRReviewComments int      `json:"numOfPRReviewComments"`
}

// OrganizationTestOracle struct to hold a test oracle for an organization
type OrganizationTestOracle struct {
	Org               string `json:"org"`
	Version           int    `json:"version"`
	URL               string `json:"url"`
	CreatedAt         string `json:"createdAt"`
	PublicRepos       int    `json:"publicRepos"`
	TotalPrivateRepos int    `json:"totalPrivateRepos"`
	NumOfUsers        int    `json:"numOfUsers"`
}

// TestOracles struct to hold the tests from json files
type TestOracles struct {
	RepositoryTestOracles   []RepositoryTestOracle   `json:",omitempty"`
	OrganizationTestOracles []OrganizationTestOracle `json:",omitempty"`
}

// Response struct to hold info about a response
type Response struct {
	Status int
	Body   string
	Header http.Header
}

// GQLRequest struct to hold query and variable strings of a GraphQL request
type GQLRequest struct {
	Query     string `json:"query"`
	Variables string `json:"variables"`
}
