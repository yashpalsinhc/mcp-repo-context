# Interview: Semantic Search & Vector Store

## Q1: Embedder strategy?
**Answer:** Fix bugs only
**Decision:** Keep LocalEmbedder as sole embedder. Fix dimension mismatch (256 vs 384), auto-init, error messages. External API embedders (Voyage, OpenAI) deferred to future work.

## Q2: Brute-force similarity performance?
**Answer:** Keep brute-force
**Decision:** Keep brute-force cosine similarity. Add monitoring/profiling and document limitations. No ANN (HNSW) for now — premature optimization for most use cases.

## Q3: get_context_budgeted ranking?
**Answer:** Add vector ranking
**Decision:** Use semantic similarity scores to rank functions in get_context_budgeted when vectors are available. Fall back to keyword scoring when not indexed. This improves relevance quality for indexed repos.
