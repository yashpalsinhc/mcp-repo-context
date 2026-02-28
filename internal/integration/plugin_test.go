package integration

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	_ "github.com/mattn/go-sqlite3"
	"github.com/yashpalc/mcp-repo-context/internal/analyzer"
	ctxpkg "github.com/yashpalc/mcp-repo-context/internal/context"
	"github.com/yashpalc/mcp-repo-context/internal/org"
	"github.com/yashpalc/mcp-repo-context/internal/orchestrator"
	"github.com/yashpalc/mcp-repo-context/internal/repo"
	"github.com/yashpalc/mcp-repo-context/internal/storage"
	"github.com/yashpalc/mcp-repo-context/internal/vectors"
)

// --- Mock Python Analyzer ---

type mockPythonAnalyzer struct{}

func (a *mockPythonAnalyzer) Name() string        { return "python" }
func (a *mockPythonAnalyzer) Languages() []string  { return []string{"py", "python"} }
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

// --- Mock Embedder ---

type testEmbedder struct {
	name string
	dim  int
}

func (e *testEmbedder) Name() string      { return e.name }
func (e *testEmbedder) Dimension() int     { return e.dim }
func (e *testEmbedder) Embed(text string) []float64 {
	vec := make([]float64, e.dim)
	for i, c := range text {
		vec[i%e.dim] += float64(c) / 1000.0
	}
	return vec
}
func (e *testEmbedder) EmbedBatch(texts []string) [][]float64 {
	result := make([][]float64, len(texts))
	for i, t := range texts {
		result[i] = e.Embed(t)
	}
	return result
}

// --- Test Helpers ---

func setupPluginTestStore(t *testing.T) (*storage.SQLiteStore, func()) {
	t.Helper()
	tmpDB, err := os.CreateTemp("", "plugin_test_*.db")
	if err != nil {
		t.Fatalf("create temp db: %v", err)
	}
	tmpDB.Close()

	store, err := storage.NewSQLiteStore(tmpDB.Name())
	if err != nil {
		os.Remove(tmpDB.Name())
		t.Fatalf("create store: %v", err)
	}

	return store, func() {
		os.Remove(tmpDB.Name())
	}
}

func createTestDir(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	return dir
}

// --- Tests ---

func TestCustomAnalyzerRegistration(t *testing.T) {
	store, cleanup := setupPluginTestStore(t)
	defer cleanup()

	// Create registry with Go + Python analyzers
	pyAnalyzer := &mockPythonAnalyzer{}
	reg := analyzer.NewRegistry(analyzer.NewGoAnalyzer(), pyAnalyzer)

	// Create temp dir with Go and Python files
	dir := createTestDir(t, map[string]string{
		"main.go": `package main

func HandleRequest() {}
`,
		"script.py": `def process_data(input_str):
    return input_str.upper()
`,
	})

	// Create manager with custom registry
	cloner, _ := repo.NewCloner(t.TempDir())
	scanner := repo.NewScanner(nil, 1024*1024)
	mgr := orchestrator.NewManager(store, cloner, scanner,
		orchestrator.WithAnalyzerRegistry(reg),
	)

	// Analyze
	result, err := mgr.AnalyzeLocal(context.Background(), dir, orchestrator.AnalyzeLocalOptions{Force: true})
	if err != nil {
		t.Fatalf("AnalyzeLocal: %v", err)
	}

	// Verify
	repoCtx, err := mgr.GetContext(context.Background(), result.ProjectID)
	if err != nil {
		t.Fatalf("GetContext: %v", err)
	}

	// Check Go file was analyzed by Go analyzer (should have real functions)
	goFile, ok := repoCtx.Files["main.go"]
	if !ok {
		t.Fatal("main.go not found in context")
	}
	if goFile.Language != "go" {
		t.Errorf("main.go language = %q, want 'go'", goFile.Language)
	}
	foundGoFunc := false
	for _, fn := range goFile.Functions {
		if fn.Name == "HandleRequest" {
			foundGoFunc = true
			break
		}
	}
	if !foundGoFunc {
		t.Error("expected HandleRequest function from Go analyzer")
	}

	// Check Python file was analyzed by mock Python analyzer
	pyFile, ok := repoCtx.Files["script.py"]
	if !ok {
		t.Fatal("script.py not found in context")
	}
	if pyFile.Language != "python" {
		t.Errorf("script.py language = %q, want 'python'", pyFile.Language)
	}
	foundMockFunc := false
	for _, fn := range pyFile.Functions {
		if fn.Name == "mock_function" {
			foundMockFunc = true
			break
		}
	}
	if !foundMockFunc {
		t.Error("expected mock_function from mock Python analyzer")
	}
}

