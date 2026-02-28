# Section 4: Incremental Indexing

## Overview

Support updating vectors for individual files without full reindex. Includes vocabulary persistence, per-repo vocabulary caching, staleness tracking, DeleteByFile method, RefreshFile/RefreshFiles methods, and vocabulary loading for search queries (fixing the server restart bug).

## Dependencies

- Section 1: Dimension fix, auto-init
- Section 3: Auto-index (indexing creates vocabulary that incremental updates use)

## Tests First

### File: `internal/vectors/incremental_test.go` (new)

```
Test: RefreshFile updates vectors for changed functions
- Index repo with file A containing [GetUser, CreateUser]
- Modify file A: remove CreateUser, add UpdateUser
- Call RefreshFile with updated file context
- Assert: GetUser vector exists (updated)
- Assert: UpdateUser vector exists (new)
- Assert: CreateUser vector gone (removed)

Test: RefreshFile updates both functions and types
- Index repo with file containing function GetUser and type User
- Update file: change User to UserProfile
- Call RefreshFile with full file context (functions + types)
- Assert: GetUser vector exists
- Assert: UserProfile vector exists
- Assert: User type vector gone

Test: RefreshFiles batch processes multiple files
- Index repo with files A, B, C
- Call RefreshFiles with updated A and B
- Assert: A vectors updated
- Assert: B vectors updated
- Assert: C vectors unchanged

Test: DeleteByFile removes all vectors for a file
- Store 3 function vectors + 1 type vector in file A
- Call DeleteByFile(repoID, "file_a.go")
- Assert: 0 vectors for file A
- Assert: other files' vectors unchanged

Test: DeleteByFile on nonexistent file is no-op
- Call DeleteByFile(repoID, "nonexistent.go")
- Assert: no error, no changes

Test: Vocabulary stored during IndexRepository
- Call IndexRepository for repo
- Query vocabularies table
- Assert: entry exists for repoID
- Assert: vocabulary_json is valid JSON with word→index mappings
- Assert: idf_json is valid JSON with word→float mappings
- Assert: version == 0

Test: Vocabulary loaded for incremental indexing
- IndexRepository (stores vocabulary)
- Clear embedder vocabulary (simulate fresh state)
- RefreshFile (should load vocabulary from DB)
- Assert: embeddings generated correctly (not using hash fallback)

Test: Vocabulary staleness warning at threshold
- IndexRepository (version=0)
- Call RefreshFile 50 times (version increments each time)
- Assert: warning logged on 50th call about stale vocabulary

Test: Vocabulary cached in SemanticSearch
- Call RefreshFile twice for same repo
- Assert: LoadVocabulary called once (cached after first load)

Test: Force reindex resets vocabulary version
- IndexRepository → version=0
- RefreshFile x 5 → version=5
- IndexRepository with force=true
- Assert: version=0, new vocabulary stored

Test: Missing vocabulary triggers rebuild
- Index repo (creates vocabulary)
- Delete vocabulary from DB manually
- Call RefreshFile
- Assert: vocabulary rebuilt from current repo context
- Assert: new vocabulary stored in DB

Test: Vocabulary loading for search after restart
- IndexRepository (stores vocabulary)
- Create new SemanticSearch with fresh embedder (simulating restart)
- Call SearchFunctions for the repo
- Assert: vocabulary loaded from DB before query embedding
- Assert: results match expected (not using hash fallback)

Test: Cross-repo vocabulary isolation
- IndexRepository for repo A (user functions)
- IndexRepository for repo B (order functions)
- Search repo B for "order"
- Assert: repo B's vocabulary used (not repo A's)

Test: file_path index exists for efficient DeleteByFile
- Create store, check indices
- Assert: idx_vectors_file index exists on (repo_id, file_path)
```

## Implementation Details

### 1. Vocabularies Table

Add to SQLiteVectorStore schema in `ensureSchema()`:

```sql
CREATE TABLE IF NOT EXISTS vocabularies (
    repo_id TEXT PRIMARY KEY,
    vocabulary_json TEXT NOT NULL,
    idf_json TEXT NOT NULL,
    version INTEGER DEFAULT 0,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
```

### 2. file_path Index

Add index for efficient DeleteByFile:

```sql
CREATE INDEX IF NOT EXISTS idx_vectors_file ON vectors(repo_id, file_path);
```

### 3. Vocabulary Serialization

Add methods to LocalEmbedder:

```go
func (e *LocalEmbedder) ExportVocabulary() (vocabJSON []byte, idfJSON []byte, err error)
```
- Marshals `e.vocabulary` and `e.idf` maps to JSON

```go
func (e *LocalEmbedder) ImportVocabulary(vocabJSON, idfJSON []byte) error
```
- Unmarshals JSON into `e.vocabulary` and `e.idf` maps
- Thread-safe: acquires write lock before modifying

