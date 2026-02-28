# Research: Plugin Interface

## Key Findings

### Analyzer Architecture (Already Plugin-Ready)
- **Interface** at `internal/analyzer/interface.go`: `Analyzer` with `Languages() []string` and `AnalyzeFile(ctx, file, content) (*FileContext, error)`
- **Registry** at `internal/analyzer/registry.go`: `Registry` interface with `Get(lang string) Analyzer`. Private implementation `registry` with map + fallback
- **Go Analyzer** at `internal/analyzer/go_analyzer.go`: Full AST parsing, deep analysis (behavior, call graphs, error handling, side effects, API flow)
- **Generic Analyzer** at `internal/analyzer/generic_analyzer.go`: Fallback for unsupported languages, basic metadata extraction
- Registry is **hardcoded** in `NewRegistry()` — only registers Go analyzer
- No `AnalyzeArchitecture` on the Analyzer interface (architecture analysis is done separately in orchestrator)

### Embedder Architecture (No Plugin System)
- **Interface** at `internal/vectors/embedder.go`: `Embedder` with `Embed(text) []float64`, `EmbedBatch(texts) [][]float64`, `Dimension() int`
- **LocalEmbedder**: TF-IDF based, 256-dimension, vocabulary building, code-specific preprocessing
- **No registry** for embedders — hardcoded `NewDefaultEmbedder()` in `internal/mcp/server.go`
- Embedder created in server constructor, not configurable

### Instantiation Points
- **Analyzer**: Created in `orchestrator.NewManager()` via `analyzer.NewRegistry()` — hardcoded
- **Embedder**: Created in `mcp.NewServer()` via `vectors.NewDefaultEmbedder()` — hardcoded
- Both need to be made configurable

### Org Config
- At `internal/org/types.go`: `OrgConfig` has `ExcludePatterns` and `MaxFileSize` only
- **No analyzer/embedder name fields** — needs extension
- Config merge logic exists in `internal/org/config.go`

### Existing Plugin Patterns in Codebase
- **AI Registry** (`internal/ai/registry.go`): `Registry` with map[string]Provider, Get by name
- **Skill Registry** (`internal/skills/types.go`): map-based, Get by name
- **Pattern Registry** (`internal/compose/patterns.go`): same pattern
- All follow the same map + Get pattern

### FileContext and ArchitectureContext
- At `internal/context/types.go`
- FileContext: rich structure with Path, Hash, Language, Functions, Types, Imports, etc.
- ArchitectureContext: Overview, Modules, EntryPoints, Dependencies, BuildSystem
- Both are returned types from analysis, consumed by storage and search

### Test Patterns
- Analyzer tests: create analyzer directly, pass content bytes, assert on FileContext fields
- Embedder tests: create embedder with config, build vocabulary, embed text, assert vector properties
- Registry tests: create registry, Get by language, check non-nil
