# Section 02: Org-Wide Vocabulary with Versioning

## Overview

This section implements org-wide vocabulary building, storage, and versioning. The LocalEmbedder assigns word-to-dimension slots based on vocabulary order (`vocabulary[word] = i % dimension`), so any vocabulary change invalidates all existing embeddings. This section addresses that by tracking vocabulary versions.

## Dependencies

- None (can be implemented in parallel with section-01)

## Tests First

### File: `internal/vectors/embedder_test.go` (extend)

```go
// Test: ExportVocabulary returns current vocabulary state
// Setup: Build vocabulary from ["hello world", "foo bar baz"], export
// Assert: VocabularyData.WordIDF has entries, DocCount == 2, VersionHash non-empty

// Test: ImportVocabulary restores embedder state
// Setup: Build vocab, export. Create new embedder, import
// Assert: Embed("hello world") produces identical vector on both embedders

// Test: VersionHash changes when vocabulary changes
// Setup: Build from set A, get hash. Build from set B, get hash
// Assert: Hashes differ

// Test: VersionHash is stable for same vocabulary
// Setup: Build from same documents twice
// Assert: Same VersionHash both times (deterministic)

// Test: VocabularyAwareEmbedder type assertion works
// Setup: Create LocalEmbedder
// Assert: Type assertion to VocabularyAwareEmbedder succeeds
```

### File: `internal/vectors/search_test.go` (extend)

```go
// Test: BuildOrgVocabulary creates vocabulary from multiple repos
// Setup: Create 2 mock RepoContexts with known functions
// Call: BuildOrgVocabulary(ctx, "test-org", repoIDs)
// Assert: org_vocabulary table has entry with correct org_id, non-empty vocabulary_json

// Test: GetOrgVocabulary loads stored vocabulary
// Setup: Store vocabulary manually in org_vocabulary table
// Call: GetOrgVocabulary(ctx, "test-org")
// Assert: Returns VocabularyData with correct VersionHash and DocCount

// Test: MarkOrgVocabularyStale sets is_stale flag
// Setup: Store non-stale vocabulary
// Call: MarkOrgVocabularyStale(ctx, "test-org")
// Assert: is_stale == true in database

// Test: BuildOrgVocabulary streams repos (does not load all at once)
// Setup: 3 repos
// Assert: Function processes one repo at a time (verified by mock that tracks concurrent loads)
```

### File: `internal/storage/sqlite_test.go` (extend)

```go
// Test: StoreOrgVocabulary and GetOrgVocabulary round-trip
// Test: UpdateOrgVocabulary replaces existing entry
// Test: DeleteOrgVocabulary removes entry
// Test: GetOrgVocabulary returns nil for non-existent org
```

## Implementation Details

### 1. VocabularyData Type

**New file:** `internal/vectors/vocabulary.go`

```go
type VocabularyData struct {
    WordIDF     map[string]float64 `json:"word_idf"`
    DocCount    int                `json:"doc_count"`
    BuiltAt     time.Time          `json:"built_at"`
    VersionHash string             `json:"version_hash"` // SHA256 of sorted vocabulary keys
}
```

**ComputeVersionHash function:** Sort all keys in WordIDF alphabetically, join with newline, SHA256 the result. This produces a deterministic hash that changes only when the vocabulary words change.

### 2. VocabularyAwareEmbedder Interface

**File:** `internal/vectors/embedder.go`

Add a new interface alongside the existing `Embedder`:

```go
type VocabularyAwareEmbedder interface {
    Embedder
    ExportVocabulary() *VocabularyData
    ImportVocabulary(vocab *VocabularyData) error
}
```

### 3. ExportVocabulary / ImportVocabulary on LocalEmbedder

**File:** `internal/vectors/embedder.go`

`ExportVocabulary()`:
1. Lock `e.mu` (read lock)
2. Copy `e.idf` map and `e.docCount` into new VocabularyData
3. Also capture `e.vocabulary` map (word → dimension slot index)
4. Compute VersionHash from sorted vocabulary keys
5. Set BuiltAt to time.Now()
6. Return the VocabularyData

