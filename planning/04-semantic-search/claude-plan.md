# Implementation Plan: Semantic Search & Vector Store

## Overview

Fix the broken semantic search experience by auto-initializing the vector store, fixing the dimension mismatch, improving error messages, adding auto-indexing on analyze, supporting incremental indexing, and enhancing get_context_budgeted with vector-based ranking.

## Current Architecture

### What Exists
- **Vector store** (`internal/vectors/store.go`): SQLite-based, stores vectors as JSON BLOBs, brute-force cosine similarity, bubble sort (O(n^2))
- **LocalEmbedder** (`internal/vectors/embedder.go`): TF-IDF with code-specific tokenization, default 256 dimensions, per-repo vocabulary
- **SemanticSearch** (`internal/vectors/search.go`): IndexRepository, IndexRepositoryWithOrg, SearchFunctions/Types/All/ByOrg
- **Server** (`cmd/mcp-server/main.go`): Creates vector store with dimension 384, uses `NewDefaultEmbedder()` (256 dim) — mismatch
- **get_context_budgeted** (`internal/mcp/tools.go`): Keyword-based scoring, no vector search integration
- **Token budgeter** (`internal/tokens/`): Greedy fill by score with summarize fallback

### What's Broken
1. **Dimension mismatch**: main.go creates vector store with dim=384, LocalEmbedder defaults to dim=256
2. **No auto-initialization**: if vector store path fails, semantic search silently disabled
3. **Error messages**: "Semantic search is not enabled" gives no guidance
4. **No auto-indexing**: users must manually call index_repository after analyze
5. **No incremental indexing**: full reindex every time, no way to update single function
6. **get_context_budgeted ignores vectors**: uses keyword scoring even when vectors are available
7. **Bubble sort O(n^2)**: sortBySimilarity and TopKSimilar use bubble sort, catastrophic at scale
8. **Vocabulary lost on restart**: after server restart, embedder has empty vocabulary, search uses hash fallback

## Section-by-Section Plan

### Section 1: Fix Dimension Mismatch & Auto-Init

**Goal:** Standardize dimensions, ensure the vector store is always available, fix sorting.

**Dimension fix:**
- Change `main.go` to use the embedder's dimension instead of hardcoding 384
- `NewDefaultEmbedder()` returns dimension 256
- Create vector store with `embedder.Dimension()` instead of literal 384

**Dimension migration for existing stores:**
- On startup, query one vector record from the DB, unmarshal it, check `len(vector)`
- If no vectors exist: no migration needed
- If existing dimension != embedder dimension: drop all vectors and log warning: "Vector dimension changed from {old} to {new}. All vectors cleared. Run index_repository to re-index."
- Add a `vector_metadata` table with key-value pairs, store `dimension` value for future checks

**Dimension validation in Store/StoreBatch:**
- Before storing any vector, validate `len(record.Vector) == s.dimension`
- Return error on mismatch — catches bugs at write time instead of silent search failures

**Auto-initialization:**
- Always create the vector store on server startup — don't make it conditional
- If the vector store path doesn't exist, create it (SQLite creates the file)
- If creation fails (permissions, disk), log error with actionable message and continue with nil (graceful degradation)
- Move vector store creation to a `initVectorStore()` function for clarity

**SemanticSearch always available:**
- Create SemanticSearch instance in NewServer unconditionally (store may still be nil for edge cases)
- Add a `IsAvailable() bool` method to SemanticSearch that checks if store is non-nil
- Tool handlers check `IsAvailable()` instead of `semanticSearch == nil`

**Fix bubble sort:**
- Replace `sortBySimilarity` in store.go with `sort.Slice` (O(n log n) introsort)
- Replace `TopKSimilar` bubble sort in similarity.go with `sort.Slice`
- These are one-line fixes but critical for performance at scale (50K vectors)

### Section 2: Fix Error Messages

**Goal:** Replace unhelpful "not enabled" errors with actionable guidance.

**Current error locations:**
- `toolSemanticSearch`: "Semantic search is not enabled. Initialize the server with a vector store."
- `toolIndexRepository`: "Semantic search is not enabled. Initialize the server with a vector store."

**New error messages:**

For `toolSemanticSearch` when no index exists:
```
Semantic search requires indexing first. Run:
  index_repository(repo_id: "{repoID}")

This generates vector embeddings for all functions and types in the repository,
enabling similarity-based search.
```

For `toolSemanticSearch` when vector store unavailable:
```
Vector store is not available. The server could not initialize the vector database at {path}.
Check file permissions and disk space, then restart the server.
```

