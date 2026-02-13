# Section 03 Code Review: Store Interface Redesign + SQLite Implementation

## CRITICAL Issues

### C-1: Missing `rows.Err()` check after iteration in `GetOrg` and `ListOrgs`
**File:** store.go, Lines 128-136 (GetOrg), 152-176 (ListOrgs)
Both iterate `*sql.Rows` using `rows.Next()` but never call `rows.Err()`. Per `database/sql` docs, `rows.Next()` can return false due to error, only retrievable via `rows.Err()`.

### C-2: `AddRepos` is not atomic — TOCTOU race between existence check and inserts
**File:** store.go, Lines 186-212
Plan specifies "AddRepos — atomic insert" but implementation does SELECT + N INSERTs outside a transaction.

### C-3: `ListOrgs` silently swallows JSON unmarshal errors
**File:** store.go, Line 163
`json.Unmarshal` error ignored. Compare with `GetOrg` which correctly checks it.

## MEDIUM Issues

### M-1: `SaveOrg` uses `CURRENT_TIMESTAMP` instead of `o.Created` from Go struct
**File:** store.go, Lines 64-68

### M-2: `SetRepoConfigOverride` error for non-existent repo is plain string, not `ErrNotFound`
**File:** store.go, Lines 278-279

### M-3: `RowsAffected()` error silently discarded in `SetRepoConfigOverride`
**File:** store.go, Line 277

### M-4: `NewSQLiteStore` no longer calls `RunMigrations()` during construction as specified in plan
**File:** store.go, Lines 38-43

### M-5: `SaveOrg` does not update `updated_at` column — column is unused in application layer
**File:** store.go, Lines 64-68

## LOW Issues

### L-1: `SQLiteStore` struct exported — leaky abstraction (mitigated by internal/ package)
### L-2: Concurrent test assertions too weak — should assert exact counts
### L-3: `testDBCounter` should use `atomic.AddUint64` instead of mutex
### L-4: No test for `SaveOrg` with nil Repos slice
### L-5: `GetOrg` two separate queries not in a transaction — read-skew risk

## COSMETIC Issues

### CO-1: Inconsistent error wrapping — `DeleteOrg` returns raw error
### CO-2: `uniqueStrings` in manager.go only used in `Register` now
### CO-3: Plan specified table-driven tests but tests are individual functions
