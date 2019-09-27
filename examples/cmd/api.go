package main

import (
	"context"
	"fmt"
	"net/http"

	"github.com/shurcooL/githubv4"
	"github.com/src-d/metadata-retrieval/github/graphql"
)

func listRepositories(ctx context.Context, httpClient *http.Client, login string, noForks bool) ([]string, error) {
	client := githubv4.NewClient(httpClient)

	repos := []string{}

	hasNextPage := true

	variables := map[string]interface{}{
		"login": githubv4.String(login),

		"repositoriesPage":   githubv4.Int(100),
		"repositoriesCursor": (*githubv4.String)(nil),
	}

	if noForks {
		variables["isFork"] = githubv4.Boolean(false)
	} else {
		variables["isFork"] = (*githubv4.Boolean)(nil)
	}

	for hasNextPage {
		var q struct {
			Organization struct {
				Repositories struct {
					PageInfo graphql.PageInfo
					Nodes    []struct {
						Name string
					}
				} `graphql:"repositories(first:$repositoriesPage, after: $repositoriesCursor, isFork: $isFork)"`
			} `graphql:"organization(login: $login)"`
		}

		err := client.Query(ctx, &q, variables)
		if err != nil {
			return nil, fmt.Errorf("failed to query organization %v repositories: %v", login, err)
		}

		for _, node := range q.Organization.Repositories.Nodes {
			repos = append(repos, node.Name)
		}

		hasNextPage = q.Organization.Repositories.PageInfo.HasNextPage
		variables["repositoriesCursor"] = githubv4.String(q.Organization.Repositories.PageInfo.EndCursor)
	}

	return repos, nil
}
