# Section 01: Schema Migration

## Overview

Create SQLite migration `003_org_tables.sql` to add organization tables to the existing database. This migration adds the `orgs` table, `org_repos` junction table, and an `updated_at` trigger. Follow the existing hard-coded migration pattern used by `internal/storage/sqlite.go`.

## Prerequisites

- Familiarity with existing migration files: `001_initial_schema.sql`, `002_file_hashes.sql`
- Existing `internal/storage/sqlite.go` migration pattern (go:embed + method call chain)

## What Was Built

### 1. Migration SQL File

Created `internal/storage/migrations/003_org_tables.sql`:

**`orgs` table:** id (TEXT PK), config_json, created_at, updated_at
**`org_repos` junction table:** org_id (FK CASCADE), repo_id, config_override_json, added_at, PK(org_id, repo_id)
**Index:** idx_org_repos_repo_id for reverse lookups
**Trigger:** `update_orgs_timestamp` with `WHEN NEW.updated_at = OLD.updated_at` guard to prevent infinite recursion

### 2. Embed and Wire Migration

In `internal/storage/sqlite.go`:
- Added `//go:embed migrations/003_org_tables.sql` → `orgTablesMigration`
- Created `migrateOrgTables()` with `COALESCE(MAX(version), 0)` version check
- Added to `migrate()` chain after `migrateFileHashes()`

### 3. Storage Package Constructor Change

Added `NewSQLiteStoreWithDB(db *sql.DB) (*SQLiteStore, error)`:
- Validates db is non-nil
- Defensively runs `PRAGMA foreign_keys = ON`
- Runs all migrations
- `NewSQLiteStore(path)` now delegates to `NewSQLiteStoreWithDB` after opening DB

### Deviations from Original Plan

1. **Trigger guard clause:** Added `WHEN NEW.updated_at = OLD.updated_at` to prevent infinite recursion (code review finding)
2. **PRAGMA foreign_keys:** `NewSQLiteStoreWithDB` defensively enables foreign keys regardless of DSN
3. **Nil check:** Added nil check for db parameter
4. **COALESCE:** Used `COALESCE(MAX(version), 0)` instead of bare `MAX(version)` for robustness
5. **Delegation:** `NewSQLiteStore` delegates to `NewSQLiteStoreWithDB` (single migration path)
6. **DB() method removed:** Was not in plan, created ownership ambiguity
7. **Tests in `org_migration_test.go`:** Used dedicated file instead of modifying `sqlite_test.go`

## Tests (10 total)

| Test | Status |
|------|--------|
| TestOrgMigration_CreatesOrgsTable | PASS |
| TestOrgMigration_CreatesOrgReposTable | PASS |
| TestOrgMigration_CreatesRepoIdIndex | PASS |
| TestOrgMigration_CreatesUpdatedAtTrigger (existence + behavior) | PASS |
| TestNewSQLiteStoreWithDB_NilDB | PASS |
| TestOrgMigration_IsIdempotent | PASS |
| TestOrgMigration_RecordsVersion3 | PASS |
| TestOrgMigration_CascadeDelete | PASS |
| TestOrgMigration_ForeignKeyConstraint | PASS |
| TestNewSQLiteStoreWithDB | PASS |

## Files Created/Modified

| File | Action | Purpose |
|------|--------|---------|
| `internal/storage/migrations/003_org_tables.sql` | Created | SQL migration |
| `internal/storage/sqlite.go` | Modified | Add embed, migrateOrgTables(), NewSQLiteStoreWithDB(), refactored NewSQLiteStore |
| `internal/storage/org_migration_test.go` | Created | Migration tests |

## Acceptance Criteria

- [x] `003_org_tables.sql` creates both tables with correct schema
- [x] Migration is idempotent (safe to run multiple times)
- [x] CASCADE delete works on org_repos when org is deleted
- [x] `updated_at` trigger fires on org UPDATE (with recursion guard)
- [x] `NewSQLiteStoreWithDB(db)` constructor works with pre-opened *sql.DB
- [x] All migration tests pass with `:memory:` SQLite
