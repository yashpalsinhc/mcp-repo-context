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

	sourceRepos := []*ctxpkg.RepoContext{
		{
			ID: "github.com/test/source1",
			Files: map[string]*ctxpkg.FileContext{
				"util.go": {
					Path: "util.go",
					Functions: []ctxpkg.FunctionDef{
						{Name: "HelperFunc", Signature: "func HelperFunc()"},
						{Name: "SharedFunc", Signature: "func SharedFunc()"},
					},
				},
			},
		},
		{
			ID: "github.com/test/source2",
			Files: map[string]*ctxpkg.FileContext{
				"helper.go": {
					Path: "helper.go",
					Functions: []ctxpkg.FunctionDef{
						{Name: "HelperFunc", Signature: "func HelperFunc()"},
						{Name: "AnotherFunc", Signature: "func AnotherFunc()"},
					},
				},
			},
		},
	}

	targetRepo := &ctxpkg.RepoContext{
		ID: "github.com/test/target",
		Files: map[string]*ctxpkg.FileContext{
			"main.go": {
				Path: "main.go",
				Functions: []ctxpkg.FunctionDef{
					{Name: "main", Signature: "func main()"},
				},
			},
		},
	}

	gaps, err := c.FindGaps(ctx, sourceRepos, targetRepo)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should find HelperFunc (from 2 repos) as high priority
	// SharedFunc and AnotherFunc should also be gaps
	if len(gaps) < 3 {
		t.Errorf("expected at least 3 gaps, got %d", len(gaps))
	}

	// HelperFunc should be high priority (appears in 2 repos)
	for _, gap := range gaps {
		if gap.Name == "HelperFunc" {
			if gap.Priority != "medium" { // 2 repos = medium
				t.Errorf("expected medium priority for HelperFunc, got %s", gap.Priority)
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
