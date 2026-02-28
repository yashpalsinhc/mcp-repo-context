# Spec: Agent Recipes & Pre-Built Workflows

## Purpose

Provide pre-built, high-level workflows that compose multiple tools for common AI agent tasks. Instead of agents making 5-10 tool calls to understand a PR, they make 1 call to `analyze_pr_impact` and get everything they need.

## Background

See `planning/deep_project_interview.md`. The user wants "both raw tools + pre-built recipes." The existing `execute_pattern` feature (`internal/compose/patterns.go`) attempted this but has bugs (silently skipping steps). This split builds proper, reliable recipes on top of the improved core.

Key insight from research: **agents should focus on reasoning, not exploring.** Pre-computed context with hybrid (structural + RAG) retrieval dramatically reduces agent token usage while maintaining quality.

## Scope

### 1. Recipe: analyze_pr_impact
**Input:** repo_id, list of changed files + change types
**Output:** Comprehensive PR impact analysis including:
- Changed functions with behavior summaries
- Direct callers of changed functions (who's affected?)
- Cross-service impact: if a handler is changed, which upstream services call it? (from split 03)
- Dependency impact: if a type is changed, which repos use this module? (from split 02)
- Risk assessment (low/medium/high) with reasoning
- Suggested reviewers (based on who authored the affected code)

**Difference from existing get_pr_context:**
- Includes cross-repo impact (not just single-repo callers)
- Includes dependency-level impact
- AI-generated risk assessment and summary

### 2. Recipe: explain_api_flow
**Input:** endpoint (method + path) or function name, org_id
**Output:** Natural language explanation of the request flow:
- "When a POST request hits /api/v1/login on the auth-service..."
- Step-by-step trace through each service hop
- Data transformations at each step
- Side effects (DB queries, external API calls) at each step
- Mermaid visualization of the full flow

**Builds on:** split 03's `trace_api_flow` + split 02's dependency graph + AI explanation

### 3. Recipe: review_architecture
**Input:** org_id or list of repo_ids
**Output:** Org-wide architecture assessment:
- Service inventory with sizes and responsibilities
- Dependency graph with health indicators (circular deps, tight coupling)
- Shared patterns across repos (common frameworks, error handling)
- Potential issues (orphan services, missing tests, outdated dependencies)
- Recommendations for improvement

**Builds on:** split 02's dependency graph + split 03's service map + AI analysis

### 4. Hybrid Context Assembly
**Required:**
- For each recipe, assemble context from multiple sources:
  - Pre-computed structural data (call graphs, dependencies, service maps)
  - Vector-retrieved content snippets (from split 04, when available)
  - AI-generated summaries (when needed for natural language output)
- Token budget management: each recipe has a configurable token budget
- Smart context selection: include most relevant items first, truncate gracefully

### 5. Agent-Optimized Response Format
**Required:**
- Structured output designed for LLM consumption (not human display)
- Include confidence scores for each piece of information
- Separate "facts" (verifiable from code) from "analysis" (AI-generated)
- Include source references (file:line) for every claim
- Support both JSON (for programmatic use) and markdown (for display)

### 6. Replace/Fix execute_pattern
**Required:**
- Either fix existing pattern system or replace with recipe system
- Recipes should be composable: recipe A can call recipe B as a step
- Clear error reporting when a recipe step fails
- Partial results when some steps succeed and others fail

## Dependencies

- **02-dependency-graph:** Dependency data for cross-repo impact analysis
- **03-api-flow-tracing:** Service maps and flow traces for API flow explanation
- **04-semantic-search:** Vector-ranked content retrieval for hybrid context (optional — recipes work without it, just less rich)

## Provides to Other Splits

- **05-service-layer:** Recipes are exposed as REST API endpoints

## Research from Interview

- **piercelamb plugin patterns:** Pre-compute everything the agent needs. File-based recovery. Progressive context loading.
- **Agent efficiency consensus (2025):** Pre-computed structural context + explicit project files > larger token windows.
- **Progressive loading:** 2K-8K tokens covers 95% of queries. Recipes should target this range.
- **User's agent use cases:** Code review (PR context), feature implementation, cross-repo API tracing — all equally important.

## Key Technical Decisions (Research During /deep-plan)

- Recipe definition format: Go structs vs declarative YAML vs hybrid
- How to compose recipes (function calls vs pipeline pattern)
- Token budgeting strategy per recipe
- Whether to deprecate execute_pattern or evolve it into recipes

## Testing Strategy

- Test each recipe with gorilla/* repos
- Test cross-repo recipes with synthetic multi-service project
- Benchmark token usage: recipe output vs manual tool composition
- Verify response quality: facts match code, confidence scores are calibrated
