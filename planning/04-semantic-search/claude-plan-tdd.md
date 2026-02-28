# TDD Plan: Semantic Search & Vector Store

## Section 1: Fix Dimension Mismatch & Auto-Init

### File: `internal/vectors/store_test.go`
```
Test: Vector store uses embedder dimension, not hardcoded
- Create embedder with dim=256
- Create vector store with embedder.Dimension()
- Assert: store.dimension == 256

Test: Dimension migration detects wrong-dimension vectors
- Create store with dim=256
- Insert a vector with 384 floats directly via SQL
- Run dimension check on startup
- Assert: all vectors cleared, warning logged

Test: Dimension migration skips when no vectors exist
- Create store with dim=256 (empty)
- Run dimension check
- Assert: no warning, no cleanup

Test: StoreBatch rejects wrong-dimension vectors
- Create store with dim=256
- Attempt to store vector with 384 floats
- Assert: error returned about dimension mismatch

Test: Store rejects wrong-dimension single vector
- Same as above for single Store() method

Test: Vector metadata table stores dimension
- Create store, verify vector_metadata table exists
- Assert: dimension key has correct value

Test: Sort uses O(n log n) not bubble sort
- Create 1000 results with random similarities
- Sort them
- Assert: correctly sorted descending
- Assert: completes in reasonable time (<10ms)

Test: IsAvailable returns true when store is set
- Create SemanticSearch with store
- Assert: IsAvailable() == true

Test: IsAvailable returns false when store is nil
- Create SemanticSearch with nil store
- Assert: IsAvailable() == false
```

## Section 2: Fix Error Messages

### File: `internal/mcp/tool_error_messages_test.go`
```
Test: Semantic search on unindexed repo shows indexing instructions
- Setup: repo analyzed but not indexed
- Call: semantic_search(repo_id, query)
- Assert: output contains "index_repository"

Test: Semantic search when store unavailable shows path info
- Setup: server with nil vector store
- Call: semantic_search
- Assert: output mentions "not available" and "file permissions"

Test: Index repository when store unavailable shows config guidance
- Setup: server with nil vector store
- Call: index_repository
- Assert: output mentions "MCP_VECTOR_STORE_PATH"

Test: Index repository on unanalyzed repo suggests analyze first
- Setup: server with vector store but repo not analyzed
- Call: index_repository(repo_id)
- Assert: output contains "analyze_repo" and "analyze_local"
```

## Section 3: Auto-Index on Analyze

### File: `internal/mcp/tool_auto_index_test.go`
```
Test: Auto-index creates vectors after analyze_local
- Setup: MCP_AUTO_INDEX=true, semantic search available
- Call: analyze_local(path)
- Assert: vectors exist for the analyzed repo
- Assert: output contains "Auto-indexed"

Test: Auto-index off by env var skips indexing
- Setup: MCP_AUTO_INDEX=false
- Call: analyze_local(path)
- Assert: no vectors exist

Test: Auto-index failure doesn't fail analyze
- Setup: MCP_AUTO_INDEX=true, vector store with write error
- Call: analyze_local(path)
- Assert: analyze succeeds, warning logged

Test: Auto-index default is true
- Setup: no MCP_AUTO_INDEX env var set
- Assert: config.AutoIndex == true
```

## Section 4: Incremental Indexing

