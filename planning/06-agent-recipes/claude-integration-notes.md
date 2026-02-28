# Integration Notes: 06-agent-recipes

## Integrated

1. **Type name collisions** (#1, #2) — Rename to `RecipeRiskAssessment` and `RecipeSourceRef` to avoid collision with `orchestrator.ImpactAnalysis`, `orchestrator.SourceRef`, and `ai.SourceRef`.

2. **CompleteRaw not on Provider interface** (#3) — Add `CompleteRaw` to the `ai.Provider` interface in Section 1. It already exists on `*AnthropicProvider` concretely; we just need to promote it to the interface.

3. **RecipeRunner via context.WithValue** (#4) — Change Recipe.Run signature to accept `*RecipeRunner` as an explicit parameter instead of pulling from context. New signature: `Run(ctx context.Context, runner *RecipeRunner, input RecipeInput) (*RecipeResult, error)`.

4. **Budget overflow handling** (#5) — Make budget percentages configurable per recipe via a BudgetAllocation struct. Use Budgeter for overflow: if a pass exceeds its slice, truncate via existing greedy selection. Document clearly.

5. **explain_api_flow endpoint lookup** (#6) — For v1, scope to function-name input only. Endpoint-to-handler lookup requires infrastructure from 03-api-flow-tracing. Add GapNote when endpoint is provided but flow tracing unavailable.

6. **Concurrency with errgroup** (#7) — Use `errgroup` for independent recipe steps (e.g., cross-service and dependency impact in analyze_pr_impact run in parallel).

7. **scope=architecture on Manager** (#8) — Clarify: recipes get full `*RepoContext` from Manager and extract architecture fields themselves. No scope parameter on Manager.

8. **TokensUsed clarification** (#9) — Split into `ContextTokens` (budget tokens for context assembly) and `AITokens` (API tokens consumed by AI calls). Both reported separately.

9. **GetStringSlice coercion** (#10) — Handle `[]interface{}` → `[]string` coercion explicitly with type assertion and element-by-element conversion.

10. **Recipe composability** (#15) — Address spec R1: recipes can call other recipes by accepting `*RecipeRunner` and invoking `runner.Execute(recipeName, input)`. Add `Execute` method to RecipeRunner.

11. **SemanticSearch mockability** (#14) — Define a `VectorSearcher` interface in `internal/recipes/` with the methods recipes need (SearchFunctions, SearchTypes). SemanticSearch satisfies it. Tests mock the interface.

## Not Integrated

1. **search_with_context equivalent** (#11) — The existing `get_context_budgeted` tool already covers this use case. No recipe equivalent needed. Note this in deprecation docs.

2. **DurationMS vs time.Duration** (#17) — Keep `int64` milliseconds for JSON serialization consistency with the rest of the codebase (MCP tool output is JSON).

3. **Gorilla repos for integration tests** (#13) — Gorilla repos are fine for structural tests. API flow tracing tests will use synthetic fixtures. The recipes degrade gracefully without flow data, which is exactly what we test.

4. **Confidence formula** (#18) — Confidence is set per-recipe based on available data sources: 1.0 if all steps succeed, 0.7 if AI unavailable, 0.5 if gaps present. Simple and transparent.

5. **RecipeRunner instantiation** (#12) — Will be created in the MCP server's NewServer constructor alongside existing managers. Detail added to Section 6.

6. **Context cancellation** (#16) — Use standard Go pattern: check `ctx.Err()` after each step, return partial results collected so far with a GapNote for remaining steps.
