# Implementation Plan: Agent Recipes & Pre-Built Workflows

## Overview

Replace the existing `execute_pattern` system (`internal/compose/`) with a new Recipe framework in `internal/recipes/`. Recipes are high-level composed workflows that assemble context from multiple sources (structural data, vector search, AI) and return agent-optimized output. Three initial recipes: `analyze_pr_impact`, `explain_api_flow`, `review_architecture`. Each recipe produces structured JSON data with an optional AI-generated natural language summary.

Recipes degrade gracefully when optional dependencies (dependency graph, API flow tracing, semantic search) are not yet available — they return partial results with clearly marked gaps.

## Current Architecture

### What Exists
- **Pattern system** (`internal/compose/`): 4 patterns (SearchWithContext, ImpactAnalysis, FindAndExpand, MultiRepoSearch) with Chain execution model. Has bugs (silent step skipping, closure bug in MultiRepoSearch). Uses Pattern/Chain/ToolExecutor interfaces.
- **PR context** (`internal/orchestrator/pr_context.go`): Single-repo PR analysis with function changes, callers, callees, DB queries, HTTP calls, side effects, impact analysis.
- **AI system** (`internal/ai/`): Provider interface (Anthropic), ContextExtractor, retry logic. `CompleteRaw` exists on `*AnthropicProvider` but is not yet on the `Provider` interface.
- **Token budget** (`internal/tokens/`): Budgeter with keyword scoring, greedy selection within budget. TokenCounter with ~4 chars/token ratio.
- **Semantic search** (`internal/vectors/`): LocalEmbedder (TF-IDF, 256 dims), SQLiteVectorStore, SemanticSearch service (concrete struct, no interface).
- **30 MCP tools** in `internal/mcp/tools.go`.

### What's Missing
1. Recipe framework (interface, runner, registry)
2. `CompleteRaw` promoted to `ai.Provider` interface
3. `VectorSearcher` interface for mockable vector search
4. Hybrid context assembly (structural + vector + AI)
5. Agent-optimized response format with facts/analysis separation
6. Cross-repo impact analysis in PR context
7. API flow explanation recipe
8. Architecture review recipe
9. Deprecation of old pattern system

## Section-by-Section Plan

### Section 1: Recipe Framework & Types

**Goal:** Define the Recipe interface, RecipeResult type, RecipeRunner, and registry. Promote `CompleteRaw` to the AI provider interface. Define `VectorSearcher` interface.

**New package: `internal/recipes/`**

**AI Interface Update:**

Add `CompleteRaw(ctx context.Context, prompt string, maxTokens int) (string, error)` to the `ai.Provider` interface. It already exists on `*AnthropicProvider` concretely — just promote it. This enables recipes to send free-form prompts for risk assessment and narrative generation.

**VectorSearcher Interface:**

Define in `internal/recipes/`:

```
type VectorSearcher interface {
    SearchFunctions(query string, repoID string, limit int) ([]vectors.SearchResult, error)
    SearchTypes(query string, repoID string, limit int) ([]vectors.SearchResult, error)
}
```

The existing `*vectors.SemanticSearch` satisfies this interface. Tests mock it.

**Recipe interface:**

```
type Recipe interface {
    Name() string
    Description() string
    InputSchema() map[string]FieldSpec
    Run(ctx context.Context, runner *RecipeRunner, input RecipeInput) (*RecipeResult, error)
}
```

Note: `*RecipeRunner` is passed explicitly as a parameter (not via context.WithValue) to avoid the anti-pattern of hiding dependencies in context.

`FieldSpec` describes each input field: name, type (string/int/bool/[]string/[]ChangedFile), required flag, description. Used for validation and documentation.

`RecipeInput` is a typed wrapper around `map[string]any` with helper methods: `GetString(key)`, `GetStringSlice(key)` (handles `[]interface{}` -> `[]string` coercion), `GetInt(key, defaultVal)`, `GetBool(key, defaultVal)`. `Validate(schema)` checks required fields.

**RecipeResult (with renamed types to avoid collisions):**

```
type RecipeResult struct {
    Recipe        string                 // recipe name
    Data          map[string]any         // structured output (facts from code)
    Analysis      string                 // AI-generated summary (optional)
    Gaps          []GapNote              // sections that couldn't be computed
    Sources       []RecipeSourceRef      // file:line references for claims
    ContextTokens int                    // context budget tokens used
    AITokens      int                    // AI API tokens consumed
    DurationMS    int64                  // execution time
    Confidence    float64                // 0-1, based on data completeness
}
```

