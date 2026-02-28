# Spec: Agent Workflows

## Problem
Agents must chain many individual tool calls (search, get_context, compare, ask) to accomplish common multi-repo tasks. This is slow, token-expensive, and error-prone. Need high-level workflow tools that orchestrate these operations.

## Requirements

1. **build_feature** — "Build feature X across repos": semantic search for relevant code, identify entry points and dependencies, return plan with files to touch and suggested order. Algorithmic analysis first, optional AI enhancement.
2. **refactor_org** — "Refactor pattern Y across org": find pattern usages via semantic search, call graph/impact analysis, return plan with affected files and risk level. Hybrid: algorithmic + optional AI.
3. **merge_repos** — extend compare_repos with merge strategy: call find_duplicates/find_conflicts/find_gaps, generate advisory merge report with consolidation targets, conflict resolutions, gap fills, suggested order.

## Design Decisions
- Hybrid approach: algorithmic analysis first, optional AI enhancement via Ask() when available
- Advisory output (not executable plans) — agents/users decide what to act on
- Reuse existing infrastructure: Comparer, SmartQuery, SemanticSearch, TokenBudgeter, compose.Chain
- Token-budgeted output to prevent oversized responses

## Dependencies
- 01-org-abstraction (org store)
- 02-dependency-graph (call graph)
- 04-semantic-search (vector search for feature-level queries)
