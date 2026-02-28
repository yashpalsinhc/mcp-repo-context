# Synthesized Spec: Dependency Graph & Import Analysis

## What We're Building

The MCP server currently has zero understanding of inter-repo dependencies. It cannot answer "which repos depend on each other?" — a fundamental question for any multi-repo codebase. This feature adds dependency graph construction from Go module files and import statements, plus a new MCP tool to visualize and query those relationships.

## Core Requirements

### 1. Parse go.mod Files (Structured Data)
- Use `golang.org/x/mod/modfile` to parse go.mod into structured data
- Extract: module path, Go version, require directives (direct + indirect), replace directives
- Create `ModuleDependency` type with fields: path, version, is_direct, is_replaced, replace_path
- Store parsed data in SQLite (hybrid: structured go.mod data persisted, cross-repo graph computed on-the-fly)

### 2. Parse Other Config Files (Structured Where Possible)
- **Dockerfile**: Extract base images, exposed ports, build stages
- **docker-compose.yml**: Extract services, ports, volumes, dependencies
- **Makefile**: Extract target names and their descriptions
- **CI configs** (.github/workflows/, .gitlab-ci.yml): Extract job names, triggers
- Cap stored content at 100KB per file
- Make content retrievable via `get_context(scope=file)`

### 3. Build Import Graph from .go Files
- Aggregate all imports across files in a repo (already extracted per-file)
- Classify imports: stdlib (via `go list std`), internal (same module), external (third-party)
- Resolve external imports to module paths using go.mod require directives
- Build per-repo import graph: "this repo uses packages from modules X, Y, Z"

### 4. Populate Architecture.Dependencies
- Fill the currently-empty `Dependencies` field in `ArchitectureContext`
- Distinguish direct vs indirect dependencies
- Include Go version requirement
- Detect package type (library vs application) based on main package presence
- For libraries, identify exported API surface as entry points

### 5. New Tool: get_dependency_graph
- Input: list of repo_ids (repo-level primary, org support later)
- Output: which repos depend on which modules, which analyzed repos provide those modules
- Mermaid/DOT visualization support (reuse existing call graph visualization pattern)

### 6. Enhance Existing Tools
- `compare_repos`: Show dependency relationships between compared repos
- `ask`: Enable answering "which repos depend on each other?"
- `get_context(scope=architecture)`: Include dependency data

## Technical Decisions

- **Analyzer approach:** Implementation plan decides (new dedicated analyzer vs extending generic)
- **go.mod parsing:** Use `golang.org/x/mod/modfile` (official, handles all edge cases)
- **Import classification:** Use `go list std` for stdlib detection (requires Go on system)
- **Storage:** Hybrid — persist parsed go.mod data in SQLite, compute cross-repo graph on-the-fly
- **Visualization:** Reuse existing `GraphVisualizer` pattern from call graph
- **Tool scope:** Repo-level primary for get_dependency_graph

## Dependencies

- **01-core-bug-fixes:** Reliable comparison logic (needed for cross-repo dependency matching)

## Testing Strategy

- **Unit tests:** Synthetic fixtures with known dependency relationships
- **Integration tests:** Clone and analyze real gorilla/* repos (gorilla/handlers depends on gorilla/mux)
- Verify `get_dependency_graph` output matches actual go.mod content

## Scale Requirements

- 50-200 repos per org
- Dependency graph must be queryable in <2s
