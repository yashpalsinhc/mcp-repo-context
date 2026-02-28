# TDD Plan: Dependency Graph & Import Analysis

Testing framework: Go standard `testing` package (existing project convention).

## 3. New Types

No tests needed — types are validated through the tests that use them.

## 4. go.mod Parser

**File:** `internal/analyzer/gomod_analyzer_test.go`

```go
// Test: Parse valid go.mod with direct and indirect dependencies
// Test: Parse go.mod with replace directives (version-specific)
// Test: Parse go.mod with wildcard replace directive (no version)
// Test: Parse go.mod with local path replace directive
// Test: Parse go.mod with no dependencies (module declaration only)
// Test: Handle malformed go.mod gracefully (return nil, no panic)
// Test: Extract correct module path from go.mod
// Test: Extract Go version from go.mod
// Test: Distinguish direct vs indirect dependencies correctly
```

## 5. Config File Parsers

**File:** `internal/analyzer/config_parsers_test.go`

```go
// Dockerfile:
// Test: Extract base images from single-stage Dockerfile
// Test: Extract base images from multi-stage Dockerfile (multiple FROM)
// Test: Extract EXPOSE ports
// Test: Extract CMD/ENTRYPOINT
// Test: Handle empty Dockerfile

// docker-compose.yml:
// Test: Extract service names and images
// Test: Extract port mappings
// Test: Extract depends_on relationships
// Test: Handle YAML parse error gracefully

// Makefile:
// Test: Extract target names
// Test: Extract target descriptions from preceding comments
// Test: Ignore recipe lines (starting with tab)
// Test: Handle Makefile with no targets

// CI config:
// Test: Parse GitHub Actions workflow for job names and triggers
// Test: Parse GitLab CI for stages and job names
// Test: Handle unknown CI format gracefully
```

## 6. Import Aggregation

**File:** `internal/analyzer/import_aggregator_test.go`

```go
// Test: Classify known stdlib imports (fmt, net/http, context)
// Test: Classify internal imports matching module path
// Test: Classify external imports not matching stdlib or module path
// Test: Resolve external import to module path via longest prefix match
// Test: Apply replace directives before import resolution
// Test: Handle aliased imports correctly
// Test: Handle import with no matching go.mod require (unresolved)
// Test: Aggregate imports across multiple files (deduplication)
// Test: Handle repo with no go.mod (skip classification, return empty)
```

## 7. Architecture Generation Updates

No separate test file — covered by existing manager tests plus integration tests.

## 8. Storage Schema Extension

**File:** `internal/storage/sqlite_test.go` (extend existing)

```go
// Test: Store and retrieve ModuleInfo
// Test: Store and retrieve ImportSummary
// Test: Store config file content and structured JSON
// Test: Batch load ModuleInfo for multiple repos
// Test: Migration idempotency (run migration twice without error)
// Test: Config file blocklist (no content stored for .env files)
```

## 9. Dependency Graph Builder

**File:** `internal/graph/dependency_graph_test.go`

```go
// Test: Build graph with two repos where one depends on the other
// Test: Build graph with repo that has only external dependencies
// Test: Build graph with repo that has no dependencies
// Test: External modules appear as unanalyzed nodes
// Test: Generate valid Mermaid diagram from dependency graph
// Test: Generate valid DOT diagram from dependency graph
// Test: Node styling distinguishes analyzed vs external
// Test: Edge labels include version information
```

## 10. New MCP Tool: get_dependency_graph

**File:** `internal/mcp/tools_test.go` (extend existing)

```go
// Test: get_dependency_graph returns Mermaid diagram for valid repo_ids
// Test: get_dependency_graph returns DOT diagram when format=dot
// Test: get_dependency_graph excludes external deps when include_external=false
// Test: get_dependency_graph returns error for unknown repo_ids
// Test: get_dependency_graph returns text summary alongside diagram
```

## 11. Existing Tool Enhancements

```go
// Test: compare_repos includes dependency relationships section
// Test: get_context(scope=architecture) includes dependency data
```

## 12b. Incremental Refresh

```go
// Test: refresh_file on go.mod re-parses ModuleInfo
// Test: refresh_file on Dockerfile updates config file content
// Test: refresh_changed detects go.mod changes and re-parses
```

## Integration Tests (Real Repos)

**File:** `internal/integration/dependency_graph_test.go`

```go
// Test: Analyze gorilla/mux — verify go.mod parsed correctly
// Test: Analyze gorilla/handlers — verify dependencies extracted
// Test: get_dependency_graph for both repos shows relationship (if exists)
// Test: compare_repos shows dependency relationships
// Test: get_context(scope=architecture) includes dependency list
```
