package workflows

import (
	"context"

	ctxpkg "github.com/yashpalc/mcp-repo-context/internal/context"
)

// RepoSearcher provides search capabilities over analyzed repos.
type RepoSearcher interface {
	// SearchFunctions searches for functions by query.
	SearchFunctions(ctx context.Context, repoID, query string) ([]ctxpkg.FunctionRef, error)

	// SearchByConcept searches for functions related to a concept.
	SearchByConcept(ctx context.Context, repoID, concept string) ([]ctxpkg.FunctionRef, error)

	// GetContext retrieves repository context.
	GetContext(ctx context.Context, repoID string) (*ctxpkg.RepoContext, error)

	// GetCallers returns callers of a function (returns function refs).
	GetCallers(ctx context.Context, repoID, funcName string) ([]ctxpkg.FunctionRef, error)
}

// OrgResolver resolves org IDs to repo IDs.
type OrgResolver interface {
	// GetOrgRepos returns the repo IDs in an org.
	GetOrgRepos(ctx context.Context, orgID string) ([]string, error)
}

// VectorSearcher provides semantic search across repos.
type VectorSearcher interface {
	// SearchAll performs semantic search across a single repo.
	SearchAll(ctx context.Context, query string, repoID string, limit int) ([]VectorResult, error)
}

// VectorResult is a semantic search result.
type VectorResult struct {
	RepoID   string
	FilePath string
	Name     string
	Line     int
	Summary  string
	Score    float64
	ItemType string // "function" or "type"
}

// AskFunc is a function type for AI-powered queries.
type AskFunc func(ctx context.Context, query string, repoIDs []string) (string, error)
