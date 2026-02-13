# Code Review Interview: Section 01 - Schema Migration

## Auto-fixes (applied without asking)
1. **CRITICAL #1:** Fix recursive trigger with WHEN guard clause
2. **MEDIUM #3:** Add `PRAGMA foreign_keys = ON` in NewSQLiteStoreWithDB
3. **MEDIUM #4:** Add nil check for db parameter
4. **LOW #6:** Use COALESCE(MAX(version), 0) in version check
5. **LOW #7:** Add trigger behavior test (UPDATE + verify updated_at changed)

## User decisions
6. **MEDIUM #2:** Refactor NewSQLiteStore to delegate → **YES, refactor**
7. **MEDIUM #5:** Remove DB() method → **YES, remove it**

## Let go (not fixing)
8. **LOW #8:** Column type assertions — overkill for this migration
9. **COSMETIC #9:** Embed placement — org code moves to its own package in section 03
