# Section 07: MCP Tools (trace_api_flow, get_service_map)

## Overview

This section adds two new MCP tools: `trace_api_flow` for tracing how an API call flows across services, and `get_service_map` for viewing the service topology of an organization. Both produce text summaries and Mermaid diagrams.

## Dependencies

- **section-05-endpoint-matching**: EndpointMatcher for cross-service matching
- **section-06-service-topology**: TopologyBuilder, ServiceTopology, GenerateMermaid

## Tests First

### File: `internal/mcp/tools_test.go` (extend)

```go
// Test: toolTraceApiFlow traces HTTP call chain
// Setup: 3 services: A→B (HTTP GET /users/{id}), B has handler
// Call: trace_api_flow with org_id, entry_point="GET /login" and repo_id of service A
// Assert: Response contains trace showing A's LoginHandler calling B's GetUser

// Test: toolTraceApiFlow traces Kafka flow
// Setup: A produces "user.events", C consumes "user.events"
// Call: trace_api_flow from A's handler that produces
// Assert: Response shows A→Kafka:user.events→C

// Test: toolTraceApiFlow detects cycles
// Setup: A calls B, B calls A
// Call: trace_api_flow from A's handler
// Assert: Response contains "[cycle detected: service-b → service-a]"

// Test: toolTraceApiFlow respects max_depth
// Setup: A→B→C→D chain (4 hops)
// Call: trace_api_flow with max_depth=2
// Assert: Trace stops at C, shows "[max depth reached]"

// Test: toolTraceApiFlow includes Mermaid sequence diagram
// Setup: A→B HTTP call
// Call: trace_api_flow
// Assert: Response contains "```mermaid" and "sequenceDiagram"

// Test: toolTraceApiFlow shows unmatched calls
// Setup: A calls /unknown/endpoint (no matching handler)
// Assert: Listed under "Unmatched Calls" with URL and expression

// Test: toolTraceApiFlow disambiguates multiple entry points
// Setup: /login endpoint registered in repo A and repo B
// Call: trace_api_flow with entry_point="POST /login" (no repo_id)
// Assert: Response lists both with "Multiple entry points found" prompt

// Test: toolTraceApiFlow with repo_id resolves unambiguously
// Setup: /login in repos A and B
// Call: trace_api_flow with entry_point="POST /login" and repo_id=A
// Assert: Traces from repo A only

// Test: toolTraceApiFlow returns error for unknown org
// Call: trace_api_flow with non-existent org_id
// Assert: Error response

// Test: toolTraceApiFlow returns error for no matching entry point
// Call: trace_api_flow with entry_point="DELETE /nonexistent"
// Assert: Error "No matching endpoint found"

// Test: toolGetServiceMap returns topology with Mermaid
// Setup: Org with 3 services, topology built
// Call: get_service_map with org_id
// Assert: Response contains service table and "```mermaid" flowchart

// Test: toolGetServiceMap rebuild flag forces fresh topology
// Setup: Stale cached topology
// Call: get_service_map with rebuild=true
// Assert: Fresh topology built (BuiltAt is recent)

// Test: toolGetServiceMap uses cached topology when not stale
// Setup: Fresh cached topology
// Call: get_service_map with rebuild=false
// Assert: Uses cached (BuiltAt matches stored)

// Test: toolGetServiceMap returns error for unknown org
// Assert: Error response with "org not found"

// Test: toolGetServiceMap with empty org
// Call: get_service_map for org with no repos
// Assert: "No services found" message, no error
```

## Implementation Details

### 1. Tool Registration

**File:** `internal/mcp/server.go` — add to `handleListTools()`

```go
// trace_api_flow tool
{
    Name:        "trace_api_flow",
    Description: "Trace how an API call flows across services in an organization",
    InputSchema: map[string]interface{}{
        "type": "object",
        "properties": map[string]interface{}{
            "org_id": map[string]interface{}{
                "type":        "string",
                "description": "Organization ID",
            },
            "entry_point": map[string]interface{}{
                "type":        "string",
                "description": "Starting endpoint: 'METHOD /path' (e.g., 'POST /login') or 'repo_id:function_name'",
            },
            "repo_id": map[string]interface{}{
                "type":        "string",
                "description": "Optional repo ID to disambiguate when multiple services have the same endpoint",
            },
            "max_depth": map[string]interface{}{
                "type":        "integer",
                "description": "Max hops to trace (default: 5)",
            },
        },
        "required": []string{"org_id", "entry_point"},
    },
}

// get_service_map tool
{
    Name:        "get_service_map",
    Description: "Get the service topology for an organization",
    InputSchema: map[string]interface{}{
        "type": "object",
        "properties": map[string]interface{}{
            "org_id": map[string]interface{}{
                "type":        "string",
                "description": "Organization ID",
            },
            "rebuild": map[string]interface{}{
                "type":        "boolean",
                "description": "Force rebuild of topology (default: false)",
            },
        },
        "required": []string{"org_id"},
    },
}
```

Add dispatch cases in `handleCallToolWithID()`:
```go
case "trace_api_flow":
    result = s.toolTraceApiFlow(ctx, params.Arguments)
case "get_service_map":
    result = s.toolGetServiceMap(ctx, params.Arguments)
