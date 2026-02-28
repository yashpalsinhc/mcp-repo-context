Now I have enough context. Let me produce the section content.

# Section 3: Incremental Vector Updates

## Overview

This section implements `RefreshFileVectors`, a method on `SemanticSearch` that updates only the embeddings for functions and types that actually changed when a file is refreshed. It also adds `DeleteByFile` to the vector store for targeted file-level cleanup. This is the bridge between the function-level hash tracking (Section 1) and vocabulary infrastructure (Section 2) and the actual refresh flow.

## Dependencies

- **Section 1 (Function-Level Hash Tracking):** Provides `FunctionHashInfo`, `GetFunctionHashes`, `UpdateFunctionHashes`, `DeleteFunctionHashes`, and `GetChangedFunctions` on `SQLiteStore`. These are used to detect which functions within a file have been added, modified, or removed.
- **Section 2 (Org-Wide Vocabulary with Versioning):** Provides `VocabularyAwareEmbedder` interface, `ExportVocabulary`/`ImportVocabulary` on `LocalEmbedder`, `GetOrgVocabulary` on `SemanticSearch`, and the `VocabularyData` type. These are used to load the correct vocabulary when embedding functions for org-scoped updates.

## Background

### Current refresh_file Flow

The existing `refresh_file` / `refresh_changed` flow in the orchestrator manager:
1. Re-analyzes the file (re-extracts functions, types, imports, etc.)
2. Updates the `file_hashes` table with the new file hash
3. Does NOT update any vector embeddings

This means after a file is refreshed, the semantic search index is stale for that file. Users must run a full `index_repository` again to update embeddings.

### Two Separate SQLite Databases

The codebase uses two independent SQLite databases:
- **Main storage DB** (`internal/storage/sqlite.go`) -- stores repos, files, functions, types, `file_hashes`, `function_hashes` (Section 1), `org_vocabulary` (Section 2)
- **Vector DB** (`internal/vectors/store.go`) -- stores the `vectors` table with embeddings

The `function_hashes` table has a `vector_id` column that is an application-level reference to a vector record's ID (not a database foreign key, since they are in separate databases). This cross-DB design means true transactional consistency is not possible. The trade-off is documented below.

### Vector ID Format

Vector records use the ID format: `{repoID}:{type}:{filePath}:{name}`, for example:
- `myrepo:func:pkg/handler.go:CreateUser`
- `myrepo:type:pkg/models.go:UserRequest`

This format is established in `internal/vectors/search.go` in the existing `IndexRepository` and `IndexRepositoryWithOrg` methods.

### Function Name Qualification

As defined in Section 1, function names include the receiver to avoid collisions. For example, `(*Foo).Bar` and `(*Baz).Bar` are distinct entries. When constructing vector IDs for functions with receivers, the fully qualified name must be used.

## Tests

All tests extend the existing test file at `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/vectors/search_test.go` and `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/vectors/store_test.go`.

### File: `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/vectors/store_test.go` (extend)

```go
// Test: DeleteByFile removes all vectors for a file path
// Setup: Store 3 vectors for file "pkg/foo.go" and 2 for "pkg/bar.go"
// Call: DeleteByFile(repoID, "pkg/foo.go")
// Assert: Only bar.go vectors remain
func TestDeleteByFile(t *testing.T) {
    // Create temp SQLite vector store
    // Store vectors with IDs following the pattern "{repoID}:func:{filePath}:{name}"
    // e.g., "repo1:func:pkg/foo.go:FuncA", "repo1:func:pkg/foo.go:FuncB", etc.
    // Call DeleteByFile(ctx, "repo1", "pkg/foo.go")
    // Verify: count for repo1 drops from 5 to 2
    // Verify: remaining vectors all have file_path "pkg/bar.go"
}
```

### File: `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/vectors/search_test.go` (extend)

