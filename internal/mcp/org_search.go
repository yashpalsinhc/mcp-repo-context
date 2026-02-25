package mcp

import (
	"context"

	ctxpkg "github.com/yashpalc/mcp-repo-context/internal/context"
)

// OrgSearcher provides org-scoped search methods.
// Implemented by storage.SQLiteStore.
type OrgSearcher interface {
	SearchFunctionsOrg(ctx context.Context, orgID string, query string, limit int) ([]ctxpkg.FunctionRef, error)
	SearchByConceptOrg(ctx context.Context, orgID string, concept string, limit int) ([]ctxpkg.FunctionRef, error)
	HybridSearchOrg(ctx context.Context, orgID string, query string, limit int) ([]ctxpkg.FunctionRef, error)
}
