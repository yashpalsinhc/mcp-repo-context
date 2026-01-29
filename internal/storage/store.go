package storage

import (
	"context"
	"errors"
	"time"

	ctxpkg "github.com/yashpalc/mcp-repo-context/internal/context"
)

// Sentinel errors
var (
	ErrNotFound    = errors.New("context not found")
	ErrStoreFailed = errors.New("failed to store context")
)

// ContextStore abstracts storage backends for repository context.
type ContextStore interface {
	// StoreRepoContext persists complete repository context.
	StoreRepoContext(ctx context.Context, repoID string, repoCtx *ctxpkg.RepoContext) error

	// GetRepoContext retrieves full repository context.
	GetRepoContext(ctx context.Context, repoID string) (*ctxpkg.RepoContext, error)

	// GetFileContext retrieves context for a specific file.
	GetFileContext(ctx context.Context, repoID, filePath string) (*ctxpkg.FileContext, error)

	// ContextExists checks if context exists and is within maxAge.
	ContextExists(ctx context.Context, repoID string, maxAge time.Duration) (bool, time.Time, error)

	// DeleteContext removes all context for a repository.
	DeleteContext(ctx context.Context, repoID string) error

	// ListContexts returns metadata for all stored contexts.
	ListContexts(ctx context.Context) ([]ctxpkg.ContextMetadata, error)
}
