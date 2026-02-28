# Section 07: Integration Tests

## Overview

End-to-end tests for all recipes and the framework working together. Tests use real analyzed repos, verify recipe composability, token budget compliance, graceful degradation, and backward compatibility with deprecated patterns.

## Dependencies

- All previous sections (01-06)
- Test infrastructure: gorilla/* repos for structural tests, mock providers for AI/vector

## Test Infrastructure

### Test Helper

```
func setupRecipeRunner(t *testing.T) (*recipes.RecipeRunner, func())
```

1. Create temp directory for data.
2. Initialize real SQLite storage.
3. Create real Manager with storage.
4. Create mock OrgManager (or real with test orgs).
5. Create mock AI Provider (records calls, returns configurable responses).
6. Create RecipeRunner with DefaultRegistry.
7. Return runner and cleanup function.

### Mock AI Provider

```
type mockAIProvider struct {
    responses map[string]string  // prompt substring -> response
    calls     []string           // recorded prompts
}
```

Implements `ai.Provider` including `CompleteRaw`. Returns configured responses based on prompt content matching.

### Mock VectorSearcher

```
type mockVectorSearcher struct {
    results map[string][]vectors.SearchResult  // query -> results
}
```

Implements `VectorSearcher` interface. Returns configured results.

## Integration Test Scenarios

### `internal/recipes/integration_test.go`

Use build tag `//go:build integration`.

**Test: Full PR impact flow with real repo**
1. Analyze a small test repo (or gorilla/mux if available).
2. Run `analyze_pr_impact` with a changed file from the repo.
3. Assert structural data returned (changed_functions non-empty or valid empty).
4. Assert GapNotes present for cross_service_impact and dependency_impact.
5. Assert risk assessment computed (heuristic if no AI mock).
6. Assert RecipeSourceRef entries present.
7. Assert ContextTokens <= token_budget.

**Test: Architecture review with multiple repos**
1. Analyze gorilla/mux and gorilla/handlers (or 2 test repos).
2. Run `review_architecture` with both repo_ids.
3. Assert services inventory has 2 entries.
4. Assert shared_patterns detected (both are gorilla ecosystem).
5. Assert health_indicators computed per repo.
6. Assert GapNote for dependency_graph.

**Test: API flow trace with real function**
1. Analyze a repo with known function structure.
2. Run `explain_api_flow` with a known function name.
3. Assert flow_steps returned (at least entry point).
4. Assert Mermaid diagram contains "sequenceDiagram".
5. Assert entry_point has file and line.

**Test: Recipe composability**
1. Create a test recipe that internally calls `runner.Execute("review_architecture", input)`.
2. Register test recipe in registry.
3. Execute test recipe.
4. Assert inner recipe result composed into outer result.
5. Verify no deadlock or infinite recursion.

**Test: Token budget compliance across all recipes**
1. Run each recipe with budget=4000.
2. Assert ContextTokens <= 4000 for each.
3. Assert output is reasonably sized (not empty, not unbounded).

**Test: Graceful degradation without AI**
1. Create runner with AI=nil.
2. Run all 3 recipes.
3. Assert all return valid results.
4. Assert Analysis field is empty.
5. Assert Confidence < 1.0 (reduced for missing AI).
6. Assert GapNotes include AI-related gaps.

**Test: Graceful degradation without vector search**
1. Create runner with Vectors=nil.
2. Run recipes that use context assembly.
3. Assert results still valid.
4. Assert structural data present (vector budget redistributed).

## Backward Compatibility Tests

### `internal/recipes/migration_test.go`

**Test: Old execute_pattern still works**
1. Call the existing pattern system with "search_with_context".
2. Assert it returns valid results (no crash).
3. This verifies internal/compose/ is still functional.

**Test: New execute_recipe routes correctly**
1. Call execute_recipe with each of the 3 recipe names.
2. Assert each returns valid RecipeResult.

**Test: Unknown recipe returns helpful error**
1. Call execute_recipe with "nonexistent".
2. Assert error message lists available recipes.

## Unit Test Additions

### `internal/recipes/input_test.go`

**Test: GetStringSlice handles various types**
- `[]string{"a","b"}` → `["a","b"]`
- `[]interface{}{"a","b"}` → `["a","b"]`
- `nil` → `[]`
- `"single"` → `["single"]`

### `internal/recipes/result_test.go`

**Test: Confidence calculation**
- No gaps, AI available → 1.0
- No gaps, AI unavailable → 0.7
- 1 gap, AI available → 0.7
- 2 gaps, AI unavailable → ~0.34
- Floor at 0.1

**Test: RecipeResult JSON serialization**
- Create full RecipeResult with all fields
- Marshal to JSON, unmarshal back
- Assert round-trip preserves all fields

## Token Benchmarks

### `internal/recipes/benchmark_test.go`

**Benchmark: analyze_pr_impact output size**
- Run recipe with typical PR (5 changed files)
- Log ContextTokens, AITokens, total output size
- Assert 2K-8K token range

**Benchmark: explain_api_flow output size**
- Run with function that has 5-level call chain
- Assert 2K-8K token range

**Benchmark: review_architecture output size**
- Run with 3 repos
- Assert 4K-12K token range

**Benchmark: Recipe vs manual tool calls**
- Compare: 1 recipe call vs equivalent 5-10 manual tool calls
- Log tool call count reduction

## File Inventory

| File | Purpose |
|------|---------|
| `internal/recipes/integration_test.go` | End-to-end integration tests |
| `internal/recipes/migration_test.go` | Backward compatibility tests |
| `internal/recipes/input_test.go` | Additional RecipeInput unit tests |
| `internal/recipes/result_test.go` | Additional RecipeResult unit tests |
| `internal/recipes/benchmark_test.go` | Token usage benchmarks |
| `internal/recipes/testhelpers_test.go` | Mock providers, test setup helpers |

## Running Tests

```bash
# Unit tests only (fast)
go test ./internal/recipes/...

# Integration tests (slower, requires test repos)
go test -tags integration ./internal/recipes/...

# With race detector
go test -race ./internal/recipes/...

# Benchmarks
go test -bench=. ./internal/recipes/...
```

## Acceptance Criteria

1. Full PR impact flow works end-to-end
2. Architecture review works with multiple repos
3. API flow trace produces valid Mermaid
4. Recipe composability works without deadlock
5. Token budgets respected across all recipes
6. Graceful degradation without AI and without vectors
7. Old execute_pattern still functional
8. All integration tests pass with -race
9. Benchmarks confirm 2K-8K token range
