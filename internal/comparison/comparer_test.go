package comparison

import (
	"context"
	"testing"
	"time"

	ctxpkg "github.com/yashpalc/mcp-repo-context/internal/context"
)

func TestNewComparer(t *testing.T) {
	c := NewComparer()
	if c == nil {
		t.Fatal("expected non-nil comparer")
	}
}

func TestComparer_Compare_Empty(t *testing.T) {
	c := NewComparer()
	ctx := context.Background()

	result, err := c.Compare(ctx, nil, DefaultCompareOptions())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

func TestComparer_Compare_SingleRepo(t *testing.T) {
	c := NewComparer()
	ctx := context.Background()

	repos := []*ctxpkg.RepoContext{
		{
			ID:     "github.com/test/repo1",
			URL:    "https://github.com/test/repo1",
			Branch: "main",
			Files: map[string]*ctxpkg.FileContext{
				"main.go": {
					Path:     "main.go",
					Language: "go",
				},
			},
			Statistics: ctxpkg.RepoStatistics{
				TotalFiles:    1,
				FunctionCount: 5,
				TypeCount:     2,
			},
		},
	}

	result, err := c.Compare(ctx, repos, DefaultCompareOptions())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Repos) != 1 {
		t.Errorf("expected 1 repo, got %d", len(result.Repos))
	}
}

func TestComparer_Compare_MultipleRepos(t *testing.T) {
	c := NewComparer()
	ctx := context.Background()

	repos := []*ctxpkg.RepoContext{
		{
			ID:     "github.com/test/repo1",
			URL:    "https://github.com/test/repo1",
			Branch: "main",
			Files: map[string]*ctxpkg.FileContext{
				"main.go": {
					Path:     "main.go",
					Language: "go",
				},
				"util.go": {
					Path:     "util.go",
					Language: "go",
				},
			},
			Statistics: ctxpkg.RepoStatistics{
				TotalFiles: 2,
				TotalLines: 100,
			},
		},
		{
			ID:     "github.com/test/repo2",
			URL:    "https://github.com/test/repo2",
			Branch: "main",
			Files: map[string]*ctxpkg.FileContext{
				"main.go": {
					Path:     "main.go",
					Language: "go",
				},
				"helper.go": {
					Path:     "helper.go",
					Language: "go",
				},
			},
			Statistics: ctxpkg.RepoStatistics{
				TotalFiles: 2,
				TotalLines: 80,
			},
		},
	}

	result, err := c.Compare(ctx, repos, DefaultCompareOptions())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Repos) != 2 {
		t.Errorf("expected 2 repos, got %d", len(result.Repos))
	}

	// Check unified stats
	if result.UnifiedStatistics.TotalRepos != 2 {
		t.Errorf("expected 2 total repos, got %d", result.UnifiedStatistics.TotalRepos)
	}
	if result.UnifiedStatistics.TotalFiles != 4 {
		t.Errorf("expected 4 total files, got %d", result.UnifiedStatistics.TotalFiles)
	}
}

