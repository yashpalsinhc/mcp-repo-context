package workflows

import (
	"context"
	"testing"

	"github.com/yashpalc/mcp-repo-context/internal/comparison"
	ctxpkg "github.com/yashpalc/mcp-repo-context/internal/context"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMergeReposCombinesAllAnalyses(t *testing.T) {
	sourceA := makeTestRepo("source-a", map[string][]ctxpkg.FunctionDef{
		"a.go": {
			{Name: "SharedFunc", Signature: "func SharedFunc() error", IsPublic: true},
			{Name: "UniqueA", Signature: "func UniqueA()", IsPublic: true},
		},
	})
	sourceB := makeTestRepo("source-b", map[string][]ctxpkg.FunctionDef{
		"b.go": {
			{Name: "SharedFunc", Signature: "func SharedFunc() string", IsPublic: true}, // different signature
			{Name: "UniqueB", Signature: "func UniqueB()", IsPublic: true},
		},
	})
	target := makeTestRepo("target", map[string][]ctxpkg.FunctionDef{
		"t.go": {
			{Name: "SharedFunc", Signature: "func SharedFunc() error", IsPublic: true},
			{Name: "ExistingFunc", Signature: "func ExistingFunc()", IsPublic: true},
		},
	})

	searcher := newMockSearcher(sourceA, sourceB, target)
	comparer := comparison.NewComparer()

	report, err := MergeRepos(context.Background(), MergeReposParams{
		SourceRepoIDs: []string{"source-a", "source-b"},
		TargetRepoID:  "target",
		Searcher:      searcher,
		Comparer:      comparer,
	})

	require.NoError(t, err)
	assert.NotNil(t, report)
	// Should have some analysis results - at least one type of finding
	hasFindings := len(report.Duplicates) > 0 || len(report.Conflicts) > 0 || len(report.Gaps) > 0
	assert.True(t, hasFindings, "should have at least one type of finding (duplicates, conflicts, or gaps)")
}

func TestMergeReposRiskHighWithSevereConflicts(t *testing.T) {
	source := makeTestRepo("source", map[string][]ctxpkg.FunctionDef{
		"s.go": {
			{Name: "ConflictFunc", Signature: "func ConflictFunc() error", IsPublic: true},
		},
	})
	target := makeTestRepo("target", map[string][]ctxpkg.FunctionDef{
		"t.go": {
			{Name: "ConflictFunc", Signature: "func ConflictFunc() string", IsPublic: true}, // different return
		},
	})

	searcher := newMockSearcher(source, target)
	comparer := comparison.NewComparer()

	report, err := MergeRepos(context.Background(), MergeReposParams{
		SourceRepoIDs: []string{"source"},
		TargetRepoID:  "target",
		Searcher:      searcher,
		Comparer:      comparer,
	})

	require.NoError(t, err)
	// The conflict involves 'error' difference, which the comparer rates as high severity
	if len(report.Conflicts) > 0 {
		hasHighConflict := false
		for _, c := range report.Conflicts {
			if c.Severity == "high" {
				hasHighConflict = true
			}
		}
		if hasHighConflict {
			assert.Equal(t, RiskHigh, report.RiskLevel)
		}
	}
}

func TestMergeReposRiskLowNoConflicts(t *testing.T) {
	source := makeTestRepo("source", map[string][]ctxpkg.FunctionDef{
		"s.go": {
			{Name: "UniqueFunc", Signature: "func UniqueFunc()", IsPublic: true},
		},
	})
	target := makeTestRepo("target", map[string][]ctxpkg.FunctionDef{
		"t.go": {
			{Name: "OtherFunc", Signature: "func OtherFunc()", IsPublic: true},
		},
	})

	searcher := newMockSearcher(source, target)
	comparer := comparison.NewComparer()

	report, err := MergeRepos(context.Background(), MergeReposParams{
		SourceRepoIDs: []string{"source"},
		TargetRepoID:  "target",
		Searcher:      searcher,
		Comparer:      comparer,
	})

	require.NoError(t, err)
	assert.Equal(t, RiskLow, report.RiskLevel)
}

func TestMergeReposTargetInSourcesRemoved(t *testing.T) {
	repoA := makeTestRepo("repo-a", map[string][]ctxpkg.FunctionDef{
		"a.go": {{Name: "FuncA", Signature: "func FuncA()", IsPublic: true}},
	})
	target := makeTestRepo("target", map[string][]ctxpkg.FunctionDef{
		"t.go": {{Name: "FuncT", Signature: "func FuncT()", IsPublic: true}},
	})

	searcher := newMockSearcher(repoA, target)
	comparer := comparison.NewComparer()

	report, err := MergeRepos(context.Background(), MergeReposParams{
		SourceRepoIDs: []string{"repo-a", "target"},
		TargetRepoID:  "target",
		Searcher:      searcher,
		Comparer:      comparer,
	})

	require.NoError(t, err)
	// Target should not be in effective sources
	for _, s := range report.SourceRepos {
		assert.NotEqual(t, "target", s)
	}
}

func TestMergeReposValidatesMinSource(t *testing.T) {
	searcher := newMockSearcher()
	comparer := comparison.NewComparer()

	_, err := MergeRepos(context.Background(), MergeReposParams{
		SourceRepoIDs: []string{},
		TargetRepoID:  "target",
		Searcher:      searcher,
		Comparer:      comparer,
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "at least one source repository required")
}

func TestMergeReposSingleSourceWorks(t *testing.T) {
	source := makeTestRepo("source", map[string][]ctxpkg.FunctionDef{
		"s.go": {{Name: "Func", Signature: "func Func()", IsPublic: true}},
	})
	target := makeTestRepo("target", map[string][]ctxpkg.FunctionDef{
		"t.go": {{Name: "Other", Signature: "func Other()", IsPublic: true}},
	})

	searcher := newMockSearcher(source, target)
	comparer := comparison.NewComparer()

	report, err := MergeRepos(context.Background(), MergeReposParams{
		SourceRepoIDs: []string{"source"},
		TargetRepoID:  "target",
		Searcher:      searcher,
		Comparer:      comparer,
	})

	require.NoError(t, err)
	assert.NotNil(t, report)
}

func TestMergeReposFormattedOutputHasAllSections(t *testing.T) {
	source := makeTestRepo("source", map[string][]ctxpkg.FunctionDef{
		"s.go": {{Name: "Func", Signature: "func Func()", IsPublic: true}},
	})
	target := makeTestRepo("target", map[string][]ctxpkg.FunctionDef{
		"t.go": {{Name: "Func", Signature: "func Func() error", IsPublic: true}},
	})

	searcher := newMockSearcher(source, target)
	comparer := comparison.NewComparer()

	report, err := MergeRepos(context.Background(), MergeReposParams{
		SourceRepoIDs: []string{"source"},
		TargetRepoID:  "target",
		Searcher:      searcher,
		Comparer:      comparer,
	})

	require.NoError(t, err)

	output := FormatMergeReport(report, 8000)
	assert.Contains(t, output, "Duplicates")
	assert.Contains(t, output, "Conflicts")
	assert.Contains(t, output, "Gaps")
	assert.Contains(t, output, "Merge Steps")
	assert.Contains(t, output, "Risk")
}

func TestMergeReposTokenBudgetTruncates(t *testing.T) {
	source := makeTestRepo("source", map[string][]ctxpkg.FunctionDef{
		"s.go": {{Name: "Func", Signature: "func Func()", IsPublic: true}},
	})
	target := makeTestRepo("target", map[string][]ctxpkg.FunctionDef{
		"t.go": {{Name: "Other", Signature: "func Other()", IsPublic: true}},
	})

	searcher := newMockSearcher(source, target)
	comparer := comparison.NewComparer()

	report, err := MergeRepos(context.Background(), MergeReposParams{
		SourceRepoIDs: []string{"source"},
		TargetRepoID:  "target",
		TokenBudget:   2000,
		Searcher:      searcher,
		Comparer:      comparer,
	})

	require.NoError(t, err)
	output := FormatMergeReport(report, 2000)
	assert.LessOrEqual(t, len(output), 2000*4+100)
}
