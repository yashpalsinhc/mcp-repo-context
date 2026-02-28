I have all the necessary information from the plan and TDD files to write the section. Let me generate the complete section content for section-05-endpoint-matching.

# Section 05: Cross-Service Endpoint Matching

## Overview

This section implements `EndpointMatcher`, the core engine that connects client-side `service_calls` rows to server-side `endpoints` rows across repositories. It lives in a new package `internal/flow/` and depends on the normalized SQLite tables from Section 04 being populated.

**Dependency:** Requires Section 04 (Normalized Endpoint Storage) to be complete. The `endpoints` and `service_calls` tables must exist and be populated before this section can be tested end-to-end.

**Blocks:** Section 06 (Service Topology Graph) and Section 07 (MCP Tools) both depend on this matcher.

---

## Files to Create

- `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/flow/matcher.go` — `EndpointMatcher` struct and all matching logic
- `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/flow/matcher_test.go` — unit tests

---

## Tests First

### File: `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/flow/matcher_test.go`

All tests use real SQLite (temp file), consistent with existing patterns in the codebase. Create a helper that sets up a temporary `SQLiteStore`, seeds the `endpoints` and `service_calls` tables, and returns a configured `EndpointMatcher`.

```go
// Test: exact path match
// Setup: endpoint "/api/v1/users", call target "/api/v1/users"
// Assert: match with confidence "exact"

// Test: parameterized path match
// Setup: endpoint "/api/v1/users/{id}", call target "/api/v1/users/123"
// Assert: match with confidence "parameterized"

// Test: no match for different paths
// Setup: endpoint "/api/v1/users", call target "/api/v1/orders"
// Assert: no match returned

// Test: method filtering
// Setup: endpoint GET "/users", call method POST
// Assert: no match (method mismatch)

// Test: service hint narrows ambiguous matches
// Setup: endpoint "/health" in repo A and repo B, call with service_hint="A"
// Assert: match from repo A ranked first

// Test: Kafka topic exact match
// Setup: producer topic "user.events", consumer entry in service_calls with topic "user.events"
// Call: MatchKafkaConsumers("user.events")
// Assert: returns consumer entry

// Test: URL normalization strips scheme and host
// Setup: call target "https://auth-service:8080/api/validate"
// Assert: normalized to "/api/validate" before matching against endpoint "/api/validate"

// Test: URL normalization handles trailing slashes
// Setup: endpoint "/api/users/", call "/api/users"
// Assert: match (trailing slash stripped on both sides)

// Test: path trie batch matching
// Setup: 100 endpoints across repos, 50 service_calls
// Call: MatchAll()
// Assert: all valid matches found, no panic, completes in reasonable time

// Test: gRPC exact path match
// Setup: endpoint with method="gRPC" and path="/UserService/GetUser"
// Call target: "/UserService/GetUser"
// Assert: match with confidence "exact"

// Test: ambiguous match when multiple endpoints match without hint
// Setup: endpoint "/users" in repos A and B, call target "/users" with no service_hint
// Assert: both returned with confidence "ambiguous"

// Test: NormalizePath removes dot-segments
// Setup: call target "/api/../api/users"
// Assert: normalized to "/api/users"

// Test: NormalizePath decodes percent-encoded chars
// Setup: call target "/api/users%2F123"
// Assert: normalized to "/api/users/123"

// Test: NormalizePath lowercases the path
// Setup: call target "/API/Users"
// Assert: normalized to "/api/users"
```

---

## Implementation

### Package Declaration

The new package is `flow`, located at `internal/flow/`. It imports from `internal/storage` to access the SQLite store's query methods for `endpoints` and `service_calls`.

### Types

```go
// MatchResult represents a single endpoint match for a service call.
type MatchResult struct {
    // Endpoint is the matched server-side endpoint.
    Endpoint storage.Endpoint

    // Confidence is one of: "exact", "parameterized", "hint-matched", "ambiguous".
    Confidence string
}

// CallMatch associates a service call with zero or more matched endpoints.
type CallMatch struct {
    Call    storage.ServiceCall
    Matches []MatchResult
}

// EndpointMatcher connects client service_calls to server endpoints via
// path-template matching, Kafka topic matching, and gRPC exact matching.
type EndpointMatcher struct {
    store Store // interface over SQLiteStore; see Store interface below
}
```

### Store Interface

Define a minimal interface over the SQLite store so the matcher can be tested with a mock or a real store:

