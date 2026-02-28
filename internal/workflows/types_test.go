package workflows

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRiskLevelConstants(t *testing.T) {
	assert.Equal(t, RiskLevel("low"), RiskLow)
	assert.Equal(t, RiskLevel("medium"), RiskMedium)
	assert.Equal(t, RiskLevel("high"), RiskHigh)
}

func TestFormatFeaturePlanRespectsBudget(t *testing.T) {
	plan := &FeaturePlan{
		Feature: "user management",
		OrgID:   "test-org",
	}
	for i := 0; i < 50; i++ {
		plan.RelevantCode = append(plan.RelevantCode, CodeLocation{
			RepoID: "repo-a", FilePath: "file.go", FuncName: "Func" + strings.Repeat("X", 20),
			Score: float64(50-i) / 50.0, Summary: "Does something important with users",
		})
	}
	for i := 0; i < 20; i++ {
		plan.Dependencies = append(plan.Dependencies, Dependency{
			SourceRepoID: "repo-a", SourceFunc: "FuncA", TargetRepoID: "repo-b", TargetFunc: "FuncB",
			Type: "name_match", Confidence: "medium",
		})
	}

	output := FormatFeaturePlan(plan, 2000)
	assert.LessOrEqual(t, len(output), 2000*4+100) // small buffer for truncation text
	assert.Contains(t, output, "Relevant Code")
	assert.Contains(t, output, "Entry Points")
	assert.Contains(t, output, "Dependencies")
}

func TestFormatFeaturePlanBudgetAllocation(t *testing.T) {
	plan := &FeaturePlan{
		Feature:      "user management",
		OrgID:        "test-org",
		RiskLevel:    RiskMedium,
		EntryPoints:  []EntryPoint{{RepoID: "r", FilePath: "f.go", FuncName: "Handler", Why: "public"}},
		Dependencies: []Dependency{{SourceRepoID: "a", SourceFunc: "X", TargetRepoID: "b", TargetFunc: "Y", Type: "name_match", Confidence: "medium"}},
		FilesToTouch: []FileAction{{RepoID: "r", FilePath: "f.go", Action: "modify", Reason: "contains"}},
	}
	for i := 0; i < 30; i++ {
		plan.RelevantCode = append(plan.RelevantCode, CodeLocation{
			RepoID: "repo-a", FilePath: "file.go", FuncName: "LongFunctionName",
			Score: 0.9, Summary: "A summary of the function behavior",
		})
	}

	output := FormatFeaturePlan(plan, 4000)

	// Relevant code section should exist and be the largest content section
	codeIdx := strings.Index(output, "## Relevant Code")
	entryIdx := strings.Index(output, "## Entry Points")
	require.Greater(t, codeIdx, -1)
	require.Greater(t, entryIdx, -1)

	codeSection := output[codeIdx:entryIdx]
	// The code section should be substantial
	assert.Greater(t, len(codeSection), 200)
}

func TestFormatRefactorPlanTruncatesWithIndicator(t *testing.T) {
	plan := &RefactorPlan{
		Pattern:   "validation",
		OrgID:     "test-org",
		RiskLevel: RiskLow,
	}
	for i := 0; i < 30; i++ {
		plan.Usages = append(plan.Usages, CodeLocation{
			RepoID: "repo-a", FilePath: "file.go", FuncName: "ValidateStuff",
			Score: 0.8, Summary: "Validates the stuff thoroughly",
		})
	}

	output := FormatRefactorPlan(plan, 1000)
	assert.Contains(t, output, "... and")
	assert.Contains(t, output, "more")
}

func TestFormatMergeReportIncludesAllSections(t *testing.T) {
	report := &MergeReport{
		SourceRepos: []string{"repo-a", "repo-b"},
		TargetRepo:  "target",
		Duplicates:  []DuplicateSummary{{FunctionName: "Func1", Repos: []string{"a", "b"}, Similarity: 1.0, Recommendation: "keep target"}},
		Conflicts:   []ConflictSummary{{FunctionName: "Func2", Type: "signature_mismatch", Severity: "high", SourceRepo: "a", Resolution: "reconcile"}},
		Gaps:        []GapSummary{{FunctionName: "Func3", SourceRepo: "a", Priority: "high", Description: "missing"}},
		MergeOrder:  []MergeStep{{Order: 1, Action: "migrate", SourceRepo: "a", TargetItem: "Func3", Description: "migrate function", Risk: RiskLow}},
		RiskLevel:   RiskHigh,
	}

	output := FormatMergeReport(report, 8000)
	assert.Contains(t, output, "Duplicates")
	assert.Contains(t, output, "Conflicts")
	assert.Contains(t, output, "Gaps")
	assert.Contains(t, output, "Merge Steps")
	assert.Contains(t, output, "Risk")
}

func TestFormatFeaturePlanEmptyProducesValidMarkdown(t *testing.T) {
	plan := &FeaturePlan{
		Feature:   "empty feature",
		OrgID:     "org",
		RiskLevel: RiskLow,
	}

	output := FormatFeaturePlan(plan, 4000)
	assert.Contains(t, output, "# Feature Plan")
	assert.Contains(t, output, "No relevant code found")
}

func TestCodeLocationSortByScoreDescending(t *testing.T) {
	locations := []CodeLocation{
		{FuncName: "A", Score: 0.3},
		{FuncName: "B", Score: 0.9},
		{FuncName: "C", Score: 0.5},
	}

	SortByScore(locations)

	assert.Equal(t, "B", locations[0].FuncName)
	assert.Equal(t, "C", locations[1].FuncName)
	assert.Equal(t, "A", locations[2].FuncName)
}

func TestFormatFeaturePlanSummaryOnlyForSmallBudget(t *testing.T) {
	plan := &FeaturePlan{
		Feature:      "test",
		OrgID:        "org",
		RelevantCode: []CodeLocation{{FuncName: "A"}},
		EntryPoints:  []EntryPoint{{FuncName: "B"}},
		Dependencies: []Dependency{{SourceFunc: "C"}},
		RiskLevel:    RiskLow,
	}

	output := FormatFeaturePlan(plan, 100)
	// Should be summary only, not full markdown
	assert.Contains(t, output, "Relevant code: 1")
	assert.Contains(t, output, "Entry points: 1")
}
