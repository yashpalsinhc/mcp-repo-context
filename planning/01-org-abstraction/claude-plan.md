# Implementation Plan: Organization Abstraction Layer (Split 01)

## 1. Background and Goals

The mcp-repo-context MCP server currently treats repositories as standalone entities. This split introduces an **Organization** abstraction that groups repos under an org for collective management, analysis, and configuration.

The org package (`internal/org/`) already exists with types, a Manager interface, a Store interface, and a FilesystemStore using JSON. Two MCP tools (`register_org`, `list_orgs`) and an existing sequential `toolAnalyzeOrg` are already wired. The goal is to:

1. **Migrate org storage to SQLite** — redesign Store interface for atomic operations
2. **Replace sequential `toolAnalyzeOrg`** with configurable concurrency and retry
3. **Implement config inheritance** (org → repo with override)
4. **Add deletion with mode choice** (detach vs cascade)
5. **Provide comprehensive tests and benchmarks**
6. **Migrate existing filesystem data** to SQLite for existing users

This is the foundation for Splits 02-05 (semantic index, org search, agent workflows, plugins).

## 2. Architecture Overview

### Package Layout

```
internal/org/
  types.go          # Existing: Org, OrgConfig, OrgWithCount (extend with ErrNotFound)
  manager.go        # Existing: Manager interface (extend with AnalyzeOrg, GetEffectiveConfig)
  store.go          # Rewrite: Replace FilesystemStore with SQLiteStore, redesign Store interface
  config.go         # New: Config inheritance resolution (pure function)
  analyzer.go       # New: analyze_org orchestration (concurrency, retry)
  migrate_fs.go     # New: FilesystemStore → SQLite one-time data migration
  store_test.go     # New: Table-driven SQLite store tests
  manager_test.go   # New: Manager integration tests
  config_test.go    # New: Config resolution tests
  analyzer_test.go  # New: Analyzer tests (mock repos)
  benchmark_test.go # New: Storage + pipeline benchmarks
```

### Dependency Direction

```
cmd/mcp-server/main.go
  └── internal/mcp/server.go (MCP tools)
        └── internal/org/manager.go (Manager interface)
              ├── internal/org/store.go (SQLiteStore - persistence)
              ├── internal/org/config.go (config resolution)
              ├── internal/org/analyzer.go (analyze_org logic)
              └── internal/orchestrator/ (repo analysis - existing)
```

## 3. Store Interface Redesign

### Current State

The existing `Store` interface has 4 methods: `Save`, `Get`, `List`, `Delete`. The `Manager` layer does read-modify-write on top (e.g., `AddRepos` reads org, appends repos, saves). This pattern has race conditions with concurrent access.

### New Store Interface

Redesign `Store` for SQLite-native atomic operations:

```go
type Store interface {
    // Org CRUD
    SaveOrg(ctx context.Context, o *Org) error        // INSERT or UPDATE (upsert)
    GetOrg(ctx context.Context, orgID string) (*Org, error)
    ListOrgs(ctx context.Context) ([]OrgWithCount, error)
    DeleteOrg(ctx context.Context, orgID string) error

    // Repo junction (atomic at DB level)
    AddRepos(ctx context.Context, orgID string, repoIDs []string) error
    RemoveRepos(ctx context.Context, orgID string, repoIDs []string) error

    // Config
    GetRepoConfigOverride(ctx context.Context, orgID, repoID string) (*OrgConfig, error)
    SetRepoConfigOverride(ctx context.Context, orgID, repoID string, config *OrgConfig) error

    // Migration
    RunMigrations() error
}
```

`SaveOrg` uses INSERT OR REPLACE (upsert) to maintain backward compatibility — existing users calling `register_org` multiple times to update repos will not break.

`AddRepos` uses `INSERT OR IGNORE` for idempotency. `RemoveRepos` uses `DELETE ... WHERE IN`. Both are atomic single-statement operations — no read-modify-write races.

### Sentinel Error

Define in `types.go`:
```go
var ErrNotFound = errors.New("org: not found")
```

Return from `GetOrg` when org doesn't exist. Matches the pattern used by `storage.ErrNotFound`.

## 4. Schema Migration

### New Migration: `003_org_tables.sql`

Embedded via `//go:embed` directive following the existing pattern in `internal/storage/sqlite.go`.

**`orgs` table:**
- `id` TEXT PRIMARY KEY
- `config_json` TEXT — serialized OrgConfig
- `created_at` DATETIME DEFAULT CURRENT_TIMESTAMP
- `updated_at` DATETIME DEFAULT CURRENT_TIMESTAMP

