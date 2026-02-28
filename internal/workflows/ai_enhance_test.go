package workflows

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEnhanceWithAIReturnsAnswer(t *testing.T) {
	mockAsk := func(ctx context.Context, query string, repoIDs []string) (string, error) {
		return "test summary", nil
	}

	result, err := EnhanceWithAI(context.Background(), mockAsk, "prompt")
	require.NoError(t, err)
	assert.Equal(t, "test summary", result)
}

func TestEnhanceWithAINilAskFuncReturnsEmpty(t *testing.T) {
	result, err := EnhanceWithAI(context.Background(), nil, "prompt")
	require.NoError(t, err)
	assert.Equal(t, "", result)
}

func TestEnhanceWithAITruncatesPromptTo4000(t *testing.T) {
	longPrompt := strings.Repeat("x", 10000)
	var capturedQuery string

	mockAsk := func(ctx context.Context, query string, repoIDs []string) (string, error) {
		capturedQuery = query
		return "ok", nil
	}

	_, err := EnhanceWithAI(context.Background(), mockAsk, longPrompt)
	require.NoError(t, err)
	assert.LessOrEqual(t, len(capturedQuery), 4000)
}

func TestEnhanceWithAIHandlesError(t *testing.T) {
	mockAsk := func(ctx context.Context, query string, repoIDs []string) (string, error) {
		return "", fmt.Errorf("ai service unavailable")
	}

	result, err := EnhanceWithAI(context.Background(), mockAsk, "prompt")
	require.Error(t, err)
	assert.Equal(t, "", result)
	assert.Contains(t, err.Error(), "ai service unavailable")
}

func TestEnhanceWithAIPassesNilRepoIDs(t *testing.T) {
	var capturedRepoIDs []string
	mockAsk := func(ctx context.Context, query string, repoIDs []string) (string, error) {
		capturedRepoIDs = repoIDs
		return "ok", nil
	}

	_, err := EnhanceWithAI(context.Background(), mockAsk, "prompt")
	require.NoError(t, err)
	assert.Nil(t, capturedRepoIDs)
}

func TestBuildFeaturePromptFormatsCorrectly(t *testing.T) {
	plan := &FeaturePlan{
		Feature: "user management",
		RelevantCode: []CodeLocation{
			{RepoID: "repo-a", FuncName: "GetUser"},
			{RepoID: "repo-a", FuncName: "CreateUser"},
			{RepoID: "repo-b", FuncName: "ListUsers"},
		},
		EntryPoints: []EntryPoint{
			{FuncName: "GetUser"},
			{FuncName: "CreateUser"},
		},
		Dependencies: []Dependency{
			{SourceRepoID: "repo-a", TargetRepoID: "repo-b", TargetFunc: "ListUsers"},
		},
	}

	prompt := BuildFeaturePrompt(plan)
	assert.Contains(t, prompt, "user management")
	assert.Contains(t, prompt, "3 relevant functions")
	assert.Contains(t, prompt, "GetUser")
	assert.Contains(t, prompt, "CreateUser")
	assert.Contains(t, prompt, "repo-a->repo-b")
	assert.Contains(t, prompt, "implementation strategy")
}

func TestBuildFeaturePromptTruncatesEntryPoints(t *testing.T) {
	plan := &FeaturePlan{
		Feature: "test",
	}
	for i := 0; i < 15; i++ {
		plan.EntryPoints = append(plan.EntryPoints, EntryPoint{
			FuncName: fmt.Sprintf("Func%d", i),
		})
	}

	prompt := BuildFeaturePrompt(plan)
	assert.Contains(t, prompt, "... and 5 more")
}

func TestBuildRefactorPromptFormatsCorrectly(t *testing.T) {
	plan := &RefactorPlan{
		Pattern: "validation pattern",
		Usages: []CodeLocation{
			{RepoID: "repo-a", FuncName: "ValidateA"},
			{RepoID: "repo-b", FuncName: "ValidateB"},
		},
		ImpactAnalysis: ImpactSummary{
			DirectCallers:   5,
			IndirectCallers: 10,
			HotPaths:        []string{"ValidateA"},
		},
	}

	prompt := BuildRefactorPrompt(plan)
	assert.Contains(t, prompt, "validation pattern")
	assert.Contains(t, prompt, "2 times")
	assert.Contains(t, prompt, "5 functions directly affected")
	assert.Contains(t, prompt, "10 indirect callers")
	assert.Contains(t, prompt, "ValidateA")
	assert.Contains(t, prompt, "refactoring approach")
}

func TestBuildRefactorPromptTruncatesHotPaths(t *testing.T) {
	plan := &RefactorPlan{
		Pattern: "test",
		ImpactAnalysis: ImpactSummary{},
	}
	for i := 0; i < 8; i++ {
		plan.ImpactAnalysis.HotPaths = append(plan.ImpactAnalysis.HotPaths, fmt.Sprintf("HotFunc%d", i))
	}

	prompt := BuildRefactorPrompt(plan)
	assert.Contains(t, prompt, "... and 3 more")
}

func TestTruncateItemsEmpty(t *testing.T) {
	result := truncateItems(nil, 5)
	assert.Equal(t, "none", result)
}

func TestTruncateItemsWithinLimit(t *testing.T) {
	items := []string{"a", "b", "c"}
	result := truncateItems(items, 5)
	assert.Equal(t, "a, b, c", result)
}

func TestTruncateItemsOverLimit(t *testing.T) {
	items := []string{"a", "b", "c", "d", "e"}
	result := truncateItems(items, 3)
	assert.Contains(t, result, "a, b, c")
	assert.Contains(t, result, "... and 2 more")
}
