# Section 04: explain_api_flow Recipe

## Overview

Natural language explanation of a request flow through one or more services, with Mermaid sequence diagram visualization. V1 supports function-name input only; endpoint-to-handler lookup requires 03-api-flow-tracing (produces GapNote when endpoint given without flow data).

## Dependencies

- Section 01 (Recipe interface, RecipeRunner, types)
- Section 02 (ContextAssembler for supplementary context)
- Internal: `internal/orchestrator` (Manager.GetFunctionContext, Manager.GetCallers)

## Recipe Registration

Register as "explain_api_flow" in `DefaultRegistry()`.

## Input Schema

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| function_name | string | yes (v1) | - | Entry function name |
| endpoint | string | no | "" | HTTP endpoint (future: resolved to handler) |
| repo_id | string | yes | - | Repository ID |
| org_id | string | no | "" | Org ID for cross-service tracing |
| token_budget | int | no | 8000 | Max context tokens |

V1: `function_name` is required. If only `endpoint` is provided and flow data is unavailable, return GapNote suggesting function_name.

## Execution Steps

### Step 1: Find Entry Point

If `function_name` provided:
- Call `runner.Manager().GetFunctionContext(repoID, "", functionName)` (search across files)
- If not found, return error: "function not found: {name}"

If `endpoint` provided but no flow data:
- Add GapNote: "Endpoint-to-handler lookup requires 03-api-flow-tracing. Use function_name input instead."
- If function_name also provided, continue with function_name
- If only endpoint, return error with GapNote

Store entry point: function name, file path, line number, behavior summary.

### Step 2: Trace Internal Flow

From the entry function, follow the call graph depth-first:

1. Get the function's callees from function context.
2. For each callee, get its function context (name, file, behavior, side effects).
3. Recurse into callee's callees.
4. Depth limit: 10 functions to prevent infinite loops.
5. Track visited set to handle cycles (if A calls B calls A, stop at revisit).

Build a list of `FlowStep`:

```
type FlowStep struct {
    Service    string   // repo name or "same"
    Function   string
    File       string
    Line       int
    Action     string   // from behavior summary
    SideEffects []string // "db_query: SELECT ...", "http_call: POST /api/..."
}
```

### Step 3: Trace Cross-Service Hops

Only if `org_id` provided and flow tracing data available.

**V1:** Flow tracing not available. Add GapNote:
```
GapNote{
    Section: "cross_service_hops",
    Reason: "Cross-service tracing requires 03-api-flow-tracing",
    Suggestion: "Implement 03-api-flow-tracing for cross-repo endpoint matching",
}
```

Set `Data["cross_service_hops"] = []`.

### Step 4: Build Mermaid Diagram

Generate a Mermaid sequence diagram from the flow steps:

```
sequenceDiagram
    participant Client
    participant Handler as auth-service:HandleLogin
    participant DB as Database

    Client->>Handler: request
    Handler->>DB: SELECT user WHERE email=?
    DB-->>Handler: user row
    Handler-->>Client: response
```

Rules:
- Each unique service/component is a participant
- DB queries become interactions with "Database" participant
- HTTP calls become interactions with target service
- Use `-->>` for responses (dashed)
- Use `->>` for requests (solid)
- Include side effect details as note labels

Handle edge cases:
- Empty flow (just entry point): show Client -> Handler -> Client
- Very long flow (>15 steps): truncate with "..." note

### Step 5: AI Explanation

If `runner.AI()` available:
- Build prompt with flow steps, side effects, and Mermaid diagram
- Ask for natural language walkthrough: "When a request hits {function}..."
- Use `CompleteRaw(ctx, prompt, 1000)`
- Store in `RecipeResult.Analysis`

Without AI:
- Leave Analysis empty
- Data is still complete (flow steps + Mermaid)

## Output

Data keys:
- `entry_point` — `{function, file, line}`
- `flow_steps` — list of FlowStep
- `cross_service_hops` — list (or gap)
- `mermaid` — Mermaid diagram string
- `data_transformations` — extracted from side effects (DB reads -> business logic -> DB writes)

## Tests

### `internal/recipes/api_flow_test.go`

**Test: Known function traces flow**
- Mock GetFunctionContext returns function with 2 callees
- Assert flow_steps has 3 entries (entry + 2 callees)
- Assert entry_point populated

**Test: Unknown function returns error**
- Mock returns not found
- Assert error "function not found"

**Test: Endpoint without flow data gives GapNote**
- Input: endpoint="POST /login", function_name=""
- Assert GapNote suggesting function_name

**Test: Mermaid diagram generated**
- Mock 3-step flow
- Assert mermaid contains "sequenceDiagram"
- Assert participant names present

**Test: Depth limit prevents infinite loop**
- Mock circular call graph (A -> B -> A)
- Assert flow terminates
- Assert visited functions not repeated

**Test: Cross-service without flow data**
- Input: org_id set
- Assert GapNote for cross_service_hops

**Test: AI explanation generated**
- Mock CompleteRaw returns narrative
- Assert Analysis non-empty

**Test: Without AI, structural data only**
- runner.AI()=nil
- Assert Analysis empty, Data present

## File Inventory

| File | Purpose |
|------|---------|
| `internal/recipes/api_flow.go` | explain_api_flow recipe implementation |
| `internal/recipes/mermaid.go` | Mermaid diagram generation helpers |
| `internal/recipes/api_flow_test.go` | All API flow tests |

## Acceptance Criteria

1. Function-name input traces call graph correctly
2. Endpoint input without flow data produces helpful GapNote
3. Depth limit prevents infinite loops on circular calls
4. Mermaid diagram generated with correct syntax
5. AI narrative generated when available
6. Graceful degradation without AI
7. All 8 tests pass
