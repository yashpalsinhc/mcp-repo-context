# Section 7: Integration Tests

## Overview

End-to-end tests for the complete semantic search pipeline after all fixes. Covers auto-init, dimension handling, full round-trips, incremental indexing, server restart recovery, cross-repo isolation, and error scenarios.

## Dependencies

- All previous sections (01-06)

## Tests First

### File: `internal/integration/semantic_search_test.go` (new)

```
Test: Full round-trip — analyze, auto-index, search
- Setup: create temp Go repo with 5 functions (GetUser, CreateUser, UpdateUser, DeleteUser, ListOrders)
- Call: analyze_local(path) with MCP_AUTO_INDEX=true
- Assert: tool output contains "Auto-indexed 5 items"
- Call: semantic_search(repo_id, query="user management")
- Assert: GetUser, CreateUser, UpdateUser ranked highly
- Assert: ListOrders ranked lower (less relevant)

Test: Dimension consistency end-to-end
- Create server with default config
- Get embedder dimension: should be 256
- Get vector store dimension: should be 256
- Assert: they match

Test: Dimension migration clears stale vectors
- Create vector store with dim=256
- Insert vectors with dim=384 directly via SQL
- Create server (triggers migration)
- Assert: vectors cleared
- Assert: semantic search returns empty (needs re-index)

Test: Dimension validation rejects bad vectors
- Create vector store with dim=256
- Attempt to store 384-dim vector via StoreBatch
- Assert: error returned

Test: Index then search — full round-trip
- Analyze a Go repo with known functions
- Manually call index_repository(repo_id)
- Search for known function name
- Assert: found with good similarity score

Test: Error message — search unindexed repo
- Analyze repo but don't index
- Call semantic_search
- Assert: output contains "index_repository" suggestion

Test: Error message — index unanalyzed repo
- Call index_repository on repo that hasn't been analyzed
- Assert: output contains "analyze_repo" or "analyze_local"

Test: Auto-index on analyze (default true)
- Analyze repo without setting MCP_AUTO_INDEX
- Assert: vectors exist (auto-indexed by default)

Test: Auto-index disabled
- Set MCP_AUTO_INDEX=false
- Analyze repo
- Assert: no vectors (not auto-indexed)

Test: Incremental indexing — RefreshFile
- Analyze + index repo with file A containing [GetUser, CreateUser]
- Modify file A: remove CreateUser, add UpdateUser
- Call refresh_file for file A
- Search for "UpdateUser": found
- Search for "CreateUser": NOT found
- Search for "GetUser": still found

Test: Batch incremental — RefreshFiles
- Analyze + index repo with files A and B
- Modify both files
- Call refresh_changed (triggers RefreshFiles)
- Assert: vectors updated for both files

Test: Vector-ranked get_context_budgeted
- Analyze + index repo
- Call get_context_budgeted(query="user authentication", budget=4000)
- Assert: user-related functions ranked first
- Assert: output does NOT contain "Tip: Run index_repository"

Test: Keyword fallback get_context_budgeted
- Analyze repo but don't index
- Call get_context_budgeted(query="user")
- Assert: results returned (keyword ranking)
- Assert: output contains "Tip: Run index_repository"

Test: Re-indexing with force=true
- Analyze + index repo (count N)
- Modify repo, re-analyze (add new functions)
- Call index_repository(force=true)
- Assert: new count > N
- Assert: new functions searchable

Test: Vocabulary persistence round-trip
- Analyze + index repo (vocabulary stored)
- Query vocabularies table
- Assert: vocabulary exists with version=0
- Call RefreshFile
- Assert: version incremented to 1

Test: Server restart — vocabulary recovery
- Analyze + index repo
- Simulate restart: create new SemanticSearch with fresh embedder
- Search for known function
- Assert: vocabulary loaded from DB automatically
- Assert: results found (not using hash fallback)

Test: Cross-repo vocabulary isolation
- Index repo A (user-related functions)
- Index repo B (order-related functions)
- Search repo B for "order processing"
- Assert: repo B functions ranked high
- Assert: uses repo B's vocabulary (not repo A's)
- Search repo A for "user management"
- Assert: repo A functions ranked high

Test: Vocabulary staleness warning
- Index repo (version=0)
- RefreshFile 50 times
- Assert: warning logged about stale vocabulary
- Assert: suggests "Run index_repository(force=true)"

Test: Sort correctness at scale
- Create 5000 vectors with random similarities
- Search
- Assert: results sorted correctly (descending similarity)
- Assert: completes in <1 second (not O(n^2) bubble sort)

Test: VectorCount in tool output
- Index repo with 15 functions
- Assert: VectorCount returns 15
```

## Implementation Details

### 1. Test Fixture Setup

```go
func setupSemanticSearchFixtures(t *testing.T) (*server, string, string)
```

Returns: server instance, repo ID, temp directory

Creates:
- Temp Go repo with known function files
- Server with auto-init vector store
- Analyzes and optionally indexes the repo

### 2. Go Source Fixtures

Create temp Go files with distinct, searchable functions:

```go
// user_handlers.go
package handlers

func GetUser(id int) (*User, error) { /* retrieves user by ID from database */ }
func CreateUser(name string) (*User, error) { /* creates a new user account */ }
func UpdateUser(id int, name string) error { /* updates user profile information */ }
func DeleteUser(id int) error { /* removes user account permanently */ }

// order_handlers.go
package handlers

func ListOrders(userID int) ([]Order, error) { /* lists all orders for a user */ }
func CreateOrder(items []Item) (*Order, error) { /* creates a new purchase order */ }
```

### 3. Restart Simulation

```go
func simulateRestart(t *testing.T, vectorStorePath string, storagePath string) *server
```

Creates a completely new server instance with fresh embedder (empty vocabulary), pointing to the same SQLite files. This simulates what happens on actual server restart.

### 4. Assertion Helpers

```go
func assertVectorCount(t *testing.T, ss *SemanticSearch, repoID string, expected int)
func assertSearchFinds(t *testing.T, results []SearchResult, funcName string)
func assertSearchNotFinds(t *testing.T, results []SearchResult, funcName string)
func assertOutputContains(t *testing.T, output string, substring string)
func assertRankedBefore(t *testing.T, results []SearchResult, higher, lower string)
```

### 5. Test Infrastructure

- Each test uses isolated temp directories and SQLite files
- Tests that modify server config (MCP_AUTO_INDEX) use `t.Setenv`
- Cleanup via `t.Cleanup()`
- Tests run in parallel where possible (separate DB files)

## Error Handling

- Fixture setup failures: `t.Fatal` with descriptive message
- Analysis failures on synthetic Go files: indicates analyzer bug — fail test
- Tests that depend on timing (sort performance): use generous thresholds (10x expected)

## File Summary

| File | Action |
|------|--------|
| `internal/integration/semantic_search_test.go` | New: end-to-end integration tests |
| `internal/integration/semantic_fixtures_test.go` | New: test fixture setup and helpers |
