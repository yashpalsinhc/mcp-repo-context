I now have enough context. Let me produce the section content.

# Section 4: Stale Embedding Cleanup

## Overview

This section implements cleanup logic for orphaned vector embeddings. When files are deleted, functions are removed, repos are detached from an org, or an org is deleted, the corresponding vectors and function hashes must be cleaned up. Additionally, vocabulary must be marked stale when org membership changes.

**Dependencies:** This section depends on:
- **Section 1 (Function Hash Tracking):** Provides the `function_hashes` table and `DeleteFunctionHashes` method used when cleaning up after file deletions.
- **Section 3 (Incremental Vector Updates):** Provides `DeleteByFile` on the vector store and the `RefreshFileVectors` method that this cleanup logic complements.

**Vocabulary staleness** uses `MarkOrgVocabularyStale` from Section 2 (Org Vocabulary), which sets `is_stale=true` on the `org_vocabulary` table row for an org.

## Files to Create or Modify

| File | Action |
|------|--------|
| `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/vectors/search.go` | Add `CleanupStaleVectors` method to `SemanticSearch` |
| `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/vectors/store.go` | Add `DeleteByFileAndNames` method to `SQLiteVectorStore` (delete vectors matching specific function names within a file) |
| `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/org/manager.go` | Extend `RemoveRepos` and `Delete` to trigger vector and vocabulary cleanup |
| `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/storage/sqlite.go` | Extend `CleanupDeletedFiles` (or the flow that calls it) to also delete vectors and function hashes for removed files |
| `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/vectors/search_test.go` | Add tests for stale cleanup |

## Tests

All tests go in `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/vectors/search_test.go` (extend existing file).

Tests use real SQLite temp databases, consistent with the existing test patterns in this codebase.

```go
// Test: CleanupStaleVectors removes vectors for absent functions
// Setup: Store vectors for functions A, B, C in a file for a given repo.
//        Store corresponding function hashes for A, B, C.
// Call: CleanupStaleVectors(ctx, repoID, filePath, currentFunctions=["A", "B"])
// Assert: Returns 1 (one vector removed for C).
//         Vector for C is gone from the store. Vectors for A and B remain.
//         Function hash for C is also removed.

// Test: Cleanup on file deletion removes vectors and hashes
// Setup: Store vectors and function hashes for a file (e.g., "pkg/foo.go") with 3 functions.
// Simulate: Call the file deletion cleanup flow (CleanupDeletedFiles or equivalent).
// Assert: All vectors for "pkg/foo.go" are gone (verified via store query).
//         All function_hashes entries for that file are gone.

// Test: RemoveRepos cleans up vectors and marks vocabulary stale
// Setup: Register an org with 2 repos. Index both repos (store vectors tagged with org_id).
//        Store org vocabulary for the org.
// Action: Remove one repo from the org.
// Assert: All vectors for the removed repo with that org_id are gone.
//         Vectors for the remaining repo are untouched.
//         org_vocabulary row has is_stale=true.

// Test: DeleteOrg removes all vectors and vocabulary
// Setup: Register an org. Index repos with org_id. Store org vocabulary.
// Call: DeleteOrg(orgID)
// Assert: All vectors with that org_id are gone (DeleteByOrg already exists).
//         org_vocabulary entry for that org_id is deleted.
```

## Implementation Details

### 1. CleanupStaleVectors on SemanticSearch

Add a new method to `SemanticSearch` in `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/vectors/search.go`:

```go
// CleanupStaleVectors removes vectors for functions that no longer exist in a file.
// currentFunctions is the list of fully-qualified function/type names currently present.
// Returns the count of vectors removed.
func (s *SemanticSearch) CleanupStaleVectors(ctx context.Context, repoID, filePath string, currentFunctions []string) (int, error)
```

Logic:
1. Build a set from `currentFunctions` for O(1) lookup.
2. Query all vectors for the given `repoID` and `filePath` from the vector store. The existing vector ID format is `{repoID}:{type}:{filePath}:{name}`, so query vectors where the ID starts with `{repoID}:func:{filePath}:` or `{repoID}:type:{filePath}:`.
3. For each vector found, extract the function/type name from the vector record's `Name` field.
4. If the name is not in the `currentFunctions` set, delete that vector.
5. Also delete the corresponding entry from the `function_hashes` table (requires access to the main storage DB -- pass a storage reference or a callback).
6. Return the count of deleted vectors.

An alternative approach is to query vectors by `file_path` column directly since the `vectors` table has a `file_path` column. This avoids parsing the ID string. The query would be:

```sql
SELECT id, name, type FROM vectors WHERE repo_id = ? AND file_path = ?
```

Then for each vector whose name is not in `currentFunctions`, call `Delete(ctx, id)`.

The `SemanticSearch` struct needs access to the main storage for deleting function hashes. This can be done by:
- Adding a `storage` field to `SemanticSearch` (a `FunctionHashDeleter` interface with just `DeleteFunctionHashByName`)
- Or having the caller handle function hash cleanup separately

The simpler approach: have `CleanupStaleVectors` only handle vector deletion, and have the caller (orchestrator) handle function hash cleanup. This keeps `SemanticSearch` focused on vectors.

### 2. DeleteByFileAndNames on SQLiteVectorStore

Add a helper method to the vector store for targeted deletion:

```go
// DeleteByFileAndNames deletes vectors matching specific names within a file.
// This is more targeted than DeleteByFile (from Section 3) which removes ALL vectors for a file.
func (s *SQLiteVectorStore) DeleteByFileAndNames(ctx context.Context, repoID, filePath string, names []string) (int, error)
```

