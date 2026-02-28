# Research: Semantic Search & Vector Store

## Key Findings

### Vector Store (`internal/vectors/store.go`)
- SQLite `vectors` table: id, repo_id, org_id, type, name, file_path, vector (JSON BLOB), metadata (JSON)
- Indices on repo_id, org_id, type, name
- Vectors stored as JSON-encoded float64 arrays
- Auto-migration for org_id column on existing DBs
- RWMutex for thread-safe concurrent access
- No vector distance indices (no HNSW, no ANN)

### Brute-Force Similarity Search
- Loads ALL vectors for a repo/org from SQLite
- Cosine similarity computed for each vector against query
- Bubble sort by similarity descending
- O(n*d) complexity where n=vectors, d=dimension
- Suitable for <10k vectors, becomes slow beyond

### Embedding (`internal/vectors/embedder.go`)
- **Only LocalEmbedder implemented** — NO Anthropic embedder
- TF-IDF based with code-specific tokenization
- Default dimension: 256 (configurable)
- Max vocabulary: 10,000 words
- Code preprocessing: splits camelCase, snake_case, namespaces, paths
- Stopword filtering includes Go/JS keywords
- Vocabulary built per-repo during IndexRepository — embeddings depend on corpus
- EmbedBatch generates all embeddings with shared vocabulary

### Semantic Search Service (`internal/vectors/search.go`)
- SemanticSearch struct holds embedder + vector store
- IndexRepository: collects functions+types, builds vocab, generates embeddings, batch stores
- IndexRepositoryWithOrg: same but tags vectors with org_id
- SearchFunctions/SearchTypes/SearchAll: filter by vector type
- SearchByOrg: cross-repo search using org_id filter
- ID format: `{repo_id}:func:{file_path}:{function_name}`

### Server Initialization (`cmd/mcp-server/main.go`)
- Vector store created with path from env `MCP_VECTOR_STORE_PATH` (default: `{storage}/vectors.db`)
- **Hardcoded dimension: 384** in main.go — but LocalEmbedder default is **256** (dimension mismatch bug!)
- If vector store creation fails, semantic search disabled but server continues
- SemanticSearch created only if VectorStore is non-nil
- Uses `NewDefaultEmbedder()` which returns LocalEmbedder(256)

### Tool Handlers
- **toolSemanticSearch**: returns "Semantic search is not enabled" if semanticSearch is nil
- **toolIndexRepository**: returns "Semantic search is not enabled" if nil; otherwise gets RepoContext, optionally clears existing, indexes
- **toolGetContextBudgeted**: does NOT use vector search — uses keyword-based scoring with extractSearchKeywords + scoreFunctionRelevance. Works without semantic search.

### get_context_budgeted Details
- Keyword extraction from query
- Scores functions: +1.0 per keyword match in name/desc/sig/summary, +0.5 boost for name match
- Normalizes by keyword count
- Budgeter.BuildFunctionContext: greedy fill by score, with SummarizeFunction fallback
- TokenCounter: chars/4 approximation

### Auto-Indexing: None
- No mechanism to auto-index on analyze_repo/analyze_local
- Indexing is manual: user must call index_repository
- No incremental indexing — always regenerates everything
- ClearRepository deletes all vectors for a repo

### Critical Issues Found
1. **Dimension mismatch**: main.go creates vector store with dim=384, LocalEmbedder defaults to dim=256
2. **No auto-initialization**: server just disables semantic search if store fails
3. **Error messages unhelpful**: "Semantic search is not enabled" gives no guidance
4. **No incremental indexing**: full reindex every time
5. **Vocabulary is per-repo**: search across repos uses vocabulary built for one repo