func TestComparer_Compare_WithTarget(t *testing.T) {
	c := NewComparer()
	ctx := context.Background()

	repos := []*ctxpkg.RepoContext{
		{
			ID:     "github.com/test/source1",
			URL:    "https://github.com/test/source1",
			Branch: "main",
			Files:  map[string]*ctxpkg.FileContext{},
		},
		{
			ID:     "github.com/test/target",
			URL:    "https://github.com/test/target",
			Branch: "main",
			Files:  map[string]*ctxpkg.FileContext{},
		},
	}

	opts := DefaultCompareOptions()
	opts.TargetRepoID = "github.com/test/target"

	result, err := c.Compare(ctx, repos, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify target is marked correctly
	targetFound := false
	for _, repo := range result.Repos {
		if repo.ID == "github.com/test/target" && repo.IsTarget {
			targetFound = true
		}
	}
	if !targetFound {
		t.Error("expected target repo to be marked as target")
	}
}

func TestComparer_FindDuplicates(t *testing.T) {
	c := NewComparer()
	ctx := context.Background()

	repos := []*ctxpkg.RepoContext{
		{
			ID: "github.com/test/repo1",
			Files: map[string]*ctxpkg.FileContext{
				"main.go": {
					Path: "main.go",
					Functions: []ctxpkg.FunctionDef{
						{Name: "handleRequest", Signature: "func handleRequest()", LineStart: 10},
						{Name: "uniqueFunc1", Signature: "func uniqueFunc1()", LineStart: 20},
					},
				},
			},
		},
		{
			ID: "github.com/test/repo2",
			Files: map[string]*ctxpkg.FileContext{
				"handler.go": {
					Path: "handler.go",
					Functions: []ctxpkg.FunctionDef{
						{Name: "handleRequest", Signature: "func handleRequest(w, r)", LineStart: 5},
						{Name: "uniqueFunc2", Signature: "func uniqueFunc2()", LineStart: 15},
					},
				},
			},
		},
	}

	duplicates, err := c.FindDuplicates(ctx, repos)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should find handleRequest as duplicate
	found := false
	for _, dup := range duplicates {
		if dup.Name == "handleRequest" {
			found = true
			if len(dup.Instances) != 2 {
				t.Errorf("expected 2 instances, got %d", len(dup.Instances))
			}
		}
	}
	if !found {
		t.Error("expected to find handleRequest as duplicate")
	}
}

func TestComparer_FindConflicts(t *testing.T) {
	c := NewComparer()
	ctx := context.Background()

	sourceRepos := []*ctxpkg.RepoContext{
		{
			ID: "github.com/test/source",
			Files: map[string]*ctxpkg.FileContext{
				"handler.go": {
					Path: "handler.go",
					Functions: []ctxpkg.FunctionDef{
						{Name: "Process", Signature: "func Process(data []byte) error", LineStart: 10},
					},
				},
			},
		},
	}

	targetRepo := &ctxpkg.RepoContext{
		ID: "github.com/test/target",
		Files: map[string]*ctxpkg.FileContext{
			"processor.go": {
				Path: "processor.go",
				Functions: []ctxpkg.FunctionDef{
					{Name: "Process", Signature: "func Process(data string) (string, error)", LineStart: 5},
				},
			},
		},
	}

	conflicts, err := c.FindConflicts(ctx, sourceRepos, targetRepo)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should find Process as conflicting (different signatures)
	found := false
	for _, conflict := range conflicts {
		if conflict.Name == "Process" {
			found = true
			if conflict.Type != "signature_mismatch" {
				t.Errorf("expected signature_mismatch, got %s", conflict.Type)
			}
		}
	}
	if !found {
		t.Error("expected to find Process as conflict")
	}
}

func TestComparer_FindGaps(t *testing.T) {
	c := NewComparer()
	ctx := context.Background()

	// Source has route-related functions that are similar to the target domain
	sourceRepos := []*ctxpkg.RepoContext{
		{
			ID: "github.com/test/source1",
			Files: map[string]*ctxpkg.FileContext{
				"util.go": {
					Path:    "util.go",
					Package: "routing",
					Functions: []ctxpkg.FunctionDef{
						{Name: "RouteHelper", Signature: "func RouteHelper()", Description: "helps with routes"},
						{Name: "SharedRouter", Signature: "func SharedRouter()", Description: "shared router setup"},
					},
				},
			},
		},
		{
			ID: "github.com/test/source2",
			Files: map[string]*ctxpkg.FileContext{
				"helper.go": {
					Path:    "helper.go",
					Package: "routing",
					Functions: []ctxpkg.FunctionDef{
						{Name: "RouteHelper", Signature: "func RouteHelper()", Description: "helps with routes"},
						{Name: "SubrouteBuilder", Signature: "func SubrouteBuilder()", Description: "builds sub-routes"},
					},
				},
			},
		},
	}

	targetRepo := &ctxpkg.RepoContext{
		ID: "github.com/test/target",
		Files: map[string]*ctxpkg.FileContext{
			"main.go": {
				Path:    "main.go",
				Package: "routing",
				Functions: []ctxpkg.FunctionDef{
					{Name: "HandleRoute", Signature: "func HandleRoute()"},
					{Name: "NewRouter", Signature: "func NewRouter()"},
				},
				Types: []ctxpkg.TypeDef{{Name: "Router"}, {Name: "Route"}},
			},
		},
	}

	gaps, err := c.FindGaps(ctx, sourceRepos, targetRepo)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should find route-related gaps that have high similarity to target domain
	if len(gaps) == 0 {
		t.Error("expected some gaps for route-related source functions")
	}

	// RouteHelper should appear from 2 repos
	for _, gap := range gaps {
		if gap.Name == "RouteHelper" {
			if gap.Priority != "medium" { // 2 repos = medium
				t.Errorf("expected medium priority for RouteHelper, got %s", gap.Priority)
			}
			if len(gap.SourceRepos) != 2 {
				t.Errorf("expected 2 source repos, got %d", len(gap.SourceRepos))
			}
		}
	}
}

func TestComparer_AnalyzeConsistency(t *testing.T) {
	c := NewComparer()
	ctx := context.Background()

	repos := []*ctxpkg.RepoContext{
		{
			ID: "github.com/test/repo1",
			Files: map[string]*ctxpkg.FileContext{
				"main.go": {
					Path: "main.go",
					Functions: []ctxpkg.FunctionDef{
						{Name: "HandleRequest"},
						{Name: "ProcessData"},
					},
				},
			},
		},
		{
			ID: "github.com/test/repo2",
			Files: map[string]*ctxpkg.FileContext{
				"handler.go": {
					Path: "handler.go",
					Functions: []ctxpkg.FunctionDef{
						{Name: "handle_request"}, // snake_case
						{Name: "process_data"},   // snake_case
					},
				},
			},
		},
	}

	report, err := c.AnalyzeConsistency(ctx, repos)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if report == nil {
		t.Fatal("expected non-nil report")
	}

	// Should have naming consistency issues (mixed camelCase and snake_case)
	if report.OverallScore >= 1.0 {
		t.Error("expected some consistency issues")
	}
}

func TestComparer_FileOverlap(t *testing.T) {
	c := NewComparer()
	ctx := context.Background()

	repos := []*ctxpkg.RepoContext{
		{
			ID: "github.com/test/repo1",
			Files: map[string]*ctxpkg.FileContext{
				"main.go": {Path: "main.go", Hash: "abc123"},
				"util.go": {Path: "util.go", Hash: "def456"},
			},
		},
		{
			ID: "github.com/test/repo2",
			Files: map[string]*ctxpkg.FileContext{
				"main.go":   {Path: "main.go", Hash: "abc123"}, // Same hash
				"helper.go": {Path: "helper.go", Hash: "ghi789"},
			},
		},
	}

	result, err := c.Compare(ctx, repos, DefaultCompareOptions())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should find main.go as overlapping
	found := false
	for _, overlap := range result.FileOverlap {
		if overlap.Path == "main.go" {
			found = true
			if !overlap.Identical {
				t.Error("expected main.go to be identical")
			}
			if len(overlap.Repos) != 2 {
				t.Errorf("expected 2 repos, got %d", len(overlap.Repos))
			}
		}
	}
	if !found {
		t.Error("expected to find main.go in overlap")
	}
}

func TestDefaultCompareOptions(t *testing.T) {
	opts := DefaultCompareOptions()

	if !opts.IncludeDuplicates {
		t.Error("expected IncludeDuplicates to be true")
	}
	if !opts.IncludeConflicts {
		t.Error("expected IncludeConflicts to be true")
	}
	if !opts.IncludeGaps {
		t.Error("expected IncludeGaps to be true")
	}
	if !opts.IncludeConsistency {
		t.Error("expected IncludeConsistency to be true")
	}
	if opts.SimilarityThreshold != 0.8 {
		t.Errorf("expected threshold 0.8, got %f", opts.SimilarityThreshold)
	}
}

// Helper to suppress unused variable warning
var _ = time.Now()

// === Section 02: Receiver-Aware Comparison Key Tests ===

func TestNormalizeFunctionKey_WithPointerReceiver(t *testing.T) {
	c := &comparer{}
	fn := &ctxpkg.FunctionDef{Name: "ServeHTTP", Receiver: "*Router"}
	got := c.normalizeFunctionKey(fn)
	if got != "Router.ServeHTTP" {
		t.Errorf("got %q, want %q", got, "Router.ServeHTTP")
	}
}

func TestNormalizeFunctionKey_WithValueReceiver(t *testing.T) {
	c := &comparer{}
	fn := &ctxpkg.FunctionDef{Name: "ServeHTTP", Receiver: "Router"}
	got := c.normalizeFunctionKey(fn)
	if got != "Router.ServeHTTP" {
		t.Errorf("got %q, want %q", got, "Router.ServeHTTP")
	}
}

func TestNormalizeFunctionKey_NoReceiver(t *testing.T) {
	c := &comparer{}
	fn := &ctxpkg.FunctionDef{Name: "NewRouter"}
	got := c.normalizeFunctionKey(fn)
	if got != "NewRouter" {
		t.Errorf("got %q, want %q", got, "NewRouter")
	}
}

func TestNormalizeTypeKey_DifferentPackages(t *testing.T) {
	c := &comparer{}
	td := &ctxpkg.TypeDef{Name: "Handler"}
	k1 := c.normalizeTypeKey(td, "mux")
	k2 := c.normalizeTypeKey(td, "cors")
	if k1 == k2 {
		t.Errorf("same-named types in different packages should produce different keys: %q vs %q", k1, k2)
	}
}

func TestFindDuplicates_DifferentReceivers_NotDuplicate(t *testing.T) {
	c := NewComparer()
	ctx := context.Background()

	mux := &ctxpkg.RepoContext{
		ID: "github.com/gorilla/mux", Version: 1,
		Files: map[string]*ctxpkg.FileContext{
			"router.go": {Package: "mux", Functions: []ctxpkg.FunctionDef{
				{Name: "ServeHTTP", Receiver: "*Router", Signature: "func(w,r)"},
			}},
		},
	}
	handlers := &ctxpkg.RepoContext{
		ID: "github.com/gorilla/handlers", Version: 1,
		Files: map[string]*ctxpkg.FileContext{
			"cors.go": {Package: "handlers", Functions: []ctxpkg.FunctionDef{
				{Name: "ServeHTTP", Receiver: "*cors", Signature: "func(w,r)"},
			}},
		},
	}

	dups, err := c.FindDuplicates(ctx, []*ctxpkg.RepoContext{mux, handlers})
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range dups {
		if d.Name == "ServeHTTP" {
			t.Error("ServeHTTP on different receivers should NOT be duplicate")
		}
	}
}

func TestFindDuplicates_SameReceiver_IsDuplicate(t *testing.T) {
	c := NewComparer()
	ctx := context.Background()

	repo1 := &ctxpkg.RepoContext{
		ID: "repo1", Version: 1,
		Files: map[string]*ctxpkg.FileContext{
			"router.go": {Package: "mux", Functions: []ctxpkg.FunctionDef{
				{Name: "ServeHTTP", Receiver: "*Router"},
			}},
		},
	}
	repo2 := &ctxpkg.RepoContext{
		ID: "repo2", Version: 1,
		Files: map[string]*ctxpkg.FileContext{
			"router.go": {Package: "mux", Functions: []ctxpkg.FunctionDef{
				{Name: "ServeHTTP", Receiver: "*Router"},
			}},
		},
	}

	dups, err := c.FindDuplicates(ctx, []*ctxpkg.RepoContext{repo1, repo2})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, d := range dups {
		if d.Name == "Router.ServeHTTP" {
			found = true
		}
	}
	if !found {
		t.Error("Router.ServeHTTP should be flagged as duplicate")
	}
}

func TestFindConflicts_DifferentReceivers_NotConflict(t *testing.T) {
	c := NewComparer()
	ctx := context.Background()

	target := &ctxpkg.RepoContext{
		ID: "mux", Version: 1,
		Files: map[string]*ctxpkg.FileContext{
			"router.go": {Package: "mux", Functions: []ctxpkg.FunctionDef{
				{Name: "ServeHTTP", Receiver: "*Router", Signature: "func(w,r)"},
			}},
		},
	}
	source := &ctxpkg.RepoContext{
		ID: "handlers", Version: 1,
		Files: map[string]*ctxpkg.FileContext{
			"cors.go": {Package: "handlers", Functions: []ctxpkg.FunctionDef{
				{Name: "ServeHTTP", Receiver: "*cors", Signature: "func(w,r,opts)"},
			}},
		},
	}

	conflicts, err := c.FindConflicts(ctx, []*ctxpkg.RepoContext{source}, target)
	if err != nil {
		t.Fatal(err)
	}
	for _, conflict := range conflicts {
		if conflict.Name == "ServeHTTP" {
			t.Error("ServeHTTP on different receivers should NOT be a conflict")
		}
	}
}

func TestFindConflicts_SameReceiver_IsConflict(t *testing.T) {
	c := NewComparer()
	ctx := context.Background()

	target := &ctxpkg.RepoContext{
		ID: "repo1", Version: 1,
		Files: map[string]*ctxpkg.FileContext{
			"router.go": {Package: "mux", Functions: []ctxpkg.FunctionDef{
				{Name: "ServeHTTP", Receiver: "*Router", Signature: "func(w,r)"},
			}},
		},
	}
	source := &ctxpkg.RepoContext{
		ID: "repo2", Version: 1,
		Files: map[string]*ctxpkg.FileContext{
			"router.go": {Package: "mux", Functions: []ctxpkg.FunctionDef{
				{Name: "ServeHTTP", Receiver: "*Router", Signature: "func(w,r,opts)"},
			}},
		},
	}

	conflicts, err := c.FindConflicts(ctx, []*ctxpkg.RepoContext{source}, target)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, conflict := range conflicts {
		if conflict.Name == "Router.ServeHTTP" {
			found = true
		}
	}
	if !found {
		t.Error("Router.ServeHTTP with different signatures should be a conflict")
	}
}

func TestFindGaps_DifferentReceivers_SimilarityFiltering(t *testing.T) {
	c := NewComparer()
	ctx := context.Background()

	target := &ctxpkg.RepoContext{
		ID: "mux", Version: 1,
		Files: map[string]*ctxpkg.FileContext{
			"router.go": {Package: "mux", Functions: []ctxpkg.FunctionDef{
				{Name: "ServeHTTP", Receiver: "*Router"},
				{Name: "HandleFunc", Receiver: "*Router"},
				{Name: "NewRouter"},
			}, Types: []ctxpkg.TypeDef{{Name: "Router", Kind: "struct"}}},
		},
	}
	source := &ctxpkg.RepoContext{
		ID: "handlers", Version: 1,
		Files: map[string]*ctxpkg.FileContext{
			"cors.go": {Package: "handlers", Functions: []ctxpkg.FunctionDef{
				{Name: "ServeHTTP", Receiver: "*cors"},
				{Name: "CORS", Description: "returns CORS middleware handler"},
				{Name: "CompressHandler", Description: "compresses response bodies"},
			}, Types: []ctxpkg.TypeDef{{Name: "cors", Kind: "struct"}}},
		},
	}

	gaps, err := c.FindGaps(ctx, []*ctxpkg.RepoContext{source}, target)
	if err != nil {
		t.Fatal(err)
	}
	for _, gap := range gaps {
		if gap.Name == "CORS" || gap.Name == "CompressHandler" {
			t.Errorf("gap %q should be filtered out (low similarity), got similarity=%.2f", gap.Name, gap.Similarity)
		}
	}
}

func TestEnsureMigrated_BumpsVersion(t *testing.T) {
	rc := &ctxpkg.RepoContext{Version: 0}
	ensureMigrated([]*ctxpkg.RepoContext{rc})
	if rc.Version != 1 {
		t.Errorf("Version should be 1 after migration, got %d", rc.Version)
	}
}

func TestEnsureMigrated_Idempotent(t *testing.T) {
	rc := &ctxpkg.RepoContext{Version: 0}
	ensureMigrated([]*ctxpkg.RepoContext{rc})
	ensureMigrated([]*ctxpkg.RepoContext{rc})
	if rc.Version != 1 {
		t.Errorf("Version should still be 1, got %d", rc.Version)
	}
}

func TestEnsureMigrated_SkipsV1(t *testing.T) {
	rc := &ctxpkg.RepoContext{Version: 1}
	ensureMigrated([]*ctxpkg.RepoContext{rc})
	if rc.Version != 1 {
		t.Errorf("Version should remain 1, got %d", rc.Version)
	}
}

func TestGaps_StopWordsNotInflating(t *testing.T) {
	c := NewComparer()
	ctx := context.Background()

	target := &ctxpkg.RepoContext{
		ID: "target", Version: 1,
		Files: map[string]*ctxpkg.FileContext{
			"route.go": {Package: "routing", Functions: []ctxpkg.FunctionDef{
				{Name: "HandleRoute"}, {Name: "NewRouter"},
			}},
		},
	}
	source := &ctxpkg.RepoContext{
		ID: "source", Version: 1,
		Files: map[string]*ctxpkg.FileContext{
			"util.go": {Package: "util", Functions: []ctxpkg.FunctionDef{
				{Name: "GetNewHandler"},
			}},
		},
	}

	gaps, err := c.FindGaps(ctx, []*ctxpkg.RepoContext{source}, target)
	if err != nil {
		t.Fatal(err)
	}
	for _, gap := range gaps {
		if gap.Name == "GetNewHandler" {
			t.Errorf("GetNewHandler (all stop words) should be filtered out, got similarity=%.2f", gap.Similarity)
		}
	}
}

func TestGaps_SortedBySimilarity(t *testing.T) {
	c := NewComparer()
	ctx := context.Background()

	target := &ctxpkg.RepoContext{
		ID: "target", Version: 1,
		Files: map[string]*ctxpkg.FileContext{
			"route.go": {Package: "routing", Functions: []ctxpkg.FunctionDef{
				{Name: "HandleRoute"}, {Name: "NewRouter"}, {Name: "RouteMatch"},
			}, Types: []ctxpkg.TypeDef{{Name: "Router"}, {Name: "Route"}}},
		},
	}
	source := &ctxpkg.RepoContext{
		ID: "source", Version: 1,
		Files: map[string]*ctxpkg.FileContext{
			"sub.go": {Package: "ext", Functions: []ctxpkg.FunctionDef{
				{Name: "SubrouteHandler", Description: "handles sub-routes for routing"},
				{Name: "RouteValidator", Description: "validates routes"},
			}},
		},
	}

	gaps, err := c.FindGaps(ctx, []*ctxpkg.RepoContext{source}, target)
	if err != nil {
		t.Fatal(err)
	}
	for i := 1; i < len(gaps); i++ {
		if gaps[i].Similarity > gaps[i-1].Similarity {
			t.Errorf("gaps not sorted: gaps[%d]=%.2f > gaps[%d]=%.2f",
				i, gaps[i].Similarity, i-1, gaps[i-1].Similarity)
		}
	}
}
