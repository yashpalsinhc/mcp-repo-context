# Section 4: search_org MCP Tool

## Overview

New MCP tool `search_org` that searches across all repositories in an organization. Supports keyword, semantic, and hybrid (RRF-merged) search modes. Includes org validation, repo_ids filtering, token budgeting, and progressive disclosure output.

## Dependencies

- Section 1: FTS5 Virtual Tables (SearchFunctionsFTS)
- Section 2: Org-Scoped Search Methods (OrgSearcher interface)
- Section 3: RRF Hybrid Ranker (MergeRRF)

## Tests First

### File: `internal/mcp/tool_search_org_test.go` (new)

```
Test: search_org tool is registered in tool list
- Get all tool definitions from MCP server
- Assert: "search_org" exists in tool list
- Assert: has required parameters (org_id, query)
- Assert: has optional parameters (search_type, repo_ids, max_results, token_budget)

Test: search_org keyword mode returns results
- Setup: store 2 repos in org, each with functions containing "User"
- Call: toolSearchOrg with org_id, query="User", search_type="keyword"
- Assert: results from both repos in output markdown
- Assert: output contains function names and file paths

Test: search_org semantic mode returns results
- Setup: store 2 repos in org, index vectors with org association
- Call: toolSearchOrg with org_id, query="user authentication", search_type="semantic"
- Assert: results returned with similarity-based ranking

Test: search_org hybrid mode merges keyword and semantic
- Setup: store repos in org with FTS data and vectors indexed
- Call: toolSearchOrg with org_id, query="User", search_type="hybrid"
- Assert: output contains results
- Assert: results that appear in both keyword and semantic are ranked higher

Test: search_org default search_type is hybrid
- Call: toolSearchOrg with org_id and query only (no search_type)
- Assert: behaves same as hybrid (both keyword and semantic executed)

Test: search_org with repo_ids filter — keyword
- Setup: org with 3 repos (a, b, c)
- Call: toolSearchOrg with repo_ids=[a, c], search_type="keyword"
- Assert: only results from repo-a and repo-c
- Assert: no results from repo-b

Test: search_org with repo_ids filter — semantic
- Setup: org with 3 repos, all indexed
- Call: toolSearchOrg with repo_ids=[a], search_type="semantic"
- Assert: only results from repo-a (post-filtered)

Test: search_org with repo_ids not in org returns error
- Setup: org with repos [a, b]
- Call: toolSearchOrg with repo_ids=[a, unknown-repo]
- Assert: error about repo not in org

Test: search_org unknown org returns error
- Call: toolSearchOrg with org_id="nonexistent"
- Assert: error message contains "not found" or "unknown"

Test: search_org empty query returns error
- Call: toolSearchOrg with query=""
- Assert: error about empty query

Test: search_org respects max_results
- Setup: org with 20+ matching functions
- Call: toolSearchOrg with max_results=5
- Assert: at most 5 results in output

Test: search_org respects token_budget
- Setup: org with many matching functions
- Call: toolSearchOrg with token_budget=500
- Assert: output token count <= ~500 (approximate)
- Assert: truncation message present if results were cut

Test: search_org degraded mode — no vectors indexed
- Setup: org with repos stored but NOT vector indexed
- Call: toolSearchOrg with search_type="hybrid"
- Assert: returns keyword results
- Assert: output contains warning about semantic search unavailable

Test: search_org with no matching results
- Setup: org with repos
- Call: toolSearchOrg with query="xyznonexistent123"
- Assert: output says "No results found"
- Assert: no error
```

## Implementation Details

### 1. Tool Definition

Register `search_org` in the MCP tools list (same file/pattern as existing tools like `search_context`, `semantic_search`).

Tool schema:
- `org_id` (string, required): Organization identifier
- `query` (string, required): Search query
- `search_type` (string, optional, default "hybrid"): One of "keyword", "semantic", "hybrid"
- `repo_ids` (array of string, optional): Filter to specific repos within the org
- `max_results` (integer, optional, default 20): Maximum number of results
- `token_budget` (integer, optional, default 4000): Maximum tokens in response

