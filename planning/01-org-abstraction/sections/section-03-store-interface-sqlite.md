# Section 03: Store Interface Redesign + SQLite Implementation

## Overview

Redesign the `Store` interface in `internal/org/store.go` for SQLite-native atomic operations, then implement `SQLiteStore`. This replaces the existing `FilesystemStore` (JSON-based, RWMutex) with a proper relational store.

## Prerequisites

- Section 01 complete (migration creates orgs + org_repos tables)
- Existing `internal/org/types.go`: Org, OrgConfig, OrgWithCount types
- Existing `internal/org/store.go`: Current Store interface (Save, Get, List, Delete)
- Existing `internal/org/manager.go`: Manager interface using Store

## What to Build

### 1. Sentinel Error (types.go)

Add to `internal/org/types.go`:

```go
var ErrNotFound = errors.New("org: not found")
```

### 2. Redesigned Store Interface (store.go)

Replace the existing 4-method Store interface with SQLite-optimized operations:

```go
type Store interface {
    // Org CRUD
    SaveOrg(ctx context.Context, o *Org) error
    GetOrg(ctx context.Context, orgID string) (*Org, error)
    ListOrgs(ctx context.Context) ([]OrgWithCount, error)
    DeleteOrg(ctx context.Context, orgID string) error

    // Repo junction — atomic at DB level
    AddRepos(ctx context.Context, orgID string, repoIDs []string) error
    RemoveRepos(ctx context.Context, orgID string, repoIDs []string) error

    // Config overrides
    GetRepoConfigOverride(ctx context.Context, orgID, repoID string) (*OrgConfig, error)
    SetRepoConfigOverride(ctx context.Context, orgID, repoID string, config *OrgConfig) error

    // Migration
    RunMigrations() error
}
```

### 3. SQLiteStore Implementation

**Constructor:**
```go
func NewSQLiteStore(db *sql.DB) (*SQLiteStore, error)
```
- Takes shared `*sql.DB` (same DB as main storage)
- Calls `RunMigrations()` during construction
- Returns error if migration fails

**SaveOrg** — upsert behavior (INSERT OR REPLACE):
- Serialize config to JSON
- INSERT OR REPLACE into orgs
- In same transaction: sync org_repos (delete old, insert new repos from `o.Repos`)
- This preserves backward compatibility — `register_org` can be called multiple times

**GetOrg** — two queries:
- SELECT from orgs WHERE id = ? → if no rows, return `ErrNotFound`
- SELECT repo_id FROM org_repos WHERE org_id = ? → populate Repos slice
- Deserialize config_json back to OrgConfig

**ListOrgs** — aggregate query:
- `SELECT o.id, o.config_json, o.created_at, COUNT(r.repo_id) FROM orgs o LEFT JOIN org_repos r ON o.id = r.org_id GROUP BY o.id`
- Returns `[]OrgWithCount` — does NOT load individual repo IDs (performance)

**DeleteOrg** — simple delete:
- `DELETE FROM orgs WHERE id = ?`
- CASCADE automatically removes org_repos entries
- No error if org doesn't exist (no-op)

**AddRepos** — atomic insert:
- Verify org exists first (SELECT 1 FROM orgs WHERE id = ?)
- `INSERT OR IGNORE INTO org_repos (org_id, repo_id) VALUES (?, ?)` for each repo
- Idempotent — duplicate repos are silently ignored

**RemoveRepos** — atomic delete:
- `DELETE FROM org_repos WHERE org_id = ? AND repo_id IN (?, ?, ...)`
- No error for non-existent repos (no-op)

**GetRepoConfigOverride:**
- `SELECT config_override_json FROM org_repos WHERE org_id = ? AND repo_id = ?`
- Return nil if config_override_json IS NULL
- Deserialize JSON if present

**SetRepoConfigOverride:**
- Serialize config to JSON
- `UPDATE org_repos SET config_override_json = ? WHERE org_id = ? AND repo_id = ?`

### 4. Update Manager Implementation

