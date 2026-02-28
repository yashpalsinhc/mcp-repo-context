# Research: Dependency Graph & Import Analysis

## Part 1: Codebase Research

### 1. Go Analyzer - Import Extraction and AST Patterns

**File:** `internal/analyzer/go_analyzer.go`

The Go analyzer extracts imports during AST parsing in `AnalyzeFile()` (lines 59-70):
- Uses `parser.ParseFile()` with `parser.ParseComments` flag
- Extracts import path and alias from `ast.ImportSpec`
- Stores as `Import{Path, Alias}` in `FileContext.Imports`

Import type in `context/types.go` (lines 134-138):
```go
type Import struct {
    Path  string `json:"path"`
    Alias string `json:"alias,omitempty"`
}
```

AST walking patterns used:
- Two-phase declaration extraction: `GenDecl` (types/consts/vars) and `FuncDecl` (functions/methods)
- `CallGraphBuilder` uses `ast.Inspect()` recursively for `ast.CallExpr` nodes
- Import usage tracking builds `importMap` (alias → path) and detects `SelectorExpr` nodes

### 2. Generic Analyzer - Non-Go File Handling

**File:** `internal/analyzer/generic_analyzer.go`

Currently treats go.mod as metadata-only (language, line count, purpose string). Content is discarded. No specialized handling for manifest files — this is the primary extension point.

### 3. Architecture Generation

**File:** `internal/orchestrator/manager.go` (lines 408-473)

`generateArchitecture()` groups files by directory into modules, detects main packages and entry points, identifies build system (go.mod, Makefile, package.json). **Does NOT populate Dependencies field.**

ArchitectureContext type (context/types.go, lines 321-330):
```go
type ArchitectureContext struct {
    Overview        string           `json:"overview"`
    Modules         []Module         `json:"modules"`
    EntryPoints     []EntryPoint     `json:"entry_points"`
    Dependencies    []string         `json:"dependencies"`  // EMPTY - needs implementation
    BuildSystem     string           `json:"build_system"`
    MainPackages    []string         `json:"main_packages"`
    AIAnalysis      *AIArchAnalysis  `json:"ai_analysis,omitempty"`
}
```

### 4. Call Graph Architecture (Reusable Pattern)

**File:** `internal/analyzer/callgraph.go`

Key types:
- `CallGraph` with `Nodes map[string]*CallGraphNode` and `Edges []CallGraphEdge`
- `CallGraphNode` has ID, File, Function, Signature, Package, Calls, CalledBy
- `CallGraphEdge` has From, To, Line, CallType

Build pattern: First pass registers nodes, second pass builds edges from `fn.Calls`.

Integration in manager (lines 166-169):
```go
callGraphBuilder := analyzer.NewCallGraphBuilder()
repoCtx.CallGraph = callGraphBuilder.BuildFromFiles(repoCtx.Files)
```

### 5. Storage and Database Schema

**File:** `internal/storage/sqlite.go` and migrations

Tables: `repos`, `files` (with `imports_json`), `functions`, `types`. No dependency-specific tables yet.

Extension approach: Add new migration file (e.g., `004_dependency_graph.sql`), implement store/retrieve methods.

### 6. Call Graph Visualization (Reusable Pattern)

**File:** `internal/graph/visualizer.go`

`GenerateMermaid()` (lines 33-89) builds call tree, traverses callers/callees to depth N, generates Mermaid flowchart syntax. Same pattern applies to dependency graphs.

### 7. MCP Tool Registration

**File:** `internal/mcp/server.go`

Pattern: Define tool in `handleListTools()` with InputSchema → Create handler `toolXxx()` → Add case in `handleCallToolWithID()` → Return `callToolResult`.

### 8. Testing Patterns

- Standard Go `testing` package (no external framework)
- Temporary directories for I/O tests
- Mock objects implementing `orchestrator.Manager` interface
- Test data builders like `createTestRepoContext()`

### 9. RepoContext Data Model

```go
type RepoContext struct {
    ID           string                  `json:"id"`
    URL          string                  `json:"url"`
    Branch       string                  `json:"branch"`
    CommitHash   string                  `json:"commit_hash"`
    AnalyzedAt   time.Time               `json:"analyzed_at"`
    Files        map[string]*FileContext `json:"files"`
    Architecture *ArchitectureContext    `json:"architecture"`
    Statistics   RepoStatistics          `json:"statistics"`
    Version      int                     `json:"version"`
    AISummary    *AISummary              `json:"ai_summary,omitempty"`
    CallGraph    *CallGraph              `json:"call_graph,omitempty"`
    SearchIndex  *SearchIndex            `json:"search_index,omitempty"`
}
```

