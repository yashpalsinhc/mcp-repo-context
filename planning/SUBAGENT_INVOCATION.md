# Subagent Invocation for Parallel Wave 2

Custom subagents are defined in `.cursor/agents/`. Cursor Agent can invoke them when you ask.

## Option 0: Ask Agent (Recommended)

Say to Cursor Agent:

> **Spawn split-02 and split-05 subagents in parallel. Each implements its spec in its worktree. Run both in background.**

Or:

> **Invoke subagents split-02-org-semantic-index and split-05-plugin-interface in parallel.**

Agent will use the custom subagents from `.cursor/agents/` and run them in parallel.

---

## Option A: Cursor Composer (Two Windows)

---

## Option A: Cursor Composer (Two Windows)

### Window 1 — Split 02: Org Semantic Index

```
Implement Split 02 (org-semantic-index) from the spec.

**Working directory:** /Users/yashpalc/yashpalc-mcp/mcp-repo-context/.worktrees/02-org-semantic-index

**Spec:** planning/02-org-semantic-index/spec.md

**Tasks:**
1. Add org_id to VectorRecord and vectors table schema
2. Extend Search to accept org_id (search across org repos)
3. Add index_org tool: for each repo in org, get context, extract chunks, embed, store with org_id
4. Wire index_org into MCP server

**Context:** Split 01 (org abstraction) is merged. Use internal/org for org lookup. Use internal/vectors for embedding. Reuse IndexRepository logic from internal/vectors/search.go.
```

### Window 2 — Split 05: Plugin Interface

```
Implement Split 05 (plugin-interface) from the spec.

**Working directory:** /Users/yashpalc/yashpalc-mcp/mcp-repo-context/.worktrees/05-plugin-interface

**Spec:** planning/05-plugin-interface/spec.md

**Tasks:**
1. Create internal/plugin/contract.go with AnalyzerPlugin and EmbedderPlugin interfaces
2. Create internal/plugin/registry.go with RegisterAnalyzer, RegisterEmbedder
3. Register built-in Go analyzer and default embedder as plugins
4. Update analyzer registry and vectors to use plugin registry (optional wiring)

**Context:** Split 01 is merged. Analyzer interface in internal/analyzer. Embedder in internal/vectors.
```

---

## Option B: Claude Code Task Tool ( ed3d-plan-and-execute )

If you have the Task tool (Claude Code with ed3d-plan-and-execute):

```
Task 1:
  subagent_type: ed3d-plan-and-execute:task-implementor-fast
  description: Implement Split 02 (org-semantic-index)
  prompt: |
    Implement Split 02 from planning/02-org-semantic-index/spec.md.
    Working directory: /Users/yashpalc/yashpalc-mcp/mcp-repo-context/.worktrees/02-org-semantic-index
    Add org_id to vector store, index_org tool, SearchByOrg.
    Commit when done.

Task 2:
  subagent_type: ed3d-plan-and-execute:task-implementor-fast
  description: Implement Split 05 (plugin-interface)
  prompt: |
    Implement Split 05 from planning/05-plugin-interface/spec.md.
    Working directory: /Users/yashpalc/yashpalc-mcp/mcp-repo-context/.worktrees/05-plugin-interface
    Create plugin interfaces, registry, register built-ins.
    Commit when done.
```

Run both tasks in parallel.

---

## After Both Complete

```bash
cd /Users/yashpalc/yashpalc-mcp/mcp-repo-context
./scripts/worktree-merge.sh 02-org-semantic-index
./scripts/worktree-merge.sh 05-plugin-interface
```
