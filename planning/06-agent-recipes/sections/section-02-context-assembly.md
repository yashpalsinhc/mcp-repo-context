# Section 02: Hybrid Context Assembly

## Overview

Build a ContextAssembler that combines structural data, vector search results, and keyword search into a unified context within a token budget. The assembler runs multiple passes with configurable budget allocation, deduplicates results, and tags each item by source type.

## Dependencies

- Section 01 (RecipeRunner, VectorSearcher, BudgetAllocation)
- Internal: `internal/orchestrator` (Manager), `internal/tokens` (Budgeter, TokenCounter)

## ContextAssembler Struct

```
type ContextAssembler struct {
    manager  orchestrator.Manager
    vectors  VectorSearcher       // optional (interface from section 1)
    budgeter *tokens.Budgeter
}
```

Constructor: `NewContextAssembler(manager, vectors, budgeter)`.

## AssemblyRequest

```
type AssemblyRequest struct {
    RepoID     string
    Query      string
    Budget     int              // total token budget
    Allocation BudgetAllocation // per-pass allocation
    Filters    AssemblyFilters  // optional narrowing
}

type AssemblyFilters struct {
    FilePaths     []string  // limit to specific files
    FunctionNames []string  // limit to specific functions
    Concept       string    // filter by concept
}
```

## AssembledContext

```
type AssembledContext struct {
    Items       []ContextItem
    TotalTokens int
    PassBreakdown map[string]int  // "structural": 1500, "vector": 800, etc.
}

type ContextItem struct {
    Type        string  // "function", "type", "file"
    Name        string
    FilePath    string
    Line        int
    Summary     string  // behavior summary or description
    Details     string  // additional context (signature, side effects, etc.)
    Source      string  // "structural", "vector", "keyword"
    Relevance   float64 // 0-1 score
    TokenCost   int     // estimated tokens for this item
}
```

## Assembly Strategy

### Assemble Method

```
func (a *ContextAssembler) Assemble(ctx context.Context, req AssemblyRequest) (*AssembledContext, error)
```

1. Calculate per-pass budgets from allocation. If vectors is nil, redistribute vector budget: structural gets +60% of vector share, keyword gets +40%.

2. **Structural pass:** Get function contexts for the repo. If filters specify function names, get those directly via `manager.GetFunctionContext()`. Otherwise, use `manager.SearchContext()` with the query. Score by keyword relevance (reuse existing scoring from `tokens.Budgeter`). Select greedily within structural budget. Tag source="structural".

3. **Vector pass:** If VectorSearcher available, call `SearchFunctions(query, repoID, limit=20)`. Convert results to ContextItems. Deduplicate against structural results (match by name+filePath). Select within vector budget. Tag source="vector".

4. **Keyword pass:** Use `manager.SearchContext()` with different keyword extraction (split query into individual terms). Deduplicate against items already selected. Fill keyword budget. Tag source="keyword".

5. **Metadata pass:** Include repo architecture summary (truncated to metadata budget). Provides framing context.

6. Sort all items by relevance descending. Build PassBreakdown map. Return AssembledContext.

### Deduplication

Items are keyed by `(name, filePath)`. When a duplicate is found:
- Keep the one with higher relevance score
- Note all discovery sources (e.g., "structural+vector")
- Only count token cost once

### Budget Overflow

If any pass exceeds its allocation, the Budgeter's greedy selection truncates automatically. No items are added beyond the per-pass budget. Unused budget from one pass does NOT carry over to the next (keeps behavior predictable).

## Tests

### `internal/recipes/assembler_test.go`

**Test: Assemble with all sources available**
- Mock manager returns 5 functions, mock vector returns 3 results
- Assemble with budget=4000
- Assert result has items from structural, vector, keyword sources
- Assert TotalTokens <= 4000

**Test: Assemble without vector search**
- vectors=nil
- Assert structural gets larger share
- Assert no vector-tagged items
- Assert PassBreakdown has no "vector" key

**Test: Assemble deduplicates across passes**
- Function "CreateUser" returned by both structural and vector
- Assert single ContextItem for CreateUser
- Assert source notes both passes

**Test: Assemble respects budget allocation**
- allocation={0.5, 0.3, 0.1, 0.1}, budget=4000
- Assert structural items use approximately 2000 tokens

**Test: Assemble with filters narrows results**
- Filters with FunctionNames=["HandleLogin"]
- Assert only HandleLogin-related items returned

**Test: Assemble tags items by source type**
- Assert each item has valid source string

**Test: Empty query returns metadata only**
- Query=""
- Assert only metadata items returned

## File Inventory

| File | Purpose |
|------|---------|
| `internal/recipes/assembler.go` | ContextAssembler struct and Assemble method |
| `internal/recipes/context_item.go` | AssemblyRequest, AssembledContext, ContextItem types |
| `internal/recipes/assembler_test.go` | All assembler tests |

## Acceptance Criteria

1. Assembler combines structural + vector + keyword data within budget
2. Vector budget redistributed when VectorSearcher is nil
3. Deduplication merges items found by multiple passes
4. Items tagged by source type
5. Budget overflow handled by truncation
6. PassBreakdown tracks per-pass token usage
7. All 7 tests pass
