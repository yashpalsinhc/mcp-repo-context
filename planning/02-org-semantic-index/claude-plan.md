# Implementation Plan: Org-Wide Semantic Index

## Overview

This plan extends the existing vector/embedding layer to support org-wide semantic indexing. The system currently supports per-repo indexing via `index_repository` and has partial org support (`SearchByOrg`, `IndexRepositoryWithOrg`). This plan completes the org-wide story by adding: an `index_org` MCP tool, org-wide vocabulary building with versioning, function-level incremental updates, and stale embedding cleanup.

## Current Architecture

### What Exists

The codebase has these relevant components:

**Vector Store** (`internal/vectors/`):
- `VectorRecord` struct with `OrgID` field, stored in SQLite vectors table
- `SQLiteVectorStore` with methods: Store, StoreBatch, Search, SearchByOrg, DeleteByRepo, DeleteByOrg, CountByOrg
- Vectors table has `org_id TEXT NOT NULL DEFAULT ''` column with index

**Embedder** (`internal/vectors/`):
- `LocalEmbedder` - offline TF-IDF with L2 normalization, 256-dim default
- `BuildVocabulary(documents []string)` builds IDF table from corpus
- `Embed(text)`, `EmbedBatch(texts)`, `EmbedCode(code)` methods
- Vocabulary capped at 10,000 words
- Uses `vocabulary[word] = i % dimension` for slot assignment -- vocabulary changes invalidate all existing embeddings

**Semantic Search** (`internal/vectors/search.go`):
- `SemanticSearch` struct with `IndexRepository`, `IndexRepositoryWithOrg`, `SearchFunctions`, `SearchByOrg`
- Document construction: functions → name+signature+description+behavior+steps+sideeffects+filepath

**Organization** (`internal/org/`):
- `Manager` interface with AnalyzeOrg (concurrent, bounded goroutines, partial failure support)
- `Store` interface with SaveOrg, GetOrg, AddRepos, RemoveRepos
- SQLite-backed with orgs and org_repos tables

**File Tracking** (`internal/storage/`):
- `file_hashes` table: (repo_id, file_path, hash SHA256, last_analyzed, file_size)
- `FileTracker` interface: GetChangedFiles, UpdateFileHashes, CleanupDeletedFiles

**Important: Two separate SQLite databases** exist:
- Main storage DB (`internal/storage/sqlite.go`) - repos, files, functions, types, file_hashes, orgs
- Vector DB (`internal/vectors/store.go`) - vectors table. Opened independently but `NewSQLiteVectorStoreWithDB` can share a `*sql.DB`

### What's Missing

1. No `index_org` MCP tool - cannot index entire org in one call
2. Vocabulary is per-repo - no org-wide vocabulary for consistent cross-repo embeddings
3. No vocabulary versioning - vocabulary changes silently invalidate all existing embeddings
4. No function-level hash tracking - can't detect which functions changed within a file
5. No incremental vector updates - `refresh_file` doesn't update embeddings
6. No stale embedding cleanup when files/functions are removed

## Implementation Sections

### Section 1: Function-Level Hash Tracking

**Goal:** Track per-function and per-type content hashes so we can detect exactly which functions changed when a file is modified.

**New Table in main storage DB:** `function_hashes`
```
function_hashes (
    repo_id TEXT NOT NULL,
    file_path TEXT NOT NULL,
    name TEXT NOT NULL,         -- Fully qualified: includes receiver, e.g. "(*Foo).Bar"
    type TEXT NOT NULL,          -- "function" or "type"
    content_hash TEXT NOT NULL,  -- SHA256 of raw source code (line range from file)
    vector_id TEXT,              -- Application-level reference to vectors table ID (NOT a real FK -- vectors are in a separate DB)
    PRIMARY KEY (repo_id, file_path, name, type)
)
```

**Name Qualification:** Function names must include receiver to avoid collisions. `(*Foo).Bar` and `(*Baz).Bar` are distinct entries even in the same file.