For `toolIndexRepository` when vector store unavailable:
```
Vector store is not available. The server could not initialize the vector database.
Check the MCP_VECTOR_STORE_PATH environment variable and file permissions.
```

For `toolIndexRepository` when repo not analyzed:
```
Repository '{repoID}' has not been analyzed yet. Run:
  analyze_repo(repo_url: "{repoURL}")
or
  analyze_local(path: "/path/to/repo")
before indexing.
```

**Implementation:**
- Add an `ErrVectorStoreUnavailable` sentinel error
- Check for specific failure modes (no index vs store unavailable vs repo not analyzed)
- Return markdown-formatted suggestions

### Section 3: Auto-Index on Analyze

**Goal:** Auto-index when analyze_repo/analyze_local completes. Default ON since LocalEmbedder is free.

**Configuration:**
- New environment variable: `MCP_AUTO_INDEX=true|false` (default: **true**)
- Store config value in ServerConfig
- Read in main.go during startup

Default is true because LocalEmbedder is entirely local with no API costs. This aligns with the "just works" goal — users get semantic search without any manual steps.

**Implementation in toolAnalyzeRepo/toolAnalyzeLocal:**
- After successful analysis and storage, check if auto-index is enabled
- If enabled and semantic search is available:
  1. Get the newly stored RepoContext
  2. Call `semanticSearch.IndexRepository(ctx, repoCtx)`
  3. Log indexing result (count, duration)
  4. Append to tool output: "Auto-indexed {count} items in {duration}"
- If enabled but semantic search unavailable: log warning, don't fail the analyze

### Section 4: Incremental Indexing

**Goal:** Support updating vectors for individual files without full reindex.

**New methods on SemanticSearch:**

`RefreshFile(ctx, repoID, filePath string, fileCtx FileContext) error`
- Removes all vectors for the given file (both functions AND types)
- Re-indexes all functions and types in the updated file
- Uses existing vocabulary (loaded from store)

`RefreshFiles(ctx, repoID string, files map[string]FileContext) error`
- Batch version: loads vocabulary once, processes all changed files
- More efficient than calling RefreshFile N times for refresh_changed

`RemoveFunction(ctx, repoID, filePath, funcName string) error`
- Deletes the vector record for the specified function
- ID format: `{repoID}:func:{filePath}:{funcName}`

**DeleteByFile method on vector store:**
- Add `DeleteByFile(ctx, repoID, filePath string) error` to SQLiteVectorStore
- SQL: `DELETE FROM vectors WHERE repo_id = ? AND file_path = ?`
- Add index: `CREATE INDEX IF NOT EXISTS idx_vectors_file ON vectors(repo_id, file_path)`

**Vocabulary persistence:**
- Current vocabulary is built in-memory during IndexRepository and discarded
- Add `StoreVocabulary(ctx, repoID, vocab *Vocabulary) error` to vector store
- Add `LoadVocabulary(ctx, repoID) (*Vocabulary, error)` to vector store
- New `vocabularies` table: `repo_id TEXT PRIMARY KEY, vocabulary_json TEXT, idf_json TEXT, version INT DEFAULT 0, updated_at DATETIME`
- During IndexRepository: store vocabulary after building it
- During incremental indexing: load existing vocabulary, increment version
- Cache loaded vocabulary in SemanticSearch struct to avoid repeated DB reads

**Vocabulary staleness policy:**
- Track `version` counter: incremented on each incremental update
- After 50 incremental updates (configurable), log warning: "Vocabulary for {repoID} is {version} versions old. Run index_repository(force=true) for optimal results."
- `index_repository force=true` always rebuilds vocabulary from scratch, resets version to 0

**Vocabulary loading for search (server restart fix):**
- When search methods are called, check if embedder has vocabulary loaded for the target repo
- If not (empty vocabulary after restart), load from `vocabularies` table
- This fixes the "vocabulary lost on restart" bug where search used hash fallback
- Cache vocabulary per-repo in SemanticSearch struct

**Integration with refresh_file/refresh_changed tools:**
- After `refresh_file` updates the context, if semantic search is available AND repo has been indexed:
  1. Load vocabulary for the repo (from cache or DB)
  2. Call `semanticSearch.RefreshFile(ctx, repoID, filePath, fileCtx)`
  3. This keeps vectors in sync without full reindex
- After `refresh_changed`, use `RefreshFiles` for batch processing

### Section 5: Vector-Ranked get_context_budgeted

**Goal:** Use vector similarity scores for ranking in get_context_budgeted when available.

