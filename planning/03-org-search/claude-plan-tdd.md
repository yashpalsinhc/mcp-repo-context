# TDD Plan: Org Search & Hybrid Ranker

## Section 1: FTS5 Virtual Tables

### Tests

**File: `internal/storage/sqlite_fts_test.go`**

```
Test: FTS5 table created during ensureSchema
- Open new SQLiteStore
- Query: SELECT name FROM sqlite_master WHERE type='table' AND name='functions_fts'
- Assert: table exists

Test: FTS5 INSERT trigger populates index
- Store a repo context with 3 functions
- Query functions_fts directly: SELECT count(*) FROM functions_fts
- Assert: 3 rows

Test: FTS5 UPDATE trigger keeps index in sync
- Store a repo, then update a function's description
- Search FTS for new description
- Assert: finds updated function

Test: FTS5 DELETE trigger removes from index
- Store a repo, delete a function row
- Search FTS for deleted function name
- Assert: no results

Test: CASCADE DELETE cleans FTS index
- Store a repo context, verify FTS has entries
- Call StoreRepoContext again (which deletes then re-inserts)
- Search FTS — should find new entries, no stale ones
- Assert: count matches new function count

Test: SearchFunctionsFTS returns ranked results
- Store repo with functions named "GetUser", "DeleteUser", "UpdateUserProfile"
- Search for "User"
- Assert: all 3 returned, ordered by BM25 rank

Test: FTS5 query sanitization handles special characters
- Search for: error OR panic (should not be interpreted as boolean)
- Search for: "unmatched quote
- Search for: func*
- Assert: no errors, results returned as phrase matches

Test: SearchFunctionsFTS scopes to org repos only
- Store 2 repos: repo-a (in org), repo-b (not in org)
- Register org with repo-a only
- SearchFunctionsFTS with org_id
- Assert: only repo-a functions returned
```

## Section 2: Org-Scoped Search Methods

### Tests

**File: `internal/storage/sqlite_org_search_test.go`**

```
Test: FunctionRef includes RepoID
- Store a repo, search via org method
- Assert: each result has non-empty RepoID field

Test: SearchFunctionsOrg returns results across multiple repos
- Store 3 repos in same org, each with distinct functions
- SearchFunctionsOrg for a term present in all 3
- Assert: results from all 3 repos

Test: SearchByConceptOrg returns cross-repo results
- Store 2 repos with "handler" concept functions
- SearchByConceptOrg for "handler"
- Assert: results from both repos

Test: HybridSearchOrg deduplicates results
- Store repo where a function matches both FTS and concept
- HybridSearchOrg
- Assert: no duplicates in results

Test: OrgSearcher interface satisfied by SQLiteStore
- Compile-time check: var _ OrgSearcher = (*SQLiteStore)(nil)

Test: Org search with empty org returns error
- SearchFunctionsOrg with orgID that has no repos
- Assert: returns empty results, no error
```

## Section 3: RRF Hybrid Ranker

### Tests

**File: `internal/search/ranker_test.go`**

```
Test: MergeRRF with keyword-only results
- Pass keyword results, empty semantic results
- Assert: all keyword results present, scored by 1/(k+rank)

Test: MergeRRF with semantic-only results
- Pass empty keyword results, semantic results
- Assert: all semantic results present, scored by 1/(k+rank)

Test: MergeRRF boosts overlapping results
- Function "GetUser" appears at rank 3 in keyword, rank 5 in semantic
- Function "DeleteUser" appears at rank 1 in keyword only
- Assert: GetUser has higher RRF score than DeleteUser
  (1/(60+3) + 1/(60+5) > 1/(60+1))

Test: MergeRRF deduplicates by repoID:filePath:name
- Same function in both keyword and semantic
- Assert: single entry in merged results with both ranks populated

Test: MergeRRF sorts by score descending
- Multiple results with known ranks
- Assert: output sorted highest score first

Test: MergeRRF filters non-function semantic results
- Semantic results include a type entry
- Assert: type entry not in merged results

Test: MergeRRF with k=0 amplifies rank differences
- Compare scores with k=0 vs k=60
- Assert: k=0 gives much larger score differences between ranks
```

## Section 4: search_org MCP Tool

### Tests

