# Parallel Plan & Execution in Worktrees

Execute splits in parallel worktrees where dependencies allow, then merge. Speeds up development.

## Dependency Graph

```
01-org-abstraction (no deps)
    │
    ├──► 02-org-semantic-index
    │         │
    │         └──► 03-org-search
    │                   │
    │                   └──► 04-agent-workflows
    │
    └──► 05-plugin-interface (Phase 2, optional)
```

## Execution Waves (Parallel Where Possible)

| Wave | Splits | Parallel? | Branch From |
|------|--------|-----------|-------------|
| 1 | 01-org-abstraction | — | main |
| 2 | 02-org-semantic-index, 05-plugin-interface | **Yes** | 01 |
| 3 | 03-org-search | — | 02 |
| 4 | 04-agent-workflows | — | 03 |

**Wave 2 runs 02 and 05 in parallel** — both depend only on 01.

## Prerequisites

1. **Commit current work** on main (design, planning, requirements)
2. **Ensure `.worktrees/` in .gitignore** (see below)

## Setup

### 1. Add .worktrees to .gitignore

```bash
# In project root
echo ".worktrees/" >> .gitignore
git add .gitignore
git commit -m "chore: add .worktrees to gitignore"
```

### 2. Worktree Directory

Worktrees will be created at `.worktrees/<branch-name>/` (project-local).

---

## Wave 1: 01-org-abstraction

```bash
# Create worktree and branch
git worktree add .worktrees/01-org-abstraction -b feature/01-org-abstraction
cd .worktrees/01-org-abstraction

# Implement
/deep-plan @planning/01-org-abstraction/spec.md

# Verify
go build ./...
go test ./...
```

**Merge when done:**
```bash
git checkout main
git merge feature/01-org-abstraction
git worktree remove .worktrees/01-org-abstraction
git branch -d feature/01-org-abstraction
```

---

## Wave 2: 02 + 05 in Parallel

**Run in separate terminals or Cursor sessions.**

### Terminal A: 02-org-semantic-index

```bash
git worktree add .worktrees/02-org-semantic-index -b feature/02-org-semantic-index main
cd .worktrees/02-org-semantic-index

# Implement (02 depends on 01 — ensure 01 is merged to main first)
/deep-plan @planning/02-org-semantic-index/spec.md
```

### Terminal B: 05-plugin-interface

```bash
git worktree add .worktrees/05-plugin-interface -b feature/05-plugin-interface main
cd .worktrees/05-plugin-interface

/deep-plan @planning/05-plugin-interface/spec.md
```

**Merge order (02 first, then 05):**
```bash
git checkout main
git merge feature/02-org-semantic-index
git merge feature/05-plugin-interface   # Resolve conflicts if any
git worktree remove .worktrees/02-org-semantic-index
git worktree remove .worktrees/05-plugin-interface
```

---

## Wave 3: 03-org-search

```bash
git worktree add .worktrees/03-org-search -b feature/03-org-search main
cd .worktrees/03-org-search

/deep-plan @planning/03-org-search/spec.md
```

**Merge:**
```bash
git checkout main
git merge feature/03-org-search
git worktree remove .worktrees/03-org-search
```

---

## Wave 4: 04-agent-workflows

```bash
git worktree add .worktrees/04-agent-workflows -b feature/04-agent-workflows main
cd .worktrees/04-agent-workflows

/deep-plan @planning/04-agent-workflows/spec.md
```

**Merge:**
```bash
git checkout main
git merge feature/04-agent-workflows
git worktree remove .worktrees/04-agent-workflows
```

---

## Subagent Dispatch (Claude Code Task Tool)

For **parallel execution** in Wave 2, use Task tool to spawn subagents:

```
Task 1: ed3d-plan-and-execute:task-implementor-fast
  Implement Split 02 (org-semantic-index) from planning/02-org-semantic-index/spec.md
  Working directory: .worktrees/02-org-semantic-index

Task 2: ed3d-plan-and-execute:task-implementor-fast
  Implement Split 05 (plugin-interface) from planning/05-plugin-interface/spec.md
  Working directory: .worktrees/05-plugin-interface
```

Run both tasks simultaneously; each works in its own worktree.

---

## Merge Strategy Summary

| Order | Branch | Merges Into |
|-------|--------|-------------|
| 1 | feature/01-org-abstraction | main |
| 2 | feature/02-org-semantic-index | main |
| 3 | feature/05-plugin-interface | main |
| 4 | feature/03-org-search | main |
| 5 | feature/04-agent-workflows | main |

**Conflicts:** 02 and 05 touch different areas (index vs plugin). If conflicts occur, resolve manually before merging 05.

---

## Quick Reference

| Command | Purpose |
|---------|---------|
| `git worktree add .worktrees/XX -b feature/XX main` | Create worktree |
| `git worktree list` | List active worktrees |
| `git worktree remove .worktrees/XX` | Remove worktree |
| `cd .worktrees/XX` | Switch to worktree |
