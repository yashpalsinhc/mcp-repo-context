# Section 04: Analyzer (analyze_org Orchestration)

## Overview

Implement `AnalyzeOrg` with semaphore-based concurrency, single-retry logic, and context cancellation in `internal/org/analyzer.go`. This replaces the existing sequential `toolAnalyzeOrg` in `internal/mcp/tools.go`.

## Prerequisites

- Section 02 complete (config inheritance — MergeConfigs)
- Section 03 complete (SQLite store — GetOrg, GetEffectiveConfig)
- Existing `internal/orchestrator/interface.go`: Manager interface with AnalyzeRepo, AnalyzeLocal

## What Was Built

### 1. Types (types.go additions)

Added to `internal/org/types.go`:

```go
type AnalysisResult struct {
    OrgID     string        `json:"org_id"`
    Total     int           `json:"total"`
    Succeeded int           `json:"succeeded"`
    Failed    int           `json:"failed"`
    Skipped   int           `json:"skipped"`
    Errors    []RepoError   `json:"errors,omitempty"`
    Duration  time.Duration `json:"duration"`
}

type RepoError struct {
    RepoID string `json:"repo_id"`
    Error  string `json:"error"`
}
```

### 2. Analyzer (analyzer.go)

Created `internal/org/analyzer.go` with:

- **Analyzer struct** with `orgManager Manager` and `orch orchestrator.Manager`
- **AnalyzeOrg**: Semaphore-based bounded concurrency (clamped 1-10, default 3). Uses WaitGroup + Mutex for safe result aggregation. Context cancellation checked via `ctx.Err() != nil` before launching goroutines; goroutines also bail on semaphore acquisition if context is cancelled. Falls through to `wg.Wait()` on cancellation to properly drain in-flight goroutines.
- **analyzeRepo**: Single retry with 1s wait using `time.NewTimer` (stoppable on cancel). Non-retryable errors (containing "not found", "no such file", "invalid") skip retry.
- **callOrchestrator**: Routes `local:` prefix repos to `AnalyzeLocal`, others to `AnalyzeRepo`. Uses `strings.CutPrefix` for clean prefix handling.

**Deviations from plan:**
- Step 6b "Get effective config" not implemented — analyzer delegates config handling to orchestrator, which has its own config management
- Skipped field not populated — deferred to section 06 when staleness detection is available
- Used `ctx.Err() != nil` check instead of `select`/`break` pattern for cleaner cancellation detection (SA4011 lint compliance)

### 3. Manager Signature Change

Changed `NewManager` in `internal/org/manager.go`:
- **Before:** `func NewManager(store Store) Manager`
- **After:** `func NewManager(store Store, orch orchestrator.Manager) Manager`

Manager creates Analyzer internally and exposes `AnalyzeOrg` via the Manager interface.

### 4. DeleteRepoContext on Orchestrator

Added to `internal/orchestrator/interface.go`:
```go
DeleteRepoContext(ctx context.Context, repoID string) error
```

Implementation in `internal/orchestrator/manager.go` delegates to `m.store.DeleteContext(ctx, repoID)`.

### 5. Mock Fixes in Other Packages

Added `DeleteRepoContext` stubs to mock implementations in:
- `internal/graph/visualizer_test.go`
- `internal/mcp/tools_test.go`
- `internal/mcp/resources_test.go`

Fixed `cmd/mcp-server/main.go` to pass orchestrator manager as second argument to `org.NewManager`.

## Tests Written

12 tests in `internal/org/analyzer_test.go` with `mockOrch` implementing `orchestrator.Manager`:

| Test | What it verifies |
|------|-----------------|
| AllSucceed | All repos succeed, Succeeded == Total |
| RetrySucceeds | Fail first, succeed on retry |
| RetryFails | Fail both attempts, error recorded |
| ConcurrencyLimit | Max concurrent never exceeds limit |
| ContextCancellation | Stops launching, in-flight drain |
| ForceFlag | Force flag passed to orchestrator |
| EmptyOrg | Zero repos, no error |
| NonExistentOrg | Returns error |
| RoutesLocalPrefix | local: prefix → AnalyzeLocal |
| Duration | Duration is populated and non-zero |
| ClampsConcurrency | 0→3, 15→10 |
| SkipsRetryForNonRetryable | "not found" error not retried |

## Files Created/Modified

| File | Action | Purpose |
|------|--------|---------|
| `internal/org/types.go` | Modified | Added AnalysisResult, RepoError types |
| `internal/org/analyzer.go` | Created | Analyzer with AnalyzeOrg, retry, concurrency |
| `internal/org/manager.go` | Modified | NewManager(Store, orchestrator.Manager), AnalyzeOrg interface |
| `internal/org/analyzer_test.go` | Created | 12 tests with mockOrch |
| `internal/orchestrator/interface.go` | Modified | Added DeleteRepoContext method |
| `internal/orchestrator/manager.go` | Modified | Implemented DeleteRepoContext |
| `internal/graph/visualizer_test.go` | Modified | Added DeleteRepoContext mock stub |
| `internal/mcp/tools_test.go` | Modified | Added DeleteRepoContext mock stub |
| `internal/mcp/resources_test.go` | Modified | Added DeleteRepoContext mock stub |
| `cmd/mcp-server/main.go` | Modified | Pass orchestrator manager to org.NewManager |

## Code Review Findings

See `implementation/code_review/section-04-review.md` and `section-04-interview.md`.

**Fixed:** C-1 (race on early return), M-4 (timer leak), CO-1 (unused param), SA4011 (ineffective break)
**Accepted:** C-2 (Total invariant during cancel), M-1/M-2 (deferred features), M-3 (string matching), L-1/L-2/L-3 (test adequacy), CO-2 (logging)

## Acceptance Criteria

- [x] AnalyzeOrg processes all repos with bounded concurrency
- [x] Retry logic works (fail → 1s wait → retry → succeed or record error)
- [x] Non-retryable errors skip retry
- [x] Context cancellation stops new goroutine launches
- [x] Concurrency clamped to 1-10 range
- [x] AnalysisResult correctly populated (Succeeded, Failed, Errors, Duration)
- [x] NewManager accepts orchestrator.Manager
- [x] DeleteRepoContext added to orchestrator interface
- [x] All analyzer tests pass with mock orchestrator
- [x] All other package tests still pass after interface changes
