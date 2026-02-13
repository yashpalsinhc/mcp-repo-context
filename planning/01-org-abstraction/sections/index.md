<!-- PROJECT_CONFIG
runtime: go
test_command: go test ./internal/org/... -v
END_PROJECT_CONFIG -->

<!-- SECTION_MANIFEST
section-01-schema-migration
section-02-config-inheritance
section-03-store-interface-sqlite
section-04-analyzer
section-05-filesystem-migration
section-06-mcp-tools-wiring
section-07-integration-tests-benchmarks
END_MANIFEST -->

# Implementation Sections Index

## Dependency Graph

| Section | Depends On | Blocks | Parallelizable With |
|---------|------------|--------|---------------------|
| section-01-schema-migration | - | 03, 05 | section-02 |
| section-02-config-inheritance | - | 03, 04 | section-01 |
| section-03-store-interface-sqlite | 01 | 04, 05, 06, 07 | - |
| section-04-analyzer | 02, 03 | 06, 07 | section-05 |
| section-05-filesystem-migration | 01, 03 | 06 | section-04 |
| section-06-mcp-tools-wiring | 03, 04, 05 | 07 | - |
| section-07-integration-tests-benchmarks | 03, 04, 06 | - | - |

## Execution Order (Batched for Parallelism)

1. **Batch 1:** section-01-schema-migration, section-02-config-inheritance (parallel, no dependencies)
2. **Batch 2:** section-03-store-interface-sqlite (depends on 01)
3. **Batch 3:** section-04-analyzer, section-05-filesystem-migration (parallel, both depend on 03)
4. **Batch 4:** section-06-mcp-tools-wiring (depends on 03, 04, 05)
5. **Batch 5:** section-07-integration-tests-benchmarks (depends on everything)

## Section Summaries

### section-01-schema-migration
Create `003_org_tables.sql` migration with orgs table, org_repos junction table, config_override_json column, updated_at trigger, and repo_id index. Embed via go:embed. Add migrateOrgTables() method. Test migration creates tables correctly.

**Plan sections:** 4 (Schema Migration)
**TDD sections:** 4 (Schema Migration tests)

### section-02-config-inheritance
Implement pure `MergeConfigs()` function in config.go. Handles ExcludePatterns union and MaxFileSize override. No database dependency — pure function with table-driven tests.

**Plan sections:** 6 (Config Inheritance)
**TDD sections:** 6 (Config Inheritance tests)

### section-03-store-interface-sqlite
Redesign Store interface for atomic SQLite operations. Implement SQLiteStore with SaveOrg (upsert), GetOrg, ListOrgs, DeleteOrg, AddRepos (INSERT OR IGNORE), RemoveRepos, config override operations. Define org.ErrNotFound sentinel. Add NewSQLiteStoreWithDB constructor to storage package. Comprehensive table-driven unit tests.

**Plan sections:** 3 (Store Interface Redesign), 5 (SQLite Org Store Implementation)
**TDD sections:** 3, 5 (Store tests)

### section-04-analyzer
Implement AnalyzeOrg with semaphore-based concurrency, retry logic (once after 1s, skip non-retryable), context cancellation. Change NewManager signature to accept orchestrator.Manager. Add DeleteRepoContext to orchestrator.Manager interface. Mock orchestrator for unit tests.

**Plan sections:** 7 (analyze_org Orchestration)
**TDD sections:** 7 (Analyzer tests)

### section-05-filesystem-migration
One-time migration from _orgs.json to SQLite. Read JSON, upsert orgs, transaction-wrapped. Rename file on success. Idempotent. Test with fixture JSON files.

**Plan sections:** 10 (Filesystem Data Migration)
**TDD sections:** 10 (Filesystem migration tests)

### section-06-mcp-tools-wiring
Register new MCP tools (analyze_org, get_org, delete_org, update_org_config, add_repos_to_org, remove_repos_from_org). Replace existing toolAnalyzeOrg body. Update main.go wiring: open shared *sql.DB, create both stores, run filesystem migration, change NewManager call.

**Plan sections:** 8 (MCP Tool Definitions), 9 (main.go Wiring Changes)
**TDD sections:** 8 (Tool handler tests), 9 (Wiring tests)

### section-07-integration-tests-benchmarks
End-to-end integration tests through Manager interface with real SQLite. Full lifecycle tests (register → analyze → delete). Config inheritance flow. Storage and pipeline benchmarks.

**Plan sections:** 11 (Test Strategy - Integration Tests, Benchmarks)
**TDD sections:** 11 (Integration tests, Benchmarks)
