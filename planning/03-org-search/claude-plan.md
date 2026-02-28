# Implementation Plan: Org Search & Hybrid Ranker

## Overview

This plan adds an org-wide search tool (`search_org`) that combines keyword search (FTS5) and semantic search (vector similarity) across all repos in an organization. Results are merged using Reciprocal Rank Fusion (RRF), token-budgeted, and returned with progressive disclosure refs.

## Current Architecture

### What Exists
- **Keyword search** (`sqlite_search.go`): LIKE-based SearchFunctions, SearchByConcept, HybridSearch — all per-repo scoped
- **Semantic search** (`vectors/search.go`): SearchByOrg exists — queries vectors WHERE org_id = ?, brute-force cosine similarity
- **Token budgeter** (`tokens/`): TokenCounter (chars/4), Budgeter with greedy fill and SummarizeFunction fallback
- **Progressive disclosure** (`mcp/progressive.go`): FunctionSummary, SearchResultCompact, detail_ref format — exists but unused
- **No FTS5**: schema uses LIKE queries. Indexes on name_lower columns.
- **All SearchableStore methods** require repoID parameter — no org-scoped methods

### What's Missing
1. FTS5 virtual tables for efficient full-text search
2. Org-scoped keyword search methods
3. RRF merging of keyword and semantic results
4. search_org MCP tool
5. Token-budgeted output formatting using progressive disclosure

## Section-by-Section Plan

### Section 1: FTS5 Virtual Tables

**Goal:** Add FTS5 virtual tables populated alongside existing storage, enabling efficient full-text search.

**New migration in `ensureSchema()`:**
```sql
CREATE VIRTUAL TABLE IF NOT EXISTS functions_fts USING fts5(
    name, signature, description, summary,
    content=functions, content_rowid=id
);
```

This is a content-synced FTS5 table — it references the `functions` table and uses its rowid. We populate it with triggers or explicit INSERT statements during StoreRepoContext.

**Triggers for auto-sync:**
```sql
CREATE TRIGGER IF NOT EXISTS functions_fts_insert AFTER INSERT ON functions BEGIN
    INSERT INTO functions_fts(rowid, name, signature, description, summary)
    VALUES (new.id, new.name, new.signature, new.description, json_extract(new.behavior_json, '$.summary'));
END;

CREATE TRIGGER IF NOT EXISTS functions_fts_update AFTER UPDATE ON functions BEGIN
    INSERT INTO functions_fts(functions_fts, rowid, name, signature, description, summary)
    VALUES ('delete', old.id, old.name, old.signature, old.description, json_extract(old.behavior_json, '$.summary'));
    INSERT INTO functions_fts(rowid, name, signature, description, summary)
    VALUES (new.id, new.name, new.signature, new.description, json_extract(new.behavior_json, '$.summary'));
END;

CREATE TRIGGER IF NOT EXISTS functions_fts_delete AFTER DELETE ON functions BEGIN
    INSERT INTO functions_fts(functions_fts, rowid, name, signature, description, summary)
    VALUES ('delete', old.id, old.name, old.signature, old.description, json_extract(old.behavior_json, '$.summary'));
END;
```

**CASCADE DELETE handling:** SQLite triggers do NOT fire for rows deleted via ON DELETE CASCADE. Since `StoreRepoContext` does `DELETE FROM repos WHERE id = ?` which cascades through files to functions, the FTS5 delete trigger will not fire. To handle this, add explicit FTS cleanup in `StoreRepoContext` BEFORE the cascade delete:
```sql
DELETE FROM functions_fts WHERE rowid IN (
    SELECT f.id FROM functions f
    JOIN files fi ON f.file_id = fi.id
    WHERE fi.repo_id = ?
);
```

**FTS5 query sanitization:** User queries must be sanitized before passing to MATCH. The approach: strip internal double quotes, then wrap the entire query in double quotes for phrase matching. This prevents FTS5 operator injection (AND, OR, NOT, NEAR, *, etc.) while still returning relevant results. For advanced users, a future `raw_query` parameter could bypass sanitization.

