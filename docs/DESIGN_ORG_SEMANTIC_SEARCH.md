# MCP Repo Context: Organization-Level Semantic Search Platform

**Design Document** — Transform mcp-repo-context into an agent-first, organization-level platform with Cursor-like semantic search and reduced per-session context processing.

---

## Executive Summary

| Goal | Current State | Target |
|------|---------------|--------|
| Scope | Per-repo only | Organization (multi-repo) |
| Agent help | Limited; tools feel "too small" | High-impact: feature build, refactor, merge |
| Semantic search | Per-repo, requires manual indexing | Org-wide, auto-indexed, cursor-like |
| Context reuse | Re-analyzed per session | Persistent index, incremental updates |
| Architecture | Monolithic Go server | Extensible (plugin-ready) |

---

## 1. Current State Analysis

### What Exists (mcp-repo-context)

```
mcp-repo-context/
├── internal/
│   ├── analyzer/     # Go AST, generic fallback
│   ├── storage/       # Filesystem + SQLite
│   ├── vectors/       # Embeddings, semantic search
│   ├── orchestrator/  # Manager, locks, smart_query
│   ├── mcp/           # Tools, resources, progressive
│   ├── comparison/    # compare_repos, find_duplicates
│   ├── ai/            # Anthropic, ask, context extraction
│   └── ...
```

**Tools (25+):** `analyze_repo`, `analyze_local`, `get_context`, `search_context`, `semantic_search`, `ask`, `compare_repos`, `review_pr`, `get_function_context`, `refresh_file`, etc.

**Gaps:**
- No organization abstraction
- Semantic search is per-repo (index_repository per repo)
- No shared org-level index
- No plugin system
- Agent workflows are fragmented (build feature, refactor, merge need many tool calls)

---

## 2. Cursor Semantic Search Model (Reference)

