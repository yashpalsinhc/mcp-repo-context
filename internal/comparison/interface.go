// Package comparison provides multi-repo comparison and merge analysis functionality.
package comparison

import (
	"context"

	ctxpkg "github.com/yashpalc/mcp-repo-context/internal/context"
)

// Comparer analyzes multiple repositories to identify similarities, conflicts, and gaps.
type Comparer interface {
	// Compare analyzes multiple repos and returns a comparison result.
	Compare(ctx context.Context, repoContexts []*ctxpkg.RepoContext, opts CompareOptions) (*CompareResult, error)

	// FindDuplicates identifies duplicate or similar implementations across repos.
	FindDuplicates(ctx context.Context, repoContexts []*ctxpkg.RepoContext) ([]DuplicateGroup, error)

	// FindConflicts identifies conflicting implementations between source and target repos.
	FindConflicts(ctx context.Context, sourceContexts []*ctxpkg.RepoContext, targetContext *ctxpkg.RepoContext) ([]Conflict, error)

	// FindGaps identifies functionality in source repos missing from target repo.
	FindGaps(ctx context.Context, sourceContexts []*ctxpkg.RepoContext, targetContext *ctxpkg.RepoContext) ([]Gap, error)

	// AnalyzeConsistency checks for implementation consistency across repos.
	AnalyzeConsistency(ctx context.Context, repoContexts []*ctxpkg.RepoContext) (*ConsistencyReport, error)
}

// Interface compliance check
var _ Comparer = (*comparer)(nil)