**New search method:**
```go
func (s *SQLiteStore) SearchFunctionsFTS(ctx context.Context, orgID string, query string, limit int) ([]FunctionRef, error)
```
SQL uses a subquery for repo filtering:
```sql
SELECT f.name, f.signature, fi.path, f.line, json_extract(f.behavior_json, '$.summary'),
       fi.repo_id, rank
FROM functions_fts fts
JOIN functions f ON f.id = fts.rowid
JOIN files fi ON f.file_id = fi.id
WHERE functions_fts MATCH ?
AND fi.repo_id IN (SELECT repo_id FROM org_repos WHERE org_id = ?)
ORDER BY rank
LIMIT ?
```

The `MATCH` syntax supports standard FTS5 query language. The `rank` is FTS5's BM25 rank. The subquery approach avoids dynamic SQL placeholder generation and handles any number of repos.

### Section 2: Org-Scoped Search Methods

**Goal:** Add org-scoped search methods via a new `OrgSearcher` interface, separate from the existing `SearchableStore` to avoid breaking existing implementations.

**New interface:**
```go
type OrgSearcher interface {
    SearchFunctionsOrg(ctx context.Context, orgID string, query string, limit int) ([]FunctionRef, error)
    SearchByConceptOrg(ctx context.Context, orgID string, concept string, limit int) ([]FunctionRef, error)
    HybridSearchOrg(ctx context.Context, orgID string, query string, limit int) ([]FunctionRef, error)
}
```

All methods use `orgID` parameter and filter via subquery `fi.repo_id IN (SELECT repo_id FROM org_repos WHERE org_id = ?)`. This is cleaner than accepting `[]string` repo IDs and avoids dynamic SQL.

**FunctionRef extension:** Add `RepoID string` field to the `FunctionRef` struct. All org-scoped methods populate it from `fi.repo_id` in the JOIN. This is needed for the RRF merge key (`repoID:filePath:name`) and for progressive disclosure output.

**SearchFunctionsOrg:** Uses SearchFunctionsFTS with FTS5. Falls back to LIKE-based search across org repos if FTS5 not available.

**SearchByConceptOrg:** Same as SearchByConcept but with org-scoped repo filter subquery.

**HybridSearchOrg:** Combines FTS5 search + concept search across org repos with dedup.

### Section 3: RRF Hybrid Ranker

**Goal:** Merge keyword and semantic search results using Reciprocal Rank Fusion.

**New file: `internal/search/ranker.go`**

```go
type RankedResult struct {
    FunctionRef  storage.FunctionRef  // includes RepoID field
    KeywordRank  int     // rank from keyword search (0 = not in results)
    SemanticRank int     // rank from semantic search (0 = not in results)
    RRFScore     float64 // combined RRF score
}
```

**RRF algorithm:**
```go
func MergeRRF(keywordResults []FunctionRef, semanticResults []SearchResult, k int) []RankedResult
```
1. Assign ranks: keyword result at position i gets rank i+1 (1-indexed)
2. Assign ranks: semantic result at position j gets rank j+1
3. For each unique function (keyed by `repoID:filePath:name`):
   - If present in keyword: `score += 1.0 / (k + keywordRank)`
   - If present in semantic: `score += 1.0 / (k + semanticRank)`
4. Sort by RRF score descending
5. Return merged list

Default `k=60` (standard RRF constant that smooths rank differences).

**Semantic result mapping:** Semantic `SearchResult` contains `Record.RepoID`, `Record.FilePath`, `Record.Name` — these map to the same merge key format. Only function-type semantic results are included in the merge (type results are filtered out).

### Section 4: search_org MCP Tool

**Goal:** New MCP tool that searches across all repos in an org.

**Tool definition:**
```
Name: "search_org"
Description: "Search across all repositories in an organization"
Parameters:
  - org_id (string, required)
  - query (string, required)
  - search_type (string, optional, default "hybrid"): "keyword", "semantic", "hybrid"
  - repo_ids (array of string, optional): filter to specific repos within org
  - max_results (integer, optional, default 20)
  - token_budget (integer, optional, default 4000): max tokens in response
```