From [Cursor docs](https://cursor.com/docs/context/semantic-search):

1. **Sync** — Workspace files synced to index
2. **Chunking** — Break into meaningful chunks (functions, classes, blocks)
3. **Embeddings** — Convert chunks to vectors via AI
4. **Vector DB** — Store for similarity search
5. **Query** — Embed query, find similar chunks
6. **Results** — Ranked by semantic similarity

**Key insight:** Compute happens during indexing (offline); search is fast and cheap. Agent uses **both** grep and semantic search together.

---

## 3. Target Architecture

### 3.1 Organization Model

```
Organization
├── org_id: "LambdatestIncPrivate"
├── repos: [repo1, repo2, ...]
├── index: org-wide semantic index
├── metadata: per-repo hashes, last_indexed
└── config: analyzers, embedding model, exclusions
```

**New concepts:**
- `org_id` — e.g. `github.com/LambdatestIncPrivate` or `local:org-name`
- `project_id` — `org_id/repo_name` or `local:path`
- Org-level operations: `analyze_org`, `search_org`, `index_org`

### 3.2 Data Flow (Cursor-Like)

```
┌─────────────────────────────────────────────────────────────────────────┐
│                        INDEXING (Offline / Background)                   │
│                                                                          │
│  Repos (org) → Clone/Scan → Chunk (functions, types) → Embed → Vector DB │
│                                                                          │
│  Triggers: analyze_org, file change (incremental), schedule              │
└─────────────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
┌─────────────────────────────────────────────────────────────────────────┐
│                        SEARCH (Per Session)                              │
│                                                                          │
│  Query → Embed query → Vector similarity → Rank → Return refs            │
│  (No re-analysis; uses persisted index)                                  │
└─────────────────────────────────────────────────────────────────────────┘
```

### 3.3 High-Level Component Diagram

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         MCP Server (Agent-Facing)                            │
│                                                                             │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │ Tool Layer (Thin)                                                     │   │
│  │  analyze_org | search_org | build_feature | refactor_org | merge_repos│   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                    │                                        │
│  ┌─────────────────┬───────────────┼───────────────┬─────────────────────┐ │
│  │                 │               │               │                       │ │
│  ▼                 ▼               ▼               ▼                       ▼ │
│ IndexSvc      SearchSvc      Orchestrator      AI Svc                 PluginMgr│
│ (org index)   (semantic +   (workflows)       (ask, review)            (extend)│
│               keyword)                                                         │
└───────────────────────────────────┬─────────────────────────────────────────┘
                                    │
┌───────────────────────────────────▼───────────────────────────────────────┐
│ Storage Layer                                                               │
│  SQLite (structured) | Vector Store (embeddings) | Metadata Cache           │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 4. Architecture Options

### Option A: Plugin Architecture ( extensible )

**Idea:** Pluggable analyzers, storage backends, embedding providers.

```
mcp-repo-context/
├── cmd/mcp-server/
├── internal/
│   ├── plugin/           # Loader, registry, contract
│   │   ├── contract.go   # AnalyzerPlugin, EmbedderPlugin, StoragePlugin
│   │   └── registry.go
│   └── builtin/          # Default implementations
│       ├── go_analyzer/
│       ├── voyage_embedder/
│       └── sqlite_store/
└── plugins/              # Optional external plugins
    └── typescript/
        └── plugin.yaml
```

**Pros:** Org-specific analyzers, swap embedding model, custom storage  
**Cons:** More complexity, plugin discovery, versioning

### Option B: Modular Monolith (recommended first)

**Idea:** Keep single binary but introduce clear interfaces and modules. Add org layer and improved workflows without plugins yet.

```
internal/
├── org/                  # NEW: org-level abstraction
│   ├── manager.go
│   ├── indexer.go
│   └── types.go
├── search/               # NEW: unified search (keyword + semantic)
│   ├── org_search.go
│   └── ranker.go
└── workflows/            # NEW: agent workflows
    ├── feature_builder.go
    ├── refactor_planner.go
    └── merge_planner.go
```

**Pros:** Simpler, faster to ship, uses existing code  
**Cons:** Less extensible until plugin layer added later

### Option C: Hybrid

Start with **Option B**, add **plugin interfaces** in `internal/plugin/` for analyzers and embedders only. Full plugin system in Phase 2.

---

## 5. Cursor-Like Semantic Search for Multi-Repo

### 5.1 Chunking Strategy

Cursor chunks by: functions, classes, logical blocks. We already have this via AST:

| Chunk Type | Source | Embedding Text |
|------------|--------|----------------|
| Function | Go AST | `name + signature + behavior.Summary` |
| Type | Go AST | `name + kind + fields + methods` |
| File | Generic | `path + purpose` |

**Extend:** Support multiple languages via analyzer registry (same contract).

### 5.2 Org-Wide Index

- **One vector store per org** (or one DB with `org_id` partition)
- **Incremental:** Track file hashes; re-embed only changed files
- **Shared across sessions:** Stored on disk, not in-memory

### 5.3 Search Flow

1. **Keyword (grep-like):** SQLite FTS5 — exact/partial match
2. **Semantic:** Embed query → vector similarity
3. **Hybrid:** Combine scores (e.g. reciprocal rank fusion)
4. **Scope:** Filter by `org_id`, optional `repo_ids`

### 5.4 Reducing Context Processing

| Mechanism | Benefit |
|-----------|---------|
| Persistent embeddings | No re-embed per session |
| Incremental index | Only changed files re-analyzed |
| `refresh_file` | Single-file refresh (~10ms) |
| Token budgets | `get_context_budgeted` sends only relevant chunks |
| Progressive disclosure | Return refs first, expand on demand |

---

## 6. Agent Workflows (High-Impact Tools)

### 6.1 Multi-Repo Feature Builder

**Tool:** `build_feature` (or `plan_feature`)

**Input:** `org_id`, `feature_description`, `target_repos` (optional)

**Flow:**
1. Semantic search across org for relevant code
2. Identify entry points, dependencies, patterns
3. Return: plan + files to touch + suggested order

### 6.2 Multi-Repo Refactor

**Tool:** `refactor_org` (or `plan_refactor`)

**Input:** `org_id`, `pattern_to_refactor`, `repos` (optional)

**Flow:**
1. Semantic search for pattern usages
2. Call graph / impact analysis
3. Return: plan + affected files + risk level

### 6.3 Repo Merge Plan

**Tool:** `merge_repos` (extend existing `compare_repos`)

**Input:** `source_repo_ids`, `target_repo_id`

**Flow:**
1. `find_duplicates`, `find_conflicts`, `find_gaps`
2. Generate merge strategy (order, conflict resolution)
3. Return: plan + duplicates + conflicts + gaps

### 6.4 Org Search (Core)

**Tool:** `search_org`

**Input:** `org_id`, `query`, `search_type` (keyword|semantic|hybrid)

**Flow:**
1. Query org index
2. Rank by relevance
3. Return refs with `detail_ref` for progressive disclosure

---

## 7. Implementation Plan (Splits)

### Split 1: Organization Abstraction

**Scope:** `internal/org/` — org model, repo grouping, org-level config

**Deliverables:**
- `Org` type with `org_id`, `repos`, `config`
- `OrgManager` — list orgs, add/remove repos
- Storage: org metadata in SQLite
- Tools: `analyze_org`, `list_orgs`

### Split 2: Org-Wide Semantic Index

**Scope:** Extend `internal/vectors/` and `internal/storage/` for org partitioning

**Deliverables:**
- Vector store keyed by `org_id`
- `index_org` — index all repos in org (batch)
- Incremental: `refresh_file` / `refresh_changed` at org level
- Chunking: reuse existing analyzer output

### Split 3: Org Search & Hybrid Ranker

**Scope:** `internal/search/` — unified org search

**Deliverables:**
- `search_org` tool — keyword + semantic
- Hybrid ranking (RRF or similar)
- Token-budgeted output
- Progressive disclosure (refs → details)

### Split 4: Agent Workflows

**Scope:** `internal/workflows/` — feature builder, refactor, merge

**Deliverables:**
- `build_feature` / `plan_feature`
- `refactor_org` / `plan_refactor`
- `merge_repos` (extend comparison)
- All return structured plans, not raw dumps

### Split 5: Plugin Interface (Optional / Phase 2)

**Scope:** `internal/plugin/` — analyzer and embedder plugins

**Deliverables:**
- `AnalyzerPlugin` interface
- `EmbedderPlugin` interface
- Registry + discovery
- Built-in implementations as "default" plugins

---

## 8. File Structure (Proposed)

```
mcp-repo-context/
├── cmd/mcp-server/
├── internal/
│   ├── org/              # NEW
│   │   ├── manager.go
│   │   ├── indexer.go
│   │   └── types.go
│   ├── search/           # NEW (or extend orchestrator)
│   │   ├── org_search.go
│   │   └── ranker.go
│   ├── workflows/        # NEW
│   │   ├── feature.go
│   │   ├── refactor.go
│   │   └── merge.go
│   ├── plugin/           # NEW (Phase 2)
│   │   ├── contract.go
│   │   └── registry.go
│   ├── analyzer/
│   ├── storage/
│   ├── vectors/
│   ├── orchestrator/
│   ├── mcp/
│   └── ...
├── data/
│   └── contexts/
│       └── orgs/         # Org-level index
├── docs/
│   └── DESIGN_ORG_SEMANTIC_SEARCH.md (this file)
└── requirements.md
```

---

## 9. Success Metrics

| Metric | Current | Target |
|--------|---------|--------|
| Org-level queries | N/A | `search_org` returns cross-repo results |
| Session startup | Re-analyze or load full JSON | Load from index, no re-embed |
| Agent tool calls for "build feature" | 10+ fragmented | 1–2 workflows |
| Semantic search scope | Single repo | Full org |
| Plugin support | None | Analyzer + embedder pluggable (Phase 2) |

---

## 10. Next Steps

1. **Validate** this design with stakeholders
2. **Start with Split 1** (org abstraction) — low risk, enables others
3. **Then Split 2** (org index) — core for semantic search
4. **Then Split 3** (search_org) — agent-facing
5. **Then Split 4** (workflows) — high-impact tools
6. **Defer Split 5** (plugins) until needed

---

## References

- [Cursor Semantic Search](https://cursor.com/docs/context/semantic-search)
- [MCP Best Practices](https://modelcontextprotocol.info/docs/best-practices/)
- [mcp-repo-context IMPLEMENTATION_ROADMAP.md](../IMPLEMENTATION_ROADMAP.md)
- [mcp-repo-context CLAUDE.md](../CLAUDE.md)
