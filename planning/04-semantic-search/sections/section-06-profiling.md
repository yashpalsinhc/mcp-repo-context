# Section 6: Profiling & Monitoring

## Overview

Add timing instrumentation to vector operations, VectorCount method, and performance documentation. Helps users and developers understand indexing state and search performance.

## Dependencies

- Section 1: Vector store and SemanticSearch
- Section 4: Incremental indexing (RefreshFile timing)

## Tests First

### File: `internal/vectors/profiling_test.go` (new)

```
Test: IndexRepository logs total duration
- Call IndexRepository with a repo
- Assert: log output contains "IndexRepository" with duration in ms

Test: SearchFunctions logs query and duration
- Index a repo, then search
- Assert: log output contains "SearchFunctions" with duration

Test: RefreshFile logs duration
- Index repo, then RefreshFile
- Assert: log output contains "RefreshFile" with duration

Test: VectorCount returns correct count for indexed repo
- Store 5 function vectors and 3 type vectors for repo A
- Assert: VectorCount(repo-a) == 8

Test: VectorCount returns 0 for unindexed repo
- Assert: VectorCount("unknown-repo") == 0

Test: VectorCount returns 0 for empty store
- Assert: VectorCount("any") == 0

Test: Timing doesn't affect results
- IndexRepository with timing enabled
- Assert: same number of vectors as without timing
- Search with timing enabled
- Assert: same results as without timing
```

## Implementation Details

### 1. Timing Wrapper

Add a timing helper in `internal/vectors/timing.go`:

```go
func logTiming(operation string, start time.Time, details ...string) {
    duration := time.Since(start)
    msg := fmt.Sprintf("[vectors] %s completed in %s", operation, duration.Round(time.Millisecond))
    if len(details) > 0 {
        msg += " (" + strings.Join(details, ", ") + ")"
    }
    log.Printf(msg)
}
```

### 2. Instrument IndexRepository

In `search.go` IndexRepository method:

```go
func (ss *SemanticSearch) IndexRepository(ctx context.Context, repoCtx *storage.RepoContext) error {
    start := time.Now()
    defer func() {
        logTiming("IndexRepository", start,
            fmt.Sprintf("repo=%s", repoCtx.RepoID),
            fmt.Sprintf("functions=%d", len(repoCtx.Functions)),
            fmt.Sprintf("types=%d", len(repoCtx.Types)),
        )
    }()
    // ... existing logic
}
```

### 3. Instrument Search Methods

In SearchFunctions, SearchAll, SearchByOrg:

```go
func (ss *SemanticSearch) SearchFunctions(ctx context.Context, query, repoID string, limit int) ([]SearchResult, error) {
    start := time.Now()
    defer func() {
        logTiming("SearchFunctions", start,
            fmt.Sprintf("repo=%s", repoID),
            fmt.Sprintf("query=%q", truncateQuery(query, 50)),
        )
    }()
    // ... existing logic
}
```

### 4. Instrument RefreshFile

```go
func (ss *SemanticSearch) RefreshFile(ctx context.Context, repoID, filePath string, ...) error {
    start := time.Now()
    defer func() {
        logTiming("RefreshFile", start,
            fmt.Sprintf("repo=%s", repoID),
            fmt.Sprintf("file=%s", filePath),
        )
    }()
    // ... existing logic
}
```

### 5. VectorCount Method

Add to SQLiteVectorStore if not present:

```go
func (s *SQLiteVectorStore) VectorCount(ctx context.Context, repoID string) (int, error) {
    var count int
    err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM vectors WHERE repo_id = ?", repoID).Scan(&count)
    return count, err
}
```

Add to SemanticSearch as well (delegates to store):

```go
func (ss *SemanticSearch) Count(ctx context.Context, repoID string) (int, error) {
    if !ss.IsAvailable() {
        return 0, nil
    }
    return ss.store.VectorCount(ctx, repoID)
}
```

### 6. Performance Documentation

Add comments at the top of `search.go`:

```go
// Performance characteristics (brute-force cosine similarity with O(n log n) sort):
//   <1K vectors:   <10ms   - suitable for all use cases
//   1K-10K vectors: 10-100ms - suitable for interactive use
//   10K-50K vectors: 100ms-1s - acceptable for batch/async
//   >50K vectors:   >1s    - consider specialized vector DB
//
// Vectors are stored as JSON-encoded float64 arrays in SQLite.
// All similarity computations are done in-memory.
```

Update tool descriptions for `semantic_search` and `index_repository` to include performance notes.

### 7. truncateQuery Helper

```go
func truncateQuery(query string, maxLen int) string {
    if len(query) <= maxLen {
        return query
    }
    return query[:maxLen] + "..."
}
```

## Error Handling

- Timing is fire-and-forget (defer pattern) — never affects operation results
- VectorCount failure: return 0, log error

## File Summary

| File | Action |
|------|--------|
| `internal/vectors/timing.go` | New: logTiming helper, truncateQuery |
| `internal/vectors/search.go` | Modify: add timing to IndexRepository, Search*, RefreshFile |
| `internal/vectors/store.go` | Modify: add VectorCount if not present |
| `internal/vectors/profiling_test.go` | New: profiling tests |
