# TDD Plan: Organization Abstraction Layer (Split 01)

Mirrors the structure of `claude-plan.md`. For each section, defines tests to write BEFORE implementation.

**Testing approach:** Go standard `testing` package. Table-driven tests. `:memory:` SQLite with `_foreign_keys=ON`. Tests co-located with implementation (`*_test.go`).

---

## 3. Store Interface Redesign

### Tests for Store interface contract (store_test.go)

```go
// Test: Store interface is satisfied by SQLiteStore (compile-time check)
// var _ Store = (*SQLiteStore)(nil)
```

### Tests for ErrNotFound sentinel (types_test.go)

```go
// Test: ErrNotFound wraps correctly with errors.Is
// Test: ErrNotFound message matches "org: not found"
```

---

## 4. Schema Migration

### Tests for migration (store_test.go)

```go
// Test: RunMigrations creates orgs table with correct columns
// Test: RunMigrations creates org_repos table with correct columns and FK
// Test: RunMigrations creates repo_id index on org_repos
// Test: RunMigrations creates updated_at trigger
// Test: RunMigrations is idempotent (run twice, no error)
// Test: RunMigrations records version in schema_migrations
// Test: CASCADE delete on org removes junction entries
```

---

## 5. SQLite Org Store Implementation

### Tests for SaveOrg (store_test.go)

```go
// Test: SaveOrg inserts new org with config
// Test: SaveOrg upserts existing org (updates config, preserves created_at)
// Test: SaveOrg with empty repo list succeeds
// Test: SaveOrg with 100 repos succeeds
// Test: SaveOrg stores config_json correctly (roundtrip)
```

### Tests for ListOrgs (store_test.go)

```go
// Test: ListOrgs returns empty slice when no orgs
// Test: ListOrgs returns single org with correct repo count
// Test: ListOrgs returns multiple orgs sorted by id
// Test: ListOrgs COUNT matches actual repo count (not loading IDs)
// Test: ListOrgs with org that has zero repos shows count=0
```

### Tests for GetOrg (store_test.go)

```go
// Test: GetOrg returns org with full repo ID list
// Test: GetOrg returns ErrNotFound for non-existent org
// Test: GetOrg config deserialization matches saved config
// Test: GetOrg repo list order is deterministic
```

### Tests for AddRepos (store_test.go)

```go
// Test: AddRepos adds new repos to existing org
// Test: AddRepos with duplicate repo is idempotent (INSERT OR IGNORE)
// Test: AddRepos to non-existent org returns FK error
// Test: AddRepos with empty list is no-op
// Test: AddRepos multiple times accumulates repos
```

### Tests for RemoveRepos (store_test.go)

```go
// Test: RemoveRepos removes specified repos
// Test: RemoveRepos with non-existent repo is no-op
// Test: RemoveRepos all repos leaves org with zero repos
// Test: RemoveRepos from non-existent org is no-op
```

### Tests for DeleteOrg (store_test.go)

```go
// Test: DeleteOrg removes org and junction entries (CASCADE)
// Test: DeleteOrg for non-existent org is no-op (no error)
// Test: DeleteOrg does not affect other orgs
// Test: After DeleteOrg, GetOrg returns ErrNotFound
```

### Tests for config overrides (store_test.go)

```go
// Test: SetRepoConfigOverride stores config for specific repo in org
// Test: GetRepoConfigOverride returns stored config
// Test: GetRepoConfigOverride returns nil for repo without override
// Test: GetRepoConfigOverride returns error for non-existent org-repo pair
// Test: Config override roundtrip preserves ExcludePatterns and MaxFileSize
```

### Tests for concurrent access (store_test.go)

```go
// Test: Concurrent SaveOrg from multiple goroutines does not corrupt data
// Test: Concurrent AddRepos and RemoveRepos on same org is safe
// Test: Concurrent reads (GetOrg, ListOrgs) during writes succeed (WAL mode)
```

---

## 6. Config Inheritance

### Tests for MergeConfigs (config_test.go)

```go
// Test: MergeConfigs with nil override returns org config unchanged
// Test: MergeConfigs with nil org config returns override
// Test: MergeConfigs with both nil returns nil (system defaults apply)
// Test: MergeConfigs ExcludePatterns are unioned (deduplicated)
// Test: MergeConfigs MaxFileSize override wins when non-zero
// Test: MergeConfigs MaxFileSize org value used when override is zero
// Test: MergeConfigs does not mutate input configs (returns new)
// Test: MergeConfigs with empty ExcludePatterns on both returns empty slice
// Test: MergeConfigs with overlapping ExcludePatterns deduplicates
```

### Tests for GetEffectiveConfig (manager_test.go or config_test.go)

