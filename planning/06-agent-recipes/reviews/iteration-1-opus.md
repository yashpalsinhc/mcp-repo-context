# Implementation Plan Review: Agent Recipes & Pre-Built Workflows

**Reviewer:** Claude Sonnet 4.6 (acting as Opus reviewer)
**Date:** 2026-02-25
**Plan:** `/planning/06-agent-recipes/claude-plan.md`
**Spec:** `/planning/06-agent-recipes/claude-spec.md`

---

## Summary

The plan is architecturally sound in its goals but has several concrete issues with type collisions, interface mismatches, dependency injection design, and incomplete concurrency guidance. The hybrid context assembly section under-specifies the actual plumbing. Several spec requirements are not addressed. The testing strategy has gaps, particularly around the AI path.

---

## Issues

### 1. `ImpactAnalysis` Type Name Collision (Critical)

The plan's `analyze_pr_impact` recipe returns a data structure that includes a `risk` block and extended fields, building on `orchestrator.PRContextResult`. However, `orchestrator.ImpactAnalysis` (in `internal/orchestrator/pr_context.go:107`) is already a concrete struct with fields `AffectedFunctions`, `AffectedRoutes`, `RiskLevel`, `RiskReasons`, etc. Simultaneously, `compose.ImpactAnalysis` (in `internal/compose/patterns.go:227`) is an empty struct that is a `Pattern` implementation.

The plan for Section 3 redefines overlapping concepts without acknowledging these existing types. If the `analyze_pr_impact` recipe re-uses or embeds `orchestrator.PRContextResult`, it inherits `orchestrator.ImpactAnalysis` as a nested field. Adding a new `risk` block in the recipe output alongside this creates an incoherent schema — two competing risk representations in one result. The plan must explicitly decide: extend `orchestrator.ImpactAnalysis` with the `confidence` field and `reasoning` string, or define a separate `RecipeRiskAssessment` type that does not conflict. Leaving this implicit will produce either a duplicate JSON key or a confused type hierarchy.

**Fix:** Define a distinct `RecipeRiskAssessment` type in `internal/recipes/` and document that it replaces — not augments — the heuristic `RiskLevel`/`RiskReasons` fields in `orchestrator.ImpactAnalysis` for recipe output.

---

### 2. `SourceRef` Name Collision (Critical)

The plan defines `SourceRef` as a field in `RecipeResult`:

```go
Sources []SourceRef  // file:line references for claims
```

This name already exists twice in the codebase:
- `orchestrator.SourceRef` in `internal/orchestrator/interface.go:104` (has `RepoID`, `FilePath`, `Function`, `Line`)
- `ai.SourceRef` in `internal/ai/query.go:26` (has `File`, `Line`, `Context`)

The plan's `SourceRef` adds a `Claim` field not present in either. Introducing a third `SourceRef` in `internal/recipes/` with a different shape will cause import confusion and defeats the goal of reusing existing types. When recipe output is consumed by MCP tool handlers that already use `orchestrator.SourceRef`, implicit conversion will be needed and is not described.

**Fix:** Either reuse `orchestrator.SourceRef` and add the `Claim` field there (with backward compatibility), or name the recipes type `RecipeSourceRef` to avoid collision.

---

### 3. `ai.Provider` Interface Does Not Expose `CompleteRaw` (Critical)

Section 2 (Hybrid Context Assembly) and Section 3-5 (individual recipes) all plan to call AI for risk assessment, narrative generation, and architecture recommendations. The plan states the `RecipeRunner` holds `ai ai.Provider`.

The `ai.Provider` interface (`internal/ai/provider.go`) only defines:
- `GenerateSummary`
- `AnalyzeArchitecture`
- `GenerateDescription`
- `IsConfigured`
- `Name`

`CompleteRaw` exists on `*AnthropicProvider` concretely (line 139 of `anthropic.go`) but is **not part of the `Provider` interface**. The research notes it and the plan calls it out as available, but the plan's recipe implementations will need to send arbitrary prompts (e.g., "assess risk given these callers and HTTP calls"). None of the three interface methods match that need — `GenerateSummary` expects `SummaryRequest` shaped for repo-level summarization, `AnalyzeArchitecture` expects `ArchitectureRequest`, and `GenerateDescription` is for individual code elements.

This means recipes either need to type-assert to `*AnthropicProvider` (breaking the interface abstraction) or the `Provider` interface must be extended with a general `Complete(ctx, prompt string, maxTokens int) (string, int, error)` method before Section 1 is complete.