**`org_repos` junction table:**
- `org_id` TEXT NOT NULL REFERENCES orgs(id) ON DELETE CASCADE
- `repo_id` TEXT NOT NULL
- `config_override_json` TEXT — nullable, per-repo config overrides
- `added_at` DATETIME DEFAULT CURRENT_TIMESTAMP
- PRIMARY KEY (org_id, repo_id)
- INDEX on repo_id for reverse lookups

**SQLite trigger for updated_at:**

```sql
CREATE TRIGGER IF NOT EXISTS update_orgs_timestamp
AFTER UPDATE ON orgs
BEGIN
    UPDATE orgs SET updated_at = CURRENT_TIMESTAMP WHERE id = NEW.id;
END;
```

### Migration Runner

Follow the existing hard-coded pattern: add `//go:embed` for the SQL file, create a `migrateOrgTables()` method, and call it from the `migrate()` chain in `SQLiteStore`. The migration checks `schema_migrations` and only runs if not already applied.

## 5. SQLite Org Store Implementation

### Constructor

```go
func NewSQLiteStore(db *sql.DB) (*SQLiteStore, error)
```

Takes the shared `*sql.DB`. Runs migrations on construction. Returns error if migration fails.

### Database Sharing with main.go

**Current problem:** `main.go` creates `storage.FilesystemStore`, and the `storage.SQLiteStore.db` field is unexported.

**Solution:** Open `*sql.DB` directly in `main.go` with proper pragmas (WAL, foreign_keys=ON), then pass it to both:
- `storage.NewSQLiteStoreWithDB(db)` — new constructor accepting pre-opened DB
- `org.NewSQLiteStore(db)` — new org store

This requires adding a `NewSQLiteStoreWithDB(db *sql.DB) *SQLiteStore` constructor to the storage package. The existing `NewSQLiteStore(path)` can delegate to this.

### Store Operations

**SaveOrg:** INSERT OR REPLACE into orgs. In same transaction, sync org_repos: delete removed repos, insert new ones. This makes `register_org` an upsert — backward compatible.

**ListOrgs:** `SELECT o.id, o.config_json, o.created_at, COUNT(r.repo_id) FROM orgs o LEFT JOIN org_repos r ON o.id = r.org_id GROUP BY o.id`. Returns `OrgWithCount` without loading individual repo IDs (performance optimization for large orgs).

**GetOrg:** Two queries: SELECT from orgs + SELECT repo_ids from org_repos. Loads full repo list.

**AddRepos:** `INSERT OR IGNORE INTO org_repos (org_id, repo_id) VALUES (?, ?)` for each repo. Atomic, idempotent.

**RemoveRepos:** `DELETE FROM org_repos WHERE org_id = ? AND repo_id IN (?, ?, ...)`. Single statement.

**DeleteOrg:** `DELETE FROM orgs WHERE id = ?`. CASCADE removes junction entries automatically.

## 6. Config Inheritance

### Resolution Logic

Add `GetEffectiveConfig(orgID, repoID string) (*OrgConfig, error)` to Manager.

**Resolution order:**
1. Per-repo config override from `org_repos.config_override_json` (if non-null)
2. Org-level config from `orgs.config_json`
3. System defaults

**Merge function in config.go:**

`MergeConfigs(orgConfig, repoOverride *OrgConfig) *OrgConfig` — pure function.

