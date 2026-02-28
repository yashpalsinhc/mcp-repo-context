# Section 6: Integration Tests

## Overview

End-to-end tests for the full org search pipeline. These tests exercise the complete flow: store repos, register org, index vectors, call search_org, and verify results. Covers all search modes, edge cases, and degraded operation.

## Dependencies

- All previous sections (01-05)

## Tests First

### File: `internal/integration/org_search_test.go` (new)

```
Test: Full pipeline — store, index, search_org hybrid
- Setup:
  - Create 3 repos with distinct Go source files:
    - repo-a (user-service): GetUser, CreateUser, DeleteUser functions
    - repo-b (order-service): GetOrder, CreateOrder functions
    - repo-c (auth-service): ValidateUser, LoginHandler functions
  - Analyze and store all 3 via GoAnalyzer + SQLiteStore
  - Register as org "test-org" with all 3 repos
  - Index vectors for all repos with org association
- Call: search_org with org_id="test-org", query="User", search_type="hybrid"
- Assert: results include GetUser, CreateUser, DeleteUser (from repo-a) and ValidateUser (from repo-c)
- Assert: results do NOT include GetOrder, CreateOrder (no "User" match)
- Assert: output is valid markdown with header, numbered results, detail_refs
- Assert: functions appearing in both keyword AND semantic results have higher ranking

Test: Re-indexing preserves FTS5 consistency
- Setup: store repo-a with functions [GetUser, CreateUser]
- Search for "User" — assert 2 results
- Re-store repo-a with functions [GetUser, UpdateUser, DeleteUser] (CreateUser removed, 2 added)
- Search for "User" — assert 3 results (GetUser, UpdateUser, DeleteUser)
- Search for "Create" — assert 0 results (CreateUser was removed)
- Verify: no stale FTS5 entries from the first indexing

Test: FTS5 special characters in query
- Store repo with function "errorHandler" and "panicRecovery"
- Search for: error OR panic (literal query, not FTS5 boolean)
- Assert: no FTS5 syntax error
- Assert: treated as phrase match (may return 0 results since exact phrase unlikely)
- Search for: "error
- Assert: no error (unmatched quote handled)
- Search for: error*
- Assert: no error

Test: Keyword-only mode (no vectors indexed)
- Setup: store repos, register org, do NOT index vectors
- Call: search_org with search_type="keyword", query="User"
- Assert: results returned from FTS5
- Assert: no error about missing vectors

Test: Semantic-only mode
- Setup: store repos, register org, index vectors
- Call: search_org with search_type="semantic", query="user authentication handler"
- Assert: results returned based on semantic similarity
- Assert: results sorted by similarity

Test: Hybrid with repo_ids filter
- Setup: org with 3 repos (a, b, c), all indexed
- Call: search_org with repo_ids=["repo-a", "repo-c"], query="User"
- Assert: results only from repo-a and repo-c
- Assert: no results from repo-b

Test: Hybrid degraded — semantic unavailable
- Setup: store repos, register org, do NOT index vectors
- Call: search_org with search_type="hybrid"
- Assert: returns keyword-only results
- Assert: output contains warning about semantic search unavailable

Test: Token budget truncation
- Setup: org with 10+ repos, each with 5+ functions matching query
- Call: search_org with token_budget=300, query="Get"
- Assert: output fits within ~300 tokens
- Assert: truncation message "... and N more results" present
- Assert: "Token budget:" footer present

Test: detail_ref round-trip
- Call search_org to get results with detail_refs
- Parse the first detail_ref string
- Assert: format is "func|repoID|filePath|funcName"
- Call ExpandDetailRef to parse it
- Use parsed values to call get_function_context
- Assert: function context returned successfully (function exists)

Test: Unknown org returns clear error
- Call: search_org with org_id="nonexistent-org-xyz"
- Assert: error contains "not found"

Test: Empty query returns error
- Call: search_org with query=""
- Assert: error about empty query

Test: Org with no repos returns empty results
- Register org "empty-org" with no repos
- Call: search_org with org_id="empty-org"
- Assert: empty results (or error about no repos)

Test: max_results limits output
- Setup: org with 20+ matching functions
- Call: search_org with max_results=3
- Assert: at most 3 results in output
```

## Implementation Details

### 1. Test Fixture Setup

Create a helper that generates synthetic Go source files and analyzes them:

```go
func setupOrgSearchFixtures(t *testing.T) (*SQLiteStore, *SemanticSearch, *OrgStore, string)
```

This helper:
1. Creates temp directories for 3 service repos
2. Writes Go source files with known function names and signatures
3. Analyzes each via `goAnalyzer.AnalyzeDirectory()`
4. Stores contexts via `SQLiteStore.StoreRepoContext()`
5. Registers org via org store
6. Returns store instances and orgID

**Service fixtures:**

**repo-a (user-service):**
```go
// handlers.go
package handlers

func GetUser(id int) (*User, error) { /* retrieves user by ID */ }
func CreateUser(name string) (*User, error) { /* creates new user */ }
func DeleteUser(id int) error { /* deletes user by ID */ }
```

**repo-b (order-service):**
```go
// handlers.go
package handlers

func GetOrder(id int) (*Order, error) { /* retrieves order */ }
func CreateOrder(userID int) (*Order, error) { /* creates order */ }
```

**repo-c (auth-service):**
```go
// handlers.go
package handlers

func ValidateUser(token string) (*Claims, error) { /* validates JWT */ }
func LoginHandler(username, password string) (string, error) { /* handles login */ }
```

### 2. Vector Indexing Helper

For tests requiring semantic search:

```go
func indexOrgVectors(t *testing.T, semanticSearch *SemanticSearch, store *SQLiteStore, orgID string, repoIDs []string)
```

Indexes all repos with org association using `IndexRepositoryWithOrg`.

### 3. Test Infrastructure

- Use real SQLite (temp files) consistent with existing test patterns
- Each test creates its own temp directory and DB files
- Tests that need semantic search set up the vector store alongside main store
- Cleanup via `t.Cleanup()` for temp files

### 4. Assertion Helpers

```go
func assertSearchResultContains(t *testing.T, output string, funcName string)
func assertSearchResultNotContains(t *testing.T, output string, funcName string)
func assertTokenCount(t *testing.T, output string, maxTokens int)
func assertValidDetailRef(t *testing.T, ref string)
```

These helpers check:
- Function name appears in formatted output
- Output fits within token budget (using chars/4 estimation)
- Detail ref has valid pipe-separated format

## Error Handling

- Fixture creation failure: `t.Fatal` with descriptive message
- Analysis failure on synthetic files: indicates analyzer bug — fail test
- Vector indexing failure: skip semantic tests with `t.Skip("vector indexing unavailable")`

## File Summary

| File | Action |
|------|--------|
| `internal/integration/org_search_test.go` | New: end-to-end integration tests |
| `internal/integration/org_search_helpers_test.go` | New: test fixture setup and assertion helpers |