**Confidence scoring:** 1.0 if all steps succeed with AI, 0.7 if AI unavailable, 0.5 if structural gaps present, compound multiplicatively.

```
type GapNote struct {
    Section    string
    Reason     string
    Suggestion string
}

type RecipeSourceRef struct {
    File  string
    Line  int
    Claim string
}
```

**RecipeRunner:**

```
type RecipeRunner struct {
    manager    orchestrator.Manager
    orgManager org.Manager
    ai         ai.Provider          // may be nil
    vectors    VectorSearcher       // may be nil (interface, not concrete)
    budgeter   *tokens.Budgeter
    registry   *Registry
    logger     *logging.Logger
}
```

Constructor: `NewRecipeRunner(manager, orgManager, opts ...RunnerOption)` where options set AI provider, vector searcher, logger, registry.

**Composability via Execute method:**

```
func (r *RecipeRunner) Execute(ctx context.Context, recipeName string, input RecipeInput) (*RecipeResult, error)
```

Looks up recipe in registry, validates input, calls `recipe.Run(ctx, r, input)`. This allows recipes to call other recipes: a recipe receives `*RecipeRunner` and can call `runner.Execute("other_recipe", input)`.

**Recipe Registry:**

```
type Registry struct {
    recipes map[string]Recipe
}
```

Methods: `Register(recipe)`, `Get(name) (Recipe, bool)`, `List() []RecipeInfo`. `DefaultRegistry()` returns pre-populated registry with all built-in recipes.

**Token Budget Management:**

Each recipe receives a configurable token budget (default 8000, max 32000) via `RecipeInput.GetInt("token_budget", 8000)`. Budget allocation percentages are configurable per recipe via a `BudgetAllocation` struct:

```
type BudgetAllocation struct {
    Structural float64  // e.g., 0.40
    Vector     float64  // e.g., 0.30
    Keyword    float64  // e.g., 0.20
    Metadata   float64  // e.g., 0.10
}
```

If a pass exceeds its slice, the existing `tokens.Budgeter` greedy selection truncates. If vector search is unavailable, redistribute: structural gets vector's share.

**Error Handling:**

Recipes return partial results on non-fatal errors. Each step can fail independently:
- Add GapNote explaining what's missing and why
- Continue with remaining steps
- Check `ctx.Err()` after each step; if cancelled, return partial results with GapNote for remaining steps
- Only return error from Run() for fatal errors (invalid input)

**Concurrency:** Independent recipe steps use `errgroup` for parallel execution (e.g., cross-service and dependency impact in analyze_pr_impact). Each goroutine writes to its own result slice; results merged after all complete.

### Section 2: Hybrid Context Assembly

**Goal:** Build a context assembler that combines structural data, vector search results, and AI summaries into a unified context within a token budget.

**ContextAssembler struct:**

```
type ContextAssembler struct {
    manager  orchestrator.Manager
    vectors  VectorSearcher        // interface, optional
    budgeter *tokens.Budgeter
}
```

**Assembly strategy:**

Given a query/topic and a token budget, the assembler runs passes with configurable allocation:

1. **Structural pass (default 40%):** Get pre-computed data from the manager — function contexts, callers, callees, side effects. These are "facts" (verifiable from code). Use `budgeter` greedy selection to fit within allocation.

2. **Vector pass (default 30%):** If VectorSearcher is non-nil and the repo is indexed, find semantically similar functions/types. Supplement structural data with related code keyword search might miss. If unavailable, redistribute to structural (50%) and keyword (30%).

3. **Keyword pass (default 20%):** Use keyword-based search (existing `search_context`) to fill gaps. Score by relevance.

4. **Metadata pass (default 10%):** Include file-level context, package structure, architecture overview for framing.

**AssembleContext method:**

```
func (a *ContextAssembler) Assemble(ctx context.Context, req AssemblyRequest) (*AssembledContext, error)
```

`AssemblyRequest`: repoID, query string, budget int, allocation BudgetAllocation, optional filters (file paths, function names, concept).

`AssembledContext`: structured result with items sorted by relevance, each tagged as "structural"/"vector"/"keyword" source. Includes total token count and per-pass breakdown.

**Deduplication:** Items found by multiple passes are merged (keep highest relevance score, note all discovery methods).

### Section 3: analyze_pr_impact Recipe

**Goal:** Comprehensive PR impact analysis extending existing `get_pr_context` with cross-repo awareness and AI risk assessment.

