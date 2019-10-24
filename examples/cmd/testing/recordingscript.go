package main

import (
	"flag"
)

// This script will use the downloader to crawl using the graphql API a repository (default: src-d/gitbase)
// and an organization (default: src-d) and will record requests (graphql queries) and responses (json body data)
// into a map stored in a gob file.
// NB: script is assumed to run from the project root (for correct file storage)
func main() {
	var (
		org   string
		repo  string
		happy bool
	)
	flag.StringVar(&org, "org", "src-d", "a GitHub organization")
	flag.StringVar(&repo, "repo", "gitbase", "a GitHub repository")
	flag.BoolVar(&happy, "happy", true, "happy or unhappy paths")
	flag.Parse()

	if happy {
		recordHappy(org, repo)
	} else {
		recordUnhappy(org, repo)
	}

}
