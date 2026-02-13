# Section 07: Integration Tests + Benchmarks

## Overview

End-to-end integration tests that exercise the full org lifecycle through the Manager interface with real SQLite, plus comprehensive benchmarks for storage operations and the analyze_org pipeline.

## Prerequisites

- All previous sections complete (01-06)
- Fully wired org system with SQLite store, analyzer, MCP tools

## What to Build

### 1. Integration Tests (manager_test.go)

Full lifecycle tests using real SQLite (`:memory:` with `_foreign_keys=ON`), real Manager, and mock orchestrator for analysis.

**Test scenarios:**

**Register → Get → Verify roundtrip:**
- Register org with 5 repos and config
- Get org by ID
- Verify all fields match: ID, repos, config, timestamps

**Register → AddRepos → Verify:**
- Register org with 3 repos
- AddRepos with 2 more
- GetOrg → verify 5 repos total
- AddRepos with duplicate → verify still 5 (idempotent)

**Register → RemoveRepos → Verify:**
- Register org with 5 repos
- RemoveRepos 2
- GetOrg → verify 3 remaining
- RemoveRepos non-existent → no error, still 3

**Register → Delete (detach) → Verify:**
- Register org with repos
- Store some repo contexts via mock orchestrator
- Delete org with mode=detach
- Verify org gone (GetOrg returns ErrNotFound)
- Verify repo contexts still exist in mock orchestrator

**Register → Delete (cascade) → Verify:**
- Register org with repos
- Store some repo contexts
- Delete org with mode=cascade
- Verify org gone
- Verify DeleteRepoContext called for each repo

**Config inheritance flow:**
- Register org with config (ExcludePatterns: ["*.log"], MaxFileSize: 1MB)
- SetRepoConfigOverride for one repo (ExcludePatterns: ["*.tmp"], MaxFileSize: 2MB)
- GetEffectiveConfig for repo with override → merged config
- GetEffectiveConfig for repo without override → org config

**analyze_org flow:**
- Register org with 5 repos
- Mock orchestrator: 4 succeed, 1 fails on first try then succeeds on retry
- Call AnalyzeOrg with concurrency=2
- Verify AnalysisResult: Succeeded=5, Failed=0

**List multiple orgs:**
- Register 3 orgs with varying repo counts (2, 5, 10)
- ListOrgs → verify 3 entries with correct counts
- Delete one → ListOrgs → verify 2 entries

### 2. Benchmarks (benchmark_test.go)

All benchmarks use real SQLite `:memory:` DB with migrations applied.

**Storage benchmarks:**

```go
func BenchmarkSaveOrg_1Repo(b *testing.B)
func BenchmarkSaveOrg_10Repos(b *testing.B)
func BenchmarkSaveOrg_50Repos(b *testing.B)
func BenchmarkSaveOrg_100Repos(b *testing.B)
```
Each creates a fresh org with N repos per iteration. Use `b.ResetTimer()` after DB setup.

```go
func BenchmarkListOrgs_1Org(b *testing.B)
func BenchmarkListOrgs_10Orgs(b *testing.B)
func BenchmarkListOrgs_50Orgs(b *testing.B)
```
Setup N orgs with 10 repos each, then benchmark ListOrgs.

```go
func BenchmarkGetOrg_SmallOrg(b *testing.B)   // 5 repos
func BenchmarkGetOrg_LargeOrg(b *testing.B)   // 100 repos
```

```go
func BenchmarkAddRepos_1(b *testing.B)
func BenchmarkAddRepos_10(b *testing.B)
func BenchmarkAddRepos_50(b *testing.B)
```
Setup org, then benchmark adding N repos.

```go
func BenchmarkDeleteOrg_SmallOrg(b *testing.B)   // 5 repos
func BenchmarkDeleteOrg_LargeOrg(b *testing.B)   // 100 repos
```

**Pipeline benchmarks (mock orchestrator with simulated latency):**

```go
func BenchmarkAnalyzeOrg_5Repos_Concurrency3(b *testing.B)
func BenchmarkAnalyzeOrg_20Repos_Concurrency5(b *testing.B)
func BenchmarkAnalyzeOrg_50Repos_Concurrency10(b *testing.B)
```

Mock orchestrator uses `time.Sleep(20 * time.Millisecond)` per repo to simulate analysis. These benchmarks measure concurrency scheduling overhead, not real analysis latency.

**All benchmarks include:**
- `b.ResetTimer()` after setup
- `b.ReportAllocs()` for allocation tracking
- Clean DB state per iteration where needed

### 3. Test Helpers

Create shared test helpers:

```go
// createTestStore creates an in-memory SQLite store with migrations
func createTestStore(t *testing.T) *SQLiteStore

// createTestManager creates a Manager with SQLite store and mock orchestrator
func createTestManager(t *testing.T) (Manager, *mockOrchestrator)

// createBenchStore creates an in-memory SQLite store for benchmarks
func createBenchStore(b *testing.B) *SQLiteStore

// seedOrgs populates store with N orgs of M repos each
func seedOrgs(t *testing.T, store *SQLiteStore, count, reposPerOrg int)
```

## Tests to Write First

The integration tests ARE the tests for this section. Write them directly.

## Files to Create/Modify

| File | Action | Purpose |
|------|--------|---------|
| `internal/org/manager_test.go` | Create | Integration tests through Manager interface |
| `internal/org/benchmark_test.go` | Create | Storage + pipeline benchmarks |
| `internal/org/testhelpers_test.go` | Create | Shared test helpers (createTestStore, etc.) |

## Running Benchmarks

```bash
# Run all benchmarks
go test ./internal/org/... -bench=. -benchmem

# Run storage benchmarks only
go test ./internal/org/... -bench=BenchmarkSave -benchmem
go test ./internal/org/... -bench=BenchmarkList -benchmem

# Run pipeline benchmarks only
go test ./internal/org/... -bench=BenchmarkAnalyze -benchmem

# Run with memory profiling
go test ./internal/org/... -bench=. -benchmem -memprofile=mem.prof
```

## Acceptance Criteria

- [ ] All integration tests pass with real SQLite
- [ ] Full lifecycle tests cover register, add, remove, delete, analyze
- [ ] Config inheritance flow tested end-to-end
- [ ] Both deletion modes (detach, cascade) verified
- [ ] All benchmarks run without error
- [ ] Benchmark output shows ops/sec and allocs for each operation
- [ ] Pipeline benchmarks demonstrate concurrency benefit (50 repos at concurrency=10 faster than concurrency=1)
