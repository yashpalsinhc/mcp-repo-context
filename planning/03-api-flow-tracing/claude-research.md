# Research: Cross-Service API Flow Tracing

## Codebase Research

### Current Static Analysis Pipeline
- `goAnalyzer.AnalyzeFile()` in `internal/analyzer/go_analyzer.go` parses Go source, then for each `*ast.FuncDecl` calls five sub-extractors: behavior, call graph, error handling, side effects, API flow
- `FunctionDef` has `APIFlow *APIFlow` field with `ExternalCalls []ExternalCall` and other flow data

### Side Effect Detection (`internal/analyzer/errorhandling.go`)
- `ExtractSideEffects()` does `ast.Inspect` matching `*ast.CallExpr` nodes
- `detectSideEffect()` switches on package names: `http` → `http_call`, `kafka` → `kafka_call`, `grpc` → `grpc_call`, etc.
- **Gap**: Only matches on `pkgName == "http"` or `strings.Contains(importPath, "http")`. Named HTTP clients (e.g., `s.userServiceClient.Get()`) are not recognized.

### Route/Handler Detection (`internal/analyzer/route_extractor.go`)
- `RouteExtractor.Extract()` detects Gin/Echo (uppercase HTTP methods), Chi (camelCase methods), and standard `http.HandleFunc`/`Handle`
- Gorilla mux NOT explicitly detected (no receiver prefix matching), though `HandleFunc`/`Handle` on any receiver will match standard case
- `Route` struct: Method, Path, Handler, File, Line, Description, Middleware
- Routes stored in `FileContext.Routes` and indexed in `SearchIndex.Routes map[string][]RouteRef`

### API Flow Data (`internal/context/types.go`)
- `APIFlow` struct: IsHTTPHandler, RequestPayload, ResponsePayload, Steps, DBQueries, ExternalCalls, ValidationSteps
- `ExternalCall`: Method, URL, Description, Line
- **Critical gap**: URL field only populated for string literals passed directly to calls. Variable-built URLs (fmt.Sprintf, constants) are empty.

### Existing External Call Extraction (`internal/analyzer/apiflow.go`)
- `extractExternalCall()` at line 526 only captures `*ast.BasicLit` string arguments
- `isHTTPHandler()` checks for ResponseWriter/Request param pattern or framework-specific contexts
- `ExtractRouteInfo()` guesses HTTP method from function name prefix (get→GET, create→POST)

### Storage Schema
- External calls stored as JSON in `functions.api_flow_json` column (opaque blob)
- Routes stored as JSON in `files.routes_json` column (opaque blob)
- Side effects indexed separately in `side_effects` table (queryable)
- **No normalized SQL columns** for external call URLs or route paths - cross-service matching requires loading full RepoContext objects

### Cross-Repo Operations
- No tool today links ExternalCall entries in one repo to Route entries in another
- `SearchableStore` methods are all single-repo scoped (`repoID` parameter mandatory)
- `comparison.Comparer` loads N RepoContext objects for cross-repo analysis

### Org Infrastructure (`internal/org/`)
- `Manager` interface with AnalyzeOrg (bounded concurrency, partial failure)
- `Org` struct: ID, Repos []string, Config OrgConfig
- OrgConfig only has ExcludePatterns and MaxFileSize - no service identity metadata

## Web Research

### Go AST for HTTP URL Extraction
- Function calls are `*ast.CallExpr` with `Fun` and `Args`
- String literals are `*ast.BasicLit` with `Kind == token.STRING`, need `strconv.Unquote()`
- Constants via `*ast.GenDecl` with `Tok == token.CONST`, track through `go/types` Uses/Defs maps
- fmt.Sprintf patterns: extract format string from first arg, parameter placeholders indicate dynamic parts

### Kafka Library Detection
- **segmentio/kafka-go**: `Writer.WriteMessages()` (producer), `Reader.ReadMessage()` (consumer). Topic in struct init `Writer{Topic: "name"}`
- **Sarama**: `AsyncProducer.SendMessage()`, `ConsumerGroup.Subscribe(topics)`. Topic in Subscribe args.
- **confluent-kafka-go**: `Producer.Produce(msg)`, `Consumer.Subscribe(topics)`. Topic in Message struct or Subscribe args.
- Topic names often in struct field initialization (`*ast.KeyValueExpr`) or string slice arguments

### Route Registration Patterns
- gorilla/mux: `router.HandleFunc("/path", handler).Methods("GET")` with `{id}` params and regex constraints
- chi: `router.Get("/path", handler)` with `{id}` params
- net/http (Go 1.22+): `mux.HandleFunc("GET /task/{id}", handler)` - method in pattern string
- gin: `router.GET("/path", handler)` with `:id` params

### URL Path Normalization (RFC 3986)
- Scheme/host: case-insensitive, lowercase
- Percent-encoding: decode unreserved characters
- Dot-segments: remove `.` and `..`
- Always normalize before matching

### Service Topology Graph
- Adjacency list model recommended for sparse microservice graphs
- Node types: Service, Topic/Queue
- Edge types: HTTP_REQUEST, KAFKA_PRODUCE, KAFKA_CONSUME, GRPC_CALL
- Distinguish sync vs async edges
- Support fan-out (one topic → multiple consumers) and fan-in
- Libraries: dominikbraun/graph for Go generics support

## Testing Patterns
- Existing tests use real SQLite with temp files
- Test repos: gorilla/mux, gorilla/handlers already analyzed
- Create synthetic multi-service test fixtures with known HTTP calls and Kafka usage
