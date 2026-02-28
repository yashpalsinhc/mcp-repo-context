# Implementation Plan: Cross-Service API Flow Tracing

## Overview

This plan adds cross-service API flow tracing to the MCP repo-context server. The system traces how HTTP, gRPC, and Kafka calls flow between services across multiple repositories in an organization. Given a starting endpoint like "POST /login in service A", it produces a complete trace: "Request enters LoginHandler → calls service B's /auth/validate → publishes to Kafka topic user.verified → consumed by service C's VerificationHandler."

The implementation enhances the existing Go static analyzer to extract richer endpoint and topic data, adds normalized SQLite tables for cross-repo querying, builds a service topology graph, and exposes two new MCP tools: `trace_api_flow` and `get_service_map`.

## Current Architecture

### What Exists

**Static Analysis** (`internal/analyzer/`):
- `goAnalyzer.AnalyzeFile()` runs five sub-extractors per function: behavior, call graph, error handling, side effects, API flow
- `ExternalCall` struct captures HTTP method, URL, description, line number — but URL is only populated for string literals
- `RouteExtractor` detects Gin/Echo, Chi, and standard `http.HandleFunc` patterns — but NOT gorilla/mux explicitly
- Side effect detection catches `http_call`, `kafka_call`, `grpc_call` as string labels without details
- `APIFlow` struct has `ExternalCalls []ExternalCall` but only basic URL extraction

**Storage**:
- Routes stored as JSON blob in `files.routes_json` column — not queryable via SQL
- External calls stored as JSON blob in `functions.api_flow_json` — not queryable via SQL
- Side effects indexed in separate `side_effects` table with `(function_id, effect)` pairs
- All `SearchableStore` methods require `repoID` parameter — no cross-repo queries

**Org Infrastructure** (`internal/org/`):
- `Manager` interface with `AnalyzeOrg` (bounded concurrency, partial failure)
- `Org` struct with ID, Repos, Config — but OrgConfig has no service metadata
- No cross-repo endpoint or route indexing

### What's Missing

1. **URL extraction beyond string literals** — fmt.Sprintf patterns, constants, package-level vars
2. **Gorilla/mux route detection** — the most popular Go router is not properly detected
3. **Kafka topic extraction** — kafka_call side effect exists but no topic name capture
4. **Normalized endpoint storage** — routes and external calls are JSON blobs, can't join across repos
5. **Cross-repo endpoint matching** — no tool or query links client URLs to server routes across repos
6. **Service topology graph** — no service-level view of the org
7. **Multi-language service awareness** — non-Go services invisible in service maps

## Design Decisions

### D1: Normalized SQLite Tables for Endpoints
Instead of matching in-memory (loading all RepoContexts), we add two normalized SQLite tables: `endpoints` (server routes) and `service_calls` (client HTTP/gRPC/Kafka calls). This enables efficient SQL joins across repos and scales to orgs with 50+ repos. The existing JSON blobs remain for backward compatibility; the new tables are populated alongside them during analysis.

### D2: HTTP + gRPC + Kafka Only (No NATS/RabbitMQ/SQS)
The initial implementation covers HTTP, gRPC, and Kafka. These are the most common communication patterns. NATS, RabbitMQ, and SQS are deferred.

### D3: Mermaid Only for Visualization
Service maps and flow traces produce Mermaid sequence diagrams and flowcharts. DOT/Graphviz is deferred.

### D4: Basic Multi-Language via Config Parsing
Non-Go services are detected by parsing docker-compose.yml (service definitions, environment variables with topic names) and package.json (Kafka client dependencies). They appear as metadata-only nodes in the service map.

### D5: Endpoint Matching Strategy
URL matching uses path-template normalization. Client URL `/api/v1/users/123` matches server route `/api/v1/users/{id}`. Path parameters across frameworks are normalized: gorilla `{id}`, chi `{id}`, gin `:id` all become `{param}` internally. Kafka matching is by exact topic name. When multiple endpoints match a client URL, use service hints (from URL hostname or variable names) to rank results. Include confidence levels on all matches.

### D6: Route Struct Consolidation
Two `Route` structs exist: `analyzer.Route` and `context.Route`. Section 2 must consolidate these by ensuring `context.Route` includes all fields from the enhanced `analyzer.Route`, with explicit mapping during the analysis pipeline.

