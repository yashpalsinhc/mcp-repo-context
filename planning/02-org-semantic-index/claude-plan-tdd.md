# TDD Plan: Org-Wide Semantic Index

Testing follows Go standard `testing` package patterns. Tests use real SQLite (temp files), consistent with existing test patterns in the codebase.

## Section 1: Function-Level Hash Tracking

### File: `internal/storage/sqlite_test.go` (extend)

```go
// Test: Store and retrieve function hashes for a file
// Setup: Create function hashes for 3 functions in a file
// Call: UpdateFunctionHashes, then GetFunctionHashes
// Assert: All 3 hashes returned with correct names, types, content hashes

// Test: GetChangedFunctions detects added functions
// Setup: Store hashes for func A and B. Pass current map with A, B, C
// Assert: added contains "C", modified and removed are empty

// Test: GetChangedFunctions detects modified functions
// Setup: Store hashes for func A with hash "abc". Pass current with A hash "def"
// Assert: modified contains "A"

// Test: GetChangedFunctions detects removed functions
// Setup: Store hashes for A, B, C. Pass current with only A, B
// Assert: removed contains "C"

// Test: DeleteFunctionHashes removes all entries for a file
// Setup: Store hashes for 3 functions. Call DeleteFunctionHashes
// Assert: GetFunctionHashes returns empty map

// Test: Receiver-qualified names are stored correctly
// Setup: Store "(*Foo).Bar" and "(*Baz).Bar" in same file
// Assert: Both retrieved as distinct entries

// Test: UpdateFunctionHashes is idempotent (upsert)
// Setup: Store hash for func A. Update with new hash
// Assert: Only one entry, with new hash
```

## Section 2: Org-Wide Vocabulary with Versioning

### File: `internal/vectors/embedder_test.go` (extend)

```go
// Test: ExportVocabulary returns current vocabulary state
// Setup: Build vocabulary from documents, then export
// Assert: VocabularyData has correct WordIDF map, DocCount, non-empty VersionHash

// Test: ImportVocabulary restores embedder state
// Setup: Export vocabulary, create new embedder, import
// Assert: New embedder produces identical embeddings for same input

// Test: VersionHash changes when vocabulary changes
// Setup: Build vocab from set A, export hash. Build from set B, export hash
// Assert: Hashes are different

// Test: VersionHash is stable for same vocabulary
// Setup: Build same vocabulary twice
// Assert: Same VersionHash both times
```

### File: `internal/vectors/search_test.go` (extend)

```go
// Test: BuildOrgVocabulary creates vocabulary from multiple repos
// Setup: Create 2 mock repos with known functions
// Call: BuildOrgVocabulary
// Assert: Stored vocabulary includes terms from both repos

// Test: GetOrgVocabulary loads stored vocabulary
// Setup: Build and store vocabulary
// Call: GetOrgVocabulary
// Assert: Returns vocabulary with correct VersionHash

// Test: MarkOrgVocabularyStale sets stale flag
// Setup: Store vocabulary
// Call: MarkOrgVocabularyStale
// Assert: Vocabulary has is_stale=true

// Test: BuildOrgVocabulary streams repos (memory check)
// Setup: Multiple repos
// Assert: Only one repo context loaded at a time (mock verifies load/discard pattern)
```

### File: `internal/storage/sqlite_test.go` (extend)

```go
// Test: Store and retrieve org vocabulary
// Test: org_vocabulary is_stale flag toggling
// Test: Delete org vocabulary on org deletion
```

## Section 3: Incremental Vector Updates

### File: `internal/vectors/search_test.go` (extend)

```go
// Test: RefreshFileVectors adds embeddings for new functions
// Setup: File with 0 existing vectors, 2 new functions
// Call: RefreshFileVectors
// Assert: 2 vectors created, RefreshResult.Added == 2

// Test: RefreshFileVectors updates embedding for modified function
// Setup: File with 1 existing vector, function source changed
// Call: RefreshFileVectors
// Assert: Old vector deleted, new vector created, RefreshResult.Modified == 1

// Test: RefreshFileVectors removes embedding for deleted function
// Setup: File with 2 vectors, one function removed
// Call: RefreshFileVectors
// Assert: Removed vector gone, remaining vector unchanged, RefreshResult.Removed == 1

// Test: RefreshFileVectors uses org vocabulary when org_id set
// Setup: Store org vocabulary, create file with functions
// Call: RefreshFileVectors with org_id
// Assert: Vectors tagged with correct vocab_version

// Test: RefreshFileVectors uses per-repo vocabulary when no org_id
// Call: RefreshFileVectors with empty org_id
// Assert: Backward compatible behavior

// Test: RefreshFileVectors is idempotent (no changes on re-run)
// Setup: Run RefreshFileVectors on unchanged file
// Assert: RefreshResult all zeros
```

