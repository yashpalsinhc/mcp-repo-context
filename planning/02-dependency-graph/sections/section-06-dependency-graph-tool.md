# Section 06: Dependency Graph Tool

## Overview

This section creates the cross-repo dependency graph builder and registers a new `get_dependency_graph` MCP tool. The graph is computed on-the-fly from stored `ModuleInfo` data (persisted by section-01 and populated by section-02/05). The tool outputs Mermaid or DOT visualizations.

## Dependencies

- **section-01-types-and-storage**: Provides `DependencyGraph`, `DependencyNode`, `DependencyEdge` types; `GetModuleInfoBatch` store method
- **section-02-gomod-parser**: Populates `ModuleInfo` during analysis (provides the data this section queries)
- **section-04-import-aggregation**: Populates `ImportSummary` (used for richer node metadata)
- **section-05-architecture-updates**: Ensures `ModuleInfo` is persisted during `AnalyzeRepository()` flow; populates package type

## Tests First

### File: `internal/graph/dependency_graph_test.go` (new)

```go
// Test: Build graph with two repos where one depends on the other
// Setup: Create ModuleInfo for repo A (module: "github.com/org/a") with no deps,
//   and repo B (module: "github.com/org/b") with dep on "github.com/org/a" v1.0.0
// Call: builder.BuildGraph(map[string]*ModuleInfo{"repoA": infoA, "repoB": infoB})
// Assert: Graph has 2 nodes (both IsAnalyzed=true)
// Assert: Graph has 1 edge from "github.com/org/b" to "github.com/org/a"
// Assert: Edge has Version="v1.0.0" and Direct=true

// Test: Build graph with repo that has only external dependencies
// Setup: Create ModuleInfo for repo A with dep on "github.com/external/lib" (not analyzed)
// Call: builder.BuildGraph(map[string]*ModuleInfo{"repoA": infoA})
// Assert: Graph has 2 nodes: repoA (IsAnalyzed=true), external/lib (IsAnalyzed=false)
// Assert: Graph has 1 edge from repoA to external/lib

// Test: Build graph with repo that has no dependencies
// Setup: Create ModuleInfo for repo A with empty Dependencies slice
// Call: builder.BuildGraph(map[string]*ModuleInfo{"repoA": infoA})
// Assert: Graph has 1 node, 0 edges

// Test: External modules appear as unanalyzed nodes
// Setup: Repo with 3 external deps, none analyzed
// Call: BuildGraph
// Assert: All 3 external nodes have IsAnalyzed=false

// Test: Generate valid Mermaid diagram from dependency graph
// Setup: Build graph with 2 repos and 1 edge
// Call: viz.GenerateDependencyMermaid(graph)
// Assert: Output starts with "graph TD"
// Assert: Contains node definitions for both repos
// Assert: Contains edge definition with version label

// Test: Generate valid DOT diagram from dependency graph
// Setup: Build graph with 2 repos and 1 edge
// Call: viz.GenerateDependencyDOT(graph)
// Assert: Output starts with "digraph"
// Assert: Contains node definitions with correct shapes
// Assert: Contains edge definition with label

// Test: Node styling distinguishes analyzed vs external
// Setup: Graph with 1 analyzed and 1 external node
// Call: viz.GenerateDependencyMermaid(graph)
// Assert: Analyzed node uses box shape (e.g., [name])
// Assert: External node uses rounded shape (e.g., (name))

// Test: Edge labels include version information
// Setup: Edge with Version="v1.2.3"
// Call: viz.GenerateDependencyMermaid(graph)
// Assert: Edge label contains "v1.2.3"
```

### File: `internal/mcp/tools_test.go` (extend existing)

```go
// Test: get_dependency_graph returns Mermaid diagram for valid repo_ids
// Setup: Store ModuleInfo for 2 repos where one depends on the other
// Call: toolGetDependencyGraph with repo_ids and format="mermaid"
// Assert: Result is not error, content contains "graph TD"

// Test: get_dependency_graph returns DOT diagram when format=dot
// Call: toolGetDependencyGraph with format="dot"
// Assert: Content contains "digraph"

// Test: get_dependency_graph excludes external deps when include_external=false
// Setup: Repo with both internal and external deps
// Call: toolGetDependencyGraph with include_external=false
// Assert: Only analyzed nodes appear in output

// Test: get_dependency_graph returns error for unknown repo_ids
// Call: toolGetDependencyGraph with non-existent repo IDs
// Assert: Result is error

// Test: get_dependency_graph returns text summary alongside diagram
// Call: toolGetDependencyGraph
// Assert: Content includes both a text summary ("X depends on Y") and the diagram
```

## Implementation Details

### 1. Dependency Graph Builder

**New file:** `internal/graph/dependency_graph.go`

Create a `GraphBuilder` struct that computes cross-repo dependency graphs on-the-fly.

**Algorithm:**

1. Accept a `map[string]*ModuleInfo` (repo_id -> ModuleInfo) loaded via batch query
2. Build a lookup map: `module_path -> repo_id` for all analyzed repos
3. For each repo, iterate its `ModuleInfo.Dependencies`:
   - If the dependency's `Path` matches an analyzed repo's module_path (via the lookup map), create an edge between the two analyzed repos (internal dependency)
   - Otherwise, create a node for the external module (marked `IsAnalyzed=false`) and an edge to it
4. Optionally filter out external nodes if `include_external=false`
5. Return `*DependencyGraph` with all nodes and edges

**Key function signatures:**

