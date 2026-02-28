# Research: Org Search & Hybrid Ranker

## Key Findings

### Existing Search (sqlite_search.go)
- All searches use LIKE queries (no FTS5) — schema says "Portable schema without FTS5"
- SearchFunctions: LIKE on name_lower, signature, description. Orders by exact name match first. Limit 50.
- SearchByConcept: exact match on concepts table via hardcoded concept mapping
- HybridSearch: combines SearchFunctions + SearchByConcept with dedup. Limit 50.
- **All methods require repoID** — no org-scoped search methods exist

### Semantic Search (vectors/search.go)
- SearchByOrg already exists — queries vectors WHERE org_id = ?
- Uses brute-force cosine similarity (loads all org vectors into memory)
- Requires repos indexed with IndexRepositoryWithOrg (not regular IndexRepository)
- Returns SearchResult{Record, Similarity} — not typed to functions

### Token Budgeting (tokens/)
- TokenCounter: chars/4 approximation
- Budgeter: greedy fill by score, with SummarizeFunction fallback
- ScoredItem[T] generic with Item, Score, TokenCost
- Used in get_context_budgeted tool already

### Progressive Disclosure (mcp/progressive.go)
- Types defined: FunctionSummary, TypeSummary, SearchResultCompact
- detail_ref format: "func:{repoID}:{filePath}:{funcName}"
- **Infrastructure exists but is UNUSED** in current search tools
- Current toolSearchContext scans in-memory, doesn't use progressive.go

### Gaps for Org Search
1. No SearchFunctionsOrg or HybridSearchOrg SQL methods
2. SQL would need: `fi.repo_id IN (SELECT repo_id FROM org_repos WHERE org_id = ?)`
3. progressive.go ready to adopt for new tool
4. Token budgeter ready to reuse
5. No FTS5 — LIKE queries may be slow for large orgs
