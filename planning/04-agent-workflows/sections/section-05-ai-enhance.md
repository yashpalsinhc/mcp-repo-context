# Section 5: AI Enhancement Layer

## Overview

Shared AI enhancement utility used by build_feature and refactor_org to optionally add AI-generated summaries. Includes timeout handling, prompt truncation, and graceful degradation.

## Dependencies

- Sections 2, 3 (build_feature and refactor_org use this layer)

## Tests First

### File: `internal/workflows/ai_enhance_test.go`

```
Test: EnhanceWithAI returns answer string from AskFunc
- Mock AskFunc: return QueryResult{Answer: "test summary"}
- Call EnhanceWithAI(ctx, mockAsk, "prompt")
- Assert result == "test summary"

Test: EnhanceWithAI respects 30s timeout
- Mock AskFunc: sleep 60s then return
- Call EnhanceWithAI with background context
- Assert returns empty string within ~31s (30s timeout + buffer)
- Assert error is context.DeadlineExceeded or nil (depending on design)

Test: EnhanceWithAI truncates prompt to 4000 chars
- Create 10000-char prompt
- Mock AskFunc: capture received query
- Call EnhanceWithAI
- Assert captured query length <= 4000

Test: EnhanceWithAI truncates lists to 10 items
- Call BuildPrompt with 50 entry points
- Assert output contains max 10 entries + "... and 40 more"

Test: EnhanceWithAI handles AskFunc error
- Mock AskFunc: return error
- Call EnhanceWithAI
- Assert returns ("", error)

Test: EnhanceWithAI with nil AskFunc returns empty
- Call EnhanceWithAI(ctx, nil, "prompt")
- Assert returns ("", nil)

Test: aiAvailable false when no AI registry
- Server with no AI configured
- Assert aiAvailable() == false

Test: aiAvailable true when AI configured
- Server with AI registry set
- Assert aiAvailable() == true

Test: BuildFeaturePrompt formats correctly
- Create FeaturePlan with known data
- Call BuildFeaturePrompt(plan)
- Assert contains feature description, function count, entry point names

Test: BuildRefactorPrompt formats correctly
- Create RefactorPlan with known data
- Call BuildRefactorPrompt(plan)
- Assert contains pattern, usage count, impact numbers
```

## Implementation Details

### 1. AskFunc Type

Create `internal/workflows/ai_enhance.go`.

Define the function type that matches the Manager's Ask signature:

```
type AskFunc func(ctx context.Context, query string, repoIDs []string) (*QueryResult, error)
```

Where QueryResult is imported from the orchestrator package (or define a local interface that extracts the answer string).

### 2. EnhanceWithAI Function

```
func EnhanceWithAI(ctx context.Context, askFunc AskFunc, prompt string) (string, error)
```

**Steps:**
1. If askFunc is nil, return ("", nil) — no AI available
2. Truncate prompt to 4000 characters if longer
3. Create timeout context: `ctx, cancel := context.WithTimeout(ctx, 30*time.Second); defer cancel()`
4. Call `result, err := askFunc(ctx, prompt, nil)` (nil repoIDs = search all)
5. If err != nil (including deadline exceeded), return ("", err)
6. Extract answer string from result and return

### 3. Prompt Builders

**BuildFeaturePrompt(plan *FeaturePlan) string**

Template:
```
Given the following analysis for building feature "{plan.Feature}":
- {len(plan.RelevantCode)} relevant functions found across {countDistinctRepos(plan.RelevantCode)} repos
- Entry points: {truncatedList(plan.EntryPoints, 10)}
- Key dependencies: {truncatedList(plan.Dependencies, 10)}

Provide a brief implementation strategy (2-3 paragraphs).
```

**BuildRefactorPrompt(plan *RefactorPlan) string**

Template:
```
The pattern "{plan.Pattern}" appears {len(plan.Usages)} times across {countDistinctRepos(plan.Usages)} repos.
Impact: {plan.ImpactAnalysis.DirectCallers} functions directly affected, {plan.ImpactAnalysis.IndirectCallers} indirect callers.
Hot paths: {truncatedList(plan.ImpactAnalysis.HotPaths, 5)}

Suggest a safe refactoring approach (2-3 paragraphs).
```

### 4. truncatedList Helper

```
func truncatedList(items []T, max int) string
```

Takes any slice (via interface or generics), formats first `max` items as comma-separated names, appends "... and N more" if truncated. Total output capped to keep prompt under 4000 chars.

### 5. aiAvailable Method

Add to the server struct (or wherever workflow handlers live):

```
func (s *server) aiAvailable() bool
```

Returns true if the AI registry / Ask functionality is configured. Check if the manager's AI-related methods are functional (e.g., if the anthropic API key is set). If the Manager has a method to check AI availability, use it. Otherwise, check if calling Ask with a trivial query would succeed (cache the result for the session).

### 6. Integration with Workflow Tools

In build_feature handler, after BuildFeature returns the plan:
```
if s.aiAvailable() {
    prompt := workflows.BuildFeaturePrompt(plan)
    enhancement, err := workflows.EnhanceWithAI(ctx, s.manager.Ask, prompt)
    if err != nil {
        log.Printf("AI enhancement failed for build_feature: %v", err)
    } else {
        plan.AIEnhancement = enhancement
    }
}
```

Same pattern for refactor_org with BuildRefactorPrompt.

## Error Handling

- AskFunc nil: return empty, no error (not a failure)
- AskFunc error: return error to caller, caller logs and continues without AI
- Timeout: context.DeadlineExceeded returned, caller treats same as error
- Prompt too long: silently truncated, never fails

## File Summary

| File | Action |
|------|--------|
| `internal/workflows/ai_enhance.go` | New: EnhanceWithAI, prompt builders, truncation |
| `internal/workflows/ai_enhance_test.go` | New: tests for AI layer |
| `internal/workflows/build_feature.go` | Modify: add AI enhancement call |
| `internal/workflows/refactor_org.go` | Modify: add AI enhancement call |
