# Section 05: Pattern Execution Fixes

## Overview

This section fixes two bugs in the pattern execution system (`internal/compose/`) and adds observability improvements:

1. **Conditional steps skip silently** -- when a chain step's condition evaluates to false, no output explains why the step did not run.
2. **The `impact_analysis` pattern calls `get_function_context` without first resolving `file_path`** -- it assumes the caller provides `file_path`, but this is not always available.
3. **The `search_with_context` pattern's transform assumes `[]map[string]any` result format** -- other formats cause silent failures.

This section has **no dependencies** on other sections and can be implemented in parallel with any other section.

## Files to Modify

- `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/compose/chain.go` -- add StepResults tracking with three-state status, partial completion output
- `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/compose/patterns.go` -- fix impact_analysis (add search step with disambiguation), improve search_with_context result parsing

## Files to Create (Tests)

- `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/compose/chain_step_status_test.go` -- new test file for step status tracking (keep separate from existing `chain_test.go`)
- `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/compose/patterns_test.go` -- tests for pattern fixes

---

## Tests First

### `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/compose/chain_step_status_test.go`

```go
package compose

import (
	"context"
	"testing"
)

// Test: Chain with all steps executed returns status "executed" for each
func TestStepResults_AllExecuted(t *testing.T) {
	// Build a chain with 3 unconditional steps, all succeed.
	// After execution, StepResults should have 3 entries, all with Status == "executed".
}

// Test: Chain with conditional step skipped returns status "skipped" with reason
func TestStepResults_ConditionalSkipped(t *testing.T) {
	// Build a chain where step 2 has a condition that evaluates to false.
	// StepResults[1] should have Status == "skipped" and a non-empty Reason.
}

// Test: Chain where step 1 fails marks remaining steps as "not_reached"
func TestStepResults_NotReached(t *testing.T) {
	// Build a 3-step chain where step 2 fails (returns error).
	// StepResults[2] should have Status == "not_reached".
}

// Test: Partial completion output includes results from executed steps
func TestPartialCompletion_IncludesExecutedResults(t *testing.T) {
	// Build a chain where step 1 succeeds, step 2 fails.
	// The chain context should contain the result data from step 1
	// and StepResults should reflect executed + failed + not_reached.
}

// Test: Partial completion output explains why skipped steps were skipped
func TestPartialCompletion_ExplainsSkippedSteps(t *testing.T) {
	// Build a chain where step 2 is conditional and skipped.
	// StepResults[1].Reason should describe the skip cause.
}

// Test: StepResults slice has entry for every step (executed, skipped, or not_reached)
func TestStepResults_HasEntryForEveryStep(t *testing.T) {
	// Build a 4-step chain: step 1 executes, step 2 skipped, step 3 executes, step 4 executes.
	// len(StepResults) should == 4, with correct statuses.
}
```

### `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/compose/patterns_test.go`

```go
package compose

import (
	"context"
	"testing"
)

// Test: impact_analysis resolves file_path via search step before get_function_context
func TestImpactAnalysis_ResolvesFilePath(t *testing.T) {
	// Register mock search_context that returns [{file: "pkg/router.go", name: "ServeHTTP"}]
	// Register mock get_function_context that verifies it receives file_path == "pkg/router.go"
	// Execute the impact_analysis pattern with only repo_id and function_name (no file_path).
	// Verify get_function_context was called with the resolved file_path.
}

// Test: impact_analysis with unknown function returns clear error (search found nothing)
func TestImpactAnalysis_UnknownFunction(t *testing.T) {
	// Register mock search_context that returns empty results.
	// Execute impact_analysis. Verify the chain stops with a clear error message.
}

// Test: impact_analysis with multi-result search uses highest-confidence result
func TestImpactAnalysis_MultiResultDisambiguation(t *testing.T) {
	// Register mock search_context returning multiple results with different confidence scores.
	// Verify the result with the highest confidence is selected for file_path.
	// Verify the chosen file_path is included in the result message.
}

// Test: impact_analysis includes chosen file_path in result message
func TestImpactAnalysis_IncludesChosenFilePath(t *testing.T) {
	// Register mock search_context returning a single result.
	// Verify the result message mentions which file was selected.
}

// Test: search_with_context handles non-array result format gracefully
func TestSearchWithContext_NonArrayResults(t *testing.T) {
	// Register mock search_context that returns a map instead of []map[string]any.
	// Verify step 2 is marked "skipped" (not a crash) with an explanatory reason.
}

// Test: search_with_context marks step 2 as "skipped" when result parsing fails (with reason)
func TestSearchWithContext_SkippedWithReason(t *testing.T) {
	// Register mock search_context returning malformed data.
	// Verify StepResults shows step 2 as "skipped" with a reason like "could not parse search results".
}

// Test: search_with_context completes successfully with well-formed results
func TestSearchWithContext_Success(t *testing.T) {
	// Register mock search_context returning []map[string]any with file and name fields.
	// Register mock get_function_context that succeeds.
	// Verify both steps executed successfully.
}
```

