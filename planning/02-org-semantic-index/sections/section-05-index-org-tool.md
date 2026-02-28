# Section 05: index_org MCP Tool

## Overview

This section creates the `index_org` MCP tool that indexes all repositories in an organization for semantic search with a single call. It builds org-wide vocabulary, then indexes all repos with bounded concurrency using per-goroutine embedder instances.

## Dependencies

- **section-02-org-vocabulary**: VocabularyData, BuildOrgVocabulary, VocabularyAwareEmbedder
- **section-03-incremental-vector-updates**: RefreshFileVectors, DeleteByFile
- **section-04-stale-cleanup**: CleanupStaleVectors, vocabulary staleness marking

## Tests First

### File: `internal/org/indexer_test.go` (new)

```go
// Test: IndexOrg indexes all repos in org with vectors
// Setup: Register org with 2 mock repos (both already analyzed with contexts)
// Call: IndexOrg(ctx, orgID, false, 3)
// Assert: IndexOrgResult.ReposIndexed == 2, TotalVectors > 0

// Test: IndexOrg builds vocabulary before indexing repos
// Setup: Register org with 2 repos
// Call: IndexOrg
// Assert: org_vocabulary table has entry for orgID, built before vectors stored

// Test: IndexOrg with bounded concurrency
// Setup: Org with 5 repos, concurrency=2
// Call: IndexOrg with concurrency=2
// Assert: At most 2 concurrent indexing operations (tracked via atomic counter)

// Test: IndexOrg creates per-goroutine embedder instances
// Setup: Org with 3 repos
// Assert: No shared embedder mutation during concurrent indexing
// Verify: Each goroutine gets its own embedder with imported vocabulary

// Test: IndexOrg handles partial failure
// Setup: Org with 3 repos, one configured to fail during indexing
// Call: IndexOrg
// Assert: ReposIndexed == 2, ReposFailed == 1, Failures contains the failed repo

// Test: IndexOrg with force re-indexes everything
// Setup: Pre-indexed org with existing vectors
// Call: IndexOrg with force=true
// Assert: All vectors regenerated with new vocab_version

// Test: IndexOrg returns error for unknown org
// Call: IndexOrg with non-existent orgID
// Assert: Error returned

// Test: IndexOrg with empty org (no repos)
// Setup: Org with no repos
// Call: IndexOrg
// Assert: ReposIndexed == 0, no error
```

### File: `internal/mcp/tools_test.go` (extend)

```go
// Test: toolIndexOrg calls IndexOrg and returns formatted result
// Setup: Mock manager that returns successful IndexOrgResult
// Call: toolIndexOrg with org_id
// Assert: Response contains repo count, vector count, no errors

// Test: toolIndexOrg returns error for missing org_id
// Call: toolIndexOrg without org_id
// Assert: Error response

// Test: toolIndexOrg passes concurrency parameter
// Setup: Mock manager
// Call: toolIndexOrg with concurrency=5
// Assert: Manager called with concurrency=5
```

## Implementation Details

### 1. IndexOrgResult and RepoFailure Types

**File:** `internal/org/types.go` or `internal/org/indexer.go`

```go
type IndexOrgResult struct {
    OrgID        string
    ReposIndexed int
    ReposFailed  int
    TotalVectors int
    Failures     []RepoFailure
    Duration     time.Duration
}

type RepoFailure struct {
    RepoID string
    Error  string
}
```

### 2. Org Indexer

**New file:** `internal/org/indexer.go`

Create an `Indexer` struct that handles org-wide semantic indexing.

```go
type Indexer struct {
    orgStore    Store              // org store for getting org/repos
    ctxStore    storage.ContextStore  // for loading repo contexts
    search      *vectors.SemanticSearch
    embedderFn  func() vectors.Embedder // factory for creating fresh embedder instances
}

func NewIndexer(orgStore Store, ctxStore storage.ContextStore, search *vectors.SemanticSearch, embedderFn func() vectors.Embedder) *Indexer
```

**IndexOrg method algorithm:**