### Known Limitations
- **File-scoped constant resolution only**: URL/topic constants defined in other files (e.g., `constants.go`) are not resolved. Per-package constant indexing is a follow-up.
- **HTTP client wrapper detection is heuristic**: Struct fields with names containing "client", "service", or "api" are detected as potential HTTP/gRPC clients, but custom wrappers with non-obvious names will be missed.
- **gRPC stored clients**: `s.userClient.GetUser()` patterns require type-aware analysis beyond current AST-only approach. Partially covered by the naming heuristic.
- **Kafka consumer group handlers**: Topic names in Sarama `ConsumerGroupHandler` implementations are in the caller, not the handler. These may be missed if the caller is in a different file.

## Section-by-Section Plan

### Section 1: Enhanced HTTP/gRPC Client Call Extraction

**Goal:** Improve the analyzer's ability to extract endpoint URLs from HTTP and gRPC client calls.

**Changes to `internal/analyzer/apiflow.go`:**

Enhance `extractExternalCall()` to handle more URL patterns beyond `*ast.BasicLit`:

1. **Constants**: When the URL argument is an `*ast.Ident`, resolve it by searching the file's `*ast.GenDecl` constant blocks. If the constant value is a string literal, use it.

2. **fmt.Sprintf patterns**: When the call is `fmt.Sprintf(format, args...)`, extract the format string (first arg, must be BasicLit). Replace `%s`, `%d`, `%v` with `{param}` placeholders. Result: `"https://api.example.com/users/{param}"`.

3. **String concatenation**: When the URL argument is a `*ast.BinaryExpr` with `Op == token.ADD`, recursively resolve both sides. If one side is a literal, capture it. Unresolvable parts become `{dynamic}`.

4. **http.NewRequest patterns**: For `http.NewRequest(method, url, body)`, extract the method (first arg) as well as the URL (second arg).

5. **Mark unresolvable URLs**: When URL cannot be statically determined, set URL to `"<dynamic>"` and store the variable name/expression in a new `URLExpression string` field on `ExternalCall`.

6. **HTTP client wrapper heuristic**: Detect method calls on struct fields whose names contain "client", "service", or "api" (case-insensitive) as potential HTTP/gRPC calls. For example, `s.authService.ValidateToken(ctx, token)` would be flagged. Mark these with lower confidence and store the receiver field name as `ServiceHint`.

**New field on ExternalCall:**
```go
type ExternalCall struct {
    Method        string  // GET, POST, etc.
    URL           string  // extracted URL or template
    URLExpression string  // original expression when URL is dynamic
    Description   string
    Line          int
    ServiceHint   string  // guessed service name from URL prefix or variable name
}
```

**gRPC client detection:**
- Detect `pb.NewXxxClient(conn).MethodName(ctx, req)` patterns by looking for calls on types ending in `Client` with protobuf imports
- Extract service name (from type name, e.g., `UserServiceClient` → `UserService`) and method name
- Store as ExternalCall with Method="gRPC", URL="/ServiceName/MethodName"

### Section 2: Enhanced Route Registration Detection

**Goal:** Improve route extraction to handle gorilla/mux and more framework patterns.

**Route struct consolidation:** The codebase has two `Route` structs — `analyzer.Route` (in `route_extractor.go`) and `context.Route` (in `types.go`). Consolidate by making `context.Route` the canonical type with all new fields, and have the analyzer produce `context.Route` directly. Remove the duplicate `analyzer.Route`.

**Fix existing bug:** The route extractor has duplicate detection blocks for Gin/Echo (lines 58-61 and 64-67 are identical). The second block is dead code. Fix as part of this section.

**Changes to `internal/analyzer/route_extractor.go`:**

1. **Gorilla/mux detection**: Add explicit detection for the gorilla/mux chaining pattern. When we see `receiver.HandleFunc(path, handler)`, check if the receiver type imports `github.com/gorilla/mux`. Also detect `.Methods("GET", "POST")` chain calls — walk the call chain to find Methods() and extract the method list.

2. **Chi nested routing**: Detect `r.Route("/prefix", func(r chi.Router) { r.Get("/sub", handler) })`. Track the prefix from the outer `Route()` call and prepend to inner paths.

3. **Go 1.22+ net/http patterns**: Parse pattern strings like `"GET /task/{id}"` to extract method and path separately.

4. **Path parameter normalization**: Normalize all parameter syntax to `{param}`:
   - gorilla `{id}` → `{id}` (already correct)
   - gorilla `{id:[0-9]+}` → `{id}` (strip regex)
   - chi `{id}` → `{id}`
   - gin `:id` → `{id}`