### 2. Handler Function

```go
func (s *Server) toolSearchOrg(ctx context.Context, params map[string]interface{}) (string, error)
```

Handler flow:

**Step 1: Validate inputs**
- Extract and validate org_id (required, non-empty)
- Extract and validate query (required, non-empty)
- Extract search_type (default "hybrid"), validate against allowed values
- Extract repo_ids (optional), max_results (default 20), token_budget (default 4000)

**Step 2: Validate org and get repos**
- Call org store to get org by ID. If not found, return error "Organization '{org_id}' not found"
- Get repo list for org via org store's GetOrgRepos method
- If repo_ids provided, validate each is in the org's repo list. Return error for any not found.
- Determine effective repo list (filtered or full)

**Step 3: Execute search based on search_type**

For "keyword":
- Call `store.SearchFunctionsOrg(ctx, orgID, query, maxResults)`
- Convert results to `[]RankedResult` (keyword-only, no semantic rank)

For "semantic":
- Call `semanticSearch.SearchByOrg(ctx, query, orgID, maxResults)`
- If repo_ids filter active, post-filter results: keep only those where Record.RepoID is in the repo_ids list
- Convert results to `[]RankedResult` (semantic-only, no keyword rank)

For "hybrid":
- Run keyword search: `store.SearchFunctionsOrg(ctx, orgID, query, maxResults)`
- Run semantic search: `semanticSearch.SearchByOrg(ctx, query, orgID, maxResults)`
- If semantic search fails (no index, no vectors): log warning, proceed with keyword-only
- If repo_ids filter active, post-filter semantic results
- Call `search.MergeRRF(keywordResults, semanticResults, search.DefaultRRFK)`

**Step 4: Apply token budget and format**
- Pass results to `FormatOrgSearchResult` (from Section 5)
- The formatter applies token budgeting and generates markdown with detail_refs
- Return formatted string

### 3. Semantic Search Post-Filtering

When `repo_ids` is provided and search_type includes semantic search, the semantic `SearchByOrg` method searches by org_id (not individual repo_ids). To narrow results:

```go
func filterSemanticByRepos(results []vectors.SearchResult, repoIDs map[string]bool) []vectors.SearchResult
```

Simple filter: iterate results, keep those where `Record.RepoID` is in the allowed set.

### 4. Degraded Mode Handling

When semantic search is unavailable (no vectors indexed for org):
- `SearchByOrg` returns empty results or an error
- Handler catches this and proceeds with keyword-only results
- Appends warning to output: `"Note: Semantic search unavailable for this org. Results are keyword-only. Run index_repository for each repo to enable semantic search."`

### 5. Server Dependencies

The handler needs access to:
- `SQLiteStore` (for keyword search via OrgSearcher)
- `SemanticSearch` (for vector search via SearchByOrg)
- `OrgStore` (for org validation and repo list)
- `Budgeter` (for token budgeting)

These should already be available on the MCP Server struct. If SemanticSearch is nil (not configured), treat semantic search as unavailable.

## Error Handling

- Unknown org: `"Organization '%s' not found. Use register_org to create it."`
- Repo not in org: `"Repository '%s' is not in organization '%s'"`
- Empty query: `"query parameter is required and cannot be empty"`
- Invalid search_type: `"search_type must be one of: keyword, semantic, hybrid"`
- Semantic search unavailable: degrade gracefully with warning (not an error)
- FTS5 unavailable: falls back to LIKE search (handled in SearchFunctionsOrg)

## File Summary

| File | Action |
|------|--------|
| `internal/mcp/tools.go` | Modify: register search_org tool definition |
| `internal/mcp/tool_search_org.go` | New: toolSearchOrg handler, filterSemanticByRepos helper |
| `internal/mcp/tool_search_org_test.go` | New: search_org tool tests |
