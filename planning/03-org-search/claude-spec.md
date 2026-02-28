# Spec: Org Search & Hybrid Ranker

## Problem
Current search tools (search_context, semantic_search) are per-repo only. Users with org-level indexed repos need to search across all repos in one call, with results ranked by relevance using both keyword and semantic signals.

## Requirements
1. New `search_org` MCP tool that queries across all repos in an org
2. Three search modes: keyword (FTS5), semantic (vector similarity), hybrid (RRF merge)
3. Token budgeting on output
4. Progressive disclosure via detail_ref for expansion with get_function_context
5. FTS5 virtual tables for performant full-text search

## Design Decisions
- FTS5 for keyword search (user chose over LIKE)
- RRF (Reciprocal Rank Fusion) for hybrid ranking (k=60)
- Reuse existing progressive.go infrastructure (currently unused)
- Reuse existing token budgeter

## Dependencies
- 01-org-abstraction (org store, org repos)
- 02-org-semantic-index (org-level vector indexing)
