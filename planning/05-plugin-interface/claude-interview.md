# Interview: Plugin Interface

## Q1: Registry design — open or closed?
**Answer:** Closed constructor
**Decision:** `NewRegistry(analyzers ...Analyzer)` — pass analyzers at construction time. No runtime Register method. Simpler, no concurrent mutation concerns.

## Q2: Embedder scope — shared or per-repo?
**Answer:** Single shared embedder
**Decision:** One embedder instance per server. Vocabulary isolation already handled at vector store level. Simpler lifecycle.
