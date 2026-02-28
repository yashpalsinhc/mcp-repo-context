Now I have all the context needed. Let me produce the section content.

# Section 7: Integration Tests

## Overview

This section adds end-to-end integration tests that verify the complete org-wide semantic indexing pipeline. These tests exercise the full stack -- from org vocabulary building through indexing, incremental updates, stale cleanup, and cross-repo search -- using real SQLite databases (temp files) and the actual `LocalEmbedder`.

**Dependencies:** This section depends on ALL previous sections (01 through 06) being implemented. It does not introduce new production code; it only creates a new test file.

## File to Create

- `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/integration/org_semantic_test.go` (new)

## Background

### Architecture Summary

The system uses two separate SQLite databases:

1. **Main storage DB** (`internal/storage/sqlite.go`) -- holds repos, files, functions, types, `file_hashes`, `org_vocabulary`, orgs, and org_repos tables.
2. **Vector DB** (`internal/vectors/store.go`) -- holds the `vectors` table (with `org_id`, `vocab_version` columns).

Key components exercised by these integration tests:

- **`SemanticSearch`** (`internal/vectors/search.go`) -- `IndexRepository`, `IndexRepositoryWithOrg`, `SearchByOrg`, `RefreshFileVectors`, `BuildOrgVocabulary`, `CleanupStaleVectors`, `GetOrgVocabulary`
- **`SQLiteVectorStore`** (`internal/vectors/store.go`) -- `StoreBatch`, `SearchByOrg`, `DeleteByFile`, `DeleteByOrg`, `CountByOrg`
- **`LocalEmbedder`** (`internal/vectors/embedder.go`) -- `BuildVocabulary`, `Embed`, `ExportVocabulary`, `ImportVocabulary`
- **`VocabularyData`** (`internal/vectors/vocabulary.go`) -- vocabulary serialization and version hashing
- **`SQLiteStore`** (`internal/storage/sqlite.go`) -- `UpdateFunctionHashes`, `GetFunctionHashes`, `GetChangedFunctions`, `DeleteFunctionHashes`, org vocabulary CRUD
- **Org Manager** (`internal/org/manager.go`) -- `Register`, `AddRepos`, `RemoveRepos`, `Delete`
- **Org Indexer** (`internal/org/indexer.go`) -- `IndexOrg` with bounded concurrency

### Test Data Strategy

Each test builds minimal `RepoContext` objects with known functions and types. Function documents are built using the existing `buildFunctionDocument` helper (name + signature + description + behavior + steps + side effects + file path). Using real embedder instances with real vocabulary ensures that the embeddings are consistent and searchable.

### Test Infrastructure

- Use `t.TempDir()` for SQLite database files (auto-cleaned by Go testing)
- Create real `SQLiteVectorStore` and `SQLiteStore` instances per test
- Create real `LocalEmbedder` instances (no mocking -- TF-IDF is deterministic and fast)
- Build small `RepoContext` objects inline with 2-5 functions each
- Use Go `testing.T` subtests for related scenarios

## Tests

### File: `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/integration/org_semantic_test.go`

This file contains the following test functions. Each test description below includes setup, action, and assertions.

```go
package integration

import (
    "context"
    "testing"

    ctxpkg "github.com/yashpalc/mcp-repo-context/internal/context"
    "github.com/yashpalc/mcp-repo-context/internal/storage"
    "github.com/yashpalc/mcp-repo-context/internal/vectors"
)
```

#### Test 1: Full Pipeline -- index_org then SearchByOrg returns cross-repo results

```go
// TestFullPipeline_IndexOrgThenSearch verifies the end-to-end flow:
// 1. Register an org with 2 repos
// 2. Build org vocabulary from both repos
// 3. Index both repos with the org vocabulary
// 4. SearchByOrg for a term that exists in repo A
// 5. SearchByOrg for a term that exists in repo B
// 6. Assert both searches return results from the correct repos
// 7. Assert vector counts match expected totals
//
// Setup:
//   - Repo A has functions: "CreateUser" (handles user creation, DB insert),
//     "ValidateEmail" (validates email format)
//   - Repo B has functions: "SendNotification" (sends email notifications, HTTP call),
//     "FormatMessage" (formats notification messages)
//   - Org "test-org" contains both repos
//
// Key assertions:
//   - CountByOrg returns total functions+types across both repos
//   - Search for "user creation database" returns CreateUser from repo A
//   - Search for "notification email" returns SendNotification from repo B
//   - All vectors have org_id set to "test-org"
//   - All vectors have vocab_version matching the org vocabulary version hash
func TestFullPipeline_IndexOrgThenSearch(t *testing.T) {
    // Implementation: create temp DBs, build repos, call IndexOrg or equivalent,
    // then call SearchByOrg and verify results
}
```