**New field on Route:**
```go
type Route struct {
    Method      string
    Path        string         // normalized path with {param} placeholders
    RawPath     string         // original path as written in code
    Handler     string         // handler function name
    HandlerFile string         // file containing the handler
    File        string         // file containing the route registration
    Line        int
    Framework   string         // "gorilla/mux", "chi", "net/http", "gin", "echo"
    Middleware  []string
    Description string
}
```

### Section 3: Kafka Producer/Consumer Detection

**Goal:** Extract Kafka topic names and classify producer vs consumer patterns.

**New file: `internal/analyzer/kafka_extractor.go`**

Create a `KafkaExtractor` that detects Kafka patterns during AST analysis:

**Producer detection:**
- segmentio/kafka-go: `writer.WriteMessages(ctx, kafka.Message{...})` or `Writer{Topic: "name"}` initialization
- Sarama: `producer.SendMessage(&sarama.ProducerMessage{Topic: "name", ...})`
- confluent-kafka-go: `producer.Produce(&kafka.Message{TopicPartition: kafka.TopicPartition{Topic: &topic}}, nil)`

**Consumer detection:**
- segmentio/kafka-go: `kafka.NewReader(kafka.ReaderConfig{Topic: "name"})` or `reader.ReadMessage(ctx)`
- Sarama: `consumerGroup.Consume(ctx, []string{"topic"}, handler)`
- confluent-kafka-go: `consumer.SubscribeTopics([]string{"topic"}, nil)`

**Topic name extraction:**
- From struct literal fields: walk `*ast.CompositeLit` to find `Topic` key
- From string slice arguments: walk `*ast.CompositeLit` for `[]string{...}` elements
- From variables: attempt constant resolution (same as HTTP URL constants)
- Mark dynamic topics as `"<dynamic>"` with expression info

**New type:**
```go
type AsyncCall struct {
    Protocol    string   // "kafka"
    Direction   string   // "produce" or "consume"
    Topic       string   // topic name or "<dynamic>"
    TopicExpr   string   // original expression for dynamic topics
    Library     string   // "segmentio/kafka-go", "sarama", "confluent"
    MessageType string   // Go type of message payload if determinable
    Line        int
    File        string
    Function    string   // function containing this call
}
```

**Integration with analyzer pipeline:**
- Add `AsyncCalls []AsyncCall` field to `FunctionDef`
- Call `KafkaExtractor.Extract(decl, imports)` alongside existing extractors in `AnalyzeFile()`
- Also add to side effects: `"kafka_produce"` and `"kafka_consume"` (more specific than current `"kafka_call"`)

### Section 4: Normalized Endpoint Storage (SQLite)

**Goal:** Add normalized SQLite tables for routes and service calls, enabling cross-repo joins.

**New migration:**

Table `endpoints` — server-side routes:
```
endpoints (
    id INTEGER PRIMARY KEY,
    repo_id TEXT NOT NULL,
    file_path TEXT NOT NULL,
    handler_name TEXT NOT NULL,
    method TEXT NOT NULL,        -- GET, POST, ANY, gRPC
    path TEXT NOT NULL,          -- normalized path with {param}
    raw_path TEXT,               -- original as written
    framework TEXT,              -- gorilla/mux, chi, net/http, gin
    line INTEGER,
    UNIQUE(repo_id, file_path, handler_name, method, path)
)
CREATE INDEX idx_endpoints_path ON endpoints(path);
CREATE INDEX idx_endpoints_repo ON endpoints(repo_id);
```

Table `service_calls` — client-side HTTP/gRPC/Kafka calls:
```
service_calls (
    id INTEGER PRIMARY KEY,
    repo_id TEXT NOT NULL,
    file_path TEXT NOT NULL,
    function_name TEXT NOT NULL,
    call_type TEXT NOT NULL,     -- "http", "grpc", "kafka_produce", "kafka_consume"
    method TEXT,                 -- GET, POST, gRPC, produce, consume
    target TEXT NOT NULL,        -- URL template or topic name
    target_expression TEXT,      -- original expression for dynamic targets
    service_hint TEXT,           -- guessed destination service
    line INTEGER,
    UNIQUE(repo_id, file_path, function_name, call_type, target, line)
)
CREATE INDEX idx_service_calls_target ON service_calls(target);
CREATE INDEX idx_service_calls_repo ON service_calls(repo_id);
CREATE INDEX idx_service_calls_type ON service_calls(call_type);
```

