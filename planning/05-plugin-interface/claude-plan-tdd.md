# TDD Plan: Plugin Interface

## Section 1: Extend Analyzer Interface & Registry

### Tests: `internal/analyzer/registry_test.go` (extend existing)

```
Test: NewGoAnalyzer returns analyzer with Name "go"
- a := NewGoAnalyzer()
- Assert a.Name() == "go"

Test: NewGenericAnalyzer returns analyzer with Name "generic"
- a := NewGenericAnalyzer()
- Assert a.Name() == "generic"

Test: NewRegistry with no args returns empty registry
- r := NewRegistry()
- Assert r.Get("go") returns generic fallback (not Go analyzer)

Test: DefaultRegistry returns Go analyzer for "go"
- r := DefaultRegistry()
- Assert r.Get("go").Name() == "go"

Test: DefaultRegistry falls back to generic for unknown
- r := DefaultRegistry()
- Assert r.Get("python").Name() == "generic"

Test: NewRegistry with custom analyzer
- Create mock analyzer with Name="python", Languages=["py","python"]
- r := NewRegistry(mockAnalyzer)
- Assert r.Get("py").Name() == "python"
- Assert r.Get("python").Name() == "python"
- Assert r.Get("go").Name() == "generic" (no Go registered)

Test: Duplicate language last wins
- Create analyzer1 with Languages=["go"]
- Create analyzer2 with Languages=["go"]
- r := NewRegistry(analyzer1, analyzer2)
- Assert r.Get("go") is analyzer2

Test: Multiple analyzers registered
- r := NewRegistry(NewGoAnalyzer(), mockPythonAnalyzer)
- Assert r.Get("go").Name() == "go"
- Assert r.Get("py").Name() == "python"
- Assert r.Get("rust").Name() == "generic"
```

## Section 2: Embedder Registry

### Tests: `internal/vectors/registry_test.go` (new)

```
Test: NewEmbedderRegistry with default embedder
- reg, err := NewEmbedderRegistry(NewDefaultEmbedder())
- Assert err == nil
- Assert reg.Default().Name() == "local"

Test: NewEmbedderRegistry Get by name
- reg, err := NewEmbedderRegistry(defaultEmb, extraEmb)
- Assert reg.Get("extra").Name() == "extra"
- Assert reg.Get("unknown") == nil or returns default

Test: NewEmbedderRegistry Names lists all
- reg, _ := NewEmbedderRegistry(emb1, emb2, emb3)
- names := reg.Names()
- Assert contains "emb1", "emb2", "emb3"

Test: NewEmbedderRegistry nil default returns error
- _, err := NewEmbedderRegistry(nil)
- Assert err != nil

Test: Dimension mismatch returns error
- emb256 with Dimension()=256
- emb384 with Dimension()=384
- _, err := NewEmbedderRegistry(emb256, emb384)
- Assert err != nil
- Assert err message contains "dimension"

Test: Same dimension accepted
- emb1 with Dimension()=256
- emb2 with Dimension()=256
- reg, err := NewEmbedderRegistry(emb1, emb2)
- Assert err == nil

Test: DefaultEmbedderRegistry returns local embedder
- reg := DefaultEmbedderRegistry()
- Assert reg.Default().Name() == "local"

Test: LocalEmbedder Name returns "local"
- e := NewDefaultEmbedder()
- Assert e.Name() == "local"

Test: Duplicate name last wins
- emb1 with Name="test"
- emb2 with Name="test"
- reg, _ := NewEmbedderRegistry(defaultEmb, emb1, emb2)
- Assert reg.Get("test") is emb2
```

## Section 3: OrgConfig Extension

### Tests: `internal/org/config_test.go` (extend existing)

