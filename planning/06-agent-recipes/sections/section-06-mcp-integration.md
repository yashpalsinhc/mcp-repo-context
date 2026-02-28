# Section 06: MCP Tool Integration & Pattern Deprecation

## Overview

Expose recipes as MCP tools, add generic `execute_recipe` and `list_recipes` tools, create RecipeRunner in the MCP server constructor, and deprecate the old `execute_pattern` system with backward compatibility.

## Dependencies

- Section 01 (RecipeRunner, Registry)
- Sections 03-05 (all three recipes registered in DefaultRegistry)
- Internal: `internal/mcp` (Server, tool registration)

## RecipeRunner in MCP Server

### Server Constructor Update

In `internal/mcp/server.go`, add `recipeRunner *recipes.RecipeRunner` field to the `Server` struct.

In `NewServer()`:
1. Create `recipes.DefaultRegistry()` (pre-populated with all 3 recipes)
2. Build RunnerOptions from available services:
   - `recipes.WithAI(server.aiProvider)` if aiProvider != nil
   - `recipes.WithVectors(server.semanticSearch)` if semanticSearch != nil (wrap as VectorSearcher)
   - `recipes.WithBudgeter(server.budgeter)`
   - `recipes.WithRegistry(registry)`
   - `recipes.WithLogger(server.logger)`
3. Create `recipes.NewRecipeRunner(server.manager, server.config.OrgManager, opts...)`
4. Store as `server.recipeRunner`

## New MCP Tools

### analyze_pr_impact

**Tool name:** `analyze_pr_impact`
**Description:** "Comprehensive PR impact analysis with cross-repo awareness and AI risk assessment. Returns changed functions, affected callers, risk level, and gaps for unavailable data."

**Input schema:**
```json
{
  "repo_id": { "type": "string", "required": true },
  "changed_files": { "type": "array", "items": { "path": "string", "change_type": "string" }, "required": true },
  "org_id": { "type": "string" },
  "token_budget": { "type": "integer", "default": 8000 },
  "include_ai": { "type": "boolean", "default": true }
}
```

**Handler:** Parse MCP input to RecipeInput, call `server.recipeRunner.Execute(ctx, "analyze_pr_impact", input)`, format result.

### explain_api_flow

**Tool name:** `explain_api_flow`
**Description:** "Trace and explain the execution flow of a function through the codebase with Mermaid visualization. Shows call chain, side effects, and cross-service hops."

**Input schema:**
```json
{
  "function_name": { "type": "string", "required": true },
  "repo_id": { "type": "string", "required": true },
  "endpoint": { "type": "string" },
  "org_id": { "type": "string" },
  "token_budget": { "type": "integer", "default": 8000 }
}
```

### review_architecture

**Tool name:** `review_architecture`
**Description:** "Assess architecture across multiple repositories or an org. Returns service inventory, shared patterns, health indicators, and AI recommendations."

**Input schema:**
```json
{
  "org_id": { "type": "string" },
  "repo_ids": { "type": "array", "items": "string" },
  "token_budget": { "type": "integer", "default": 12000 },
  "focus_areas": { "type": "array", "items": "string" }
}
```

### execute_recipe

**Tool name:** `execute_recipe`
**Description:** "Execute any registered recipe by name. Use list_recipes to see available recipes and their input schemas."

**Input schema:**
```json
{
  "recipe_name": { "type": "string", "required": true },
  "params": { "type": "object", "required": true },
  "output_format": { "type": "string", "enum": ["markdown", "json"], "default": "markdown" }
}
```

**Handler:**
1. Look up recipe via `server.recipeRunner.Execute(ctx, recipeName, input)`
2. Format based on output_format

### list_recipes

**Tool name:** `list_recipes`
**Description:** "List all available recipes with their descriptions and input schemas."

**Input schema:** (no required inputs)

**Handler:** Call `server.recipeRunner.Registry().List()`, format as markdown table or JSON.

## Output Formatting

### Markdown Mode (default for MCP)

```markdown
# analyze_pr_impact

## Changed Functions
- **HandleLogin** (pkg/handlers/auth.go:42) - Validates user credentials
  - Callers: LoginController, TestHandleLogin
  - Side effects: db_query (SELECT user)

## Risk Assessment
**Level:** medium (confidence: 0.8)
Reasoning: 5 callers affected, 1 DB query changed

## Gaps
- Cross-service impact: requires 03-api-flow-tracing
- Dependency impact: requires 02-dependency-graph

## Sources
- pkg/handlers/auth.go:42 (HandleLogin behavior)
- pkg/handlers/auth.go:87 (ValidateCredentials)

---
Context tokens: 2150 | AI tokens: 450 | Duration: 342ms | Confidence: 0.56
```

### JSON Mode (for REST API)

Raw `RecipeResult` serialized as JSON.

## Pattern Deprecation

### Mark execute_pattern as deprecated

Update the existing `execute_pattern` tool description in `tools.go`:

Old: "Execute a pre-defined pattern of tool calls..."
New: "**DEPRECATED: Use execute_recipe instead.** Execute a pre-defined pattern of tool calls..."

### Add deprecation logging

In the `execute_pattern` handler, add a log warning:
```
server.logger.Warn("execute_pattern is deprecated, use execute_recipe instead", "pattern", patternName)
```

### Pattern-to-recipe mapping

Document in tool description:
- `impact_analysis` → use `analyze_pr_impact`
- `search_with_context` → use `get_context_budgeted`
- `multi_repo_search` → use `search_context` with org scope
- `find_and_expand` → use `search_context` + `get_function_context`

### Keep internal/compose/ functional

Do NOT delete the compose package. It remains functional for backward compatibility. Removal is a future cleanup task.

## Tests

### `internal/mcp/recipe_tools_test.go`

**Test: analyze_pr_impact tool invokes recipe**
- Call tool handler with valid input
- Assert recipe executed, output contains structured data

**Test: execute_recipe routes to correct recipe**
- Call with recipe_name="analyze_pr_impact"
- Assert correct recipe invoked

**Test: execute_recipe with unknown recipe returns error**
- Assert error with available names

**Test: list_recipes returns all registered**
- Assert 3 recipes listed with schemas

**Test: output_format=json returns raw JSON**
- Assert valid JSON RecipeResult

**Test: output_format=markdown returns formatted**
- Assert section headers present

**Test: execute_pattern deprecated but functional**
- Call execute_pattern
- Assert works (backward compat)
- Assert deprecation warning logged

**Test: RecipeRunner created in server constructor**
- Create server with all dependencies
- Assert recipeRunner non-nil, registry has 3 recipes

## File Inventory

| File | Purpose |
|------|---------|
| `internal/mcp/recipe_tools.go` | MCP tool handlers for all recipe tools |
| `internal/mcp/recipe_format.go` | Markdown and JSON output formatters |
| `internal/mcp/server.go` | Updated with recipeRunner field |
| `internal/mcp/tools.go` | Updated execute_pattern description |
| `internal/mcp/recipe_tools_test.go` | All MCP tool tests |

## Acceptance Criteria

1. All 5 new MCP tools registered and functional
2. RecipeRunner created in server constructor with all available services
3. Markdown output formatted with sections
4. JSON output is raw RecipeResult
5. execute_pattern still works with deprecation warning
6. list_recipes shows all 3 recipes with schemas
7. All 8 tests pass
