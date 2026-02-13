# Section 01: Schema Migration

## Overview

Create SQLite migration `003_org_tables.sql` to add organization tables to the existing database. This migration adds the `orgs` table, `org_repos` junction table, and an `updated_at` trigger. Follow the existing hard-coded migration pattern used by `internal/storage/sqlite.go`.

## Prerequisites

- Familiarity with existing migration files: `001_initial_schema.sql`, `002_file_hashes.sql`
- Existing `internal/storage/sqlite.go` migration pattern (go:embed + method call chain)

## What to Build

### 1. Migration SQL File

Create `internal/storage/migrations/003_org_tables.sql` (or embed location matching existing pattern):

**`orgs` table:**
- `id` TEXT PRIMARY KEY — organization identifier (e.g., `github.com/LambdatestIncPrivate`)
- `config_json` TEXT — JSON-serialized OrgConfig (ExcludePatterns, MaxFileSize)
- `created_at` DATETIME DEFAULT CURRENT_TIMESTAMP
- `updated_at` DATETIME DEFAULT CURRENT_TIMESTAMP

**`org_repos` junction table:**
- `org_id` TEXT NOT NULL — FK referencing orgs(id) ON DELETE CASCADE
- `repo_id` TEXT NOT NULL — repo identifier string
- `config_override_json` TEXT — nullable, per-repo config override (JSON)
- `added_at` DATETIME DEFAULT CURRENT_TIMESTAMP
- PRIMARY KEY (org_id, repo_id)
- CREATE INDEX idx_org_repos_repo_id ON org_repos(repo_id) — for reverse lookups

**SQLite trigger for automatic `updated_at`:**
```sql
CREATE TRIGGER IF NOT EXISTS update_orgs_timestamp
AFTER UPDATE ON orgs
BEGIN
    UPDATE orgs SET updated_at = CURRENT_TIMESTAMP WHERE id = NEW.id;
END;
```

**Schema migration tracking:**
- INSERT INTO schema_migrations (version) to mark migration as applied
- Check existence before running (idempotent)

### 2. Embed and Wire Migration

In `internal/storage/sqlite.go` (or the appropriate file):

1. Add `//go:embed` directive for the new SQL file
2. Create `migrateOrgTables()` method on SQLiteStore
3. Add call to `migrateOrgTables()` in the `migrate()` method chain, after existing migrations
4. Check `schema_migrations` table — only run if version 3 not present

### 3. Storage Package Constructor Change

Add a new constructor to `internal/storage/` that accepts a pre-opened `*sql.DB`:

```go
func NewSQLiteStoreWithDB(db *sql.DB) (*SQLiteStore, error)
```

This enables sharing the same database connection between the main storage and the org storage. The existing `NewSQLiteStore(path string)` can delegate to this new constructor after opening the DB.

## Tests to Write First

**In store_test.go or a dedicated migration_test.go:**

```go
// Test: RunMigrations creates orgs table with correct columns (id, config_json, created_at, updated_at)
// Test: RunMigrations creates org_repos table with correct columns (org_id, repo_id, config_override_json, added_at)
// Test: RunMigrations creates repo_id index on org_repos
// Test: RunMigrations creates updated_at trigger on orgs
// Test: RunMigrations is idempotent — running twice produces no error
// Test: RunMigrations records version 3 in schema_migrations
// Test: CASCADE delete — deleting from orgs removes corresponding org_repos rows
// Test: Foreign key constraint — inserting into org_repos with non-existent org_id fails
```

**Test setup pattern:**
- Use `sql.Open("sqlite3", ":memory:?_foreign_keys=ON")`
- Run all migrations (001, 002, 003) to get a complete schema
- Each test gets a fresh in-memory database

## Files to Create/Modify

| File | Action | Purpose |
|------|--------|---------|
| `internal/storage/migrations/003_org_tables.sql` | Create | SQL migration |
| `internal/storage/sqlite.go` | Modify | Add embed, migrateOrgTables(), NewSQLiteStoreWithDB() |
| `internal/storage/sqlite_test.go` | Modify | Add migration tests |

## Acceptance Criteria

- [ ] `003_org_tables.sql` creates both tables with correct schema
- [ ] Migration is idempotent (safe to run multiple times)
- [ ] CASCADE delete works on org_repos when org is deleted
- [ ] `updated_at` trigger fires on org UPDATE
- [ ] `NewSQLiteStoreWithDB(db)` constructor works with pre-opened *sql.DB
- [ ] All migration tests pass with `:memory:` SQLite
