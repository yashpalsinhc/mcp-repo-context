# MCP Repo Context - Organization Semantic Search Platform

## Project Description

Transform the existing mcp-repo-context MCP server into a production-ready, organization-level platform that helps AI agents build multi-repo features, refactor code across repositories, and merge repos. The current implementation provides too little value to agents as MCP—we need something like [Cursor's semantic search](https://cursor.com/docs/context/semantic-search) for multi-repo contexts, with reduced per-session context processing.

## Goals

1. **Organization-level model**: One organization, many repositories—agents work across an org's codebase, not just single repos.
2. **Agent-first design**: Current MCP gives "too small help" to agents; tools must be more impactful for multi-repo features, refactoring, and merge planning.
3. **Cursor-like semantic search**: Build semantic search (vector embeddings, chunking, similarity) for multi-repo—find code by meaning, not just keywords.
4. **Reduce context processing per session**: Index/cache so the same context is not reprocessed every session; embeddings and analysis should persist and be reused.
5. **Plugin or better architecture**: Design MCP with plugin architecture or superior alternatives for extensibility.

## Current State (mcp-repo-context)

- **Existing**: Go MCP server, AST analysis (Go), SQLite storage, vector embeddings, semantic search, call graph, PR review, skills, Docker.
- **Gaps**: No org-level grouping; limited agent utility; no shared indexing across sessions; no plugin system; semantic search is per-repo only.

## Feature Requirements

### Core

- **Organization abstraction**: Group repos under an org; support org-level queries, comparisons, and indexing.
- **Persistent semantic index**: Vector embeddings stored and reused across sessions; incremental updates when code changes.
- **Multi-repo semantic search**: Query across all org repos by meaning; rank by relevance and scope.
- **Cross-session context reuse**: Pre-computed context (embeddings, summaries, call graphs) cached; avoid re-analysis on every session.

### Agent-Facing

- **Multi-repo feature builder**: "Build feature X across repos A, B, C" — find relevant code, plan changes, identify dependencies.
- **Multi-repo refactor**: "Refactor pattern Y across the org" — find usages, plan refactor, detect conflicts.
- **Repo merge planning**: "Merge repos A and B" — find duplicates, conflicts, gaps; suggest merge strategy.
- **Context-aware answers**: Reduce token usage by sending only relevant context; use token budgets and progressive disclosure.

### Technical

- **Semantic search pipeline**: Chunking (functions, types, logical blocks); embeddings; vector store; similarity search.
- **Caching**: Embeddings, architecture summaries, call graphs; invalidation on file/repo changes.
- **Plugin architecture**: Extensible analyzers, storage backends, embedding providers; org-specific plugins.

## Constraints

- Must work with existing MCP protocol (Cursor, Claude Code, etc.).
- Must support private repos (GitHub token).
- Local and remote (GitHub) repos.
- Reasonable resource usage (memory, disk, API calls).

## Known Context

- Existing codebase: `/Users/yashpalc/yashpalc-mcp/mcp-repo-context`
- Already has: SQLite, vectors, embeddings, semantic search, call graph, PR review.
- Cursor docs reference: https://cursor.com/docs/context/semantic-search
- Target: Single organization, multi-repo workflows.

## Design Output

- **Design document:** `docs/DESIGN_ORG_SEMANTIC_SEARCH.md`
- **Project manifest:** `planning/project-manifest.md`
- **Split specs:** `planning/01-org-abstraction/spec.md` through `planning/05-plugin-interface/spec.md`