Update `internal/org/manager.go` to use the new Store interface methods:
- `Register` → calls `store.SaveOrg()` (was `store.Save()`)
- `List` → calls `store.ListOrgs()` (was `store.List()`)
- `Get` → calls `store.GetOrg()` (was `store.Get()`)
- `AddRepos` → calls `store.AddRepos()` directly (was read-modify-write)
- `RemoveRepos` → calls `store.RemoveRepos()` directly (was read-modify-write)
- `Delete` → calls `store.DeleteOrg()` (was `store.Delete()`)

Add `GetEffectiveConfig(ctx, orgID, repoID) (*OrgConfig, error)` method:
1. Get org config via `store.GetOrg()`
2. Get repo override via `store.GetRepoConfigOverride()`
3. Call `MergeConfigs()` from config.go (Section 02)
4. Return merged config

## Tests to Write First

**In store_test.go — table-driven, each test uses fresh `:memory:` SQLite with `_foreign_keys=ON`:**

```go
// SaveOrg tests:
// Test: SaveOrg inserts new org with config — GetOrg returns it
// Test: SaveOrg upserts existing org — updates config, preserves created_at
// Test: SaveOrg with empty repo list succeeds
// Test: SaveOrg with 100 repos succeeds and all are retrievable
// Test: SaveOrg config_json roundtrip — ExcludePatterns and MaxFileSize preserved

// ListOrgs tests:
// Test: ListOrgs returns empty slice when no orgs exist
// Test: ListOrgs returns single org with correct repo count
// Test: ListOrgs returns multiple orgs with correct counts
// Test: ListOrgs with org that has zero repos shows count=0

// GetOrg tests:
// Test: GetOrg returns org with full repo ID list
// Test: GetOrg returns ErrNotFound for non-existent org (check with errors.Is)
// Test: GetOrg config deserialization matches saved config

// AddRepos tests:
// Test: AddRepos adds new repos to existing org
// Test: AddRepos with duplicate repo is idempotent (INSERT OR IGNORE)
// Test: AddRepos to non-existent org returns error (FK constraint)
// Test: AddRepos with empty list is no-op

// RemoveRepos tests:
// Test: RemoveRepos removes specified repos
// Test: RemoveRepos with non-existent repo is no-op
// Test: RemoveRepos all repos leaves org with zero repos

// DeleteOrg tests:
// Test: DeleteOrg removes org and junction entries (CASCADE)
// Test: DeleteOrg for non-existent org is no-op
// Test: After DeleteOrg, GetOrg returns ErrNotFound

// Config override tests:
// Test: SetRepoConfigOverride stores config for specific repo
// Test: GetRepoConfigOverride returns stored config
// Test: GetRepoConfigOverride returns nil for repo without override
// Test: Config override JSON roundtrip preserves all fields

// Concurrent access tests:
// Test: Concurrent SaveOrg from 10 goroutines does not corrupt data
// Test: Concurrent AddRepos and RemoveRepos on same org is safe
// Test: Concurrent reads during writes succeed (WAL mode)
```

## Files to Create/Modify

| File | Action | Purpose |
|------|--------|---------|
| `internal/org/types.go` | Modify | Add ErrNotFound sentinel |
| `internal/org/store.go` | Rewrite | New Store interface + SQLiteStore implementation |
| `internal/org/manager.go` | Modify | Update to use new Store methods, add GetEffectiveConfig |
| `internal/org/store_test.go` | Create | Comprehensive table-driven SQLite store tests |

## Acceptance Criteria

- [ ] Store interface has atomic operations (AddRepos, RemoveRepos — no read-modify-write)
- [ ] SQLiteStore passes all table-driven tests
- [ ] ErrNotFound returned correctly and checkable with errors.Is
- [ ] SaveOrg is upsert (backward compatible with register_org)
- [ ] ListOrgs uses COUNT aggregate (doesn't load repo IDs)
- [ ] Concurrent access tests pass
- [ ] Manager updated to use new Store interface
- [ ] GetEffectiveConfig merges org config with repo override correctly
