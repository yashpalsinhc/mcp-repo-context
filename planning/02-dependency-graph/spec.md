# Spec: Dependency Graph & Import Analysis

## Purpose

Enable the MCP server to understand how repos connect through Go module dependencies and import statements. Currently, inter-repo dependency information is completely absent — the server cannot answer "which repos depend on each other?"

## Background

See `planning/mcp-server-gaps-requirements.md` sections 4-5 and 9. The Go analyzer (`internal/analyzer/go_analyzer.go`) parses `.go` file ASTs but ignores `go.mod`. The architecture context `Dependencies` field is always empty. Config file content (go.mod, Dockerfile, etc.) is discarded by the generic analyzer.

## Scope

### 1. Parse go.mod Files
**Current state:** `internal/analyzer/generic_analyzer.go` treats go.mod as metadata-only (language, line count, purpose string). Content is discarded.

**Required:**
- Parse go.mod for: module path, Go version, require directives (direct + indirect), replace directives
- Store parsed dependencies as structured data (not raw content)
- Create new type (e.g., `ModuleDependency`) with fields: path, version, is_direct, is_replaced, replace_path
- Register a go.mod-specific analyzer in the registry, or extend the generic analyzer

### 2. Build Import Graph from .go Files
**Current state:** `go_analyzer.go:59-70` extracts imports per file but they're not aggregated.

**Required:**
- Aggregate all imports across files in a repo
- Classify imports: stdlib, internal (same module), external (third-party)
- For external imports, resolve to module path (from go.mod require directives)
- Build a per-repo import graph: "this repo uses packages from modules X, Y, Z"

### 3. Populate Architecture.Dependencies
**Current state:** `internal/orchestrator/manager.go:generateArchitecture()` never populates the Dependencies field.

**Required:**
- Populate `Architecture.Dependencies` from parsed go.mod data
- Distinguish direct vs indirect dependencies
- Include Go version requirement
- For library repos (no main.go), identify exported API surface as "entry points"
- Detect package type (library vs application) based on main package presence

### 4. Store Config File Content
**Current state:** Generic analyzer stores metadata only. `get_context(scope=file)` for go.mod returns "Language: go-mod, Lines: 4" with no content.

**Required:**
- Store content for key config files: go.mod, Dockerfile, docker-compose.yml, Makefile, CI configs (.github/workflows/, .gitlab-ci.yml)
- Cap stored content at reasonable size (e.g., 100KB per file)
- Make content retrievable via `get_context(scope=file)`

### 5. New Tool: get_dependency_graph
**Required:**
- Show inter-repo dependency relationships for analyzed repos
- Input: list of repo_ids (or org_id)
- Output: which repos depend on which modules, which analyzed repos provide those modules
- Mermaid/DOT visualization support (like existing call graph visualization)

### 6. Enhance Existing Tools
- `compare_repos`: Show dependency relationships between compared repos
- `ask`: Enable answering "which repos depend on each other?" and "what modules does repo X use?"
- `get_context(scope=architecture)`: Include dependency data

## Dependencies

- **01-core-bug-fixes:** Reliable comparison logic (needed for cross-repo dependency matching)

## Provides to Other Splits

- **03-api-flow-tracing:** Dependency graph enables identifying which repos are services that talk to each other
- **05-service-layer:** Dependency data needed for org-level analysis
- **06-agent-recipes:** Dependency context for architecture review recipes

## Key Technical Decisions

- Parse go.mod via regex or Go's `modfile` package (research in /deep-plan)
- Storage: extend existing SQLite schema or add new tables for dependencies
- Import classification heuristic: stdlib detection via known package list

## Research from Interview

- **Industry approach:** Greptile builds language-agnostic dependency graphs. Sourcegraph uses RAG with dependency-aware context.
- **User's scale:** 50-200 repos per org. Dependency graph must be queryable in <2s.
- **User's vision:** "Which repos depend on each other?" is a fundamental question the server must answer.

## Testing Strategy

- Test with gorilla/* repos (gorilla/handlers depends on gorilla/mux in its examples)
- Create a synthetic multi-repo test case with explicit inter-repo dependencies
- Verify `get_dependency_graph` output matches actual go.mod content