**Current flow:**
1. Extract keywords from query
2. Score functions by keyword matches (name, description, signature, summary)
3. Sort by score, greedy fill by token budget

**New flow:**
1. Check if repo has vectors indexed (check vector count for repoID)
2. If indexed:
   a. Load vocabulary for the target repo (for correct query embedding)
   b. Run semantic search for the query → get ranked results with similarity scores
   c. Convert to ScoredItem list with similarity as Score
   d. For functions not in semantic results but in repo context, assign score 0
   e. Use these scores for budgeter
3. If NOT indexed:
   a. Fall back to current keyword-based scoring (unchanged)
   b. Append note: "Tip: Run index_repository to enable semantic ranking for better results"

**Score normalization:**
- Cosine similarity range is [-1, 1], but search already filters to similarity > 0
- So effective range for scores is (0, 1]
- Keyword scores are 0.0-N (unnormalized)
- When using vector ranking, use similarity directly (already normalized)
- No need to combine keyword + vector — vector replaces keyword when available

**Tiered budget support:**
- Add guidance in tool description about budget tiers:
  - 2K: Quick — function signatures + one-line summaries
  - 4K: Standard — signatures + behavior summaries + side effects
  - 8K: Thorough — full details including callers, callees, DB queries
- The budgeter already handles this via SummarizeFunction fallback at lower budgets
- No code change needed for tiers — just documentation

### Section 6: Profiling & Monitoring

**Goal:** Add timing/profiling to vector operations for performance monitoring.

**Timing instrumentation:**
- Add `time.Since` logging to:
  - `IndexRepository`: total time, per-function embedding time, batch store time
  - `Search*` methods: query embedding time, vector load time, similarity time, sort time
  - `RefreshFile`: vocabulary load time, embedding time, store time
- Log at INFO level with operation name and duration

**Vector count endpoint:**
- Add `VectorCount(ctx, repoID) (int, error)` method if not already present
- Expose in tool output: "Repository has {count} indexed vectors"
- Helps users understand indexing state

**Brute-force performance documentation:**
- Add comments in search.go documenting expected performance:
  - <1K vectors: <10ms
  - 1K-10K vectors: 10-100ms
  - 10K-50K vectors: 100ms-1s (with O(n log n) sort fix)
  - >50K vectors: consider alternatives
- Document in tool descriptions

### Section 7: Integration Tests

**Goal:** End-to-end tests for the fixed semantic search pipeline.

**Test scenarios:**
1. Auto-init: server starts with vector store available without explicit config
2. Dimension consistency: embedder and store use same dimension
3. Dimension migration: existing store with wrong dimension is detected and cleared
4. Dimension validation: StoreBatch rejects wrong-dimension vectors
5. Index then search: full round-trip — analyze, index, search, verify results
6. Error messages: verify helpful messages for unindexed repo, unavailable store
7. Auto-index on analyze: with MCP_AUTO_INDEX=true, vectors created after analyze
8. Incremental indexing: refresh_file updates vectors for changed functions AND types
9. Batch incremental: refresh_changed updates vectors for multiple files
10. Vector-ranked budgeted context: indexed repo gets vector-scored results
11. Keyword fallback: unindexed repo gets keyword-scored results with tip
12. Re-indexing: force=true clears and re-indexes correctly, rebuilds vocabulary
13. Vocabulary persistence: stored vocabulary reused for incremental indexing
14. Vocabulary loaded on search after server restart
15. Cross-repo vocabulary isolation: searching repo B loads repo B's vocabulary
16. Vocabulary staleness warning after N incremental updates
17. Sort correctness: verify O(n log n) sort produces correct ordering

## Error Handling

- Vector store init failure: log error, continue without semantic search (graceful degradation)
- Dimension mismatch on existing store: drop vectors, log warning, suggest re-indexing
- Dimension mismatch on write: return error (validation in Store/StoreBatch)
- Auto-index failure: log error, don't fail the analyze operation
- Vocabulary not found for incremental index: rebuild vocabulary from current repo
- Semantic search unavailable: all tools return helpful error messages
- Vocabulary staleness: log warning after 50 incremental updates

## Performance Considerations

- Brute-force search is O(n*d) — acceptable for <10K vectors
- Sort fixed from O(n^2) bubble sort to O(n log n) introsort
- Vocabulary persistence avoids rebuilding on every incremental update
- Vocabulary caching in SemanticSearch avoids repeated DB reads
- Auto-indexing is on by default (LocalEmbedder is free, no API costs)
- Incremental indexing (RefreshFile) is much faster than full reindex for single-file changes
- Batch incremental (RefreshFiles) loads vocabulary once for multiple files
