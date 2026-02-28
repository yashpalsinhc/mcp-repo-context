I now have all the context needed. Here is the section content:

# Section 1: Function-Level Hash Tracking

## Overview

This section introduces per-function and per-type content hash tracking so the system can detect exactly which functions changed when a file is modified. This is the foundation for incremental vector updates (Section 3) and stale embedding cleanup (Section 4).

Currently, the system tracks file-level hashes via the `file_hashes` table and `FileTracker` interface in `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/storage/file_tracker.go`. This section adds a parallel `function_hashes` table and corresponding CRUD methods on `SQLiteStore` to track individual function/type content hashes within a file.

## Dependencies

- None. This section has no dependencies and can be implemented independently.
- Sections 3 (incremental vector updates) and 4 (stale cleanup) depend on this section.

## Key Design Decisions

**Name qualification:** Function names must include the receiver to avoid collisions. For example, `(*Foo).Bar` and `(*Baz).Bar` are distinct entries even in the same file. The qualified name format is `receiver.name` when a receiver exists, otherwise just `name`.

**Hash computation:** SHA256 of the raw source code of the function, extracted from the file using `LineStart`/`LineEnd` from `FunctionDef`. If source extraction is unavailable (no file on disk), fall back to SHA256 of `receiver + name + signature + body` from the AST.

**Vector ID reference:** The `vector_id` column is an application-level reference to the vectors table ID (NOT a real foreign key). The vectors table lives in a separate SQLite database, so a database-level FK constraint is not possible.

## Tests

All tests go in `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/storage/sqlite_test.go` (extend the existing file).

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

Tests should use the existing `createTestSQLiteStore(t)` helper which creates a temp-file-backed SQLite database. The helper is already defined in `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/storage/sqlite_test.go`.

## Implementation

### 1. Migration File

**File:** `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/storage/migrations/004_function_hashes.sql`

Create a new migration file (version 4 is the next available, since versions 1, 2, and 3 already exist for `initial_schema`, `file_hashes`, and `org_tables` respectively).

The SQL migration should create:

```sql
-- Function-level hash tracking for incremental vector updates
-- Tracks SHA256 hashes of individual functions/types to detect changes
-- at function granularity rather than file granularity.

CREATE TABLE IF NOT EXISTS function_hashes (
    repo_id TEXT NOT NULL,
    file_path TEXT NOT NULL,
    name TEXT NOT NULL,         -- Fully qualified: includes receiver, e.g. "(*Foo).Bar"
    type TEXT NOT NULL,          -- "function" or "type"
    content_hash TEXT NOT NULL,  -- SHA256 of raw source code (line range from file)
    vector_id TEXT,              -- Application-level reference to vectors table ID (NOT a real FK)
    PRIMARY KEY (repo_id, file_path, name, type)
);

-- Index for efficient lookups by repo_id + file_path (common query pattern)
CREATE INDEX IF NOT EXISTS idx_function_hashes_repo_file
    ON function_hashes(repo_id, file_path);

-- Record migration version
INSERT OR IGNORE INTO schema_migrations (version) VALUES (4);
```

Note: No FOREIGN KEY on `repo_id` referencing `repos(id)` with `ON DELETE CASCADE` is needed here because the `function_hashes` table will be cleaned up explicitly when repos or files are removed (Section 4). However, if you want cascade behavior for safety, you may add it -- it follows the pattern used by `file_hashes`.

### 2. FunctionHashInfo Type

**File:** `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/storage/function_hashes.go` (new file)

Create a new file in the storage package dedicated to function hash tracking. This follows the pattern of `file_tracker.go` being separate from `sqlite.go`.

```go
// FunctionHashInfo contains metadata about a single function or type's content hash.
type FunctionHashInfo struct {
    Name        string // Fully qualified with receiver, e.g. "(*Foo).Bar"
    Type        string // "function" or "type"
    ContentHash string // SHA256 of raw source code
    VectorID    string // Application-level ref to vectors table
}
```

### 3. Migration Wiring

**File:** `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/storage/sqlite.go`

Add an `//go:embed` directive for the new migration file and call a new `migrateFunctionHashes()` method from the existing `migrate()` function. Follow the exact pattern used by `migrateFileHashes()` and `migrateOrgTables()`:

- Embed the migration SQL with `//go:embed migrations/004_function_hashes.sql`
- Add `migrateFunctionHashes()` method that checks `schema_migrations` for version >= 4
- Call it from `migrate()` after `migrateOrgTables()`