```go
// Test: RefreshFileVectors adds embeddings for new functions
// Setup: File with 0 existing vectors, 2 new functions
// Call: RefreshFileVectors
// Assert: 2 vectors created, RefreshResult.Added == 2
func TestRefreshFileVectors_AddsNew(t *testing.T) {
    // Create temp main DB (SQLiteStore) and vector DB (SQLiteVectorStore)
    // Create a FileContext with 2 functions
    // No existing function hashes in storage
    // Call RefreshFileVectors(ctx, repoID, filePath, &fileCtx, "")
    // Assert result.Added == 2, result.Modified == 0, result.Removed == 0
    // Assert vector store count for repo == 2
    // Assert function hashes stored for both functions
}

// Test: RefreshFileVectors updates embedding for modified function
// Setup: File with 1 existing vector, function source changed
// Call: RefreshFileVectors
// Assert: Old vector deleted, new vector created, RefreshResult.Modified == 1
func TestRefreshFileVectors_UpdatesModified(t *testing.T) {
    // Pre-populate function_hashes with hash "oldhash" for FuncA
    // Pre-populate vector store with a vector for FuncA
    // Create FileContext with FuncA having different source (new hash)
    // Call RefreshFileVectors
    // Assert result.Modified == 1
    // Assert the vector ID still exists but with updated embedding
}

// Test: RefreshFileVectors removes embedding for deleted function
// Setup: File with 2 vectors, one function removed
// Call: RefreshFileVectors
// Assert: Removed vector gone, remaining vector unchanged, RefreshResult.Removed == 1
func TestRefreshFileVectors_RemovesDeleted(t *testing.T) {
    // Pre-populate function_hashes for FuncA and FuncB
    // Pre-populate vectors for FuncA and FuncB
    // Create FileContext with only FuncA (FuncB removed)
    // Call RefreshFileVectors
    // Assert result.Removed == 1
    // Assert vector for FuncB is gone, vector for FuncA still present
}

// Test: RefreshFileVectors uses org vocabulary when org_id set
// Setup: Store org vocabulary, create file with functions
// Call: RefreshFileVectors with org_id
// Assert: Vectors tagged with correct vocab_version
func TestRefreshFileVectors_UsesOrgVocabulary(t *testing.T) {
    // Store an org vocabulary via the org_vocabulary table
    // Create FileContext with functions
    // Call RefreshFileVectors with orgID="test-org"
    // Assert: vectors have org_id set
    // Assert: vectors have vocab_version matching stored vocabulary's VersionHash
}

// Test: RefreshFileVectors uses per-repo vocabulary when no org_id
// Call: RefreshFileVectors with empty org_id
// Assert: Backward compatible behavior
func TestRefreshFileVectors_PerRepoVocab(t *testing.T) {
    // Create FileContext with functions
    // Call RefreshFileVectors with orgID=""
    // Assert: vectors created with empty org_id
    // Assert: vectors have empty vocab_version (per-repo mode)
}

// Test: RefreshFileVectors is idempotent (no changes on re-run)
// Setup: Run RefreshFileVectors on unchanged file
// Assert: RefreshResult all zeros
func TestRefreshFileVectors_Idempotent(t *testing.T) {
    // First call: RefreshFileVectors creates vectors and hashes
    // Second call: same FileContext, same source
    // Assert result.Added == 0, result.Modified == 0, result.Removed == 0
}
```

## Implementation Details

### 1. Add `DeleteByFile` to `SQLiteVectorStore`

**File:** `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/vectors/store.go`

Add a new method that deletes all vectors for a specific file within a repo. The deletion matches on the `file_path` column directly (not by ID prefix parsing), since the `file_path` column stores the file path for every vector record.

```go
// DeleteByFile removes all vectors for a specific file in a repository.
func (s *SQLiteVectorStore) DeleteByFile(ctx context.Context, repoID, filePath string) error {
    // Lock, then execute: DELETE FROM vectors WHERE repo_id = ? AND file_path = ?
}
```

Also add `DeleteByFile` to the `VectorStore` interface:

```go
// DeleteByFile removes all vectors for a specific file.
DeleteByFile(ctx context.Context, repoID, filePath string) error
```

### 2. Add `vocab_version` Column to Vectors Table

**File:** `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/vectors/store.go`

Add a migration in `initSchema` to add the `vocab_version TEXT DEFAULT ''` column to the vectors table. Follow the existing pattern used for the `org_id` migration:

```go
// Migration: add vocab_version if missing (existing DBs)
if _, err := s.db.Exec("SELECT vocab_version FROM vectors LIMIT 1"); err != nil {
    s.db.Exec("ALTER TABLE vectors ADD COLUMN vocab_version TEXT DEFAULT ''")
}
```

Update the `VectorRecord` struct to include the new field:

```go
type VectorRecord struct {
    // ... existing fields ...
    VocabVersion string `json:"vocab_version,omitempty"` // Vocabulary version hash this was embedded with
}
```

Update all `Store`, `StoreBatch`, `Get`, `Search`, `SearchByType`, and `SearchByOrg` methods to include `vocab_version` in their SQL queries. The INSERT statements should include the new column, and SELECT statements should scan it.

### 3. Define `RefreshResult` Type

**File:** `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/vectors/search.go`

