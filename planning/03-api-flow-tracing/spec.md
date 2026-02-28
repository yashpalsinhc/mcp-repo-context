# Spec: Cross-Service API Flow Tracing

## Purpose

The "killer feature" — trace how calls flow between services across multiple repos, including **synchronous** (HTTP, gRPC) and **asynchronous** (Kafka, RabbitMQ, NATS, SQS) communication. When a user asks "what happens when someone hits /login?", the server should show: "Request enters service A's LoginHandler, which calls service B's /auth/validate endpoint, which publishes to Kafka topic `user.verified`, which is consumed by service C's VerificationHandler."

## Background

See `planning/mcp-server-gaps-requirements.md` and `planning/deep_project_interview.md`. The user wants **full API flow maps** — end-to-end tracing of request paths across services. The existing `search_by_side_effect(effect="http_call")` finds functions that make HTTP calls, but doesn't connect them to destination handlers.

**Critical constraint:** Repos in an org can be written in **different languages** (Go, TypeScript, Python, Java, etc.). While deep analysis is Go-only for now, the flow tracing architecture must support multi-language service maps where some nodes have deep analysis and others have metadata-only analysis. Communication between services is often **async through Kafka or other message queues**, not just request-response HTTP/gRPC.

## Scope

### 1. Detect HTTP Client Calls in Go Code (Static Analysis)
**Current state:** The Go analyzer detects `http_call` as a side effect but doesn't extract the URL/endpoint being called.

**Required:**
- Detect standard library HTTP calls: `http.Get(url)`, `http.Post(url, ...)`, `http.NewRequest(method, url, ...)`
- Detect common HTTP client patterns: `client.Do(req)`, custom client wrappers
- Extract URL/endpoint when statically determinable (string literals, constants, fmt.Sprintf patterns)
- Store: function name, file, line, HTTP method, URL pattern, request/response types if available
- Handle cases where URL is constructed dynamically (mark as "dynamic URL")

### 2. Detect gRPC Client Calls
**Required:**
- Detect gRPC client calls: `pb.NewXxxClient(conn).MethodName(ctx, req)`
- Extract service name and method name from protobuf-generated client stubs
- Store: service, method, request type, response type

### 2b. Detect Async Message Producers (Kafka, RabbitMQ, NATS, SQS)
**Required:**
- Detect Kafka producer calls in Go: `producer.Produce(topic, msg)`, `writer.WriteMessages(ctx, kafka.Message{Topic: "..."})` (segmentio/kafka-go, confluent-kafka-go, Sarama)
- Detect RabbitMQ publish calls: `channel.Publish(exchange, routingKey, ...)`
- Detect NATS publish calls: `nc.Publish(subject, data)`
- Detect AWS SQS/SNS publish calls
- Extract: topic/queue/subject name, message type/schema if determinable, publish function location
- Store as a new side effect type: `kafka_call`, `amqp_call`, `nats_call`, `sqs_call` (or unified `async_publish`)
- Handle cases where topic name is dynamic (variable, config-driven) — mark as "dynamic topic"

### 2c. Detect Async Message Consumers
**Required:**
- Detect Kafka consumer group handlers: `consumer.Subscribe(topic, handler)`, `reader.ReadMessage(ctx)`, consumer group loops
- Detect RabbitMQ consumers: `channel.Consume(queue, ...)`
- Detect NATS subscriptions: `nc.Subscribe(subject, handler)`
- Detect SQS polling patterns
- Extract: topic/queue/subject being consumed, handler function, message type
- Map consumer handler functions to their topic subscriptions (similar to how HTTP handlers map to routes)

### 3. Detect HTTP Server Handlers
**Current state:** The analyzer detects `ServeHTTP` implementations and handler functions but doesn't extract route registrations.

**Required:**
- Parse route registrations: `mux.HandleFunc("/path", handler)`, `router.Handle("/path", handler)`
- Support gorilla/mux-style routing: `r.HandleFunc("/users/{id}", handler).Methods("GET")`
- Support net/http default mux: `http.HandleFunc("/path", handler)`
- Extract: path pattern, HTTP methods, handler function reference
- Map handler functions to their route registrations

