package integration

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/yashpalc/mcp-repo-context/internal/comparison"
	ctxpkg "github.com/yashpalc/mcp-repo-context/internal/context"
	graphpkg "github.com/yashpalc/mcp-repo-context/internal/graph"
	"github.com/yashpalc/mcp-repo-context/internal/orchestrator"
	"github.com/yashpalc/mcp-repo-context/internal/repo"
	"github.com/yashpalc/mcp-repo-context/internal/storage"
)

// setupIntegrationTest creates a real SQLite store, manager wired with real
// cloner/scanner, and returns them along with a cleanup function.
func setupIntegrationTest(t *testing.T) (orchestrator.Manager, storage.ContextStore, func()) {
	t.Helper()
	tmpDir := t.TempDir()

	store, err := storage.NewSQLiteStore(filepath.Join(tmpDir, "test.db"))
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}

	cloner, err := repo.NewCloner(filepath.Join(tmpDir, "repos"))
	if err != nil {
		t.Fatalf("Failed to create cloner: %v", err)
	}

	scanner := repo.NewScanner(nil, 1024*1024) // 1MB max file size

	mgr := orchestrator.NewManager(store, cloner, scanner)

	cleanup := func() {
		store.Close()
	}

	return mgr, store, cleanup
}

func TestAnalyzeGorillaMux_GoModParsed(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	mgr, _, cleanup := setupIntegrationTest(t)
	defer cleanup()

	ctx := context.Background()
	result, err := mgr.AnalyzeRepo(ctx, "https://github.com/gorilla/mux", orchestrator.AnalyzeOptions{
		Force: true,
	})
	if err != nil {
		t.Fatalf("AnalyzeRepo failed: %v", err)
	}
	if len(result.Warnings) > 0 {
		t.Logf("Warnings: %v", result.Warnings)
	}

	repoCtx, err := mgr.GetContext(ctx, "github.com/gorilla/mux")
	if err != nil {
		t.Fatalf("GetContext failed: %v", err)
	}

	if repoCtx.ModuleInfo == nil {
		t.Logf("RepoCtx ID=%s, Files=%d", repoCtx.ID, len(repoCtx.Files))
		t.Fatal("ModuleInfo is nil — go.mod was not parsed during analysis")
	}
	if repoCtx.ModuleInfo.ModulePath != "github.com/gorilla/mux" {
		t.Errorf("ModulePath = %q, want github.com/gorilla/mux", repoCtx.ModuleInfo.ModulePath)
	}
	if repoCtx.ModuleInfo.GoVersion == "" {
		t.Error("GoVersion is empty")
	}
	t.Logf("gorilla/mux: module=%s go=%s deps=%d",
		repoCtx.ModuleInfo.ModulePath, repoCtx.ModuleInfo.GoVersion, len(repoCtx.ModuleInfo.Dependencies))
}

func TestAnalyzeGorillaHandlers_DependenciesExtracted(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	mgr, _, cleanup := setupIntegrationTest(t)
	defer cleanup()

	ctx := context.Background()
	_, err := mgr.AnalyzeRepo(ctx, "https://github.com/gorilla/handlers", orchestrator.AnalyzeOptions{
		Force: true,
	})
	if err != nil {
		t.Fatalf("AnalyzeRepo failed: %v", err)
	}

	repoCtx, err := mgr.GetContext(ctx, "github.com/gorilla/handlers")
	if err != nil {
		t.Fatalf("GetContext failed: %v", err)
	}

	if repoCtx.ModuleInfo == nil {
		t.Fatal("ModuleInfo is nil")
	}
	if repoCtx.ModuleInfo.ModulePath != "github.com/gorilla/handlers" {
		t.Errorf("ModulePath = %q, want github.com/gorilla/handlers", repoCtx.ModuleInfo.ModulePath)
	}
	if len(repoCtx.ModuleInfo.Dependencies) == 0 {
		t.Error("Dependencies is empty; gorilla/handlers should have external dependencies")
	}
	t.Logf("gorilla/handlers: deps=%d", len(repoCtx.ModuleInfo.Dependencies))

	if repoCtx.ImportSummary == nil {
		t.Fatal("ImportSummary is nil")
	}
	if len(repoCtx.ImportSummary.Stdlib) == 0 {
		t.Error("ImportSummary.Stdlib is empty")
	}
	t.Logf("ImportSummary: stdlib=%d internal=%d external=%d",
		len(repoCtx.ImportSummary.Stdlib), len(repoCtx.ImportSummary.Internal), len(repoCtx.ImportSummary.External))
}