### 4. StoreVocabulary / LoadVocabulary

On SQLiteVectorStore:

```go
func (s *SQLiteVectorStore) StoreVocabulary(ctx context.Context, repoID string, vocabJSON, idfJSON []byte, version int) error
```
- `INSERT OR REPLACE INTO vocabularies(repo_id, vocabulary_json, idf_json, version, updated_at) VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP)`

```go
func (s *SQLiteVectorStore) LoadVocabulary(ctx context.Context, repoID string) (vocabJSON, idfJSON []byte, version int, err error)
```
- `SELECT vocabulary_json, idf_json, version FROM vocabularies WHERE repo_id = ?`
- Returns sql.ErrNoRows if not found

```go
func (s *SQLiteVectorStore) IncrementVocabularyVersion(ctx context.Context, repoID string) (int, error)
```
- `UPDATE vocabularies SET version = version + 1, updated_at = CURRENT_TIMESTAMP WHERE repo_id = ? RETURNING version`

### 5. DeleteByFile

On SQLiteVectorStore:

```go
func (s *SQLiteVectorStore) DeleteByFile(ctx context.Context, repoID, filePath string) error
```
- `DELETE FROM vectors WHERE repo_id = ? AND file_path = ?`
- Uses the new idx_vectors_file index for efficient lookup

### 6. Vocabulary Cache in SemanticSearch

Add to SemanticSearch struct:

```go
type SemanticSearch struct {
    embedder     Embedder
    store        *SQLiteVectorStore
    vocabCache   map[string]*cachedVocab  // repo_id → cached vocabulary
    vocabCacheMu sync.RWMutex
}

type cachedVocab struct {
    vocabJSON []byte
    idfJSON   []byte
    version   int
}
```

```go
func (ss *SemanticSearch) loadVocabularyForRepo(ctx context.Context, repoID string) error
```
1. Check cache: if repoID is cached, import to embedder and return
2. Load from DB: `store.LoadVocabulary(ctx, repoID)`
3. If not found: return error (caller rebuilds)
4. Import to embedder: `embedder.ImportVocabulary(vocabJSON, idfJSON)`
5. Cache the vocabulary

### 7. RefreshFile

```go
func (ss *SemanticSearch) RefreshFile(ctx context.Context, repoID, filePath string, functions []storage.FunctionDetail, types []storage.TypeDef) error
```

Steps:
1. Load vocabulary for repo (from cache or DB). If missing, return error.
2. Delete all vectors for file: `store.DeleteByFile(ctx, repoID, filePath)`
3. For each function: generate embedding, create VectorRecord, collect
4. For each type: generate embedding, create VectorRecord, collect
5. Batch store all records
6. Increment vocabulary version: `store.IncrementVocabularyVersion(ctx, repoID)`
7. Check staleness: if version >= 50, log warning

### 8. RefreshFiles (Batch)

```go
func (ss *SemanticSearch) RefreshFiles(ctx context.Context, repoID string, files map[string]FileContext) error
```

Steps:
1. Load vocabulary once
2. For each file: DeleteByFile + collect new records
3. Batch store all records at once
4. Increment version once

### 9. Update IndexRepository

After building vocabulary, store it:
```go
vocabJSON, idfJSON, _ := embedder.ExportVocabulary()
store.StoreVocabulary(ctx, repoID, vocabJSON, idfJSON, 0) // version 0 = fresh
```

Clear vocabulary cache for the repo (force reload on next access).

### 10. Vocabulary Loading for Search

In `SearchFunctions`, `SearchAll`, `SearchByOrg` — before embedding the query:

```go
func (ss *SemanticSearch) ensureVocabulary(ctx context.Context, repoID string) error
```
- Check if embedder has vocabulary loaded (non-empty)
- If empty, call `loadVocabularyForRepo(ctx, repoID)`
- If vocabulary not found in DB, proceed with hash-based fallback (log warning)

This fixes the "vocabulary lost on server restart" bug.

## Error Handling

- Vocabulary not found in DB: rebuild from current repo context if possible, else fall back to hash-based embedding
- DeleteByFile on nonexistent file: no-op (DELETE WHERE returns 0 rows)
- Vocabulary staleness: log warning, do not error
- Import vocabulary failure (corrupt JSON): return error, caller falls back to full reindex

## File Summary

| File | Action |
|------|--------|
| `internal/vectors/store.go` | Modify: add vocabularies table, idx_vectors_file index, DeleteByFile, StoreVocabulary, LoadVocabulary, IncrementVocabularyVersion |
| `internal/vectors/embedder.go` | Modify: add ExportVocabulary, ImportVocabulary methods |
| `internal/vectors/search.go` | Modify: add RefreshFile, RefreshFiles, loadVocabularyForRepo, ensureVocabulary, vocabulary cache |
| `internal/vectors/incremental_test.go` | New: incremental indexing tests |
