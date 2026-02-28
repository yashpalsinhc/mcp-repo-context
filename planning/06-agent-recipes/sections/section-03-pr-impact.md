# Section 03: analyze_pr_impact Recipe

## Overview

Comprehensive PR impact analysis recipe that extends existing `get_pr_context` with cross-repo awareness, AI risk assessment, and structured output with gaps for missing dependencies. Independent steps (cross-service and dependency impact) run concurrently via errgroup.

## Dependencies

- Section 01 (Recipe interface, RecipeRunner, RecipeInput, RecipeResult, RecipeRiskAssessment)
- Section 02 (ContextAssembler for supplementary context)
- Internal: `internal/orchestrator` (Manager.GetPRContext), `internal/ai` (Provider.CompleteRaw)

## Recipe Registration

Register as "analyze_pr_impact" in `DefaultRegistry()`.

## Input Schema

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| repo_id | string | yes | - | Repository ID |
| changed_files | []ChangedFile | yes | - | Files changed in PR |
| org_id | string | no | "" | Org ID for cross-repo analysis |
| token_budget | int | no | 8000 | Max context tokens |
| include_ai | bool | no | true | Include AI risk assessment |

ChangedFile is `{path: string, change_type: string}` — same as existing `orchestrator.ChangedFile`.

## Execution Steps

### Step 1: Single-Repo Impact (always runs)

Call `runner.Manager().GetPRContext(repoID, changedFiles)`. This returns the existing `PRContextResult` with:
- Changed functions with behavior summaries
- Direct callers (who's affected)
- Callees (downstream dependencies)
- DB queries, HTTP calls, side effects
- Affected routes
- Impact analysis

Extract into RecipeResult.Data:
- `changed_functions` — list of changed function summaries
- `affected_callers` — callers of changed functions
- `affected_routes` — API routes affected

Add RecipeSourceRef for each function (file:line).

### Step 2: Cross-Service Impact (parallel with step 3)

Only if `org_id` is provided.

For each HTTP call made by changed functions (from step 1's callees):
- Check if any repo in the org handles that endpoint
- This requires API flow tracing data from 03-api-flow-tracing

**V1:** Flow tracing is not yet available. Always add GapNote:
```
GapNote{
    Section: "cross_service_impact",
    Reason: "Cross-service impact analysis requires 03-api-flow-tracing",
    Suggestion: "Implement 03-api-flow-tracing split for cross-repo endpoint matching",
}
```

Set `Data["cross_service_impact"] = []` (empty array, not nil).

**Future:** When flow tracing is available, replace with real cross-service matching.

### Step 3: Dependency Impact (parallel with step 2)

Only if `org_id` is provided.

Check if the changed repo is a Go module used by other repos in the org. This requires dependency graph data from 02-dependency-graph.

**V1:** Dependency graph is not yet available. Always add GapNote:
```
GapNote{
    Section: "dependency_impact",
    Reason: "Dependency impact analysis requires 02-dependency-graph",
    Suggestion: "Implement 02-dependency-graph split for cross-repo dependency tracking",
}
```

Set `Data["dependency_impact"] = []`.

### Concurrency for Steps 2 & 3

Use `errgroup.Group` with the recipe's context:

```go
g, gctx := errgroup.WithContext(ctx)
g.Go(func() error { /* step 2 */ })
g.Go(func() error { /* step 3 */ })
g.Wait()
```

Each goroutine writes to its own result variable. Results merged after Wait().

### Step 4: Risk Assessment (after steps 1-3)

**With AI (include_ai=true and runner.AI() != nil):**

Build prompt with structural data from steps 1-3. Include: number of changed functions, number of affected callers, types of side effects changed, whether external APIs or DB schemas are affected. Ask AI for risk level (low/medium/high) with reasoning. Use `runner.AI().CompleteRaw(ctx, prompt, 500)`. Limit prompt to 4000 chars.

Set:
```
Data["risk"] = RecipeRiskAssessment{
    Level:      aiResult,   // parsed from AI response
    Reasoning:  aiReasoning,
    Confidence: 0.8,
}
```

**Without AI (include_ai=false or runner.AI() == nil):**

Heuristic scoring:
- `score = len(affectedCallers)*2 + len(externalAPICalls)*5 + len(dbSchemaChanges)*3`
- Level: score < 5 → "low", 5-15 → "medium", >15 → "high"
- Reasoning: "Heuristic: N affected callers, M external API calls, K DB changes"
- Confidence: 0.5

### Step 5: Suggested Reviewers

Extract author information from function metadata if available (git blame data). If not available in the current data model, add GapNote.

**V1:** Git blame data is not stored in function metadata. Add GapNote:
```
GapNote{
    Section: "suggested_reviewers",
    Reason: "Reviewer suggestions require git blame data in function metadata",
    Suggestion: "Add git blame extraction during analysis",
}
```

### Context Cancellation

After each step, check `ctx.Err()`. If cancelled, return partial results with GapNotes for remaining steps.

## Output

Data keys: `changed_functions`, `affected_callers`, `affected_routes`, `cross_service_impact`, `dependency_impact`, `risk`, `suggested_reviewers`.

Analysis: AI-generated narrative summarizing the PR impact (from step 4 prompt, extended).

## Tests

### `internal/recipes/pr_impact_test.go`

**Test: Basic PR with single file change**
- Mock manager.GetPRContext returns 2 changed functions, 3 callers
- Assert Data has changed_functions (2), affected_callers (3)

**Test: PR with no changed functions**
- Mock returns empty PR context
- Assert valid result with empty arrays

**Test: PR with org_id but no flow data**
- Assert GapNote for cross_service_impact

**Test: PR with org_id but no dependency data**
- Assert GapNote for dependency_impact

**Test: AI risk assessment**
- Mock CompleteRaw returns "Risk: medium\nReasoning: 5 callers affected"
- Assert risk.Level="medium", Confidence=0.8

**Test: Heuristic risk when AI unavailable**
- runner AI=nil, mock 10 callers
- Assert risk based on heuristic, Confidence=0.5

**Test: Steps 2 and 3 run concurrently**
- Run with -race flag, verify no data race

**Test: Context cancellation returns partial**
- Cancel after step 1
- Assert step 1 data present, remaining steps have GapNotes

**Test: Input validation rejects missing repo_id**
- Assert error

**Test: Sources populated**
- Assert RecipeSourceRef entries for changed functions

## File Inventory

| File | Purpose |
|------|---------|
| `internal/recipes/pr_impact.go` | analyze_pr_impact recipe implementation |
| `internal/recipes/pr_impact_test.go` | All PR impact tests |

## Acceptance Criteria

1. Single-repo impact uses existing GetPRContext
2. Cross-service and dependency impact have proper GapNotes for v1
3. Steps 2 & 3 run concurrently without data races
4. AI risk assessment works with CompleteRaw
5. Heuristic fallback when AI unavailable
6. Context cancellation returns partial results
7. All 10 tests pass
