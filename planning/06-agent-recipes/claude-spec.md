# Spec: Agent Recipes & Pre-Built Workflows

## Purpose

Provide pre-built, high-level workflows (recipes) that compose multiple tools for common AI agent tasks. Instead of agents making 5-10 tool calls to understand a PR, they make 1 call to `analyze_pr_impact` and get everything they need. Recipes replace the existing `execute_pattern` system which has bugs (silently skipping steps).

## Background

Key insight: agents should focus on reasoning, not exploring. Pre-computed context with hybrid (structural + RAG) retrieval dramatically reduces agent token usage while maintaining quality. Progressive loading targets 2K-8K tokens for 95% of queries.

## Architecture Decisions

1. **Replace execute_pattern** — The existing Pattern/Chain system in `internal/compose/` is deprecated. Recipes become the single abstraction for composed workflows. Avoids maintaining two systems.

2. **Partial results on missing data** — Recipes return what's available and clearly mark sections as "not available — requires [split name]." Recipes work progressively as more features are implemented.

3. **Structured + AI summary** — Recipe output includes structured JSON data plus an optional AI-generated natural language summary. Agents can use either format.

## Requirements

### R1: Recipe Framework
- Define a Recipe interface that replaces Pattern
- Each recipe has: name, description, input schema, output schema
- Recipes are composable (recipe A can use recipe B as a step)
- Clear error reporting when a step fails
- Partial results when some steps succeed and others fail
- Token budget management per recipe

### R2: Recipe — analyze_pr_impact
- Input: repo_id, list of changed files + change types
- Output: changed functions with behavior, direct callers, cross-service impact (from 03-api-flow-tracing when available), dependency impact (from 02-dependency-graph when available), risk assessment (AI), suggested reviewers
- Extends existing get_pr_context with cross-repo and AI capabilities

### R3: Recipe — explain_api_flow
- Input: endpoint (method + path) or function name, org_id
- Output: natural language request flow explanation, step-by-step trace through services, data transformations, side effects at each step, Mermaid visualization
- Builds on 03-api-flow-tracing + 02-dependency-graph + AI explanation

### R4: Recipe — review_architecture
- Input: org_id or list of repo_ids
- Output: service inventory, dependency graph with health indicators, shared patterns, potential issues, recommendations
- Builds on 02-dependency-graph + AI analysis

### R5: Hybrid Context Assembly
- Assemble context from: pre-computed structural data, vector-retrieved content (from 04-semantic-search when available), AI summaries
- Token budget management with smart context selection
- Include most relevant items first, truncate gracefully

### R6: Agent-Optimized Response Format
- Structured output for LLM consumption
- Separate "facts" (from code) from "analysis" (AI-generated)
- Source references (file:line) for every claim
- Confidence scores for AI-generated content
- Support JSON and markdown output

### R7: Deprecate execute_pattern
- Mark existing patterns as deprecated
- Provide migration path: each existing pattern has a recipe equivalent
- Remove old Pattern/Chain/ToolExecutor interfaces after migration

## Dependencies

- 02-dependency-graph: cross-repo dependency data (optional — recipes degrade gracefully)
- 03-api-flow-tracing: service maps and flow traces (optional — recipes degrade gracefully)
- 04-semantic-search: vector search for hybrid context (optional — recipes use keyword fallback)

## Existing Code to Build On

- `internal/orchestrator/pr_context.go` — existing PR context (single-repo)
- `internal/ai/` — AI provider, context extraction, retry
- `internal/tokens/` — token budgeting and scoring
- `internal/vectors/` — semantic search
- `internal/compose/` — existing patterns (to be replaced)

## Testing Strategy

- Unit tests for each recipe with mock data
- Integration tests with gorilla/* repos
- Benchmark token usage: recipe output vs manual tool composition
- Verify partial results when dependencies are missing
