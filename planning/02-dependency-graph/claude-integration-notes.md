# Integration Notes: Opus Review Feedback

## Integrating

### 1. Replace `go list std` with library approach
**Integrating.** Using `go/build` from stdlib to enumerate stdlib packages is more reliable and doesn't require Go CLI. Will update Section 6.

### 2. Fix `ConfigFile.Structured` type safety
**Integrating.** Will use `json.RawMessage` with a `Type` discriminator and helper methods for typed access. Will update Section 3.

### 3. Add incremental refresh support
**Integrating.** Will add a new section covering refresh_file and refresh_changed handling for go.mod and config files.

### 4. Secret leakage protection
**Integrating.** Will add a blocklist (`.env`, files with "secret"/"credential" in name) and only store structured output for sensitive types.

### 5. Standardize DependencyGraph edge identifiers
**Integrating.** Will use ModulePath consistently for both From and To fields.

### 6. SQLite migration idempotency
**Integrating.** Will add PRAGMA table_info check before ALTER TABLE.

### 7. Replace directive handling in import resolution
**Integrating.** Will add explicit step to apply replace directives before import resolution.

### 8. Batch queries for graph building
**Integrating.** Will use batch SELECT query.

### 9. Config file deduplication
**Integrating.** Will extend existing `files` table with optional content/structured columns rather than creating a separate table.

### 10. Manager interface update
**Integrating.** Will explicitly call out interface changes.

## Not Integrating

### 11. Stdlib list in ImportSummary (nice to have)
**Not integrating.** Keeping stdlib list — it's useful for understanding a repo's stdlib footprint and costs negligible storage.

### 12. PackageType hybrid detection
**Not integrating now.** The simple heuristic covers 95% of cases. Can refine later if needed.

### 13. Sentinel error types
**Partially integrating.** Will add key error types but not all. This is a nice practice but not critical for the plan.

### 14. YAML dependency choice
**Not integrating.** Will use `gopkg.in/yaml.v3` as it's the most widely used. Not worth a plan change.
