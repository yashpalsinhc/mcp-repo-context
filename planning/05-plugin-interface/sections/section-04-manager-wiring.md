# Section 4: Manager & Server Wiring

## Overview

Update Manager to use functional options pattern (replacing hardcoded registry creation), remove NewManagerWithAI in favor of WithAIRegistry option, update ServerConfig with EmbedderRegistry, and update main.go wiring.

## Dependencies

- Section 1 (analyzer registry constructors)
- Section 2 (embedder registry)

## Tests First

### File: `internal/orchestrator/manager_test.go` (extend existing)

```
Test: NewManager with no options uses DefaultRegistry
- m := NewManager(store, cloner, scanner)
- Analyze a .go file
- Assert Go analyzer was used (not generic)

Test: NewManager WithAnalyzerRegistry uses custom registry
- mockAnalyzer with Name="custom", Languages=["custom"]
- customReg := analyzer.NewRegistry(mockAnalyzer)
- m := NewManager(store, cloner, scanner, WithAnalyzerRegistry(customReg))
- Assert m's registry returns mockAnalyzer for "custom"

Test: NewManager WithAIRegistry
- aiReg := ai.NewRegistry()
- m := NewManager(store, cloner, scanner, WithAIRegistry(aiReg))
- Assert AI registry is set on manager

Test: NewManager with both options
- m := NewManager(store, cloner, scanner, WithAnalyzerRegistry(reg), WithAIRegistry(aiReg))
- Assert both are set

Test: AnalyzeOptions AnalyzerName respected
- Register analyzer with Name="special" in registry
- m := NewManager(store, cloner, scanner, WithAnalyzerRegistry(analyzer.NewRegistry(specialAnalyzer)))
- Call AnalyzeRepo/AnalyzeLocal with options.AnalyzerName="special"
- Assert specialAnalyzer was used
```

### File: `internal/mcp/server_test.go` (extend existing)

```
Test: NewServer with EmbedderRegistry
- reg := vectors.DefaultEmbedderRegistry()
- config := &ServerConfig{EmbedderRegistry: reg, VectorStore: vs}
- s := NewServer(manager, comparer, config)
- Assert semantic search initialized with registry's default

Test: NewServer without EmbedderRegistry (backward compat)
- config := &ServerConfig{VectorStore: vs}
- s := NewServer(manager, comparer, config)
- Assert semantic search works (NewDefaultEmbedder used)

Test: NewServer without VectorStore (no semantic search)
- config := &ServerConfig{}
- s := NewServer(manager, comparer, config)
- Assert no panic, semantic search is nil
```

## Implementation Details

### 1. ManagerOption Type

**File: `internal/orchestrator/manager.go`**

Define functional option type:

```go
type ManagerOption func(*manager)

func WithAnalyzerRegistry(reg analyzer.Registry) ManagerOption {
    return func(m *manager) {
        m.registry = reg
    }
}

func WithAIRegistry(reg *ai.Registry) ManagerOption {
    return func(m *manager) {
        m.aiRegistry = reg
    }
}
```

### 2. Update NewManager

Change signature to accept variadic options:

```go
func NewManager(store storage.ContextStore, cloner repo.Source, scanner repo.FileScanner, opts ...ManagerOption) Manager {
    m := &manager{
        store:      store,
        cloner:     cloner,
        scanner:    scanner,
        registry:   analyzer.DefaultRegistry(),    // default
        aiRegistry: ai.NewRegistryFromEnv(),        // default
        locks:      NewLockManager(),
    }
    for _, opt := range opts {
        opt(m)
    }
    return m
}
```

### 3. Remove NewManagerWithAI

Delete `NewManagerWithAI` function. Update all callers to use:
```go
NewManager(store, cloner, scanner, WithAIRegistry(aiReg))
```

Search for all `NewManagerWithAI` call sites and replace.

### 4. Add AnalyzerName to AnalyzeOptions

If an `AnalyzeOptions` struct exists, add `AnalyzerName string` field. If not, add it to the analysis parameters.

In the analysis flow (AnalyzeRepo/AnalyzeLocal), when AnalyzerName is set:
1. Iterate registered analyzers to find one with matching Name()
2. If found, use that analyzer for all files it handles
3. If not found, log warning, use standard language dispatch

The manager does NOT know about orgs. The MCP server layer resolves org config and passes AnalyzerName via options.

### 5. Update ServerConfig

**File: `internal/mcp/server.go`**

Add `EmbedderRegistry` to ServerConfig:

```go
type ServerConfig struct {
    Name             string
    Version          string
    GitHubToken      string
    VectorStore      *vectors.SQLiteVectorStore
    UsageTracker     *analytics.UsageTracker
    OrgManager       org.Manager
    EmbedderRegistry vectors.EmbedderRegistry  // NEW
}
```

In `NewServer`:
```go
if config.VectorStore != nil {
    var embedder vectors.Embedder
    if config.EmbedderRegistry != nil {
        embedder = config.EmbedderRegistry.Default()
    } else {
        embedder = vectors.NewDefaultEmbedder()
    }
    s.semanticSearch = vectors.NewSemanticSearch(embedder, config.VectorStore)
    s.embedderRegistry = config.EmbedderRegistry
}
```

### 6. Update main.go

**File: `cmd/mcp-server/main.go`**

```go
analyzerReg := analyzer.DefaultRegistry()
embedderReg := vectors.DefaultEmbedderRegistry()

manager := orchestrator.NewManager(store, cloner, scanner,
    orchestrator.WithAnalyzerRegistry(analyzerReg),
    orchestrator.WithAIRegistry(ai.NewRegistryFromEnv()),
)

serverConfig := &mcp.ServerConfig{
    // ... existing fields
    EmbedderRegistry: embedderReg,
}
```

## Error Handling

- No options provided: all defaults applied (backward compatible)
- Nil registry in option: could check and skip, or let it panic at use time. Prefer: check nil in option function and no-op.

## File Summary

| File | Action |
|------|--------|
| `internal/orchestrator/manager.go` | Modify: add ManagerOption, update NewManager, remove NewManagerWithAI, add AnalyzerName to options |
| `internal/orchestrator/manager_test.go` | Modify: add tests for options pattern |
| `internal/mcp/server.go` | Modify: add EmbedderRegistry to ServerConfig, use in NewServer |
| `internal/mcp/server_test.go` | Modify: add tests for registry in config |
| `cmd/mcp-server/main.go` | Modify: create registries and pass to constructors |
| All callers of NewManagerWithAI | Modify: switch to NewManager with WithAIRegistry |