**Storage methods on SQLiteStore:**
- `StoreEndpoints(ctx, repoID string, endpoints []Endpoint) error` — delete existing for repo, insert new batch
- `StoreServiceCalls(ctx, repoID string, calls []ServiceCall) error` — same pattern
- `GetEndpoints(ctx, repoID string) ([]Endpoint, error)`
- `GetServiceCalls(ctx, repoID string) ([]ServiceCall, error)`
- `FindMatchingEndpoints(ctx, method, pathPattern string) ([]Endpoint, error)` — cross-repo query
- `FindMatchingConsumers(ctx, topic string) ([]ServiceCall, error)` — cross-repo Kafka query
- `FindMatchingProducers(ctx, topic string) ([]ServiceCall, error)`

**Migration strategy:** Follow the existing pattern in `SQLiteStore.ensureSchema()` which uses `CREATE TABLE IF NOT EXISTS`. Add the new tables in the same idempotent migration block. No separate migration files needed — matches the existing approach.

**Populate during analysis:** After `AnalyzeFile()` completes for each file, walk `FileContext.Routes` to populate the `endpoints` table, and walk `FileContext.Functions[*].APIFlow.ExternalCalls` plus `FileContext.Functions[*].AsyncCalls` to populate `service_calls`. This extraction happens in `StoreRepoContext` — add a new `storeEndpointIndex(ctx, repoID, fileCtx)` helper that is called per-file during the existing storage loop.

**Backward compatibility:** New fields on `ExternalCall` (`URLExpression`, `ServiceHint`) are stored as JSON. Existing stored data will simply have these fields as zero values when deserialized, which is correct.

### Section 5: Cross-Service Endpoint Matching

**Goal:** Build the matching engine that connects client calls to server endpoints across repos.

**New file: `internal/flow/matcher.go`**

`EndpointMatcher` struct with methods:

**HTTP matching algorithm:**
1. For a given service_call with `call_type="http"` and target URL template
2. Normalize the URL: strip scheme/host, extract path, normalize parameters
3. Query `endpoints` table for matching path patterns
4. Path matching: split both into segments, compare segment by segment
   - Literal segments must match exactly
   - Parameter segments (`{param}`) match any literal segment
   - `/api/v1/users/{param}` matches client URL `/api/v1/users/123`
5. If HTTP method is specified, filter by method
6. **Service hint filtering**: When the client call has a `service_hint` (from URL hostname or variable name), prefer matches from repos whose name or service name matches the hint. This prevents false positives where common paths like `/health` or `/api/v1/status` match every service.
7. Return ranked matches with confidence levels:
   - "exact": all segments match literally
   - "parameterized": matched via parameter substitution
   - "hint-matched": service hint narrows to single candidate
   - "ambiguous": multiple matches without hint to disambiguate

**Kafka matching algorithm:**
1. For a given service_call with `call_type="kafka_produce"` and topic name
2. Query service_calls table for entries with `call_type="kafka_consume"` and matching topic
3. Exact topic name match
4. Return all consumers for the topic (fan-out support)

**gRPC matching:**
1. For a gRPC call with target `/ServiceName/MethodName`
2. Query endpoints where method="gRPC" and path matches
3. gRPC paths are exact matches (no parameterization)

**URL normalization function:**
- Strip scheme (`https://`) and host (`service-b.internal`)
- Lowercase the path
- Remove trailing slashes
- Decode percent-encoded characters (RFC 3986)
- Remove dot-segments (`.`, `..`)
- Replace consecutive slashes with single slash

### Section 6: Service Topology Graph

**Goal:** Build and store a service-level topology graph from matched endpoints.

**New file: `internal/flow/topology.go`**

Types:
```go
type ServiceNode struct {
    RepoID      string
    ServiceName string   // derived from repo name or docker-compose service name
    Language    string   // "go", "typescript", "python", "unknown"
    Endpoints   int      // count of registered routes
    IsDeepAnalyzed bool  // true for Go repos with full analysis
}

type ServiceEdge struct {
    Source      string   // source service name
    Target      string   // target service name or topic name
    EdgeType    string   // "http", "grpc", "kafka_produce", "kafka_consume"
    Method      string   // HTTP method or produce/consume
    Path        string   // URL path or topic name
    SourceFunc  string   // function making the call
    TargetFunc  string   // handler function (if matched)
    Confidence  string   // "exact", "parameterized", "prefix", "unmatched"
}

type ServiceTopology struct {
    OrgID    string
    Nodes    []ServiceNode
    Edges    []ServiceEdge
    BuiltAt  time.Time
}
```