**Hash Computation:** SHA256 of the raw source code of the function (extracted from the file using line_start/line_end from FunctionDef). This correctly detects code changes regardless of analyzer upgrades. If source extraction is not available, fall back to SHA256 of `receiver + name + signature + body` from the AST.

**New Storage Methods on SQLiteStore:**
- `GetFunctionHashes(ctx, repoID, filePath) (map[string]FunctionHashInfo, error)` - key is "name:type"
- `UpdateFunctionHashes(ctx, repoID, filePath string, hashes []FunctionHashInfo) error` - bulk upsert in transaction
- `DeleteFunctionHashes(ctx, repoID, filePath string) error` - remove all hashes for a file
- `GetChangedFunctions(ctx, repoID, filePath string, current map[string]string) (added, modified, removed []string, error)`

**FunctionHashInfo type:**
```go
type FunctionHashInfo struct {
    Name        string // Fully qualified with receiver
    Type        string // "function" or "type"
    ContentHash string
    VectorID    string // Application-level ref to vectors table
}
```

**Migration:** Add next available migration to create `function_hashes` table with index on (repo_id, file_path).

### Section 2: Org-Wide Vocabulary with Versioning

**Goal:** Build a unified vocabulary from all repos in an org so embeddings are consistent across repos. Track vocabulary version to detect when re-embedding is needed.

**Critical Design Decision:** Because the LocalEmbedder assigns word-to-dimension slots based on vocabulary order, any vocabulary change invalidates all existing embeddings. Therefore:
- When `index_org` is called, it always rebuilds vocabulary AND re-embeds all vectors
- Incremental updates (`refresh_file`) use the stored org vocabulary without rebuilding
- When repos are added/removed from org, vocabulary is marked stale

**New Table in main storage DB:** `org_vocabulary`
```
org_vocabulary (
    org_id TEXT PRIMARY KEY,
    vocabulary_json TEXT NOT NULL,  -- Serialized word → IDF mapping
    version_hash TEXT NOT NULL,     -- SHA256 of sorted vocabulary keys (for change detection)
    doc_count INTEGER,
    built_at TIMESTAMP,
    repo_count INTEGER,
    is_stale BOOLEAN DEFAULT 0     -- Marked stale when repos added/removed
)
```

**Add to vectors table:** `vocab_version TEXT DEFAULT ''` column. Each vector record stores the vocabulary version_hash it was embedded with.

**New Interface:** `VocabularyAwareEmbedder`
```go
type VocabularyAwareEmbedder interface {
    Embedder // existing: Embed, EmbedBatch, Dimension
    ExportVocabulary() *VocabularyData
    ImportVocabulary(vocab *VocabularyData) error
}
```

**VocabularyData type:**
```go
type VocabularyData struct {
    WordIDF     map[string]float64
    DocCount    int
    BuiltAt     time.Time
    VersionHash string // SHA256 of sorted keys
}
```

**New Methods on SemanticSearch:**
- `BuildOrgVocabulary(ctx, orgID string, repoIDs []string) (*VocabularyData, error)` - stream documents repo-by-repo (not all at once), build vocabulary, store result
- `GetOrgVocabulary(ctx, orgID string) (*VocabularyData, error)` - load stored vocabulary
- `MarkOrgVocabularyStale(ctx, orgID string) error` - called when repos added/removed

**Memory Optimization:** When building vocabulary, stream documents per-repo rather than loading all RepoContexts simultaneously. For each repo: load context, extract document strings, discard context, continue.

**Embedder Lifecycle:** Do NOT mutate the shared embedder instance. Instead:
- Create a new `LocalEmbedder` for each org-indexing operation
- Import vocabulary into the new instance
- Use it for all embedding in that operation
- Discard after operation completes

### Section 3: Incremental Vector Updates

**Goal:** When `refresh_file` or `refresh_changed` is called, update only the embeddings for functions/types that actually changed.

**Integration Point:** The existing `refresh_file` flow in the manager currently:
1. Re-analyzes the file (re-extracts functions, types, etc.)
2. Updates file_hashes table
3. Does NOT update vectors

