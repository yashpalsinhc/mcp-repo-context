# Code Review Interview: Section 03 - Store Interface + SQLite

## Auto-fixes
1. Added `rows.Err()` checks after `rows.Next()` loops in GetOrg and ListOrgs (C-1)
2. Wrapped AddRepos in BeginTx/Commit transaction for atomicity (C-2)
3. Checked `json.Unmarshal` error in ListOrgs instead of ignoring it (C-3)
4. Checked `RowsAffected()` error in SetRepoConfigOverride (M-3)
5. Added table existence check in NewSQLiteStore for fast-fail (M-4)
6. Strengthened concurrent test assertions to exact counts (L-2)
7. Added test for SaveOrg with nil Repos slice (L-4)

## Let go
- SaveOrg uses CURRENT_TIMESTAMP vs o.Created (M-1): practically equivalent, migration use case is hypothetical
- SetRepoConfigOverride plain string error (M-2): no upstream code needs to distinguish yet
- updated_at column unused in app layer (M-5): schema design for future use
- Exported SQLiteStore struct (L-1): internal package, acceptable
- Mutex vs atomic for test counter (L-3): test code, trivial
- GetOrg not transactional (L-5): theoretical read-skew with single-conn pool, defer to section 07
- Inconsistent error wrapping in DeleteOrg (CO-1): cosmetic
- uniqueStrings only used in Register (CO-2): still needed for initial dedup
- Individual tests vs table-driven (CO-3): coverage equivalent, style preference
