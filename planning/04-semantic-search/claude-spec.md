# Spec: Semantic Search & Vector Store

## Problem
Semantic search is broken for new users. The vector store requires explicit initialization that's undocumented, `index_repository` fails with a confusing "not enabled" error, there's a dimension mismatch between the vector store (384) and the embedder (256), and `get_context_budgeted` doesn't use vector search for ranking.

## Requirements

1. **Auto-initialize vector store** on first use (lazy init). Server should always have semantic search available without user configuration.
2. **Fix dimension mismatch** between main.go (384) and LocalEmbedder default (256). Standardize to a single consistent dimension.
3. **Fix error messages** — replace "Semantic search is not enabled" with actionable guidance. Explain what's needed and suggest next steps.
4. **Auto-index on analyze** (optional) — add configuration option to auto-index when `analyze_repo`/`analyze_local` runs. Default: off.
5. **Incremental indexing** — only re-index changed functions on `refresh_file`/`refresh_changed` instead of full reindex.
6. **Vector-ranked get_context_budgeted** — use semantic similarity scores to rank functions when vectors are available, fall back to keyword scoring when not indexed.
7. **Keep brute-force search** — add profiling/monitoring, document limitations. No ANN for now.
8. **Keep LocalEmbedder** as sole embedder. External API embedders deferred to future work.

## Design Decisions
- LocalEmbedder only (no Voyage, OpenAI, or Anthropic API embedders)
- Brute-force cosine similarity kept (with profiling)
- Vector ranking added to get_context_budgeted with keyword fallback
- Auto-init on startup or lazy (first use)
- Auto-index is opt-in via config

## Dependencies
- None (standalone)

## Provides
- 06-agent-recipes: Vector-ranked context for hybrid pre-computed + RAG model
