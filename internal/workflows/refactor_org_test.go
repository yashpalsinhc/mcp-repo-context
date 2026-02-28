package workflows

import (
	"context"
	"fmt"
	"testing"

	ctxpkg "github.com/yashpalc/mcp-repo-context/internal/context"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRefactorOrgFindsUsages(t *testing.T) {
	repoA := makeTestRepo("repo-a", map[string][]ctxpkg.FunctionDef{
		"auth.go": {
			{Name: "ValidateToken", Signature: "func ValidateToken()", IsPublic: true,
				Behavior: &ctxpkg.FunctionBehavior{Summary: "validates authentication token"}},
		},
	})
	repoB := makeTestRepo("repo-b", map[string][]ctxpkg.FunctionDef{
		"auth.go": {
			{Name: "CheckAuth", Signature: "func CheckAuth()", IsPublic: true,
				Behavior: &ctxpkg.FunctionBehavior{Summary: "checks authentication"}},
		},
	})

	searcher := newMockSearcher(repoA, repoB)
	orgResolver := newMockOrgResolver("org", []string{"repo-a", "repo-b"})

	plan, err := RefactorOrg(context.Background(), RefactorOrgParams{
		OrgID:              "org",
		PatternDescription: "authentication pattern",
		Searcher:           searcher,
		OrgResolver:        orgResolver,
	})

	require.NoError(t, err)
	assert.NotEmpty(t, plan.Usages)
}

func TestRefactorOrgMergesConceptAndSemanticWithoutDuplicates(t *testing.T) {
	repo := makeTestRepo("repo-a", map[string][]ctxpkg.FunctionDef{
		"auth.go": {
			{Name: "ValidateAuth", Signature: "func ValidateAuth()", IsPublic: true,
				Behavior: &ctxpkg.FunctionBehavior{Summary: "validates authentication"}},
		},
	})

	searcher := newMockSearcher(repo)
	orgResolver := newMockOrgResolver("org", []string{"repo-a"})

	plan, err := RefactorOrg(context.Background(), RefactorOrgParams{
		OrgID:              "org",
		PatternDescription: "authentication",
		Searcher:           searcher,
		OrgResolver:        orgResolver,
	})

	require.NoError(t, err)
	// Should be deduplicated - function appears only once
	count := 0
	for _, u := range plan.Usages {
		if u.FuncName == "ValidateAuth" {
			count++
		}
	}
	assert.Equal(t, 1, count, "function should appear exactly once after dedup")
}

func TestRefactorOrgGroupsByRepo(t *testing.T) {
	repos := make([]*ctxpkg.RepoContext, 3)
	repoIDs := make([]string, 3)
	for i := 0; i < 3; i++ {
		id := fmt.Sprintf("repo-%d", i)
		repoIDs[i] = id
		repos[i] = makeTestRepo(id, map[string][]ctxpkg.FunctionDef{
			"validate.go": {{Name: "ValidateInput", Signature: "func ValidateInput()", IsPublic: true,
				Behavior: &ctxpkg.FunctionBehavior{Summary: "validates input"}}},
		})
	}

	searcher := newMockSearcher(repos...)
	orgResolver := newMockOrgResolver("org", repoIDs)

	plan, err := RefactorOrg(context.Background(), RefactorOrgParams{
		OrgID:              "org",
		PatternDescription: "validation",
		Searcher:           searcher,
		OrgResolver:        orgResolver,
	})

	require.NoError(t, err)
	// Verify usages from each repo
	repoSeen := make(map[string]bool)
	for _, u := range plan.Usages {
		repoSeen[u.RepoID] = true
	}
	for _, id := range repoIDs {
		assert.True(t, repoSeen[id], "should have usages from %s", id)
	}
}

func TestRefactorOrgImpactCountsCallers(t *testing.T) {
	repo := makeTestRepo("repo-a", map[string][]ctxpkg.FunctionDef{
		"pattern.go": {
			{Name: "PatternFunc", Signature: "func PatternFunc()", IsPublic: true,
				Behavior: &ctxpkg.FunctionBehavior{Summary: "the pattern function"}},
		},
		"caller1.go": {
			{Name: "CallerA", Signature: "func CallerA()", IsPublic: true,
				Calls: []ctxpkg.CallRef{{Function: "PatternFunc"}}},
		},
		"caller2.go": {
			{Name: "CallerB", Signature: "func CallerB()", IsPublic: true,
				Calls: []ctxpkg.CallRef{{Function: "PatternFunc"}}},
		},
	})

	searcher := newMockSearcher(repo)
	orgResolver := newMockOrgResolver("org", []string{"repo-a"})

	plan, err := RefactorOrg(context.Background(), RefactorOrgParams{
		OrgID:              "org",
		PatternDescription: "pattern",
		Searcher:           searcher,
		OrgResolver:        orgResolver,
	})

	require.NoError(t, err)
	assert.GreaterOrEqual(t, plan.ImpactAnalysis.DirectCallers, 2)
}

func TestRefactorOrgIdentifiesHotPaths(t *testing.T) {
	funcs := map[string][]ctxpkg.FunctionDef{
		"hot.go": {
			{Name: "HotFunc", Signature: "func HotFunc()", IsPublic: true,
				Behavior: &ctxpkg.FunctionBehavior{Summary: "hot path function"}},
		},
	}
	// Add 6 callers to make it a hot path
	for i := 0; i < 6; i++ {
		file := fmt.Sprintf("caller%d.go", i)
		funcs[file] = []ctxpkg.FunctionDef{
			{Name: fmt.Sprintf("Caller%d", i), Signature: fmt.Sprintf("func Caller%d()", i), IsPublic: true,
				Calls: []ctxpkg.CallRef{{Function: "HotFunc"}}},
		}
	}

	repo := makeTestRepo("repo-a", funcs)
	searcher := newMockSearcher(repo)
	orgResolver := newMockOrgResolver("org", []string{"repo-a"})

	plan, err := RefactorOrg(context.Background(), RefactorOrgParams{
		OrgID:              "org",
		PatternDescription: "hot path",
		Searcher:           searcher,
		OrgResolver:        orgResolver,
	})

	require.NoError(t, err)
	assert.Contains(t, plan.ImpactAnalysis.HotPaths, "HotFunc")
}

func TestRefactorOrgRiskHighManyRepos(t *testing.T) {
	repos := make([]*ctxpkg.RepoContext, 6)
	repoIDs := make([]string, 6)
	for i := 0; i < 6; i++ {
		id := fmt.Sprintf("repo-%d", i)
		repoIDs[i] = id
		repos[i] = makeTestRepo(id, map[string][]ctxpkg.FunctionDef{
			"f.go": {{Name: "Func", Signature: "func Func()", IsPublic: true,
				Behavior: &ctxpkg.FunctionBehavior{Summary: "pattern func"}}},
		})
	}

	searcher := newMockSearcher(repos...)
	orgResolver := newMockOrgResolver("org", repoIDs)

	plan, err := RefactorOrg(context.Background(), RefactorOrgParams{
		OrgID:              "org",
		PatternDescription: "pattern func",
		Searcher:           searcher,
		OrgResolver:        orgResolver,
	})

	require.NoError(t, err)
	assert.Equal(t, RiskHigh, plan.RiskLevel)
}

func TestRefactorOrgRiskLowSingleRepo(t *testing.T) {
	repo := makeTestRepo("repo-a", map[string][]ctxpkg.FunctionDef{
		"f.go": {
			{Name: "SmallFunc", Signature: "func SmallFunc()", IsPublic: true,
				Behavior: &ctxpkg.FunctionBehavior{Summary: "small pattern"}},
		},
	})

	searcher := newMockSearcher(repo)
	orgResolver := newMockOrgResolver("org", []string{"repo-a"})

	plan, err := RefactorOrg(context.Background(), RefactorOrgParams{
		OrgID:              "org",
		PatternDescription: "small pattern",
		Searcher:           searcher,
		OrgResolver:        orgResolver,
	})

	require.NoError(t, err)
	assert.Equal(t, RiskLow, plan.RiskLevel)
}

func TestRefactorOrgAffectedFilesIncludesCallerFiles(t *testing.T) {
	repo := makeTestRepo("repo-a", map[string][]ctxpkg.FunctionDef{
		"pattern.go": {
			{Name: "PatternFunc", Signature: "func PatternFunc()", IsPublic: true,
				Behavior: &ctxpkg.FunctionBehavior{Summary: "the pattern function"}},
		},
		"caller.go": {
			{Name: "CallerFunc", Signature: "func CallerFunc()", IsPublic: true,
				Calls: []ctxpkg.CallRef{{Function: "PatternFunc"}}},
		},
	})

	searcher := newMockSearcher(repo)
	orgResolver := newMockOrgResolver("org", []string{"repo-a"})

	plan, err := RefactorOrg(context.Background(), RefactorOrgParams{
		OrgID:              "org",
		PatternDescription: "pattern",
		Searcher:           searcher,
		OrgResolver:        orgResolver,
	})

	require.NoError(t, err)

	// pattern.go should have "modify" action
	foundModify := false
	foundCallerFile := false
	for _, fa := range plan.AffectedFiles {
		if fa.FilePath == "pattern.go" && fa.Action == "modify" {
			foundModify = true
		}
		if fa.FilePath == "caller.go" {
			foundCallerFile = true
			// caller.go may be "modify" (if CallerFunc is also a usage) or "review" (if only a caller)
			assert.Contains(t, []string{"modify", "review"}, fa.Action)
		}
	}
	assert.True(t, foundModify, "pattern.go should have modify action")
	assert.True(t, foundCallerFile, "caller.go should appear in affected files")
}

func TestRefactorOrgValidatesMinLength(t *testing.T) {
	searcher := newMockSearcher()
	orgResolver := newMockOrgResolver("org", []string{"repo-a"})

	_, err := RefactorOrg(context.Background(), RefactorOrgParams{
		OrgID:              "org",
		PatternDescription: "ab",
		Searcher:           searcher,
		OrgResolver:        orgResolver,
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "pattern_description must be at least 3 characters")
}

func TestRefactorOrgTargetReposFilters(t *testing.T) {
	repoA := makeTestRepo("repo-a", map[string][]ctxpkg.FunctionDef{
		"a.go": {{Name: "FuncA", Signature: "func FuncA()", IsPublic: true,
			Behavior: &ctxpkg.FunctionBehavior{Summary: "does things"}}},
	})
	repoB := makeTestRepo("repo-b", map[string][]ctxpkg.FunctionDef{
		"b.go": {{Name: "FuncB", Signature: "func FuncB()", IsPublic: true,
			Behavior: &ctxpkg.FunctionBehavior{Summary: "does things"}}},
	})

	searcher := newMockSearcher(repoA, repoB)
	orgResolver := newMockOrgResolver("org", []string{"repo-a", "repo-b"})

	plan, err := RefactorOrg(context.Background(), RefactorOrgParams{
		OrgID:              "org",
		PatternDescription: "things",
		TargetRepos:        []string{"repo-b"},
		Searcher:           searcher,
		OrgResolver:        orgResolver,
	})

	require.NoError(t, err)
	for _, u := range plan.Usages {
		assert.Equal(t, "repo-b", u.RepoID)
	}
}

func TestRefactorOrgKeywordFallback(t *testing.T) {
	repo := makeTestRepo("repo-a", map[string][]ctxpkg.FunctionDef{
		"f.go": {{Name: "HelperFunc", Signature: "func HelperFunc()", IsPublic: true,
			Behavior: &ctxpkg.FunctionBehavior{Summary: "helps"}}},
	})

	searcher := newMockSearcher(repo)
	orgResolver := newMockOrgResolver("org", []string{"repo-a"})

	plan, err := RefactorOrg(context.Background(), RefactorOrgParams{
		OrgID:              "org",
		PatternDescription: "helper func",
		Searcher:           searcher,
		OrgResolver:        orgResolver,
		Vectors:            nil,
	})

	require.NoError(t, err)
	assert.NotEmpty(t, plan.Usages)
}
