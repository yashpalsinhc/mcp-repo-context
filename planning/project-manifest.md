<!-- SPLIT_MANIFEST
01-org-abstraction
02-org-semantic-index
03-org-search
04-agent-workflows
05-plugin-interface
END_MANIFEST -->

# Project Manifest: MCP Repo Context - Organization Semantic Search

## Overview

Transform mcp-repo-context into an organization-level platform with Cursor-like semantic search and reduced per-session context processing.

## Split Structure

| Split | Purpose | Dependencies |
|-------|---------|--------------|
| 01-org-abstraction | Org model, repo grouping, org-level config | None |
| 02-org-semantic-index | Org-wide vector index, incremental updates | 01 |
| 03-org-search | search_org tool, hybrid ranking | 01, 02 |
| 04-agent-workflows | build_feature, refactor_org, merge_repos | 01, 02, 03 |
| 05-plugin-interface | Analyzer + embedder plugins (Phase 2) | 01 |

## Execution Order

1. **01-org-abstraction** — Foundation; enables all org-level features
2. **02-org-semantic-index** — Core indexing; depends on org model
3. **03-org-search** — Agent-facing search; depends on index
4. **04-agent-workflows** — High-impact tools; depends on search
5. **05-plugin-interface** — Optional; can run in parallel with 04 if desired

## Cross-Cutting Concerns

- Storage: SQLite already exists; extend schema for org metadata
- Vectors: Extend existing `internal/vectors` for org partitioning
- MCP: Add new tools in `internal/mcp/tools.go`

## Parallel Execution (Worktrees)

Use worktrees for fast parallel development. See **`planning/PARALLEL_WORKTREE_WORKFLOW.md`**.

**Waves:**
- Wave 1: 01 only
- Wave 2: **02 + 05 in parallel** (both depend on 01)
- Wave 3: 03
- Wave 4: 04

**Quick start:**
```bash
./scripts/worktree-create.sh 01-org-abstraction
cd .worktrees/01-org-abstraction
# implement, then:
./scripts/worktree-merge.sh 01-org-abstraction
```

## /deep-plan Commands

```bash
/deep-plan @planning/01-org-abstraction/spec.md
/deep-plan @planning/02-org-semantic-index/spec.md
/deep-plan @planning/03-org-search/spec.md
/deep-plan @planning/04-agent-workflows/spec.md
/deep-plan @planning/05-plugin-interface/spec.md
```