**Input:**
- `repo_id` (string, required)
- `changed_files` ([]ChangedFile, required) — path + change_type
- `org_id` (string, optional) — enables cross-repo analysis
- `token_budget` (int, optional, default 8000)
- `include_ai` (bool, optional, default true)

**Execution steps:**

1. **Single-repo impact** — Call existing `manager.GetPRContext(repoID, changedFiles)`. Returns function changes, callers, callees, DB queries, HTTP calls, side effects, affected routes. Core structural data.

2. **Cross-service impact** (parallel with step 3, if org_id provided) — For each HTTP call made by changed functions, check if any repo in the org handles that endpoint. Uses API flow tracing data. If unavailable, add GapNote: "Cross-service impact requires 03-api-flow-tracing."

3. **Dependency impact** (parallel with step 2, if org_id provided) — Check if the changed repo is a library used by other repos in the org. Uses dependency graph data. If unavailable, add GapNote: "Dependency impact requires 02-dependency-graph."

Steps 2 and 3 run concurrently via `errgroup` since they are independent.

4. **Risk assessment** (after steps 1-3) — If AI available, build prompt with structural data and ask AI for risk (low/medium/high) with reasoning. Use `ai.Provider.CompleteRaw()`. If AI unavailable, use heuristic: `affectedCallers * 2 + externalAPIChanges * 5 + dbSchemaChanges * 3`. Threshold: <5 low, 5-15 medium, >15 high. Confidence: 0.8 for AI, 0.5 for heuristic.

5. **Suggested reviewers** — Extract from function metadata if git blame data available. Otherwise skip with GapNote.

**Output:** Uses `RecipeRiskAssessment` type (not `ImpactAnalysis` to avoid collision with `orchestrator.ImpactAnalysis`):

```
type RecipeRiskAssessment struct {
    Level      string  // "low", "medium", "high"
    Reasoning  string
    Confidence float64
}
```

Data keys: `changed_functions`, `affected_callers`, `affected_routes`, `cross_service_impact`, `dependency_impact`, `risk`, `suggested_reviewers`.

### Section 4: explain_api_flow Recipe

**Goal:** Natural language explanation of a request flow with Mermaid visualization.

**Input:**
- `function_name` (string, required for v1) — e.g., "HandleLogin"
- `endpoint` (string, optional) — e.g., "POST /api/v1/login". **V1 limitation:** endpoint-to-handler lookup requires 03-api-flow-tracing. If endpoint provided without flow data, add GapNote and suggest using function_name instead.
- `repo_id` (string, required if no org_id)
- `org_id` (string, optional) — enables cross-service tracing
- `token_budget` (int, optional, default 8000)

**Execution steps:**

1. **Find entry point** — Search for function by name using `manager.GetFunctionContext()`. If not found, return error. If endpoint provided and flow data available, resolve endpoint to handler function.

2. **Trace internal flow** — From the entry function, follow the call graph: handler -> service -> repository -> external calls. For each function in chain: name, file, behavior summary, side effects. Depth limit: 10 functions to prevent infinite loops.

3. **Trace cross-service hops** (if org_id and flow data available) — For each outbound HTTP/gRPC call, find receiving handler in another repo. Recurse with depth limit of 5 services. If flow tracing unavailable, add GapNote.

4. **Build Mermaid diagram** — Generate sequence diagram from the traced flow. Each service is a participant. DB/cache calls shown as notes. Return as string in output.

5. **AI explanation** (if available) — Generate natural language walkthrough via `CompleteRaw`. If unavailable, return structural data without narrative.

**Output keys:** `entry_point`, `flow_steps`, `cross_service_hops` (or gap), `mermaid`, `data_transformations`.

### Section 5: review_architecture Recipe

**Goal:** Org-wide or multi-repo architecture assessment with health indicators.

**Input:**
- `org_id` (string, optional)
- `repo_ids` ([]string, optional) — alternative to org_id
- `token_budget` (int, optional, default 12000)
- `focus_areas` ([]string, optional)

One of `org_id` or `repo_ids` required.

**Execution steps:**

1. **Service inventory** — For each repo, get full `*RepoContext` from manager (no scope parameter on Manager — extract architecture fields in recipe code). Extract: name, language, framework, file count, function count, purpose (from AI summary if available).

2. **Dependency analysis** (if dependency data available) — Build dependency graph. Identify circular deps, tight coupling, orphans. If unavailable, add GapNote.

3. **Pattern analysis** — Compare repos for shared patterns. Use existing `compare_repos` for duplicates. Identify inconsistencies.

4. **Health indicators** — Per repo: test file count, documentation presence, code size, dependency freshness.