**Fix:** Add `Complete(ctx context.Context, prompt string, maxTokens int) (string, int, error)` to the `ai.Provider` interface in Section 1, and implement it on `AnthropicProvider` (it already has `CompleteRaw` — just promote it to the interface).

---

### 4. `RecipeRunner` Injected via `context.WithValue` is an Anti-Pattern (Significant)

The plan states:

> "The runner is passed to each recipe via the context (using `context.WithValue`) so recipes can access shared services."

Using `context.WithValue` to pass service dependencies (managers, AI providers, vector search) is a known Go anti-pattern. `context.Context` is for request-scoped cancellation signals and deadlines, not for dependency injection. The Go documentation explicitly warns against this for non-request-scoped values.

More concretely: every recipe's `Run(ctx, input)` signature already receives the context. If the runner is in the context, recipes must always perform an unsafe lookup (`RunnerFromContext(ctx)`) with a nil-check, instead of having compile-time guarantees. This is worse than the existing pattern system which passes `executor ToolExecutor` directly.

The research confirms the existing `Pattern.Build(executor ToolExecutor, params)` passes dependencies directly — this approach should be continued.

**Fix:** Pass `*RecipeRunner` (or a `Services` interface) as an explicit parameter to `Run`. Change the signature to `Run(ctx context.Context, runner *RecipeRunner, input RecipeInput) (*RecipeResult, error)`. This is the idiomatic Go approach and provides compile-time safety.

---

### 5. `ContextAssembler` Budget Percentages Are Hard-Coded and Untested (Significant)

Section 2 specifies fixed budget splits: 40% structural, 30% vector, 20% keyword, 10% metadata, with a fallback redistribution when vectors are unavailable. This design has two problems:

First, the ratios are arbitrary and will need tuning based on real query types. A PR impact query needs more structural data; an architecture review needs more metadata. Baking in fixed percentages means the assembler will be wrong for at least two of the three recipes.

Second, the plan provides no error handling for the case where the structural pass itself exhausts more than 40% of budget (which is likely for large PRs with many affected callers). There is no description of what happens when `manager.GetFunctionContext` returns data that exceeds the allocated slice of budget before the vector and keyword passes run.

The existing `tokens.Budgeter.BuildFunctionContext` already handles per-item budget overflow by summarizing — but the assembler plan does not describe how it integrates with `Budgeter` at all. The assembler appears to duplicate budgeting logic rather than delegate to the existing `Budgeter`.

**Fix:** Make the budget percentages configurable per recipe via `AssemblyRequest`. Add explicit overflow handling: if the structural pass exceeds its slice, truncate using `Budgeter.BuildFunctionContext` before proceeding. Document the integration with `tokens.Budgeter` explicitly — the assembler should use it, not reinvent it.

---

### 6. `explain_api_flow` Depends Entirely on Unbuilt Infrastructure (Significant)

Section 4 describes tracing the call graph from an HTTP handler through service and repository layers. The implementation steps assume:

- HTTP handler lookup by endpoint string (e.g., "POST /api/v1/login")
- Call graph traversal from handler to repository
- Cross-service hop resolution via flow tracing

None of this infrastructure exists today. The `orchestrator.Manager` interface has no method to find a handler by route. `GetFunctionContext` requires knowing the `file_path` and `function_name` already. The semantic search and smart query tools can find a function by concept, but have no HTTP-route-to-handler mapping.

The plan's Step 1 says "If endpoint given, search for the matching HTTP handler in the repo" but provides no mechanism. The existing `ImpactAnalysis.AffectedRoutes` data in `PRContextResult` contains route metadata for PR-changed functions, but there is no reverse lookup (route string → handler function).

The plan notes cross-service tracing requires `03-api-flow-tracing` and adds a `GapNote` when unavailable — that degradation is well thought out. But the single-service case (no org_id) also cannot work today without basic route-to-handler lookup, which is not in scope for this split.

**Fix:** Explicitly scope Section 4 to function-name-input only for this split. The endpoint-input path should be marked as requiring `03-api-flow-tracing` and produce a `GapNote` at this stage. This is honest about what can be built now versus what requires future splits.

---

### 7. No Concurrency Model Specified for Multi-Step Recipe Execution (Significant)

Recipes execute multiple steps (PR context, cross-service check, risk assessment). The plan says "Step failure adds a GapNote, continue with remaining steps" but never specifies whether steps run sequentially or concurrently.

For `analyze_pr_impact`, Steps 2 (cross-service) and 3 (dependency impact) are independent of each other and could run concurrently after Step 1 (single-repo impact) completes. Step 4 (AI risk assessment) depends on Steps 1-3. Step 5 (suggested reviewers) is independent.

