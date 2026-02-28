# Opus Plan Review: Semantic Search & Vector Store

## Critical Issues

### 1. Vocabulary drift makes incremental indexing semantically broken
TF-IDF vocabulary is corpus-dependent. Persisting it for incremental updates means new terms get hash-based fallback. Quality degrades over time.
**Rec:** Add staleness policy, full rebuild trigger after N incremental updates.

### 2. Dimension mismatch migration under-specified
No dimension column in SQLite. Can't detect stored vector dimensions. Need to inspect sample vector or add metadata table.

### 3. CosineSimilarity silently returns 0 on dimension mismatch
Add dimension validation in Store/StoreBatch.

## Significant Issues

### 4. Vocabulary persistence schema — eager vs lazy loading, thread safety
Specify loading strategy. Cache vocabulary in SemanticSearch struct.

### 5. RefreshFile only re-indexes functions, not types
IndexRepository indexes both functions AND types. RefreshFile plan only mentions functions.

### 6. DeleteByFile doesn't exist in store
Need to add method + file_path index.

### 7. Auto-index default should be true
LocalEmbedder is free. Defaulting to false contradicts "just works" goal.

## Minor Issues

### 8. Bubble sort O(n^2) in production
Replace with sort.Slice for O(n log n).

### 9. Vocabulary lost on server restart
After restart, embedder has empty vocabulary. Search queries use hash fallback only.

### 10. refresh_changed has no batch incremental indexing
50 file changes = 50 serial RefreshFile calls, each loading vocabulary.

### 11. Cross-repo vocabulary contamination
Search repo B uses repo A's vocabulary if A was indexed last.

### 12. Cosine similarity range claim wrong
Returns -1 to 1, not 0 to 1. Negative scores filtered.

### 13. No ctx.Done() checks in long indexing operations
