# TDD Plan: Cross-Service API Flow Tracing

Testing follows Go standard `testing` package patterns. Tests use real SQLite (temp files), consistent with existing test patterns in the codebase.

## Section 1: Enhanced HTTP/gRPC Client Call Extraction

### File: `internal/analyzer/apiflow_test.go` (extend)

```go
// Test: extractExternalCall extracts URL from string literal
// Setup: AST with http.Get("https://api.example.com/users")
// Assert: ExternalCall.URL == "https://api.example.com/users"

// Test: extractExternalCall extracts URL from file-scope constant
// Setup: const baseURL = "https://api.example.com"; http.Get(baseURL)
// Assert: ExternalCall.URL == "https://api.example.com"

// Test: extractExternalCall extracts fmt.Sprintf URL template
// Setup: http.Get(fmt.Sprintf("https://api.example.com/users/%d", id))
// Assert: ExternalCall.URL == "https://api.example.com/users/{param}"

// Test: extractExternalCall handles string concatenation
// Setup: http.Get("https://api.example.com" + "/users/" + id)
// Assert: ExternalCall.URL contains "https://api.example.com/users/{dynamic}"

// Test: extractExternalCall marks dynamic URL
// Setup: http.Get(getURL())
// Assert: ExternalCall.URL == "<dynamic>", URLExpression contains "getURL()"

// Test: extractExternalCall extracts method from http.NewRequest
// Setup: http.NewRequest("POST", url, body)
// Assert: ExternalCall.Method == "POST"

// Test: HTTP client wrapper heuristic detects service field calls
// Setup: s.authService.ValidateToken(ctx, token)
// Assert: Detected as potential HTTP call with ServiceHint == "authService"

// Test: gRPC client detection from protobuf pattern
// Setup: pb.NewUserServiceClient(conn).GetUser(ctx, req)
// Assert: ExternalCall with Method="gRPC", URL="/UserService/GetUser"

// Test: ExternalCall ServiceHint derived from URL hostname
// Setup: http.Get("http://auth-service:8080/validate")
// Assert: ServiceHint == "auth-service"
```

## Section 2: Enhanced Route Registration Detection

### File: `internal/analyzer/route_extractor_test.go` (extend)

```go
// Test: detect gorilla/mux HandleFunc with Methods chain
// Setup: r.HandleFunc("/users/{id}", handler).Methods("GET")
// Assert: Route with Path="/users/{id}", Method="GET", Framework="gorilla/mux"

// Test: detect gorilla/mux HandleFunc without Methods (ANY)
// Setup: r.HandleFunc("/health", handler)
// Assert: Route with Method="ANY"

// Test: path parameter normalization strips regex
// Setup: r.HandleFunc("/users/{id:[0-9]+}", handler)
// Assert: Route.Path == "/users/{id}", Route.RawPath == "/users/{id:[0-9]+}"

// Test: gin colon params normalized to braces
// Setup: r.GET("/users/:id", handler)
// Assert: Route.Path == "/users/{id}"

// Test: Go 1.22+ pattern parsing
// Setup: mux.HandleFunc("GET /tasks/{id}", handler)
// Assert: Route.Method == "GET", Route.Path == "/tasks/{id}"

// Test: chi nested routing path concatenation
// Setup: r.Route("/api", func(r chi.Router) { r.Get("/users", handler) })
// Assert: Route.Path == "/api/users"

// Test: duplicate Gin/Echo blocks fixed (existing bug)
// Assert: No duplicate routes for same path

// Test: Route struct consolidation — context.Route has all fields
// Assert: context.Route includes RawPath, HandlerFile, Framework fields
```

## Section 3: Kafka Producer/Consumer Detection

### File: `internal/analyzer/kafka_extractor_test.go` (new)