**BuildTopology method:**
1. Get all repos in org
2. For each Go repo: create ServiceNode with endpoint count from `endpoints` table
3. For non-Go repos (detected via basic multi-lang, Section 8): create metadata-only nodes
4. **Batch matching**: Load all endpoints into an in-memory path trie (keyed by normalized path segments). For each service_call across all repos, match against the trie. This avoids O(N*M) SQL queries — one bulk load + in-memory matching is much faster for orgs with many repos.
5. Create ServiceEdge for each match
6. Return ServiceTopology

**Store topology in SQLite:**
- Table `service_topologies` with `org_id`, `topology_json`, `built_at`
- Rebuilt on `analyze_org` or explicit tool call
- Cache with staleness based on repo analysis timestamps

### Section 7: MCP Tools (trace_api_flow, get_service_map)

**Goal:** Two new MCP tools for querying the flow data.

**Tool: `trace_api_flow`**

```
Name: "trace_api_flow"
Description: "Trace how an API call flows across services"
Parameters:
  - org_id (string, required)
  - entry_point (string, required): "POST /login" or "ServiceA:LoginHandler"
  - max_depth (integer, optional, default 5): max hops to trace
```

Handler flow:
1. Parse entry_point:
   - `"METHOD /path"` format (e.g., "POST /login"): query `endpoints` table filtered by org repos. If multiple matches, require `repo_id` parameter or return all matches with disambiguation prompt.
   - `"repo_id:function_name"` format (e.g., "github.com/org/auth-service:LoginHandler"): lookup function directly in the repo context.
   - `"service_name:function_name"` format: resolve service name to repo_id via ServiceTopology node mapping.
2. Find the starting endpoint and load the handler function's ExternalCalls and AsyncCalls
3. For each outbound call, find the matching endpoint/consumer (Section 5)
4. **Cycle detection**: Maintain a visited set of `(repoID, functionName)` tuples. If a trace would visit an already-seen function, report it as `"[cycle detected: ServiceA → ServiceB → ServiceA]"` and stop that branch.
5. Recursively trace from each matched handler up to max_depth
6. Build a trace tree with each hop showing: service → method → endpoint → handler function → confidence level
7. Generate Mermaid sequence diagram from the trace
8. Return formatted text with trace details and Mermaid diagram

**Sample output:**
```
## API Flow Trace: POST /login

### Trace
1. **auth-service** → POST /login → `LoginHandler` (auth/handler.go:45)
   ├─ HTTP GET /api/v1/users/{id} → **user-service** `GetUser` (user/handler.go:12) [exact]
   ├─ Kafka produce → topic: user.logged_in [exact]
   │  └─ **notification-service** `HandleUserLogin` (notify/consumer.go:30) [exact]
   └─ HTTP POST /audit/log → **audit-service** `LogEvent` (audit/handler.go:8) [parameterized]

### Unmatched Calls
- HTTP GET <dynamic> (auth/handler.go:67) - URL expression: `s.configService.GetURL()`

### Mermaid
` ` `mermaid
sequenceDiagram
    participant A as auth-service
    participant B as user-service
    participant K as Kafka: user.logged_in
    participant C as notification-service
    A->>B: GET /api/v1/users/{id}
    A->>K: produce user.logged_in
    K-->>C: consume user.logged_in
` ` `
```

**Tool: `get_service_map`**

```
Name: "get_service_map"
Description: "Get the service topology for an organization"
Parameters:
  - org_id (string, required)
  - rebuild (boolean, optional, default false): force rebuild of topology
```

Handler flow:
1. Load or build ServiceTopology for org
2. Format as text summary: list of services with endpoint counts, interconnections
3. Generate Mermaid flowchart showing service nodes and edges
4. Distinguish sync (solid lines) vs async (dashed lines) edges
5. Return formatted text with summary and Mermaid diagram

