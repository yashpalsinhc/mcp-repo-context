# Integration Notes: Semantic Search & Vector Store

## Integrating

### 1. Vocabulary staleness policy → INTEGRATE
Add a `vocabulary_version` counter to stored vocabulary. After N incremental updates (default 50), log a warning suggesting full re-index. `index_repository force=true` always rebuilds vocabulary.

### 2. Dimension detection via sample vector → INTEGRATE
On startup, query one vector record, check `len(vector)`. Compare to embedder dimension. If mismatch: drop all vectors and log warning. Add `metadata` row for dimension.

### 3. Dimension validation in Store/StoreBatch → INTEGRATE
Add `len(record.Vector) == s.dimension` check. Return error on mismatch.

### 5. RefreshFile handles both functions and types → INTEGRATE
Accept full file context (functions + types). Re-index both.

### 6. Add DeleteByFile method + file_path index → INTEGRATE
Add `DELETE FROM vectors WHERE repo_id = ? AND file_path = ?` method. Add index on (repo_id, file_path).

### 7. Auto-index default to true → INTEGRATE
Since LocalEmbedder is free and fast, default MCP_AUTO_INDEX=true. Aligns with "just works" goal.

### 8. Replace bubble sort with sort.Slice → INTEGRATE
One-line fix. Add to Section 1.

### 9. Load vocabulary on search (server restart fix) → INTEGRATE
SearchFunctions/SearchAll load persisted vocabulary before embedding query if embedder vocabulary is empty for that repo.

### 10. Add RefreshFiles (batch) method → INTEGRATE
Single vocabulary load, process all changed files in one call.

### 11. Cross-repo vocabulary loading → INTEGRATE
Search methods load the target repo's vocabulary before embedding the query.

### 12. Cosine similarity range → INTEGRATE
Correct documentation: range is [-1, 1], negative scores filtered.

## NOT Integrating

### 4. Vocabulary eager vs lazy loading detail
Already covered by integrating #9 and #11 — load vocabulary when needed for search. Cache in SemanticSearch struct.

### 13. ctx.Done() checks in indexing loops
Good practice but not specific to this plan. Can be done as a general improvement later.
