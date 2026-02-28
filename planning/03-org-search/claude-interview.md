# Interview: Org Search & Hybrid Ranker

## Q1: FTS5 vs LIKE for org-wide search?
**Answer:** Add FTS5
**Decision:** Add FTS5 virtual tables for org-wide full-text search. Better performance for large orgs. Keep existing LIKE-based methods for backward compatibility.

## Q2: Hybrid ranking algorithm?
**Answer:** RRF (Reciprocal Rank Fusion)
**Decision:** Use RRF for merging keyword and semantic results. Simple formula: score(d) = 1/(k+rank_keyword) + 1/(k+rank_semantic), where k=60 is standard. No tuning needed.
