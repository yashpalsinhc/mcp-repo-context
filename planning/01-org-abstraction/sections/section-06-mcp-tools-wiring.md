# Section 06: MCP Tools + main.go Wiring

## Overview

Register new MCP tools, replace the existing sequential `toolAnalyzeOrg`, and rewire `main.go` to use the new SQLite-backed org store with shared database connection.

## Prerequisites

- Section 03 complete (SQLiteStore, updated Manager)
- Section 04 complete (Analyzer with AnalyzeOrg, NewManager signature change)
- Section 05 complete (filesystem migration)
- Existing `internal/mcp/server.go`: tool dispatch and registration
- Existing `internal/mcp/tools.go`: existing tool implementations including toolAnalyzeOrg
- Existing `cmd/mcp-server/main.go`: server initialization

## What to Build

### 1. New MCP Tool Handlers (tools.go)

Add to `internal/mcp/tools.go`:

**toolAnalyzeOrg** — REPLACE existing sequential implementation:
- Extract `org_id` (required string), `force` (optional bool, default false), `concurrency` (optional int, default 3)
- Clamp concurrency to 1-10 range
- Call `s.orgManager.AnalyzeOrg(ctx, orgID, force, concurrency)`
- Format `AnalysisResult` as text content: total, succeeded, failed, skipped, errors, duration

**toolGetOrg** — NEW:
- Extract `org_id` (required string)
- Call `s.orgManager.Get(ctx, orgID)`
- Return formatted text: org ID, repo count, repo list, config details

**toolDeleteOrg** — NEW:
- Extract `org_id` (required string), `mode` (optional string, default "detach")
- If mode == "cascade": for each repo in org, call `s.orchestratorManager.DeleteRepoContext(ctx, repoID)`
- Call `s.orgManager.Delete(ctx, orgID)`
- Return confirmation text with mode used

**toolUpdateOrgConfig** — NEW:
- Extract `org_id` (required string), `config` object (exclude_patterns, max_file_size)
- Get existing org, update config fields, call SaveOrg
- Return updated org details

**toolAddReposToOrg** — NEW:
- Extract `org_id` (required string), `repo_ids` (required []string)
- Call `s.orgManager.AddRepos(ctx, orgID, repoIDs)`
- Return updated org details

**toolRemoveReposFromOrg** — NEW:
- Extract `org_id` (required string), `repo_ids` (required []string)
- Call `s.orgManager.RemoveRepos(ctx, orgID, repoIDs)`
- Return updated org details

### 2. Tool Definitions (server.go)

Add to `handleListTools()` — JSON Schema inputSchemas:

| Tool | InputSchema |
|------|-------------|
| `analyze_org` | `{ org_id: string (required), force: boolean, concurrency: integer }` |
| `get_org` | `{ org_id: string (required) }` |
| `delete_org` | `{ org_id: string (required), mode: string ("detach" or "cascade") }` |
| `update_org_config` | `{ org_id: string (required), config: { exclude_patterns: []string, max_file_size: integer } }` |
| `add_repos_to_org` | `{ org_id: string (required), repo_ids: []string (required) }` |
| `remove_repos_from_org` | `{ org_id: string (required), repo_ids: []string (required) }` |

Add dispatch cases in `handleCallToolWithID()` switch statement.

### 3. main.go Wiring Changes

**Current flow in `cmd/mcp-server/main.go`:**
```go
store, err := storage.NewFilesystemStore(*storagePath)
orgStore := org.NewFilesystemStore(*storagePath)
orgManager := org.NewManager(orgStore)
```

**New flow:**
```go
// 1. Open shared *sql.DB
db, err := sql.Open("sqlite3", dbPath + "?_journal_mode=WAL&_foreign_keys=ON")
db.SetMaxOpenConns(10)
db.SetMaxIdleConns(5)

// 2. Create main storage with shared DB
store, err := storage.NewSQLiteStoreWithDB(db)

// 3. Create org storage with same DB
orgStore, err := org.NewSQLiteStore(db)

// 4. Run filesystem migration (one-time)
if err := org.MigrateFromFilesystem(*storagePath, orgStore); err != nil {
    log.Printf("WARNING: org filesystem migration failed: %v", err)
}

// 5. Create orchestrator manager (existing)
orchestratorManager := orchestrator.NewManager(store, ...)

// 6. Create org manager (new signature with orchestrator)
orgManager := org.NewManager(orgStore, orchestratorManager)

// 7. Create MCP server
server := mcp.NewServer(orgManager, orchestratorManager, ...)
```

**Key changes:**
- Open `*sql.DB` directly (not via storage.NewSQLiteStore)
- Pass same DB to both stores
- Change org.NewManager call to include orchestrator
- Remove FilesystemStore creation for orgs

### 4. Update Existing Tools

Ensure `register_org` and `list_orgs` work correctly with the new SQLite store. The handlers should not need changes since they call Manager methods, but verify the Manager method names match.

## Tests to Write First

**In tools_test.go or server_test.go:**

```go
// Test: toolAnalyzeOrg dispatches to org.Manager.AnalyzeOrg with correct params
// Test: toolAnalyzeOrg with missing org_id returns error content
// Test: toolAnalyzeOrg formats AnalysisResult as readable text
// Test: toolGetOrg returns full org details as text content
// Test: toolGetOrg with non-existent org returns error text (isError: true)
// Test: toolDeleteOrg with mode=detach removes org, repos remain
// Test: toolDeleteOrg with mode=cascade calls DeleteRepoContext for each repo
// Test: toolDeleteOrg with default mode uses detach
// Test: toolUpdateOrgConfig updates config and returns updated org
// Test: toolAddReposToOrg adds repos and returns updated org
// Test: toolRemoveReposFromOrg removes repos and returns updated org
// Test: toolRegisterOrg uses upsert behavior (backward compatible)
// Test: toolListOrgs returns formatted list with repo counts
```

## Files to Create/Modify

| File | Action | Purpose |
|------|--------|---------|
| `internal/mcp/tools.go` | Modify | Replace toolAnalyzeOrg, add new tool handlers |
| `internal/mcp/server.go` | Modify | Add tool definitions and dispatch cases |
| `cmd/mcp-server/main.go` | Modify | Rewire to shared *sql.DB, new constructors |

## Acceptance Criteria

- [ ] All 6 new/updated MCP tools register and dispatch correctly
- [ ] toolAnalyzeOrg replaced with concurrent version
- [ ] delete_org supports both detach and cascade modes
- [ ] main.go uses shared *sql.DB for both stores
- [ ] Filesystem migration runs on startup
- [ ] Existing register_org and list_orgs still work
- [ ] All tool handler tests pass