```go
// Test: GetEffectiveConfig with no override returns org config
// Test: GetEffectiveConfig with override returns merged config
// Test: GetEffectiveConfig for non-existent org returns error
// Test: GetEffectiveConfig for repo not in org returns error
```

---

## 7. analyze_org Orchestration

### Tests for AnalyzeOrg (analyzer_test.go)

```go
// Test: AnalyzeOrg with all repos succeeding returns Succeeded=Total
// Test: AnalyzeOrg with one repo failing and retry succeeding returns Succeeded=Total
// Test: AnalyzeOrg with one repo failing after retry returns Failed=1, errors populated
// Test: AnalyzeOrg respects concurrency limit (max N goroutines via atomic counter)
// Test: AnalyzeOrg with context cancellation stops launching new repos
// Test: AnalyzeOrg with force=true passes force to orchestrator
// Test: AnalyzeOrg with empty org (no repos) returns Succeeded=0, no error
// Test: AnalyzeOrg with non-existent org returns error
// Test: AnalyzeOrg correctly routes local: prefixed repos to AnalyzeLocal
// Test: AnalyzeOrg correctly routes non-local repos to AnalyzeRepo
// Test: AnalyzeOrg Duration is populated (non-zero)
// Test: AnalyzeOrg clamps concurrency to range 1-10
```

### Mock orchestrator interface

```go
// MockOrchestrator: implements orchestrator.Manager with configurable behavior
//   - AnalyzeRepo: configurable success/failure per repo
//   - AnalyzeLocal: configurable success/failure per path
//   - Track call count and concurrency (atomic counter for max concurrent)
//   - Configurable latency (time.Sleep)
```

---

## 8. MCP Tool Definitions

### Tests for tool handlers (tools_test.go or server_test.go)

```go
// Test: toolAnalyzeOrg dispatches to org.Manager.AnalyzeOrg with correct params
// Test: toolAnalyzeOrg with missing org_id returns error
// Test: toolGetOrg returns full org details as text content
// Test: toolGetOrg with non-existent org returns error text
// Test: toolDeleteOrg with mode=detach removes org, repos remain
// Test: toolDeleteOrg with mode=cascade removes org and repo contexts
// Test: toolDeleteOrg with default mode uses detach
// Test: toolUpdateOrgConfig updates config and returns updated org
// Test: toolAddReposToOrg adds repos and returns updated org
// Test: toolRemoveReposFromOrg removes repos and returns updated org
// Test: toolRegisterOrg uses upsert behavior (backward compatible)
// Test: toolListOrgs returns formatted list with repo counts
```

---

## 9. main.go Wiring Changes

### Tests for wiring (integration-level)

```go
// Test: NewSQLiteStoreWithDB accepts pre-opened *sql.DB (storage package)
// Test: org.NewSQLiteStore and storage.NewSQLiteStoreWithDB share same DB without locking issues
// Test: NewManager accepts Store and orchestrator.Manager (signature check)
```

---

## 10. Filesystem Data Migration

### Tests for MigrateFromFilesystem (migrate_fs_test.go)

```go
// Test: MigrateFromFilesystem with no _orgs.json returns nil (no-op)
// Test: MigrateFromFilesystem imports all orgs with correct repos
// Test: MigrateFromFilesystem imports org configs correctly
// Test: MigrateFromFilesystem renames _orgs.json to _orgs.json.migrated
// Test: MigrateFromFilesystem is idempotent (run twice, same result)
// Test: MigrateFromFilesystem with corrupted JSON returns error, no partial data
// Test: MigrateFromFilesystem with empty _orgs.json succeeds (empty list)
```

---

## 11. Benchmarks (benchmark_test.go)

```go
// Benchmark: BenchmarkSaveOrg_1Repo, BenchmarkSaveOrg_10Repos, BenchmarkSaveOrg_50Repos, BenchmarkSaveOrg_100Repos
// Benchmark: BenchmarkListOrgs_1Org, BenchmarkListOrgs_10Orgs, BenchmarkListOrgs_50Orgs
// Benchmark: BenchmarkGetOrg_SmallOrg, BenchmarkGetOrg_LargeOrg (100 repos)
// Benchmark: BenchmarkAddRepos_1, BenchmarkAddRepos_10, BenchmarkAddRepos_50
// Benchmark: BenchmarkDeleteOrg_SmallOrg, BenchmarkDeleteOrg_LargeOrg
// Benchmark: BenchmarkAnalyzeOrg_5Repos_Concurrency3
// Benchmark: BenchmarkAnalyzeOrg_20Repos_Concurrency5
// Benchmark: BenchmarkAnalyzeOrg_50Repos_Concurrency10
// All use b.ResetTimer() after setup, b.ReportAllocs()
```