### File: `internal/vectors/store_test.go` (extend)

```go
// Test: DeleteByFile removes all vectors for a file path
// Setup: Store 3 vectors for file "pkg/foo.go" and 2 for "pkg/bar.go"
// Call: DeleteByFile(repoID, "pkg/foo.go")
// Assert: Only bar.go vectors remain
```

## Section 4: Stale Embedding Cleanup

### File: `internal/vectors/search_test.go` (extend)

```go
// Test: CleanupStaleVectors removes vectors for absent functions
// Setup: File has vectors for A, B, C. Current functions are A, B only
// Call: CleanupStaleVectors with currentFunctions=["A", "B"]
// Assert: Returns 1 (removed C), vector for C gone

// Test: Cleanup on file deletion removes vectors and hashes
// Setup: Store vectors and function hashes for a file
// Simulate: File deletion via CleanupDeletedFiles
// Assert: All vectors and function hashes for file are gone

// Test: RemoveRepos cleans up vectors and marks vocabulary stale
// Setup: Org with 2 repos, both indexed. Remove one repo
// Assert: Vectors for removed repo gone, org vocabulary is_stale=true

// Test: DeleteOrg removes all vectors and vocabulary
// Setup: Indexed org with vocabulary
// Call: DeleteOrg
// Assert: All vectors gone, org_vocabulary entry gone
```

## Section 5: index_org MCP Tool

### File: `internal/mcp/tools_test.go` (extend)

```go
// Test: toolIndexOrg indexes all repos in org
// Setup: Register org with 2 repos, both analyzed
// Call: toolIndexOrg with org_id
// Assert: Both repos have vectors, vectors tagged with org_id

// Test: toolIndexOrg returns partial failure results
// Setup: Org with 2 repos, one fails during indexing
// Call: toolIndexOrg
// Assert: One repo indexed, one in failures list

// Test: toolIndexOrg with force re-indexes everything
// Setup: Pre-indexed org
// Call: toolIndexOrg with force=true
// Assert: All vectors regenerated (vocab_version updated)

// Test: toolIndexOrg returns error for unknown org
// Call: toolIndexOrg with non-existent org_id
// Assert: Error response
```

### File: `internal/org/indexer_test.go` (new)

```go
// Test: IndexOrg with bounded concurrency
// Setup: Org with 5 repos, concurrency=2
// Assert: Max 2 concurrent indexing operations

// Test: IndexOrg builds org vocabulary before indexing
// Assert: Vocabulary stored before any repo indexing begins

// Test: IndexOrg creates per-goroutine embedder instances
// Assert: No shared embedder mutation during concurrent indexing
```

## Section 6: Extend index_repository and SearchByOrg

### File: `internal/mcp/tools_test.go` (extend)

```go
// Test: toolIndexRepository with org_id uses org vocabulary
// Setup: Pre-built org vocabulary
// Call: toolIndexRepository with org_id
// Assert: Vectors tagged with org_id and vocab_version

// Test: toolIndexRepository without org_id uses per-repo vocabulary
// Call: toolIndexRepository without org_id
// Assert: Backward compatible, no org_id on vectors

// Test: toolIndexRepository with org_id but no vocabulary returns error
// Call: toolIndexRepository with org_id but no stored vocabulary
// Assert: Error suggesting to run index_org first
```

### File: `internal/vectors/search_test.go` (extend)

```go
// Test: SearchByOrg loads org vocabulary before embedding query
// Setup: Index org with org vocabulary
// Call: SearchByOrg
// Assert: Query embedded with org vocabulary (verified by checking result quality)
```

## Section 7: Integration Tests

### File: `internal/integration/org_semantic_test.go` (new)

```go
// Test: Full pipeline - index_org then search returns cross-repo results
// Test: Incremental update - change function, refresh, verify only that embedding changed
// Test: Delete function - refresh, verify embedding removed
// Test: Add repo to org - re-index, verify new repo included
// Test: Remove repo from org - verify vectors cleaned, vocabulary stale
// Test: Vocabulary consistency - same function text in different repos gets same embedding
```