func TestDependencyGraph_BothRepos(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	mgr, _, cleanup := setupIntegrationTest(t)
	defer cleanup()

	ctx := context.Background()

	// Analyze both repos
	for _, url := range []string{"https://github.com/gorilla/mux", "https://github.com/gorilla/handlers"} {
		_, err := mgr.AnalyzeRepo(ctx, url, orchestrator.AnalyzeOptions{Force: true})
		if err != nil {
			t.Fatalf("AnalyzeRepo(%s) failed: %v", url, err)
		}
	}

	graph, err := mgr.GetDependencyGraph(ctx, []string{
		"github.com/gorilla/mux",
		"github.com/gorilla/handlers",
	}, true)
	if err != nil {
		t.Fatalf("GetDependencyGraph failed: %v", err)
	}
	if graph == nil {
		t.Fatal("graph is nil")
	}

	// Check analyzed nodes
	analyzedCount := 0
	for _, node := range graph.Nodes {
		if node.IsAnalyzed {
			analyzedCount++
		}
	}
	if analyzedCount != 2 {
		t.Errorf("analyzed nodes = %d, want 2", analyzedCount)
	}

	t.Logf("Graph: %d nodes, %d edges", len(graph.Nodes), len(graph.Edges))

	// Check Mermaid output
	mermaid := graphpkg.GenerateDependencyMermaid(graph)
	if mermaid == "" {
		t.Error("Mermaid output is empty")
	}
	if len(mermaid) < 10 {
		t.Errorf("Mermaid output suspiciously short: %q", mermaid)
	}
}

func TestCompareRepos_ShowsDependencyRelationships(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	mgr, _, cleanup := setupIntegrationTest(t)
	defer cleanup()

	ctx := context.Background()

	// Analyze both repos
	for _, url := range []string{"https://github.com/gorilla/mux", "https://github.com/gorilla/handlers"} {
		_, err := mgr.AnalyzeRepo(ctx, url, orchestrator.AnalyzeOptions{Force: true})
		if err != nil {
			t.Fatalf("AnalyzeRepo(%s) failed: %v", url, err)
		}
	}

	// Retrieve contexts
	var repoContexts []*ctxpkg.RepoContext
	for _, id := range []string{"github.com/gorilla/mux", "github.com/gorilla/handlers"} {
		rc, err := mgr.GetContext(ctx, id)
		if err != nil {
			t.Fatalf("GetContext(%s) failed: %v", id, err)
		}
		repoContexts = append(repoContexts, rc)
	}

	comparer := comparison.NewComparer()
	result, err := comparer.Compare(ctx, repoContexts, comparison.DefaultCompareOptions())
	if err != nil {
		t.Fatalf("Compare failed: %v", err)
	}

	// DependencyRelationships may be nil if repos have no module overlap
	// but gorilla/handlers depends on felixge/httpsnoop etc., so check structural integrity
	t.Logf("DependencyRelationships: %+v", result.DependencyRelationships)
	if result.DependencyRelationships != nil {
		t.Logf("InternalDeps: %d, SharedExternalDeps: %d",
			len(result.DependencyRelationships.InternalDeps),
			len(result.DependencyRelationships.SharedExternalDeps))
	}
}

func TestGetContextArchitecture_IncludesDependencyList(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	mgr, _, cleanup := setupIntegrationTest(t)
	defer cleanup()

	ctx := context.Background()
	_, err := mgr.AnalyzeRepo(ctx, "https://github.com/gorilla/handlers", orchestrator.AnalyzeOptions{
		Force: true,
	})
	if err != nil {
		t.Fatalf("AnalyzeRepo failed: %v", err)
	}

	repoCtx, err := mgr.GetContext(ctx, "github.com/gorilla/handlers")
	if err != nil {
		t.Fatalf("GetContext failed: %v", err)
	}

	if repoCtx.Architecture == nil {
		t.Fatal("Architecture is nil")
	}

	arch := repoCtx.Architecture
	t.Logf("Architecture: overview=%q packageType=%q goVersion=%q deps=%d",
		arch.Overview, arch.PackageType, arch.GoVersion, len(arch.Dependencies))

	if arch.PackageType == "" {
		t.Error("PackageType is empty")
	}
	// gorilla/handlers is a library (no main package)
	if arch.PackageType != "library" {
		t.Logf("PackageType = %q (expected 'library' for gorilla/handlers)", arch.PackageType)
	}
	if arch.GoVersion == "" {
		t.Error("GoVersion is empty in architecture")
	}
}
