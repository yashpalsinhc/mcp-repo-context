# Spec: Semantic Search & Vector Store

## Purpose

Enable intelligent content retrieval via vector embeddings. Currently, semantic search is broken ("not enabled" error). Fix the UX, auto-initialize the vector store, and evaluate better embedding models for code.

## Background

See `planning/mcp-server-gaps-requirements.md` section 7. The vector store (`internal/vectors/store.go`) uses SQLite with brute-force cosine similarity. It requires manual initialization that isn't documented. The `index_repository` tool fails with a confusing error.

## Scope

### 1. Auto-Initialize Vector Store
**Current state:** Server requires vector store to be explicitly configured. `index_repository` fails with "Semantic search is not enabled. Initialize the server with a vector store."

**Required:**
- Auto-create SQLite vector store on first use (lazy initialization)
- Or initialize on server startup if configured
- Clear error message with instructions if initialization fails (e.g., missing API key for embeddings)
- Configuration option to disable vector indexing (for minimal setups)

### 2. Fix Error Messages
- Replace "Semantic search is not enabled" with actionable message
- Explain prerequisites: embedding API key, what `index_repository` does
- Suggest auto-indexing option

### 3. Evaluate Embedding Models
**Current state:** Uses Anthropic API for embeddings (research exact model during /deep-plan).

**Required research:**
- Voyage Code-3: 16% better than OpenAI on code retrieval, supports Matryoshka dimensions
- Anthropic embeddings: current default, assess quality
- Consider supporting multiple providers with fallback
- Decision: keep Anthropic default + add Voyage Code-3 option, or switch default?

### 4. Progressive Context Loading
**Current state:** `get_context_budgeted` works (87% efficiency in testing) but doesn't use vector-ranked results.

**Required:**
- Use vector similarity scores to rank functions in `get_context_budgeted`
- Support tiered budgets: 2K (quick), 4K (standard), 8K (thorough)
- Include function signatures + behavior summaries at lower budgets
- Add full function details (callers, callees, side effects) at higher budgets

### 5. Auto-Index on Analyze (Optional)
- Add configuration option: auto-index when `analyze_repo` runs
- Default: off (to avoid API costs)
- When enabled: index all functions and types after analysis completes
- Incremental: only re-index changed functions on `refresh_file`

### 6. Brute-Force Similarity Assessment
**Current state:** `internal/vectors/store.go:278-349` loads ALL vectors and calculates cosine similarity.

**Required assessment:**
- Profile performance at 200-repo scale (~50k functions)
- If too slow: evaluate ANN options (HNSW via SQLite extension, or external)
- Decision during /deep-plan based on profiling

## Dependencies

- **None.** Can run in parallel with 01-core-bug-fixes (Wave 1).

## Provides to Other Splits

- **06-agent-recipes:** Vector-ranked context for hybrid pre-computed + RAG model

## Research from Interview

- **Voyage Code-3** outperforms OpenAI v3-large by 16% on code retrieval tasks
- **Hybrid context model:** Pre-computed structural context + real-time RAG content retrieval
- **Progressive loading research (2025):** 2K-8K tokens covers 95% of agent queries with comparable quality to monolithic loading
- **User's context model choice:** Hybrid approach

## Testing Strategy

- Test auto-initialization from clean state
- Benchmark embedding quality: Anthropic vs Voyage Code-3 on gorilla/* repos
- Profile brute-force similarity at scale (1k, 10k, 50k vectors)
- Test progressive loading: verify quality at 2K, 4K, 8K budgets