#### Test 2: Incremental Update -- change function, refresh, verify only that embedding changed

```go
// TestIncrementalUpdate_ModifyFunction verifies that RefreshFileVectors
// correctly detects a modified function and updates only its embedding.
//
// Setup:
//   - Index a repo with 3 functions in one file (FuncA, FuncB, FuncC)
//   - Store initial function hashes via UpdateFunctionHashes
//   - Record the vector IDs for all 3 functions
//
// Action:
//   - Modify FuncB's description/behavior (change the RepoContext in memory)
//   - Call RefreshFileVectors for that file
//
// Assertions:
//   - RefreshResult.Modified == 1, Added == 0, Removed == 0
//   - FuncA and FuncC vectors are unchanged (same vector ID, same content)
//   - FuncB vector is updated (new embedding reflecting changed content)
//   - Function hash for FuncB is updated in the function_hashes table
//   - Total vector count for the repo is still 3
func TestIncrementalUpdate_ModifyFunction(t *testing.T) {}
```

#### Test 3: Delete Function -- refresh, verify embedding removed

```go
// TestIncrementalUpdate_DeleteFunction verifies that RefreshFileVectors
// removes the embedding for a function that no longer exists in the file.
//
// Setup:
//   - Index a repo with 3 functions in one file
//   - Store function hashes for all 3
//
// Action:
//   - Remove FuncC from the FileContext (simulate deletion)
//   - Call RefreshFileVectors for that file
//
// Assertions:
//   - RefreshResult.Removed == 1, Added == 0, Modified == 0
//   - Vector for FuncC is gone (Get by ID returns nil)
//   - Vectors for FuncA, FuncB still exist
//   - Function hash for FuncC is removed from function_hashes table
//   - Total vector count decreased by 1
func TestIncrementalUpdate_DeleteFunction(t *testing.T) {}
```

#### Test 4: Add Repo to Org -- re-index, verify new repo included

```go
// TestAddRepoToOrg_ReindexIncludesNewRepo verifies that adding a repo
// to an org and re-indexing includes the new repo's functions in the
// org-wide search.
//
// Setup:
//   - Register org with 1 repo (repoA), index the org
//   - Verify search returns only repoA results
//
// Action:
//   - Add repoB to the org (via org store AddRepos)
//   - Re-index the org (which rebuilds vocabulary and re-embeds everything)
//
// Assertions:
//   - Org vocabulary is rebuilt (version_hash changed because new documents)
//   - SearchByOrg returns results from both repoA and repoB
//   - CountByOrg reflects vectors from both repos
//   - All vectors have the new vocab_version
func TestAddRepoToOrg_ReindexIncludesNewRepo(t *testing.T) {}
```

#### Test 5: Remove Repo from Org -- vectors cleaned, vocabulary stale

```go
// TestRemoveRepoFromOrg_CleanupAndStale verifies that removing a repo
// from an org cleans up its vectors and marks vocabulary as stale.
//
// Setup:
//   - Register org with 2 repos, index both
//   - Verify CountByOrg includes vectors from both repos
//
// Action:
//   - Call RemoveRepos to remove repoB from the org
//   - The removal flow should delete vectors for repoB and mark vocab stale
//
// Assertions:
//   - Vectors for repoB with org_id="test-org" are gone
//   - Vectors for repoA with org_id="test-org" still exist
//   - Org vocabulary is_stale flag is true
//   - SearchByOrg no longer returns results from repoB
func TestRemoveRepoFromOrg_CleanupAndStale(t *testing.T) {}
```

#### Test 6: Vocabulary Consistency -- same function text gets same embedding

```go
// TestVocabularyConsistency_SameTextSameEmbedding verifies that when
// using a shared org vocabulary, identical function text in different
// repos produces identical embeddings.
//
// Setup:
//   - Two repos each containing a function with identical name, signature,
//     description, and behavior (e.g., "ParseJSON" in both)
//   - Build org vocabulary from both repos
//
// Action:
//   - Index both repos with the org vocabulary
//   - Retrieve the vectors for the identical functions from both repos
//
// Assertions:
//   - The two embedding vectors are exactly equal (same vocabulary =>
//     same TF-IDF weights => same L2-normalized output)
//   - Cosine similarity between them is 1.0 (within floating point tolerance)
func TestVocabularyConsistency_SameTextSameEmbedding(t *testing.T) {}
```

