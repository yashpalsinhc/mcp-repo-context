# Specification: Organization Abstraction Layer (Split 01)

## Overview

Introduce a complete organization model to mcp-repo-context, enabling grouping of repositories under organizations with org-level configuration, analysis, and management. This is the foundation for all subsequent org-level features (semantic index, search, agent workflows, plugins).

## Current State

The `internal/org/` package already exists with:
- `Org` type (ID, Repos, Config, Created)
- `OrgConfig` (ExcludePatterns, MaxFileSize)
- `Manager` interface (Register, List, Get, AddRepos, RemoveRepos, Delete)
- `FilesystemStore` implementation (JSON file, RWMutex)
- MCP tools `register_org` and `list_orgs` already wired

The vectors table already has an `org_id` field with indexing.

## Target State

Migrate org storage from filesystem JSON to SQLite, add `analyze_org` tool with configurable concurrency and retry logic, implement org config inheritance, and provide comprehensive tests with benchmarks.

## Requirements

### R1: SQLite-Backed Org Storage

**Migrate from FilesystemStore to SQLite.**

New tables in SQLite schema:
- `orgs`: id (PK TEXT), config_json (TEXT), created_at, updated_at
- `org_repos`: org_id (FK), repo_id (FK), added_at — junction table
- Indexes on org_repos(org_id) and org_repos(repo_id)
- Foreign keys with ON DELETE behavior controlled at query time (see R6)

The existing `org.Manager` interface stays the same. Replace `FilesystemStore` with `SQLiteStore`.

### R2: analyze_org Tool

New MCP tool that analyzes all repos in an organization.

**Parameters:**
- `org_id` (required): Organization to analyze
- `force` (optional, default false): Force re-analysis even if cached
- `concurrency` (optional, default 3): Max parallel repo analyses (1-10)

**Behavior:**
1. Look up org by ID, get list of repos
2. For each repo, run existing `AnalyzeRepo` or `AnalyzeLocal`
3. On failure: retry once, then continue
4. Return summary: total repos, succeeded, failed (with errors), skipped (already cached)

**Concurrency:** Use a semaphore/worker pool pattern bounded by `concurrency` parameter.

### R3: Config Inheritance

Org config should cascade to repos with per-repo override capability.

**Resolution order:**
1. Per-repo config (if set) takes precedence
2. Org config (inherited default)
3. Global system config (fallback)

**Implementation:** When analyzing a repo in org context, merge configs. Add `GetEffectiveConfig(orgID, repoID)` to Manager.

### R4: Org Deletion with User Choice

`delete_org` tool takes a `mode` parameter:
- `mode: "detach"` (default): Remove org and org-repo links. Repos remain.
- `mode: "cascade"`: Remove org, links, AND delete all repo contexts.

### R5: MCP Tools

| Tool | Params | Returns |
|------|--------|---------|
| `register_org` | org_id, repo_ids, config? | Org details |
| `list_orgs` | - | List of orgs with counts |
| `get_org` | org_id | Full org details with repo list |
| `analyze_org` | org_id, force?, concurrency? | Analysis summary |
| `delete_org` | org_id, mode? | Confirmation |
| `update_org_config` | org_id, config | Updated config |
| `add_repos_to_org` | org_id, repo_ids | Updated org |
| `remove_repos_from_org` | org_id, repo_ids | Updated org |

### R6: Full Test Suite

**Unit tests (table-driven):**
- SQLite org store: CRUD operations, concurrent access, edge cases
- Config inheritance resolution
- Manager operations

**Integration tests:**
- analyze_org with mock repos
- MCP tool handlers end-to-end
- Deletion modes (detach vs cascade)

**Benchmarks:**
- Storage CRUD: Register, List, Get, Delete orgs at various scales (1, 10, 50 orgs)
- Add/remove repos: 1, 20, 100 repos per org
- analyze_org pipeline: 5, 20, 50 repos with configurable concurrency

### R7: Schema Migration

Add new migration file `003_org_tables.sql`:
- Create `orgs` and `org_repos` tables
- Follow existing migration pattern
- Support both fresh install and upgrade from existing data

## Non-Requirements

- Org-wide semantic index (Split 02)
- Org search across repos (Split 03)
- Agent workflows (Split 04)
- Plugin interface (Split 05)

## Technical Constraints

- Must work with existing `mattn/go-sqlite3` driver
- Must maintain WAL mode and existing connection pooling
- Must not break existing repo-level operations
- SQLite in-memory for tests (`?_foreign_keys=ON`)
- Follow existing manager/store/interface patterns

## Scale Expectations

- 10-50 organizations
- 20-100 repos per org
- Medium enterprise use case
- Needs proper indexing on junction table
