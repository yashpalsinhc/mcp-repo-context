package compose

import (
	"context"
	"testing"
)

func TestStepResults_AllExecuted(t *testing.T) {
	executor := mockExecutor()
	chain := NewChain(executor)

	chain.Add(ToolCall{Name: "echo", Params: map[string]any{"msg": "a"}})
	chain.Add(ToolCall{Name: "echo", Params: map[string]any{"msg": "b"}})
	chain.Add(ToolCall{Name: "echo", Params: map[string]any{"msg": "c"}})

	result, err := chain.Execute(context.Background())
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if len(result.StepResults) != 3 {
		t.Fatalf("expected 3 StepResults, got %d", len(result.StepResults))
	}

	for i, sr := range result.StepResults {
		if sr.Status != StepExecuted {
			t.Errorf("step %d: status = %s, want executed", i, sr.Status)
		}
	}
}

func TestStepResults_ConditionalSkipped(t *testing.T) {
	executor := mockExecutor()
	chain := NewChain(executor)

	chain.Add(ToolCall{Name: "echo", Params: map[string]any{"msg": "a"}})

	// This step is conditional and should be skipped
	chain.AddConditional(
		ToolCall{Name: "echo", Params: map[string]any{"msg": "skip"}},
		func(ctx *ChainContext) bool {
			_, ok := ctx.Get("nonexistent")
			return ok
		},
	)

	chain.Add(ToolCall{Name: "echo", Params: map[string]any{"msg": "c"}})

	result, err := chain.Execute(context.Background())
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if len(result.StepResults) != 3 {
		t.Fatalf("expected 3 StepResults, got %d", len(result.StepResults))
	}

	if result.StepResults[0].Status != StepExecuted {
		t.Errorf("step 0: status = %s, want executed", result.StepResults[0].Status)
	}
	if result.StepResults[1].Status != StepSkipped {
		t.Errorf("step 1: status = %s, want skipped", result.StepResults[1].Status)
	}
	if result.StepResults[1].Reason == "" {
		t.Error("step 1: skipped reason should not be empty")
	}
	if result.StepResults[2].Status != StepExecuted {
		t.Errorf("step 2: status = %s, want executed", result.StepResults[2].Status)
	}
}

func TestStepResults_NotReached(t *testing.T) {
	executor := mockExecutor()
	chain := NewChain(executor)

	chain.Add(ToolCall{Name: "echo", Params: nil})
	chain.Add(ToolCall{Name: "error", Params: nil})
	chain.Add(ToolCall{Name: "echo", Params: nil}) // Should not execute

	result, _ := chain.Execute(context.Background())

	if len(result.StepResults) != 3 {
		t.Fatalf("expected 3 StepResults, got %d", len(result.StepResults))
	}

	if result.StepResults[0].Status != StepExecuted {
		t.Errorf("step 0: status = %s, want executed", result.StepResults[0].Status)
	}
	if result.StepResults[1].Status != StepFailed {
		t.Errorf("step 1: status = %s, want failed", result.StepResults[1].Status)
	}
	if result.StepResults[2].Status != StepNotReached {
		t.Errorf("step 2: status = %s, want not_reached", result.StepResults[2].Status)
	}
}

func TestStepResults_HasEntryForEveryStep(t *testing.T) {
	executor := mockExecutor()
	chain := NewChain(executor)

	// Step 1: executes
	chain.Add(ToolCall{Name: "store", Params: map[string]any{"value": "test"}})
	// Step 2: skipped (missing key)
	chain.AddConditional(
		ToolCall{Name: "echo", Params: nil},
		func(ctx *ChainContext) bool {
			_, ok := ctx.Get("missing_key")
			return ok
		},
	)
	// Step 3: executes
	chain.Add(ToolCall{Name: "echo", Params: nil})
	// Step 4: executes
	chain.Add(ToolCall{Name: "echo", Params: nil})

	result, _ := chain.Execute(context.Background())

	if len(result.StepResults) != 4 {
		t.Fatalf("expected 4 StepResults, got %d", len(result.StepResults))
	}

	expected := []StepStatus{StepExecuted, StepSkipped, StepExecuted, StepExecuted}
	for i, want := range expected {
		if result.StepResults[i].Status != want {
			t.Errorf("step %d: status = %s, want %s", i, result.StepResults[i].Status, want)
		}
	}
}

func TestStepResults_CustomSkipReason(t *testing.T) {
	executor := mockExecutor()
	chain := NewChain(executor)

	chain.AddConditional(
		ToolCall{Name: "echo", Params: nil},
		func(ctx *ChainContext) bool {
			ctx.Set("_skip_reason", "no results to parse")
			return false
		},
	)

	result, _ := chain.Execute(context.Background())

	if len(result.StepResults) != 1 {
		t.Fatalf("expected 1 StepResult, got %d", len(result.StepResults))
	}
	if result.StepResults[0].Reason != "no results to parse" {
		t.Errorf("reason = %q, want %q", result.StepResults[0].Reason, "no results to parse")
	}
}

func TestSummary_IncludesStepCounts(t *testing.T) {
	executor := mockExecutor()
	chain := NewChain(executor)

	chain.Add(ToolCall{Name: "echo", Params: nil})
	chain.AddConditional(
		ToolCall{Name: "echo", Params: nil},
		func(ctx *ChainContext) bool { return false },
	)
	chain.Add(ToolCall{Name: "error", Params: nil})
	chain.Add(ToolCall{Name: "echo", Params: nil}) // not reached

	result, _ := chain.Execute(context.Background())
	summary := result.Summary()

	if summary.TotalSteps != 4 {
		t.Errorf("TotalSteps = %d, want 4", summary.TotalSteps)
	}
	if summary.SkippedSteps != 1 {
		t.Errorf("SkippedSteps = %d, want 1", summary.SkippedSteps)
	}
	if summary.NotReached != 1 {
		t.Errorf("NotReached = %d, want 1", summary.NotReached)
	}
	if len(summary.StepDetails) != 4 {
		t.Errorf("StepDetails len = %d, want 4", len(summary.StepDetails))
	}
}
