# TDD Plan: Agent Recipes & Pre-Built Workflows

## Section 1: Recipe Framework & Types

### Tests: `internal/recipes/framework_test.go`

```
Test: RecipeInput validates required fields
- Create input missing required "repo_id"
- Call Validate(schema)
- Assert error listing missing field

Test: RecipeInput GetString returns value
- Create input with "repo_id" = "github.com/org/repo"
- Assert GetString("repo_id") returns correct value
- Assert GetString("missing") returns ""

Test: RecipeInput GetStringSlice coerces []interface{}
- Create input with "repo_ids" = []interface{}{"a", "b"}
- Assert GetStringSlice("repo_ids") returns []string{"a", "b"}

Test: RecipeInput GetInt returns default when missing
- Assert GetInt("budget", 8000) returns 8000 when not set
- Assert GetInt("budget", 8000) returns 4000 when set to 4000

Test: Registry register and get
- Register recipe with name "test_recipe"
- Get("test_recipe") returns it
- Get("unknown") returns false

Test: Registry list returns all recipes
- Register 3 recipes
- List() returns 3 RecipeInfo items

Test: RecipeRunner Execute calls correct recipe
- Register mock recipe
- runner.Execute(ctx, "mock_recipe", input)
- Assert mock recipe's Run was called with correct input

Test: RecipeRunner Execute returns error for unknown recipe
- runner.Execute(ctx, "nonexistent", input)
- Assert error mentions available recipes

Test: GapNote created correctly
- Create RecipeResult with gap
- Assert Gaps[0] has Section, Reason, Suggestion

Test: Confidence scoring
- All steps succeed with AI -> 1.0
- AI unavailable -> 0.7
- Structural gap present -> 0.5
```

### Tests: `internal/ai/provider_test.go` (addition)

```
Test: Provider interface includes CompleteRaw
- Compile-time assertion: var _ ai.Provider = &ai.AnthropicProvider{}
- Assert CompleteRaw is callable on interface
```

### Tests: `internal/recipes/vector_interface_test.go`

```
Test: SemanticSearch satisfies VectorSearcher
- Compile-time: var _ VectorSearcher = &vectors.SemanticSearch{}
```

## Section 2: Hybrid Context Assembly

### Tests: `internal/recipes/assembler_test.go`

```
Test: Assemble with all sources available
- Setup: mock manager, mock vector searcher
- Assemble with budget=4000
- Assert result has structural, vector, keyword items
- Assert total tokens <= 4000

Test: Assemble without vector search
- Setup: mock manager, vectors=nil
- Assemble with budget=4000
- Assert structural gets larger share (~50%)
- Assert no vector items

Test: Assemble deduplicates across passes
- Setup: function "CreateUser" found by structural and vector passes
- Assert single entry with merged sources

Test: Assemble respects budget allocation
- Setup: allocation {0.5, 0.3, 0.1, 0.1}
- Assert structural items use ~50% of budget

Test: Assemble with empty query returns metadata only
- Assemble with query=""
- Assert only metadata pass runs

Test: Assemble tags items by source type
- Assert each item has source "structural", "vector", or "keyword"
```

## Section 3: analyze_pr_impact Recipe

### Tests: `internal/recipes/pr_impact_test.go`

```
Test: Basic PR with single file change
- Input: repo_id, one changed file
- Mock manager.GetPRContext returns function changes
- Assert result has changed_functions, affected_callers

Test: PR with no changed functions
- Mock returns empty PR context
- Assert valid result with empty arrays

Test: PR with org_id but no flow data
- Input includes org_id
- Flow data unavailable
- Assert GapNote: "Cross-service impact requires 03-api-flow-tracing"

Test: PR with org_id but no dependency data
- Assert GapNote: "Dependency impact requires 02-dependency-graph"

Test: AI risk assessment with provider
- Mock AI CompleteRaw returns "medium" risk
- Assert risk.level = "medium", confidence = 0.8

Test: Heuristic risk when AI unavailable
- runner.ai = nil
- Mock 10 affected callers
- Assert risk based on heuristic, confidence = 0.5

Test: Steps 2 and 3 run concurrently
- Use race detector (-race flag)
- Both steps complete without data race

Test: Context cancellation returns partial results
- Cancel context after step 1
- Assert step 1 data present, steps 2-5 have GapNotes

Test: Input validation rejects missing repo_id
- Input without repo_id
- Assert error

Test: RecipeSourceRef populated for changed functions
- Assert Sources include file:line for each claim
```

