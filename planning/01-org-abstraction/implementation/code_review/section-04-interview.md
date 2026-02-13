# Section 04 Code Review Interview: Analyzer

## Fixes Applied (from review)

### C-1: Race on context cancellation early return (CRITICAL — auto-fixed)
Changed for-loop to `break` on ctx cancellation instead of `return`, so execution falls through to `wg.Wait()` for in-flight goroutines. Further simplified by removing the ineffective `select`/`break` pattern and using a direct `ctx.Err() != nil` check.

### M-4: time.After creates timer leak (MEDIUM — auto-fixed)
Replaced `time.After(1*time.Second)` with `time.NewTimer` + `defer timer.Stop()` to prevent timer leak when context cancels during the retry wait.

### CO-1: analyzeRepo unused orgID parameter (COSMETIC — auto-fixed)
Removed unused `orgID` parameter from `analyzeRepo` signature.

### Compilation fixes (not in review — auto-fixed)
- Added `DeleteRepoContext` stub to mock implementations in `visualizer_test.go`, `tools_test.go`, `resources_test.go`
- Fixed `main.go` `org.NewManager` call to pass orchestrator manager as second argument
- Removed ineffective `break` inside `select` (SA4011 lint warning)

## Items Let Go (with rationale)

### C-2: Total invariant broken during cancellation
During context cancellation, `Total > Succeeded + Failed` is expected behavior. Callers should check `ctx.Err()` alongside the result. Enforcing the invariant mid-cancellation would add complexity without practical benefit.

### M-1: Plan step 6b "Get effective config" not implemented
The analyzer delegates to `orchestrator.AnalyzeLocal/AnalyzeRepo` which handle their own configuration. Per-repo config merging is not needed at the analyzer level.

### M-2: Skipped field never populated
Will be populated when MCP tools implement staleness checks in section 06.

### M-3: isNonRetryable uses fragile string matching
Standard Go pattern for classifying third-party errors that don't export sentinel types. Acceptable trade-off.

### L-1, L-2, L-3: Test adequacy concerns
Tests are sufficient for current scope. Force tracking is per-test (not per-repo), clamping logic is trivially correct, and cancellation test uses generous timeouts.

### CO-2: No logging in analyzer
Deferred to when structured logging infrastructure is established across the codebase.