### Key Integration Points Summary

| Component | Location | Extension Point |
|-----------|----------|-----------------|
| Import Extraction | `go_analyzer.go:59-70` | Already working; need manifest parsing |
| Architecture Gen | `manager.go:408-473` | Populate `Dependencies` field |
| Call Graph | `callgraph.go` | Reuse pattern for module dependencies |
| Visualization | `visualizer.go` | Add `GenerateDependencyDiagram()` |
| Storage | `sqlite.go` + migrations | Add dependency tables |
| MCP Tools | `server.go:1083-1161` | Register new dependency tools |
| Types | `context/types.go` | Add `DependencyGraph` type |
| Search Index | `searchindex.go` | Add `Dependencies` index |

---

## Part 2: Web Research

### 1. Go modfile Package for Parsing go.mod

**Package:** `golang.org/x/mod/modfile`

Key data structures:
- `Module` — module declaration with `Mod module.Version`, `Deprecated string`
- `Require` — dependency with `Mod module.Version`, `Indirect bool`
- `Replace` — with `Old` and `New` module.Version
- `Exclude` — prevents specific version selection

**Direct vs Indirect:** Check `Require.Indirect` boolean. In Go 1.17+, all transitive imports are listed (module graph pruning).

**Parsing:**
```go
content, _ := os.ReadFile("go.mod")
file, err := modfile.Parse("go.mod", content)
for _, req := range file.Require {
    if !req.Indirect {
        // Direct dependency
    }
}
```

**Replace directive gotchas:**
- Only apply in main module (ignored in dependency go.mod files)
- Version-specific vs wildcard replacements
- Local path replacements use `./` or `../` without version
- Cannot replace same module twice

**Sources:** [golang.org/x/mod/modfile](https://pkg.go.dev/golang.org/x/mod/modfile), [Go Modules Reference](https://go.dev/ref/mod)

### 2. Go Import Graph Building Patterns

**Option A: `golang.org/x/tools/go/packages` (higher-level)**
- One-stop-shop for package loading, AST parsing, type checking
- Simpler code but potentially slower for large codebases
- Good for: full type-aware analysis

**Option B: Manual AST walking with `go/parser` (lower-level)**
- Fine-grained control, better performance
- Use `parser.ImportsOnly` for import-only analysis
- Good for: import-focused analysis (our use case)

**Recommendation for this project:** Use manual AST walking since the codebase already does this pattern. The Go analyzer already extracts imports via AST — we just need to aggregate them.

**Import classification:**
1. **Standard library:** Check against known stdlib packages
2. **Internal:** Match against module path from go.mod
3. **External:** Everything else
4. **Vendored:** Has `/vendor/` in path

**Resolving imports to modules:**
1. Parse go.mod for require directives
2. Match import path prefix against module paths
3. Handle replace directives (check before matching)

**Performance tips:**
- Use `parser.ImportsOnly` for import-only analysis
- Cache parsed ASTs for repeated traversals
- For 5+ traversals of same AST, use `golang.org/x/tools/go/ast/inspector` (~2.5x faster)
- Process packages in parallel for large codebases

**Sources:** [Cloudflare blog on Go static analysis](https://blog.cloudflare.com/building-the-simplest-go-static-analysis-tool/), [Eli Bendersky on multi-package tools](https://eli.thegreenplace.net/2020/writing-multi-package-analysis-tools-for-go/)

### 3. SQLite Schema Migration Patterns in Go

**Recommended approach:** This project already uses manual SQL migration files (001_initial_schema.sql, etc.). Continue this pattern rather than introducing golang-migrate or goose.

**Best practices:**
- Sequential numbering: `004_dependency_graph.sql`
- One logical change per migration
- Use `IF NOT EXISTS` / `IF NOT TARGET_EXISTS` for idempotency
- SQLite wraps each migration in implicit transaction — don't add explicit `BEGIN`/`COMMIT`
- Add columns with defaults for backward compatibility

**Existing pattern in this project:** Manual SQL files in migrations directory, executed at store initialization. Follow the same approach for new dependency tables.

**Sources:** [golang-migrate documentation](https://github.com/golang-migrate/migrate), [Database migrations in Go](https://betterstack.com/community/guides/scaling-go/golang-migrate/)