---

## Implementation Details

### 1. Add Three-State Step Tracking to `ChainContext`

**File:** `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/compose/chain.go`

Add a new `StepStatus` type and `StepResult` struct to track each step's outcome:

```go
// StepStatus represents the execution state of a chain step.
type StepStatus string

const (
	StepExecuted   StepStatus = "executed"
	StepSkipped    StepStatus = "skipped"
	StepNotReached StepStatus = "not_reached"
	StepFailed     StepStatus = "failed"
)

// StepResult records the outcome of a single chain step.
type StepResult struct {
	StepName string     `json:"step_name"`
	ToolName string     `json:"tool_name"`
	Status   StepStatus `json:"status"`
	Reason   string     `json:"reason,omitempty"`
	Data     any        `json:"data,omitempty"`
	Error    string     `json:"error,omitempty"`
}
```

Add a `StepResults` field to `ChainContext`:

```go
type ChainContext struct {
	context.Context
	Results     []ToolResult   `json:"results"`
	StepResults []StepResult   `json:"step_results"`   // NEW: per-step tracking
	Vars        map[string]any `json:"vars"`
	StopReason  string         `json:"stop_reason,omitempty"`
	stopped     bool
}
```

Initialize `StepResults` in `NewChainContext`:

```go
func NewChainContext(ctx context.Context) *ChainContext {
	return &ChainContext{
		Context:     ctx,
		Results:     make([]ToolResult, 0),
		StepResults: make([]StepResult, 0),
		Vars:        make(map[string]any),
	}
}
```

### 2. Add a `Name` Field to `ChainStep`

The `ChainStep` struct currently has `Call`, `Condition`, and `Transform`. Add a `Name` field so steps can be identified in status output:

```go
type ChainStep struct {
	Name      string                                  // Human-readable step name
	Call      ToolCall
	Condition func(*ChainContext) bool
	Transform func(*ChainContext, *ToolResult) error
}
```

Update `Add`, `AddConditional`, and `AddWithTransform` to accept or auto-generate step names. The simplest approach: derive the name from the tool call name and step index (e.g., `"step_1_search_context"`). Alternatively, add new `AddNamed` variants or set the name from `Call.Name` automatically.

The recommended approach is to auto-generate names from `Call.Name` when not explicitly provided. This avoids changing the existing API surface.

### 3. Modify `Chain.Execute()` to Track Step Status

The core change is in the `Execute` method. The current loop in `chain.go:165-203` must be restructured to:

1. **Before the loop:** Know the total number of steps so we can mark unreached ones.
2. **When a condition is false:** Append a `StepResult` with `Status: StepSkipped` and a reason string (e.g., `"condition evaluated to false"`).
3. **When a step executes successfully:** Append a `StepResult` with `Status: StepExecuted`.
4. **When a step fails:** Append a `StepResult` with `Status: StepFailed` and the error message.
5. **After the loop exits early (due to failure or stop):** Iterate over remaining steps and mark them `StepNotReached` with reason `"earlier step failed: <step_name>"`.

