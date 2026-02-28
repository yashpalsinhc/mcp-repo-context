# Implementation Plan: Dependency Graph & Import Analysis

## 1. Problem Statement

The MCP repo-context server analyzes Go repositories but has no understanding of inter-repo dependencies. The `ArchitectureContext.Dependencies` field is always empty. Config files like go.mod are treated as metadata-only — their content is discarded. Users cannot answer "which repos depend on each other?" or "what modules does repo X use?"

This plan adds structured dependency extraction from go.mod files, import graph construction from Go source files, config file content storage, and a new MCP tool for querying/visualizing dependency relationships.

## 2. Architecture Overview

### Data Flow

```
go.mod file → GoModAnalyzer → ModuleInfo (persisted in SQLite)
                                    ↓
.go files → GoAnalyzer → FileContext.Imports (already exists)
                                    ↓
                          ImportAggregator (new)
                                    ↓
                          DependencyGraph (computed on-the-fly at query time)
                                    ↓
                          get_dependency_graph tool → Mermaid/DOT output
```

### Storage Strategy (Hybrid)

**Persisted in SQLite:**
- Parsed go.mod data (module path, Go version, require/replace directives)
- Config file content (Dockerfile, docker-compose.yml, Makefile, CI configs)
- Import aggregation per repo (stdlib/internal/external classification)

**Computed on-the-fly:**
- Cross-repo dependency graph (which analyzed repos depend on each other)
- Visualization output (Mermaid/DOT diagrams)

This avoids complex graph schema in SQLite while keeping query time under 2s for 200 repos.

## 3. New Types

### context/types.go additions

```go
type ModuleInfo struct {
    ModulePath   string             `json:"module_path"`
    GoVersion    string             `json:"go_version"`
    Dependencies []ModuleDependency `json:"dependencies"`
    Replaces     []ModuleReplace    `json:"replaces,omitempty"`
}

type ModuleDependency struct {
    Path       string `json:"path"`
    Version    string `json:"version"`
    IsDirect   bool   `json:"is_direct"`
    IsReplaced bool   `json:"is_replaced,omitempty"`
}

type ModuleReplace struct {
    Old     string `json:"old"`
    New     string `json:"new"`
    Version string `json:"version,omitempty"`
}

type ImportSummary struct {
    Stdlib   []string         `json:"stdlib"`
    Internal []string         `json:"internal"`
    External []ExternalImport `json:"external"`
}

type ExternalImport struct {
    ImportPath string `json:"import_path"`
    ModulePath string `json:"module_path"`
    Version    string `json:"version,omitempty"`
}

type ConfigFile struct {
    Path           string          `json:"path"`
    Type           string          `json:"type"` // "go.mod", "dockerfile", "docker-compose", "makefile", "ci-config"
    Content        string          `json:"content,omitempty"`
    StructuredJSON json.RawMessage `json:"structured,omitempty"` // Typed parsed data, deserialized lazily via helper methods
    SizeBytes      int             `json:"size_bytes"`
}

// Helper methods for typed access:
// func (c *ConfigFile) AsDockerfile() (*DockerfileInfo, error)
// func (c *ConfigFile) AsCompose() (*ComposeInfo, error)
// func (c *ConfigFile) AsMakefile() (*MakefileInfo, error)
// func (c *ConfigFile) AsCIConfig() (*CIInfo, error)

type DependencyGraph struct {
    Nodes []DependencyNode `json:"nodes"`
    Edges []DependencyEdge `json:"edges"`
}

type DependencyNode struct {
    RepoID     string `json:"repo_id"`
    ModulePath string `json:"module_path"`
    IsAnalyzed bool   `json:"is_analyzed"` // true if this is a repo we've analyzed
    PackageType string `json:"package_type"` // "library" or "application"
}

type DependencyEdge struct {
    From    string `json:"from"`    // module_path of the dependent repo
    To      string `json:"to"`      // module_path of the dependency
    Version string `json:"version"`
    Direct  bool   `json:"direct"`
}
// Both From and To use module_path consistently. Look up RepoID from DependencyNode if needed.
```

### RepoContext additions

Add to the existing `RepoContext` struct:
- `ModuleInfo *ModuleInfo` — parsed go.mod data
- `ImportSummary *ImportSummary` — aggregated import classification
- `ConfigFiles []ConfigFile` — parsed config file content

## 4. go.mod Parser

### New file: `internal/analyzer/gomod_analyzer.go`