**Sample output:**
```
## Service Map: github.com/org

### Services (4)
| Service | Language | Endpoints | Calls Out | Calls In |
|---------|----------|-----------|-----------|----------|
| auth-service | Go | 5 | 3 HTTP, 1 Kafka | 2 HTTP |
| user-service | Go | 8 | 1 HTTP | 4 HTTP |
| notification-service | Go | 2 | 0 | 1 Kafka |
| frontend | TypeScript | - | - | - (metadata only) |

### Mermaid
` ` `mermaid
graph LR
    A[auth-service] -->|HTTP| B[user-service]
    A -.->|Kafka: user.logged_in| C[notification-service]
    B -->|HTTP| A
` ` `
```

**Topology invalidation:**
- When a single repo is re-analyzed, mark the cached topology as stale (don't auto-rebuild — let `get_service_map` with `rebuild=true` or next explicit call rebuild it)
- When a repo is removed from an org, delete its `endpoints` and `service_calls` rows, mark topology stale
- `StoreEndpoints` and `StoreServiceCalls` use delete-then-insert per repo, which is safe since the topology is rebuilt from tables

### Section 8: Basic Multi-Language Service Detection

**Goal:** Detect non-Go services in the org to create metadata-only nodes in the service map.

**New file: `internal/analyzer/multiservice_detector.go`**

**docker-compose.yml parsing:**
1. Parse YAML for `services` block
2. For each service: extract service name, image, environment variables
3. Scan environment variables for topic-name patterns (e.g., `KAFKA_TOPIC=user.events`, `TOPIC_NAME=orders`)
4. Scan environment variables for service URL patterns (e.g., `AUTH_SERVICE_URL=http://auth-service:8080`)
5. Create ServiceNode entries for non-Go services with detected topics/URLs

**package.json parsing:**
1. Check for Kafka client packages in dependencies: `kafkajs`, `node-rdkafka`, `@nestjs/microservices`
2. If found, mark service as a potential Kafka participant
3. Check for HTTP client packages: `axios`, `node-fetch`, `got`

**Integration:** When building service topology (Section 6), also scan the org's repos for docker-compose.yml and package.json files. Create ServiceNode entries with `IsDeepAnalyzed=false` for non-Go services detected this way.

### Section 9: Enhanced get_pr_context

**Goal:** Add cross-service impact information to PR context.

**Changes to `internal/mcp/tools.go` (toolGetPRContext):**

After computing the standard PR context (function changes, callers, callees):
1. For each modified handler function: query `service_calls` to find which services call this endpoint ("upstream impact")
2. For each modified function with ExternalCalls: query `endpoints` to find which services handle the destination ("downstream impact")
3. Add a "Cross-Service Impact" section to the output showing:
   - Upstream services that call modified endpoints
   - Downstream services affected by modified client calls
   - Risk level based on number of affected services

### Section 10: Integration Tests

**Goal:** End-to-end tests for the full flow tracing pipeline.

**Test fixtures:**
- Create a synthetic multi-service Go project with 3 services:
  - Service A: HTTP handler for `/login`, calls Service B's `/auth/validate`, publishes to Kafka `user.logged_in`
  - Service B: HTTP handler for `/auth/validate`, queries DB
  - Service C: Kafka consumer for `user.logged_in`, calls Service A's `/users/{id}` (circular dependency)

**Test scenarios:**
1. Analyze all 3 services, build topology → verify all edges detected
2. `trace_api_flow` from `POST /login` → verify full trace A→B and A→C(Kafka)
3. `get_service_map` → verify Mermaid output contains all services and edges
4. PR context for modified handler → verify upstream callers shown
5. Dynamic URL handling → verify `<dynamic>` placeholder and expression captured
6. Fan-out: one topic → two consumers → verify both edges in topology
7. Circular dependency detection → verify detected and reported

## Error Handling

- If org has no analyzed repos: return error suggesting `analyze_org` first
- If endpoint matching finds no matches: include "unmatched" entries in output with the URL/topic
- If trace exceeds max_depth: truncate with "[max depth reached]" indicator
- If topology build fails for one repo: continue with others (partial topology)
- Dynamic URLs that can't be resolved: store with `<dynamic>` marker, included in topology as unmatched

## Performance Considerations

- Endpoint matching uses SQL indexes on `path` and `target` columns — avoids loading full contexts
- Topology is cached in `service_topologies` table — rebuilt only on explicit request or after re-analysis
- Trace depth is bounded (default 5) to prevent runaway recursion
- Concurrent analysis during `analyze_org` populates endpoint tables alongside main analysis
