# Split 05: Plugin Interface (Phase 2)

## Purpose

Add plugin architecture for analyzers and embedders. Enables org-specific analyzers (e.g. TypeScript) and embedding model swaps.

## Context

- **Requirements:** `/mcp-repo-context/requirements.md`
- **Design:** `/mcp-repo-context/docs/DESIGN_ORG_SEMANTIC_SEARCH.md`
- **Current:** Single Go analyzer, single embedder; no extensibility

## Scope

### In Scope

1. **AnalyzerPlugin interface** — Load, register, run
2. **EmbedderPlugin interface** — Load, register, embed
3. **Registry** — Discover plugins by name
4. **Built-in defaults** — Default implementations as plugins

### Out of Scope

- Full plugin discovery from filesystem
- Plugin versioning
- Storage plugin (future)

## Technical Details

### AnalyzerPlugin

```go
type AnalyzerPlugin interface {
    Name() string
    Languages() []string
    AnalyzeFile(ctx, file, content) (*FileContext, error)
    AnalyzeArchitecture(ctx, repoPath, files) (*ArchitectureContext, error)
}
```

### EmbedderPlugin

```go
type EmbedderPlugin interface {
    Name() string
    Dimension() int
    Embed(ctx, text string) ([]float64, error)
    EmbedBatch(ctx, texts []string) ([][]float64, error)
}
```

### Registry

- `RegisterAnalyzer(name, plugin)`
- `RegisterEmbedder(name, plugin)`
- Config: `org_id` or global can specify which plugin to use

## Dependencies

- Split 01 (org config can reference plugin names)
- `internal/analyzer`
- `internal/vectors`

## Verification

- [ ] Built-in Go analyzer registered as plugin
- [ ] Built-in embedder registered as plugin
- [ ] External plugin can be loaded (if mechanism exists)
- [ ] Config can specify plugin per org

## Priority

**Phase 2** — defer until org abstraction, index, and search are stable. Can run in parallel with Split 04 if needed.
