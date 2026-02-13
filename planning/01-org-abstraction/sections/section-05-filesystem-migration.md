# Section 05: Filesystem Data Migration

## Overview

Implement one-time migration from the existing `_orgs.json` FilesystemStore format to SQLite. This ensures existing users don't lose their org data when upgrading to the new SQLite-backed storage.

## Prerequisites

- Section 01 complete (SQLite tables exist)
- Section 03 complete (SQLiteStore with SaveOrg, AddRepos methods)
- Existing `internal/org/store.go` FilesystemStore reads `_orgs.json`

## What to Build

### 1. Migration Function (migrate_fs.go)

Create `internal/org/migrate_fs.go`:

```go
func MigrateFromFilesystem(fsPath string, sqlStore *SQLiteStore) error
```

**Logic:**
1. Construct path: `filepath.Join(fsPath, "_orgs.json")`
2. Check if file exists — if not, return nil (no-op, nothing to migrate)
3. Read and parse JSON file into `[]Org` (matching existing FilesystemStore format)
4. Begin SQLite transaction
5. For each org:
   a. Call `sqlStore.SaveOrg(ctx, &org)` — uses INSERT OR REPLACE (idempotent)
   b. If org has repos, call `sqlStore.AddRepos(ctx, org.ID, org.Repos)`
6. Commit transaction
7. On success: rename `_orgs.json` to `_orgs.json.migrated` (preserve, don't delete)
8. On failure: rollback transaction, return error with context

**Idempotency:** Uses upsert (INSERT OR REPLACE), so running the migration multiple times is safe. If `_orgs.json.migrated` already exists but `_orgs.json` doesn't, the function is a no-op.

### 2. JSON Format

The existing FilesystemStore stores orgs as:
```json
{
  "org-id-1": {
    "id": "org-id-1",
    "repos": ["repo1", "repo2"],
    "config": {
      "exclude_patterns": ["*.log"],
      "max_file_size": 1048576
    },
    "created": "2025-01-01T00:00:00Z"
  }
}
```

Read this format, iterate over map entries, and insert each into SQLite.

### 3. Integration Point

Called in `main.go` after both stores are created, before server starts:

```go
// After creating org SQLiteStore
if err := org.MigrateFromFilesystem(storagePath, orgSQLiteStore); err != nil {
    log.Printf("WARNING: filesystem org migration failed: %v", err)
    // Non-fatal — user can retry on next startup
}
```

## Tests to Write First

**In migrate_fs_test.go:**

```go
// Test: MigrateFromFilesystem with no _orgs.json returns nil (no-op)
// Test: MigrateFromFilesystem imports single org with correct ID and repos
// Test: MigrateFromFilesystem imports multiple orgs with configs
// Test: MigrateFromFilesystem preserves org config (ExcludePatterns, MaxFileSize)
// Test: MigrateFromFilesystem renames _orgs.json to _orgs.json.migrated
// Test: MigrateFromFilesystem is idempotent — run twice, same result (no duplicate error)
// Test: MigrateFromFilesystem with corrupted JSON returns error, no partial data in SQLite
// Test: MigrateFromFilesystem with empty JSON object ({}) succeeds (zero orgs)
// Test: MigrateFromFilesystem with _orgs.json.migrated already present and no _orgs.json is no-op
```

**Test setup:**
- Create temp directory with `_orgs.json` fixture
- Create in-memory SQLite store
- Run migration
- Verify data via store.GetOrg()
- Verify file renamed

## Files to Create/Modify

| File | Action | Purpose |
|------|--------|---------|
| `internal/org/migrate_fs.go` | Create | MigrateFromFilesystem function |
| `internal/org/migrate_fs_test.go` | Create | Migration tests with fixture JSON files |

## Acceptance Criteria

- [ ] No-op when `_orgs.json` doesn't exist
- [ ] All orgs imported with correct repos and config
- [ ] `_orgs.json` renamed to `.migrated` after success
- [ ] Transaction ensures atomicity — no partial data on failure
- [ ] Idempotent — safe to run multiple times
- [ ] All tests pass
