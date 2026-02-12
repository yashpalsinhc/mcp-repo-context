# Split 01: Organization Abstraction

## Purpose

Introduce the organization model to mcp-repo-context. Enables grouping repos under an org for org-level operations.

## Context

- **Requirements:** `/mcp-repo-context/requirements.md`
- **Design:** `/mcp-repo-context/docs/DESIGN_ORG_SEMANTIC_SEARCH.md`
- **Current:** Repos are standalone; no org grouping

## Scope

### In Scope

1. **Org type** — `org_id`, list of repos, config (analyzers, exclusions)
2. **OrgManager** — Create, list, add/remove repos
3. **Storage** — Org metadata in SQLite (new table or extension)
4. **Tools** — `analyze_org`, `list_orgs`, `register_org`
5. **Integration** — Wire into existing `analyze_repo` / `analyze_local` flow

### Out of Scope

- Org-wide semantic index (Split 02)
- Org search (Split 03)
- Agent workflows (Split 04)

## Technical Details

### Data Model

```go
type Org struct {
    ID      string   // e.g. "github.com/LambdatestIncPrivate"
    Repos   []string // repo IDs
    Config  OrgConfig
    Created time.Time
}

type OrgConfig struct {
    ExcludePatterns []string
    MaxFileSize    int64
}
```

### Storage

- New table `orgs` in SQLite (or extend existing schema)
- `org_repos` junction table for org ↔ repo relationship

### Tools

| Tool | Input | Output |
|------|-------|--------|
| `register_org` | org_id, repo_ids | Registered org |
| `list_orgs` | - | List of orgs with repo counts |
| `analyze_org` | org_id, force | Analyzes all repos in org (calls existing analyze) |

## Dependencies

- Existing `storage` package
- Existing `orchestrator` manager
- Existing `repo` package

## Verification

- [ ] `register_org` creates org with repos
- [ ] `list_orgs` returns orgs
- [ ] `analyze_org` triggers analysis for each repo in org
- [ ] Org metadata persists across restarts
