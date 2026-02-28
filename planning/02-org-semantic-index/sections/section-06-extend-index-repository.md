# Section 06: Extend index_repository Tool and SearchByOrg

## Overview

This section adds an optional `org_id` parameter to the existing `index_repository` MCP tool and fixes `SearchByOrg` to load org vocabulary before embedding the search query. When `org_id` is provided, the tool loads the org vocabulary, creates a fresh embedder with that vocabulary, and tags all vectors with `org_id` and `vocab_version`. When absent, behavior is unchanged (per-repo vocabulary).

## Dependencies

- **section-02-org-vocabulary**: VocabularyData, VocabularyAwareEmbedder, ExportVocabulary/ImportVocabulary, GetOrgVocabulary
- **section-03-incremental-vector-updates**: RefreshFileVectors (for org-scoped incremental updates)

## Tests First

### File: `internal/mcp/tools_test.go` (extend)

```go
// Test: toolIndexRepository with org_id uses org vocabulary
// Setup: Register org, build and store org vocabulary via BuildOrgVocabulary
// Call: toolIndexRepository with repo_id and org_id
// Assert: Vectors stored with org_id field set, vocab_version matches stored vocabulary hash

// Test: toolIndexRepository without org_id uses per-repo vocabulary (backward compat)
// Setup: Analyzed repo with functions
// Call: toolIndexRepository with repo_id only (no org_id)
// Assert: Vectors stored with empty org_id, no vocab_version tag. Behavior identical to current.

// Test: toolIndexRepository with org_id but no stored vocabulary returns error
// Setup: Register org but do NOT build vocabulary
// Call: toolIndexRepository with org_id
// Assert: Error response containing "run index_org first" or similar guidance

// Test: toolIndexRepository with org_id validates org exists
// Setup: No org registered
// Call: toolIndexRepository with non-existent org_id
// Assert: Error response indicating org not found
```

### File: `internal/vectors/search_test.go` (extend)

```go
// Test: SearchByOrg loads org vocabulary before embedding query
// Setup: Build org vocabulary, index 2 repos with org vocabulary
// Call: SearchByOrg(ctx, orgID, "search terms", 10)
// Assert: Results returned (proves query was embedded with same vocabulary as vectors)
// Verify: Create a fresh embedder WITHOUT org vocab, embed same query — cosine similarity
//         with stored vectors should be lower than SearchByOrg's results

// Test: SearchByOrg without stored vocabulary falls back gracefully
// Setup: Index repos with org_id but no vocabulary stored (edge case)
// Call: SearchByOrg
// Assert: Returns results (uses default embedder) or returns descriptive error

// Test: SearchByOrg respects org_id filtering
// Setup: Index repo A with org "alpha", repo B with org "beta"
// Call: SearchByOrg with org "alpha"
// Assert: Only vectors from org "alpha" repos returned

// Test: IndexRepositoryWithOrg tags vectors with vocab_version
// Setup: Build org vocabulary (version hash = "abc123")
// Call: IndexRepositoryWithOrg with org_id
// Assert: All stored VectorRecords have VocabVersion == "abc123"
```

## Implementation Details

### 1. Extend index_repository Tool Definition

**File:** `internal/mcp/server.go`

Add `org_id` to the `index_repository` tool's InputSchema properties:

```go
"org_id": map[string]interface{}{
    "type":        "string",
    "description": "Optional organization ID. When provided, uses org-wide vocabulary for embeddings",
},
```

No change to `required` — `org_id` remains optional.

### 2. Update toolIndexRepository Handler

**File:** `internal/mcp/tools.go`

Modify the existing `toolIndexRepository` handler:

1. Parse optional `org_id` from arguments (empty string if absent)
2. **If org_id is provided:**
   a. Validate org exists via org store (`GetOrg`)
   b. Load org vocabulary via `GetOrgVocabulary(ctx, orgID)`
   c. If vocabulary not found: return error "No vocabulary found for org. Run index_org first to build org-wide vocabulary."
   d. Call `manager.IndexRepositoryWithOrg(ctx, repoID, orgID)` — this existing method already handles org-tagged vectors
3. **If org_id is absent:** call existing `manager.IndexRepository(ctx, repoID)` — no behavior change

The key insight is that `IndexRepositoryWithOrg` already exists but doesn't currently load org vocabulary. The vocabulary loading needs to happen in the `SemanticSearch` layer.

### 3. Update IndexRepositoryWithOrg on SemanticSearch

**File:** `internal/vectors/search.go`

Modify `IndexRepositoryWithOrg` to accept and use org vocabulary:

**Current behavior:** `IndexRepositoryWithOrg` calls `IndexRepository` then tags vectors with org_id. The embedder uses whatever vocabulary it currently has (typically from the last `BuildVocabulary` call on that embedder instance).

**New behavior:**
1. Load org vocabulary from storage via `GetOrgVocabulary(ctx, orgID)`
2. If vocabulary exists:
   a. Create fresh embedder via factory function (or type-assert existing to `VocabularyAwareEmbedder`)
   b. Import org vocabulary into the new embedder
   c. Use this embedder for all embedding operations in this call
   d. Tag vectors with `vocab_version` from vocabulary's VersionHash
3. If no vocabulary: return error indicating `index_org` should be run first
4. Proceed with existing embedding logic using the org-aware embedder

**Alternative approach (simpler):** Add a `SetEmbedder` or accept an embedder parameter. Since `SemanticSearch` owns the embedder, the cleanest approach is:

```go
func (s *SemanticSearch) IndexRepositoryWithOrg(ctx context.Context, repoID, orgID string) error {
    // Load org vocabulary
    vocab, err := s.store.GetOrgVocabulary(ctx, orgID)
    if err != nil { return err }
    if vocab == nil {
        return fmt.Errorf("no vocabulary for org %s: run index_org first", orgID)
    }

    // Create org-aware embedder
    embedder := s.embedderFactory()
    if va, ok := embedder.(VocabularyAwareEmbedder); ok {
        va.ImportVocabulary(vocab)
    }

    // Use this embedder for indexing (pass to internal method)
    return s.indexRepoWithEmbedder(ctx, repoID, orgID, vocab.VersionHash, embedder)
}
```

This requires refactoring the internal indexing logic to accept an embedder parameter. Extract the core of `IndexRepository` into a shared `indexRepoWithEmbedder` method.

### 4. Fix SearchByOrg Vocabulary Loading

**File:** `internal/vectors/search.go`

**Current behavior:** `SearchByOrg` embeds the query using the current embedder state (whatever vocabulary was last built). If the embedder was used for a different repo's indexing, the vocabulary is wrong.

**New behavior:**
1. Before embedding the query, load org vocabulary via `GetOrgVocabulary(ctx, orgID)`
2. If vocabulary exists:
   a. Create fresh embedder or import vocabulary into a temporary embedder
   b. Embed query with the org vocabulary
   c. Search vectors filtered by org_id
3. If no vocabulary: fall back to current behavior (use default embedder) with a log warning

```go
func (s *SemanticSearch) SearchByOrg(ctx context.Context, orgID, query string, limit int) ([]SearchResult, error) {
    // Load org vocabulary for consistent query embedding
    vocab, err := s.store.GetOrgVocabulary(ctx, orgID)
    if err != nil {
        return nil, fmt.Errorf("loading org vocabulary: %w", err)
    }

    var queryVector []float32
    if vocab != nil {
        // Create embedder with org vocabulary
        embedder := s.embedderFactory()
        if va, ok := embedder.(VocabularyAwareEmbedder); ok {
            va.ImportVocabulary(vocab)
        }
        queryVector = embedder.Embed(query)
    } else {
        // Fallback: use default embedder (backward compat)
        queryVector = s.embedder.Embed(query)
    }

    // Search with org_id filter (existing logic)
    return s.vectorStore.SearchByOrg(ctx, orgID, queryVector, limit)
}
```

### 5. Embedder Factory on SemanticSearch

**File:** `internal/vectors/search.go`

Add an `embedderFactory` field to `SemanticSearch`:

```go
type SemanticSearch struct {
    vectorStore    VectorStore
    embedder       Embedder
    embedderFactory func() Embedder  // creates fresh embedder instances
    store          storage.ContextStore
}
```

Wire the factory during initialization (in `NewSemanticSearch` or the server setup). Default factory creates a `LocalEmbedder` with `DefaultDimension`:

```go
func defaultEmbedderFactory() Embedder {
    return NewLocalEmbedder(DefaultDimension)
}
```

If `embedderFactory` is nil, fall back to using `s.embedder` directly (backward compat for tests and existing callers).

### 6. Shared indexRepoWithEmbedder Method

**File:** `internal/vectors/search.go`

Extract core indexing logic from `IndexRepository` into a shared private method:

```go
func (s *SemanticSearch) indexRepoWithEmbedder(ctx context.Context, repoID, orgID, vocabVersion string, embedder Embedder) error
```

This method:
1. Loads repo context
2. Extracts document strings from functions and types
3. Builds vocabulary (only if no org vocabulary — i.e., orgID is empty)
4. Embeds all documents using the provided embedder
5. Creates VectorRecords with orgID and vocabVersion fields set
6. Stores via StoreBatch

`IndexRepository` becomes a thin wrapper that calls `indexRepoWithEmbedder` with `s.embedder`, empty orgID, and empty vocabVersion.

`IndexRepositoryWithOrg` loads the org vocabulary, creates a fresh embedder, imports vocabulary, then calls `indexRepoWithEmbedder`.

## Error Handling

- Missing org_id in `index_repository` call: no error (backward compat, use per-repo vocabulary)
- org_id provided but org doesn't exist: return error "org not found"
- org_id provided but no vocabulary stored: return error "no vocabulary for org, run index_org first"
- Vocabulary import failure: return error (shouldn't happen with valid VocabularyData)
- SearchByOrg with no vocabulary: fall back to default embedder with log warning

## File Summary

| File | Action |
|------|--------|
| `internal/mcp/server.go` | Add `org_id` to `index_repository` tool schema |
| `internal/mcp/tools.go` | Update `toolIndexRepository` to parse and pass `org_id` |
| `internal/vectors/search.go` | Add `embedderFactory`, refactor `IndexRepositoryWithOrg`, fix `SearchByOrg` vocabulary loading, extract `indexRepoWithEmbedder` |
| `internal/mcp/tools_test.go` | Tests for `toolIndexRepository` with/without `org_id` |
| `internal/vectors/search_test.go` | Tests for `SearchByOrg` vocabulary loading, `IndexRepositoryWithOrg` vocab_version tagging |

## Implementation Order

1. Write tests
2. Add `embedderFactory` to `SemanticSearch` and wire in initialization
3. Extract `indexRepoWithEmbedder` from `IndexRepository`
4. Update `IndexRepositoryWithOrg` to load org vocabulary and use fresh embedder
5. Fix `SearchByOrg` to load org vocabulary before embedding query
6. Add `org_id` to `index_repository` tool schema
7. Update `toolIndexRepository` handler
8. Run all tests