With no concurrency model, implementers will default to sequential execution, making AI-enabled recipes take 3-5x longer than necessary. Conversely, if someone naively parallelizes all steps, Step 4 will race with Steps 2-3.

The existing `org.Analyzer` uses a worker pool with bounded concurrency (see `internal/org/analyzer.go`) as a reference pattern that could be reused.

**Fix:** Add a step dependency graph to the plan. Specify which steps can be parallelized. At minimum, state that steps with no data dependency on prior steps SHOULD run concurrently using `errgroup` or a similar pattern, with a context cancellation propagation to all goroutines on fatal error.

---

### 8. `org.Manager.Get` Returns Repos But the Plan Does Not Use It Correctly (Significant)

The `review_architecture` recipe (Section 5) says "For each repo, get architecture context" when `org_id` is provided. To do this it needs the list of repo IDs in the org. The `org.Manager` interface provides `Get(ctx, orgID) (*Org, error)` which returns an `*Org` with a `Repos []string` field — this is the correct path.

However, the plan conflates two different manager interfaces: `org.Manager` and `orchestrator.Manager`. The `RecipeRunner` holds both. The plan's Section 5 Step 1 says "get architecture context (scope=architecture)" — this is `orchestrator.Manager.GetContext(ctx, repoID)` with scope filtering in the result, which is not a direct method on the `Manager` interface. The actual `Manager.GetContext` returns `*ctxpkg.RepoContext` which includes the full context; there is no `scope` parameter on the Go method (scope filtering exists in the MCP tool layer, not the Manager layer).

**Fix:** Clarify that `review_architecture` calls `org.Manager.Get` to enumerate repo IDs, then calls `orchestrator.Manager.GetContext` for each repo and extracts the architecture fields in the recipe code (not via a scope parameter). Remove the reference to a non-existent `scope=architecture` parameter on the Manager interface.

---

### 9. Token Counting for `RecipeResult.TokensUsed` Is Underspecified (Medium)

The plan says `TokensUsed int` tracks "total tokens consumed." This is ambiguous:

- Does it count the tokens in the output (what `tokens.TokenCounter` counts for output serialization)?
- Does it count AI API tokens consumed (from `SummaryResponse.TokensUsed` or the return value of `CompleteRaw`)?
- Does it count both, summed?

The existing `QueryResult` in `orchestrator/interface.go` has `TokensUsed int` which tracks AI API tokens. The `tokens.Budgeter` tracks context selection budget. These are different quantities. Conflating them in a single field makes the metric misleading (AI API tokens are billed; output tokens are just size indicators).

**Fix:** Split into `ContextTokens int` (tokens used from the budget for context assembly) and `AITokens int` (tokens consumed by AI API calls). This matches the distinction already implicit in the existing system.

---

### 10. `RecipeInput.GetStringSlice` Type Coercion Is Underspecified (Medium)

The plan defines `GetStringSlice(key)` as a helper on `RecipeInput`. The input comes from `map[string]any` deserialized from JSON MCP tool arguments. JSON arrays of strings arrive as `[]interface{}` not `[]string`. The plan does not describe how `GetStringSlice` handles the `[]interface{}` → `[]string` conversion, nor what it returns when the value is a single string (which agents sometimes pass instead of a one-element array).

The existing tools in `tools.go` all handle this inline (e.g., line 2324: `var changedFiles []orchestrator.ChangedFile` with explicit type assertion loops). The recipe framework should codify this pattern, not leave it implicit.

**Fix:** Specify that `GetStringSlice` handles: `[]string` directly, `[]interface{}` with element-wise string casting, and single `string` wrapped in a slice. Add a note that elements that fail string casting are skipped with a warning.

---

### 11. Migration Path for `search_with_context` → `execute_recipe` Is Incomplete (Medium)

Section 6 states:

> `search_with_context` → use `execute_recipe` with equivalent params

But `search_with_context` is a pattern that takes `repo_id` and `query`, then chains `search_context` + `get_function_context`. The new recipe system has no direct equivalent — `analyze_pr_impact` is PR-scoped, not a general search-then-expand workflow. The plan explicitly says `find_and_expand` has "no direct equivalent (remove)" but `search_with_context` is similar in nature.

If `search_with_context` is deprecated without a recipe equivalent, users of `execute_pattern` who relied on it for general search workflows will have no migration path — they'll need to make two separate tool calls (`search_context` + `get_function_context`), which defeats the composition purpose.

**Fix:** Either implement a `search_with_context` recipe as part of this split (it is simple: two sequential steps), or explicitly document that the use case is served by `smart_query` and provide a comparison showing why `smart_query` is the replacement.

---