**Tool registration:** Register in the MCP server's tool list alongside existing tools (e.g., `search_context`, `semantic_search`). The handler function follows the same pattern as existing tool handlers in `mcp/tools.go`.

**Handler flow:**
1. Validate org exists via org store's `GetOrg(orgID)`. Return error if not found.
2. Get repo list from org store's `GetOrgRepos(orgID)`.
3. If `repo_ids` provided, validate they are a subset of org repos, then filter. For semantic search, post-filter results by the provided repo_ids since `SearchByOrg` only accepts orgID.
4. Based on search_type:
   - "keyword": SearchFunctionsOrg with FTS5
   - "semantic": SemanticSearch.SearchByOrg, post-filtered by repo_ids if provided
   - "hybrid": both keyword and semantic, then MergeRRF
5. Apply token budget using Budgeter
6. Format results using progressive disclosure (SearchResultCompact from progressive.go)
7. Return formatted results with detail_refs

**Output format:**
```
## Search Results: "{query}" across {orgID}

Found {total} results across {repoCount} repos (showing top {shown})

### Results

1. **GetUser** (user-service) - `pkg/handlers/user.go:45`
   Retrieves user by ID from database
   → detail: `func|github.com/org/user-service|pkg/handlers/user.go|GetUser`

2. **ValidateUser** (auth-service) - `pkg/auth/validator.go:12`
   Validates user credentials against stored hash
   → detail: `func|github.com/org/auth-service|pkg/auth/validator.go|ValidateUser`

Token budget: {used}/{budget}
```

### Section 5: Progressive Disclosure Integration

**Goal:** Wire up the existing progressive.go infrastructure for search_org output.

**Changes:**
1. Update `FunctionSummary` to include `RepoID` and `Score` fields
2. Add `FormatOrgSearchResult` function that builds markdown from `[]RankedResult` with token budgeting
3. Each result includes a `detail_ref` string that can be passed to `get_function_context`
4. Add `ExpandDetailRef(ref string)` function that parses the ref format and dispatches to the appropriate tool

**detail_ref format change:** Use `|` (pipe) as separator instead of `:` (colon) to avoid ambiguity with colons in repo IDs (e.g., `github.com/org/repo`). Format: `func|{repoID}|{filePath}|{funcName}`. The `ExpandDetailRef` parser splits on `|` which is unambiguous.

**Token budgeting integration:**
- Estimate tokens per result (~100 tokens for summary line, accounting for longer paths)
- Fill results greedily until budget reached
- Truncate with "... and N more results. Use detail_ref to expand."

### Section 6: Integration Tests

**Goal:** End-to-end tests for the org search pipeline.

**Test scenarios:**
1. search_org keyword returns results from multiple repos
2. search_org semantic returns results from multiple repos
3. search_org hybrid returns better results than either alone
4. RRF scoring promotes results appearing in both keyword and semantic
5. Token budget limits output size
6. detail_ref works with get_function_context
7. repo_ids filter narrows search (both keyword and semantic)
8. Error for unknown org
9. Empty results handled gracefully
10. FTS5 index consistency after StoreRepoContext re-indexing (store, search, re-store, search again — no stale results)
11. FTS5 query with special characters handled correctly (quotes, operators)
12. Degraded mode: semantic search unavailable, returns keyword results only with warning

## Error Handling

- Unknown org: return error
- No indexed repos: return error "Run analyze_org first"
- Empty query: return error
- FTS5 not available (old database): fall back to LIKE search
- Semantic search fails (no vectors): return keyword results only with warning
- FTS5 special characters in query: sanitize by wrapping in double quotes

## Performance Considerations

- FTS5 with BM25 ranking is much faster than LIKE for text search
- Subquery `IN (SELECT repo_id FROM org_repos WHERE org_id = ?)` avoids dynamic SQL and is optimized by SQLite
- Semantic SearchByOrg loads all org vectors into memory — pre-existing design, not addressed here
- RRF merge is O(n+m) where n,m are keyword and semantic result counts
- Token budgeting prevents oversized responses
