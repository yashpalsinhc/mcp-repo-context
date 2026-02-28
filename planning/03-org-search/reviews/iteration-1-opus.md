# Opus Plan Review: Org Search & Hybrid Ranker

## Section 1: FTS5 Virtual Tables

### 1.1 CASCADE DELETE won't fire FTS5 triggers (CRITICAL)
SQLite triggers do not fire for rows deleted via ON DELETE CASCADE. StoreRepoContext does `DELETE FROM repos WHERE id = ?` which cascades through files to functions, so FTS5 delete entries will never be written. Stale entries will remain.

**Recommendation:** Either (a) switch to contentless FTS5 (`content=''`), (b) add explicit FTS cleanup in StoreRepoContext before cascade, or (c) use REBUILD after bulk operations.

### 1.2 Missing UPDATE trigger
Only INSERT and DELETE triggers defined. If functions are updated in-place, FTS index becomes stale.

### 1.3 NULL behavior_json
json_extract on NULL behavior_json returns NULL, treated as empty string by FTS5. Should be documented.

### 1.4 FTS5 availability not checked
No mechanism to detect if FTS5 is compiled in. Add startup check.

### 1.5 FTS5 query sanitization missing
User queries with special characters (*, ", AND, OR, NOT, NEAR) will be interpreted as FTS5 operators. Must sanitize or wrap in quotes.

## Section 2: Org-Scoped Search Methods

### 2.1 SQL IN clause dynamic parameters
Go's database/sql doesn't support slice parameters. Need to generate placeholders dynamically or use subquery.

### 2.2 Interface design ambiguity
Plan says "extend or create new interface" — needs a decision. Separate OrgSearcher interface recommended.

### 2.3 No limit on repo count
Use subquery `fi.repo_id IN (SELECT repo_id FROM org_repos WHERE org_id = ?)` instead of dynamic IN clause.

## Section 3: RRF Hybrid Ranker

### 3.1 Dedup key mismatch
FunctionRef has no RepoID field. Keyword results lack repo ID for merge key construction. Need to extend FunctionRef or use a different merge key strategy.

### 3.2 Type search excluded
Semantic search returns both functions and types. RRF merger only handles functions. Types from semantic search will be silently dropped.

### 3.3 k=60 may be too large for code search
With short result lists (20-50), k=60 makes rank differences nearly meaningless. Consider k=10 or k=20.

## Section 4: search_org MCP Tool

### 4.1 No pagination support
No offset parameter. Existing SearchResultCompact has NextOffset/HasMore fields suggesting pagination was intended.

### 4.2 org_id validation underspecified
Which store method to call not mentioned.

### 4.3 Semantic search memory pressure
SearchByOrg loads ALL vectors for org into memory. 10 repos x 1000 functions = 10K vectors per search.

### 4.4 repo_ids filter gap for semantic search
Keyword search uses repoIDs list, but semantic SearchByOrg uses orgID. No way to filter semantic results by specific repos.

## Section 5: Progressive Disclosure

### 5.1 detail_ref parsing ambiguity
Format `func:{repoID}:{filePath}:{funcName}` — repoID contains colons (github.com/org/repo). Splitting on `:` is ambiguous.

### 5.2 Token estimation rough
~50-80 tokens per result may undershoot. Longer paths/summaries could be 100-150 tokens.

## Section 6: Integration Tests

### 6.1 Missing: FTS5 consistency after re-indexing test
### 6.2 Missing: FTS5 special character query test
### 6.3 Missing: Degraded mode (no vectors) test

## Cross-Cutting

- Database separation: FTS5 in main DB, vectors in separate DB. Handler needs both stores.
- Missing: tool registration in MCP server
- Missing: mixed-indexing scenarios (repos indexed without org_id)
- Missing: FTS5 REBUILD strategy
