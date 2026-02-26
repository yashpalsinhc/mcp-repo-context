package mcp

import (
	"context"
	"fmt"
	"log"
	"time"

	ctxpkg "github.com/yashpalc/mcp-repo-context/internal/context"
)

// autoIndexRepo indexes a repository's vectors after analysis.
// Returns the number of indexed items. Errors are non-fatal.
func (s *server) autoIndexRepo(ctx context.Context, repoID string, repoCtx *ctxpkg.RepoContext) (int, error) {
	if !s.semanticSearch.IsAvailable() {
		return 0, fmt.Errorf("semantic search not available")
	}
	if repoCtx == nil || len(repoCtx.Files) == 0 {
		return 0, nil
	}

	// Clear existing vectors for this repo (in case of re-analyze)
	if err := s.semanticSearch.ClearRepository(ctx, repoID); err != nil {
		log.Printf("Warning: failed to clear existing vectors for %s: %v", repoID, err)
	}

	if err := s.semanticSearch.IndexRepository(ctx, repoCtx); err != nil {
		return 0, fmt.Errorf("indexing failed: %w", err)
	}

	count, _ := s.semanticSearch.Count(ctx, repoID)
	return count, nil
}

// maybeAutoIndex runs auto-indexing if enabled and appends status to output.
func (s *server) maybeAutoIndex(ctx context.Context, repoID string, sb interface{ WriteString(string) (int, error) }) {
	if !s.config.AutoIndex || !s.semanticSearch.IsAvailable() {
		return
	}

	repoCtx, err := s.manager.GetContext(ctx, repoID)
	if err != nil || repoCtx == nil {
		return
	}

	start := time.Now()
	indexed, err := s.autoIndexRepo(ctx, repoID, repoCtx)
	if err != nil {
		log.Printf("Warning: auto-indexing failed for %s: %v", repoID, err)
		return
	}
	duration := time.Since(start)
	sb.WriteString(fmt.Sprintf("\n\n**Auto-indexed** %d items in %s", indexed, duration.Round(time.Millisecond)))
}
