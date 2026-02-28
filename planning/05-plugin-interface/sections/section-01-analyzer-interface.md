# Section 1: Extend Analyzer Interface & Registry

## Overview

Add `Name() string` to the Analyzer interface, make the registry configurable via a closed constructor pattern, and expose Go/Generic analyzer constructors publicly.

## Dependencies

None — this is a foundation section.

## Tests First

### File: `internal/analyzer/registry_test.go` (extend existing)

```
Test: NewGoAnalyzer returns analyzer with Name "go"
- a := NewGoAnalyzer()
- Assert a.Name() == "go"
- Assert a.Languages() contains "go"

Test: NewGenericAnalyzer returns analyzer with Name "generic"
- a := NewGenericAnalyzer()
- Assert a.Name() == "generic"

Test: NewRegistry with no args returns empty registry (generic fallback only)
- r := NewRegistry()
- Assert r.Get("go").Name() == "generic" (no Go analyzer registered)
- Assert r.Get("python").Name() == "generic"

Test: DefaultRegistry returns Go analyzer for "go"
- r := DefaultRegistry()
- Assert r.Get("go").Name() == "go"

Test: DefaultRegistry falls back to generic for unknown
- r := DefaultRegistry()
- Assert r.Get("python").Name() == "generic"

Test: NewRegistry with custom analyzer
- Create mock analyzer: Name="python", Languages=["py","python"]
- r := NewRegistry(mockAnalyzer)
- Assert r.Get("py").Name() == "python"
- Assert r.Get("python").Name() == "python"
- Assert r.Get("go").Name() == "generic" (not registered)

Test: Duplicate language last wins
- analyzer1 with Name="first", Languages=["go"]
- analyzer2 with Name="second", Languages=["go"]
- r := NewRegistry(analyzer1, analyzer2)
- Assert r.Get("go").Name() == "second"

Test: Multiple analyzers registered
- r := NewRegistry(NewGoAnalyzer(), mockPythonAnalyzer)
- Assert r.Get("go").Name() == "go"
- Assert r.Get("py").Name() == "python"
- Assert r.Get("rust").Name() == "generic"
```

## Implementation Details

### 1. Add Name() to Analyzer Interface

**File: `internal/analyzer/interface.go`**

Add `Name() string` to the `Analyzer` interface. The interface becomes:
- `Name() string`
- `Languages() []string`
- `AnalyzeFile(ctx context.Context, file repo.FileInfo, content []byte) (*ctxpkg.FileContext, error)`

### 2. Breaking Change Audit

Before compiling, grep for all types implementing the `Analyzer` interface across the codebase:
- `goAnalyzer` in `go_analyzer.go` — add `Name() string` returning `"go"`
- `genericAnalyzer` in `generic_analyzer.go` — add `Name() string` returning `"generic"`
- Any test mocks — add `Name() string` method

### 3. Public Constructors

**File: `internal/analyzer/go_analyzer.go`**
- Rename `newGoAnalyzer()` to `NewGoAnalyzer() Analyzer` (public)
- Add method: `func (a *goAnalyzer) Name() string { return "go" }`

**File: `internal/analyzer/generic_analyzer.go`**
- Rename `newGenericAnalyzer()` to `NewGenericAnalyzer() Analyzer` (public)
- Add method: `func (a *genericAnalyzer) Name() string { return "generic" }`

### 4. Registry Constructors

**File: `internal/analyzer/registry.go`**

Replace current `NewRegistry()`:

**`NewRegistry(analyzers ...Analyzer) Registry`** — Creates empty registry with generic fallback. Iterates provided analyzers and registers each for its Languages(). Does NOT auto-register Go analyzer. Duplicate languages: last wins.

**`DefaultRegistry() Registry`** — Returns `NewRegistry(NewGoAnalyzer())`. This is the canonical way to get a registry with built-in analyzers. Equivalent to current behavior.

The `Get(lang string) Analyzer` method is unchanged — returns the registered analyzer or generic fallback.

### 5. Update Internal Callers

All places that call `NewRegistry()` (no args) and expect Go analyzer must switch to `DefaultRegistry()`:
- `internal/orchestrator/manager.go` — will be updated in Section 4

## Error Handling

- No analyzers provided: empty registry, generic fallback for all languages
- Nil analyzer in variadic list: skip (or panic with descriptive message)

## File Summary

| File | Action |
|------|--------|
| `internal/analyzer/interface.go` | Modify: add Name() to Analyzer interface |
| `internal/analyzer/go_analyzer.go` | Modify: public constructor, add Name() |
| `internal/analyzer/generic_analyzer.go` | Modify: public constructor, add Name() |
| `internal/analyzer/registry.go` | Modify: NewRegistry(analyzers...), DefaultRegistry() |
| `internal/analyzer/registry_test.go` | Modify: add new tests, update existing |
| Test mocks (various) | Modify: add Name() method |
