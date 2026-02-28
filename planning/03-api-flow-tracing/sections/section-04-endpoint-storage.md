# Section 04: Normalized Endpoint Storage (SQLite)

## Overview

This section adds two normalized SQLite tables — `endpoints` (server-side routes) and `service_calls` (client-side HTTP/gRPC/Kafka calls) — enabling efficient cross-repo SQL joins for endpoint matching. The existing JSON blob storage remains for backward compatibility; these tables are populated alongside during `StoreRepoContext`.

## Dependencies

- **section-01-http-grpc-extraction**: Enhanced ExternalCall with URLExpression, ServiceHint
- **section-02-route-detection**: Enhanced Route with RawPath, Framework, consolidated Route struct
- **section-03-kafka-extraction**: AsyncCall type with topic data

## Tests First

### File: `internal/storage/sqlite_test.go` (extend)

```go
// Test: StoreEndpoints and GetEndpoints round-trip
// Setup: Store 3 endpoints for repo "test-repo" with varied methods and paths
// Call: StoreEndpoints, then GetEndpoints
// Assert: All 3 returned with correct repo_id, file_path, handler_name, method, path, framework

// Test: StoreEndpoints replaces existing for same repo (delete-then-insert)
// Setup: Store 3 endpoints for repo "test-repo", then store 2 different endpoints
// Call: GetEndpoints for "test-repo"
// Assert: Returns 2 (not 5), previous entries deleted

// Test: StoreEndpoints with multiple repos keeps them separate
// Setup: Store endpoints for repo A and repo B
// Call: GetEndpoints for repo A
// Assert: Only repo A endpoints returned

// Test: StoreServiceCalls and GetServiceCalls round-trip
// Setup: Store 2 service calls (1 HTTP, 1 Kafka) for repo "test-repo"
// Assert: Both returned with correct call_type, method, target, service_hint

// Test: StoreServiceCalls replaces existing for same repo
// Setup: Store 3 calls, then store 1 call for same repo
// Assert: Only 1 returned

// Test: FindMatchingEndpoints returns cross-repo results
// Setup: Repo A has GET /api/users, Repo B has GET /api/users
// Call: FindMatchingEndpoints("GET", "/api/users")
// Assert: Returns endpoints from both repos

// Test: FindMatchingEndpoints filters by method
// Setup: Repo A has GET /api/users, Repo B has POST /api/users
// Call: FindMatchingEndpoints("GET", "/api/users")
// Assert: Only repo A endpoint returned

// Test: FindMatchingEndpoints with ANY method matches all
// Setup: Repo A has GET /api/users (method=ANY)
// Call: FindMatchingEndpoints("POST", "/api/users")
// Assert: Repo A matches (ANY matches any method)

// Test: FindMatchingConsumers returns Kafka consumers by topic
// Setup: Repo B has kafka_consume for "user.events", Repo C has kafka_consume for "user.events"
// Call: FindMatchingConsumers("user.events")
// Assert: Returns entries from both repos

// Test: FindMatchingProducers returns Kafka producers by topic
// Setup: Repo A has kafka_produce for "user.events"
// Call: FindMatchingProducers("user.events")
// Assert: Returns repo A entry

// Test: Endpoint tables populated during StoreRepoContext
// Setup: Create a RepoContext with files containing routes and external calls
// Call: StoreRepoContext
// Assert: endpoints table has entries from routes, service_calls has entries from external calls and async calls

// Test: Tables created by ensureSchema
// Setup: Create fresh SQLite database via NewSQLiteStore
// Assert: endpoints and service_calls tables exist (query information_schema or PRAGMA)

// Test: DeleteContext removes endpoints and service_calls for repo
// Setup: Store endpoints and service calls for a repo, then DeleteContext
// Assert: No endpoints or service_calls remain for that repo
```

## Implementation Details

### 1. Go Types for Endpoint and ServiceCall

**File:** `internal/storage/types.go` or `internal/context/types.go`

```go
type Endpoint struct {
    ID          int64
    RepoID      string
    FilePath    string
    HandlerName string
    Method      string  // GET, POST, ANY, gRPC
    Path        string  // normalized with {param}
    RawPath     string  // original as written
    Framework   string  // gorilla/mux, chi, net/http, gin, echo
    Line        int
}

type ServiceCall struct {
    ID               int64
    RepoID           string
    FilePath         string
    FunctionName     string
    CallType         string  // "http", "grpc", "kafka_produce", "kafka_consume"
    Method           string  // GET, POST, gRPC, produce, consume
    Target           string  // URL template or topic name
    TargetExpression string  // original expression for dynamic targets
    ServiceHint      string  // guessed destination service
    Line             int
}
```

### 2. SQLite Schema Migration

**File:** `internal/storage/sqlite.go` — add to `ensureSchema()` method

Add these CREATE TABLE IF NOT EXISTS statements alongside existing schema:

