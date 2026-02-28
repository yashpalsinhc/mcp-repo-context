# Section 06: Service Topology Graph

## Overview

This section builds and stores a service-level topology graph from matched endpoints across an organization's repos. It uses the EndpointMatcher (Section 5) to batch-match all service calls against all endpoints, constructs ServiceNode and ServiceEdge records, generates Mermaid visualizations, and caches the topology in SQLite.

## Dependencies

- **section-04-endpoint-storage**: endpoints and service_calls tables
- **section-05-endpoint-matching**: EndpointMatcher, PathTrie, MatchResult
- **section-08-multi-language**: MultiServiceDetector for non-Go service nodes (optional — can use nil detector)

## Tests First

### File: `internal/flow/topology_test.go` (new)

```go
// Test: BuildTopology creates nodes for all repos in org
// Setup: Org with 3 repos, each with endpoints stored
// Call: BuildTopology(ctx, orgID)
// Assert: 3 ServiceNode entries, each with correct RepoID and endpoint count

// Test: ServiceName derived from repo name
// Setup: Repo "github.com/org/auth-service"
// Assert: ServiceNode.ServiceName == "auth-service"

// Test: BuildTopology creates HTTP edges from matched calls
// Setup: Repo A has service_call HTTP GET /api/users, Repo B has endpoint GET /api/users
// Call: BuildTopology
// Assert: ServiceEdge with Source="service-a", Target="service-b", EdgeType="http", Confidence="exact"

// Test: BuildTopology creates Kafka produce/consume edges
// Setup: Repo A has kafka_produce for "user.events", Repo C has kafka_consume for "user.events"
// Call: BuildTopology
// Assert: Two edges: A→"user.events" (produce), "user.events"→C (consume)

// Test: Fan-out produces multiple consumer edges
// Setup: Repo A produces "user.events", Repos B and C both consume "user.events"
// Call: BuildTopology
// Assert: 3 edges total (1 produce + 2 consume)

// Test: BuildTopology includes unmatched calls
// Setup: Repo A calls /unknown/endpoint, no matching endpoint exists
// Call: BuildTopology
// Assert: ServiceEdge with Target="<unknown>", Confidence="unmatched"

// Test: BuildTopology uses service hint for ambiguous matches
// Setup: Repos B and C both have GET /health, Repo A calls /health with service_hint="service-b"
// Assert: Edge targets service-b (not service-c)

// Test: Topology caching - store and retrieve
// Setup: Build topology, store in service_topologies table
// Call: GetTopology(ctx, orgID)
// Assert: Returns cached topology with correct nodes and edges

// Test: Topology marked stale after repo re-analysis
// Setup: Build and cache topology, then re-analyze one repo
// Assert: IsTopologyStale(ctx, orgID) returns true

// Test: BuildTopology includes non-Go metadata nodes
// Setup: docker-compose.yml with non-Go service "frontend"
// Call: BuildTopology with MultiServiceDetector
// Assert: ServiceNode "frontend" with IsDeepAnalyzed=false, Language="typescript"

// Test: GenerateMermaid produces valid Mermaid flowchart
// Setup: Topology with 3 nodes, 2 HTTP edges, 1 Kafka edge
// Call: GenerateMermaid
// Assert: Output contains "graph LR", solid arrows for HTTP, dashed for Kafka

// Test: GenerateMermaid distinguishes sync and async edges
// Assert: HTTP edges use "-->" (solid), Kafka edges use "-.->" (dashed)

// Test: Empty org returns empty topology (no error)
// Setup: Org with no repos
// Call: BuildTopology
// Assert: Empty nodes and edges, no error
```

## Implementation Details

### 1. Types

**File:** `internal/flow/topology.go` (new)

```go
type ServiceNode struct {
    RepoID         string
    ServiceName    string   // derived from repo name
    Language       string   // "go", "typescript", "python", "unknown"
    EndpointCount  int      // count of registered routes
    IsDeepAnalyzed bool     // true for Go repos with full analysis
}

type ServiceEdge struct {
    Source     string   // source service name
    Target     string   // target service name, topic name, or "<unknown>"
    EdgeType   string   // "http", "grpc", "kafka_produce", "kafka_consume"
    Method     string   // HTTP method or produce/consume
    Path       string   // URL path or topic name
    SourceFunc string   // function making the call
    TargetFunc string   // handler function (if matched)
    Confidence string   // "exact", "parameterized", "hint-matched", "ambiguous", "unmatched"
}

type ServiceTopology struct {
    OrgID   string
    Nodes   []ServiceNode
    Edges   []ServiceEdge
    BuiltAt time.Time
}
```

### 2. TopologyBuilder

**File:** `internal/flow/topology.go`

