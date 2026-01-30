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

// SearchableStore extends ContextStore with search capabilities.
// SQLiteStore implements this interface; FilesystemStore does not.
type SearchableStore interface {
	ContextStore

	// SearchFunctions searches for functions using full-text search.
	SearchFunctions(ctx context.Context, repoID, query string) ([]ctxpkg.FunctionRef, error)

	// SearchBySideEffect finds functions with specific side effects.
	SearchBySideEffect(ctx context.Context, repoID, effect string) ([]ctxpkg.FunctionRef, error)

	// SearchByConcept finds functions related to a concept.
	SearchByConcept(ctx context.Context, repoID, concept string) ([]ctxpkg.FunctionRef, error)

	// SearchFiles searches for files.
	SearchFiles(ctx context.Context, repoID, query string) ([]FileSearchResult, error)

	// SearchTypes searches for types.
	SearchTypes(ctx context.Context, repoID, query string) ([]TypeSearchResult, error)

	// GetFunctionCallers finds all functions that call the given function.
	GetFunctionCallers(ctx context.Context, repoID, funcName string) ([]ctxpkg.FunctionRef, error)

	// HybridSearch performs combined keyword + concept search.
	HybridSearch(ctx context.Context, repoID, query string) ([]ctxpkg.FunctionRef, error)

	// GetRepoStats returns statistics for a repository.
	GetRepoStats(ctx context.Context, repoID string) (*RepoStats, error)

	// Close closes the store (needed for SQLite).
	Close() error
}
