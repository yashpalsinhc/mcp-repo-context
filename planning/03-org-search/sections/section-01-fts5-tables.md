# Section 1: FTS5 Virtual Tables

## Overview

Add FTS5 virtual tables to the SQLite schema for efficient full-text search across functions. This includes the FTS5 table creation, INSERT/UPDATE/DELETE triggers for auto-sync, explicit cascade delete cleanup, query sanitization, and the SearchFunctionsFTS method.

## Dependencies

- None (foundational section)

## Tests First

### File: `internal/storage/sqlite_fts_test.go` (new)

```
Test: FTS5 table created during ensureSchema
- Open new SQLiteStore with temp DB
- Query: SELECT name FROM sqlite_master WHERE type='table' AND name='functions_fts'
- Assert: table exists

Test: FTS5 INSERT trigger populates index
- Store a repo context with 3 functions via StoreRepoContext
- Query functions_fts directly: SELECT count(*) FROM functions_fts
- Assert: 3 rows

Test: FTS5 UPDATE trigger keeps index in sync
- Store a repo with function named "GetUser" and description "old desc"
- Update the function row directly: UPDATE functions SET description='new description' WHERE ...
- Search FTS for "new description"
- Assert: finds the updated function
- Search FTS for "old desc"
- Assert: no results

Test: FTS5 DELETE trigger removes from index
- Store a repo with 3 functions
- Delete one function row directly
- Search FTS for deleted function name
- Assert: not found
- Assert: FTS count is 2

Test: CASCADE DELETE cleans FTS index (critical)
- Store a repo context with 5 functions, verify FTS has 5 entries
- Call StoreRepoContext again with same repo ID but different functions (3 functions)
- Query FTS count
- Assert: count is 3 (not 8 — no stale entries)
- Search for old function names
- Assert: not found

Test: SearchFunctionsFTS returns ranked results
- Store repo in org with functions: "GetUser", "DeleteUser", "UpdateUserProfile", "CreateOrder"
- Register org with this repo
- SearchFunctionsFTS for "User" with orgID
- Assert: returns 3 results (GetUser, DeleteUser, UpdateUserProfile)
- Assert: CreateOrder not in results
- Assert: results ordered by BM25 rank

Test: SearchFunctionsFTS scopes to org repos only
- Store 2 repos: repo-a (in org) with "GetUser", repo-b (not in org) with "GetUser"
- Register org with repo-a only
- SearchFunctionsFTS with orgID
- Assert: only repo-a's GetUser returned

Test: FTS5 query sanitization — special characters
- Store repo with function "errorHandler"
- Search for: error OR panic (would be boolean without sanitization)
- Assert: no error, treated as phrase "error OR panic"
- Search for: "unmatched quote
- Assert: no error, quotes stripped
- Search for: func*
- Assert: no error, treated as literal

Test: FTS5 query sanitization — empty after sanitization
- Search for: "" (empty string)
- Assert: returns empty results, no error
```

## Implementation Details

### 1. Schema Migration

In `ensureSchema()` in `internal/storage/sqlite.go`, add after existing table creation:

```sql
CREATE VIRTUAL TABLE IF NOT EXISTS functions_fts USING fts5(
    name, signature, description, summary,
    content=functions, content_rowid=id
);
```

This is a content-synced FTS5 table — it reads from `functions` table at query time and uses the functions table's `id` as its rowid.

### 2. Triggers

Add three triggers for auto-sync:

**INSERT trigger** — fires when new function rows are inserted during StoreRepoContext:
```sql
CREATE TRIGGER IF NOT EXISTS functions_fts_insert AFTER INSERT ON functions BEGIN
    INSERT INTO functions_fts(rowid, name, signature, description, summary)
    VALUES (new.id, new.name, new.signature, new.description,
            json_extract(new.behavior_json, '$.summary'));
END;
```

**UPDATE trigger** — fires if a function row is updated in-place (delete old entry, insert new):
```sql
CREATE TRIGGER IF NOT EXISTS functions_fts_update AFTER UPDATE ON functions BEGIN
    INSERT INTO functions_fts(functions_fts, rowid, name, signature, description, summary)
    VALUES ('delete', old.id, old.name, old.signature, old.description,
            json_extract(old.behavior_json, '$.summary'));
    INSERT INTO functions_fts(rowid, name, signature, description, summary)
    VALUES (new.id, new.name, new.signature, new.description,
            json_extract(new.behavior_json, '$.summary'));
END;
```