```

### 2. trace_api_flow Handler

**File:** `internal/mcp/tools.go`

```go
func (s *server) toolTraceApiFlow(ctx context.Context, args map[string]interface{}) callToolResult
```

**Entry point resolution:**
1. Parse `entry_point` string:
   - If contains ":" and no space → "repo_id:function_name" format. Split on ":" to get repo and function.
   - If matches "METHOD /path" pattern → parse method and path
2. If method/path format:
   - Query `endpoints` table for matching entries in org repos
   - If `repo_id` arg provided, filter to that repo
   - If multiple matches and no repo_id → return disambiguation response listing all matching repos
   - If no matches → return error "No matching endpoint found"
3. Load the starting handler function from the matched endpoint's repo context

**Recursive tracing algorithm:**

```go
type TraceNode struct {
    ServiceName string
    RepoID      string
    FuncName    string
    FilePath    string
    Line        int
    CallType    string   // "entry", "http", "grpc", "kafka_produce", "kafka_consume"
    Method      string
    Path        string
    Confidence  string
    Children    []*TraceNode
    IsCycle     bool
}

func (s *server) traceFlow(ctx context.Context, repoID, funcName string, matcher *EndpointMatcher, visited map[string]bool, depth, maxDepth int) *TraceNode
```

Algorithm:
1. Check visited set: if `repoID + ":" + funcName` already visited → return node with IsCycle=true
2. Check depth: if depth >= maxDepth → return node with "[max depth reached]"
3. Add to visited set
4. Load function from repo context
5. For each ExternalCall in function:
   - Run matcher.Match() to find target endpoint
   - If matched: recursively trace from target handler
   - If unmatched: create leaf node with Confidence="unmatched"
6. For each AsyncCall (kafka_produce):
   - Find consumers via matcher.FindKafkaConsumers()
   - For each consumer: recursively trace
7. Return node with all children

**Output formatting:**

Build text output:
1. Header: `## API Flow Trace: {METHOD} {PATH}`
2. Trace tree (indented, showing each hop with service, function, file:line, confidence)
3. Unmatched Calls section (if any)
4. Mermaid sequence diagram

**Mermaid sequence diagram generation:**
- Each service as a participant
- Kafka topics as participants (with "Kafka:" prefix)
- HTTP/gRPC calls as solid arrows: `A->>B: GET /users/{id}`
- Kafka produce as solid arrow to topic: `A->>K: produce`
- Kafka consume as dashed arrow from topic: `K-->>C: consume`
- Cycles noted as a note: `Note over A,B: Cycle detected`

### 3. get_service_map Handler

**File:** `internal/mcp/tools.go`

```go
func (s *server) toolGetServiceMap(ctx context.Context, args map[string]interface{}) callToolResult
```

Handler flow:
1. Parse `org_id` (required) and `rebuild` (default false)
2. Validate org exists
3. If not rebuild: check for cached topology (`GetTopology`). Use if not stale.
4. If rebuild or stale or no cache: call `TopologyBuilder.BuildTopology(ctx, orgID)`
5. Store new topology in cache
6. Format output

**Output formatting:**
1. Header: `## Service Map: {orgID}`
2. Service table with columns: Service, Language, Endpoints, Calls Out, Calls In
3. Edge summary: total HTTP edges, gRPC edges, Kafka edges
4. Unmatched calls count
5. Mermaid flowchart (from `ServiceTopology.GenerateMermaid()`)

### 4. Wiring

**File:** `internal/mcp/server.go` or initialization code

The `server` struct needs access to:
- `TopologyBuilder` (for get_service_map and rebuild)
- `EndpointMatcher` (for trace_api_flow)

Add these as fields on `server`:
```go
type server struct {
    // ... existing fields ...
    topologyBuilder *flow.TopologyBuilder
    endpointMatcher *flow.EndpointMatcher
}
```

Initialize during server creation, reusing existing stores.

## Error Handling

- Unknown org: return error "Organization not found"
- No matching entry point: return error "No matching endpoint found for {entry_point}. Ensure the org has been analyzed."
- Multiple ambiguous entry points: return disambiguation response (not error) listing all matches
- Trace exceeds max_depth: truncate branch with indicator, continue other branches
- Cycle detected: mark branch as cycle, continue other branches
- Empty org: return "No services found in organization" with empty Mermaid

## File Summary

| File | Action |
|------|--------|
| `internal/mcp/server.go` | Register trace_api_flow, get_service_map tools. Add dispatch cases. Add topologyBuilder, endpointMatcher fields. |
| `internal/mcp/tools.go` | Add toolTraceApiFlow, toolGetServiceMap handlers. TraceNode type and traceFlow recursive method. |
| `internal/mcp/tools_test.go` | Tests for both handlers: traces, cycles, max_depth, disambiguation, Mermaid output. |

## Implementation Order

1. Write tests
2. Register tool definitions and dispatch cases
3. Add server fields for TopologyBuilder and EndpointMatcher
4. Implement toolGetServiceMap (simpler — delegates to TopologyBuilder)
5. Implement trace_api_flow entry point resolution
6. Implement recursive traceFlow algorithm
7. Implement Mermaid sequence diagram generation for traces
8. Run all tests
