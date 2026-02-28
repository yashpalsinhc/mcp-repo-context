<!-- SECTION_MANIFEST
section-01-fts5-tables
section-02-org-search-methods
section-03-rrf-ranker
section-04-search-org-tool
section-05-progressive-disclosure
section-06-integration-tests
END_MANIFEST -->

# Section Index: Org Search & Hybrid Ranker

## Batch 1 (parallel)

### section-01-fts5-tables.md
FTS5 virtual tables and triggers. Adds `functions_fts` FTS5 table to schema, INSERT/UPDATE/DELETE triggers, cascade delete cleanup in StoreRepoContext, FTS5 query sanitization, and SearchFunctionsFTS method. From plan Section 1 and TDD Section 1.

### section-02-org-search-methods.md
Org-scoped search methods. Creates OrgSearcher interface, extends FunctionRef with RepoID, implements SearchFunctionsOrg, SearchByConceptOrg, HybridSearchOrg using orgID subquery. From plan Section 2 and TDD Section 2.

### section-03-rrf-ranker.md
RRF hybrid ranker. New `internal/search/ranker.go` with RankedResult type and MergeRRF function. Handles merge key construction, semantic result filtering, score calculation with k=60. From plan Section 3 and TDD Section 3.

## Batch 2 (parallel, depends on batch 1)

### section-04-search-org-tool.md
search_org MCP tool. Tool definition, registration, handler flow including org validation, search_type dispatch, repo_ids filtering (including post-filter for semantic), token budgeting, and output formatting. From plan Section 4 and TDD Section 4.

### section-05-progressive-disclosure.md
Progressive disclosure integration. Updates FunctionSummary with RepoID/Score, FormatOrgSearchResult function, pipe-separated detail_ref format, ExpandDetailRef parser, token budget truncation. From plan Section 5 and TDD Section 5.

## Batch 3 (depends on batch 2)

### section-06-integration-tests.md
End-to-end integration tests. Full pipeline test, FTS5 consistency after re-indexing, special character handling, keyword-only/semantic-only/hybrid modes, repo_ids filter, token budget truncation, detail_ref round-trip, error cases. From plan Section 6 and TDD Section 6.