### File: `internal/vectors/incremental_test.go`
```
Test: RefreshFile updates vectors for changed functions
- Index repo, verify vectors for file A
- Call RefreshFile with updated file A (new function added)
- Assert: new function has vector
- Assert: removed function has no vector

Test: RefreshFile updates both functions and types
- Index repo with file containing function + type
- Call RefreshFile with updated file
- Assert: both function and type vectors updated

Test: RefreshFiles batch processes multiple files
- Index repo with 3 files
- Call RefreshFiles with 2 updated files
- Assert: vectors updated for both files
- Assert: third file vectors unchanged

Test: DeleteByFile removes all vectors for a file
- Store vectors for 3 functions in file A
- Call DeleteByFile for file A
- Assert: 0 vectors remain for file A
- Assert: other files' vectors unchanged

Test: Vocabulary stored during IndexRepository
- Call IndexRepository
- Assert: vocabularies table has entry for repo
- Assert: vocabulary_json is non-empty

Test: Vocabulary loaded for incremental indexing
- IndexRepository (stores vocabulary)
- RefreshFile (should load vocabulary, not rebuild)
- Assert: vocabulary loaded from DB (mock/spy)

Test: Vocabulary staleness warning after 50 updates
- Set version to 49
- Call RefreshFile
- Assert: warning logged about stale vocabulary

Test: Vocabulary cached in SemanticSearch
- Load vocabulary once
- Call RefreshFile twice
- Assert: vocabulary loaded from DB only once (cached)

Test: Force reindex rebuilds vocabulary
- IndexRepository (version 0)
- RefreshFile x 5 (version 5)
- IndexRepository force=true
- Assert: version reset to 0, new vocabulary stored

Test: Missing vocabulary triggers rebuild
- Delete vocabulary from DB
- Call RefreshFile
- Assert: vocabulary rebuilt from current repo context
```

## Section 5: Vector-Ranked get_context_budgeted

### File: `internal/mcp/tool_budgeted_vector_test.go`
```
Test: Indexed repo uses vector ranking
- Setup: repo analyzed and indexed
- Call: get_context_budgeted(repo_id, query, budget=4000)
- Assert: results ranked by semantic similarity (not keyword)

Test: Unindexed repo falls back to keyword ranking
- Setup: repo analyzed but NOT indexed
- Call: get_context_budgeted(repo_id, query)
- Assert: results ranked by keyword score
- Assert: output contains "Tip: Run index_repository"

Test: Vector ranking loads correct vocabulary
- Setup: two repos indexed with different vocabularies
- Call: get_context_budgeted for repo B
- Assert: repo B's vocabulary used for query embedding

Test: Cosine similarity scores used correctly
- Setup: indexed repo with known function similarities
- Call: get_context_budgeted
- Assert: higher-similarity functions ranked first

Test: Budget tiers described in tool output
- Call: get_context_budgeted with budget=2000
- Assert: output focuses on signatures and summaries (not full details)
```

## Section 6: Profiling & Monitoring

### File: `internal/vectors/profiling_test.go`
```
Test: IndexRepository logs timing
- Call IndexRepository
- Assert: log contains "IndexRepository" with duration

Test: Search logs timing
- Call SearchFunctions
- Assert: log contains "Search" with duration

Test: VectorCount returns correct count
- Store 5 vectors for repo A
- Assert: VectorCount(repo-a) == 5

Test: VectorCount returns 0 for unindexed repo
- Assert: VectorCount(unknown-repo) == 0
```

## Section 7: Integration Tests

### File: `internal/integration/semantic_search_test.go`
```
Test: Full round-trip — analyze, auto-index, search
- Analyze a Go repo
- Assert: vectors auto-created (MCP_AUTO_INDEX=true default)
- Search for known function
- Assert: found with good similarity

Test: Dimension consistency end-to-end
- Create server
- Assert: embedder.Dimension() == vectorStore.dimension

Test: Incremental update preserves search quality
- Analyze + index repo
- Search for "GetUser" → found
- Refresh file with new function "UpdateUser"
- Search for "UpdateUser" → found
- Search for "GetUser" → still found

Test: Server restart + search works
- Analyze + index repo
- Simulate restart: create new SemanticSearch with fresh embedder
- Search for known function
- Assert: vocabulary loaded from DB, results found

Test: Cross-repo vocabulary isolation
- Index repo A (user functions)
- Index repo B (order functions)
- Search repo B for "order"
- Assert: uses repo B's vocabulary (not repo A's)

Test: Error cascade — unanalyzed repo
- Call index_repository on non-existent repo
- Assert: helpful error about analyze_repo

Test: Re-index rebuilds everything
- Index repo, get count N
- Modify repo (add functions), re-analyze
- Index with force=true
- Assert: new count > N, vocabulary rebuilt
```