`ImportVocabulary(vocab *VocabularyData) error`:
1. Lock `e.mu` (write lock)
2. Restore `e.idf` from vocab.WordIDF
3. Rebuild `e.vocabulary` map from the keys (same order as original: sort keys, assign `i % e.dimension`)
4. Set `e.docCount` from vocab.DocCount
5. Return nil

**Critical:** The ImportVocabulary must reproduce the exact same word-to-slot mapping as when the vocabulary was originally built. Sort keys alphabetically and assign slots sequentially to ensure determinism.

### 4. org_vocabulary SQLite Table

**File:** `internal/storage/migrations/NNN_org_vocabulary.sql` (documentation)

Actual migration in Go code (idempotent pattern matching existing migrations):

```sql
CREATE TABLE IF NOT EXISTS org_vocabulary (
    org_id TEXT PRIMARY KEY,
    vocabulary_json TEXT NOT NULL,
    version_hash TEXT NOT NULL,
    doc_count INTEGER DEFAULT 0,
    built_at TIMESTAMP,
    repo_count INTEGER DEFAULT 0,
    is_stale BOOLEAN DEFAULT 0
);
```

### 5. Storage Methods

**File:** `internal/storage/sqlite.go` (extend)

- `StoreOrgVocabulary(ctx, orgID string, vocab *VocabularyData, repoCount int) error` - INSERT OR REPLACE into org_vocabulary, serialize vocabulary_json
- `GetOrgVocabulary(ctx, orgID string) (*VocabularyData, error)` - SELECT and deserialize. Returns nil if not found.
- `MarkOrgVocabularyStale(ctx, orgID string) error` - UPDATE set is_stale=1
- `DeleteOrgVocabulary(ctx, orgID string) error` - DELETE from org_vocabulary
- `IsOrgVocabularyStale(ctx, orgID string) (bool, error)` - SELECT is_stale

### 6. BuildOrgVocabulary on SemanticSearch

**File:** `internal/vectors/search.go` (extend)

```go
func (s *SemanticSearch) BuildOrgVocabulary(ctx context.Context, orgID string, repoIDs []string) (*VocabularyData, error)
```

**Algorithm:**
1. Collect documents by streaming per-repo (not all at once):
   - For each repoID: load RepoContext, extract function/type document strings, discard context
   - Append document strings to a growing slice
2. Call `embedder.BuildVocabulary(allDocuments)` on a fresh LocalEmbedder instance
3. Export vocabulary via `ExportVocabulary()`
4. Store via `StoreOrgVocabulary(ctx, orgID, vocab, len(repoIDs))`
5. Return the VocabularyData

**Memory optimization:** Process one repo at a time. After extracting document strings from a repo, the RepoContext can be GC'd before loading the next one.

### 7. vocab_version Column on Vectors Table

**File:** `internal/vectors/store.go`

Add `vocab_version TEXT DEFAULT ''` column to vectors table via migration. Update `VectorRecord` struct to include `VocabVersion string`. Populate during StoreBatch when org-wide vocabulary is used.

## Error Handling

- If vocabulary serialization fails (JSON marshal), return error
- If org doesn't exist, return descriptive error
- If no repos in org, return empty vocabulary (not error)

## File Summary

| File | Action |
|------|--------|
| `internal/vectors/vocabulary.go` | New: VocabularyData type, ComputeVersionHash |
| `internal/vectors/embedder.go` | Add VocabularyAwareEmbedder interface, ExportVocabulary/ImportVocabulary on LocalEmbedder |
| `internal/vectors/search.go` | Add BuildOrgVocabulary, GetOrgVocabulary, MarkOrgVocabularyStale |
| `internal/vectors/store.go` | Add vocab_version column to vectors table and VectorRecord |
| `internal/storage/sqlite.go` | Add org_vocabulary CRUD methods and migration |
| `internal/vectors/embedder_test.go` | Tests for Export/Import vocabulary |
| `internal/vectors/search_test.go` | Tests for BuildOrgVocabulary |
| `internal/storage/sqlite_test.go` | Tests for org_vocabulary storage |

## Implementation Order

1. Write tests
2. Implement VocabularyData type and ComputeVersionHash
3. Add VocabularyAwareEmbedder interface
4. Implement ExportVocabulary/ImportVocabulary
5. Add org_vocabulary table and storage methods
6. Implement BuildOrgVocabulary
7. Add vocab_version column
8. Run all tests
