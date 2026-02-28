package workflows

import (
	"context"
	"fmt"
	"testing"

	ctxpkg "github.com/yashpalc/mcp-repo-context/internal/context"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildFeatureReturnsRelevantCode(t *testing.T) {
	repoA := makeTestRepo("repo-a", map[string][]ctxpkg.FunctionDef{
		"user.go": {
			{Name: "GetUser", Signature: "func GetUser(id int) (*User, error)", IsPublic: true,
				Behavior: &ctxpkg.FunctionBehavior{Summary: "Retrieves user by ID"}},
		},
	})
	repoB := makeTestRepo("repo-b", map[string][]ctxpkg.FunctionDef{
		"order.go": {
			{Name: "CreateOrder", Signature: "func CreateOrder()", IsPublic: true,
				Behavior: &ctxpkg.FunctionBehavior{Summary: "Creates an order"}},
		},
	})

	searcher := newMockSearcher(repoA, repoB)
	orgResolver := newMockOrgResolver("test-org", []string{"repo-a", "repo-b"})

	plan, err := BuildFeature(context.Background(), BuildFeatureParams{
		OrgID:              "test-org",
		FeatureDescription: "user management",
		Searcher:           searcher,
		OrgResolver:        orgResolver,
		Vectors:            nil,
	})

	require.NoError(t, err)
	assert.NotEmpty(t, plan.RelevantCode)
	found := false
	for _, loc := range plan.RelevantCode {
		if loc.FuncName == "GetUser" {
			found = true
		}
	}
	assert.True(t, found, "should find GetUser for 'user management' query")

	for i := 1; i < len(plan.RelevantCode); i++ {
		assert.GreaterOrEqual(t, plan.RelevantCode[i-1].Score, plan.RelevantCode[i].Score)
	}
}

func TestBuildFeatureIdentifiesEntryPoints(t *testing.T) {
	repo := makeTestRepo("repo-a", map[string][]ctxpkg.FunctionDef{
		"handler.go": {
			{Name: "GetUser", Signature: "func GetUser()", IsPublic: true,
				Behavior: &ctxpkg.FunctionBehavior{Summary: "Gets user"}},
			{Name: "validateUser", Signature: "func validateUser()", IsPublic: false,
				Behavior: &ctxpkg.FunctionBehavior{Summary: "Validates user"}},
		},
	})

	searcher := newMockSearcher(repo)
	orgResolver := newMockOrgResolver("org", []string{"repo-a"})

	plan, err := BuildFeature(context.Background(), BuildFeatureParams{
		OrgID:              "org",
		FeatureDescription: "user",
		Searcher:           searcher,
		OrgResolver:        orgResolver,
	})

	require.NoError(t, err)

	foundPublic := false
	for _, ep := range plan.EntryPoints {
		if ep.FuncName == "GetUser" {
			foundPublic = true
			assert.Contains(t, ep.Why, "public")
		}
	}
	assert.True(t, foundPublic)

	for _, ep := range plan.EntryPoints {
		assert.NotEqual(t, "validateUser", ep.FuncName)
	}
}

func TestBuildFeatureIdentifiesNameDependencies(t *testing.T) {
	repoA := makeTestRepo("repo-a", map[string][]ctxpkg.FunctionDef{
		"a.go": {
			{Name: "FuncX", Signature: "func FuncX()", IsPublic: true,
				Behavior: &ctxpkg.FunctionBehavior{Summary: "calls FuncY"},
				Calls:    []ctxpkg.CallRef{{Function: "FuncY"}}},
		},
	})
	repoB := makeTestRepo("repo-b", map[string][]ctxpkg.FunctionDef{
		"b.go": {
			{Name: "FuncY", Signature: "func FuncY()", IsPublic: true,
				Behavior: &ctxpkg.FunctionBehavior{Summary: "target function"}},
		},
	})

	searcher := newMockSearcher(repoA, repoB)
	orgResolver := newMockOrgResolver("org", []string{"repo-a", "repo-b"})

	plan, err := BuildFeature(context.Background(), BuildFeatureParams{
		OrgID:              "org",
		FeatureDescription: "func",
		Searcher:           searcher,
		OrgResolver:        orgResolver,
	})

	require.NoError(t, err)

	found := false
	for _, dep := range plan.Dependencies {
		if dep.SourceRepoID == "repo-a" && dep.TargetRepoID == "repo-b" && dep.TargetFunc == "FuncY" {
			found = true
			assert.Equal(t, "name_match", dep.Type)
			assert.Equal(t, "medium", dep.Confidence)
		}
	}
	assert.True(t, found, "should find cross-repo dependency")
}

func TestBuildFeatureSuggestsOrderSharedLibsFirst(t *testing.T) {
	repoShared := makeTestRepo("shared-lib", map[string][]ctxpkg.FunctionDef{
		"shared.go": {{Name: "Validate", Signature: "func Validate()", IsPublic: true,
			Behavior: &ctxpkg.FunctionBehavior{Summary: "validates"}}},
	})
	repoService := makeTestRepo("service", map[string][]ctxpkg.FunctionDef{
		"svc.go": {{Name: "HandleRequest", Signature: "func HandleRequest()", IsPublic: true,
			Behavior: &ctxpkg.FunctionBehavior{Summary: "handles request"},
			Calls:    []ctxpkg.CallRef{{Function: "Validate"}}}},
	})

	searcher := newMockSearcher(repoShared, repoService)
	orgResolver := newMockOrgResolver("org", []string{"shared-lib", "service"})

	plan, err := BuildFeature(context.Background(), BuildFeatureParams{
		OrgID:              "org",
		FeatureDescription: "validate handle",
		Searcher:           searcher,
		OrgResolver:        orgResolver,
	})

	require.NoError(t, err)
	require.Len(t, plan.SuggestedOrder, 2)
	assert.Equal(t, "shared-lib", plan.SuggestedOrder[0])
}

func TestBuildFeatureRiskHighWhenManyReposOrFunctions(t *testing.T) {
	repos := make([]*ctxpkg.RepoContext, 6)
	repoIDs := make([]string, 6)
	for i := 0; i < 6; i++ {
		id := fmt.Sprintf("repo-%d", i)
		repoIDs[i] = id
		repos[i] = makeTestRepo(id, map[string][]ctxpkg.FunctionDef{
			"f.go": {{Name: "Func", Signature: "func Func()", IsPublic: true,
				Behavior: &ctxpkg.FunctionBehavior{Summary: "does stuff"}}},
		})
	}

	searcher := newMockSearcher(repos...)
	orgResolver := newMockOrgResolver("org", repoIDs)

	plan, err := BuildFeature(context.Background(), BuildFeatureParams{
		OrgID:              "org",
		FeatureDescription: "func stuff",
		Searcher:           searcher,
		OrgResolver:        orgResolver,
	})

	require.NoError(t, err)
	assert.Equal(t, RiskHigh, plan.RiskLevel)
}

func TestBuildFeatureRiskLowSingleRepoFewFunctions(t *testing.T) {
	repo := makeTestRepo("repo-a", map[string][]ctxpkg.FunctionDef{
		"f.go": {
			{Name: "helperA", Signature: "func helperA()", IsPublic: false,
				Behavior: &ctxpkg.FunctionBehavior{Summary: "helps with A"}},
			{Name: "helperB", Signature: "func helperB()", IsPublic: false,
				Behavior: &ctxpkg.FunctionBehavior{Summary: "helps with B"}},
		},
	})

	searcher := newMockSearcher(repo)
	orgResolver := newMockOrgResolver("org", []string{"repo-a"})

	plan, err := BuildFeature(context.Background(), BuildFeatureParams{
		OrgID:              "org",
		FeatureDescription: "helper",
		Searcher:           searcher,
		OrgResolver:        orgResolver,
	})

	require.NoError(t, err)
	assert.Equal(t, RiskLow, plan.RiskLevel)
}

func TestBuildFeatureTargetReposFiltersResults(t *testing.T) {
	repoA := makeTestRepo("repo-a", map[string][]ctxpkg.FunctionDef{
		"a.go": {{Name: "FuncA", Signature: "func FuncA()", IsPublic: true,
			Behavior: &ctxpkg.FunctionBehavior{Summary: "does stuff"}}},
	})
	repoB := makeTestRepo("repo-b", map[string][]ctxpkg.FunctionDef{
		"b.go": {{Name: "FuncB", Signature: "func FuncB()", IsPublic: true,
			Behavior: &ctxpkg.FunctionBehavior{Summary: "does stuff"}}},
	})

	searcher := newMockSearcher(repoA, repoB)
	orgResolver := newMockOrgResolver("org", []string{"repo-a", "repo-b"})

	plan, err := BuildFeature(context.Background(), BuildFeatureParams{
		OrgID:              "org",
		FeatureDescription: "stuff",
		TargetRepos:        []string{"repo-a"},
		Searcher:           searcher,
		OrgResolver:        orgResolver,
	})

	require.NoError(t, err)
	for _, loc := range plan.RelevantCode {
		assert.Equal(t, "repo-a", loc.RepoID)
	}
}

func TestBuildFeatureValidatesMinLength(t *testing.T) {
	searcher := newMockSearcher()
	orgResolver := newMockOrgResolver("org", []string{"repo-a"})

	_, err := BuildFeature(context.Background(), BuildFeatureParams{
		OrgID:              "org",
		FeatureDescription: "",
		Searcher:           searcher,
		OrgResolver:        orgResolver,
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "feature_description must be at least 3 characters")
}

func TestBuildFeatureValidatesTokenBudget(t *testing.T) {
	repo := makeTestRepo("repo-a", map[string][]ctxpkg.FunctionDef{
		"a.go": {{Name: "Func", Signature: "func Func()", IsPublic: true,
			Behavior: &ctxpkg.FunctionBehavior{Summary: "test"}}},
	})

	searcher := newMockSearcher(repo)
	orgResolver := newMockOrgResolver("org", []string{"repo-a"})

	plan, err := BuildFeature(context.Background(), BuildFeatureParams{
		OrgID:              "org",
		FeatureDescription: "test function",
		TokenBudget:        50,
		Searcher:           searcher,
		OrgResolver:        orgResolver,
	})

	require.NoError(t, err)
	require.NotNil(t, plan)
}

func TestBuildFeatureKeywordFallback(t *testing.T) {
	repo := makeTestRepo("repo-a", map[string][]ctxpkg.FunctionDef{
		"user.go": {{Name: "GetUser", Signature: "func GetUser()", IsPublic: true,
			Behavior: &ctxpkg.FunctionBehavior{Summary: "gets user"}}},
	})

	searcher := newMockSearcher(repo)
	orgResolver := newMockOrgResolver("org", []string{"repo-a"})

	plan, err := BuildFeature(context.Background(), BuildFeatureParams{
		OrgID:              "org",
		FeatureDescription: "user management",
		Searcher:           searcher,
		OrgResolver:        orgResolver,
		Vectors:            nil,
	})

	require.NoError(t, err)
	assert.NotEmpty(t, plan.RelevantCode)
	assert.Contains(t, plan.Warnings, "semantic search unavailable, using keyword fallback")
}

func TestBuildFeatureInvalidOrg(t *testing.T) {
	searcher := newMockSearcher()
	orgResolver := newMockOrgResolver("other-org", []string{})

	_, err := BuildFeature(context.Background(), BuildFeatureParams{
		OrgID:              "nonexistent",
		FeatureDescription: "test feature",
		Searcher:           searcher,
		OrgResolver:        orgResolver,
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "nonexistent")
}

func TestBuildFeatureReposNotAnalyzed(t *testing.T) {
	searcher := newMockSearcher() // empty - no repos analyzed
	orgResolver := newMockOrgResolver("org", []string{"repo-x", "repo-y"})

	_, err := BuildFeature(context.Background(), BuildFeatureParams{
		OrgID:              "org",
		FeatureDescription: "test feature",
		Searcher:           searcher,
		OrgResolver:        orgResolver,
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "not analyzed")
}