Create a dedicated go.mod analyzer (not inside generic_analyzer.go) because:
- go.mod parsing is complex enough to warrant its own file
- Uses `golang.org/x/mod/modfile` which has its own data structures
- Keeps the generic analyzer simple for truly generic files

**Approach:**
1. Read go.mod content as bytes
2. Parse with `modfile.Parse("go.mod", content)` — returns `*modfile.File`
3. Extract `File.Module.Mod.Path` as module path
4. Extract `File.Go.Version` as Go version
5. Iterate `File.Require` — map each to `ModuleDependency`, using `req.Indirect` for classification
6. Iterate `File.Replace` — map each to `ModuleReplace`
7. Return `ModuleInfo` struct

**Integration point:** Called from `manager.go` during `AnalyzeRepository()`, after file analysis but before architecture generation. The manager checks if a `go.mod` file was found and triggers go.mod parsing.

**Error handling:** If go.mod is malformed, log a warning and continue analysis without dependency data. Don't fail the entire repo analysis.

## 5. Config File Parsers

### New file: `internal/analyzer/config_parsers.go`

A collection of lightweight parsers for common config files. Each returns typed structured data.

**Dockerfile parser:**
- Extract `FROM` directives (base images, build stages)
- Extract `EXPOSE` ports
- Extract `CMD`/`ENTRYPOINT`
- Return `DockerfileInfo{BaseImages []string, Ports []string, Stages []string, Entrypoint string}`

**docker-compose.yml parser:**
- Use Go's `gopkg.in/yaml.v3` to parse YAML
- Extract service names, image/build config, ports, volumes, depends_on
- Return `ComposeInfo{Services []ComposeService}`

**Makefile parser:**
- Extract target names using regex: lines matching `^[a-zA-Z_-]+:` (not starting with tab)
- Extract target descriptions from comments above targets
- Return `MakefileInfo{Targets []MakeTarget}`

**CI config parser:**
- GitHub Actions: parse YAML for job names, triggers, steps
- GitLab CI: parse YAML for stage names, job names
- Return `CIInfo{Jobs []CIJob, Triggers []string}`

**Content cap:** Store raw content up to 100KB per file. If larger, truncate with a note.

**Registration:** Each parser is a function called from the generic analyzer when it detects the file type. The generic analyzer already identifies file purpose — extend it to call the appropriate parser and attach structured data.

## 6. Import Aggregation

### New file: `internal/analyzer/import_aggregator.go`

Aggregates per-file imports (already extracted by GoAnalyzer) into a per-repo `ImportSummary`.

**Steps:**
1. Collect all unique import paths across all files in the repo
2. Classify each import:
   - **Stdlib:** Use a compiled-in set of known stdlib packages (from `go/build` or a static list). Compare import paths against this set.
   - **Internal:** Import path starts with the module path from go.mod
   - **External:** Everything else
3. For external imports, first apply any replace directives from go.mod (check if the import path prefix matches a replaced module, and substitute the replacement), then resolve to module path by matching the import path prefix against go.mod require directives (longest prefix match). Both direct and indirect dependencies participate in the lookup.
4. Build `ImportSummary` with all three categories

**Stdlib detection detail:** Use Go's `go/build` standard library package to enumerate stdlib packages at compile time, or maintain a static set of known stdlib package paths. This avoids shelling out to `go list std` which is fragile (Go binary may not be on PATH, especially in Docker mode). The fallback heuristic "no dots in import path = stdlib" remains as a secondary check for any packages not in the static set.

**Integration point:** Called from `manager.go` after both file analysis and go.mod parsing are complete (needs both import data and module path).

## 7. Architecture Generation Updates

### Modify: `internal/orchestrator/manager.go`

In `generateArchitecture()`:

1. **Populate Dependencies field:** Convert `ModuleInfo.Dependencies` into the existing `[]string` format (just module paths for now). Later sections can make this richer.

2. **Detect package type:** Scan for `main` package in analyzed files. If present → "application". If absent → "library".

3. **Library entry points:** For libraries, scan for exported functions/types in root package and list them as entry points (the `EntryPoints` field already exists but is only populated for `main` packages).

4. **Add Go version:** Include the Go version from go.mod in the architecture overview.

## 8. Storage Schema Extension

### New migration: `internal/storage/migrations/004_dependency_graph.sql`

The migration must be idempotent since SQLite `ALTER TABLE ADD COLUMN` has no `IF NOT EXISTS`. Check `PRAGMA table_info(repos)` before each ALTER, or wrap in a Go migration function that checks column existence.