Key behavioral change: currently, when a conditional step is skipped, the loop uses `continue` and no `ToolResult` is appended. This behavior stays the same for backward compatibility (the `Results` slice only contains actually-executed tool results). The new `StepResults` slice is the one that gets an entry for every step regardless of whether it ran.

Here is the modified loop logic in pseudocode:

```
for i, step := range steps:
    if context cancelled:
        mark remaining steps (i..end) as not_reached("context cancelled")
        return
    if chain stopped:
        mark remaining steps (i..end) as not_reached("chain stopped: " + reason)
        break
    if condition is false:
        append StepResult{Status: skipped, Reason: "condition evaluated to false"}
        continue
    execute step
    if success:
        append StepResult{Status: executed, Data: result.Data}
    else:
        append StepResult{Status: failed, Error: result.Error}
        mark remaining steps (i+1..end) as not_reached("step failed: " + step.Name)
        break
```

### 4. Update `ChainSummary` for Partial Completion

Extend the `Summary()` method (or add a new `DetailedSummary()` method) to include step-level status counts:

```go
type ChainSummary struct {
	TotalSteps    int          `json:"total_steps"`
	SuccessSteps  int          `json:"success_steps"`
	FailedSteps   int          `json:"failed_steps"`
	SkippedSteps  int          `json:"skipped_steps"`    // NEW
	NotReached    int          `json:"not_reached"`       // NEW
	TotalDuration string       `json:"total_duration"`
	TotalTokens   int          `json:"total_tokens"`
	StopReason    string       `json:"stop_reason,omitempty"`
	StepDetails   []StepResult `json:"step_details,omitempty"` // NEW
}
```

The `Summary()` method should iterate `StepResults` to compute `SkippedSteps` and `NotReached` counts, and include the full `StepResults` slice as `StepDetails`.

### 5. Fix `ImpactAnalysis` Pattern

**File:** `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/compose/patterns.go`

The current `ImpactAnalysis.Build()` (lines 241-285) has step 1 calling `get_function_context` with `params` directly. The problem: `params` may not include `file_path`, and `get_function_context` requires it.

**Fix:** Insert a new step 0 that calls `search_context` to resolve the `file_path` from the function name.

The new step sequence for `impact_analysis`:

1. **Step 0 (NEW): Search for function** -- Call `search_context` with `function_name` and `repo_id`. Extract the `file_path` from the first (highest-confidence) result. Store it in the chain context.
2. **Step 1: Get function context** -- Call `get_function_context` using the resolved `file_path` from step 0 (via `{{file_path}}` variable resolution).
3. **Step 2: Get callers** -- Unchanged.
4. **Step 3: Search by concept** -- Unchanged.

**Disambiguation logic for step 0's transform:** When `search_context` returns multiple results, select the one with the highest confidence score. The transform function should:

- Type-switch on the result data (handle `[]map[string]any`, `map[string]any` with `items` key, and other formats).
- If results is a slice, iterate to find the entry with the highest `"confidence"` or `"score"` field.
- Store the chosen `file_path` and `function_name` in the chain context.
- Include the chosen file path in a context variable (e.g., `"resolved_file_message"`) so it can be surfaced in output: `"Selected file: pkg/router.go (confidence: 0.95)"`.
- If no results found, do not set `file_path` -- the subsequent `get_function_context` step should fail with a clear error because the required parameter is missing.

**Skip step 0 when `file_path` is already provided:** Add a condition to step 0 that checks if `params` already contains a non-empty `file_path`. If so, skip the search step (the user already knows the file). Store the provided `file_path` directly in the chain context.

### 6. Fix `SearchWithContext` Result Parsing

**File:** `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/compose/patterns.go`

The current condition function for step 2 (lines 142-155) only handles `case []map[string]any`. Add a type switch to handle other result formats:

