# Split 03: Org Search & Hybrid Ranker

## Purpose

Implement `search_org` tool that combines keyword and semantic search across an org. Returns ranked results with progressive disclosure.

## Context

- **Requirements:** `/mcp-repo-context/requirements.md`
- **Design:** `/mcp-repo-context/docs/DESIGN_ORG_SEMANTIC_SEARCH.md`
- **Current:** `search_context` and `semantic_search` are per-repo

## Scope

### In Scope

1. **search_org tool** — Query across org
2. **Search modes** — keyword, semantic, hybrid
3. **Hybrid ranking** — Combine FTS5 + vector similarity (e.g. RRF)
4. **Token budgeting** — Limit output size
5. **Progressive disclosure** — Return refs; `get_function_context` for details

### Out of Scope

- Agent workflows (Split 04)
- Plugin system (Split 05)

## Technical Details

### Tool

```json
{
  "name": "search_org",
  "parameters": {
    "org_id": "required",
    "query": "required",
    "search_type": "keyword|semantic|hybrid",
    "repo_ids": "optional filter",
    "max_results": 20
  }
}
```

### Flow

1. Get org repos from Split 01
2. Keyword: SQLite FTS5 across org repos
3. Semantic: Vector similarity across org
4. Hybrid: Merge with RRF (reciprocal rank fusion)
5. Apply token budget
6. Return refs with `detail_ref` for expansion

### Output Format

```json
{
  "results": [
    {
      "ref": "func:org:repo:file:name",
      "score": 0.9,
      "summary": "1-line description"
    }
  ]
}
```

## Dependencies

- Split 01 (org abstraction)
- Split 02 (org index)
- `internal/storage/sqlite_search`
- `internal/vectors`
- `internal/tokens/budgeter`

## Verification

- [ ] `search_org` returns results from multiple repos
- [ ] Hybrid mode improves relevance over keyword-only
- [ ] Token budget enforced
- [ ] `detail_ref` supports `get_function_context` expansion
