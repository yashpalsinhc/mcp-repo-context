# Opus Review

**Model:** claude-opus-4
**Generated:** 2026-02-25T08:45:00Z

---

## Overall Assessment

The plan is well-structured, clearly scoped, and demonstrates good understanding of the existing codebase. The hybrid storage strategy (SQLite persistence + on-the-fly graph computation) is sensible. However, there are several concrete issues worth addressing before implementation.

## Must Fix Before Implementation

1. **Replace `go list std` shell-out with library-based approach** — Shelling out is fragile, unreliable in Docker mode. Use `go/build` from stdlib or maintain a static stdlib set.

2. **`ConfigFile.Structured` uses `interface{}`** — Loses type safety after JSON round-trip. Use `json.RawMessage` with lazy deserialization, or separate typed fields.

3. **Add incremental refresh support** — `refresh_file` on go.mod should re-parse ModuleInfo; config files should update config_files table.

4. **Secret leakage in config file storage** — .env files, docker-compose env vars, CI tokens. Exclude .env, add configurable blocklist.

5. **Edge identifier inconsistency in DependencyGraph** — `From` is "repo_id or module_path" while `To` is only "module_path". Standardize on module_path.

## Should Fix

6. **SQLite migration idempotency** — `ALTER TABLE ADD COLUMN` has no `IF NOT EXISTS` in SQLite. Check `PRAGMA table_info` first.

7. **Replace directive handling in import resolution** — Plan stores ModuleReplace but doesn't mention using it during resolution. Apply replaces before import matching.

8. **Batch queries for graph building** — 200 individual queries slow. Use `SELECT ... WHERE id IN (...)`.

9. **Config file deduplication** — Same file in both `files` table and `config_files`. Pick one source of truth.

10. **Manager interface update** — New methods must be added to Manager interface, not just the struct.

## Nice to Have

11. **Stdlib list optimization** — Store counts only, or omit stdlib from ImportSummary.

12. **PackageType refinement** — Handle repos with both library and CLI (hybrid).

13. **Sentinel error types** — Define `ErrGoModNotFound`, `ErrGoModMalformed` etc.

14. **YAML dependency choice** — Explicitly choose between `gopkg.in/yaml.v3` and `sigs.k8s.io/yaml`.
