# Section 05 Code Review: Filesystem Migration

## CRITICAL
- C-1: `Created` timestamp silently lost during migration. `migrate_fs.go` line 46 uses `COALESCE((SELECT created_at FROM orgs WHERE id = ?), CURRENT_TIMESTAMP)` which ignores `o.Created` from the JSON file entirely. The original creation date is permanently discarded and replaced with the current time. `TestMigrateFromFilesystem_SingleOrg` sets `Created: time.Date(2025, 1, 1, ...)` at line 46 of `migrate_fs_test.go` but never asserts the timestamp was preserved in SQLite, masking the bug.

## IMPORTANT
- I-1: Plan deviation -- bypasses `SaveOrg`/`AddRepos`. Plan step 5a/5b says "Call `sqlStore.SaveOrg(ctx, &org)`" and "call `sqlStore.AddRepos(ctx, org.ID, org.Repos)`." Implementation instead reaches into `sqlStore.db` (private field) at `migrate_fs.go` line 32 and writes raw SQL (lines 44-68), duplicating the upsert logic from `store.go` lines 68-92. If `SaveOrg` ever gains an `updated_at` column or changes its conflict resolution, the migration code silently diverges.
- I-2: Commit-then-rename is not atomic. `migrate_fs.go` lines 71-79: transaction commits, then `os.Rename` is attempted. If rename fails (permissions, NFS, disk full), data is committed to SQLite but `_orgs.json` remains, causing a misleading "migration failed" warning on next startup while data was actually migrated successfully.
- I-3: Map key vs `Org.ID` mismatch not validated. `orgData.Orgs` is `map[string]*Org`. A hand-edited `_orgs.json` with key `"org-1"` but inner `ID: ""` or `ID: "different"` silently inserts with the wrong/empty ID. No validation in the loop at `migrate_fs.go` line 38.
- I-4: Test coverage gaps vs plan requirements:
  - (a) `TestMigrateFromFilesystem_SingleOrg` never asserts `Created` timestamp was preserved (see C-1).
  - (b) Plan requires "no partial data in SQLite" for corrupted JSON. `TestMigrateFromFilesystem_CorruptedJSON` (line 214) never queries SQLite to confirm zero orgs exist after failure.
  - (c) No test for true idempotency where `_orgs.json` is re-created between migrations. Current test only verifies second call is a no-op because file is gone.
  - (d) No test for both `_orgs.json` AND `_orgs.json.migrated` existing simultaneously (rename overwrites silently).

## LOW
- L-1: Nil `*Org` pointer in map causes panic. JSON `{"orgs":{"bad":null}}` unmarshals as nil `*Org`. `migrate_fs.go` line 39 `o.Config` would panic. Add nil check.
- L-2: `os.IsNotExist` at `migrate_fs.go` line 20 should use `errors.Is(err, fs.ErrNotExist)` per Go 1.13+ idioms for wrapped error compatibility.
- L-3: Nil map guard missing after unmarshal. Unlike `FilesystemStore.load()` (`store_fs.go` line 44), migration does not guard `d.Orgs == nil`. Safe because ranging over nil map is valid Go, but inconsistent with the codebase.

## COSMETIC
- CO-1: No logging. Migration runs completely silently on success. Should log org count migrated for operational visibility.
- CO-2: Function accepts `*SQLiteStore` instead of `Store` interface. Consequence of bypassing Store methods (I-1).
- CO-3: Repo ordering assertion in `TestMigrateFromFilesystem_SingleOrg` line 67 implicitly depends on `ORDER BY repo_id` from `GetOrg` but does not document the assumption.