```go
// Test: detect segmentio/kafka-go writer with topic in struct
// Setup: writer := &kafka.Writer{Topic: "user.events"}; writer.WriteMessages(ctx, msg)
// Assert: AsyncCall with Protocol="kafka", Direction="produce", Topic="user.events", Library="segmentio/kafka-go"

// Test: detect segmentio/kafka-go reader with topic
// Setup: reader := kafka.NewReader(kafka.ReaderConfig{Topic: "user.events"})
// Assert: AsyncCall with Direction="consume", Topic="user.events"

// Test: detect Sarama producer
// Setup: producer.SendMessage(&sarama.ProducerMessage{Topic: "orders"})
// Assert: AsyncCall with Direction="produce", Topic="orders", Library="sarama"

// Test: detect Sarama consumer group
// Setup: consumerGroup.Consume(ctx, []string{"orders", "payments"}, handler)
// Assert: Two AsyncCalls, one per topic, both Direction="consume"

// Test: detect confluent producer
// Setup: producer.Produce(&kafka.Message{TopicPartition: kafka.TopicPartition{Topic: &topicVar}}, nil)
// Assert: AsyncCall with Direction="produce", Topic="<dynamic>" (variable)

// Test: dynamic topic marked correctly
// Setup: writer := &kafka.Writer{Topic: getTopicName()}
// Assert: Topic == "<dynamic>", TopicExpr contains "getTopicName()"

// Test: kafka_produce and kafka_consume added to side effects
// Setup: function with producer call
// Assert: SideEffects contains "kafka_produce" (not just "kafka_call")

// Test: AsyncCalls populated on FunctionDef
// Setup: function with producer and consumer calls
// Assert: FunctionDef.AsyncCalls has 2 entries
```

## Section 4: Normalized Endpoint Storage

### File: `internal/storage/sqlite_test.go` (extend)

```go
// Test: StoreEndpoints and GetEndpoints round-trip
// Setup: Store 3 endpoints for repo A
// Assert: GetEndpoints returns all 3 with correct fields

// Test: StoreEndpoints replaces existing for same repo
// Setup: Store 3 endpoints, then store 2 endpoints for same repo
// Assert: GetEndpoints returns 2 (not 5)

// Test: StoreServiceCalls and GetServiceCalls round-trip
// Setup: Store 2 service calls for repo A
// Assert: GetServiceCalls returns both with correct fields

// Test: FindMatchingEndpoints returns cross-repo results
// Setup: Store endpoints for repo A and repo B, both with path "/api/users"
// Call: FindMatchingEndpoints("GET", "/api/users")
// Assert: Returns endpoints from both repos

// Test: FindMatchingConsumers finds Kafka consumers by topic
// Setup: Store kafka_consume calls for repos B and C with topic "user.events"
// Call: FindMatchingConsumers("user.events")
// Assert: Returns entries from both repos

// Test: Endpoint storage populated during StoreRepoContext
// Setup: Analyze a Go file with routes and external calls, store context
// Assert: endpoints and service_calls tables populated automatically

// Test: Tables created by ensureSchema migration
// Setup: Fresh database
// Assert: endpoints and service_calls tables exist
```

## Section 5: Cross-Service Endpoint Matching

### File: `internal/flow/matcher_test.go` (new)

```go
// Test: exact path match
// Setup: endpoint "/api/v1/users", call target "/api/v1/users"
// Assert: match with confidence "exact"

// Test: parameterized path match
// Setup: endpoint "/api/v1/users/{id}", call target "/api/v1/users/123"
// Assert: match with confidence "parameterized"

// Test: no match for different paths
// Setup: endpoint "/api/v1/users", call target "/api/v1/orders"
// Assert: no match

// Test: method filtering
// Setup: endpoint GET "/users", call method POST
// Assert: no match

// Test: service hint narrows ambiguous matches
// Setup: endpoint "/health" in repo A and repo B, call with service_hint="A"
// Assert: match from repo A ranked higher

// Test: Kafka topic exact match
// Setup: producer topic "user.events", consumer topic "user.events"
// Assert: match

// Test: URL normalization strips scheme and host
// Setup: call target "https://auth-service:8080/api/validate"
// Assert: normalized to "/api/validate" for matching

// Test: URL normalization handles trailing slashes
// Setup: endpoint "/api/users/", call "/api/users"
// Assert: match

// Test: path trie batch matching
// Setup: 100 endpoints, 50 service calls
// Assert: all matches found, performance acceptable
```

## Section 6: Service Topology Graph

### File: `internal/flow/topology_test.go` (new)