**File: `internal/mcp/tool_search_org_test.go`**

```
Test: search_org keyword mode returns results
- Setup: org with 2 repos, functions stored
- Call: search_org with search_type="keyword"
- Assert: results from both repos in output

Test: search_org semantic mode returns results
- Setup: org with indexed vectors
- Call: search_org with search_type="semantic"
- Assert: results returned

Test: search_org hybrid mode merges results
- Setup: org with both FTS and vectors
- Call: search_org with search_type="hybrid"
- Assert: results present, RRF scores in output

Test: search_org with repo_ids filter
- Setup: org with 3 repos
- Call: search_org with repo_ids=[repo-a, repo-b]
- Assert: only results from repo-a and repo-b

Test: search_org repo_ids filter applies to semantic results
- Setup: org with 3 repos, vectors indexed
- Call: search_org search_type="semantic" repo_ids=[repo-a]
- Assert: only repo-a results (post-filtered)

Test: search_org unknown org returns error
- Call: search_org with org_id="nonexistent"
- Assert: error message contains "not found"

Test: search_org empty query returns error
- Call: search_org with query=""
- Assert: error message

Test: search_org respects token_budget
- Setup: org with many functions
- Call: search_org with token_budget=500
- Assert: output is within ~500 tokens

Test: search_org tool is registered
- List all MCP tools
- Assert: "search_org" in tool list

Test: search_org degraded mode (no vectors)
- Setup: org with FTS but no vectors indexed
- Call: search_org with search_type="hybrid"
- Assert: returns keyword results with warning about missing vectors
```

## Section 5: Progressive Disclosure Integration

### Tests

**File: `internal/mcp/progressive_org_test.go`**

```
Test: FunctionSummary includes RepoID and Score
- Create FunctionSummary with RepoID and Score
- Assert: fields populated

Test: FormatOrgSearchResult generates correct markdown
- Pass []RankedResult with 3 results
- Assert: markdown contains function names, repo names, file paths, detail_refs

Test: detail_ref uses pipe separator
- Generate detail_ref for function in repo "github.com/org/repo"
- Assert: format is "func|github.com/org/repo|path/file.go|FuncName"
- Assert: no colon-splitting ambiguity

Test: ExpandDetailRef parses pipe-separated ref
- Input: "func|github.com/org/repo|pkg/auth/handler.go|Login"
- Assert: parsed repoID="github.com/org/repo", filePath="pkg/auth/handler.go", funcName="Login"

Test: FormatOrgSearchResult respects token budget
- Pass 50 results with budget=500
- Assert: output truncated, shows "... and N more results"

Test: FormatOrgSearchResult empty results
- Pass empty []RankedResult
- Assert: output shows "No results found"
```

## Section 6: Integration Tests

### Tests

**File: `internal/integration/org_search_test.go`**

```
Test: Full pipeline — store, index, search_org hybrid
- Create 3 repos with distinct functions
- Store contexts, register as org
- Index vectors for org
- Call search_org hybrid
- Assert: results from multiple repos, RRF scored

Test: Re-indexing preserves FTS5 consistency
- Store repo, search (find results)
- Re-store same repo with different functions
- Search again
- Assert: old functions gone, new functions found

Test: FTS5 special characters in search query
- Store repo with function "errorHandler"
- Search for 'error OR handler' (should be sanitized to phrase)
- Assert: no FTS5 syntax error

Test: Keyword-only mode (no vectors)
- Store repos, register org, do NOT index vectors
- search_org search_type="keyword"
- Assert: results returned from FTS5

Test: Semantic-only mode
- Store repos, register org, index vectors
- search_org search_type="semantic"
- Assert: results returned from vector search

Test: Hybrid with repo_ids filter
- 3 repos in org
- search_org with repo_ids=[repo-a, repo-c]
- Assert: no results from repo-b

Test: Token budget truncation
- Org with many functions
- search_org with token_budget=200
- Assert: output small, truncated message present

Test: detail_ref round-trip
- search_org returns results with detail_refs
- Parse a detail_ref, call get_function_context with parsed values
- Assert: function context returned successfully

Test: Unknown org returns clear error
- search_org with nonexistent org
- Assert: error mentions org not found

Test: Empty query returns error
- search_org with query=""
- Assert: error about empty query
```