```go
type GraphBuilder struct{}

func NewGraphBuilder() *GraphBuilder

func (b *GraphBuilder) BuildGraph(moduleInfos map[string]*context.ModuleInfo) *context.DependencyGraph

func (b *GraphBuilder) BuildGraphFiltered(moduleInfos map[string]*context.ModuleInfo, includeExternal bool) *context.DependencyGraph
```

**Performance:** The `GetModuleInfoBatch` store method (from section-01) loads all ModuleInfo in a single query. For 200 repos this is one query returning ~200 rows, then in-memory prefix matching. Well under 2 seconds.

### 2. Visualization

**Same file or new file:** `internal/graph/dependency_visualizer.go`

Add two visualization functions:

```go
func GenerateDependencyMermaid(graph *context.DependencyGraph) string
func GenerateDependencyDOT(graph *context.DependencyGraph) string
```

**Mermaid format:**
```
graph TD
    A["github.com/org/repo-a"]
    B["github.com/org/repo-b"]
    C("github.com/external/lib")
    B -->|"v1.0.0"| A
    B -->|"v0.3.0"| C
```

- Analyzed repos use box notation `[name]`
- External modules use rounded notation `(name)`
- Edge labels show the version

**DOT format:**
```
digraph dependencies {
    rankdir=LR;
    "repo-a" [shape=box, label="github.com/org/repo-a"];
    "repo-b" [shape=box, label="github.com/org/repo-b"];
    "external" [shape=ellipse, style=dashed, label="github.com/external/lib"];
    "repo-b" -> "repo-a" [label="v1.0.0"];
    "repo-b" -> "external" [label="v0.3.0"];
}
```

### 3. Text Summary Generator

Add a function that generates a human-readable text summary alongside the diagram:

```go
func GenerateTextSummary(graph *context.DependencyGraph) string
```

Output format:
```
Dependency Summary:
- github.com/org/repo-b depends on:
  - github.com/org/repo-a (v1.0.0) [analyzed]
  - github.com/external/lib (v0.3.0) [external]
- github.com/org/repo-a: no dependencies on analyzed repos
```

### 4. Manager Interface Addition

**File:** `internal/orchestrator/manager.go`

Add to the `Manager` interface:

```go
GetDependencyGraph(ctx context.Context, repoIDs []string) (*context.DependencyGraph, error)
```

Implementation in the manager struct:
1. Call `store.GetModuleInfoBatch(repoIDs)` to load all ModuleInfo
2. Call `graphBuilder.BuildGraph(moduleInfos)` to compute the graph
3. Return the graph

### 5. MCP Tool Registration

**File:** `internal/mcp/server.go`

Add the tool definition in `handleListTools()`:

```go
{
    Name:        "get_dependency_graph",
    Description: "Show inter-repo dependency relationships for analyzed repos",
    InputSchema: map[string]interface{}{
        "type": "object",
        "properties": map[string]interface{}{
            "repo_ids": map[string]interface{}{
                "type":        "array",
                "items":       map[string]interface{}{"type": "string"},
                "description": "Repository IDs to analyze",
            },
            "format": map[string]interface{}{
                "type":        "string",
                "description": "Output format: 'mermaid' (default) or 'dot'",
                "enum":        []string{"mermaid", "dot"},
            },
            "include_external": map[string]interface{}{
                "type":        "boolean",
                "description": "Include external (non-analyzed) dependencies (default: true)",
            },
        },
        "required": []string{"repo_ids"},
    },
}
```

Add case in `handleCallToolWithID()` dispatch:

```go
case "get_dependency_graph":
    result = s.toolGetDependencyGraph(ctx, params.Arguments)
```

### 6. Tool Handler

**File:** `internal/mcp/tools.go` (or new file `tools_dependency_graph.go`)

```go
func (s *server) toolGetDependencyGraph(ctx context.Context, args map[string]interface{}) callToolResult
```

The handler:
1. Parse `repo_ids` (required), `format` (default "mermaid"), `include_external` (default true)
2. Validate repo_ids exist by checking stored contexts
3. Call `manager.GetDependencyGraph(ctx, repoIDs)`
4. Generate visualization (Mermaid or DOT) based on format param
5. Generate text summary
6. Return both the text summary and the diagram in the response content

## Error Handling

- If no repo_ids are provided, return error
- If a repo_id doesn't exist, return error listing the unknown IDs
- If repos have no `ModuleInfo` (not Go repos, or haven't been analyzed with go.mod parsing), return a message indicating no dependency data is available
- Empty graph (no edges) is not an error -- return the summary indicating no inter-repo dependencies found

## File Summary

| File | Action |
|------|--------|
| `internal/graph/dependency_graph.go` | New: GraphBuilder with BuildGraph and BuildGraphFiltered |
| `internal/graph/dependency_visualizer.go` | New: GenerateDependencyMermaid, GenerateDependencyDOT, GenerateTextSummary |
| `internal/graph/dependency_graph_test.go` | New: Tests for graph building and visualization |
| `internal/orchestrator/manager.go` | Add GetDependencyGraph to Manager interface and implementation |
| `internal/mcp/server.go` | Register get_dependency_graph tool definition and dispatch |
| `internal/mcp/tools.go` | Add toolGetDependencyGraph handler |

## Implementation Order

1. Write tests in `dependency_graph_test.go`
2. Implement `GraphBuilder` in `dependency_graph.go`
3. Implement visualization functions in `dependency_visualizer.go`
4. Add `GetDependencyGraph` to Manager interface and implementation
5. Register MCP tool and implement handler
6. Run all tests