### 4. CRUD Methods on SQLiteStore

**File:** `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/storage/function_hashes.go`

Implement these four methods on `SQLiteStore`:

**`GetFunctionHashes(ctx context.Context, repoID, filePath string) (map[string]FunctionHashInfo, error)`**
- Query all rows from `function_hashes` WHERE `repo_id = ? AND file_path = ?`
- Return a map keyed by `"name:type"` (e.g., `"(*Foo).Bar:function"`)
- Return empty map (not nil) if no rows found

**`UpdateFunctionHashes(ctx context.Context, repoID, filePath string, hashes []FunctionHashInfo) error`**
- Bulk upsert using `INSERT OR REPLACE` in a single transaction
- For each `FunctionHashInfo`, insert/replace into `function_hashes` with the composite primary key `(repo_id, file_path, name, type)`
- The transaction ensures atomicity -- either all hashes update or none do

**`DeleteFunctionHashes(ctx context.Context, repoID, filePath string) error`**
- Execute `DELETE FROM function_hashes WHERE repo_id = ? AND file_path = ?`
- Used when a file is deleted entirely

**`GetChangedFunctions(ctx context.Context, repoID, filePath string, current map[string]string) (added, modified, removed []string, err error)`**
- `current` parameter is a map of `"name:type"` to `content_hash` representing the functions currently in the file
- Load stored hashes via `GetFunctionHashes`
- Compare:
  - **added**: keys in `current` but not in stored
  - **modified**: keys in both but hashes differ
  - **removed**: keys in stored but not in `current`
- Return the three slices of `"name:type"` keys

### 5. Qualified Name Helper

**File:** `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/storage/function_hashes.go`

Add a helper function to build qualified names from `FunctionDef` and `TypeDef`:

```go
// QualifyFunctionName builds a fully qualified function name including receiver.
// For methods: "(*Foo).Bar" or "(Foo).Bar"
// For plain functions: "MyFunction"
func QualifyFunctionName(name, receiver string) string
```

This uses the `Receiver` field from `FunctionDef` (type `string` in `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/context/types.go`). If receiver is non-empty, format as `(receiver).name`. If receiver is empty, return name as-is.

### 6. Content Hash Computation Helper

**File:** `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/storage/function_hashes.go`

Add a helper to compute the content hash for a function:

```go
// ComputeFunctionHash computes SHA256 hash of a function's source code.
// It reads lines [lineStart, lineEnd] from fileContent.
// If lineStart/lineEnd are 0 or extraction fails, falls back to hashing
// the receiver+name+signature from the AST.
func ComputeFunctionHash(fileContent string, lineStart, lineEnd int, name, receiver, signature string) string
```

- Primary: split `fileContent` by newlines, extract lines `[lineStart-1, lineEnd)` (1-indexed to 0-indexed), SHA256 the joined result
- Fallback (when `lineStart == 0` or `lineEnd == 0` or extraction out of bounds): SHA256 of `receiver + name + signature`

## File Summary

| File | Action |
|------|--------|
| `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/storage/migrations/004_function_hashes.sql` | New: migration creating `function_hashes` table |
| `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/storage/function_hashes.go` | New: `FunctionHashInfo` type, CRUD methods, helper functions |
| `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/storage/sqlite.go` | Modify: add embed directive, `migrateFunctionHashes()`, wire into `migrate()` |
| `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/storage/sqlite_test.go` | Extend: add 7 test functions for function hash CRUD |

## Implementation Checklist

1. Create migration file `004_function_hashes.sql` with the table schema and version 4 marker
2. Define `FunctionHashInfo` struct in new `function_hashes.go`
3. Implement `QualifyFunctionName` helper
4. Implement `ComputeFunctionHash` helper
5. Wire the migration: embed directive in `sqlite.go`, `migrateFunctionHashes()` method, call from `migrate()`
6. Implement `GetFunctionHashes` on `SQLiteStore`
7. Implement `UpdateFunctionHashes` on `SQLiteStore` (bulk upsert in transaction)
8. Implement `DeleteFunctionHashes` on `SQLiteStore`
9. Implement `GetChangedFunctions` on `SQLiteStore` (diff logic)
10. Write all 7 tests, verify they pass with `go test ./internal/storage/ -run TestFunctionHash -v`