#### Test 7: Partial Failure -- one repo fails, others succeed

```go
// TestIndexOrg_PartialFailure verifies that if one repo fails during
// org-wide indexing, the other repos are still indexed successfully.
//
// Setup:
//   - Register org with 3 repos
//   - RepoA and RepoC have valid contexts
//   - RepoB has a context that triggers an error (e.g., nil context or
//     a context loader that returns an error for that repo ID)
//
// Action:
//   - Call IndexOrg for the org
//
// Assertions:
//   - IndexOrgResult.ReposIndexed == 2
//   - IndexOrgResult.ReposFailed == 1
//   - IndexOrgResult.Failures contains repoB's ID and error message
//   - Vectors exist for repoA and repoC
//   - No vectors exist for repoB
func TestIndexOrg_PartialFailure(t *testing.T) {}
```

#### Test 8: Vocabulary Version Tracking -- detect stale vectors

```go
// TestVocabularyVersionTracking verifies that vectors store the
// vocab_version they were embedded with, and that a vocabulary change
// produces a different version hash.
//
// Setup:
//   - Build org vocabulary from repos A and B, record version_hash_v1
//   - Index all repos, all vectors get vocab_version = version_hash_v1
//
// Action:
//   - Add repoC to the org
//   - Rebuild org vocabulary (now includes repoC's terms)
//   - Record version_hash_v2
//
// Assertions:
//   - version_hash_v1 != version_hash_v2 (vocabulary changed)
//   - Existing vectors still have vocab_version = version_hash_v1 (not yet re-embedded)
//   - After re-indexing the org, all vectors have vocab_version = version_hash_v2
func TestVocabularyVersionTracking(t *testing.T) {}
```

## Helper Functions

The test file should include a small set of helper functions to reduce boilerplate.

```go
// makeTestRepo builds a minimal RepoContext with the given functions.
// Each function gets a basic signature, description, and behavior.
func makeTestRepo(repoID string, files map[string][]ctxpkg.FunctionDef) *ctxpkg.RepoContext {
    // Build RepoContext with Files map populated from the input.
    // Each file path becomes a FileContext entry with the given functions.
}

// makeFunction creates a FunctionDef with sensible defaults for testing.
func makeFunction(name, description string, sideEffects []string) ctxpkg.FunctionDef {
    // Returns a FunctionDef with Name, Signature ("func " + name + "()"),
    // Description, Behavior (Summary = description, Steps from side effects),
    // SideEffects, IsPublic=true, LineStart=1, LineEnd=10.
}

// setupTestDBs creates temporary main storage DB and vector store DB,
// returning the store, vector store, and a cleanup function.
func setupTestDBs(t *testing.T) (*storage.SQLiteStore, *vectors.SQLiteVectorStore) {
    t.Helper()
    // Uses t.TempDir() for automatic cleanup.
    // Creates and returns real SQLiteStore and SQLiteVectorStore instances.
}

// assertVectorCount checks that the vector count for a repo/org matches expected.
func assertVectorCount(t *testing.T, store *vectors.SQLiteVectorStore, repoID string, expected int) {
    t.Helper()
    // Calls store.Count or store.CountByOrg and checks against expected.
}
```

## Implementation Notes

1. **No mocking required.** The `LocalEmbedder` is deterministic (TF-IDF with sorted vocabulary), fast (sub-millisecond per embedding), and has no external dependencies. Use real instances throughout.

2. **Temp databases.** Each test should create its own pair of SQLite databases in `t.TempDir()`. This ensures test isolation and automatic cleanup.

3. **Small test data.** Each repo should have 2-5 functions. This keeps tests fast while still exercising cross-repo and incremental scenarios.

4. **Build tag consideration.** If these tests are slow or depend on all sections being complete, consider adding a `//go:build integration` build tag so they can be run separately via `go test -tags integration ./internal/integration/...`. However, if they complete in under a few seconds (expected given the small data sizes), a build tag is not necessary.

5. **Cross-DB consistency.** Tests 2 and 3 (incremental updates) should verify that both the vector DB and the main storage DB (function_hashes) are consistent after operations. This validates the cross-DB consistency trade-off documented in Section 3.

6. **Vector ID format.** Vectors use the ID format `{repoID}:{type}:{filePath}:{name}` (e.g., `"repo-a:func:pkg/user.go:CreateUser"`). Tests should use `store.Get(ctx, expectedID)` to verify specific vectors exist or are removed.

7. **Floating point comparisons.** When comparing embeddings for equality (Test 6), use a small epsilon (e.g., 1e-10) rather than exact equality to account for floating point arithmetic.