func TestDefaultRegistryBackwardCompat(t *testing.T) {
	store, cleanup := setupPluginTestStore(t)
	defer cleanup()

	dir := createTestDir(t, map[string]string{
		"main.go": `package main

func Hello() string { return "world" }
`,
	})

	// Create manager with NO options (uses defaults)
	cloner, _ := repo.NewCloner(t.TempDir())
	scanner := repo.NewScanner(nil, 1024*1024)
	mgr := orchestrator.NewManager(store, cloner, scanner)

	result, err := mgr.AnalyzeLocal(context.Background(), dir, orchestrator.AnalyzeLocalOptions{Force: true})
	if err != nil {
		t.Fatalf("AnalyzeLocal: %v", err)
	}

	repoCtx, err := mgr.GetContext(context.Background(), result.ProjectID)
	if err != nil {
		t.Fatalf("GetContext: %v", err)
	}

	goFile, ok := repoCtx.Files["main.go"]
	if !ok {
		t.Fatal("main.go not found")
	}

	// Go analyzer should have extracted the function
	foundFunc := false
	for _, fn := range goFile.Functions {
		if fn.Name == "Hello" {
			foundFunc = true
			break
		}
	}
	if !foundFunc {
		t.Error("expected Hello function from default Go analyzer")
	}
}

func TestEmbedderRegistryDimensionValidation(t *testing.T) {
	emb256 := &testEmbedder{name: "a", dim: 256}
	emb384 := &testEmbedder{name: "b", dim: 384}

	_, err := vectors.NewEmbedderRegistry(emb256, emb384)
	if err == nil {
		t.Fatal("expected error for dimension mismatch")
	}
}

func TestEmbedderRegistryRoundTrip(t *testing.T) {
	reg := vectors.DefaultEmbedderRegistry()

	defaultEmb := reg.Default()
	if defaultEmb == nil {
		t.Fatal("expected non-nil default embedder")
	}

	// Verify it can generate embeddings
	vec := defaultEmb.Embed("test query")
	if len(vec) != defaultEmb.Dimension() {
		t.Errorf("embedding dimension = %d, want %d", len(vec), defaultEmb.Dimension())
	}
}

func TestOrgConfigMergePrecedence(t *testing.T) {
	orgCfg := &org.OrgConfig{AnalyzerName: "org-level", EmbedderName: "org-emb"}
	repoCfg := &org.OrgConfig{AnalyzerName: "repo-level"}

	merged := org.MergeConfigs(orgCfg, repoCfg)
	if merged.AnalyzerName != "repo-level" {
		t.Errorf("AnalyzerName = %q, want 'repo-level'", merged.AnalyzerName)
	}
	// EmbedderName should come from org since repo doesn't override
	if merged.EmbedderName != "org-emb" {
		t.Errorf("EmbedderName = %q, want 'org-emb'", merged.EmbedderName)
	}
}

func TestNoOrgDefaultBehavior(t *testing.T) {
	store, cleanup := setupPluginTestStore(t)
	defer cleanup()

	dir := createTestDir(t, map[string]string{
		"main.go": `package main

func Run() {}
`,
	})

	cloner, _ := repo.NewCloner(t.TempDir())
	scanner := repo.NewScanner(nil, 1024*1024)
	mgr := orchestrator.NewManager(store, cloner, scanner)

	result, err := mgr.AnalyzeLocal(context.Background(), dir, orchestrator.AnalyzeLocalOptions{Force: true})
	if err != nil {
		t.Fatalf("AnalyzeLocal: %v", err)
	}

	// Should succeed with defaults, no warnings about missing plugins
	if result.FileCount == 0 {
		t.Error("expected at least 1 file analyzed")
	}
}

func TestRegistryGetByName(t *testing.T) {
	goAnalyzer := analyzer.NewGoAnalyzer()
	reg := analyzer.NewRegistry(goAnalyzer)

	// Should find Go by name
	found := reg.GetByName("go")
	if found == nil {
		t.Fatal("expected to find Go analyzer by name")
	}
	if found.Name() != "go" {
		t.Errorf("found.Name() = %q, want 'go'", found.Name())
	}

	// Should find generic fallback by name
	generic := reg.GetByName("generic")
	if generic == nil {
		t.Fatal("expected to find generic analyzer by name")
	}

	// Unknown should return nil
	unknown := reg.GetByName("nonexistent")
	if unknown != nil {
		t.Errorf("expected nil for unknown analyzer name, got %v", unknown)
	}
}

func TestRegistryNames(t *testing.T) {
	pyAnalyzer := &mockPythonAnalyzer{}
	reg := analyzer.NewRegistry(analyzer.NewGoAnalyzer(), pyAnalyzer)

	names := reg.Names()
	if len(names) < 2 {
		t.Fatalf("expected at least 2 names, got %v", names)
	}

	// Should include go, python, and generic (fallback)
	nameSet := make(map[string]bool)
	for _, n := range names {
		nameSet[n] = true
	}
	if !nameSet["go"] {
		t.Error("expected 'go' in names")
	}
	if !nameSet["python"] {
		t.Error("expected 'python' in names")
	}
	if !nameSet["generic"] {
		t.Error("expected 'generic' in names")
	}
}

func TestEmbedderRegistryNames(t *testing.T) {
	extra := &testEmbedder{name: "extra", dim: 256}
	reg, err := vectors.NewEmbedderRegistry(vectors.NewDefaultEmbedder(), extra)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	names := reg.Names()
	if len(names) != 2 {
		t.Fatalf("expected 2 names, got %v", names)
	}
}