```go
type TopologyBuilder struct {
    orgStore     org.Store
    ctxStore     storage.ContextStore
    matcher      *EndpointMatcher
    detector     *MultiServiceDetector  // may be nil
}

func NewTopologyBuilder(orgStore org.Store, ctxStore storage.ContextStore, matcher *EndpointMatcher, detector *MultiServiceDetector) *TopologyBuilder
```

### 3. BuildTopology Algorithm

```go
func (b *TopologyBuilder) BuildTopology(ctx context.Context, orgID string) (*ServiceTopology, error)
```

1. **Get org repos**: `orgStore.GetOrg(ctx, orgID)` → list of repo IDs
2. **Build service nodes**: For each repo:
   - Derive service name from repo ID (last path segment, e.g., "github.com/org/auth-service" → "auth-service")
   - Query endpoint count from `endpoints` table
   - Determine language (Go if context exists and has Go files, otherwise from file extensions)
   - Create ServiceNode
3. **Add non-Go nodes** (if detector available):
   - Scan org repos for docker-compose.yml and package.json
   - Create metadata-only ServiceNode entries
4. **Batch endpoint matching**:
   - Load ALL endpoints from all org repos into a PathTrie (one SQL query: `SELECT * FROM endpoints WHERE repo_id IN (?)`)
   - Load ALL service_calls from all org repos (one SQL query: `SELECT * FROM service_calls WHERE repo_id IN (?)`)
   - For each service_call: match against PathTrie (for HTTP/gRPC) or exact topic match (for Kafka)
   - Create ServiceEdge for each match result
5. **Handle unmatched calls**: Create edges with Target="<unknown>" and Confidence="unmatched"
6. **Set timestamp**: `BuiltAt = time.Now()`
7. **Return topology**

### 4. Service Name Derivation

```go
func deriveServiceName(repoID string) string
```

- For GitHub repos: `github.com/org/auth-service` → `"auth-service"`
- For local repos: `local:/path/to/auth-service` → `"auth-service"`
- Simply takes the last path segment and strips common suffixes like `-service`, `-api` only for display purposes in Mermaid (keep full name in ServiceNode)

### 5. Topology Storage

**File:** `internal/storage/sqlite.go` (extend)

Add to ensureSchema:
```sql
CREATE TABLE IF NOT EXISTS service_topologies (
    org_id TEXT PRIMARY KEY,
    topology_json TEXT NOT NULL,
    built_at TIMESTAMP,
    is_stale BOOLEAN DEFAULT 0
);
```

Methods:
- `StoreTopology(ctx, orgID string, topology *ServiceTopology) error` — INSERT OR REPLACE, serialize to JSON
- `GetTopology(ctx, orgID string) (*ServiceTopology, error)` — SELECT and deserialize
- `MarkTopologyStale(ctx, orgID string) error` — UPDATE SET is_stale=1
- `IsTopologyStale(ctx, orgID string) (bool, error)` — SELECT is_stale
- `DeleteTopology(ctx, orgID string) error` — DELETE

### 6. Mermaid Generation

```go
func (t *ServiceTopology) GenerateMermaid() string
```

Produces a Mermaid flowchart:
- Each service as a node: `A[auth-service]`
- HTTP/gRPC edges as solid arrows: `A -->|GET /users| B`
- Kafka produce edges as dashed arrows to topic: `A -.->|produce| K[Kafka: user.events]`
- Kafka consume edges as dashed arrows from topic: `K -.->|consume| C`
- Unmatched edges: `A -->|GET /unknown| U[???]`
- Metadata-only nodes styled differently: `style F fill:#ddd,stroke:#999` (grayed out)

### 7. Invalidation Hooks

**Mark topology stale when:**
- A repo in the org is re-analyzed (add to `StoreRepoContext` or org analyzer callback)
- A repo is added or removed from the org (add to org.Manager.AddRepos/RemoveRepos)
- Endpoints or service_calls tables change for any org repo

This is done by calling `MarkTopologyStale(ctx, orgID)` in the relevant code paths.

## Error Handling

- Unknown org: return error
- Empty org: return empty topology (no error)
- Missing endpoints/service_calls tables: graceful degradation — return topology with nodes only
- Failed non-Go detection: skip metadata nodes, continue with Go-only topology
- Matcher errors for individual calls: log and create unmatched edge, continue

## File Summary

| File | Action |
|------|--------|
| `internal/flow/topology.go` | New: ServiceNode, ServiceEdge, ServiceTopology types. TopologyBuilder, BuildTopology, GenerateMermaid |
| `internal/flow/topology_test.go` | New: Tests for BuildTopology, Mermaid generation, caching, staleness |
| `internal/storage/sqlite.go` | Add service_topologies table, StoreTopology, GetTopology, MarkTopologyStale |

## Implementation Order

1. Write tests
2. Define ServiceNode, ServiceEdge, ServiceTopology types
3. Implement TopologyBuilder and BuildTopology
4. Implement GenerateMermaid
5. Add topology storage and staleness methods
6. Add invalidation hooks
7. Run all tests
