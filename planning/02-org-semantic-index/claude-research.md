# Research: Org-Wide Semantic Index

## Codebase Research

### 1. Vector Store Implementation (`internal/vectors/`)

**VectorRecord** - Core data structure:
- Fields: ID, RepoID, OrgID, Type, Name, FilePath, Vector ([]float64), Metadata (map[string]string)
- ID format: `"{repoID}:{type}:{filePath}:{name}"`
- Vectors stored in SQLite as JSON-serialized BLOB columns
- Metadata as JSON strings

**Database Schema (vectors table):**
```sql
CREATE TABLE vectors (
    id TEXT PRIMARY KEY,
    repo_id TEXT NOT NULL,
    org_id TEXT NOT NULL DEFAULT '',
    type TEXT NOT NULL,
    name TEXT NOT NULL,
    file_path TEXT,
    vector BLOB NOT NULL,
    metadata TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_vectors_repo ON vectors(repo_id);
CREATE INDEX idx_vectors_org ON vectors(org_id);
CREATE INDEX idx_vectors_type ON vectors(type);
CREATE INDEX idx_vectors_name ON vectors(name);
```

**Embedder**: LocalEmbedder - offline, vocabulary-based (TF-IDF style)
- Dimension: configurable (default 256)
- Vocabulary: up to 10,000 most frequent words
- Code preprocessing: camelCase/snake_case expansion, special separator splitting
- No external API calls

**Similarity**: CosineSimilarity (primary), EuclideanDistance, DotProduct available

**VectorStore Interface Methods:**
- `Store`, `StoreBatch`, `Get`, `Delete`, `DeleteByRepo`
- `Search(ctx, query, repoID, limit)` - per-repo scoped
- `SearchByOrg(ctx, query, orgID, limit)` - already implemented for cross-org search
- `SearchByType`, `Count`, `CountByOrg`, `DeleteByOrg`

**SemanticSearch Service Layer:**
- `IndexRepository(ctx, repo)` - per-repo indexing
- `IndexRepositoryWithOrg(ctx, repo, orgID)` - already supports org tagging
- `SearchFunctions(ctx, query, repoID, limit)` - repo-scoped search
- `SearchByOrg(ctx, query, orgID, limit)` - org-scoped search

**Document Construction for Embedding:**
- Functions: Name + Signature + Description + Behavior Summary + Steps + SideEffects + FilePath
- Types: Name + Kind + Description + FieldNames + FieldTypes + FilePath

### 2. Organization Abstraction (`internal/org/`)

**Org struct**: ID, Repos []string, Config OrgConfig, Created time.Time
**OrgConfig**: ExcludePatterns []string, MaxFileSize int64

**Store Interface** (SQLite-backed):
- SaveOrg, GetOrg, ListOrgs, DeleteOrg
- AddRepos, RemoveRepos (atomic, INSERT OR IGNORE)
- GetRepoConfigOverride, SetRepoConfigOverride
- RunMigrations

**Schema** (migration 003):
- `orgs` table: id, config_json, created_at, updated_at
- `org_repos` table: org_id, repo_id, config_override_json, added_at (PRIMARY KEY (org_id, repo_id))
- Cascade delete, index on repo_id for reverse lookup

**Manager Interface**:
- Register, List, Get, AddRepos, RemoveRepos, Delete
- GetEffectiveConfig, SetRepoConfigOverride
- AnalyzeOrg(ctx, orgID, force, concurrency) - concurrent analysis with bounded goroutines

### 3. Storage Layer (`internal/storage/`)

**SQLite Config**: WAL mode, foreign keys enforced, 10 max open / 5 idle connections, NORMAL sync

**File Tracking** (migration 002):
```sql
CREATE TABLE file_hashes (
    repo_id TEXT NOT NULL,
    file_path TEXT NOT NULL,
    hash TEXT NOT NULL,        -- SHA256
    last_analyzed TIMESTAMP,
    file_size INTEGER,
    PRIMARY KEY (repo_id, file_path)
);
```

**FileTracker Interface**: GetChangedFiles, GetFileHashes, UpdateFileHash, UpdateFileHashes, CleanupDeletedFiles, GetStaleFiles, DeleteRepoHashes, GetFileHash

**Incremental Workflow**: Compute SHA256 → GetChangedFiles → Analyze changed → UpdateFileHashes → CleanupDeletedFiles

### 4. Analyzer Output

**Available for Embedding**:
- FunctionDef: Name, Signature, Description, Behavior (Summary, Steps, Patterns), SideEffects, ErrorHandling, Calls, CalledBy
- TypeDef: Name, Kind, Description, Fields (with JSON tags), Methods, Implements

**Refresh Flows**:
- `refresh_file`: hash check → re-analyze → update file_hashes → update vectors
- `refresh_changed`: scan all → diff hashes → re-analyze changed → batch update

### 5. Testing Patterns

- Go standard `testing` package
- SQLite temp files for test databases with cleanup
- Mock orchestrator pattern for concurrent testing (tracks max concurrent, call counts, failures)
- Context-based tests with timeouts

---

## Web Research

### Vector Store Partitioning

**Three strategies for multi-tenant vector storage:**

1. **Per-Tenant Indexing**: Separate indexes per org. Best for massive datasets. High memory overhead.
2. **Metadata Filtering**: Single shared index, filter by org_id during queries. Memory-efficient, suitable for typical codebase sizes.
3. **Curator Pattern**: Hybrid approach achieving per-tenant performance with shared index efficiency. 32.9x faster than metadata-filtered IVF.

**Recommendation**: Shared SQLite with metadata filtering (org_id column). Already partially implemented. Adequate for typical codebase sizes (thousands of functions per org).

### Incremental Embedding Updates

**Hash-Based Change Detection**: SHA-256 of function source code (not file hash). Store hash alongside embedding for O(1) comparison.

**Stale Embedding Cleanup**: Two strategies:
1. Soft deletion with periodic cleanup (safe rollback window)
2. Versioning with expiration triggers

**Batch vs Incremental**: Batch updates recommended for codebase analysis (process all changes together, better rate limit management, atomic transactions).

**Cost Optimization**: Content-hash deduplication (60-80% reduction), significance filtering (skip small/generated functions), batch API calls.

### Batch Embedding Performance in Go

**Concurrency Patterns**:
1. Worker pool with fixed concurrency (pond library)
2. Semaphore-based rate limiting
3. Context-based cancellation with per-call timeouts

**Rate Limiting**: Exponential backoff with jitter (cenkalti/backoff/v4). Respect Retry-After headers. Token bucket for smooth limiting.

**Memory Efficiency**: Stream processing with channels, cursor-based pagination, slice capacity reuse, batch transactions.

**Key insight**: The current LocalEmbedder has no API calls (offline TF-IDF), so rate limiting/retry is not needed now. But the architecture should support it for future external embedder integration.

### Summary Recommendations

| Topic | Recommendation |
|-------|---------------|
| Vector Storage | Shared SQLite with org_id metadata filter (already partially done) |
| Change Detection | SHA-256 content hashing per function/type |
| Update Strategy | Batch updates after analysis; soft deletions with cleanup |
| Concurrency | Worker pool with bounded goroutines (already in org analyzer) |
| Memory | Stream/batch processing; reuse slice capacity |
