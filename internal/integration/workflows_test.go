package integration

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "github.com/mattn/go-sqlite3"
	"github.com/yashpalc/mcp-repo-context/internal/comparison"
	ctxpkg "github.com/yashpalc/mcp-repo-context/internal/context"
	"github.com/yashpalc/mcp-repo-context/internal/orchestrator"
	"github.com/yashpalc/mcp-repo-context/internal/repo"
	"github.com/yashpalc/mcp-repo-context/internal/storage"
	"github.com/yashpalc/mcp-repo-context/internal/workflows"
)

// --- Workflow Integration Adapters ---

// orchestratorSearcher adapts orchestrator.Manager to workflows.RepoSearcher.
type orchestratorSearcher struct {
	mgr orchestrator.Manager
}

func (s *orchestratorSearcher) GetContext(ctx context.Context, repoID string) (*ctxpkg.RepoContext, error) {
	return s.mgr.GetContext(ctx, repoID)
}

func (s *orchestratorSearcher) SearchFunctions(ctx context.Context, repoID, query string) ([]ctxpkg.FunctionRef, error) {
	return s.mgr.SearchFunctions(ctx, repoID, query)
}

func (s *orchestratorSearcher) SearchByConcept(ctx context.Context, repoID, concept string) ([]ctxpkg.FunctionRef, error) {
	return s.mgr.SearchByConcept(ctx, repoID, concept)
}

func (s *orchestratorSearcher) GetCallers(ctx context.Context, repoID, funcName string) ([]ctxpkg.FunctionRef, error) {
	repoCtx, err := s.mgr.GetContext(ctx, repoID)
	if err != nil {
		return nil, err
	}
	var callers []ctxpkg.FunctionRef
	for filePath, fc := range repoCtx.Files {
		for _, fn := range fc.Functions {
			for _, call := range fn.Calls {
				if call.Function == funcName {
					callers = append(callers, ctxpkg.FunctionRef{
						File:     filePath,
						Function: fn.Name,
						Line:     call.Line,
					})
				}
			}
		}
	}
	return callers, nil
}

// staticOrgResolver implements workflows.OrgResolver with a fixed mapping.
type staticOrgResolver struct {
	orgs map[string][]string
}

func (r *staticOrgResolver) GetOrgRepos(_ context.Context, orgID string) ([]string, error) {
	if repos, ok := r.orgs[orgID]; ok {
		return repos, nil
	}
	return nil, nil
}

// --- Setup Helpers ---