1. Get org from store, validate exists
2. Get repo IDs from org
3. If no repos, return empty result
4. **Build org-wide vocabulary:** Call `search.BuildOrgVocabulary(ctx, orgID, repoIDs)`
5. **Index repos with bounded concurrency:**
   - Create semaphore channel of size `concurrency`
   - For each repo:
     a. Acquire semaphore
     b. Create fresh embedder via `embedderFn()`
     c. Import org vocabulary into the new embedder
     d. Load repo context
     e. If `force` or not already indexed: full index
     f. Else: incremental update (check for changed files)
     g. Tag vectors with org_id and vocab_version
     h. Release semaphore
   - Collect results and failures
6. Return IndexOrgResult with aggregated stats

**Concurrency pattern:** Matches the existing `org.Analyzer` approach using semaphore + sync.WaitGroup + error collection via mutex-protected slice.

### 3. Manager Interface Extension

**File:** `internal/org/manager.go` or `internal/orchestrator/manager.go`

Add to the Manager interface:

```go
IndexOrg(ctx context.Context, orgID string, force bool, concurrency int) (*IndexOrgResult, error)
```

The orchestrator Manager implementation delegates to `org.Indexer`.

### 4. MCP Tool Registration

**File:** `internal/mcp/server.go`

Add tool definition in `handleListTools()`:

```go
{
    Name:        "index_org",
    Description: "Index all repositories in an organization for semantic search",
    InputSchema: map[string]interface{}{
        "type": "object",
        "properties": map[string]interface{}{
            "org_id": map[string]interface{}{
                "type":        "string",
                "description": "Organization ID to index",
            },
            "force": map[string]interface{}{
                "type":        "boolean",
                "description": "Force re-index even if already indexed (default: false)",
            },
            "concurrency": map[string]interface{}{
                "type":        "integer",
                "description": "Max concurrent repo indexing (default: 3)",
            },
        },
        "required": []string{"org_id"},
    },
}
```

Add dispatch case in `handleCallToolWithID()`:

```go
case "index_org":
    result = s.toolIndexOrg(ctx, params.Arguments)
```

### 5. Tool Handler

**File:** `internal/mcp/tools.go`

```go
func (s *server) toolIndexOrg(ctx context.Context, args map[string]interface{}) callToolResult
```

Handler flow:
1. Parse `org_id` (required), `force` (default false), `concurrency` (default 3)
2. Call `manager.IndexOrg(ctx, orgID, force, concurrency)`
3. Format result as text summary:
   - "Indexed N repos (M failed) in Xs"
   - "Total vectors: V"
   - If failures: list each failed repo with error
4. Return formatted result

### 6. Per-Goroutine Embedder Factory

The `embedderFn` factory creates a fresh `LocalEmbedder` instance for each goroutine. This avoids shared state mutation. The factory is wired during server/manager initialization:

```go
embedderFn := func() vectors.Embedder {
    return vectors.NewLocalEmbedder(vectors.DefaultDimension)
}
```

Each goroutine:
1. Creates embedder via factory
2. Imports org vocabulary
3. Uses the embedder for all indexing in that goroutine
4. Discards embedder when done

## Error Handling

- Unknown org_id: return error with message
- Empty org (no repos): return success with ReposIndexed=0
- Vocabulary build failure: return error (vocabulary is prerequisite)
- Individual repo failure: capture in Failures slice, continue with remaining repos
- Context cancellation: stop launching new repos, wait for in-flight to complete

## File Summary

| File | Action |
|------|--------|
| `internal/org/indexer.go` | New: Indexer struct, IndexOrg method, IndexOrgResult/RepoFailure types |
| `internal/org/indexer_test.go` | New: Tests for IndexOrg concurrency, partial failure, vocabulary building |
| `internal/org/manager.go` | Add IndexOrg to Manager interface |
| `internal/orchestrator/manager.go` | Wire IndexOrg through orchestrator Manager |
| `internal/mcp/server.go` | Register index_org tool definition and dispatch |
| `internal/mcp/tools.go` | Add toolIndexOrg handler |
| `internal/mcp/tools_test.go` | Tests for toolIndexOrg handler |

## Implementation Order

1. Write tests
2. Define IndexOrgResult/RepoFailure types
3. Implement Indexer struct and IndexOrg method
4. Add to Manager interface and orchestrator
5. Register MCP tool and implement handler
6. Run all tests
