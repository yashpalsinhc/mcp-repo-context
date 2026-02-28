# Section 08: Integration Test (End-to-End)

## Overview

This section covers end-to-end integration tests that verify the full dependency graph pipeline works correctly with real-world repositories. The tests analyze gorilla/mux and gorilla/handlers, then verify that go.mod parsing, import classification, dependency graph construction, `compare_repos` dependency relationships, and `get_context(scope=architecture)` dependency data all work together.

These tests depend on all previous sections (01 through 07) being fully implemented. They exercise the complete flow from repository analysis through to MCP tool output.

## Dependencies

- **section-01-types-and-storage**: New types (`ModuleInfo`, `ImportSummary`, `ConfigFile`, `DependencyGraph`) and SQLite schema migration 004
- **section-02-gomod-parser**: `GoModAnalyzer` for parsing go.mod files
- **section-03-config-parsers**: Config file parsers (Dockerfile, Makefile, etc.)
- **section-04-import-aggregation**: `ImportAggregator` for classifying imports
- **section-05-architecture-updates**: Updated `generateArchitecture()` with dependency data, incremental refresh
- **section-06-dependency-graph-tool**: `DependencyGraphBuilder` and `get_dependency_graph` MCP tool
- **section-07-tool-enhancements**: Updated `compare_repos` and `get_context(scope=architecture)`

## File to Create

`/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/integration/dependency_graph_test.go`

This is a new file in a new `internal/integration/` package. The integration package exists solely for cross-cutting tests that exercise the full pipeline.

## Test Stubs

The following test stubs must be implemented. Each test is described with its purpose, setup, and expected assertions.

```go
package integration

import (
    "context"
    "os"
    "testing"

    ctxpkg "github.com/yashpalc/mcp-repo-context/internal/context"
    "github.com/yashpalc/mcp-repo-context/internal/orchestrator"
    "github.com/yashpalc/mcp-repo-context/internal/storage"
    // additional imports as needed for repo, analyzer, graph, comparison packages
)

// setupIntegrationTest creates a real SQLite store (in a temp dir), a manager
// wired with real cloner/scanner/analyzers, and returns them along with a
// cleanup function. This mirrors production wiring but uses a temp database.
func setupIntegrationTest(t *testing.T) (orchestrator.Manager, storage.ContextStore, func()) {
    // Creates temp dir, initializes SQLite with all migrations (including 004),
    // creates a real Manager via orchestrator.NewManager with real repo.Source
    // (git clone) and repo.FileScanner. Returns manager, store, cleanup func.
    t.Helper()
    // ... implementation
    return nil, nil, func() {}
}

// TestAnalyzeGorillaMux_GoModParsed analyzes the gorilla/mux repository and
// verifies that go.mod was correctly parsed into ModuleInfo.
//
// Setup: Analyze https://github.com/gorilla/mux with Force=true.
// Assertions:
//   - Analysis completes without error
//   - RepoContext.ModuleInfo is not nil
//   - ModuleInfo.ModulePath == "github.com/gorilla/mux"
//   - ModuleInfo.GoVersion is a non-empty string
//   - ModuleInfo.Dependencies is populated (length > 0 or == 0 depending on
//     gorilla/mux's actual go.mod; the test should verify it is non-nil)
func TestAnalyzeGorillaMux_GoModParsed(t *testing.T) {
    // Uses setupIntegrationTest, calls manager.AnalyzeRepo for gorilla/mux,
    // retrieves context, checks ModuleInfo fields.
}

// TestAnalyzeGorillaHandlers_DependenciesExtracted analyzes gorilla/handlers
// and verifies that its dependencies are correctly extracted from go.mod.
//
// Setup: Analyze https://github.com/gorilla/handlers with Force=true.
// Assertions:
//   - Analysis completes without error
//   - ModuleInfo.ModulePath == "github.com/gorilla/handlers"
//   - ModuleInfo.Dependencies contains entries (gorilla/handlers has real deps)
//   - ImportSummary is populated with stdlib, internal, and external categories
//   - ImportSummary.External contains resolved module paths matching go.mod requires
func TestAnalyzeGorillaHandlers_DependenciesExtracted(t *testing.T) {
    // Uses setupIntegrationTest, analyzes gorilla/handlers, retrieves context,
    // checks ModuleInfo.Dependencies and ImportSummary fields.
}

// TestDependencyGraph_BothRepos analyzes both gorilla/mux and gorilla/handlers,
// then builds a dependency graph and verifies the relationship between them.
//
// Setup: Analyze both repos, then call manager.GetDependencyGraph with both IDs.
// Assertions:
//   - DependencyGraph is not nil
//   - Graph has nodes for both analyzed repos (IsAnalyzed=true)
//   - If gorilla/handlers depends on gorilla/mux (check its go.mod), there is
//     an edge from handlers to mux
//   - External dependencies appear as nodes with IsAnalyzed=false
//   - Mermaid visualization output is a non-empty string containing "graph" keyword
func TestDependencyGraph_BothRepos(t *testing.T) {
    // Uses setupIntegrationTest, analyzes both repos, calls
    // manager.GetDependencyGraph, checks nodes, edges, and visualization.
}

// TestCompareRepos_ShowsDependencyRelationships runs compare_repos on both
// gorilla repos and verifies the output includes a dependency relationships
// section.
//
// Setup: Analyze both repos, then call comparer.Compare with both repo IDs.
// Assertions:
//   - Comparison result is not nil and has no error
//   - Result contains a "Dependency Relationships" section (or equivalent field)
//   - Shared external dependencies are listed if any exist
func TestCompareRepos_ShowsDependencyRelationships(t *testing.T) {
    // Uses setupIntegrationTest, analyzes both repos, creates a Comparer,
    // runs comparison, checks for dependency relationship data in output.
}

// TestGetContextArchitecture_IncludesDependencyList verifies that retrieving
// context with scope=architecture includes dependency information.
//
// Setup: Analyze gorilla/handlers, then retrieve its context.
// Assertions:
//   - Architecture is not nil
//   - Architecture.Dependencies is populated (non-empty list of module paths)
//   - Architecture overview or a new field includes Go version
//   - Architecture includes package type (library or application)
func TestGetContextArchitecture_IncludesDependencyList(t *testing.T) {
    // Uses setupIntegrationTest, analyzes gorilla/handlers, retrieves context,
    // checks Architecture.Dependencies and related fields.
}
```

