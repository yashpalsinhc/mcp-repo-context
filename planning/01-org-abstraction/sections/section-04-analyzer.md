# Section 04: Analyzer (analyze_org Orchestration)

## Overview

Implement `AnalyzeOrg` with semaphore-based concurrency, single-retry logic, and context cancellation in `internal/org/analyzer.go`. This replaces the existing sequential `toolAnalyzeOrg` in `internal/mcp/tools.go`.

## Prerequisites

- Section 02 complete (config inheritance — MergeConfigs)
- Section 03 complete (SQLite store — GetOrg, GetEffectiveConfig)
- Existing `internal/orchestrator/interface.go`: Manager interface with AnalyzeRepo, AnalyzeLocal

## What to Build

### 1. Types (types.go additions)

Add to `internal/org/types.go`:

```go
type AnalysisResult struct {
    OrgID     string
    Total     int
    Succeeded int
    Failed    int
    Skipped   int           // cached repos when force=false
    Errors    []RepoError
    Duration  time.Duration
}

type RepoError struct {
    RepoID string
    Error  string
}
```

### 2. Analyzer (analyzer.go)

Create `internal/org/analyzer.go`:

**Analyzer struct:**
```go
type Analyzer struct {
    orgManager Manager
    orch       orchestrator.Manager
}

func NewAnalyzer(orgManager Manager, orch orchestrator.Manager) *Analyzer
```

**AnalyzeOrg method:**

```go
func (a *Analyzer) AnalyzeOrg(ctx context.Context, orgID string, force bool, concurrency int) (*AnalysisResult, error)
```

**Logic:**
1. Clamp concurrency to range 1-10 (default 3 if <= 0)
2. Get org via `a.orgManager.Get(ctx, orgID)` — return error if not found
3. Start timer
4. Create semaphore: `sem := make(chan struct{}, concurrency)`
5. Create results channel and WaitGroup
6. For each repo in org.Repos:
   - Check `ctx.Done()` before launching — if cancelled, stop
   - Acquire semaphore
   - Launch goroutine:
     a. Determine type: `strings.HasPrefix(repoID, "local:")` → AnalyzeLocal, else AnalyzeRepo
     b. Get effective config for repo context
     c. Call orchestrator method
     d. On failure: wait 1 second, retry once
     e. On retry failure: classify as non-retryable if error indicates "not found" or "invalid path", record error
     f. Release semaphore
7. Wait for all goroutines
8. Calculate Duration, populate AnalysisResult

**Error classification:** Skip retry for errors containing "not found", "no such file", "invalid". Retry transient errors (network, timeout).

### 3. Manager Signature Change

Change `NewManager` in `internal/org/manager.go`:

**Before:** `func NewManager(store Store) Manager`
**After:** `func NewManager(store Store, orch orchestrator.Manager) Manager`

The Manager creates the Analyzer internally and exposes `AnalyzeOrg` via the Manager interface.

Add to Manager interface:
```go
AnalyzeOrg(ctx context.Context, orgID string, force bool, concurrency int) (*AnalysisResult, error)
```

### 4. Add DeleteRepoContext to Orchestrator

Add to `internal/orchestrator/interface.go`:
```go
DeleteRepoContext(ctx context.Context, repoID string) error
```

Implementation in `internal/orchestrator/manager.go`: delegate to `storage.ContextStore.DeleteContext()`.

This is needed for cascade deletion in Section 06 (delete_org with mode=cascade).

## Tests to Write First

**In analyzer_test.go — mock orchestrator:**

Create a `mockOrchestrator` implementing `orchestrator.Manager` with:
- Configurable success/failure per repo
- Configurable retry behavior (fail first, succeed on retry)
- Atomic counter tracking max concurrent goroutines
- Configurable latency (time.Sleep)

```go
// Test: AnalyzeOrg with all repos succeeding — Succeeded equals Total, no Errors
// Test: AnalyzeOrg with one repo failing then succeeding on retry — Succeeded equals Total
// Test: AnalyzeOrg with one repo failing after retry — Failed=1, Errors has entry, rest succeed
// Test: AnalyzeOrg respects concurrency limit — max concurrent goroutines never exceeds limit
// Test: AnalyzeOrg with context cancellation — stops launching new repos, in-flight complete
// Test: AnalyzeOrg with force=true — passes force flag to orchestrator calls
// Test: AnalyzeOrg with empty org (no repos) — Succeeded=0, no error, Duration > 0
// Test: AnalyzeOrg with non-existent org — returns error (ErrNotFound)
// Test: AnalyzeOrg routes local: prefix repos to AnalyzeLocal
// Test: AnalyzeOrg routes non-local repos to AnalyzeRepo
// Test: AnalyzeOrg Duration is populated and non-zero
// Test: AnalyzeOrg clamps concurrency — 0 becomes 3, 15 becomes 10
// Test: AnalyzeOrg skips retry for non-retryable errors ("not found")
```

## Files to Create/Modify

| File | Action | Purpose |
|------|--------|---------|
| `internal/org/types.go` | Modify | Add AnalysisResult, RepoError types |
| `internal/org/analyzer.go` | Create | Analyzer with AnalyzeOrg method |
| `internal/org/manager.go` | Modify | Change NewManager signature, add AnalyzeOrg to interface |
| `internal/org/analyzer_test.go` | Create | Mock orchestrator + concurrency tests |
| `internal/orchestrator/interface.go` | Modify | Add DeleteRepoContext method |
| `internal/orchestrator/manager.go` | Modify | Implement DeleteRepoContext |

## Acceptance Criteria

- [ ] AnalyzeOrg processes all repos with bounded concurrency
- [ ] Retry logic works (fail → 1s wait → retry → succeed or record error)
- [ ] Non-retryable errors skip retry
- [ ] Context cancellation stops new goroutine launches
- [ ] Concurrency clamped to 1-10 range
- [ ] AnalysisResult correctly populated (Succeeded, Failed, Skipped, Errors, Duration)
- [ ] NewManager accepts orchestrator.Manager
- [ ] DeleteRepoContext added to orchestrator interface
- [ ] All analyzer tests pass with mock orchestrator
