# Integration Notes: Org Search & Hybrid Ranker

## Integrating

### 1.1 CASCADE DELETE vs FTS5 triggers → INTEGRATE
Use explicit FTS cleanup before cascade delete in StoreRepoContext. Add `DELETE FROM functions_fts WHERE rowid IN (SELECT id FROM functions WHERE file_id IN (SELECT id FROM files WHERE repo_id = ?))` before the repo delete. This is the simplest fix.

### 1.2 Missing UPDATE trigger → INTEGRATE
Add an AFTER UPDATE trigger on functions table to keep FTS5 in sync.

### 1.5 FTS5 query sanitization → INTEGRATE
Wrap user queries in double quotes for phrase matching by default. Strip internal double quotes. This prevents operator injection while still being useful.

### 2.2 Interface design → INTEGRATE
Create separate `OrgSearcher` interface. Cleaner, doesn't break existing implementations.

### 2.3 Subquery instead of IN clause → INTEGRATE
Use `fi.repo_id IN (SELECT repo_id FROM org_repos WHERE org_id = ?)` instead of dynamic placeholder generation. Simpler and handles any repo count.

### 3.1 FunctionRef missing RepoID → INTEGRATE
Add RepoID to FunctionRef struct. All org-scoped search methods will populate it from the JOIN.

### 4.4 repo_ids filter for semantic search → INTEGRATE
Post-filter semantic results by repo_ids when the filter is provided. Simple and avoids needing a new vector store method.

### 5.1 detail_ref parsing → INTEGRATE
Use a different separator (e.g., `|`) or a structured format. Change to `func|repoID|filePath|funcName` to avoid colon ambiguity.

### 6.1-6.3 Additional tests → INTEGRATE
Add the three missing test scenarios: FTS5 consistency after re-indexing, special character queries, degraded mode without vectors.

### Cross-cutting: tool registration → INTEGRATE
Add note about where search_org gets registered in MCP tool list.

## NOT Integrating

### 1.3 NULL behavior_json documentation
Already handled implicitly — FTS5 treats NULL as empty string which is correct behavior.

### 1.4 FTS5 availability check
The project uses mattn/go-sqlite3 with CGO which always includes FTS5. Adding a startup check is unnecessary complexity.

### 3.2 Type search excluded
search_org is intentionally function-focused. Types can be searched via search_context. Adding type support to RRF adds complexity without clear user value.

### 3.3 k=60 too large
k=60 is the standard constant. Making it configurable adds API surface. The ranking still works correctly — items appearing in both lists get boosted. We can tune later if needed.

### 4.1 Pagination
Token budgeting with progressive disclosure serves the same purpose. Users can expand individual results via detail_ref. Adding offset/limit pagination adds API complexity for minimal gain.

### 4.3 Semantic search memory pressure
This is a pre-existing issue with SearchByOrg, not specific to this plan. Addressing it here would scope-creep.