5. **AI recommendations** (if available) — Generate recommendations via `CompleteRaw`. Focus on coupling, standardization, testing gaps.

**Output keys:** `services`, `dependency_graph` (or gap), `shared_patterns`, `health_indicators`, `issues`.

### Section 6: MCP Tool Integration & Pattern Deprecation

**Goal:** Expose recipes as MCP tools, deprecate old pattern system.

**New MCP tools:**

```
analyze_pr_impact    — calls analyze_pr_impact recipe
explain_api_flow     — calls explain_api_flow recipe
review_architecture  — calls review_architecture recipe
execute_recipe       — generic recipe execution by name
list_recipes         — list available recipes with schemas
```

**RecipeRunner instantiation:**

Create `RecipeRunner` in the MCP server's `NewServer` constructor (alongside existing managers). Store as a field on `Server`. Pass available services:
- `manager` — existing
- `orgManager` — existing (already in `ServerConfig`)
- `ai.Provider` — existing
- `VectorSearcher` — wrap existing `*vectors.SemanticSearch` if non-nil
- `registry` — `DefaultRegistry()`

**Tool handler pattern:**

Each recipe tool:
1. Parse MCP tool input to RecipeInput
2. Call `server.recipeRunner.Execute(ctx, recipeName, input)`
3. Format RecipeResult as MCP tool output

**execute_recipe tool:**

Generic entry point: `recipe_name` (string), `params` (map), `output_format` ("markdown"|"json", default "markdown").

**Output formatting:**

- **Markdown mode** (default for MCP): sections for Data, Analysis, Gaps, Sources
- **JSON mode** (for REST API): raw RecipeResult JSON

**Deprecation of execute_pattern:**

1. Mark existing `execute_pattern` tool description as deprecated: "Deprecated: use execute_recipe instead."
2. Map: `impact_analysis` -> `analyze_pr_impact`. `search_with_context` -> use `get_context_budgeted`. `multi_repo_search` -> use `search_context` with org scope.
3. Keep functional with deprecation warnings. Do NOT delete `internal/compose/` — removal is a future cleanup.

### Section 7: Integration Tests

**Goal:** End-to-end tests for recipes and framework.

**Test infrastructure:**

Use gorilla/* repos for structural tests. Use synthetic fixtures for flow tracing. Mock `VectorSearcher` interface for vector tests. Mock `ai.Provider` for AI tests (including `CompleteRaw`).

**Recipe framework tests:**

- Registry: register, get, list, get unknown returns false
- RecipeInput: validation (missing required fields), `GetStringSlice` coercion from `[]interface{}`
- RecipeResult: partial results with gaps, source refs present
- Token budget: recipes stay within budget (output tokens < input budget)
- Error handling: step failure produces gap, not error
- Composability: recipe A calls runner.Execute("recipe_b", input)

**analyze_pr_impact tests:**

- Basic PR with single file change -> function changes, callers
- PR with no changed functions -> valid empty result
- PR with org_id but no flow data -> partial result with GapNote
- AI unavailable -> heuristic risk score with confidence 0.5
- Steps 2 & 3 run concurrently (verify no data races with `-race`)

**explain_api_flow tests:**

- Known function name -> flow steps with Mermaid diagram
- Unknown function -> error
- Endpoint provided but no flow data -> GapNote suggesting function_name
- Cross-service with no flow data -> single-service flow with GapNote

**review_architecture tests:**

- Single repo -> service inventory and health
- Multi-repo -> comparison and shared patterns
- With org -> org-level view
- No dependency data -> partial with GapNote

**Migration tests:**

- Old execute_pattern still works (backward compat)
- Deprecation warning logged
- execute_recipe works with recipe name

**Token benchmarks:**

- Measure output size for each recipe
- Verify 2K-8K token range for standard inputs
- Compare tool call count: recipe (1 call) vs manual (5-10 calls)

## Error Handling

- Invalid recipe name: error with available names
- Missing required input: error listing missing fields
- Step failure: GapNote, continue with remaining steps
- AI unavailable: skip AI steps, structured data only, GapNote, confidence reduced
- Context cancelled: partial results with GapNote for remaining steps
- Token budget exceeded: truncate via Budgeter, note truncation

## Performance Considerations

- Pre-computed structural data is fast; AI calls are the bottleneck
- Max 1-2 AI calls per recipe (risk assessment + narrative summary)
- Independent steps run concurrently via errgroup
- Token budget prevents unbounded output
- Partial results allow fast response even when steps fail
- Recipe results could be cached (future enhancement)
