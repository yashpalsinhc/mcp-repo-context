# Section 3: refactor_org Tool

## Overview

Implement the `refactor_org` MCP tool that plans a refactoring across repositories in an organization. Finds pattern usages via semantic + concept search, performs impact analysis via call graph, and assesses risk.

## Dependencies

- Section 1 (shared types and formatting)

## Tests First

### File: `internal/workflows/refactor_org_test.go`

```
Test: RefactorOrg finds usages via semantic search
- Setup: org with 2 repos, both containing "authentication" functions
- Call RefactorOrg("authentication pattern")
- Assert Usages contains auth functions from both repos

Test: RefactorOrg merges concept and semantic results without duplicates
- Setup: repo with function that matches both concept "authentication" and semantic query
- Call RefactorOrg("authentication")
- Assert function appears exactly once in Usages (deduplicated)

Test: RefactorOrg groups usages by repo
- Setup: org with 3 repos, each having pattern usages
- Call RefactorOrg
- Verify usages from each repo are present
- Assert grouping by repoID is correct

Test: RefactorOrg impact analysis counts callers
- Setup: funcB matches pattern, funcA and funcC both call funcB
- Call RefactorOrg
- Assert ImpactAnalysis.DirectCallers >= 2

Test: RefactorOrg identifies hot paths
- Setup: function with 6 callers (>5 threshold)
- Call RefactorOrg
- Assert ImpactAnalysis.HotPaths includes that function name

Test: RefactorOrg risk high when >5 repos affected
- Setup: org with 6 repos, pattern in all
- Assert RiskLevel == RiskHigh

Test: RefactorOrg risk low when single repo <5 functions
- Setup: 1 repo, 2 pattern usages
- Assert RiskLevel == RiskLow

Test: RefactorOrg affected files includes caller files as "review"
- Setup: funcA in file1 matches, funcB in file2 calls funcA
- Assert file1 has action "modify"
- Assert file2 has action "review"

Test: RefactorOrg validates pattern_description min length
- Call with pattern_description="ab"
- Assert validation error

Test: RefactorOrg target_repos filters results
- Setup: org with repos A, B, C
- Call with target_repos=["B"]
- Assert all Usages are from repo B only

Test: RefactorOrg keyword fallback without vectors
- Setup: repos analyzed but not indexed
- Call RefactorOrg
- Assert results returned (keyword-based)
```

## Implementation Details

### 1. Workflow Function

Create `internal/workflows/refactor_org.go`.

**Primary function:**

```
func RefactorOrg(ctx context.Context, params RefactorOrgParams) (*RefactorPlan, error)
```

**RefactorOrgParams struct:**
- OrgID (string, required)
- PatternDescription (string, required, min 3 chars)
- TargetRepos ([]string, optional)
- TokenBudget (int, default 4000, range 500-32000)
- Manager — orchestrator.Manager reference
- SemanticSearch — vectors.SemanticSearch reference

### 2. Input Validation

Same pattern as build_feature: validate PatternDescription length >= 3, clamp TokenBudget, validate org exists and has analyzed repos.

### 3. Find Pattern Usages

**Two search paths, merged:**

a) **Semantic search:** SearchByOrg(ctx, orgID, patternDescription, limit=30). Convert results to []CodeLocation.

b) **Concept search:** Check if patternDescription maps to a known concept. Known concepts include: "authentication", "validation", "handler", "crud", "middleware", "error", "database", "http", "logging". If pattern contains one of these keywords, iterate each repo and call SearchByConcept(repoID, concept). Convert results to []CodeLocation.

**Merge and deduplicate:** Combine both result sets. Deduplicate by key (repoID + filePath + funcName). For duplicates, keep the higher score. Post-filter by TargetRepos if specified. Cap at 30 results.

### 4. Group by Repo

Build map: repoID -> []CodeLocation. Identify hotspot repos (most usages). Store in plan for display.

### 5. Impact Analysis

For each usage (up to top 20):
- Get callers via Manager.GetCallers(repoID, funcName)
- Count direct callers
- For each direct caller, get its callers (1 more level) for indirect count
- Track which repos have callers (AffectedRepos)
- If a function has >5 direct callers, add to HotPaths

**Cross-repo impact (best-effort):** For each caller function name, check if it appears in other repos' contexts. This is name-based matching, not guaranteed accurate.

Build ImpactSummary with totals.

### 6. Affected Files

- Files containing pattern usages -> FileAction{Action: "modify", Reason: "Contains pattern usage"}
- Files containing direct callers of usages -> FileAction{Action: "review", Reason: "Calls affected function"}
- Deduplicate by (repoID, filePath), prefer "modify" over "review" if same file

### 7. Risk Assessment

- repoCount = len(ImpactAnalysis.AffectedRepos)
- funcCount = DirectCallers + len(Usages)
- hotPathCount = len(HotPaths)
- If repoCount > 5 OR funcCount > 20 -> RiskHigh
- If repoCount 2-5 OR funcCount 5-20 -> RiskMedium
- Else -> RiskLow

### 8. MCP Tool Handler

Register tool:
- Name: "refactor_org"
- Description: "Plan a refactoring across repositories in an organization"
- Parameters: org_id (required), pattern_description (required), target_repos (optional), token_budget (optional)

Handler calls RefactorOrg, then FormatRefactorPlan.

## Error Handling

- Same org/repo validation errors as build_feature
- Concept search failure on individual repo: skip that repo, continue with others
- Call graph unavailable: skip impact analysis for that repo, note in output

## File Summary

| File | Action |
|------|--------|
| `internal/workflows/refactor_org.go` | New: RefactorOrg function |
| `internal/workflows/refactor_org_test.go` | New: unit tests |
| `cmd/mcp-repo-context/tools.go` | Modify: register refactor_org tool |
