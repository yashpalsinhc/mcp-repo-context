package org

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	ctxpkg "github.com/yashpalc/mcp-repo-context/internal/context"
	"github.com/yashpalc/mcp-repo-context/internal/vectors"
)

// IndexOrgResult holds the outcome of indexing all repos in an org.
type IndexOrgResult struct {
	OrgID        string        `json:"org_id"`
	ReposIndexed int           `json:"repos_indexed"`
	ReposFailed  int           `json:"repos_failed"`
	TotalVectors int           `json:"total_vectors"`
	Failures     []RepoFailure `json:"failures,omitempty"`
	Duration     time.Duration `json:"duration"`
}

// RepoFailure records a per-repo indexing failure.
type RepoFailure struct {
	RepoID string `json:"repo_id"`
	Error  string `json:"error"`
}

// ContextLoader loads repository context by ID.
type ContextLoader interface {
	GetContext(ctx context.Context, repoID string) (*ctxpkg.RepoContext, error)
}

// VocabStorer stores org vocabulary data.
type VocabStorer interface {
	StoreOrgVocabulary(ctx context.Context, orgID string, vocabJSON []byte, versionHash string, docCount, repoCount int) error
}

// Indexer orchestrates org-wide semantic indexing.
type Indexer struct {
	orgManager Manager
	ctxLoader  ContextLoader
	search     *vectors.SemanticSearch
	vocabStore VocabStorer
}

// NewIndexer creates an Indexer.
func NewIndexer(orgManager Manager, ctxLoader ContextLoader, search *vectors.SemanticSearch, vocabStore VocabStorer) *Indexer {
	return &Indexer{
		orgManager: orgManager,
		ctxLoader:  ctxLoader,
		search:     search,
		vocabStore: vocabStore,
	}
}

// IndexOrg indexes all repositories in an org for semantic search.
// It builds org-wide vocabulary, then indexes repos with bounded concurrency.
func (idx *Indexer) IndexOrg(ctx context.Context, orgID string, force bool, concurrency int) (*IndexOrgResult, error) {
	if concurrency <= 0 {
		concurrency = 3
	}
	if concurrency > 10 {
		concurrency = 10
	}

	o, err := idx.orgManager.Get(ctx, orgID)
	if err != nil {
		return nil, fmt.Errorf("failed to get org: %w", err)
	}

	start := time.Now()
	result := &IndexOrgResult{OrgID: orgID}

	if len(o.Repos) == 0 {
		result.Duration = time.Since(start)
		return result, nil
	}

	// Step 1: Load all repo contexts
	repoContexts := make(map[string]*ctxpkg.RepoContext)
	var loadErrors []RepoFailure
	for _, repoID := range o.Repos {
		repoCtx, err := idx.ctxLoader.GetContext(ctx, repoID)
		if err != nil {
			loadErrors = append(loadErrors, RepoFailure{RepoID: repoID, Error: err.Error()})
			continue
		}
		repoContexts[repoID] = repoCtx
	}

	// Step 2: Build org vocabulary from all loaded contexts
	var repos []*ctxpkg.RepoContext
	for _, rc := range repoContexts {
		repos = append(repos, rc)
	}

	vocabData, err := idx.search.BuildOrgVocabulary(repos)
	if err != nil {
		return nil, fmt.Errorf("failed to build org vocabulary: %w", err)
	}

	// Store vocabulary
	vocabJSON, err := json.Marshal(vocabData.WordIDF)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal vocabulary: %w", err)
	}

	if idx.vocabStore != nil {
		if err := idx.vocabStore.StoreOrgVocabulary(ctx, orgID, vocabJSON, vocabData.VersionHash, vocabData.DocCount, len(repos)); err != nil {
			return nil, fmt.Errorf("failed to store org vocabulary: %w", err)
		}
	}

	// Step 3: Index repos with bounded concurrency
	sem := make(chan struct{}, concurrency)
	var mu sync.Mutex
	var wg sync.WaitGroup

	for repoID, repoCtx := range repoContexts {
		if ctx.Err() != nil {
			break
		}

		wg.Add(1)
		go func(repoID string, repoCtx *ctxpkg.RepoContext) {
			defer wg.Done()

			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				return
			}
			defer func() { <-sem }()

			// Clear existing if force
			if force {
				_ = idx.search.CleanupRepoFromOrg(ctx, orgID, repoID)
			}

			indexErr := idx.search.IndexRepositoryWithOrg(ctx, repoCtx, orgID)

			mu.Lock()
			defer mu.Unlock()
			if indexErr != nil {
				result.ReposFailed++
				result.Failures = append(result.Failures, RepoFailure{
					RepoID: repoID,
					Error:  indexErr.Error(),
				})
			} else {
				result.ReposIndexed++
			}
		}(repoID, repoCtx)
	}

	wg.Wait()

	// Add load errors
	result.ReposFailed += len(loadErrors)
	result.Failures = append(result.Failures, loadErrors...)

	// Get total vector count
	count, _ := idx.search.CountByOrg(ctx, orgID)
	result.TotalVectors = count
	result.Duration = time.Since(start)

	return result, nil
}