**New Flow for refresh_file:**
1. Re-analyze the file (existing)
2. Update file_hashes (existing)
3. Compute function-level hashes from raw source code
4. Call `GetChangedFunctions()` to diff against stored hashes
5. For **added** functions: generate embedding, store vector, store hash
6. For **modified** functions: delete old vector, generate new embedding, store vector, update hash
7. For **removed** functions: delete vector, delete hash

**Vocabulary Handling for Incremental Updates:**
- If org_id is set: load org vocabulary, create fresh embedder with that vocabulary
- Verify vocabulary version_hash matches vectors being updated (if not, the vectors are stale -- log warning but still update the changed function with current vocabulary)
- If org_id not set: use per-repo vocabulary (backward compatible)

**New Method on SemanticSearch:**
- `RefreshFileVectors(ctx, repoID, filePath string, file *FileContext, orgID string) (*RefreshResult, error)`

**RefreshResult type:**
```go
type RefreshResult struct {
    Added    int
    Modified int
    Removed  int
    Errors   []string
}
```

**Transaction Handling:** Wrap the delete-old + insert-new + update-hash operations for each function in a single transaction where possible. Since function_hashes is in the main DB and vectors are in a separate DB, document the cross-DB consistency trade-off: if vector insert succeeds but hash update fails, the hash will mismatch on next refresh, triggering a redundant re-embed (safe but wasteful).

**New Method on VectorStore:**
- `DeleteByFile(ctx, repoID, filePath string) error` - delete all vectors for a specific file (matches by ID prefix `{repoID}:{type}:{filePath}:`)

**Batch refresh_changed:** For each changed file, call RefreshFileVectors. Aggregate results.

### Section 4: Stale Embedding Cleanup

**Goal:** Remove orphaned embeddings when files, functions, or repos are removed from an org.

**File Deletion:** When `CleanupDeletedFiles` runs (existing flow), also:
- Delete all vectors for the deleted file via `DeleteByFile`
- Delete all function_hashes for the deleted file

**Repo Removal from Org:** When `RemoveRepos` is called on org manager:
- Delete all vectors tagged with org_id + repo_id
- Delete function_hashes for the repo
- Mark org vocabulary as stale (`MarkOrgVocabularyStale`)

**Org Deletion:** When `DeleteOrg` is called:
- Call `DeleteByOrg(orgID)` on vector store (already exists)
- Delete org_vocabulary entry

**New Method on SemanticSearch:**
- `CleanupStaleVectors(ctx, repoID, filePath string, currentFunctions []string) (int, error)` - delete vectors for functions no longer present in the file

### Section 5: index_org MCP Tool

**Goal:** New MCP tool that indexes all repos in an org with a single call.

**Tool Definition:**
```
Name: "index_org"
Description: "Index all repositories in an organization for semantic search"
Parameters:
  - org_id (string, required): Organization ID
  - force (boolean, optional, default false): Force re-index even if already indexed
  - concurrency (integer, optional, default 3): Max concurrent repo indexing
```

**Handler Flow:**
1. Validate org_id exists via org store
2. Get list of repo IDs from org
3. Load repo contexts (stream per-repo for vocabulary building)
4. Build org-wide vocabulary from all repos (Section 2) - this always happens
5. For each repo (with bounded concurrency, per-repo transactions):
   a. Create fresh embedder with org vocabulary
   b. Generate embeddings for all functions/types
   c. Store vectors tagged with org_id and vocab_version
   d. Update function_hashes
6. Return summary: repos indexed, total vectors, failures

**Concurrency:** Reuse the bounded goroutine pattern from `org.Analyzer` (semaphore + WaitGroup). Each goroutine creates its own embedder instance with the shared vocabulary (read-only after building).

**Partial Failure:** If a repo fails to index, log the error, continue with remaining repos. Return the failure list in the response.

**Manager Method:**
- `IndexOrg(ctx, orgID string, force bool, concurrency int) (*IndexOrgResult, error)` on the orchestrator Manager

**IndexOrgResult type:**
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

### Section 6: Extend index_repository Tool and SearchByOrg

**Goal:** Add optional `org_id` parameter to `index_repository` and ensure `SearchByOrg` loads the correct vocabulary.