```
Test: OrgConfig AnalyzerName JSON round-trip
- config := OrgConfig{AnalyzerName: "python", EmbedderName: "voyage"}
- Marshal to JSON, unmarshal back
- Assert fields preserved

Test: MergeConfigs preserves analyzer name from org
- orgConfig := OrgConfig{AnalyzerName: "python"}
- repoOverride := OrgConfig{}
- merged := MergeConfigs(orgConfig, repoOverride)
- Assert merged.AnalyzerName == "python"

Test: MergeConfigs repo override takes precedence
- orgConfig := OrgConfig{AnalyzerName: "python"}
- repoOverride := OrgConfig{AnalyzerName: "typescript"}
- merged := MergeConfigs(orgConfig, repoOverride)
- Assert merged.AnalyzerName == "typescript"

Test: copyConfig copies new fields
- src := OrgConfig{AnalyzerName: "go", EmbedderName: "local", MaxFileSize: 1000}
- dst := copyConfig(src)
- Assert dst.AnalyzerName == "go"
- Assert dst.EmbedderName == "local"
- Assert dst.MaxFileSize == 1000

Test: MergeConfigs nil orgConfig
- merged := MergeConfigs(nil, &OrgConfig{AnalyzerName: "python"})
- Assert merged.AnalyzerName == "python"

Test: MergeConfigs nil repoOverride
- merged := MergeConfigs(&OrgConfig{EmbedderName: "local"}, nil)
- Assert merged.EmbedderName == "local"
```

## Section 4: Manager & Server Wiring

### Tests: `internal/orchestrator/manager_test.go` (extend existing)

```
Test: NewManager with default options
- m := NewManager(store, cloner, scanner)
- Assert m has analyzer registry (DefaultRegistry)
- Assert m has AI registry

Test: NewManager WithAnalyzerRegistry
- customReg := analyzer.NewRegistry(mockAnalyzer)
- m := NewManager(store, cloner, scanner, WithAnalyzerRegistry(customReg))
- Analyze a file that mockAnalyzer handles
- Assert mockAnalyzer was used

Test: NewManager WithAIRegistry
- aiReg := ai.NewRegistry()
- m := NewManager(store, cloner, scanner, WithAIRegistry(aiReg))
- Assert m has the provided AI registry

Test: AnalyzeOptions with AnalyzerName
- Register custom analyzer with Name="custom"
- Call AnalyzeRepo with options.AnalyzerName="custom"
- Assert custom analyzer was used for file analysis
```

### Tests: `internal/mcp/server_test.go` (extend existing)

```
Test: NewServer with EmbedderRegistry in config
- reg := vectors.DefaultEmbedderRegistry()
- config := ServerConfig{EmbedderRegistry: reg, VectorStore: vs}
- s := NewServer(manager, comparer, config)
- Assert semantic search uses registry's default embedder

Test: NewServer without EmbedderRegistry (backward compat)
- config := ServerConfig{VectorStore: vs}
- s := NewServer(manager, comparer, config)
- Assert semantic search still works (falls back to NewDefaultEmbedder)
```

## Section 5: Per-Org Plugin Selection

### Tests: `internal/mcp/org_plugin_test.go` (new)

```
Test: Org with AnalyzerName passes to AnalyzeOptions
- Register org with config.AnalyzerName="custom"
- Trigger analysis for repo in org
- Assert AnalyzeOptions.AnalyzerName == "custom"

Test: Unknown AnalyzerName falls back with warning
- Register org with config.AnalyzerName="nonexistent"
- Trigger analysis
- Assert analysis succeeds (uses default)
- Assert output contains warning "not found"

Test: Org with no AnalyzerName uses default
- Register org with empty config
- Trigger analysis
- Assert default analyzer used (no warning)

Test: Unknown EmbedderName falls back with warning
- Register org with config.EmbedderName="nonexistent"
- Trigger indexing
- Assert indexing succeeds (uses default)
- Assert output contains warning

Test: Per-org embedder selection
- Register org with config.EmbedderName="local"
- Trigger indexing
- Assert "local" embedder used
```

## Section 6: Integration Tests

### Tests: `internal/integration/plugin_test.go` (new)

```
Test: End-to-end custom analyzer registration and usage
- Create mock Python analyzer
- Build registry with Go + Python analyzers
- Create manager with custom registry
- Analyze local dir with .go and .py files
- Assert .go files analyzed by Go analyzer
- Assert .py files analyzed by Python analyzer
- Assert .rs files analyzed by generic fallback

Test: End-to-end embedder registry
- Create embedder registry with default
- Create server with registry
- Index repository
- Search — assert results use registry embedder

Test: OrgConfig round-trip through analysis
- Register org with AnalyzerName
- Analyze repo in org
- Assert correct analyzer was used

Test: Backward compatibility — no config changes
- Create manager and server with no plugin options
- Assert everything works as before (DefaultRegistry used)
```
