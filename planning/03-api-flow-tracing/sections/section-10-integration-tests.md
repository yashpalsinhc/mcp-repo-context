# Section 10: Integration Tests

## Overview

End-to-end tests for the full cross-service API flow tracing pipeline. Uses synthetic multi-service Go fixtures with known HTTP calls and Kafka usage to verify the entire flow from analysis through topology building and trace querying.

## Dependencies

- All previous sections (01-09)

## Tests First

### File: `internal/integration/flow_tracing_test.go` (new)

```go
// Test: Full pipeline — analyze 3 services, build topology, verify edges
// Setup: Create synthetic Go source files for 3 services:
//   Service A (auth-service): handler POST /login, calls GET http://user-service:8080/api/users/{id},
//     produces to Kafka topic "user.logged_in"
//   Service B (user-service): handler GET /api/users/{id}, queries DB
//   Service C (notification-service): consumes Kafka topic "user.logged_in", calls POST http://audit-service:9090/audit/log
// Action: Analyze all 3 repos, register as org, build topology
// Assert: Topology has 3 nodes, edges: A→B (HTTP), A→"user.logged_in" (Kafka produce),
//   "user.logged_in"→C (Kafka consume), C→<unknown> (audit-service not analyzed)

// Test: trace_api_flow from POST /login shows full trace
// Setup: Same 3 services as above, analyzed and indexed
// Call: trace_api_flow with org_id, entry_point="POST /login"
// Assert: Trace shows:
//   1. auth-service LoginHandler
//      ├─ HTTP GET /api/users/{id} → user-service GetUser [parameterized]
//      └─ Kafka produce → user.logged_in
//         └─ notification-service HandleLogin [exact]
// Assert: Mermaid sequence diagram present

// Test: get_service_map shows all services with edges
// Call: get_service_map with org_id
// Assert: Service table lists auth-service (1 endpoint, 2 calls out),
//   user-service (1 endpoint, 0 calls out), notification-service (0 endpoints, 1 Kafka in)
// Assert: Mermaid flowchart present with correct edge types

// Test: Dynamic URL marked as <dynamic> with expression
// Setup: Service A has http.Get(getServiceURL("/validate"))
// Action: Analyze
// Assert: ExternalCall.URL == "<dynamic>", URLExpression contains "getServiceURL"
// Assert: service_calls table has target="<dynamic>"

// Test: Fan-out — one topic, two consumers
// Setup: Service A produces "user.events", Services B and C both consume "user.events"
// Action: Analyze all, build topology
// Assert: 3 Kafka edges (1 produce + 2 consume)
// Assert: trace_api_flow shows both consumer branches

// Test: Circular dependency detected and reported
// Setup: Service A calls B's /users, Service B calls A's /auth
// Action: trace_api_flow from A
// Assert: Trace shows A→B, then B→A with "[cycle detected]" marker
// Assert: No infinite recursion

// Test: PR context for modified handler shows cross-service impact
// Setup: Same 3 services. Simulate PR modifying user-service GetUser handler.
// Call: get_pr_context for user-service with modified file
// Assert: Cross-Service Impact section lists auth-service as upstream caller
// Assert: Risk level "Low" (1 service)

// Test: Endpoint storage populated during analysis
// Setup: Analyze service A with 3 routes and 2 external calls
// Assert: endpoints table has 3 entries for service A's repo
// Assert: service_calls table has 2 entries for service A's repo

// Test: Topology rebuild after repo re-analysis
// Setup: Build topology. Re-analyze service A (adds new endpoint).
// Call: get_service_map with rebuild=true
// Assert: New endpoint appears in topology edges

// Test: fmt.Sprintf URL pattern extraction
// Setup: http.Get(fmt.Sprintf("http://user-service:8080/api/users/%d", userID))
// Action: Analyze
// Assert: ExternalCall.URL == "http://user-service:8080/api/users/{param}"
// Assert: Matched to user-service's GET /api/users/{id} endpoint

// Test: gorilla/mux route detection with Methods() chain
// Setup: r.HandleFunc("/users/{id}", GetUser).Methods("GET")
// Action: Analyze
// Assert: endpoints table has entry with method="GET", path="/users/{id}", framework="gorilla/mux"

// Test: Kafka segmentio/kafka-go producer detection
// Setup: writer := &kafka.Writer{Topic: "user.logged_in"}; writer.WriteMessages(ctx, msg)
// Action: Analyze
// Assert: service_calls table has entry with call_type="kafka_produce", target="user.logged_in"

// Test: Multi-language detection from docker-compose
// Setup: docker-compose.yml with node-based frontend service and KAFKA_TOPIC=user.events
// Action: Build topology
// Assert: "frontend" node with IsDeepAnalyzed=false, Language="typescript"
// Assert: Edge connecting frontend to "user.events" topic

// Test: Service hint narrows endpoint matching
// Setup: Both user-service and auth-service register GET /health
//   Service A calls http://user-service:8080/health (ServiceHint="user-service")
// Assert: Matched to user-service's /health, not auth-service's
```

## Implementation Details

### 1. Test Fixture Setup

Create a test helper that generates synthetic Go source files:

```go
func createTestServices(t *testing.T, tmpDir string) map[string]string
```

Returns a map of service name → directory path. Each service directory contains valid Go source files with:
- `main.go` with route registrations and handler functions
- `handlers.go` with handler implementations containing HTTP calls and Kafka usage
- `go.mod` with appropriate module name and dependencies

**Service A (auth-service):**
```go
// main.go: mux.HandleFunc("/login", LoginHandler).Methods("POST")
// handlers.go: LoginHandler calls http.Get(fmt.Sprintf("http://user-service:8080/api/users/%d", userID))
//              and writes to kafka.Writer{Topic: "user.logged_in"}
```

**Service B (user-service):**
```go
// main.go: mux.HandleFunc("/api/users/{id}", GetUser).Methods("GET")
// handlers.go: GetUser does db.Query("SELECT ...")
```

**Service C (notification-service):**
```go
// main.go: kafka.NewReader(kafka.ReaderConfig{Topic: "user.logged_in"})
// handlers.go: HandleLogin processes messages
```

### 2. Test Infrastructure

Use real SQLite (temp files) consistent with existing test patterns. Each integration test:
1. Creates temp directories for service fixtures
2. Analyzes each service via `goAnalyzer.AnalyzeDirectory()`
3. Stores contexts via `SQLiteStore.StoreRepoContext()`
4. Registers services as an org via `orgStore.SaveOrg()`
5. Builds topology and runs traces
6. Cleans up temp files

### 3. Assertion Helpers

```go
func assertTopologyHasEdge(t *testing.T, topology *ServiceTopology, source, target, edgeType string)
func assertTraceContains(t *testing.T, trace *TraceNode, serviceName, funcName string)
func assertMermaidContains(t *testing.T, mermaid string, elements ...string)
```

## Error Handling

- Test fixture creation failure: t.Fatal with descriptive message
- Analysis failure on synthetic files: indicates a bug in analyzer — fail test
- Missing assertions: each test verifies specific outputs to catch regressions

## File Summary

| File | Action |
|------|--------|
| `internal/integration/flow_tracing_test.go` | New: End-to-end tests for full pipeline |
| `internal/integration/testdata/` | Synthetic Go source files for 3 services (may be generated at test time) |

## Implementation Order

1. Create test fixture helper
2. Write integration tests (all initially failing)
3. Tests should pass after all sections 01-09 are implemented
4. Add assertion helpers for common patterns
5. Run full test suite
