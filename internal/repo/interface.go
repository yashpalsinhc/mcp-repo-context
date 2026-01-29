package repo

import (
	"context"
)

// Source abstracts repository access methods.
type Source interface {
	// Clone fetches repository to local temporary directory.
	// Returns path to cloned repo and cleanup function.
	Clone(ctx context.Context, repoURL string, opts CloneOptions) (localPath string, cleanup func(), err error)

	// GetCurrentCommit returns the HEAD commit hash.
	GetCurrentCommit(ctx context.Context, localPath string) (string, error)

	// GetBranch returns the current branch name.
	GetBranch(ctx context.Context, localPath string) (string, error)
}

// FileScanner walks repository files.
type FileScanner interface {
	// Scan walks all files in repository, respecting ignore patterns.
	Scan(ctx context.Context, rootPath string, fn func(FileInfo) error) error
}