### 12. `RecipeRunner` Constructor Does Not Address Server Integration (Medium)

The plan defines `NewRecipeRunner(manager, orgManager, opts ...RunnerOption)` as a standalone constructor. However, the MCP `server` struct already holds `manager`, `semanticSearch`, and `config.OrgManager` as fields. Each recipe tool call (5 new tools) would need to construct a `RecipeRunner` from these server fields.

The plan does not describe where `RecipeRunner` is instantiated — once at server startup (stored as a field) or per-request (constructed in each tool handler). Per-request construction is wasteful if the runner holds pre-initialized services. Per-startup storage means the runner is a server field, but this is not mentioned in the server struct changes.

**Fix:** Specify that `RecipeRunner` is constructed once during `NewServer` and stored as a field on `server`. Show the updated `server` struct with the `recipeRunner *recipes.RecipeRunner` field. This also makes the relationship between `RecipeRunner` and the existing `patternRegistry` explicit (they coexist during the deprecation period).

---

### 13. Integration Tests Rely on Gorilla Repos But Do Not Verify Gorilla Has Routes (Medium)

Section 7 specifies using gorilla/mux, gorilla/handlers, gorilla/sessions for integration tests. The `explain_api_flow` recipe test case "Known endpoint → returns flow steps with Mermaid" requires a repo with known HTTP routes.

`gorilla/mux` is a routing library; its own codebase registers test routes internally but does not expose production endpoints that the recipe can trace. The test would need to either (a) use the gorilla/mux library from the perspective of a hypothetical app using it (not the library itself), or (b) use a different test fixture repo that actually defines HTTP handlers.

The same issue affects `analyze_pr_impact` cross-service tests — the gorilla repos are independent libraries, not a multi-service application with HTTP calls between them, so cross-service impact tests will always produce empty results.

**Fix:** Add a `testdata/` fixture with a minimal multi-file Go application that defines HTTP routes and calls between packages. This fixture can be analyzed by the test setup and used for all recipe integration tests that require realistic flow data.

---

### 14. No Mock Interface Defined for `RecipeRunner` Dependencies (Medium)

The plan describes unit tests for recipes with "mock data" but does not define how dependencies are mocked. The `RecipeRunner` holds `orchestrator.Manager` (an interface — mockable), `org.Manager` (an interface — mockable), `ai.Provider` (an interface — mockable), and `*vectors.SemanticSearch` (a concrete struct, not an interface).

`vectors.SemanticSearch` being a concrete struct means it cannot be directly mocked without a wrapper interface. Unit tests for recipes that test the vector pass of `ContextAssembler` would need either a real `SemanticSearch` with a temp SQLite DB (slow, integration-style) or an interface extraction.

**Fix:** Define a `VectorSearcher` interface in `internal/recipes/` (or extract one in `internal/vectors/`) with the methods recipes actually call (`SearchFunctions`, `SearchAll`). Use this interface in `RecipeRunner` instead of `*vectors.SemanticSearch`. The existing concrete type can implement it without modification.

---

### 15. Spec Requirement R1 "Recipes Are Composable" Is Not Addressed (Medium)

The spec states: "Recipes are composable (recipe A can use recipe B as a step)."

The plan's `Recipe` interface is `Run(ctx, input) (*RecipeResult, error)`. Nothing in the plan describes how one recipe calls another. The `RecipeRunner` is available (via context or parameter), and technically a recipe could call `runner.Run(recipeName, input)` — but this is not documented and `RecipeRunner` in the plan does not have a `Run` method that takes a recipe name.

The plan's registry has `Get(name) (Recipe, bool)` but the `RecipeRunner` has no method to execute a named recipe. The gap between "recipes are composable" and the actual interfaces is not bridged.

