package types

import bitbucketv1 "github.com/gfleury/go-bitbucket-v1"

// TODO alias bitbucketv1 types

type Commit struct {
	ID        string
	DisplayID string
	Message   string
	Author    bitbucketv1.User
	// AuthorTimestamp
	committer bitbucketv1.User
	// CommitterTimestamp
	// Parents
}

type Diff struct {
	Source      struct{}
	Destination struct{}
	Hunks       []struct {
		Segments []struct {
			Type  string
			Lines []struct {
				Destination int
				Source      int
				Line        string
			}
		}
	}
}

type DiffResp struct {
	Diffs []Diff
}

type PullRequest struct {
	bitbucketv1.PullRequest
	Commits        int
	ChangedFiles   int
	Additions      int
	Deletions      int
	Comments       int
	ReviewComments int

	ClosedAt int64
	MergedAt int64
	MergedBy bitbucketv1.User
}

type Comment struct {
	ID          int
	Text        string
	Author      bitbucketv1.User
	CreatedDate int64
	UpdatedDate int64
	Comments    []Comment
	// tasks
}

type Review struct {
	ID          int
	State       string
	User        bitbucketv1.User
	CreatedDate int64
}

type PRStateUpdate struct {
	State string
	User  bitbucketv1.User
	Date  int64
}

type CommentAnchor struct {
	Line     int
	LineType string
	FileType string
	Path     string
	SrcPath  string
	FromHash string
	ToHash   string
}

type Activity struct {
	ID          int
	CreatedDate int64
	User        bitbucketv1.User
	Action      string
	// fields below are only for comments
	CommentAction string
	Comment       Comment
	// fields below are only for comments in code
	CommentAnchor *CommentAnchor
	Diff          *struct {
		Hunks []struct {
			DestinationLine int
			DestinationSpan int
			SourceLine      int
			SourceSpan      int
			// segments
		}
	}
}

type DiffComment struct {
	Comment
	CommentAnchor
}
