# Section 2: Org-Scoped Search Methods

## Overview

Add org-scoped search methods via a new `OrgSearcher` interface. These methods query across all repos in an organization using the org_repos table subquery pattern. Includes SearchFunctionsOrg (FTS5-backed), SearchByConceptOrg, and HybridSearchOrg.

## Dependencies

- Section 1: FTS5 Virtual Tables (SearchFunctionsFTS method, FunctionRef.RepoID)

## Tests First

### File: `internal/storage/sqlite_org_search_test.go` (new)

```
Test: OrgSearcher interface satisfied by SQLiteStore
- Compile-time check: var _ OrgSearcher = (*SQLiteStore)(nil)
- Assert: compiles without error

Test: SearchFunctionsOrg returns results across multiple repos
- Store 3 repos in same org:
  - repo-a: functions GetUser, CreateUser
  - repo-b: functions GetOrder, CreateOrder
  - repo-c: functions GetProduct, UpdateUser
- Register org with all 3 repos
- SearchFunctionsOrg for "User"
- Assert: returns GetUser (repo-a), CreateUser (repo-a), UpdateUser (repo-c)
- Assert: no Order/Product functions returned
- Assert: each result has correct RepoID

Test: SearchFunctionsOrg returns empty for no-match query
- Store repos, register org
- SearchFunctionsOrg for "nonexistent_xyz"
- Assert: empty results, no error

Test: SearchByConceptOrg returns cross-repo results
- Store 2 repos:
  - repo-a: functions with "handler" concept (HTTP handler)
  - repo-b: functions with "handler" concept (event handler)
- Register org
- SearchByConceptOrg for "handler"
- Assert: results from both repos
- Assert: each result has RepoID populated

Test: SearchByConceptOrg with unknown concept
- SearchByConceptOrg for "nonexistent_concept"
- Assert: empty results, no error

Test: HybridSearchOrg combines FTS and concept results
- Store repo with function "AuthHandler" that matches both FTS "Auth" and concept "handler"
- Store repo with function "GetUser" that matches FTS "User" but not concept "handler"
- Register org
- HybridSearchOrg for "Auth"
- Assert: AuthHandler in results (from both FTS and concept)
- Assert: results are deduplicated (AuthHandler appears once)

Test: HybridSearchOrg deduplicates correctly
- Store repo where function "LoginHandler" matches:
  - FTS for "Login"
  - Concept for "handler"
- HybridSearchOrg for "Login"
- Assert: LoginHandler appears exactly once in results

Test: SearchFunctionsOrg respects limit
- Store org with 20 functions matching query
- SearchFunctionsOrg with limit=5
- Assert: exactly 5 results returned

Test: Org search with empty org (no repos)
- Register org with no repos
- SearchFunctionsOrg with that orgID
- Assert: empty results, no error

Test: Org search with nonexistent org
- SearchFunctionsOrg with orgID="nonexistent"
- Assert: empty results (subquery returns no repo_ids)
```

## Implementation Details

### 1. OrgSearcher Interface

Define in `internal/storage/interfaces.go` (or alongside existing SearchableStore):

```go
type OrgSearcher interface {
    SearchFunctionsOrg(ctx context.Context, orgID string, query string, limit int) ([]FunctionRef, error)
    SearchByConceptOrg(ctx context.Context, orgID string, concept string, limit int) ([]FunctionRef, error)
    HybridSearchOrg(ctx context.Context, orgID string, query string, limit int) ([]FunctionRef, error)
}
```

This is a separate interface from `SearchableStore` to avoid breaking existing implementations and test mocks. `SQLiteStore` satisfies both interfaces.

### 2. SearchFunctionsOrg

Delegates to `SearchFunctionsFTS` from Section 1. This is the primary org-scoped keyword search method.

```go
func (s *SQLiteStore) SearchFunctionsOrg(ctx context.Context, orgID string, query string, limit int) ([]FunctionRef, error)
```

Implementation:
1. Call `SearchFunctionsFTS(ctx, orgID, query, limit)`
2. Return results directly

If FTS5 is not available (detected by checking if the table exists), fall back to a LIKE-based query across org repos:
```sql
SELECT f.name, f.signature, fi.path, f.line,
       json_extract(f.behavior_json, '$.summary'), fi.repo_id
FROM functions f
JOIN files fi ON f.file_id = fi.id
WHERE fi.repo_id IN (SELECT repo_id FROM org_repos WHERE org_id = ?)
AND (f.name_lower LIKE ? OR f.signature LIKE ? OR f.description LIKE ?)
ORDER BY CASE WHEN f.name_lower = ? THEN 0 ELSE 1 END, f.name_lower
LIMIT ?
```

### 3. SearchByConceptOrg

Same logic as existing `SearchByConcept` but with org-scoped repo filter.

```go
func (s *SQLiteStore) SearchByConceptOrg(ctx context.Context, orgID string, concept string, limit int) ([]FunctionRef, error)
```

SQL:
```sql
SELECT f.name, f.signature, fi.path, f.line,
       json_extract(f.behavior_json, '$.summary'), fi.repo_id
FROM functions f
JOIN files fi ON f.file_id = fi.id
JOIN function_concepts fc ON fc.function_id = f.id
JOIN concepts c ON c.id = fc.concept_id
WHERE fi.repo_id IN (SELECT repo_id FROM org_repos WHERE org_id = ?)
AND c.name = ?
LIMIT ?
```

Uses the existing concept mapping system. The hardcoded concept expansion (e.g., "auth" → "authentication", "error" → "error_handling") from the existing `SearchByConcept` should be reused via a shared helper.

### 4. HybridSearchOrg

Combines FTS5 keyword search and concept search, deduplicating results.

```go
func (s *SQLiteStore) HybridSearchOrg(ctx context.Context, orgID string, query string, limit int) ([]FunctionRef, error)
```

Implementation:
1. Run `SearchFunctionsOrg(ctx, orgID, query, limit)` for keyword results
2. Map query to concept via existing concept mapping helper
3. If concept found, run `SearchByConceptOrg(ctx, orgID, concept, limit)`
4. Merge results with dedup using key `repoID:filePath:funcName`
5. Keyword results come first (higher priority), followed by concept-only results
6. Truncate to `limit`

The dedup key uses `FunctionRef.RepoID + ":" + FunctionRef.File + ":" + FunctionRef.Function` which uniquely identifies a function across repos.

### 5. Scan Helper

Add `scanOrgFunctionRefs` helper that scans SQL rows into `[]FunctionRef` with RepoID populated. Similar to existing `scanFunctionRefs` but includes the `fi.repo_id` column.

## Error Handling

- orgID with no repos: subquery returns empty set, result is empty (no error)
- FTS5 unavailable: detected at query time, falls back to LIKE
- Concept not found in mapping: concept search returns empty, hybrid returns keyword-only results
- SQL errors: wrapped and returned

## File Summary

| File | Action |
|------|--------|
| `internal/storage/interfaces.go` | Modify: add OrgSearcher interface |
| `internal/storage/sqlite_search.go` | Modify: add SearchFunctionsOrg, SearchByConceptOrg, HybridSearchOrg, scanOrgFunctionRefs |
| `internal/storage/sqlite_org_search_test.go` | New: org-scoped search tests |