### 4. Cross-Repo Endpoint & Topic Matching
**Required:**
- For each HTTP client call with a known URL pattern, search all analyzed repos for matching server handlers
- **For each async producer with a known topic, search all analyzed repos for matching consumers on that topic**
- Match HTTP by URL path pattern (exact and parameterized)
- **Match async by topic/queue/subject name (exact match, wildcard patterns for NATS)**
- Use dependency graph (from split 02) to narrow search to repos that are actually related
- Handle partial matches and suggest possible matches when exact match fails
- **Handle multi-language orgs:** If a Go service publishes to Kafka topic "user.created" and a TypeScript service consumes it, the flow should still be visible even if the TS service only has metadata-level analysis. Use config file parsing (package.json, docker-compose.yml, env vars) to identify topic subscriptions in non-Go services when deep analysis isn't available.

### 5. Build Service-to-Service Flow Maps
**Required:**
- Combine client calls + server handlers into a directed graph: Service A → (HTTP GET /users/{id}) → Service B
- **Include async edges:** Service A → (Kafka: user.created) → Service C
- **Distinguish sync vs async edges** — async edges are fundamentally different (fire-and-forget, eventual consistency, fan-out possible)
- Support multi-hop tracing: A → B → C → D (including mixed sync+async hops)
- **Support fan-out:** One producer → multiple consumers on same topic
- Detect circular dependencies
- Store flow maps per org/repo-group

### 6. New Tool: trace_api_flow
**Required:**
- Input: starting endpoint (e.g., "POST /login" in repo X) or function name
- Output: full flow trace showing each service hop, handler function, downstream calls
- Include request/response types at each hop if available
- Mermaid visualization of the flow

### 7. New Tool: get_service_map
**Required:**
- Input: org_id or list of repo_ids
- Output: all services, their endpoints, and which services call which
- Overview format: list of services with endpoint counts and interconnection summary
- Mermaid visualization of the service topology

### 8. Enhance get_pr_context
**Required:**
- When a PR modifies a handler function, show which upstream services call this endpoint
- When a PR modifies an HTTP client call, show which downstream service handles it
- Add "cross-service impact" section to PR context output

## Dependencies

- **02-dependency-graph:** Need inter-repo dependency data to know which repos are related services
- **01-core-bug-fixes:** Reliable call graph extraction for tracing internal function chains

## Provides to Other Splits

- **06-agent-recipes:** API flow data for `explain_api_flow` and `analyze_pr_impact` recipes
- **05-service-layer:** Service map data for org-level dashboard

## Key Technical Challenges

1. **Static URL extraction:** Many URLs are constructed dynamically. Need heuristics for common patterns (base URL + path, fmt.Sprintf, url.JoinPath).
2. **Cross-repo matching:** URL patterns in client code must match route patterns in server code. Need normalization for path parameters ({id}, :id, etc.).
3. **Multi-repo scale:** With 50-200 repos, matching needs to be efficient. Consider pre-building an endpoint index.
4. **Accuracy vs coverage:** Some flows will be unprovable statically. Need clear confidence indicators.
5. **Async topic names are often config-driven:** Kafka topic names may come from env vars, config files, or constants. Need heuristics to extract from common patterns (viper config, env var lookups, const blocks).
6. **Multi-language orgs:** A Go service might publish to Kafka, consumed by a Python/TS/Java service. The flow map must work even when only one side has deep analysis. Use docker-compose.yml, Kubernetes manifests, or CI configs to infer service-to-topic relationships for non-Go services.
7. **Fan-out/fan-in patterns:** One topic with multiple consumers creates branching flows. One consumer reading from multiple topics creates merging flows.

## Research from Interview

- **Industry:** OpenTelemetry is the standard for runtime tracing. Our approach is static analysis (no runtime needed), which is unique and valuable because it works on code that isn't deployed yet.
- **Greptile:** Builds dependency graphs but doesn't trace API flows between services.
- **User's vision:** "When user hits /login, it goes through service A → B → C with these payloads."
- **User clarification:** Repos can be in different languages. Communication is often async through Kafka or other message queues.

## Testing Strategy

- Create a synthetic multi-service project with:
  - Go services communicating via HTTP
  - Go services publishing/consuming Kafka topics
  - A non-Go service (TS or Python) consuming from a Kafka topic (metadata-only analysis)
- Test with gorilla/* repos (handlers CORS middleware wraps mux router)
- Verify flow traces match actual code paths
- Test edge cases: dynamic URLs, dynamic topic names, fan-out patterns, mixed sync+async flows, multi-language service maps
