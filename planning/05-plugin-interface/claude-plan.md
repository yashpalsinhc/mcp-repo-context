# Implementation Plan: Plugin Interface

## Overview

Add plugin architecture for analyzers and embedders. Extend existing interfaces with `Name()` method, create registries with closed constructor pattern, update OrgConfig for per-org plugin selection, and rewire Manager/Server to use registries.

**Plugin model:** Compile-time only. New analyzers/embedders are added by writing Go code and recompiling. No runtime discovery or dynamic loading.

**Registries are immutable after construction.** No concurrent mutation concerns. Thread-safe for reads by design.

## Current Architecture

### What Exists
- **Analyzer interface** (`internal/analyzer/interface.go`): `Analyzer` with `Languages()` and `AnalyzeFile()`. `Registry` interface with `Get(lang)`.
- **Private registry** (`internal/analyzer/registry.go`): `registry` struct, hardcoded Go + generic analyzer registration in `NewRegistry()`.
- **Embedder interface** (`internal/vectors/embedder.go`): `Embedder` with `Embed()`, `EmbedBatch()`, `Dimension()`. No registry.
- **LocalEmbedder**: TF-IDF based, hardcoded in `mcp.NewServer()` via `NewDefaultEmbedder()`.
- **Manager** (`internal/orchestrator/manager.go`): Creates `analyzer.NewRegistry()` internally. Also has `NewManagerWithAI()` that also hardcodes registry.
- **Server** (`internal/mcp/server.go`): Creates `vectors.NewDefaultEmbedder()` internally.
- **OrgConfig** (`internal/org/types.go`): Only has `ExcludePatterns` and `MaxFileSize`. `copyConfig` helper copies these two fields.
- **Existing plugin patterns**: AI Registry, Skill Registry, Pattern Registry — all use map + Get pattern.

### What's Missing
1. `Name()` method on Analyzer and Embedder interfaces
2. Configurable analyzer registry constructor
3. Embedder registry
4. OrgConfig plugin name fields
5. Manager/Server accepting registries as parameters

## Section-by-Section Plan

### Section 1: Extend Analyzer Interface & Registry

**Goal:** Add `Name()` to Analyzer interface, make registry accept custom analyzers via closed constructor.

**Breaking change audit:** Before modifying the interface, grep for all `Analyzer` and `Embedder` implementations in the codebase including test files. Update every mock/stub that implements these interfaces to add the `Name()` method.

**Changes to `internal/analyzer/interface.go`:**

Add `Name() string` to the `Analyzer` interface. This is a breaking change — all implementations (goAnalyzer, genericAnalyzer, test mocks) must add a Name method.

**Changes to `internal/analyzer/registry.go`:**

Replace hardcoded `NewRegistry()` with two constructors:

```
func NewRegistry(analyzers ...Analyzer) Registry
```

`NewRegistry()` with no args creates an **empty registry** with generic analyzer as fallback only. Does NOT auto-register Go analyzer. This is a clean slate constructor.

```
func DefaultRegistry() Registry
```

`DefaultRegistry()` is the **only way to get built-in analyzers**. Returns `NewRegistry(NewGoAnalyzer())`. This is the canonical path for standard usage. No dual-path drift risk.

The constructor:
1. Creates the map
2. Sets generic analyzer as fallback (always)
3. For each provided analyzer, registers it for all its `Languages()`
4. **Duplicate language handling:** Last registration wins. If two analyzers claim the same language, the last one in the variadic list takes precedence. Document this behavior.

Keep the `Get(lang) Analyzer` method unchanged.

**Changes to Go analyzer (`internal/analyzer/go_analyzer.go`):**
- Make `goAnalyzer` constructor public: `NewGoAnalyzer() Analyzer`
- Add `Name() string` returning `"go"`

**Changes to generic analyzer (`internal/analyzer/generic_analyzer.go`):**
- Make constructor public: `NewGenericAnalyzer() Analyzer`
- Add `Name() string` returning `"generic"`

### Section 2: Embedder Registry

**Goal:** Create a registry for embedders, similar to the analyzer registry pattern.