```go
func(ctx *ChainContext) bool {
    results := ctx.LastData()
    if results == nil {
        return false
    }

    switch r := results.(type) {
    case []map[string]any:
        // Current handling -- extract first result's file and name
        if len(r) > 0 {
            if file, ok := r[0]["file"].(string); ok {
                ctx.Set("first_file", file)
            }
            if name, ok := r[0]["name"].(string); ok {
                ctx.Set("first_func", name)
            }
            return true
        }
    case map[string]any:
        // Handle wrapped results like {"items": [...], "count": N}
        if items, ok := r["items"].([]any); ok && len(items) > 0 {
            if item, ok := items[0].(map[string]any); ok {
                if file, ok := item["file"].(string); ok {
                    ctx.Set("first_file", file)
                }
                if name, ok := item["name"].(string); ok {
                    ctx.Set("first_func", name)
                }
                return true
            }
        }
        // Handle single result directly in the map
        if file, ok := r["file"].(string); ok {
            ctx.Set("first_file", file)
            if name, ok := r["name"].(string); ok {
                ctx.Set("first_func", name)
            }
            return true
        }
    case []any:
        // Handle []any (JSON deserialized arrays)
        if len(r) > 0 {
            if item, ok := r[0].(map[string]any); ok {
                if file, ok := item["file"].(string); ok {
                    ctx.Set("first_file", file)
                }
                if name, ok := item["name"].(string); ok {
                    ctx.Set("first_func", name)
                }
                return true
            }
        }
    }
    // If we reach here, result format is unrecognized.
    // Log or mark as skipped with reason.
    return false
}
```

When this condition returns `false`, the step is skipped. With the new step tracking from the chain changes, this will automatically produce a `StepResult` with `Status: "skipped"` and a reason. To provide a more specific reason, the condition function can set a context variable like `ctx.Set("_skip_reason", "could not parse search results: unexpected format")` and the chain execution loop can check for this variable when recording skip reasons.

### 7. Integrating Skip Reasons from Conditions

To allow condition functions to provide specific skip reasons (rather than the generic "condition evaluated to false"), add a convention: if a condition function sets `ctx.Vars["_skip_reason"]` before returning `false`, the chain executor uses that string as the `Reason` in the `StepResult`. After reading it, the executor deletes the key to avoid polluting the context for subsequent steps.

This is a lightweight convention that does not require changing the `Condition` function signature.

---

## Existing Code Context

The existing `ChainContext` struct (in `chain.go:27-33`) has `Results`, `Vars`, `StopReason`, and `stopped`. The new `StepResults` field is additive.

The existing `ChainStep` struct (in `chain.go:110-114`) has `Call`, `Condition`, and `Transform`. The new `Name` field is additive.

The existing `Execute` loop (in `chain.go:156-206`) iterates steps, checks conditions with `continue` on false, executes, transforms, and stops on error with `break`. The modification wraps each of these paths with `StepResult` recording.

The existing test file (`chain_test.go`) has tests for basic execution, conditional steps, variable resolution, context cancellation, and summary. The new tests in `chain_step_status_test.go` are additive and test only the new step-tracking behavior.

The `ImpactAnalysis` pattern (in `patterns.go:227-285`) currently has 3 steps: get_function_context, get_callers, search_by_concept. The fix inserts a search_context step at position 0.

The `SearchWithContext` pattern (in `patterns.go:92-159`) has 2 steps: search_context, get_function_context. The fix improves the condition function on step 2.

---

## Risk Notes

- **Step status tracking is additive** -- it does not change execution flow. Existing behavior (the `Results` slice, stop-on-error, conditional skip) is unchanged. The `StepResults` slice is a parallel tracking mechanism.
- **Pattern changes (impact_analysis search step, search_with_context parsing)** do change behavior, but in a backward-compatible way: impact_analysis now works when `file_path` is missing (previously it would fail), and search_with_context handles more result formats (previously it silently failed).
- The `_skip_reason` convention is optional -- if not adopted, skip reasons default to `"condition evaluated to false"`, which is still more informative than the current silent skip.