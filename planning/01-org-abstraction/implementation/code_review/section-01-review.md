# Code Review: Section 01 - Schema Migration

## Issues Found

### 1. CRITICAL: Infinite recursive trigger (Data corruption risk)
The `update_orgs_timestamp` trigger fires `AFTER UPDATE ON orgs` and then itself does `UPDATE orgs SET updated_at = ...`. This can cause infinite recursion. Needs a `WHEN` guard clause.

### 2. MEDIUM: `NewSQLiteStore` does not delegate to `NewSQLiteStoreWithDB`
Two separate migration call paths that could diverge. `NewSQLiteStoreWithDB` also loses the `path` field.

### 3. MEDIUM: `NewSQLiteStoreWithDB` does not set `_foreign_keys=ON`
No guarantee the caller's `*sql.DB` has foreign keys enabled. Should defensively execute `PRAGMA foreign_keys = ON`.

### 4. MEDIUM: `NewSQLiteStoreWithDB` does not validate nil db
Passing nil `*sql.DB` will cause nil pointer dereference.

### 5. MEDIUM: `DB()` method exposes internal state without ownership semantics
Creates ambiguity about connection lifecycle. `Close()` on one store will close the connection for both.

### 6. LOW: `migrateOrgTables` version check fragile with empty table
`MAX(version)` returns NULL if table is empty. Use `COALESCE(MAX(version), 0)` for robustness.

### 7. LOW: Test for trigger only checks existence, not behavior
Does not perform an UPDATE and verify `updated_at` changed.

### 8. LOW: Missing test for column types and constraints

### 9. COSMETIC: Inconsistent embed placement
003 embed is in sqlite.go instead of a dedicated file like file_tracker.go.
