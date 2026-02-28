# Spec: Cross-Service API Flow Tracing

## Problem
The MCP server can analyze individual Go repos (functions, call graphs, side effects) but cannot trace how API calls flow between services. When a user asks "what happens when someone hits /login?", we can show the handler function and its internal call chain, but not that it calls service B's `/auth/validate` endpoint, which publishes to Kafka topic `user.verified`, consumed by service C.

## Current State
- `ExternalCall` struct exists with Method, URL, Description, Line — but URL is only populated for string literals
- `Route` struct exists with Method, Path, Handler — stored in `FileContext.Routes`
- Side effect detection catches `http_call`, `kafka_call`, `grpc_call` but without endpoint details
- Routes and external calls stored as opaque JSON blobs in SQLite — no cross-repo querying
- All `SearchableStore` methods are single-repo scoped
- No tool links ExternalCall in one repo to Route in another

## Requirements

### 1. Enhanced HTTP Client URL Extraction
- Extract URLs from `http.Get()`, `http.NewRequest()`, `http.Post()`, `client.Do(req)` patterns
- Support string literals, constants, `fmt.Sprintf` format strings
- Mark dynamically-constructed URLs as "dynamic URL" with best-effort template

### 2. Enhanced Route Registration Detection
- Detect gorilla/mux route registrations: `r.HandleFunc("/path", handler).Methods("GET")`
- Detect chi, net/http (Go 1.22+), and gin patterns
- Extract path patterns with parameters ({id}, :id)
- Map handler functions to their registered routes

### 3. Kafka Producer/Consumer Detection
- Detect segmentio/kafka-go, Sarama, confluent-kafka-go patterns
- Extract topic names from struct initialization and method arguments
- Distinguish producers vs consumers
- Store as new side effect subtypes or extend ExternalCall

### 4. Normalized Endpoint Index (SQLite)
- New tables: `endpoints` (server routes) and `external_calls` (client calls)
- Proper columns for URL, method, topic, direction (producer/consumer)
- Enable cross-repo SQL joins for endpoint matching

### 5. Cross-Service Endpoint Matching
- Match HTTP client URLs to server route patterns across repos in an org
- Match Kafka producer topics to consumer topics
- URL path normalization per RFC 3986
- Parameter matching ({id} vs :id vs path segment)

### 6. Service Topology Graph
- Build directed graph: services as nodes, HTTP/gRPC/Kafka edges
- Distinguish sync (HTTP, gRPC) vs async (Kafka) edges
- Support fan-out (one topic → multiple consumers)
- Detect circular dependencies

### 7. New MCP Tools
- `trace_api_flow`: Given starting endpoint, trace full flow across services with Mermaid visualization
- `get_service_map`: Given org_id, return service topology with Mermaid visualization

### 8. Enhanced get_pr_context
- Show upstream callers when PR modifies a handler
- Show downstream impact when PR modifies a client call
- Add "cross-service impact" section

### 9. Basic Multi-Language Support
- Parse docker-compose.yml for service definitions with environment variables containing topic names
- Parse package.json for Kafka/HTTP client dependencies to identify non-Go services
- Show non-Go services as metadata-only nodes in service map

## Scope Exclusions
- NATS, RabbitMQ, SQS detection (deferred)
- DOT/Graphviz output (Mermaid only)
- Runtime tracing integration (OpenTelemetry)
- Non-Go deep analysis (only config-file metadata parsing)

## Dependencies
- 02-dependency-graph: inter-repo dependency data
- 01-core-bug-fixes: reliable call graph extraction
