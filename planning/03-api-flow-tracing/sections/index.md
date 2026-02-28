<!-- PROJECT_CONFIG
runtime: go
test_command: go test ./...
END_PROJECT_CONFIG -->

<!-- SECTION_MANIFEST
section-01-http-grpc-extraction
section-02-route-detection
section-03-kafka-extraction
section-04-endpoint-storage
section-05-endpoint-matching
section-06-service-topology
section-07-mcp-tools
section-08-multi-language
section-09-pr-context
section-10-integration-tests
END_MANIFEST -->

# Implementation Sections Index

## Dependency Graph

| Section | Depends On | Blocks | Parallelizable |
|---------|------------|--------|----------------|
| section-01-http-grpc-extraction | - | 04, 05 | Yes |
| section-02-route-detection | - | 04, 05 | Yes |
| section-03-kafka-extraction | - | 04, 05 | Yes |
| section-04-endpoint-storage | 01, 02, 03 | 05, 06 | No |
| section-05-endpoint-matching | 04 | 06, 07, 09 | No |
| section-06-service-topology | 05, 08 | 07 | No |
| section-07-mcp-tools | 05, 06 | 10 | No |
| section-08-multi-language | - | 06 | Yes |
| section-09-pr-context | 05 | 10 | Yes |
| section-10-integration-tests | all | - | No |

## Execution Order

1. **Batch 1:** section-01-http-grpc-extraction, section-02-route-detection, section-03-kafka-extraction, section-08-multi-language (parallel, no dependencies)
2. **Batch 2:** section-04-endpoint-storage (requires 01, 02, 03)
3. **Batch 3:** section-05-endpoint-matching (requires 04)
4. **Batch 4:** section-06-service-topology, section-09-pr-context (parallel, both need 05)
5. **Batch 5:** section-07-mcp-tools (requires 05, 06)
6. **Batch 6:** section-10-integration-tests (requires all)

## Section Summaries

### section-01-http-grpc-extraction
Enhance extractExternalCall() to handle constants, fmt.Sprintf patterns, string concatenation, http.NewRequest method extraction, and HTTP client wrapper heuristic. Add URLExpression, ServiceHint fields. gRPC client detection.

### section-02-route-detection
Gorilla/mux route registration with Methods() chain detection. Chi nested routing. Go 1.22+ patterns. Path parameter normalization ({id}, :id → {param}). Route struct consolidation between analyzer.Route and context.Route. Fix duplicate Gin/Echo detection bug.

### section-03-kafka-extraction
New KafkaExtractor for segmentio/kafka-go, Sarama, confluent-kafka-go. Producer and consumer detection. Topic name extraction from struct literals, string slices, constants. AsyncCall type. kafka_produce/kafka_consume side effect subtypes.

### section-04-endpoint-storage
New SQLite tables: endpoints (server routes) and service_calls (client calls). Normalized columns for cross-repo SQL joins. Storage methods on SQLiteStore. Auto-population during StoreRepoContext. Migration via ensureSchema().

### section-05-endpoint-matching
EndpointMatcher with path-template matching. URL normalization (RFC 3986). Service-hint-based filtering. Confidence levels. Path trie for batch matching. Kafka exact topic matching. gRPC exact path matching.

### section-06-service-topology
ServiceTopology graph with ServiceNode and ServiceEdge types. Batch endpoint matching for topology build. Mermaid flowchart generation. Topology caching with staleness. Non-Go metadata nodes.

### section-07-mcp-tools
trace_api_flow tool: entry_point parsing, recursive tracing with cycle detection, Mermaid sequence diagram. get_service_map tool: topology loading/building, service summary, Mermaid flowchart.

### section-08-multi-language
docker-compose.yml parsing for service names, Kafka topics, and service URLs from environment variables. package.json parsing for Kafka/HTTP client dependencies. Metadata-only ServiceNode creation.

### section-09-pr-context
Enhanced get_pr_context with "Cross-Service Impact" section. Upstream callers for modified handlers. Downstream impact for modified client calls. Risk level based on affected service count.

### section-10-integration-tests
End-to-end tests with synthetic 3-service project (HTTP + Kafka). Full pipeline: analyze → build topology → trace flow. Fan-out, circular dependency, dynamic URL, and PR context scenarios.