## Test Infrastructure Details

### `setupIntegrationTest` Implementation Notes

The setup function must wire together the real production components:

1. Create a temporary directory with `t.TempDir()` for the SQLite database
2. Initialize `storage.NewSQLiteStore(filepath.Join(tmpDir, "test.db"))` -- this must run all migrations including the new migration 004 from section-01
3. Create a real `repo.NewGitSource()` for cloning repositories
4. Create a real `repo.NewFileScanner()` for scanning files
5. Create the manager via `orchestrator.NewManager(store, source, scanner)`
6. Return the manager, store, and a cleanup function that closes the store

### Build Tag and Test Flags

These tests clone real repositories from GitHub and take significant time. They should be gated behind a build tag or `testing.Short()` skip:

```go
func TestAnalyzeGorillaMux_GoModParsed(t *testing.T) {
    if testing.Short() {
        t.Skip("skipping integration test in short mode")
    }
    // ...
}
```

All integration tests should follow this pattern so that `go test -short ./...` skips them while `go test ./internal/integration/...` runs them.

### Repository URLs

- gorilla/mux: `https://github.com/gorilla/mux`
- gorilla/handlers: `https://github.com/gorilla/handlers`

These are small, stable, public repositories well suited for integration testing. gorilla/handlers historically depends on gorilla/mux, making them ideal for verifying cross-repo dependency detection.

### Repo IDs

After analysis, the repo IDs will be:
- `github.com/gorilla/mux` (derived from URL by `repo.RepoIDFromURL`)
- `github.com/gorilla/handlers`

## Key Verification Points

### go.mod Parsing Verification

After analyzing a repo, retrieve the context and check:
- `repoCtx.ModuleInfo` is non-nil (added by section-01 to `RepoContext`)
- `repoCtx.ModuleInfo.ModulePath` matches the expected module path
- `repoCtx.ModuleInfo.GoVersion` is non-empty
- `repoCtx.ModuleInfo.Dependencies` is a valid slice (may be empty for repos with no external deps)

### Import Summary Verification

After analyzing a repo:
- `repoCtx.ImportSummary` is non-nil
- `repoCtx.ImportSummary.Stdlib` contains known Go stdlib packages (e.g., "fmt", "net/http")
- `repoCtx.ImportSummary.Internal` contains paths prefixed with the module path
- `repoCtx.ImportSummary.External` contains resolved module paths matching go.mod requires

### Dependency Graph Verification

After analyzing both repos and calling `GetDependencyGraph`:
- `graph.Nodes` contains entries for both analyzed repos with `IsAnalyzed=true`
- `graph.Edges` contains at least the expected cross-repo dependency (if it exists in the actual go.mod)
- External dependencies appear as nodes with `IsAnalyzed=false`
- The Mermaid output string is non-empty and contains expected syntax markers (`graph`, `-->`)

### compare_repos Verification

The comparison output (from section-07 enhancements) should include:
- A dependency relationships section showing inter-repo dependencies
- Shared external dependencies if both repos depend on the same module

### Architecture Verification

The architecture context (from section-05 enhancements) should include:
- `Dependencies` field populated with module paths from go.mod
- Go version information
- Package type detection ("library" for gorilla/mux since it has no main package)

## Error Handling in Integration Tests

Integration tests must handle real-world conditions gracefully:

- If GitHub is unreachable, skip the test with `t.Skip("GitHub unreachable")`
- If a repo's go.mod changes upstream (e.g., gorilla/mux adds or removes a dependency), the test assertions should be resilient -- check for structural correctness (non-nil, correct types) rather than exact counts
- Use `t.Logf` to output actual values for debugging when assertions fail

## Running the Tests

```bash
# Run only integration tests
go test -v -timeout 300s ./internal/integration/...

# Run all tests except integration (for CI fast path)
go test -short ./...

# Run a specific integration test
go test -v -timeout 120s -run TestDependencyGraph_BothRepos ./internal/integration/...
```

The timeout should be generous (300s) since cloning and analyzing two repositories involves network I/O and substantial computation. Each individual test that clones a repo should set its own timeout context if finer control is needed.
