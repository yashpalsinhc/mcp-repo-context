# Section 6: Integration Tests

## Overview

End-to-end integration tests verifying the complete plugin architecture: custom analyzer registration, embedder registry, OrgConfig-driven selection, and backward compatibility.

## Dependencies

- All previous sections (1-5)

## Tests First

### File: `internal/integration/plugin_test.go` (new)

```
Test: Custom analyzer registration and file dispatch
- Create mock Python analyzer (Name="python", Languages=["py"])
- Build registry: NewRegistry(NewGoAnalyzer(), pythonAnalyzer)
- Create manager with custom registry
- Create temp dir with main.go and script.py
- Analyze local directory
- Assert .go files have Go analyzer output (functions with behavior)
- Assert .py files have Python analyzer output (mock output)
- Assert unknown extensions get generic fallback

Test: DefaultRegistry backward compatibility
- Create manager with no options (uses DefaultRegistry)
- Analyze temp dir with .go files
- Assert Go analyzer used (functions extracted, not just metadata)
- Assert results match current behavior

Test: Embedder registry round-trip
- Create embedder registry with default LocalEmbedder
- Create server with registry
- Analyze and index a Go repo
- Search semantically
- Assert results returned (embedder working)

Test: Embedder dimension validation at construction
- Create two mock embedders: dim 256, dim 384
- Attempt NewEmbedderRegistry(emb256, emb384)
- Assert error returned with dimension info

Test: OrgConfig with AnalyzerName through analysis
- Register org with config.AnalyzerName="go"
- Add repo to org
- Trigger analyze_local for repo
- Assert Go analyzer used (name resolved from org config)

Test: OrgConfig unknown AnalyzerName warning
- Register org with config.AnalyzerName="ruby"
- Trigger analysis
- Assert analysis completes (default used)
- Assert output contains warning "ruby" not found

Test: OrgConfig EmbedderName selection
- Register org with config.EmbedderName="local"
- Trigger indexing
- Assert indexing succeeds

Test: OrgConfig merge precedence
- Org config: AnalyzerName="org-level"
- Repo override: AnalyzerName="repo-level"
- Resolve config for repo
- Assert "repo-level" is used

Test: No org — default behavior
- Create server with registries
- Analyze repo NOT in any org
- Assert defaults used, no warnings

Test: Full pipeline — analyze with custom plugin, index, search
- Register custom analyzer
- Create registry with it
- Create full server pipeline (manager + server)
- Analyze temp dir
- Index
- Search
- Assert end-to-end works
```

## Implementation Details

### 1. Test Fixture: Mock Python Analyzer

```go
type mockPythonAnalyzer struct{}

func (a *mockPythonAnalyzer) Name() string { return "python" }
func (a *mockPythonAnalyzer) Languages() []string { return []string{"py", "python"} }
func (a *mockPythonAnalyzer) AnalyzeFile(ctx context.Context, file repo.FileInfo, content []byte) (*ctxpkg.FileContext, error) {
    return &ctxpkg.FileContext{
        Path:     file.Path,
        Language: "python",
        Purpose:  "Python source file (mock)",
        Functions: []ctxpkg.FunctionDef{
            {Name: "mock_function", IsPublic: true, Description: "Mock Python function"},
        },
    }, nil
}
```

### 2. Test Fixture: Mock Embedder

```go
type mockEmbedder struct {
    name string
    dim  int
}

func (e *mockEmbedder) Name() string { return e.name }
func (e *mockEmbedder) Dimension() int { return e.dim }
func (e *mockEmbedder) Embed(text string) []float64 {
    vec := make([]float64, e.dim)
    // Simple hash-based embedding for testing
    for i, c := range text {
        vec[i%e.dim] += float64(c) / 1000.0
    }
    return vec
}
func (e *mockEmbedder) EmbedBatch(texts []string) [][]float64 {
    result := make([][]float64, len(texts))
    for i, t := range texts {
        result[i] = e.Embed(t)
    }
    return result
}
```

### 3. Test Setup Helper

```go
func setupPluginTestServer(t *testing.T, opts ...setupOption) (*server, string)
```

Options:
- `withCustomAnalyzer(a analyzer.Analyzer)` — add to registry
- `withCustomEmbedder(e vectors.Embedder)` — add to registry
- `withOrg(orgID string, config org.OrgConfig)` — register org

Returns server and temp directory.

### 4. Source Fixtures

**Go file (`main.go`):**
```go
package main

func HandleRequest(w http.ResponseWriter, r *http.Request) {}
func processData(input string) string { return input }
```

**Python file (`script.py`):**
```python
def process_data(input_str):
    return input_str.upper()
```

### 5. Test Isolation

- Each test uses isolated temp directories
- Each test creates its own server/manager/registries
- Tests can run in parallel (separate resources)
- Cleanup via `t.TempDir()` and `t.Cleanup()`

## Error Handling

- Fixture setup failures: `t.Fatal` with descriptive message
- Mock analyzer errors: fail the test, not silently degrade

## File Summary

| File | Action |
|------|--------|
| `internal/integration/plugin_test.go` | New: end-to-end integration tests |
| `internal/integration/plugin_fixtures_test.go` | New: mock analyzers, embedders, setup helpers |
