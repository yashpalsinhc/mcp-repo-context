

The stop hook failed due to a path issue with the Python script execution, but the section content I generated is complete. Let me provide the final output again cleanly so the content can be captured.

# Section 07: Tool Enhancements

## Overview

This section enhances three existing MCP tools to surface dependency data computed by prior sections:

1. **compare_repos** -- Add a "Dependency Relationships" section showing which compared repos depend on each other and which external dependencies they share.
2. **get_context (scope=architecture)** -- Include module path, Go version, direct dependency count/list, package type (library/application), and import summary counts.
3. **ask (AI-powered)** -- Update the AI prompt template to include dependency data so the AI can answer dependency-related questions.

## Dependencies

- **section-01-types-and-storage**: Provides `ModuleInfo`, `ImportSummary`, `ConfigFile`, `DependencyGraph` types in `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/context/types.go`, plus SQLite storage methods (`GetModuleInfo`, `GetImportSummary`, etc.) in the store.
- **section-03-config-parsers**: Provides parsed config file data (used indirectly through stored `ConfigFile` records).
- **section-05-architecture-updates**: Ensures that `ArchitectureContext.Dependencies` is populated, package type is detected, Go version is stored in the architecture overview, and `RepoContext.ModuleInfo` / `RepoContext.ImportSummary` fields exist and are populated during analysis.

## Tests First

### File: `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/comparison/comparer_test.go` (extend existing)

Add the following test stubs to the existing test file:

```go
// Test: compare_repos includes dependency relationships section
func TestComparer_Compare_IncludesDependencyRelationships(t *testing.T) {
    // Setup: Create two RepoContext objects where repo B has a ModuleInfo
    // whose Dependencies list includes repo A's module path.
    // Both RepoContext objects should have ModuleInfo populated.
    //
    // Call: c.Compare(ctx, repos, opts)
    //
    // Assert: result.DependencyRelationships is non-nil,
    //   result.DependencyRelationships.InternalDeps has at least one entry
    //   showing repo B depends on repo A.
    //
    // Assert: result.DependencyRelationships.SharedExternalDeps lists
    //   external dependencies that appear in both repos.
}
```

### File: `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/mcp/tools_test.go` (extend existing or create)

```go
// Test: get_context(scope=architecture) includes dependency data
func TestToolGetContext_Architecture_IncludesDependencyData(t *testing.T) {
    // Setup: Create a mock manager that returns a RepoContext with:
    //   - ModuleInfo with module_path, go_version, and dependencies
    //   - ImportSummary with stdlib/internal/external counts
    //   - Architecture with Dependencies populated and PackageType set
    //
    // Call: toolGetContext with scope=architecture
    //
    // Assert: Response text contains "Module Path:", "Go Version:",
    //   "Package Type:", "Direct Dependencies:", and "Import Summary:"
}

// Test: compare_repos includes dependency relationships section
func TestToolCompareRepos_IncludesDependencyRelationships(t *testing.T) {
    // Setup: Two repos with ModuleInfo where one depends on the other
    //
    // Call: toolCompareRepos
    //
    // Assert: Response text contains "Dependency Relationships" section
    //   with the inter-repo dependency listed
}
```

### File: `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/ai/query_test.go` (extend existing or create)

```go
// Test: buildQueryPrompt includes dependency data when available
func TestBuildQueryPrompt_IncludesDependencyContext(t *testing.T) {
    // Setup: Create a RelevantContext with DependencyInfo populated
    //   (new field on RelevantContext)
    //
    // Call: handler.buildQueryPrompt(query, ctx)
    //
    // Assert: Prompt string contains "## Dependencies" section
    //   with module path, Go version, and dependency list
}
```

## Implementation Details

### 1. Update `CompareResult` type

**File:** `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/comparison/types.go`

Add a new field to `CompareResult`:

```go
type CompareResult struct {
    // ... existing fields ...

    // DependencyRelationships shows how compared repos relate via go.mod dependencies.
    DependencyRelationships *DependencyRelationships `json:"dependency_relationships,omitempty"`
}
```

Add new types:

```go
// DependencyRelationships describes inter-repo and shared dependency info.
type DependencyRelationships struct {
    // InternalDeps lists dependency edges between the compared repos.
    InternalDeps []InternalDep `json:"internal_deps,omitempty"`
    // SharedExternalDeps lists external modules used by multiple compared repos.
    SharedExternalDeps []SharedDep `json:"shared_external_deps,omitempty"`
}

// InternalDep represents one compared repo depending on another.
type InternalDep struct {
    FromRepoID   string `json:"from_repo_id"`
    FromModule   string `json:"from_module"`
    ToRepoID     string `json:"to_repo_id"`
    ToModule     string `json:"to_module"`
    Version      string `json:"version"`
}

// SharedDep represents an external dependency shared by multiple compared repos.
type SharedDep struct {
    ModulePath string            `json:"module_path"`
    RepoIDs    []string          `json:"repo_ids"`
    Versions   map[string]string `json:"versions"` // repo_id -> version
}
```

### 2. Update comparer to compute dependency relationships

**File:** `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/comparison/comparer.go`

In the `Compare()` method, after the existing analysis steps (duplicates, conflicts, gaps, consistency) and before `generateRecommendations`, add a new block to compute dependency relationships.

The logic works as follows:

1. Build a map from module_path to repo_id for all compared repos that have `ModuleInfo` set on their `RepoContext`. Section 05 adds `ModuleInfo *ModuleInfo` to `RepoContext`, so the comparer reads `rc.ModuleInfo` directly.
2. For each repo with `ModuleInfo`, iterate its `ModuleInfo.Dependencies`.
3. If a dependency's `Path` matches another compared repo's module path, record an `InternalDep`.
4. For external dependencies (not matching any compared repo), track which repos share them using a map of module_path to list of repo_ids and their versions.
5. Only include shared external deps that appear in two or more compared repos.
6. Store the result in `result.DependencyRelationships`. If no repos have `ModuleInfo`, leave the field nil.

Create a private helper method on the comparer:

```go
// findDependencyRelationships computes inter-repo dependency edges and shared external deps.
func (c *comparer) findDependencyRelationships(repoContexts []*ctxpkg.RepoContext) *DependencyRelationships {
    // Returns nil if no repos have ModuleInfo.
    // Otherwise returns populated DependencyRelationships.
}
```

### 3. Update `toolCompareRepos` output formatting

**File:** `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/mcp/tools.go`

In the `toolCompareRepos` function, insert rendering for the new `DependencyRelationships` field before the Recommendations section (before the block starting around line 814). The current flow is: Repos -> Unified Stats -> Duplicates -> Conflicts -> Gaps -> Consistency -> Recommendations. Insert the new section between Consistency and Recommendations.

The output format:

```
## Dependency Relationships

### Inter-Repo Dependencies

- **github.com/gorilla/handlers** depends on **github.com/gorilla/mux** (v1.8.0)

### Shared External Dependencies

- `golang.org/x/net` used by: github.com/gorilla/mux (v0.5.0), github.com/gorilla/handlers (v0.5.0)
```

If `result.DependencyRelationships` is nil, skip the entire section.

### 4. Update `getArchitectureContext` response

**File:** `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/mcp/tools.go`

Currently, `getArchitectureContext()` (line 181) serializes `ArchitectureContext` as raw JSON via `json.MarshalIndent`. Replace this with a markdown-formatted response that includes the existing architecture data plus new dependency sections.

The new response builds a `strings.Builder` with these sections:

1. **Overview** -- existing `Architecture.Overview`
2. **Build System** -- existing `Architecture.BuildSystem`
3. **Module Info** -- from `repoCtx.ModuleInfo` (if non-nil): module path, Go version
4. **Package Type** -- from `repoCtx.Architecture` (section 05 adds a `PackageType` field or it can be inferred from `MainPackages`)
5. **Entry Points** -- existing list
6. **Modules** -- existing list
7. **Dependencies** -- direct dependency count and list from `repoCtx.ModuleInfo.Dependencies` (filtered to `IsDirect == true`)
8. **Import Summary** -- from `repoCtx.ImportSummary` (if non-nil): count of stdlib, internal, and external imports

Access `ModuleInfo` and `ImportSummary` from `repoCtx.ModuleInfo` and `repoCtx.ImportSummary` -- fields added to `RepoContext` by section 05.

If `ModuleInfo` or `ImportSummary` are nil, simply skip those sections (graceful degradation for repos analyzed before dependency graph support was added).

### 5. Update AI prompt template for `ask`

**File:** `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/ai/query.go`

Add a new type and field to `RelevantContext`:

```go
// RepoDependencyInfo holds dependency context for a single repo.
type RepoDependencyInfo struct {
    RepoID              string
    ModulePath          string
    GoVersion           string
    PackageType         string   // "library" or "application"
    DirectDeps          []string // Module paths of direct dependencies
    ExternalImportCount int
    StdlibImportCount   int
    InternalImportCount int
}
```