**Fix:** Add `RunRecipe(ctx, name string, input RecipeInput) (*RecipeResult, error)` to `RecipeRunner`. Document the composability pattern with one concrete example (e.g., `review_architecture` could call `analyze_pr_impact` for each repo's recent changes).

---

### 16. Error Return on Context Cancellation Does Not Preserve Partial Results (Medium)

The plan states: "Context cancelled: return partial results collected so far." But the `Recipe` interface returns `(*RecipeResult, error)`. In Go, if you return both a non-nil `*RecipeResult` and a non-nil `error`, callers must know to check the result even when `err != nil`. This is an uncommon pattern in Go — most callers write `if err != nil { return nil, err }`.

The MCP tool handlers in `tools.go` all follow the `if err != nil { return errorResult(...) }` pattern, discarding any partial result. If the recipe returns `(partialResult, context.Canceled)`, the current tool handler idiom would discard `partialResult` and show only an error to the user.

**Fix:** Document explicitly in the plan (and eventual code) that recipe callers MUST check the result even when error is non-nil. Alternatively, encode cancellation as a `GapNote` (not an error return) and return `(result, nil)` when the context is cancelled mid-execution with partial results already collected. This is more idiomatic for this use case.

---

### 17. `DurationMS int64` Field Should Use `time.Duration` (Minor)

`RecipeResult.DurationMS int64` stores milliseconds as a raw integer. The rest of the codebase uses `time.Duration` (e.g., `AnalyzeResult.Duration time.Duration`, `org.AnalysisResult.Duration time.Duration`). Using `int64` for milliseconds is inconsistent and loses precision for sub-millisecond operations.

**Fix:** Change to `Duration time.Duration` to match existing conventions.

---

### 18. `Confidence float64` on `RecipeResult` Is Ambiguous Without a Definition (Minor)

The plan adds `Confidence float64` (0-1) to `RecipeResult` as "overall confidence score" but does not define how it is computed. For a recipe with 3 steps where one produces a gap, is confidence 0.67? If AI is unavailable, does confidence drop by a fixed penalty? Without a formula, different recipe implementations will compute it differently, making the field unreliable across recipes.

**Fix:** Either remove `Confidence` from `RecipeResult` and keep it only on AI-generated sub-fields (where the AI response naturally produces it), or define a concrete formula: `confidence = (steps_completed / total_steps) * (1.0 if AI available else 0.7)`.

---

### 19. Section 7 Token Benchmark Test Is Not a Go Test (Minor)

The plan describes "Token usage benchmarks — measure token output for each recipe, verify recipes target 2K-8K token range." Go benchmarks use `func BenchmarkX(b *testing.B)`, not assertions. Verifying a token range is a test assertion (`t.Assert`), not a benchmark. Mixing benchmark and assertion semantics in the same function is incorrect Go testing practice.

**Fix:** Separate into two: `BenchmarkRecipeTokens` functions that measure token cost (using `b.ReportMetric`), and `TestRecipeTokenBudget` table-driven tests that assert recipes stay within the specified budget range.

---

### 20. Plan Does Not Address `server.go` `OrgManager` Field Already Exists (Minor)

`ServerConfig.OrgManager org.Manager` is already defined in `internal/mcp/server.go:57`. The plan describes adding org-level capabilities to recipes via `RecipeRunner.orgManager`, which is the right approach. However, the plan does not note that `config.OrgManager` exists and should be passed into the `RecipeRunner` during `NewServer`. An implementer reading only the plan might add a duplicate field or call path.

**Fix:** Add a note in Section 6 (MCP Tool Integration) that `RecipeRunner` is initialized with `config.OrgManager` from the existing `ServerConfig` field.

---

## Summary Table

| # | Issue | Severity | Section |
|---|-------|----------|---------|
| 1 | `ImpactAnalysis` type name collision | Critical | S3 |
| 2 | `SourceRef` name collision (3rd definition) | Critical | S1 |
| 3 | `ai.Provider` lacks `Complete` for recipe use | Critical | S1, S3-5 |
| 4 | `RecipeRunner` via `context.WithValue` is anti-pattern | Significant | S1 |
| 5 | Budget percentages hard-coded, no overflow handling | Significant | S2 |
| 6 | `explain_api_flow` single-service case requires unbuilt infra | Significant | S4 |
| 7 | No concurrency model for multi-step recipe execution | Significant | S3-5 |
| 8 | `scope=architecture` parameter does not exist on Manager | Significant | S5 |
| 9 | `TokensUsed` conflates AI API tokens and context tokens | Medium | S1 |
| 10 | `GetStringSlice` coercion from `[]interface{}` unspecified | Medium | S1 |
| 11 | No recipe equivalent for `search_with_context` | Medium | S6 |
| 12 | `RecipeRunner` instantiation and server integration not shown | Medium | S6 |
| 13 | Integration tests depend on gorilla repos lacking routes | Medium | S7 |
| 14 | `*vectors.SemanticSearch` is concrete, not mockable | Medium | S1, S7 |
| 15 | Spec R1 composability requirement not addressed in plan | Medium | S1 |
| 16 | Context cancellation with partial results not idiomatically handled | Medium | S1 |
| 17 | `DurationMS int64` should be `time.Duration` | Minor | S1 |
| 18 | `Confidence float64` on `RecipeResult` has no defined formula | Minor | S1 |
| 19 | Token benchmark test mixes benchmark and assertion semantics | Minor | S7 |
| 20 | `config.OrgManager` already exists, plan should reference it | Minor | S6 |
