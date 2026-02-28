# Section 09: Enhanced get_pr_context

## Overview

This section adds a "Cross-Service Impact" section to the `get_pr_context` tool output. When a PR modifies a handler function, it shows which upstream services call that endpoint. When a PR modifies a function with external calls, it shows which downstream services handle those calls.

## Dependencies

- **section-04-endpoint-storage**: endpoints and service_calls tables
- **section-05-endpoint-matching**: EndpointMatcher for resolving call targets

## Tests First

### File: `internal/mcp/tools_test.go` (extend)

```go
// Test: PR context shows upstream callers for modified handler
// Setup: Service B has handler GET /users/{id}. Service A has service_call to GET /users/{id}.
// PR modifies /users/{id} handler in Service B.
// Call: toolGetPRContext with changed_files including the handler file
// Assert: Output contains "Cross-Service Impact" section with Service A listed as upstream caller

// Test: PR context shows downstream impact for modified client call
// Setup: Service A calls POST /auth/validate. Service B handles /auth/validate.
// PR modifies the function in Service A that makes the call.
// Call: toolGetPRContext
// Assert: Output contains Service B as downstream dependency

// Test: PR context shows Kafka impact
// Setup: Service A produces to "user.events". Service C consumes "user.events".
// PR modifies the producer function in Service A.
// Assert: Output shows Service C as downstream consumer via Kafka

// Test: PR context with no cross-service impact
// Setup: Modified function has no external calls and is not a registered handler
// Assert: "Cross-Service Impact" section says "None detected" or is omitted

// Test: PR context risk level based on affected service count
// Setup: Modified handler called by 3 different services
// Assert: Risk level shown as "High" (3+ services affected)

// Test: PR context risk level low for 1 service
// Setup: Modified handler called by 1 service
// Assert: Risk level "Low"

// Test: PR context handles repo not in any org
// Setup: PR for a standalone repo (not in an org)
// Assert: Cross-Service Impact section omitted (no org context)
```

## Implementation Details

### 1. Cross-Service Impact Analysis

**File:** `internal/flow/impact.go` (new)

```go
type CrossServiceImpact struct {
    UpstreamCallers  []ServiceCallRef  // services that call modified endpoints
    DownstreamDeps   []ServiceCallRef  // services called by modified functions
    KafkaConsumers   []ServiceCallRef  // consumers of topics produced by modified functions
    KafkaProducers   []ServiceCallRef  // producers to topics consumed by modified functions
    RiskLevel        string            // "low", "medium", "high"
    AffectedServices int               // total unique services affected
}

type ServiceCallRef struct {
    ServiceName  string
    RepoID       string
    FunctionName string
    CallType     string  // "http", "grpc", "kafka"
    Path         string  // URL or topic
}
```

```go
func AnalyzeCrossServiceImpact(ctx context.Context, repoID string, changedFunctions []string, store storage.SearchableStore) (*CrossServiceImpact, error)
```

Algorithm:
1. For each changed function:
   a. Check if function is a registered handler: query `endpoints` table for handler_name matching function name in this repo
   b. If handler: query `service_calls` table for entries targeting this endpoint's path → these are upstream callers
   c. Check if function has service_calls: query `service_calls` table for function_name matching in this repo
   d. If has calls: for each HTTP/gRPC call, query `endpoints` for matching handlers → downstream deps
   e. If has Kafka produce calls: query `service_calls` for kafka_consume with same topic → Kafka consumers
2. Deduplicate by service (a service might call multiple endpoints)
3. Calculate risk level:
   - 0 services: "none"
   - 1 service: "low"
   - 2 services: "medium"
   - 3+ services: "high"

### 2. Integrate into toolGetPRContext

**File:** `internal/mcp/tools.go`

Modify `toolGetPRContext` (or `toolGetPRContextRich`) to add cross-service analysis after the existing per-function analysis:

1. Determine org_id: Check if the repo belongs to any org (query org_repos table)
2. If no org: skip cross-service analysis
3. If org: for each changed file, collect the function names that were modified
4. Call `AnalyzeCrossServiceImpact(ctx, repoID, changedFunctions, store)`
5. Append "Cross-Service Impact" section to output

### 3. Output Format

```
## Cross-Service Impact

**Risk Level: Medium** (2 services affected)

### Upstream (services calling modified endpoints)
- **auth-service** calls `GET /users/{id}` → modified handler `GetUser`
  - Function: `ValidateUserToken` (auth/middleware.go:45)

### Downstream (services called by modified functions)
- **notification-service** consumes Kafka topic `user.updated`
  - Handler: `HandleUserUpdate` (notify/consumer.go:22)

### Summary
| Direction | Service | Protocol | Path |
|-----------|---------|----------|------|
| Upstream  | auth-service | HTTP GET | /users/{id} |
| Downstream | notification-service | Kafka | user.updated |
```

## Error Handling

- Repo not in any org: skip cross-service analysis silently (not an error)
- Endpoints/service_calls tables empty: return empty impact (no error)
- Function not found in endpoints or service_calls: skip that function
- Database errors: log and return partial results

## File Summary

| File | Action |
|------|--------|
| `internal/flow/impact.go` | New: CrossServiceImpact type, AnalyzeCrossServiceImpact |
| `internal/mcp/tools.go` | Extend toolGetPRContext to include cross-service impact |
| `internal/mcp/tools_test.go` | Tests for cross-service impact in PR context |

## Implementation Order

1. Write tests
2. Define CrossServiceImpact and ServiceCallRef types
3. Implement AnalyzeCrossServiceImpact
4. Integrate into toolGetPRContext
5. Format output section
6. Run all tests