**Dimension validation:** All embedders in a registry MUST have the same `Dimension()`. The constructor validates this at construction time and returns an error if dimensions differ. This prevents silent data corruption from mixing vector dimensions in the same store.

**New file: `internal/vectors/registry.go`**

Define `EmbedderRegistry` interface:

```
type EmbedderRegistry interface {
    Get(name string) Embedder
    Default() Embedder
    Names() []string
}
```

Implementation:

```
type embedderRegistry struct {
    embedders  map[string]Embedder
    defaultEmb Embedder
    dimension  int  // shared dimension, validated at construction
}
```

Constructor (returns error for validation):

```
func NewEmbedderRegistry(defaultEmbedder Embedder, extras ...Embedder) (EmbedderRegistry, error)
```

- First argument is the default embedder (used when no name specified). Must not be nil.
- Extra embedders registered by `Name()`. Duplicate names: last wins.
- **Validates all embedders have same Dimension().** If mismatch, return error.

**Extend Embedder interface** in `internal/vectors/embedder.go`:

Add `Name() string` to the `Embedder` interface. Update all test mocks.

**Update LocalEmbedder:**
- Add `Name() string` returning `"local"`

**Convenience constructor:**

```
func DefaultEmbedderRegistry() EmbedderRegistry
```

Returns `NewEmbedderRegistry(NewDefaultEmbedder())`. Panics on error (should never fail with defaults).

### Section 3: OrgConfig Extension

**Goal:** Add analyzer and embedder name fields to OrgConfig for per-org plugin selection.

**Changes to `internal/org/types.go`:**

Add to OrgConfig:
```
AnalyzerName string `json:"analyzer_name,omitempty"`
EmbedderName string `json:"embedder_name,omitempty"`
```

**Changes to `internal/org/config.go`:**

Update `MergeConfigs` to merge analyzer/embedder names. Per-repo override takes precedence. If override has empty string, falls back to org-level.

**Update `copyConfig` helper** to copy the two new fields. Without this, the nil-check branches in MergeConfigs will lose AnalyzerName and EmbedderName.

**Validation:** At use time, not at save time. When the MCP server resolves org config before analysis, it checks the names against the registry. If unknown, surface warning in the analysis result (not silent — user sees the warning in tool output).

### Section 4: Manager & Server Wiring

**Goal:** Update Manager and Server constructors to accept registries instead of creating them internally.

**Changes to `internal/orchestrator/manager.go`:**

Update `NewManager` to use functional options:

```
func NewManager(store storage.ContextStore, cloner repo.Source, scanner repo.FileScanner, opts ...ManagerOption) Manager
```

Add ManagerOption functions:
- `WithAnalyzerRegistry(registry analyzer.Registry) ManagerOption`
- `WithAIRegistry(reg *ai.Registry) ManagerOption`
- Default: `analyzer.DefaultRegistry()` if no analyzer registry provided
- Default: `ai.NewRegistryFromEnv()` if no AI registry provided

**Remove `NewManagerWithAI` constructor.** Fold its functionality into `WithAIRegistry` option. All callers of `NewManagerWithAI` updated to use `NewManager(..., WithAIRegistry(reg))`.

The manager uses the registry via `m.registry.Get(lang)` — this is unchanged. The difference is the registry is now injectable.

**Add AnalyzerName/EmbedderName to AnalyzeOptions:**

```
type AnalyzeOptions struct {
    // ... existing fields
    AnalyzerName string  // Optional: use specific analyzer (resolved from org config by caller)
}
```

The manager stays org-agnostic. The MCP server layer resolves org config and passes the analyzer name to AnalyzeOptions. The manager looks up the name in its registry.

**Changes to `internal/mcp/server.go`:**

Update `ServerConfig` to include embedder registry:

```
type ServerConfig struct {
    // ... existing fields
    EmbedderRegistry vectors.EmbedderRegistry  // NEW
}
```

In `NewServer`, use the registry:

```
if config.VectorStore != nil {
    var embedder vectors.Embedder
    if config.EmbedderRegistry != nil {
        embedder = config.EmbedderRegistry.Default()
    } else {
        embedder = vectors.NewDefaultEmbedder()
    }
    s.semanticSearch = vectors.NewSemanticSearch(embedder, config.VectorStore)
}
```

Store registry on server for per-org selection:

```
s.embedderRegistry = config.EmbedderRegistry
```

**Changes to `cmd/mcp-server/main.go`:**

Update to create registries and pass them:

```
analyzerReg := analyzer.DefaultRegistry()
embedderReg := vectors.DefaultEmbedderRegistry()
manager := orchestrator.NewManager(store, cloner, scanner,
    orchestrator.WithAnalyzerRegistry(analyzerReg),
    orchestrator.WithAIRegistry(ai.NewRegistryFromEnv()),
)
serverConfig.EmbedderRegistry = embedderReg
```

### Section 5: Per-Org Plugin Selection

**Goal:** When analyzing repos in an org, use the org's configured analyzer/embedder if specified.

**Flow (MCP server layer, not manager):**

When a tool handler triggers analysis for a repo:
1. MCP server checks if repo belongs to an org (via OrgManager)
2. If yes, load org config
3. If `AnalyzerName` set, pass it in AnalyzeOptions
4. If `EmbedderName` set, use it for embedder selection

**Manager behavior with AnalyzerName:**

In `AnalyzeRepo`/`AnalyzeLocal`:
1. If `options.AnalyzerName` is set, look up in registry by iterating registered analyzers and matching `Name()`
2. If found, use that analyzer for all files it claims to handle (via its `Languages()`)
3. If not found, **log warning and fall back to default**. Add warning to result: `"Analyzer '{name}' not found, using default"`
4. If not set, use standard language-based dispatch (current behavior)

**Embedder selection (MCP server):**

When indexing (index_repository, auto-index):
1. Check if repo belongs to org with EmbedderName
2. If so, get from embedder registry via `Get(name)`
3. If not found, use `Default()` and add warning to output
4. All embedders in registry share same dimension (validated at construction), so no mismatch risk

**Warning surfacing:** Fallback warnings appear in the tool's markdown output, not just logs. Example: "Warning: Analyzer 'python' not found in registry, using default analyzer."

### Section 6: Integration Tests

**Goal:** End-to-end tests verifying the plugin architecture works.

**Test scenarios:**

Analyzer plugin:
1. NewRegistry with custom analyzer, verify Get returns it for its languages
2. DefaultRegistry returns Go analyzer for .go files
3. NewRegistry with no args: fallback to generic for all languages
4. Name() method returns correct name for each built-in analyzer
5. Manager with custom registry uses custom analyzer
6. Duplicate language: last registration wins
7. All test mocks compile with Name() method

Embedder plugin:
1. NewEmbedderRegistry with custom embedder, Get by name returns it
2. Default() returns the default embedder
3. Names() lists all registered embedders
4. Server with custom registry uses custom embedder
5. Dimension mismatch at construction returns error
6. Nil default embedder returns error

OrgConfig:
1. OrgConfig with AnalyzerName serializes/deserializes correctly (JSON)
2. MergeConfigs preserves analyzer/embedder names
3. Per-repo override takes precedence over org-level
4. copyConfig copies new fields correctly

Per-org selection:
1. Org with AnalyzerName: analysis uses that analyzer
2. Unknown analyzer name: falls back to default, warning in output
3. Org with no analyzer name: uses default (standard dispatch)
4. Unknown embedder name: falls back to default, warning in output

## Error Handling

- Unknown analyzer name in OrgConfig: log warning + surface in output, use default (never fail analysis)
- Unknown embedder name in OrgConfig: log warning + surface in output, use default
- No analyzers passed to NewRegistry: empty registry (generic fallback only)
- No embedder passed to NewEmbedderRegistry: return error (at least one required)
- Dimension mismatch in NewEmbedderRegistry: return error with dimension values

## Performance Considerations

- Registry lookups are map-based O(1) — no performance impact
- Registries are immutable after construction — no locking needed for reads
- Single shared embedder means no memory duplication
- Analyzer selection per file is already done via registry.Get — no change