```go
// Store is the subset of storage.SQLiteStore methods needed by EndpointMatcher.
type Store interface {
    FindMatchingEndpoints(ctx context.Context, method, pathPattern string) ([]storage.Endpoint, error)
    FindMatchingConsumers(ctx context.Context, topic string) ([]storage.ServiceCall, error)
    FindMatchingProducers(ctx context.Context, topic string) ([]storage.ServiceCall, error)
    GetServiceCalls(ctx context.Context, repoID string) ([]storage.ServiceCall, error)
    GetEndpoints(ctx context.Context, repoID string) ([]storage.Endpoint, error)
}
```

### Constructor

```go
// NewEndpointMatcher creates an EndpointMatcher backed by the given store.
func NewEndpointMatcher(store Store) *EndpointMatcher
```

---

### URL Normalization

Implement a standalone exported function so it can be tested independently:

```go
// NormalizePath normalizes a raw URL or path for use in matching.
//
// Steps applied in order:
//  1. Parse the string as a URL; if it has a scheme/host, extract only the path component.
//  2. Lowercase the path.
//  3. URL-decode percent-encoded characters (RFC 3986).
//  4. Resolve dot-segments (".", "..") per RFC 3986 §5.2.4.
//  5. Collapse consecutive slashes into one.
//  6. Strip trailing slash (unless path is "/").
//
// Returns the normalized path string.
func NormalizePath(raw string) string
```

---

### Path Segment Matching

Implement segment-by-segment matching. Path parameter syntax from Section 02 is already normalized to `{param}` in stored endpoints (e.g., gorilla `{id}`, gin `:id` all become `{param}` or `{id}` with braces). The matcher treats any segment enclosed in `{...}` as a wildcard that matches any single literal segment.

```go
// matchSegments returns true if the endpoint path template matches the
// normalized client path. A template segment of the form "{...}" matches
// any single literal segment in the client path. All other segments must
// match exactly.
//
// Both paths should already be normalized before calling this function.
func matchSegments(template, clientPath string) bool
```

Confidence is determined by whether any wildcard substitution occurred:
- All segments matched literally → `"exact"`
- At least one `{param}` substitution occurred → `"parameterized"`

---

### HTTP Matching

```go
// MatchHTTP finds server-side endpoints that match the given service call.
//
// Algorithm:
//  1. Normalize call.Target using NormalizePath.
//  2. Call store.FindMatchingEndpoints to retrieve candidate endpoints whose
//     stored path, when normalized, could match the call target. The SQL
//     query uses prefix or equality on the path index.
//  3. For each candidate: run matchSegments against the normalized call target.
//  4. If the call has a non-empty ServiceHint, prefer (rank first) endpoints
//     from repos whose name contains the hint. Multiple remaining matches
//     are labeled "ambiguous".
//  5. Return all matches with assigned confidence levels.
//
// When no matches are found, returns an empty slice (not an error).
func (m *EndpointMatcher) MatchHTTP(ctx context.Context, call storage.ServiceCall) ([]MatchResult, error)
```

#### Service Hint Ranking

Service hints come from the `ServiceHint` field on `storage.ServiceCall` (populated in Section 01 from the URL hostname or receiver field name). When a hint is present:
- Matches from repos whose `repo_id` or service name contains the hint string (case-insensitive) → confidence `"hint-matched"`.
- All remaining matches (if any) → confidence `"ambiguous"`.
- If only one match remains after hint filtering → confidence unchanged (keep `"exact"` or `"parameterized"`).

---

### Kafka Matching

```go
// MatchKafkaConsumers finds all service_calls with call_type="kafka_consume"
// matching the given topic name. Topic matching is exact string equality.
//
// Returns consumer service_calls from any repo. Empty slice if none found.
func (m *EndpointMatcher) MatchKafkaConsumers(ctx context.Context, topic string) ([]storage.ServiceCall, error)

// MatchKafkaProducers finds all service_calls with call_type="kafka_produce"
// for the given topic. Used for reverse lookup (find who produces a topic).
func (m *EndpointMatcher) MatchKafkaProducers(ctx context.Context, topic string) ([]storage.ServiceCall, error)
```

---

### gRPC Matching

gRPC targets are stored as `/ServiceName/MethodName` by the Section 01 extractor. Matching is exact (no parameterization):

```go
// MatchGRPC finds endpoints where method="gRPC" and path exactly matches
// the normalized target from the service call. Returns matches with
// confidence "exact" only.
func (m *EndpointMatcher) MatchGRPC(ctx context.Context, call storage.ServiceCall) ([]MatchResult, error)
```

---

### Batch Matching (Path Trie)

For topology building (Section 06), matching all calls against all endpoints individually would be O(N*M) SQL queries. Instead, implement a bulk loader that builds an in-memory path trie and matches all calls in one pass.