func setupWorkflowStore(t *testing.T) (*storage.SQLiteStore, func()) {
	t.Helper()
	tmpDB, err := os.CreateTemp("", "workflow_integ_*.db")
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

func createWorkflowTestDir(t *testing.T, files map[string]string) string {
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

func analyzeDir(t *testing.T, mgr orchestrator.Manager, dir string) string {
	t.Helper()
	result, err := mgr.AnalyzeLocal(context.Background(), dir, orchestrator.AnalyzeLocalOptions{Force: true})
	if err != nil {
		t.Fatalf("AnalyzeLocal %s: %v", dir, err)
	}
	return result.ProjectID
}

// --- Integration Tests ---

func TestBuildFeatureEndToEnd(t *testing.T) {
	store, cleanup := setupWorkflowStore(t)
	defer cleanup()

	// Create repo with user handlers
	dirA := createWorkflowTestDir(t, map[string]string{
		"user_handlers.go": `package handlers

// GetUser retrieves a user by ID.
func GetUser(id int) error { return nil }

// CreateUser creates a new user record.
func CreateUser(name string) error { return nil }

// UpdateUser updates user details.
func UpdateUser(id int, name string) error { return nil }

// validateUserInput validates user input (private).
func validateUserInput(name string) error { return nil }
`,
	})

	// Create repo with order handlers
	dirB := createWorkflowTestDir(t, map[string]string{
		"order_handlers.go": `package handlers

// ListOrders lists orders for a user.
func ListOrders(userID int) error { return nil }

// CreateOrder creates a new order.
func CreateOrder(items []string) error { return nil }
`,
		"shared.go": `package handlers

// ValidateInput validates any input data.
func ValidateInput(data interface{}) error { return nil }
`,
	})

	cloner, _ := repo.NewCloner(t.TempDir())
	scanner := repo.NewScanner(nil, 1024*1024)
	mgr := orchestrator.NewManager(store, cloner, scanner)

	idA := analyzeDir(t, mgr, dirA)
	idB := analyzeDir(t, mgr, dirB)

	searcher := &orchestratorSearcher{mgr: mgr}
	orgResolver := &staticOrgResolver{orgs: map[string][]string{
		"test-org": {idA, idB},
	}}

	plan, err := workflows.BuildFeature(context.Background(), workflows.BuildFeatureParams{
		OrgID:              "test-org",
		FeatureDescription: "user management",
		Searcher:           searcher,
		OrgResolver:        orgResolver,
	})
	if err != nil {
		t.Fatalf("BuildFeature: %v", err)
	}

	if len(plan.RelevantCode) == 0 {
		t.Fatal("expected non-empty RelevantCode")
	}

	// Format output
	output := workflows.FormatFeaturePlan(plan, 8000)
	assertOutputContainsWorkflow(t, output, "Relevant Code")
	assertOutputContainsWorkflow(t, output, "Risk")
}

func TestBuildFeatureKeywordFallbackIntegration(t *testing.T) {
	store, cleanup := setupWorkflowStore(t)
	defer cleanup()

	dir := createWorkflowTestDir(t, map[string]string{
		"handlers.go": `package handlers

// GetUser retrieves user by ID.
func GetUser(id int) error { return nil }
`,
	})

	cloner, _ := repo.NewCloner(t.TempDir())
	scanner := repo.NewScanner(nil, 1024*1024)
	mgr := orchestrator.NewManager(store, cloner, scanner)

	projectID := analyzeDir(t, mgr, dir)

	searcher := &orchestratorSearcher{mgr: mgr}
	orgResolver := &staticOrgResolver{orgs: map[string][]string{
		"org": {projectID},
	}}

	plan, err := workflows.BuildFeature(context.Background(), workflows.BuildFeatureParams{
		OrgID:              "org",
		FeatureDescription: "user handler",
		Searcher:           searcher,
		OrgResolver:        orgResolver,
		Vectors:            nil, // No vectors — forces keyword fallback
	})
	if err != nil {
		t.Fatalf("BuildFeature: %v", err)
	}

	if len(plan.RelevantCode) == 0 {
		t.Error("expected keyword fallback to find results")
	}
	foundWarning := false
	for _, w := range plan.Warnings {
		if strings.Contains(w, "keyword fallback") {
			foundWarning = true
		}
	}
	if !foundWarning {
		t.Error("expected warning about keyword fallback")
	}
}

func TestBuildFeatureTargetReposIntegration(t *testing.T) {
	store, cleanup := setupWorkflowStore(t)
	defer cleanup()

	dirA := createWorkflowTestDir(t, map[string]string{
		"a.go": `package a
func FuncA() error { return nil }
`,
	})
	dirB := createWorkflowTestDir(t, map[string]string{
		"b.go": `package b
func FuncB() error { return nil }
`,
	})

	cloner, _ := repo.NewCloner(t.TempDir())
	scanner := repo.NewScanner(nil, 1024*1024)
	mgr := orchestrator.NewManager(store, cloner, scanner)

	idA := analyzeDir(t, mgr, dirA)
	idB := analyzeDir(t, mgr, dirB)

	searcher := &orchestratorSearcher{mgr: mgr}
	orgResolver := &staticOrgResolver{orgs: map[string][]string{
		"org": {idA, idB},
	}}

	plan, err := workflows.BuildFeature(context.Background(), workflows.BuildFeatureParams{
		OrgID:              "org",
		FeatureDescription: "func",
		TargetRepos:        []string{idA},
		Searcher:           searcher,
		OrgResolver:        orgResolver,
	})
	if err != nil {
		t.Fatalf("BuildFeature: %v", err)
	}

	for _, loc := range plan.RelevantCode {
		if loc.RepoID != idA {
			t.Errorf("expected only repo %s, got %s", idA, loc.RepoID)
		}
	}
}

func TestBuildFeatureUnknownOrg(t *testing.T) {
	store, cleanup := setupWorkflowStore(t)
	defer cleanup()

	cloner, _ := repo.NewCloner(t.TempDir())
	scanner := repo.NewScanner(nil, 1024*1024)
	mgr := orchestrator.NewManager(store, cloner, scanner)

	searcher := &orchestratorSearcher{mgr: mgr}
	orgResolver := &staticOrgResolver{orgs: map[string][]string{}}

	_, err := workflows.BuildFeature(context.Background(), workflows.BuildFeatureParams{
		OrgID:              "nonexistent",
		FeatureDescription: "test feature",
		Searcher:           searcher,
		OrgResolver:        orgResolver,
	})
	if err == nil {
		t.Fatal("expected error for unknown org")
	}
}

func TestRefactorOrgEndToEnd(t *testing.T) {
	store, cleanup := setupWorkflowStore(t)
	defer cleanup()

	dir := createWorkflowTestDir(t, map[string]string{
		"validate.go": `package handlers

// ValidateInput validates the input data.
func ValidateInput(data string) error { return nil }

// ValidateEmail validates email format.
func ValidateEmail(email string) error { return nil }

// ProcessData processes the data after validation.
func ProcessData(d string) error {
	ValidateInput(d)
	return nil
}
`,
	})

	cloner, _ := repo.NewCloner(t.TempDir())
	scanner := repo.NewScanner(nil, 1024*1024)
	mgr := orchestrator.NewManager(store, cloner, scanner)

	projectID := analyzeDir(t, mgr, dir)

	searcher := &orchestratorSearcher{mgr: mgr}
	orgResolver := &staticOrgResolver{orgs: map[string][]string{
		"org": {projectID},
	}}

	// Use "validate" which matches function names via keyword fallback
	plan, err := workflows.RefactorOrg(context.Background(), workflows.RefactorOrgParams{
		OrgID:              "org",
		PatternDescription: "validate input email",
		Searcher:           searcher,
		OrgResolver:        orgResolver,
	})
	if err != nil {
		t.Fatalf("RefactorOrg: %v", err)
	}

	// The keyword fallback via SearchFunctions should find functions
	if plan != nil {
		output := workflows.FormatRefactorPlan(plan, 8000)
		assertOutputContainsWorkflow(t, output, "Risk")
	}
}

func TestMergeReposEndToEnd(t *testing.T) {
	store, cleanup := setupWorkflowStore(t)
	defer cleanup()

	dirSource := createWorkflowTestDir(t, map[string]string{
		"source.go": `package source

// SharedFunc is a shared function.
func SharedFunc() error { return nil }

// UniqueFunc is unique to source.
func UniqueFunc() string { return "" }
`,
	})

	dirTarget := createWorkflowTestDir(t, map[string]string{
		"target.go": `package target

// SharedFunc is a shared function with different return type.
func SharedFunc() string { return "" }

// ExistingFunc already exists in target.
func ExistingFunc() {}
`,
	})

	cloner, _ := repo.NewCloner(t.TempDir())
	scanner := repo.NewScanner(nil, 1024*1024)
	mgr := orchestrator.NewManager(store, cloner, scanner)

	sourceID := analyzeDir(t, mgr, dirSource)
	targetID := analyzeDir(t, mgr, dirTarget)

	searcher := &orchestratorSearcher{mgr: mgr}
	comparer := comparison.NewComparer()

	report, err := workflows.MergeRepos(context.Background(), workflows.MergeReposParams{
		SourceRepoIDs: []string{sourceID},
		TargetRepoID:  targetID,
		Searcher:      searcher,
		Comparer:      comparer,
	})
	if err != nil {
		t.Fatalf("MergeRepos: %v", err)
	}

	if report == nil {
		t.Fatal("expected non-nil report")
	}

	output := workflows.FormatMergeReport(report, 8000)
	assertOutputContainsWorkflow(t, output, "Duplicates")
	assertOutputContainsWorkflow(t, output, "Risk")
}

func TestAllWorkflowsValidateShortDescriptions(t *testing.T) {
	store, cleanup := setupWorkflowStore(t)
	defer cleanup()

	dir := createWorkflowTestDir(t, map[string]string{
		"main.go": `package main
func Func() {}
`,
	})

	cloner, _ := repo.NewCloner(t.TempDir())
	scanner := repo.NewScanner(nil, 1024*1024)
	mgr := orchestrator.NewManager(store, cloner, scanner)

	projectID := analyzeDir(t, mgr, dir)

	searcher := &orchestratorSearcher{mgr: mgr}
	orgResolver := &staticOrgResolver{orgs: map[string][]string{
		"org": {projectID},
	}}

	// build_feature with short description
	_, err := workflows.BuildFeature(context.Background(), workflows.BuildFeatureParams{
		OrgID:              "org",
		FeatureDescription: "ab",
		Searcher:           searcher,
		OrgResolver:        orgResolver,
	})
	if err == nil {
		t.Error("expected validation error for short feature_description")
	}

	// refactor_org with short description
	_, err = workflows.RefactorOrg(context.Background(), workflows.RefactorOrgParams{
		OrgID:              "org",
		PatternDescription: "",
		Searcher:           searcher,
		OrgResolver:        orgResolver,
	})
	if err == nil {
		t.Error("expected validation error for short pattern_description")
	}
}

func TestAllWorkflowsRespectTokenBudget(t *testing.T) {
	store, cleanup := setupWorkflowStore(t)
	defer cleanup()

	dir := createWorkflowTestDir(t, map[string]string{
		"main.go": `package main
func FuncA() {}
func FuncB() {}
`,
	})

	cloner, _ := repo.NewCloner(t.TempDir())
	scanner := repo.NewScanner(nil, 1024*1024)
	mgr := orchestrator.NewManager(store, cloner, scanner)

	projectID := analyzeDir(t, mgr, dir)

	searcher := &orchestratorSearcher{mgr: mgr}
	orgResolver := &staticOrgResolver{orgs: map[string][]string{
		"org": {projectID},
	}}

	// build_feature with small budget
	plan, err := workflows.BuildFeature(context.Background(), workflows.BuildFeatureParams{
		OrgID:              "org",
		FeatureDescription: "func",
		TokenBudget:        1000,
		Searcher:           searcher,
		OrgResolver:        orgResolver,
	})
	if err != nil {
		t.Fatalf("BuildFeature: %v", err)
	}
	output := workflows.FormatFeaturePlan(plan, 1000)
	// ~4 chars per token, generous bound of 5x
	if len(output) > 1000*5+100 {
		t.Errorf("build_feature output too long: %d chars for 1000 token budget", len(output))
	}

	// merge_repos with small budget - need two different repos
	dirSource := createWorkflowTestDir(t, map[string]string{
		"src.go": `package src
func SourceFunc() {}
`,
	})
	sourceID := analyzeDir(t, mgr, dirSource)

	comparer := comparison.NewComparer()
	report, err := workflows.MergeRepos(context.Background(), workflows.MergeReposParams{
		SourceRepoIDs: []string{sourceID},
		TargetRepoID:  projectID,
		TokenBudget:   1000,
		Searcher:      searcher,
		Comparer:      comparer,
	})
	if err != nil {
		t.Fatalf("MergeRepos: %v", err)
	}
	mergeOutput := workflows.FormatMergeReport(report, 1000)
	if len(mergeOutput) > 1000*5+100 {
		t.Errorf("merge_repos output too long: %d chars for 1000 token budget", len(mergeOutput))
	}
}

// --- Assertion Helpers ---

func assertOutputContainsWorkflow(t *testing.T, output, substring string) {
	t.Helper()
	if !strings.Contains(output, substring) {
		preview := output
		if len(preview) > 500 {
			preview = preview[:500] + "..."
		}
		t.Errorf("output missing %q\nGot: %s", substring, preview)
	}
}