```go
// RefreshResult summarizes the outcome of an incremental vector refresh.
type RefreshResult struct {
    Added    int      // Number of new embeddings created
    Modified int      // Number of embeddings updated
    Removed  int      // Number of embeddings deleted
    Errors   []string // Non-fatal errors encountered
}
```

### 4. Implement `RefreshFileVectors` on `SemanticSearch`

**File:** `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/vectors/search.go`

This is the core method. It needs access to both the main storage (for function hashes) and the vector store (for embeddings). The `SemanticSearch` struct currently holds only the `embedder` and `store` (vector store). To access function hash methods, the `SemanticSearch` struct needs a reference to the main storage.

**Option:** Add a `storage` field to `SemanticSearch` that implements the function hash interface. Define a minimal interface so there is no import cycle:

```go
// FunctionHashStore provides function hash operations needed for incremental updates.
// Implemented by storage.SQLiteStore (from internal/storage).
type FunctionHashStore interface {
    GetFunctionHashes(ctx context.Context, repoID, filePath string) (map[string]FunctionHashInfo, error)
    UpdateFunctionHashes(ctx context.Context, repoID, filePath string, hashes []FunctionHashInfo) error
    DeleteFunctionHashes(ctx context.Context, repoID, filePath string) error
    GetChangedFunctions(ctx context.Context, repoID, filePath string, current map[string]string) (added, modified, removed []string, err error)
}

// OrgVocabularyStore provides org vocabulary operations needed for incremental updates.
// Implemented by storage.SQLiteStore (from internal/storage).
type OrgVocabularyStore interface {
    GetOrgVocabulary(ctx context.Context, orgID string) (*VocabularyData, error)
}
```

Note: `FunctionHashInfo` is defined in the storage package (Section 1). To avoid import cycles, either re-define a compatible type in the vectors package, or use a shared types package. The simplest approach: define the interface using the storage package's types and have the wiring happen in the orchestrator where both packages are visible.

**Update `NewSemanticSearch`** to accept the additional stores:

```go
func NewSemanticSearch(embedder Embedder, store *SQLiteVectorStore, hashStore FunctionHashStore, vocabStore OrgVocabularyStore) *SemanticSearch
```

If `hashStore` or `vocabStore` are nil, `RefreshFileVectors` returns an error indicating incremental updates are not available.

**`RefreshFileVectors` method signature:**

```go
// RefreshFileVectors incrementally updates vector embeddings for a single file.
// It diffs function-level content hashes to determine which functions were added,
// modified, or removed, then updates only those embeddings.
func (s *SemanticSearch) RefreshFileVectors(ctx context.Context, repoID, filePath string, file *ctxpkg.FileContext, orgID string) (*RefreshResult, error)
```

**Algorithm (step by step):**

1. **Compute current function hashes.** Iterate over `file.Functions` and `file.Types`. For each, compute a content hash. The hash is SHA256 of the raw source code extracted from the file using `LineStart`/`LineEnd`. If raw source is not available, fall back to SHA256 of the concatenation of receiver, name, signature, and a representation of fields/behavior. Build a map of `qualifiedName:type` to hash string.

2. **Call `GetChangedFunctions`** with the current hash map. This returns `added`, `modified`, and `removed` string slices (each element is `"name:type"` format).

