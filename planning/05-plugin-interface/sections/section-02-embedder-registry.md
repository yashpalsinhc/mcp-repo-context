# Section 2: Embedder Registry

## Overview

Add `Name() string` to the Embedder interface, create an EmbedderRegistry with dimension validation at construction time, and provide DefaultEmbedderRegistry convenience constructor.

## Dependencies

None — parallel with Section 1.

## Tests First

### File: `internal/vectors/registry_test.go` (new)

```
Test: LocalEmbedder Name returns "local"
- e := NewDefaultEmbedder()
- Assert e.Name() == "local"

Test: NewEmbedderRegistry with default embedder
- reg, err := NewEmbedderRegistry(NewDefaultEmbedder())
- Assert err == nil
- Assert reg.Default().Name() == "local"

Test: NewEmbedderRegistry Get by name
- mock := mockEmbedder{name: "custom", dim: 256}
- reg, err := NewEmbedderRegistry(NewDefaultEmbedder(), &mock)
- Assert err == nil
- Assert reg.Get("custom").Name() == "custom"
- Assert reg.Get("unknown") == nil

Test: NewEmbedderRegistry Names lists all
- reg, _ := NewEmbedderRegistry(emb1, emb2, emb3)
- names := reg.Names()
- Assert len(names) == 3
- Assert contains "emb1", "emb2", "emb3"

Test: NewEmbedderRegistry nil default returns error
- _, err := NewEmbedderRegistry(nil)
- Assert err != nil
- Assert err contains "default embedder required"

Test: Dimension mismatch returns error
- emb256 := mockEmbedder{name: "a", dim: 256}
- emb384 := mockEmbedder{name: "b", dim: 384}
- _, err := NewEmbedderRegistry(&emb256, &emb384)
- Assert err != nil
- Assert err contains "dimension mismatch"

Test: Same dimension accepted
- emb1 := mockEmbedder{name: "a", dim: 256}
- emb2 := mockEmbedder{name: "b", dim: 256}
- reg, err := NewEmbedderRegistry(&emb1, &emb2)
- Assert err == nil

Test: DefaultEmbedderRegistry returns working registry
- reg := DefaultEmbedderRegistry()
- Assert reg.Default().Name() == "local"
- Assert reg.Default().Dimension() == 256

Test: Duplicate name last wins
- emb1 := mockEmbedder{name: "test", dim: 256}
- emb2 := mockEmbedder{name: "test", dim: 256}
- reg, _ := NewEmbedderRegistry(defaultEmb, &emb1, &emb2)
- Assert reg.Get("test") is emb2 (or at least returns non-nil)
```

## Implementation Details

### 1. Add Name() to Embedder Interface

**File: `internal/vectors/embedder.go`**

Add `Name() string` to the `Embedder` interface. Interface becomes:
- `Name() string`
- `Embed(text string) []float64`
- `EmbedBatch(texts []string) [][]float64`
- `Dimension() int`

### 2. Update LocalEmbedder

Add method: `func (e *LocalEmbedder) Name() string { return "local" }`

### 3. Breaking Change Audit

Grep for all Embedder implementations in test files. Update each mock to add `Name() string`.

### 4. EmbedderRegistry Interface

**New file: `internal/vectors/registry.go`**

```
type EmbedderRegistry interface {
    Get(name string) Embedder       // Returns nil if not found
    Default() Embedder              // Returns the default embedder
    Names() []string                // Lists all registered embedder names
}
```

### 5. Registry Implementation

Private struct:

```
type embedderRegistry struct {
    embedders  map[string]Embedder
    defaultEmb Embedder
    dimension  int
}
```

**Constructor:**

```
func NewEmbedderRegistry(defaultEmbedder Embedder, extras ...Embedder) (EmbedderRegistry, error)
```

Steps:
1. If defaultEmbedder is nil, return error "default embedder required"
2. Set dimension from defaultEmbedder.Dimension()
3. Register defaultEmbedder by its Name()
4. For each extra embedder:
   a. Check Dimension() matches — if not, return error "dimension mismatch: embedder '{name}' has dimension {d1}, expected {d2}"
   b. Register by Name() — duplicate names: last wins
5. Return the registry

**Get(name):** Map lookup. Returns nil if not found.
**Default():** Returns defaultEmb.
**Names():** Return sorted slice of map keys (deterministic iteration).

### 6. Convenience Constructor

```
func DefaultEmbedderRegistry() EmbedderRegistry
```

Calls `NewEmbedderRegistry(NewDefaultEmbedder())`. Panics on error (should never fail with single default embedder).

### 7. Mock Embedder for Tests

Create a test helper `mockEmbedder` struct with configurable name, dimension, and simple Embed/EmbedBatch that returns zero vectors of the right dimension.

## Error Handling

- Nil default: return descriptive error
- Dimension mismatch: return error with both dimension values and embedder name
- DefaultEmbedderRegistry panic: should never happen, but panic message includes context

## File Summary

| File | Action |
|------|--------|
| `internal/vectors/embedder.go` | Modify: add Name() to Embedder interface, add to LocalEmbedder |
| `internal/vectors/registry.go` | New: EmbedderRegistry interface and implementation |
| `internal/vectors/registry_test.go` | New: registry tests with mock embedder |
| Test mocks (various) | Modify: add Name() method |