Add to `RelevantContext`:

```go
type RelevantContext struct {
    // ... existing fields ...

    // DependencyInfo holds dependency data for repos in context.
    DependencyInfo []RepoDependencyInfo
}
```

Update `buildQueryPrompt()` to add a "## Dependencies" section after the existing "## Repositories" section but before "## Relevant Files". The section should only appear when `len(ctx.DependencyInfo) > 0`. For each entry, output:

```
### <RepoID>
- Module: <ModulePath>
- Go Version: <GoVersion>
- Type: <PackageType>
- Direct Dependencies: <count>
  - <dep1>
  - <dep2>
  ...
- Imports: <StdlibCount> stdlib, <InternalCount> internal, <ExternalCount> external
```

### 6. Update context extractor to populate dependency data

**File:** `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/ai/context_extractor.go`

In the `Extract()` method, after building `RepoSummary` entries for each repo, check if the `RepoContext` has `ModuleInfo` and/or `ImportSummary` populated. If so, build a `RepoDependencyInfo` entry:

- `RepoID` from `repoCtx.ID`
- `ModulePath` from `repoCtx.ModuleInfo.ModulePath`
- `GoVersion` from `repoCtx.ModuleInfo.GoVersion`
- `PackageType` inferred from whether `Architecture.MainPackages` is non-empty ("application") or empty ("library")
- `DirectDeps` from filtering `ModuleInfo.Dependencies` where `IsDirect == true`, taking the `Path` field
- Import counts from `ImportSummary`: `len(ImportSummary.Stdlib)`, `len(ImportSummary.Internal)`, `len(ImportSummary.External)`

Add each entry to `ctx.DependencyInfo`. This data is lightweight (a few strings and counts per repo) so it does not significantly impact the token budget.

### 7. Update `generateRecommendations` in comparer

**File:** `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/comparison/comparer.go`

In the `generateRecommendations()` method (line 635), after existing recommendation checks, add dependency-aware recommendations:

- If `result.DependencyRelationships` is non-nil and has `InternalDeps`, check for circular dependencies (A depends on B and B depends on A). If found, add recommendation: "Circular dependency detected between repos -- review dependency structure."
- If `result.DependencyRelationships.SharedExternalDeps` has entries where `Versions` map contains different version strings for the same module across repos, add recommendation: "Shared dependencies have version mismatches across repos -- consider aligning dependency versions."

## File Summary

| File | Action |
|------|--------|
| `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/comparison/types.go` | Add `DependencyRelationships`, `InternalDep`, `SharedDep` types; add field to `CompareResult` |
| `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/comparison/comparer.go` | Add `findDependencyRelationships()` method; call it in `Compare()`; update `generateRecommendations()` |
| `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/comparison/comparer_test.go` | Add `TestComparer_Compare_IncludesDependencyRelationships` |
| `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/mcp/tools.go` | Update `toolCompareRepos` formatting to render dependency relationships; rewrite `getArchitectureContext` to include module info, package type, dependencies, import summary |
| `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/mcp/tools_test.go` | Add `TestToolGetContext_Architecture_IncludesDependencyData` and `TestToolCompareRepos_IncludesDependencyRelationships` |
| `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/ai/query.go` | Add `RepoDependencyInfo` type; add `DependencyInfo` field to `RelevantContext`; update `buildQueryPrompt()` to include "## Dependencies" section |
| `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/ai/query_test.go` | Add `TestBuildQueryPrompt_IncludesDependencyContext` |
| `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/ai/context_extractor.go` | Populate `DependencyInfo` from `RepoContext.ModuleInfo` and `RepoContext.ImportSummary` in `Extract()` |

## Implementation Order

1. Add new types to `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/comparison/types.go`
2. Write test stubs in `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/comparison/comparer_test.go`
3. Implement `findDependencyRelationships()` and integrate into `Compare()` in `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/comparison/comparer.go`
4. Update `generateRecommendations()` in the same file
5. Update `toolCompareRepos` formatting in `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/mcp/tools.go`
6. Rewrite `getArchitectureContext` in `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/mcp/tools.go`
7. Add `RepoDependencyInfo` type and `DependencyInfo` field to `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/ai/query.go`
8. Update `buildQueryPrompt()` in `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/ai/query.go`
9. Update `Extract()` in `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/ai/context_extractor.go`
10. Write and run all tests