```sql
CREATE TABLE IF NOT EXISTS endpoints (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    repo_id TEXT NOT NULL,
    file_path TEXT NOT NULL,
    handler_name TEXT NOT NULL,
    method TEXT NOT NULL,
    path TEXT NOT NULL,
    raw_path TEXT DEFAULT '',
    framework TEXT DEFAULT '',
    line INTEGER DEFAULT 0,
    UNIQUE(repo_id, file_path, handler_name, method, path)
);
CREATE INDEX IF NOT EXISTS idx_endpoints_path ON endpoints(path);
CREATE INDEX IF NOT EXISTS idx_endpoints_repo ON endpoints(repo_id);

CREATE TABLE IF NOT EXISTS service_calls (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    repo_id TEXT NOT NULL,
    file_path TEXT NOT NULL,
    function_name TEXT NOT NULL,
    call_type TEXT NOT NULL,
    method TEXT DEFAULT '',
    target TEXT NOT NULL,
    target_expression TEXT DEFAULT '',
    service_hint TEXT DEFAULT '',
    line INTEGER DEFAULT 0,
    UNIQUE(repo_id, file_path, function_name, call_type, target, line)
);
CREATE INDEX IF NOT EXISTS idx_service_calls_target ON service_calls(target);
CREATE INDEX IF NOT EXISTS idx_service_calls_repo ON service_calls(repo_id);
CREATE INDEX IF NOT EXISTS idx_service_calls_type ON service_calls(call_type);
```

### 3. Storage Methods

**File:** `internal/storage/sqlite.go` (extend)

**StoreEndpoints(ctx, repoID string, endpoints []Endpoint) error:**
1. Begin transaction
2. DELETE FROM endpoints WHERE repo_id = ? (clear existing for repo)
3. INSERT each endpoint using prepared statement
4. Commit transaction

**StoreServiceCalls(ctx, repoID string, calls []ServiceCall) error:**
Same pattern: delete existing for repo, batch insert new.

**GetEndpoints(ctx, repoID string) ([]Endpoint, error):**
SELECT * FROM endpoints WHERE repo_id = ? ORDER BY path, method

**GetServiceCalls(ctx, repoID string) ([]ServiceCall, error):**
SELECT * FROM service_calls WHERE repo_id = ? ORDER BY call_type, target

**FindMatchingEndpoints(ctx, method, path string) ([]Endpoint, error):**
```sql
SELECT * FROM endpoints WHERE path = ? AND (method = ? OR method = 'ANY')
```
Note: This is exact path match. Parameterized matching happens in the matcher layer (Section 5) which loads endpoints into a trie. This SQL query is for exact matches or pre-filtering.

**FindMatchingConsumers(ctx, topic string) ([]ServiceCall, error):**
```sql
SELECT * FROM service_calls WHERE call_type = 'kafka_consume' AND target = ?
```

**FindMatchingProducers(ctx, topic string) ([]ServiceCall, error):**
```sql
SELECT * FROM service_calls WHERE call_type = 'kafka_produce' AND target = ?
```

### 4. Populating During StoreRepoContext

**File:** `internal/storage/sqlite.go` — modify `StoreRepoContext`

After storing files and functions (existing logic), add a call to populate the normalized tables:

```go
func (s *SQLiteStore) storeEndpointIndex(ctx context.Context, tx *sql.Tx, repoID string, repoCtx *context.RepoContext) error
```

This method:
1. Collects all routes from `repoCtx.Files[*].Routes` → creates `[]Endpoint`
2. Collects all external calls from `repoCtx.Files[*].Functions[*].APIFlow.ExternalCalls` → creates `[]ServiceCall` with call_type="http" or "grpc"
3. Collects all async calls from `repoCtx.Files[*].Functions[*].AsyncCalls` → creates `[]ServiceCall` with call_type="kafka_produce" or "kafka_consume"
4. Calls StoreEndpoints and StoreServiceCalls within the existing transaction

### 5. Cleanup on DeleteContext

**File:** `internal/storage/sqlite.go` — modify `DeleteContext`

Add DELETE FROM endpoints WHERE repo_id = ? and DELETE FROM service_calls WHERE repo_id = ? alongside existing cleanup.

## Error Handling

- Transaction failure during endpoint storage: rollback, return error (repo analysis still succeeds — endpoint index is supplementary)
- Missing APIFlow or AsyncCalls fields on functions: skip those entries (backward compatible with pre-enhancement contexts)
- Duplicate key on UNIQUE constraint: use INSERT OR REPLACE to handle gracefully

## File Summary

| File | Action |
|------|--------|
| `internal/storage/sqlite.go` | Add ensureSchema tables, StoreEndpoints, StoreServiceCalls, GetEndpoints, GetServiceCalls, FindMatchingEndpoints, FindMatchingConsumers, FindMatchingProducers, storeEndpointIndex, cleanup on DeleteContext |
| `internal/storage/types.go` | Add Endpoint, ServiceCall types (or in context/types.go) |
| `internal/storage/sqlite_test.go` | Tests for all CRUD methods and auto-population |

## Implementation Order

1. Write tests
2. Add Endpoint and ServiceCall types
3. Add CREATE TABLE statements to ensureSchema
4. Implement CRUD methods (Store, Get, FindMatching*)
5. Add storeEndpointIndex to StoreRepoContext
6. Add cleanup to DeleteContext
7. Run all tests
