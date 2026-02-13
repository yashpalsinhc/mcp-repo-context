# Opus Review

**Model:** claude-opus-4
**Generated:** 2026-02-13

---

## Critical Issues

### 1. Store Interface Mismatch
The plan in Section 4 says SQLiteStore implements `org.Manager` interface, but existing code separates `Store` from `Manager`. The `Store` interface has `Save`, `Get`, `List`, `Delete` — four methods. The `Manager` has `Register`, `AddRepos`, `RemoveRepos` etc. Plan must clarify whether Store interface is being redesigned or Manager stays as-is on top.

### 2. Database Sharing Strategy is Underspecified
Main.go currently uses `storage.FilesystemStore`, not `SQLiteStore`. The `db` field in SQLiteStore is unexported. Plan assumes SQLite is already being used — it is not. Need to: switch main.go to SQLiteStore, add `DB()` accessor or refactor constructor pattern.

### 3. Migration System Inconsistency
Existing migrations are hard-coded method calls, not auto-discovered from directory. There's no generic migration runner. Plan should specify: embed SQL, add migrate method, call from `migrate()`.

### 4. analyze_org Already Exists Sequentially
`toolAnalyzeOrg` exists in tools.go with sequential loop. Plan doesn't acknowledge replacement. Also, `NewManager` currently takes only `Store` — adding orchestrator dependency is a breaking signature change.

## Medium Issues

### 5. Race Condition in AddRepos/RemoveRepos
Read-modify-write pattern in current manager. SQLite store needs dedicated `AddRepos`/`RemoveRepos` at store level, not generic `Save`.

### 6. Config Override Column Missing from Schema
Section 5 mentions `config_override_json` on `org_repos` but Section 3 migration doesn't include it.

### 7. Delete Cascade: No DeleteContext on Orchestrator
`orchestrator.Manager` has no `DeleteContext` method. Need to add one or give org package direct storage access.

### 8. Retry Logic is Simplistic
"Retry once with exponential backoff" is contradictory. Need: fixed delay, error classification (retryable vs non-retryable).

### 9. No Progress Reporting for Long-Running analyze_org
50 repos at concurrency 3 could take minutes. No streaming progress in MCP request-response model.

### 10. Filesystem Migration is a Footnote
Should be dedicated implementation section with: location, trigger, idempotency, cleanup of `_orgs.json`.

## Minor Issues

### 11. Concurrency validation missing from MCP schema (min/max)
### 12. EffectiveConfig Source map — clarify consumer
### 13. Need `org.ErrNotFound` sentinel error
### 14. Register semantics: existing code upserts, plan says error-on-duplicate — breaking change
### 15. No `updated_at` trigger in SQLite
### 16. Benchmark mock latency doesn't reflect production
### 17. OrgWithCount List optimization — load IDs or just count?

## Required Plan Changes

1. Clarify Store vs Manager interface explicitly
2. Address main.go uses FilesystemStore, detail the switch
3. Fix migration system description to match hard-coded approach
4. State existing toolAnalyzeOrg is being replaced, NewManager signature changes
5. Add config_override_json to migration or defer explicitly
6. Address missing DeleteContext on orchestrator
7. Define org.ErrNotFound sentinel
8. Decide Register: upsert vs error-on-duplicate
9. Specify updated_at trigger or manual update
10. Promote filesystem migration to implementation section
