# Section 4: merge_repos Tool

## Overview

Implement the `merge_repos` MCP tool that generates a merge strategy report for consolidating repositories. Orchestrates FindDuplicates, FindConflicts, FindGaps, and AnalyzeConsistency to produce an advisory report with suggested merge order.

## Dependencies

- Section 1 (shared types and formatting)

## Tests First

### File: `internal/workflows/merge_repos_test.go`

```
Test: MergeRepos combines all comparison analyses
- Setup: 2 source repos and 1 target with overlapping functions
- Call MergeRepos
- Assert Duplicates populated (overlapping functions)
- Assert Conflicts populated (signature mismatches)
- Assert Gaps populated (functions in source not in target)

Test: MergeRepos merge order dependencies first
- Setup: repo-A has types used by repo-B functions
- Call MergeRepos with sources=[A, B]
- Assert repo-A steps appear before repo-B in MergeOrder

Test: MergeRepos merge order types before functions within repo
- Call MergeRepos
- Within MergeSteps for a single repo, assert type items come before function items

Test: MergeRepos circular dependency alphabetical fallback
- Setup: repo-A imports from repo-B, repo-B imports from repo-A
- Call MergeRepos
- Assert both in MergeOrder
- Assert output contains "circular dependency" warning

Test: MergeRepos risk high with severe conflicts
- Setup: source with high-severity signature_mismatch conflict
- Assert RiskLevel == RiskHigh

Test: MergeRepos risk high with >10 total conflicts
- Setup: 11 conflicts all medium severity
- Assert RiskLevel == RiskHigh

Test: MergeRepos risk low with no conflicts few gaps
- Setup: no overlapping functions, 2 gaps
- Assert RiskLevel == RiskLow

Test: MergeRepos target in sources removed with warning
- Call with source_repo_ids=["A", "B"], target_repo_id="A"
- Assert "A" NOT in effective sources
- Assert output contains warning about target in sources

Test: MergeRepos validates min 1 source
- Call with source_repo_ids=[]
- Assert error "at least one source repository required"

Test: MergeRepos single source repo works
- Setup: 1 source, 1 target
- Call MergeRepos
- Assert valid report with no errors

Test: MergeRepos advisory report has all sections
- Call MergeRepos
- Assert formatted output contains: "Duplicates", "Conflicts", "Gaps", "Merge Steps", "Risk"

Test: MergeRepos token budget truncates
- Setup: many duplicates/conflicts/gaps
- Call with token_budget=2000
- Assert output character count reasonable
```

## Implementation Details

### 1. Workflow Function

Create `internal/workflows/merge_repos.go`.

**Primary function:**

```
func MergeRepos(ctx context.Context, params MergeReposParams) (*MergeReport, error)
```

**MergeReposParams struct:**
- SourceRepoIDs ([]string, required, min 1)
- TargetRepoID (string, required)
- TokenBudget (int, default 8000, range 500-32000)
- Manager — orchestrator.Manager reference
- Comparer — comparison.Comparer reference

### 2. Input Validation

- SourceRepoIDs must have >= 1 entry
- If TargetRepoID is in SourceRepoIDs, remove it and set a warning flag
- Validate all repo IDs have loaded contexts via Manager
- Clamp TokenBudget to 500-32000

### 3. Load Contexts

Load RepoContext for each source and the target via Manager. This is the same approach as the existing compare_repos tool handler.

### 4. Run Comparison Analyses

Call all four comparison methods:
- `comparer.FindDuplicates(ctx, allContexts)` — returns []DuplicateGroup
- `comparer.FindConflicts(ctx, sourceContexts, targetContext)` — returns []Conflict
- `comparer.FindGaps(ctx, sourceContexts, targetContext)` — returns []Gap
- `comparer.AnalyzeConsistency(ctx, allContexts)` — returns ConsistencyReport

Convert each to the workflow summary types:
- DuplicateGroup -> DuplicateSummary: extract function name, repos, similarity, generate recommendation ("keep target" if target has it, else "keep most complete")
- Conflict -> ConflictSummary: extract function name, type, severity, source repo, generate resolution suggestion
- Gap -> GapSummary: extract function name, source repo, priority, description

### 5. Generate Merge Order

**Step 1: Determine repo order (topological sort with cycle handling)**

Build a dependency graph among source repos. Dependencies can be inferred from:
- Function name overlap (if repo-A has function X that repo-B's functions call, repo-A is a dependency of repo-B)
- Gap analysis (if repo-A has something repo-B needs, repo-A should come first)

Attempt topological sort. If cycle detected:
- Break cycle by sorting tied repos alphabetically
- Add warning to report: "Circular dependency detected between {repos}; ordering alphabetically"

**Step 2: Within each repo, order items**

For each source repo, create MergeSteps:
1. Types/interfaces first (Action: "migrate", Risk based on conflict existence)
2. Utility functions (functions with no HTTP/handler concept)
3. Business logic / handlers last

Each MergeStep gets:
- Action: "migrate" for gaps, "resolve_conflict" for conflicts, "consolidate_duplicate" for duplicates
- Order: sequential within the overall list
- Description: human-readable action description

### 6. Risk Assessment

- highConflicts = count of conflicts with severity "high"
- totalConflicts = total conflict count
- publicAPIChanges = count of conflicts involving exported functions
- If highConflicts > 0 OR totalConflicts > 10 -> RiskHigh
- If totalConflicts > 0 and all medium, OR gaps > 20 -> RiskMedium
- Else -> RiskLow

### 7. MCP Tool Handler

Register tool:
- Name: "merge_repos"
- Description: "Generate a merge strategy report for consolidating repositories"
- Parameters: source_repo_ids (required array), target_repo_id (required string), token_budget (optional int)

Handler calls MergeRepos, then FormatMergeReport.

## Error Handling

- Repo context not loaded: return error with list of repos that need analysis
- Comparison method fails: if FindDuplicates fails, continue with empty duplicates; same for others. Only fail if ALL methods fail.
- Target in sources: silently remove, add note to report

## File Summary

| File | Action |
|------|--------|
| `internal/workflows/merge_repos.go` | New: MergeRepos function |
| `internal/workflows/merge_repos_test.go` | New: unit tests |
| `cmd/mcp-repo-context/tools.go` | Modify: register merge_repos tool |
