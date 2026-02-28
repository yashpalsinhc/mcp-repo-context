# Section 1: Fix Dimension Mismatch & Auto-Init

## Overview

Standardize vector dimensions between embedder (256) and store (384), auto-initialize vector store on startup, add dimension migration for existing stores, add dimension validation on write, fix O(n^2) bubble sort, and add IsAvailable method.

## Dependencies

- None (foundational section)

## Tests First

### File: `internal/vectors/store_dimension_test.go` (new)

```
Test: Vector store uses embedder dimension not hardcoded
- Create embedder with dim=256
- Create vector store with embedder.Dimension()
- Assert: store.dimension == 256

Test: Dimension migration detects wrong-dimension vectors
- Create store with dim=256
- Insert vector with 384 floats directly via SQL
- Run detectStoredDimension()
- Assert: returns 384
- Run migrateDimension() — clears vectors, logs warning

Test: Dimension migration skips when no vectors exist
- Create empty store with dim=256
- Run detectStoredDimension()
- Assert: returns 0 (no vectors)
- No cleanup needed

Test: StoreBatch rejects wrong-dimension vectors
- Create store with dim=256
- Attempt StoreBatch with 384-dim vectors
- Assert: error "vector dimension 384 does not match store dimension 256"

Test: Store rejects wrong-dimension single vector
- Create store with dim=256
- Attempt Store with 384-dim vector
- Assert: same dimension error

Test: Vector metadata table created
- Create store
- Assert: vector_metadata table exists
- Assert: dimension key has value "256"

Test: Sort correctness with sort.Slice
- Create 1000 SearchResult with random similarities
- Sort with new sortBySimilarity
- Assert: sorted descending by similarity
- Assert: no panics, all results present

Test: IsAvailable true when store is set
- Create SemanticSearch with valid store
- Assert: IsAvailable() == true

Test: IsAvailable false when store is nil
- Create SemanticSearch with nil store
- Assert: IsAvailable() == false

Test: Auto-init creates vector store on startup
- Call initVectorStore() with temp path
- Assert: SQLite file created
- Assert: store is non-nil
```

## Implementation Details

### 1. Fix main.go Dimension

In `cmd/mcp-server/main.go`, change:
```go
// Before: vectorStore, err := vectors.NewSQLiteVectorStore(vectorStorePath, 384)
// After:
embedder := vectors.NewDefaultEmbedder()
vectorStore, err := vectors.NewSQLiteVectorStore(vectorStorePath, embedder.Dimension())
```

Pass the embedder to NewServer via ServerConfig so it's reused (not created twice).

### 2. Dimension Migration

Add to `SQLiteVectorStore`:

```go
func (s *SQLiteVectorStore) detectStoredDimension(ctx context.Context) (int, error)
```
- Query one vector: `SELECT vector FROM vectors LIMIT 1`
- Unmarshal JSON, return `len(floats)`
- If no vectors: return 0

```go
func (s *SQLiteVectorStore) migrateDimension(ctx context.Context, expectedDim int) error
```
- Call detectStoredDimension
- If 0: no migration needed
- If matches expectedDim: no migration needed
- If mismatch: `DELETE FROM vectors`, log warning

Call migrateDimension in `NewSQLiteVectorStore` after schema creation.

### 3. Metadata Table

Add to schema:
```sql
CREATE TABLE IF NOT EXISTS vector_metadata (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL
);
```

After creating store, insert/update dimension:
```sql
INSERT OR REPLACE INTO vector_metadata(key, value) VALUES ('dimension', ?)
```

### 4. Dimension Validation

In `Store()` and `StoreBatch()`, before writing:
```go
if len(record.Vector) != s.dimension {
    return fmt.Errorf("vector dimension %d does not match store dimension %d", len(record.Vector), s.dimension)
}
```

### 5. Fix Bubble Sort

In `store.go`, replace `sortBySimilarity` (lines 522-530):
```go
// Before: bubble sort loop
// After:
sort.Slice(results, func(i, j int) bool {
    return results[i].Similarity > results[j].Similarity
})
```

In `similarity.go`, replace `TopKSimilar` sort:
```go
sort.Slice(results, func(i, j int) bool {
    return results[i].Similarity > results[j].Similarity
})
```

### 6. Auto-Init Vector Store

Extract vector store creation into `initVectorStore()` in main.go:
```go
func initVectorStore(storagePath string, dim int) *vectors.SQLiteVectorStore
```
- Always attempt creation (not conditional)
- If fails: log error with path and permission info, return nil
- If succeeds: run dimension migration, return store

### 7. IsAvailable Method

Add to SemanticSearch:
```go
func (ss *SemanticSearch) IsAvailable() bool {
    return ss != nil && ss.store != nil
}
```

### 8. ServerConfig Change

Add `Embedder` field to ServerConfig. NewServer uses the provided embedder instead of creating a new one. This ensures consistent dimension across store and embedder.

## Error Handling

- Dimension mismatch on existing store: drop all vectors, log warning
- Store creation failure: log error with path, return nil (graceful degradation)
- Dimension validation failure: return error on Store/StoreBatch

## File Summary

| File | Action |
|------|--------|
| `cmd/mcp-server/main.go` | Modify: use embedder.Dimension(), extract initVectorStore(), pass embedder in config |
| `internal/vectors/store.go` | Modify: add detectStoredDimension, migrateDimension, metadata table, dimension validation, fix sort |
| `internal/vectors/similarity.go` | Modify: fix TopKSimilar sort |
| `internal/vectors/search.go` | Modify: add IsAvailable() method |
| `internal/mcp/server.go` | Modify: add Embedder to ServerConfig, use provided embedder |
| `internal/vectors/store_dimension_test.go` | New: dimension and init tests |
