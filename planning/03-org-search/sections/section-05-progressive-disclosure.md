# Section 5: Progressive Disclosure Integration

## Overview

Wire up the existing progressive.go infrastructure for search_org output. Update FunctionSummary with org-aware fields, implement FormatOrgSearchResult for token-budgeted markdown output, change detail_ref to use pipe separator, and add ExpandDetailRef for ref parsing.

## Dependencies

- Section 3: RRF Hybrid Ranker (RankedResult type)
- Section 4: search_org MCP Tool (uses FormatOrgSearchResult)

## Tests First

### File: `internal/mcp/progressive_org_test.go` (new)

```
Test: FunctionSummary includes RepoID and Score fields
- Create FunctionSummary with RepoID="github.com/org/repo" and Score=0.0312
- Assert: fields are populated and accessible

Test: MakeOrgFunctionDetailRef uses pipe separator
- Input: repoID="github.com/org/user-service", filePath="pkg/handlers/user.go", funcName="GetUser"
- Assert: returns "func|github.com/org/user-service|pkg/handlers/user.go|GetUser"

Test: MakeOrgFunctionDetailRef handles paths with special characters
- Input: repoID="github.com/org/my-service", filePath="pkg/auth/jwt_validator.go", funcName="Validate"
- Assert: "func|github.com/org/my-service|pkg/auth/jwt_validator.go|Validate"

Test: ExpandDetailRef parses pipe-separated ref correctly
- Input: "func|github.com/org/user-service|pkg/handlers/user.go|GetUser"
- Assert: refType="func", repoID="github.com/org/user-service", filePath="pkg/handlers/user.go", funcName="GetUser"

Test: ExpandDetailRef with old colon format returns error
- Input: "func:github.com/org/repo:path/file.go:Func"
- Assert: error about invalid format (not enough pipe-separated parts)

Test: ExpandDetailRef with invalid ref returns error
- Input: "invalid-ref-string"
- Assert: error

Test: FormatOrgSearchResult generates correct markdown header
- Input: query="User", orgID="my-org", 5 results from 2 repos
- Assert: output starts with "## Search Results: \"User\" across my-org"
- Assert: contains "Found 5 results across 2 repos"

Test: FormatOrgSearchResult formats individual results correctly
- Input: single RankedResult for GetUser from user-service at pkg/handlers/user.go:45
- Assert: output contains "**GetUser**"
- Assert: output contains "(user-service)" (short repo name)
- Assert: output contains "`pkg/handlers/user.go:45`"
- Assert: output contains summary text
- Assert: output contains detail_ref with pipe separator

Test: FormatOrgSearchResult respects token budget
- Input: 50 RankedResults, token_budget=500
- Assert: output is within ~500 tokens (±100)
- Assert: output ends with "... and N more results" truncation message
- Assert: shows "Token budget: X/500"

Test: FormatOrgSearchResult with budget larger than all results
- Input: 3 RankedResults, token_budget=4000
- Assert: all 3 results shown
- Assert: no truncation message

Test: FormatOrgSearchResult with empty results
- Input: empty []RankedResult
- Assert: output contains "No results found"

Test: FormatOrgSearchResult extracts short repo name
- Input: repoID="github.com/myorg/user-service"
- Assert: display name is "user-service" (last path segment)

Test: FormatOrgSearchResult with semantic-only results (incomplete FunctionRef)
- Input: RankedResult where Line=0 and Signature=""
- Assert: line number omitted from display
- Assert: no error

Test: Token estimation accuracy
- Format 10 results, measure actual token count
- Assert: estimated tokens within 50% of actual
```

## Implementation Details

### 1. FunctionSummary Extension

Update the `FunctionSummary` struct in `internal/mcp/progressive.go`:

Add two new fields:
- `RepoID string` — the repository ID for org-scoped results
- `Score float64` — the RRF score or similarity score

These fields are only populated for org search results. Existing per-repo search usage is unaffected.

### 2. Detail Ref Format Change

Change the detail_ref separator from colon (`:`) to pipe (`|`). This avoids ambiguity with colons in repository IDs like `github.com/org/repo`.

New format: `func|{repoID}|{filePath}|{funcName}`

Add new function:
```go
func MakeOrgFunctionDetailRef(repoID, filePath, funcName string) string
```

Returns: `"func|" + repoID + "|" + filePath + "|" + funcName`

The existing `MakeFunctionDetailRef` (colon-based) remains for backward compatibility with per-repo search. Only org search uses the pipe-based format.

### 3. ExpandDetailRef

```go
func ExpandDetailRef(ref string) (refType, repoID, filePath, funcName string, err error)
```

Implementation:
1. Split on `|` (pipe)
2. Expect exactly 4 parts: [refType, repoID, filePath, funcName]
3. If fewer than 4 parts, return error "invalid detail_ref format"
4. Return parsed components

This function is used by the search_org handler to dispatch detail_ref expansion to `get_function_context`.

### 4. FormatOrgSearchResult

```go
func FormatOrgSearchResult(query, orgID string, results []search.RankedResult, tokenBudget int) string
```

Implementation:

**Step 1: Header**
```
## Search Results: "{query}" across {orgID}

Found {total} results across {repoCount} repos (showing top {shown})
```

Count unique repos from results.

**Step 2: Estimate tokens per result**
Each result line is approximately:
```
N. **FuncName** (repo-name) - `path/to/file.go:line`
   Summary text here
   → detail: `func|repoID|path|funcName`
```

Estimate ~100 tokens per result (conservative). Use the token counter from `tokens/` package for more accurate estimation if available.

**Step 3: Greedy fill with budget**
1. Calculate header tokens (~50 tokens)
2. Remaining budget = tokenBudget - header tokens
3. For each result (in RRF score order):
   - Format the result line
   - Estimate its token count
   - If adding it would exceed remaining budget, stop
   - Add to output, subtract tokens from remaining
4. If results were truncated: append `"\n... and {remaining} more results. Use detail_ref to expand individual results."`

**Step 4: Footer**
```
Token budget: {used}/{tokenBudget}
```

**Short repo name extraction:**
```go
func shortRepoName(repoID string) string
```
Extract last path segment: `github.com/org/user-service` → `user-service`. Split on `/`, take last element.

**Result formatting for each item:**
```
{rank}. **{funcName}** ({shortRepo}) - `{filePath}:{line}`
   {summary}
   → detail: `{detailRef}`
```

If line is 0 (unknown), omit the `:line` suffix. If summary is empty, show `(no summary)`.

### 5. Token Counter Integration

Use the existing `tokens.TokenCounter` for estimation:
```go
counter := tokens.NewTokenCounter()
estimate := counter.Count(formattedLine)
```

This uses the chars/4 approximation. For the greedy fill, track cumulative token count and stop when budget would be exceeded.

## Error Handling

- Invalid detail_ref format: return descriptive error with expected format
- Empty results: return "No results found" message (not an error)
- Token budget too small for even one result: return header + "Token budget too small to display results"
- Nil/zero token budget: use default of 4000

## File Summary

| File | Action |
|------|--------|
| `internal/mcp/progressive.go` | Modify: add RepoID/Score to FunctionSummary, add MakeOrgFunctionDetailRef, add ExpandDetailRef |
| `internal/mcp/progressive_format.go` | New: FormatOrgSearchResult, shortRepoName, token-budgeted formatting |
| `internal/mcp/progressive_org_test.go` | New: tests for org progressive disclosure |
