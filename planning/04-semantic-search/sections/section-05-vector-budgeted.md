# Section 5: Vector-Ranked get_context_budgeted

## Overview

Enhance get_context_budgeted to use vector similarity scores for ranking when the repo has been indexed. Falls back to keyword-based scoring when not indexed. Loads per-repo vocabulary for correct query embedding.

## Dependencies

- Section 1: IsAvailable()
- Section 4: Vocabulary loading for search (ensureVocabulary)

## Tests First

### File: `internal/mcp/tool_budgeted_vector_test.go` (new)

```
Test: Indexed repo uses vector ranking
- Setup: analyze and index a repo with 10 functions
- Call: get_context_budgeted(repo_id, query="user authentication", budget=4000)
- Assert: results are ranked by semantic similarity
- Assert: functions semantically related to "user authentication" appear first
- Assert: output does NOT contain "Tip: Run index_repository"

Test: Unindexed repo falls back to keyword ranking
- Setup: analyze repo but do NOT index
- Call: get_context_budgeted(repo_id, query="user")
- Assert: results ranked by keyword matches
- Assert: output contains "Tip: Run index_repository to enable semantic ranking"

Test: Vector ranking loads correct vocabulary
- Setup: index repo A (user functions) and repo B (order functions)
- Call: get_context_budgeted for repo B with query="order processing"
- Assert: repo B functions ranked highly (not repo A functions)
- Assert: correct vocabulary loaded for repo B

Test: Higher similarity = higher rank
- Setup: index repo with GetUser (very relevant to "user"), CreateOrder (less relevant)
- Call: get_context_budgeted(query="user management")
- Assert: GetUser appears before CreateOrder in results

Test: Functions not in semantic results get score 0
- Setup: index repo, search returns 5 of 10 functions
- Call: get_context_budgeted
- Assert: 5 matched functions have positive scores
- Assert: remaining 5 have score 0 (included only if budget allows)

Test: Vector ranking with small budget uses summaries
- Setup: index repo with 20 functions
- Call: get_context_budgeted(budget=2000)
- Assert: results use summarized format (not full details)
- Assert: fits within ~2000 tokens

Test: Vector ranking with large budget includes full details
- Setup: index repo
- Call: get_context_budgeted(budget=8000)
- Assert: top functions include callers, callees, side effects

Test: Semantic search failure falls back to keywords
- Setup: index repo, then corrupt vector store (simulate search error)
- Call: get_context_budgeted
- Assert: falls back to keyword ranking
- Assert: warning logged about semantic search failure

Test: Empty query returns all functions by keyword score
- Setup: index repo
- Call: get_context_budgeted(query="")
- Assert: uses keyword ranking (empty query can't do semantic search)
```

## Implementation Details

### 1. Modify toolGetContextBudgeted

Current location: `internal/mcp/tools.go` (lines ~2483-2586)

Add a branching point after extracting parameters:

```go
func (s *server) toolGetContextBudgeted(ctx context.Context, args map[string]any) callToolResult {
    repoID := extractString(args, "repo_id")
    query := extractString(args, "query")
    budget := extractFloat(args, "token_budget", 4000)

    repoCtx := s.manager.GetContext(ctx, repoID)
    if repoCtx == nil {
        return errorResult("Repository not found")
    }

    var scoredFunctions []tokens.ScoredItem[storage.FunctionDetail]
    var usingVectors bool

    // Try vector ranking first
    if s.semanticSearch.IsAvailable() && query != "" {
        scored, err := s.vectorScoreFunctions(ctx, repoID, query, repoCtx)
        if err == nil && len(scored) > 0 {
            scoredFunctions = scored
            usingVectors = true
        }
    }

    // Fall back to keyword ranking
    if !usingVectors {
        scoredFunctions = s.keywordScoreFunctions(query, repoCtx)
    }

    // Apply budget (existing logic)
    selected := s.budgeter.BuildFunctionContext(scoredFunctions, int(budget))

    // Format output
    output := formatBudgetedContext(selected, usingVectors, budget)
    if !usingVectors {
        output += "\n\nTip: Run `index_repository(repo_id: \"" + repoID + "\")` to enable semantic ranking for better results."
    }

    return textResult(output)
}
```

### 2. vectorScoreFunctions

```go
func (s *server) vectorScoreFunctions(ctx context.Context, repoID, query string, repoCtx *storage.RepoContext) ([]tokens.ScoredItem[storage.FunctionDetail], error)
```

Steps:
1. Check if repo has vectors: `count, _ := s.semanticSearch.Count(ctx, repoID)`. If 0, return nil.
2. Ensure vocabulary loaded for repo: `s.semanticSearch.ensureVocabulary(ctx, repoID)` (from Section 4)
3. Run semantic search: `results, err := s.semanticSearch.SearchFunctions(ctx, query, repoID, 100)` (get up to 100 ranked results)
4. Build a map of function name → similarity score from results
5. For each function in repoCtx:
   - Look up score in map. If found, use similarity as Score.
   - If not in results, assign Score = 0
   - Calculate TokenCost using counter
   - Append to scoredFunctions
6. Return scored list (budgeter will sort and fill)

### 3. keywordScoreFunctions (Extract Existing Logic)

Extract the existing keyword scoring logic from toolGetContextBudgeted into a separate method:

```go
func (s *server) keywordScoreFunctions(query string, repoCtx *storage.RepoContext) []tokens.ScoredItem[storage.FunctionDetail]
```

This is a refactor of existing code — no behavior change. The existing `extractSearchKeywords` and `scoreFunctionRelevance` functions are reused.

### 4. Score Handling

Cosine similarity range is [-1, 1], but SearchFunctions already filters to similarity > 0. So effective scores are (0, 1]. Functions not in semantic results get 0.

The budgeter sorts by score descending, so vector-scored functions with higher similarity appear first. Functions with score 0 appear last (filler if budget allows).

### 5. Tiered Budget Documentation

Update the tool description (in tool registration) with budget tier guidance:

```
token_budget: Maximum tokens for the context.
  Recommended tiers:
  - 2000: Quick lookup — function signatures and one-line summaries
  - 4000: Standard — behavior summaries and side effects (default)
  - 8000: Thorough — full details including callers, callees, DB queries
```

No code change needed for tier behavior — the existing budgeter handles this via SummarizeFunction fallback.

## Error Handling

- Semantic search fails: fall back to keyword scoring, log warning
- Vocabulary not found: ensureVocabulary handles this (hash fallback or rebuild)
- Empty query with vector mode: fall back to keyword (can't embed empty query)
- Repo not indexed: keyword fallback with tip message

## File Summary

| File | Action |
|------|--------|
| `internal/mcp/tools.go` | Modify: refactor toolGetContextBudgeted to support vector ranking |
| `internal/mcp/vector_scoring.go` | New: vectorScoreFunctions, keywordScoreFunctions helpers |
| `internal/mcp/tool_budgeted_vector_test.go` | New: vector-ranked budgeted context tests |