- `ExcludePatterns`: union (repo adds to org's list, deduplicated)
- `MaxFileSize`: repo value overrides org value if non-zero

Returns a new `*OrgConfig` with merged values. Used by the analyzer when processing repos in org context.

## 7. analyze_org Orchestration

### Replace Existing Sequential Implementation

The current `toolAnalyzeOrg` in `internal/mcp/tools.go` iterates repos sequentially. Replace it with a call to the new `org.Analyzer`.

### Analyzer Constructor

```go
func NewAnalyzer(orgManager Manager, orchestrator orchestrator.Manager) *Analyzer
```

The `Analyzer` needs both the org manager (to look up orgs and configs) and the orchestrator (to trigger repo analysis). This means **`org.NewManager` signature changes** to accept `orchestrator.Manager`:

```go
func NewManager(store Store, orch orchestrator.Manager) Manager
```

The `Analyzer` is created inside the Manager, not as a separate top-level component.

### AnalyzeOrg Logic

1. Get org via Store.GetOrg — return error if not found
2. Get effective config for each repo via GetEffectiveConfig
3. Create buffered channel semaphore: `sem := make(chan struct{}, concurrency)`
4. Launch goroutines per repo, bounded by semaphore
5. Each goroutine determines analysis type:
   - `local:/...` prefix → `orchestrator.AnalyzeLocal(path, force)`
   - Otherwise → `orchestrator.AnalyzeRepo(url, force)`
6. On failure: retry once after 1-second delay. Skip non-retryable errors (repo not found, invalid path)
7. Respect `ctx.Done()` — stop launching new goroutines on cancellation
8. Collect all results into `AnalysisResult`

**Concurrency parameter:** Clamped to range 1-10 in the tool handler (not via JSON Schema constraints). Default is 3.

**Expected latency note:** For large orgs (50+ repos), analysis may take several minutes. The tool description should document this.

### Adding DeleteRepoContext to Orchestrator

For cascade deletion, add `DeleteRepoContext(ctx context.Context, repoID string) error` to `orchestrator.Manager` interface. Implementation delegates to `storage.ContextStore.DeleteContext()`. This maintains proper layering — org package calls orchestrator, not storage directly.

## 8. MCP Tool Definitions

### New Tools

**`analyze_org`** — replaces existing sequential implementation:
- Input: `{ org_id: string, force?: boolean, concurrency?: integer }`
- Output: Text summary (total, succeeded, failed, skipped, errors, duration)

**`get_org`:**
- Input: `{ org_id: string }`
- Output: Full org details with repo list and config

**`delete_org`:**
- Input: `{ org_id: string, mode?: "detach"|"cascade" }`
- Default mode: "detach" — removes org and links, repos remain
- "cascade" — also calls `orchestrator.DeleteRepoContext()` for each repo

**`update_org_config`:**
- Input: `{ org_id: string, config: { exclude_patterns?: []string, max_file_size?: integer } }`
- Output: Updated org details

**`add_repos_to_org`:**
- Input: `{ org_id: string, repo_ids: []string }`
- Output: Updated org

**`remove_repos_from_org`:**
- Input: `{ org_id: string, repo_ids: []string }`
- Output: Updated org

### Existing Tools to Update

- `register_org`: Switch to use new SQLite store (upsert behavior preserved)
- `list_orgs`: Switch to use new SQLite store
- `toolAnalyzeOrg`: Replace body with call to `org.Manager.AnalyzeOrg()`

### Registration Pattern

Follow existing switch-case dispatch in `handleCallToolWithID()`. Add tool definitions in `handleListTools()` with JSON Schema inputSchemas.

## 9. main.go Wiring Changes

**Current flow:**
1. `storage.NewFilesystemStore(path)` for main storage
2. `org.NewFilesystemStore(path)` for org storage
3. `org.NewManager(orgStore)` — single arg

**New flow:**
1. Open `*sql.DB` with pragmas (`_journal_mode=WAL`, `_foreign_keys=ON`)
2. `storage.NewSQLiteStoreWithDB(db)` for main storage (new constructor)
3. `org.NewSQLiteStore(db)` for org storage — runs 003 migration
4. Check for `_orgs.json` — if exists, run one-time data migration
5. `org.NewManager(orgStore, orchestratorManager)` — now takes orchestrator
6. Pass manager to MCP server

## 10. Filesystem Data Migration

### One-Time Migration (migrate_fs.go)

For existing users who have org data in `_orgs.json`:

**`MigrateFromFilesystem(fsPath string, sqlStore *SQLiteStore) error`**

1. Check if `_orgs.json` exists — if not, return nil (no-op)
2. Read and parse JSON file
3. For each org: call `sqlStore.SaveOrg()` and `sqlStore.AddRepos()`
4. Wrap entire migration in a transaction for atomicity
5. On success: rename `_orgs.json` to `_orgs.json.migrated` (preserve, don't delete)
6. On failure: rollback transaction, log error, return error (user retries on next startup)

**Idempotency:** Uses upsert (INSERT OR REPLACE), so re-running is safe.

**Trigger:** Called in `main.go` after both stores are created, before server starts.

## 11. Test Strategy

### Unit Tests: store_test.go

Table-driven tests using `:memory:` SQLite with `_foreign_keys=ON`:

- **SaveOrg:** New org insert, upsert existing (update config), empty repo list, large repo list (100)
- **ListOrgs:** Empty, single org, multiple orgs, correct COUNT without loading repo IDs
- **GetOrg:** Existing org with repos, non-existent (`org.ErrNotFound`), correct repo list
- **AddRepos:** Valid add, duplicate repo (idempotent via INSERT OR IGNORE), non-existent org (FK error)
- **RemoveRepos:** Valid remove, non-existent repo (no-op), remove all repos
- **DeleteOrg:** Existing org, verify junction CASCADE cleanup, non-existent (no-op)
- **Config overrides:** Set, get, merge with org config
- **Concurrent access:** Multiple goroutines doing CRUD simultaneously (WAL mode)
- **Migration:** Verify migration creates tables and indexes

Each test creates a fresh in-memory database and runs migrations.

### Unit Tests: config_test.go

Table-driven tests for `MergeConfigs`:
- Org config only (nil override) → org config returned
- Full override → override values win
- Partial override → merge (repo ExcludePatterns added to org's, non-zero MaxFileSize overrides)
- Both nil → system defaults
- Empty ExcludePatterns lists → no duplicates

### Unit Tests: analyzer_test.go

Mock orchestrator interface for testing concurrency logic:
- All repos succeed → AnalysisResult with Succeeded=Total
- One repo fails, retry succeeds → Succeeded=Total, retry logged
- One repo fails, retry fails → Failed=1, Succeeded=rest, Errors populated
- Concurrency limit: track max concurrent goroutines via atomic counter
- Context cancellation: cancel after 2 repos, verify remaining not launched
- Force=true: verify force flag passed to orchestrator
- Empty org (no repos): Succeeded=0, no error

### Integration Tests: manager_test.go

End-to-end with real SQLite:
- Register → Get → verify roundtrip
- Register → AddRepos → verify added
- Register → RemoveRepos → verify removed
- Register → Delete(detach) → verify org gone, repos remain in context store
- Register → Delete(cascade) → verify org and repo contexts gone
- Config inheritance: register org with config → set repo override → GetEffectiveConfig

### Benchmarks: benchmark_test.go

**Storage benchmarks (real SQLite, `:memory:`):**
- `BenchmarkSaveOrg` — with 1, 10, 50, 100 repos
- `BenchmarkListOrgs` — with 1, 10, 50 orgs
- `BenchmarkGetOrg` — single org lookup with varying repo counts
- `BenchmarkAddRepos` — add 1, 10, 50 repos to existing org
- `BenchmarkDeleteOrg` — with cascading junction cleanup at various scales

**Pipeline benchmarks (mock orchestrator, 10-50ms simulated latency):**
- `BenchmarkAnalyzeOrg_5Repos` — concurrency=3
- `BenchmarkAnalyzeOrg_20Repos` — concurrency=5
- `BenchmarkAnalyzeOrg_50Repos` — concurrency=10
- Note: measures concurrency mechanics, not real analysis latency

All benchmarks use `b.ResetTimer()` after setup and `b.ReportAllocs()`.

## 12. Parallelizable Implementation Sections

These sections can be implemented independently to maximize parallel execution:

**Batch 1 (Independent — run in parallel):**
- Section A: Schema migration (003_org_tables.sql + embed + migrate method)
- Section B: Config inheritance (config.go + config_test.go) — pure functions, no DB dependency

**Batch 2 (Depends on A):**
- Section C: SQLite store + Store interface redesign (store.go + store_test.go)

**Batch 3 (Depends on C):**
- Section D: Analyzer (analyzer.go + analyzer_test.go) — depends on Store for org lookup
- Section E: Filesystem data migration (migrate_fs.go) — depends on SQLite store

**Batch 4 (Depends on C, D):**
- Section F: MCP tools + main.go wiring (server.go, main.go changes)
- Section G: Integration tests + benchmarks (benchmark_test.go, manager_test.go)

**4 batches total, with parallelism in batches 1, 3, and 4.**

## 13. Risk Mitigation

**Risk: Shared database connection**
- Mitigation: SQLite WAL mode already enabled. Per-repo locking via LockManager prevents analysis contention. Org store operations are fast CRUD.

**Risk: Migration breaks existing data**
- Mitigation: New tables only (003_org_tables.sql). No ALTER on existing tables.

**Risk: analyze_org resource exhaustion**
- Mitigation: Bounded concurrency (1-10, default 3). Context cancellation. Per-repo orchestrator timeout.

**Risk: Breaking change in NewManager signature**
- Mitigation: This is an internal package. Update all call sites in main.go and tests. No public API impact.

**Risk: Filesystem migration data loss**
- Mitigation: Rename (not delete) `_orgs.json` after migration. Transaction wraps entire migration for atomicity.