```sql
-- Store parsed go.mod data per repo (check column existence first via PRAGMA)
ALTER TABLE repos ADD COLUMN module_info_json TEXT;

-- Store import summary per repo
ALTER TABLE repos ADD COLUMN import_summary_json TEXT;

-- Extend files table for config file content and structured data
-- (avoids duplication between a separate config_files table and the existing files table)
ALTER TABLE files ADD COLUMN content TEXT;
ALTER TABLE files ADD COLUMN structured_json TEXT;
ALTER TABLE files ADD COLUMN config_type TEXT;  -- "dockerfile", "docker-compose", "makefile", "ci-config", null for regular files
```

**Note:** Each ALTER TABLE must be checked for column existence first using `PRAGMA table_info()` in the Go migration code, since SQLite does not support `ALTER TABLE ADD COLUMN IF NOT EXISTS`.

### SQLiteStore method additions

- `StoreModuleInfo(repoID string, info *ModuleInfo) error` — serialize to JSON, store in `module_info_json`
- `GetModuleInfo(repoID string) (*ModuleInfo, error)` — deserialize from JSON
- `GetModuleInfoBatch(repoIDs []string) (map[string]*ModuleInfo, error)` — batch load for dependency graph
- `StoreConfigFileContent(repoID, filePath string, content string, structuredJSON json.RawMessage, configType string) error` — update existing file row
- `GetConfigFiles(repoID string) ([]ConfigFile, error)` — query files with non-null config_type
- `StoreImportSummary(repoID string, summary *ImportSummary) error`
- `GetImportSummary(repoID string) (*ImportSummary, error)`

These follow the existing pattern of JSON serialization into TEXT columns.

### Security: Config File Blocklist

Never store raw content for files that may contain secrets:
- `.env`, `.env.*` files
- Files with "secret", "credential", or "token" in the name
- `*.pem`, `*.key` files

For these, store only the structured/parsed output (if any) with raw content omitted.

## 9. Dependency Graph Builder

### New file: `internal/graph/dependency_graph.go`

Computes cross-repo dependency graph on-the-fly from stored ModuleInfo data.

