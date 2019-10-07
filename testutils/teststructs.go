package testutils

// RepositoryTest struct to hold a test oracle for a repository
type RepositoryTest struct {
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

// OrganizationTest struct to hold a test oracle for an organization
type OrganizationTest struct {
	Org               string `json:"org"`
	Version           int    `json:"version"`
	URL               string `json:"url"`
	CreatedAt         string `json:"createdAt"`
	PublicRepos       int    `json:"publicRepos"`
	TotalPrivateRepos int    `json:"totalPrivateRepos"`
	NumOfUsers        int    `json:"numOfUsers"`
}

// Tests struct to hold the tests from json files
type Tests struct {
	RepositoryTests    []RepositoryTest   `json:"repositoryTests"`
	OrganizationsTests []OrganizationTest `json:"organizationTests"`
}