**Changes to index_repository tool definition:**
- Add `org_id` (string, optional) parameter
- When provided: load org vocabulary, create fresh embedder, use it for embedding, tag vectors with org_id and vocab_version
- When absent: behave exactly as before (per-repo vocabulary)

**SearchByOrg Vocabulary Fix:** When `SearchByOrg` is called, load the org vocabulary before embedding the search query. This ensures the query embedding uses the same vocabulary as the stored vectors.

**Handler Changes:**
1. Parse optional `org_id` from arguments
2. If org_id provided:
   a. Validate org exists
   b. Load org vocabulary (error if not available - suggest running index_org first)
   c. Create fresh embedder with org vocabulary
   d. Index repo with org-tagged vectors
3. If org_id absent: call existing `IndexRepository`

### Section 7: Integration Tests

**Goal:** End-to-end tests verifying the complete org-wide indexing pipeline.

**Test Scenarios:**
1. `index_org` indexes all repos in org, search returns cross-repo results
2. `refresh_file` after function change updates only that function's embedding
3. `refresh_file` after function deletion removes its embedding
4. Org vocabulary produces consistent embeddings across repos (same function in different repos gets same embedding)
5. `index_repository` with org_id tags vectors correctly
6. Partial failure: one repo fails, others succeed
7. Stale cleanup: remove repo from org, vectors cleaned up, vocabulary marked stale
8. Vocabulary version tracking: vectors embedded with old vocabulary are detectable

**Test Infrastructure:**
- Use real SQLite (temp files)
- Mock or small test repos with known functions
- Verify vector counts, vocabulary versions, and search results

## File Summary

| File | Action |
|------|--------|
| `internal/storage/migrations/NNN_function_hashes.sql` | New: function_hashes and org_vocabulary tables |
| `internal/storage/sqlite.go` | Add function hash CRUD and org vocabulary CRUD methods |
| `internal/vectors/embedder.go` | Add VocabularyAwareEmbedder interface, ExportVocabulary/ImportVocabulary on LocalEmbedder |
| `internal/vectors/store.go` | Add DeleteByFile method, add vocab_version column |
| `internal/vectors/search.go` | Add BuildOrgVocabulary, RefreshFileVectors, CleanupStaleVectors, fix SearchByOrg vocab loading |
| `internal/vectors/vocabulary.go` | New: VocabularyData type, version hash computation |
| `internal/org/manager.go` | Add IndexOrg method to Manager interface |
| `internal/org/indexer.go` | New: Org indexing logic with bounded concurrency and per-goroutine embedders |
| `internal/orchestrator/manager.go` | Wire IndexOrg, extend refresh flows for vector updates |
| `internal/mcp/server.go` | Register index_org tool, add org_id to index_repository |
| `internal/mcp/tools.go` | Add toolIndexOrg handler, update toolIndexRepository |
| `internal/integration/org_semantic_test.go` | New: Integration tests |

## Implementation Order

1. Section 1: Function-level hash tracking (foundation)
2. Section 2: Org-wide vocabulary with versioning (foundation)
3. Section 3: Incremental vector updates (depends on 1, 2)
4. Section 4: Stale embedding cleanup (depends on 1, 3)
5. Section 5: index_org MCP tool (depends on 2, 3, 4)
6. Section 6: Extend index_repository and SearchByOrg (depends on 2)
7. Section 7: Integration tests (depends on all)

## Risk Assessment

**Low Risk:** Function hash tracking and stale cleanup are well-scoped additions to existing patterns.

**Medium Risk:** Vocabulary versioning is the most complex aspect. The key insight is that vocabulary changes = full re-embed. This simplifies the design: `index_org` always does a full re-embed with fresh vocabulary, while `refresh_file` uses the stored vocabulary for incremental updates.

**Low Risk:** index_org tool follows the established pattern of AnalyzeOrg (bounded concurrency, partial failures).

**Known Limitation:** Brute-force vector search (loads all vectors into memory for cosine similarity) is adequate for expected org sizes (< 100k vectors). ANN/vector DB is a future optimization if needed.
