# Section 1: Workflow Types & Shared Infrastructure

## Overview

Define the shared types and formatting utilities used by all three workflow tools (build_feature, refactor_org, merge_repos). New package `internal/workflows/`.

## Dependencies

None — this is the foundation section.

## Tests First

### File: `internal/workflows/types_test.go`

```
Test: RiskLevel constants are valid strings
- Assert RiskLow == "low", RiskMedium == "medium", RiskHigh == "high"

Test: FormatFeaturePlan respects token budget
- Create FeaturePlan with 50 CodeLocations, 20 Dependencies
- Format with budget=2000
- Assert output character count <= budget * 4
- Assert output contains headers: "Relevant Code", "Entry Points", "Dependencies"

Test: FormatFeaturePlan budget allocation ~40/30/20/10
- Create FeaturePlan with data in all sections
- Format with budget=4000
- Measure each section's character count
- Assert relevant code section is largest (~40%)

Test: FormatRefactorPlan truncates with indicator
- Create RefactorPlan with 30 usages
- Format with budget=1000
- Assert output contains "... and N more"

Test: FormatMergeReport includes all sections
- Create MergeReport with duplicates, conflicts, gaps, merge steps
- Format with budget=8000
- Assert sections: "Duplicates", "Conflicts", "Gaps", "Merge Steps", "Risk"

Test: FormatFeaturePlan empty plan produces valid markdown
- Create empty FeaturePlan
- Format
- Assert valid markdown with "No relevant code found" or similar

Test: CodeLocation sorting by score descending
- Create slice with scores [0.3, 0.9, 0.5]
- Sort
- Assert order: 0.9, 0.5, 0.3
```

## Implementation Details

### 1. New Package: `internal/workflows/`

Create `internal/workflows/types.go` with all shared types.

### 2. RiskLevel Type

Define as a typed string constant to prevent typos:

```go
type RiskLevel string

const (
    RiskLow    RiskLevel = "low"
    RiskMedium RiskLevel = "medium"
    RiskHigh   RiskLevel = "high"
)
```

### 3. Core Types

**FeaturePlan** — result of build_feature workflow:
- Feature (string): the feature description query
- OrgID (string): org context
- RelevantCode ([]CodeLocation): semantic search results, sorted by score
- EntryPoints ([]EntryPoint): public functions with few callers, handlers
- Dependencies ([]Dependency): cross-repo name-based matches
- FilesToTouch ([]FileAction): files needing modification/creation/review
- SuggestedOrder ([]string): repo-level implementation order
- RiskLevel (RiskLevel): computed risk
- AIEnhancement (string): optional AI summary, empty if unavailable

**RefactorPlan** — result of refactor_org workflow:
- Pattern (string): pattern being refactored
- OrgID (string)
- Usages ([]CodeLocation): where pattern found
- AffectedFiles ([]FileAction)
- ImpactAnalysis (ImpactSummary): caller counts, hot paths
- RiskLevel (RiskLevel)
- AIEnhancement (string)

**MergeReport** — result of merge_repos workflow:
- SourceRepos ([]string)
- TargetRepo (string)
- Duplicates ([]DuplicateSummary)
- Conflicts ([]ConflictSummary)
- Gaps ([]GapSummary)
- MergeOrder ([]MergeStep)
- RiskLevel (RiskLevel)

### 4. Supporting Types

**CodeLocation**: RepoID, FilePath, FuncName, Line (int), Summary, Score (float64)

**FileAction**: RepoID, FilePath, Action (string: "modify"/"create"/"review"), Reason

**EntryPoint**: RepoID, FilePath, FuncName, Why (string explanation)

**Dependency**: SourceRepoID, SourceFunc, TargetRepoID, TargetFunc, Type (string: "name_match"/"module_import"), Confidence (string: "high"/"medium")

**ImpactSummary**: DirectCallers (int), IndirectCallers (int), AffectedRepos ([]string), HotPaths ([]string — functions with >5 callers)

**DuplicateSummary**: FunctionName, Repos ([]string), Similarity (float64), Recommendation (string)

**ConflictSummary**: FunctionName, Type, Severity, SourceRepo, Resolution

**GapSummary**: FunctionName, SourceRepo, Priority, Description

**MergeStep**: Order (int), Action (string: "migrate"/"resolve_conflict"/"fill_gap"/"consolidate_duplicate"), SourceRepo, TargetItem, Description, Risk (RiskLevel)

### 5. Formatting Utilities

Create `internal/workflows/format.go`.

**FormatFeaturePlan(plan *FeaturePlan, budget int) string**

Produces markdown with sections:
1. Header with feature description and org
2. "## Relevant Code" — list CodeLocations with repo, file, function, summary
3. "## Entry Points" — list with why explanation
4. "## Dependencies" — list cross-repo dependencies
5. "## Files to Touch" — grouped by action type
6. "## Implementation Order" — numbered list
7. "## Risk Assessment" — risk level with explanation
8. "## AI Analysis" — if AIEnhancement non-empty

**Budget allocation strategy:**
- Convert budget to approximate character limit (budget * 4)
- Allocate: 40% relevant code, 30% dependencies, 20% files/order, 10% risk/AI
- For each section, wrap items as ScoredItem using the existing `internal/tokens` budgeter
- CodeLocation items scored by their Score field
- FileAction items scored: "modify" = 1.0, "create" = 0.8, "review" = 0.5
- If section exceeds allocation, truncate with "... and N more items"

**FormatRefactorPlan(plan *RefactorPlan, budget int) string** — same pattern, sections: Usages, Impact Analysis, Affected Files, Risk, AI Analysis

**FormatMergeReport(report *MergeReport, budget int) string** — sections: Duplicates, Conflicts, Gaps, Merge Steps, Risk. Default budget 8000 (larger than other tools).

### 6. Helper: SortByScore

Utility to sort []CodeLocation by Score descending using sort.Slice.

## Error Handling

- Empty plan/report: produce valid markdown with "no results" messages per section
- Budget < 500: produce summary-only output (just headers and counts, no details)

## File Summary

| File | Action |
|------|--------|
| `internal/workflows/types.go` | New: all shared types |
| `internal/workflows/format.go` | New: formatting utilities with budget allocation |
| `internal/workflows/types_test.go` | New: type and formatting tests |
