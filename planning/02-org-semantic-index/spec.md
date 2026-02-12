# Split 02: Org-Wide Semantic Index

## Purpose

Extend the vector/embedding layer to support org-wide indexing. Store embeddings by org; enable incremental updates.

## Context

- **Requirements:** `/mcp-repo-context/requirements.md`
- **Design:** `/mcp-repo-context/docs/DESIGN_ORG_SEMANTIC_SEARCH.md`
- **Current:** `index_repository` is per-repo; vectors stored per repo

## Scope

### In Scope

1. **Org partitioning** — Vector store keyed by org_id
2. **index_org** — Index all repos in org; batch embed
3. **Incremental** — Re-embed only changed files (use existing file hashes)
4. **Chunking** — Reuse existing analyzer output (functions, types)
5. **Storage** — Extend vectors/embedder for org scope

### Out of Scope

- Org search UI/tool (Split 03)
- Agent workflows (Split 04)

## Technical Details

### Vector Store

- Add `org_id` column to vector records (or partition by org)
- `StoreBatch` with org_id filter
- `Search` accepts org_id

### Index Flow

```
analyze_org (or index_org)
  → For each repo in org:
      → Get existing context (or analyze)
      → Extract chunks (functions, types)
      → Embed chunks
      → Store with org_id
```

### Incremental

- Use existing `file_tracker` / `file_hashes` in storage
- On `refresh_file` or `refresh_changed`: re-embed only changed chunks
- Delete stale embeddings for removed/changed files

## Dependencies

- Split 01 (org abstraction)
- `internal/vectors`
- `internal/storage`
- `internal/analyzer`

## Verification

- [ ] `index_org` indexes all repos in org
- [ ] Vector search returns results scoped to org
- [ ] Incremental: changing one file updates only that file's embeddings
- [ ] No re-embed per session; index persists