## Section 4: explain_api_flow Recipe

### Tests: `internal/recipes/api_flow_test.go`

```
Test: Known function name traces flow
- Mock function context with callees
- Assert flow_steps non-empty
- Assert entry_point populated

Test: Unknown function returns error
- Assert error "function not found"

Test: Endpoint provided but no flow data
- Input: endpoint="POST /login", no flow tracing
- Assert GapNote suggesting function_name

Test: Flow generates Mermaid diagram
- Mock 3-step flow
- Assert mermaid field contains "sequenceDiagram"
- Assert participants match service names

Test: Depth limit prevents infinite loop
- Mock circular call graph (A calls B calls A)
- Assert flow terminates, doesn't hang

Test: Cross-service with no flow data
- Input: org_id set
- Assert single-service flow returned
- Assert GapNote for cross-service

Test: AI explanation generated
- Mock AI CompleteRaw
- Assert analysis field non-empty

Test: Without AI, structural data only
- runner.ai = nil
- Assert analysis field empty, data present
```

## Section 5: review_architecture Recipe

### Tests: `internal/recipes/architecture_test.go`

```
Test: Single repo returns service inventory
- Mock manager returns context for 1 repo
- Assert services[0] has name, functions count

Test: Multi-repo returns comparison
- Mock 3 repos
- Assert services has 3 entries
- Assert shared_patterns populated

Test: With org resolves repo list
- Mock orgManager returns org with 2 repos
- Assert services has 2 entries

Test: No dependency data adds GapNote
- Assert GapNote for dependency_graph

Test: Health indicators computed
- Mock repo with 5 test files, 50 functions
- Assert health_indicators has test_files=5

Test: AI recommendations generated
- Mock CompleteRaw returns recommendations
- Assert analysis non-empty

Test: Focus areas filter output
- Input: focus_areas=["testing"]
- Assert health_indicators emphasized, dependency_graph skipped

Test: Input validation requires org_id or repo_ids
- Input with neither
- Assert error
```

## Section 6: MCP Tool Integration

### Tests: `internal/mcp/recipe_tools_test.go`

```
Test: analyze_pr_impact tool invokes recipe
- Call tool handler with valid input
- Assert recipe was executed
- Assert MCP output contains structured data

Test: execute_recipe tool routes to correct recipe
- Call with recipe_name="analyze_pr_impact"
- Assert correct recipe invoked

Test: execute_recipe with unknown recipe returns error
- Call with recipe_name="nonexistent"
- Assert error with available names

Test: list_recipes returns all registered recipes
- Assert output lists 3 recipes with schemas

Test: output_format=json returns raw JSON
- Call with output_format="json"
- Assert output is valid JSON RecipeResult

Test: output_format=markdown returns formatted markdown
- Call with output_format="markdown" (default)
- Assert output has section headers

Test: execute_pattern deprecated but functional
- Call execute_pattern with "impact_analysis"
- Assert it works (backward compat)
- Assert deprecation warning logged

Test: RecipeRunner created in server constructor
- Create server with all dependencies
- Assert server.recipeRunner non-nil
- Assert registry has 3 recipes
```

## Section 7: Integration Tests

### Tests: `internal/recipes/integration_test.go`

```
Test: Full PR impact flow with real repo
- Analyze a gorilla repo
- Run analyze_pr_impact with changed files
- Assert structural data returned
- Assert gaps noted for cross-service (no flow data)

Test: Architecture review with multiple repos
- Analyze gorilla/mux, gorilla/handlers
- Run review_architecture with both repo_ids
- Assert services inventory correct
- Assert shared_patterns detected

Test: API flow trace with real function
- Analyze gorilla/mux
- Run explain_api_flow with known function name
- Assert flow_steps returned with Mermaid

Test: Recipe composability
- Recipe A calls runner.Execute("recipe_b")
- Assert both results composed correctly

Test: Token budget compliance
- Run each recipe with budget=4000
- Assert ContextTokens <= 4000

Test: Graceful degradation without AI
- runner.ai = nil
- Run all 3 recipes
- Assert all return valid partial results with GapNotes
```