**Algorithm:**
1. Load `ModuleInfo` for all requested repo_ids
2. Build a map: module_path → repo_id (for repos that are Go modules we've analyzed)
3. For each repo, iterate its `ModuleInfo.Dependencies`
4. If a dependency's module_path matches an analyzed repo, create an edge (internal dependency)
5. Otherwise, create a node for the external module (not analyzed)
6. Return `DependencyGraph` with nodes and edges

**Performance:** Use a batch query (`SELECT id, module_info_json FROM repos WHERE id IN (...)`) to load all ModuleInfo in a single query. For 200 repos this is one query returning ~200 rows, then in-memory prefix matching. Well under 2s. Consider caching ModuleInfo in memory after first load since it only changes on re-analysis.

### Visualization

Extend existing `GraphVisualizer` with:
- `GenerateDependencyMermaid(graph *DependencyGraph) string` — generates Mermaid flowchart
- `GenerateDependencyDOT(graph *DependencyGraph) string` — generates DOT format

Node styling: analyzed repos as boxes, external modules as rounded boxes. Edge labels show version.

## 10. New MCP Tool: get_dependency_graph

### Tool definition

```
Name: get_dependency_graph
Description: Show inter-repo dependency relationships for analyzed repos
InputSchema:
  repo_ids: array of strings (required) — repo IDs to analyze
  format: string (optional, default "mermaid") — "mermaid" or "dot"
  include_external: boolean (optional, default true) — show external (non-analyzed) dependencies
```

### Handler: `internal/mcp/server.go`

1. Add tool definition in `handleListTools()`
2. Create `toolGetDependencyGraph()` handler
3. Add case in `handleCallToolWithID()` dispatch

The handler:
1. Validates repo_ids exist
2. Calls `manager.GetDependencyGraph(ctx, repoIDs)`
3. Formats as Mermaid/DOT via visualizer
4. Returns both the diagram and a text summary (repo X depends on Y, Z)

## 11. Existing Tool Enhancements

### compare_repos
In `internal/comparison/comparer.go`, after finding duplicates/conflicts/gaps:
- Add a "Dependency Relationships" section
- Show which compared repos depend on each other (from stored ModuleInfo)
- Show shared external dependencies

### get_context (scope=architecture)
In the architecture response, include:
- Module path and Go version
- Direct dependency count and list
- Package type (library/application)
- Import summary (stdlib/internal/external counts)

### ask (AI-powered)
Update the AI prompt template to include dependency data when available, so the AI can answer "which repos depend on each other?"

## 12. Manager Integration

### Modified: `internal/orchestrator/manager.go`

In the `AnalyzeRepository()` flow, add these steps after file analysis:

1. **Find go.mod:** Check if `go.mod` exists in analyzed files
2. **Parse go.mod:** Call `GoModAnalyzer.Parse(goModContent)`
3. **Parse config files:** For each detected config file (Dockerfile, docker-compose.yml, Makefile, CI configs), call the appropriate parser
4. **Aggregate imports:** Call `ImportAggregator.Aggregate(files, moduleInfo)`
5. **Update architecture:** Pass ModuleInfo to `generateArchitecture()` to populate Dependencies
6. **Persist:** Store ModuleInfo, ConfigFiles, ImportSummary via SQLiteStore

New manager interface methods (must be added to the `Manager` interface in addition to the `manager` struct):
- `GetDependencyGraph(ctx context.Context, repoIDs []string) (*DependencyGraph, error)` — builds cross-repo graph
- `GetConfigFiles(ctx context.Context, repoID string) ([]ConfigFile, error)` — retrieves stored config files

## 12b. Incremental Refresh Support

The existing `refresh_file` and `refresh_changed` features must handle the new data types:

**refresh_file on go.mod:**
- Re-parse ModuleInfo via GoModAnalyzer
- Re-aggregate imports (since module path may have changed)
- Update SQLite `module_info_json` and `import_summary_json`

**refresh_file on config files (Dockerfile, docker-compose.yml, etc.):**
- Re-parse structured data via the appropriate config parser
- Update the `content`, `structured_json`, and `config_type` columns in `files` table

**refresh_changed:**
- Detect changes to go.mod and config files in the change set
- Trigger appropriate re-parsing for each changed file type

## 13. Directory Structure

```
internal/
  analyzer/
    gomod_analyzer.go       # go.mod parser using x/mod/modfile
    gomod_analyzer_test.go
    config_parsers.go       # Dockerfile, docker-compose, Makefile, CI parsers
    config_parsers_test.go
    import_aggregator.go    # Import classification and aggregation
    import_aggregator_test.go
  graph/
    dependency_graph.go     # Cross-repo dependency graph builder
    dependency_graph_test.go
  storage/
    migrations/
      004_dependency_graph.sql  # Schema migration
  context/
    types.go                # Updated with new types (ModuleInfo, etc.)
  mcp/
    server.go               # Updated with get_dependency_graph tool
  orchestrator/
    manager.go              # Updated analysis flow
```

## 14. Dependencies (Go Modules)

New Go module dependencies to add:
- `golang.org/x/mod` — for `modfile.Parse()`
- `gopkg.in/yaml.v3` — for docker-compose.yml and CI config parsing (if not already present)

## 15. Error Handling Strategy

- **go.mod not found:** Skip dependency analysis, log info. Not an error — many repos are analyzed without go.mod.
- **go.mod malformed:** Log warning, skip dependency data. Don't fail the repo.
- **Stdlib detection edge cases:** If a package is not in the compiled-in stdlib set, fall back to "no dots" heuristic. Define `ErrGoModNotFound` and `ErrGoModMalformed` sentinel errors for programmatic handling.
- **Config file too large:** Truncate at 100KB, store with a note about truncation.
- **External module not in analyzed repos:** Show as external node in graph (not an error).

## 16. Testing Strategy

### Unit Tests (Synthetic Fixtures)

**gomod_analyzer_test.go:**
- Parse valid go.mod with direct + indirect deps
- Parse go.mod with replace directives (version-specific and wildcard)
- Parse go.mod with no dependencies
- Handle malformed go.mod gracefully

**config_parsers_test.go:**
- Parse multi-stage Dockerfile
- Parse docker-compose.yml with services and depends_on
- Parse Makefile with various target formats
- Parse GitHub Actions workflow YAML

**import_aggregator_test.go:**
- Classify stdlib imports correctly
- Classify internal imports (matching module path)
- Resolve external imports to module paths
- Handle aliased imports

**dependency_graph_test.go:**
- Build graph with inter-repo dependencies
- Handle repos with no dependencies
- Handle external-only dependencies
- Generate correct Mermaid/DOT output

### Integration Tests (Real Repos)

**Test with gorilla/* repos:**
- Analyze gorilla/mux and gorilla/handlers
- Verify gorilla/handlers shows dependency on gorilla/mux (if present in go.mod)
- Verify get_dependency_graph produces valid Mermaid diagram
- Verify compare_repos includes dependency relationships