3. **Prepare the embedder.** If `orgID` is non-empty:
   - Load org vocabulary via `vocabStore.GetOrgVocabulary(ctx, orgID)`
   - Create a new `LocalEmbedder` instance
   - Call `ImportVocabulary` with the loaded vocabulary
   - Use this embedder for all embedding in this operation
   - Set `vocabVersion` to the vocabulary's `VersionHash`
   
   If `orgID` is empty:
   - Collect all documents from the file's functions and types
   - Build per-file vocabulary on a fresh embedder (or use the existing shared embedder -- for backward compatibility, build vocabulary from just this file's documents)
   - Set `vocabVersion` to empty string

4. **Process added functions.** For each name in `added`:
   - Find the corresponding `FunctionDef` or `TypeDef` in the file context
   - Build a document string using `buildFunctionDocument` or `buildTypeDocument`
   - Generate embedding using the prepared embedder
   - Create a `VectorRecord` with the standard ID format, set `OrgID` and `VocabVersion`
   - Store the vector via `store.Store(ctx, record)`
   - Record the `vector_id` (the record's ID) in the hash info

5. **Process modified functions.** For each name in `modified`:
   - Delete the old vector by its ID (reconstructed from the name pattern)
   - Generate new embedding (same as added)
   - Store new vector
   - Update the hash entry with new hash and vector_id

6. **Process removed functions.** For each name in `removed`:
   - Delete the vector by its ID
   - (Hash cleanup happens in the bulk update below)

7. **Update function hashes.** Call `UpdateFunctionHashes` with the full current set of hashes (including vector_id references for all functions in the file). This is a bulk upsert that replaces all entries for the file.

8. **Return `RefreshResult`** with counts.

### 5. Helper: Compute Function Content Hash

**File:** `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/vectors/search.go` (or a new helper file)

```go
// computeFunctionHash computes a SHA256 content hash for a function.
// Uses line range extraction from the source file when available,
// otherwise falls back to AST-derived content.
func computeFunctionHash(fn ctxpkg.FunctionDef) string {
    // Build a deterministic string from: receiver + name + signature + description + behavior summary
    // SHA256 hash it
    // Return hex string
}

// computeTypeHash computes a SHA256 content hash for a type definition.
func computeTypeHash(t ctxpkg.TypeDef) string {
    // Build from: name + kind + fields (sorted) + description
    // SHA256 hash it
}

// qualifyFunctionName returns the receiver-qualified name for a function.
// e.g., "Bar" for a standalone function, "(*Foo).Bar" for a method.
func qualifyFunctionName(fn ctxpkg.FunctionDef) string {
    if fn.Receiver != "" {
        return fn.Receiver + "." + fn.Name
    }
    return fn.Name
}
```

### 6. Cross-DB Transaction Consistency

Since `function_hashes` lives in the main storage DB and `vectors` lives in the vector DB, true atomic transactions across both are not possible. The design accepts this trade-off:

- **Vector insert succeeds, hash update fails:** On next refresh, the hash will mismatch (old hash vs. new source), triggering a redundant re-embed. The old vector is replaced by `INSERT OR REPLACE`. This is safe but wastes one extra embed operation.
- **Hash update succeeds, vector insert fails:** The hash points to a vector_id that does not exist. On next refresh, the hash matches (no change detected), so the missing vector is not recovered automatically. To handle this, `RefreshFileVectors` should verify vector existence for functions where hashes match but vectors are missing. This is a defensive check at the start.
- **Mitigation:** Process operations in order: (1) delete old vectors, (2) insert new vectors, (3) update hashes. If step 3 fails, step 1+2 results in slightly stale data that self-corrects on next refresh.

Log a warning when cross-DB operations partially fail, including which function and which operation failed.

### 7. Wire into the Orchestrator Refresh Flow

**File:** `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/orchestrator/manager.go`

The orchestrator's `RefreshFile` and `RefreshChanged` methods need to be updated to call `RefreshFileVectors` after the existing file re-analysis and hash update steps. The orchestrator has access to both the main storage and the semantic search service.

After the existing refresh logic:
```go
// Existing: re-analyze file, update file_hashes
// NEW: update vector embeddings incrementally
if s.semanticSearch != nil {
    result, err := s.semanticSearch.RefreshFileVectors(ctx, repoID, filePath, fileCtx, orgID)
    if err != nil {
        // Log warning but don't fail the refresh
        log.Printf("WARNING: vector refresh failed for %s: %v", filePath, err)
    }
}
```

The `orgID` for the file can be determined by checking if the repo belongs to any org (query the `org_repos` table).

### 8. Batch `refresh_changed` Support

For `refresh_changed` (which processes multiple files), call `RefreshFileVectors` for each changed file and aggregate results:

```go
// AggregateRefreshResult combines results from multiple file refreshes.
type AggregateRefreshResult struct {
    FilesProcessed int
    TotalAdded     int
    TotalModified  int
    TotalRemoved   int
    Errors         []string
}
```

## File Summary

| File | Action |
|------|--------|
| `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/vectors/store.go` | Add `DeleteByFile` method; add `vocab_version` column migration; update `VectorRecord` struct; update all SQL queries to include `vocab_version` |
| `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/vectors/search.go` | Add `RefreshFileVectors` method; add `RefreshResult` and `AggregateRefreshResult` types; add `FunctionHashStore` and `OrgVocabularyStore` interfaces; add hash computation helpers; update `NewSemanticSearch` constructor |
| `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/vectors/store_test.go` | Add `TestDeleteByFile` |
| `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/vectors/search_test.go` | Add `TestRefreshFileVectors_*` test functions (6 tests) |
| `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/orchestrator/manager.go` | Wire `RefreshFileVectors` into existing `RefreshFile` and `RefreshChanged` flows |