**DELETE trigger** — fires when function rows are explicitly deleted:
```sql
CREATE TRIGGER IF NOT EXISTS functions_fts_delete AFTER DELETE ON functions BEGIN
    INSERT INTO functions_fts(functions_fts, rowid, name, signature, description, summary)
    VALUES ('delete', old.id, old.name, old.signature, old.description,
            json_extract(old.behavior_json, '$.summary'));
END;
```

### 3. CASCADE DELETE Cleanup

SQLite triggers do NOT fire for rows deleted via ON DELETE CASCADE. The current `StoreRepoContext` deletes repos with `DELETE FROM repos WHERE id = ?`, which cascades to files then functions. The FTS5 DELETE trigger will NOT fire for these cascaded deletions.

**Fix:** In `StoreRepoContext`, add an explicit FTS cleanup statement BEFORE the cascade delete:

```sql
DELETE FROM functions_fts WHERE rowid IN (
    SELECT f.id FROM functions f
    JOIN files fi ON f.file_id = fi.id
    WHERE fi.repo_id = ?
);
```

This runs before `DELETE FROM repos WHERE id = ?`, ensuring the FTS index is cleaned up before the cascade happens. The order is:
1. Delete from functions_fts (explicit, using subquery)
2. Delete from repos (triggers cascade to files → functions)
3. Insert new repo data (triggers fire for new inserts)

### 4. Query Sanitization

Add a `sanitizeFTSQuery` helper function:

```go
func sanitizeFTSQuery(query string) string
```

Logic:
1. Strip all double quotes from the query
2. Trim whitespace
3. If empty after stripping, return empty string
4. Wrap the result in double quotes: `"` + query + `"`

This ensures the query is treated as a phrase match, preventing FTS5 operator injection. The special characters `*`, `AND`, `OR`, `NOT`, `NEAR`, `{`, `}`, `:` are all neutralized inside double quotes.

### 5. SearchFunctionsFTS Method

Add to `SQLiteStore`:

```go
func (s *SQLiteStore) SearchFunctionsFTS(ctx context.Context, orgID string, query string, limit int) ([]FunctionRef, error)
```

Steps:
1. Call `sanitizeFTSQuery(query)` — if empty, return nil
2. Execute SQL with MATCH and org subquery:
   ```sql
   SELECT f.name, f.signature, fi.path, f.line,
          json_extract(f.behavior_json, '$.summary'), fi.repo_id, rank
   FROM functions_fts fts
   JOIN functions f ON f.id = fts.rowid
   JOIN files fi ON f.file_id = fi.id
   WHERE functions_fts MATCH ?
   AND fi.repo_id IN (SELECT repo_id FROM org_repos WHERE org_id = ?)
   ORDER BY rank
   LIMIT ?
   ```
3. Scan results into `[]FunctionRef` (with RepoID populated from fi.repo_id)
4. Return results

The `rank` column is FTS5's built-in BM25 ranking. Lower (more negative) rank = better match. ORDER BY rank returns best matches first.

### 6. FunctionRef Extension

Add `RepoID string` field to the `FunctionRef` struct in `internal/storage/types.go`. This field is populated by org-scoped search methods but left empty by existing per-repo methods (backward compatible).

## Error Handling

- FTS5 table creation failure: logged as warning, LIKE-based search remains functional
- Sanitized query is empty: return empty results, no error
- FTS MATCH syntax error (shouldn't happen with sanitization): return wrapped error
- CASCADE cleanup failure: return error from StoreRepoContext (data integrity critical)

## File Summary

| File | Action |
|------|--------|
| `internal/storage/sqlite.go` | Modify: add FTS5 table + triggers in ensureSchema(), add FTS cleanup in StoreRepoContext |
| `internal/storage/sqlite_search.go` | Modify: add SearchFunctionsFTS method, add sanitizeFTSQuery helper |
| `internal/storage/types.go` | Modify: add RepoID field to FunctionRef |
| `internal/storage/sqlite_fts_test.go` | New: FTS5 tests |