```go
// BuildTrie loads all endpoints from all repos and constructs an in-memory
// path trie keyed by normalized path segments. Used for batch matching.
//
// The trie node stores a list of Endpoint values at the leaf so that
// multiple endpoints with the same path structure (different repos or methods)
// are all reachable.
func (m *EndpointMatcher) BuildTrie(ctx context.Context, repoIDs []string) (*PathTrie, error)

// PathTrie is an in-memory trie over path segments for O(depth) lookup.
// Wildcard nodes (from {param} segments) match any single segment.
type PathTrie struct { /* internal */ }

// Match returns all endpoints in the trie whose path template matches the
// given normalized path. Wildcard segments match any literal segment.
func (t *PathTrie) Match(normalizedPath string) []MatchResult

// MatchAll runs all service_calls from the given repos against the trie
// and returns a CallMatch slice covering all calls (including those with
// zero matches, so callers can report unmatched calls).
func (m *EndpointMatcher) MatchAll(ctx context.Context, repoIDs []string) ([]CallMatch, error)
```

The `PathTrie` should handle:
- Literal segment nodes (exact string match).
- Wildcard nodes (any `{...}` segment in the stored path becomes a wildcard node at that position).
- Multiple endpoints at the same path (different repos, methods).
- A single `{param}` in one position does not consume multiple client segments.

---

### Confidence Levels (Reference)

| Confidence | When assigned |
|---|---|
| `"exact"` | All path segments matched literally, or gRPC exact match |
| `"parameterized"` | At least one `{param}` segment substituted |
| `"hint-matched"` | Service hint caused narrowing to a single repo's match |
| `"ambiguous"` | Multiple matches remain with no hint to disambiguate |

---

## Key Design Notes

### Known Limitations (from plan)

- **File-scoped constant resolution only**: If a client URL was built from a constant in another file and Section 01 could not resolve it, the `Target` stored in `service_calls` will be `"<dynamic>"`. The matcher skips rows where `Target == "<dynamic>"` and they appear in the `Unmatched` list in the topology and trace output.
- **Kafka dynamic topics**: Similarly, rows with `Topic == "<dynamic>"` cannot be matched and are reported as unmatched.
- **Common paths**: `/health`, `/metrics`, `/api/v1/status` often appear in many repos. Service hints are the primary disambiguation mechanism. Without a hint, these will be labeled `"ambiguous"` and returned with all candidates.

### What Section 04 Must Provide

The matcher calls the following methods on the `Store` interface (all defined in Section 04 on `SQLiteStore`):
- `FindMatchingEndpoints(ctx, method, pathPattern string) ([]Endpoint, error)` — returns endpoints filtered by a normalized path pattern (SQL `WHERE path = ?` or prefix query using the `idx_endpoints_path` index).
- `FindMatchingConsumers(ctx, topic string) ([]ServiceCall, error)` — returns service_calls where `call_type='kafka_consume'` and `target = ?`.
- `FindMatchingProducers(ctx, topic string) ([]ServiceCall, error)` — same for produce.
- `GetEndpoints(ctx, repoID string) ([]Endpoint, error)` — used by `BuildTrie` to bulk-load all endpoints.
- `GetServiceCalls(ctx, repoID string) ([]ServiceCall, error)` — used by `MatchAll`.

The `Endpoint` and `ServiceCall` types are defined in Section 04 in the `internal/storage` package.

### What This Section Provides to Section 06

Section 06 (Service Topology) calls `EndpointMatcher.MatchAll(ctx, repoIDs)` to build the edge list for the topology graph. It also calls `MatchKafkaConsumers` and `MatchKafkaProducers` for Kafka edges. The `CallMatch` type (with zero or more `MatchResult` entries per call) is the primary output contract.

---

## Implementation Checklist

1. Create directory `internal/flow/` if it does not exist.
2. Create `internal/flow/matcher.go` with package declaration `package flow`.
3. Define the `Store` interface.
4. Implement `NormalizePath(raw string) string`.
5. Implement `matchSegments(template, clientPath string) bool` (unexported helper).
6. Implement `EndpointMatcher` struct and `NewEndpointMatcher`.
7. Implement `MatchHTTP` with service-hint ranking.
8. Implement `MatchKafkaConsumers` and `MatchKafkaProducers`.
9. Implement `MatchGRPC`.
10. Implement `PathTrie` with wildcard-aware trie structure.
11. Implement `BuildTrie` to bulk-load endpoints from the store.
12. Implement `MatchAll` using the trie.
13. Create `internal/flow/matcher_test.go` with all tests listed above using real SQLite temp files.