```go
// Test: BuildTopology creates nodes for all repos
// Setup: org with 3 repos
// Assert: 3 ServiceNode entries

// Test: BuildTopology creates HTTP edges from matched calls
// Setup: repo A calls repo B's /users endpoint
// Assert: ServiceEdge with EdgeType="http"

// Test: BuildTopology creates Kafka edges
// Setup: repo A produces "user.events", repo C consumes "user.events"
// Assert: Two edges: A→topic (produce), topic→C (consume)

// Test: fan-out produces multiple edges
// Setup: repo A produces, repos B and C both consume same topic
// Assert: 3 edges total

// Test: BuildTopology includes non-Go metadata nodes
// Setup: docker-compose.yml with non-Go service
// Assert: ServiceNode with IsDeepAnalyzed=false

// Test: topology caching and staleness
// Setup: build topology, re-analyze one repo
// Assert: cached topology marked stale

// Test: Mermaid output generation
// Setup: topology with sync and async edges
// Assert: solid lines for HTTP, dashed for Kafka
```

## Section 7: MCP Tools

### File: `internal/mcp/tools_test.go` (extend)

```go
// Test: toolTraceApiFlow traces HTTP call chain
// Setup: 3 services with A→B HTTP call
// Call: trace_api_flow with entry_point="POST /login"
// Assert: response contains trace through A and B

// Test: toolTraceApiFlow detects cycles
// Setup: A→B→A circular calls
// Assert: response contains "[cycle detected]"

// Test: toolTraceApiFlow respects max_depth
// Setup: A→B→C→D chain
// Call: with max_depth=2
// Assert: trace stops at C

// Test: toolTraceApiFlow includes Mermaid diagram
// Assert: response contains "```mermaid"

// Test: toolTraceApiFlow disambiguates multiple matches
// Setup: /login endpoint in 2 repos
// Call: without repo_id
// Assert: response lists both with disambiguation prompt

// Test: toolGetServiceMap returns topology
// Setup: org with 3 services
// Assert: response contains service list and Mermaid graph

// Test: toolGetServiceMap rebuild flag
// Setup: stale cached topology
// Call: with rebuild=true
// Assert: fresh topology built

// Test: toolGetServiceMap error for unknown org
// Assert: error response
```

## Section 8: Multi-Language Detection

### File: `internal/analyzer/multiservice_detector_test.go` (new)

```go
// Test: parse docker-compose.yml extracts service names
// Setup: docker-compose.yml with 3 services
// Assert: 3 ServiceNode entries with names matching service block keys

// Test: parse docker-compose.yml extracts Kafka topics from env vars
// Setup: environment: KAFKA_TOPIC=user.events
// Assert: ServiceNode linked to topic "user.events"

// Test: parse docker-compose.yml extracts service URLs from env vars
// Setup: environment: AUTH_SERVICE_URL=http://auth:8080
// Assert: Detected as calling "auth" service

// Test: parse package.json detects Kafka dependency
// Setup: package.json with "kafkajs" in dependencies
// Assert: service marked as Kafka participant

// Test: handles missing docker-compose.yml gracefully
// Assert: no error, no nodes created
```

## Section 9: Enhanced get_pr_context

### File: `internal/mcp/tools_test.go` (extend)

```go
// Test: PR context shows upstream callers for modified handler
// Setup: handler /users/{id} modified, service A calls this endpoint
// Assert: Cross-Service Impact section lists service A as upstream

// Test: PR context shows downstream impact for modified client call
// Setup: function calling /auth/validate modified
// Assert: Cross-Service Impact section lists auth-service as downstream

// Test: PR context with no cross-service impact
// Setup: modified function has no external calls or registered routes
// Assert: No Cross-Service Impact section (or "none detected")
```

## Section 10: Integration Tests

### File: `internal/integration/flow_tracing_test.go` (new)

```go
// Test: full pipeline — analyze 3 services, build topology, trace flow
// Test: trace_api_flow from /login shows A→B(HTTP) and A→C(Kafka) path
// Test: get_service_map shows all 3 services with correct edges
// Test: PR context for modified handler shows upstream services
// Test: dynamic URL marked as <dynamic> with expression
// Test: fan-out: one topic, two consumers
// Test: circular dependency detected and reported
// Test: vocabulary of path segments used correctly in matching
```