This uses:
```sql
DELETE FROM vectors WHERE repo_id = ? AND file_path = ? AND name IN (?, ?, ...)
```

Returns the number of rows deleted. This is used by `CleanupStaleVectors` internally.

### 3. Hook into File Deletion Flow

The existing file deletion flow happens when `CleanupDeletedFiles` is called (from the file tracker in `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/storage/sqlite.go`). This currently removes entries from the `file_hashes` table for files that no longer exist on disk.

Extend this flow so that when a file is detected as deleted:
1. Call `DeleteByFile(ctx, repoID, filePath)` on the vector store to remove all vectors for that file.
2. Call `DeleteFunctionHashes(ctx, repoID, filePath)` on the main storage to remove function hash entries.

The cleanest integration point is in the orchestrator layer (`/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/orchestrator/manager.go`) where the refresh flow coordinates between storage, analysis, and vectors. After `CleanupDeletedFiles` returns the list of deleted file paths, iterate over them and call the vector and hash cleanup.

### 4. Hook into Repo Removal from Org

When `RemoveRepos` is called on the org manager, add cleanup logic. The current `RemoveRepos` in `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/org/manager.go` simply delegates to `store.RemoveRepos` which deletes rows from `org_repos`.

Extend the `manager.RemoveRepos` method to also:
1. For each removed repo, delete vectors that are tagged with the org_id AND that repo_id. This requires a new method on the vector store:

```go
// DeleteByOrgAndRepo removes vectors for a specific repo within an org.
func (s *SQLiteVectorStore) DeleteByOrgAndRepo(ctx context.Context, orgID, repoID string) error
```

SQL: `DELETE FROM vectors WHERE org_id = ? AND repo_id = ?`

2. Call `MarkOrgVocabularyStale(ctx, orgID)` on the semantic search (or directly on storage). This sets `is_stale=true` on the `org_vocabulary` row, signaling that the next `index_org` call should rebuild vocabulary.

The org manager needs access to the vector store and vocabulary storage. This can be done by:
- Adding a `SemanticSearch` (or a cleanup-focused interface) to the org manager constructor.
- Or having the orchestrator manager handle the cleanup after calling `orgManager.RemoveRepos`.

The orchestrator approach is cleaner since it already coordinates between org, storage, and vectors.

### 5. Hook into Org Deletion

When `DeleteOrg` is called on the org manager, extend it to:
1. Call `DeleteByOrg(orgID)` on the vector store -- this method already exists on `SQLiteVectorStore`.
2. Delete the `org_vocabulary` row for that org from the main storage DB:

```sql
DELETE FROM org_vocabulary WHERE org_id = ?
```

This should be a method on the main storage: `DeleteOrgVocabulary(ctx, orgID) error`.

Again, the orchestrator is the right place to coordinate this since the org manager does not currently hold a reference to the vector store.

### 6. Orchestrator Coordination Pattern

The orchestrator manager (`/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/orchestrator/manager.go`) is where all cleanup flows converge. It has access to:
- The main storage (SQLiteStore)
- The vector store (SQLiteVectorStore) via SemanticSearch
- The org manager

Add or extend these orchestrator methods:

```go
// CleanupDeletedFileVectors removes vectors and function hashes for files
// that were deleted during a refresh cycle.
// deletedFiles is the list of file paths returned by CleanupDeletedFiles.
func (m *manager) CleanupDeletedFileVectors(ctx context.Context, repoID string, deletedFiles []string) error

// RemoveReposFromOrg removes repos from an org and cleans up their vectors.
func (m *manager) RemoveReposFromOrg(ctx context.Context, orgID string, repoIDs []string) error

// DeleteOrgWithCleanup deletes an org and all associated vectors and vocabulary.
func (m *manager) DeleteOrgWithCleanup(ctx context.Context, orgID string) error
```

These methods orchestrate the calls across storage, vector store, and org manager in the correct order.

## Key Design Decisions

1. **Cleanup is best-effort for cross-DB operations.** Since vectors live in a separate SQLite database from function hashes, there is no cross-DB transaction guarantee. If vector deletion succeeds but hash deletion fails, the next refresh will see a hash mismatch and re-embed (safe but wasteful). This is acceptable.

2. **Vocabulary staleness is a flag, not automatic rebuild.** When repos are added or removed from an org, the vocabulary is marked stale rather than immediately rebuilt. The rebuild happens on the next `index_org` call. This avoids expensive vocabulary rebuilds during simple org membership changes.

3. **Orchestrator owns cleanup coordination.** The org manager and storage layer stay focused on their own data. The orchestrator ties cleanup across stores together. This keeps each layer testable in isolation.

4. **Vector store methods for targeted deletion.** `DeleteByFileAndNames` enables surgical cleanup of individual stale functions without touching other vectors in the same file. `DeleteByOrgAndRepo` enables cleanup when a repo is removed from an org without affecting the repo's standalone vectors (if any).

## Checklist

- [ ] Add `CleanupStaleVectors` method to `SemanticSearch`
- [ ] Add `DeleteByFileAndNames` method to `SQLiteVectorStore`
- [ ] Add `DeleteByOrgAndRepo` method to `SQLiteVectorStore`
- [ ] Add `DeleteOrgVocabulary` method to main storage
- [ ] Extend orchestrator to clean up vectors on file deletion
- [ ] Extend orchestrator to clean up vectors on repo removal from org
- [ ] Extend orchestrator to clean up vectors and vocabulary on org deletion
- [ ] Mark org vocabulary stale when repos are added or removed
- [ ] Write tests for all cleanup scenarios