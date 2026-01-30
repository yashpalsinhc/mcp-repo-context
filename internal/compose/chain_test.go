package compose

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestNewChainContext(t *testing.T) {
	ctx := NewChainContext(context.Background())

	if ctx == nil {
		t.Fatal("NewChainContext returned nil")
	}

	if len(ctx.Results) != 0 {
		t.Error("Results should be empty initially")
	}

	if len(ctx.Vars) != 0 {
		t.Error("Vars should be empty initially")
	}
}

func TestChainContext_SetGet(t *testing.T) {
	ctx := NewChainContext(context.Background())

	ctx.Set("key", "value")

	v, ok := ctx.Get("key")
	if !ok {
		t.Error("Get should return ok=true for existing key")
	}
	if v != "value" {
		t.Errorf("Get returned %v, expected 'value'", v)
	}

	_, ok = ctx.Get("nonexistent")
	if ok {
		t.Error("Get should return ok=false for nonexistent key")
	}
}

func TestChainContext_GetString(t *testing.T) {
	ctx := NewChainContext(context.Background())

	ctx.Set("str", "hello")
	ctx.Set("num", 123)

	if ctx.GetString("str") != "hello" {
		t.Error("GetString should return string value")
	}

	if ctx.GetString("num") != "" {
		t.Error("GetString should return empty string for non-string")
	}

	if ctx.GetString("missing") != "" {
		t.Error("GetString should return empty string for missing key")
	}
}

func TestChainContext_LastResult(t *testing.T) {
	ctx := NewChainContext(context.Background())

	if ctx.LastResult() != nil {
		t.Error("LastResult should return nil when no results")
	}

	ctx.Results = append(ctx.Results, ToolResult{ToolName: "first"})
	ctx.Results = append(ctx.Results, ToolResult{ToolName: "second"})

	last := ctx.LastResult()
	if last == nil || last.ToolName != "second" {
		t.Error("LastResult should return most recent result")
	}
}

func TestChainContext_Stop(t *testing.T) {
	ctx := NewChainContext(context.Background())

	if ctx.IsStopped() {
		t.Error("Chain should not be stopped initially")
	}

	ctx.Stop("test reason")

	if !ctx.IsStopped() {
		t.Error("Chain should be stopped after Stop()")
	}

	if ctx.StopReason != "test reason" {
		t.Error("StopReason should be set")
	}
}

func mockExecutor() *MapExecutor {
	executor := NewMapExecutor()

	executor.Register("echo", func(ctx *ChainContext, params map[string]any) (any, error) {
		return params, nil
	})

	executor.Register("error", func(ctx *ChainContext, params map[string]any) (any, error) {
		return nil, errors.New("intentional error")
	})

	executor.Register("store", func(ctx *ChainContext, params map[string]any) (any, error) {
		if value, ok := params["value"]; ok {
			ctx.Set("stored", value)
		}
		return "stored", nil
	})

	return executor
}

func TestChain_Execute(t *testing.T) {
	executor := mockExecutor()
	chain := NewChain(executor)

	chain.Add(ToolCall{Name: "echo", Params: map[string]any{"msg": "hello"}})
	chain.Add(ToolCall{Name: "echo", Params: map[string]any{"msg": "world"}})

	result, err := chain.Execute(context.Background())
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if len(result.Results) != 2 {
		t.Errorf("Expected 2 results, got %d", len(result.Results))
	}

	for _, r := range result.Results {
		if !r.Success {
			t.Errorf("Result should be successful: %s", r.Error)
		}
	}
}

func TestChain_ExecuteWithError(t *testing.T) {
	executor := mockExecutor()
	chain := NewChain(executor)

	chain.Add(ToolCall{Name: "echo", Params: nil})
	chain.Add(ToolCall{Name: "error", Params: nil})
	chain.Add(ToolCall{Name: "echo", Params: nil}) // Should not execute

	result, _ := chain.Execute(context.Background())

	// Should stop on error
	if len(result.Results) != 2 {
		t.Errorf("Expected 2 results (stopped on error), got %d", len(result.Results))
	}

	if result.Results[1].Success {
		t.Error("Second result should be an error")
	}

	if !result.IsStopped() {
		t.Error("Chain should be stopped after error")
	}
}

func TestChain_ConditionalStep(t *testing.T) {
	executor := mockExecutor()
	chain := NewChain(executor)

	// Always runs
	chain.Add(ToolCall{Name: "store", Params: map[string]any{"value": "test"}})

	// Conditional - should run because "stored" was set
	chain.AddConditional(
		ToolCall{Name: "echo", Params: map[string]any{"cond": "yes"}},
		func(ctx *ChainContext) bool {
			_, ok := ctx.Get("stored")
			return ok
		},
	)

	// Conditional - should not run because "missing" doesn't exist
	chain.AddConditional(
		ToolCall{Name: "echo", Params: map[string]any{"cond": "no"}},
		func(ctx *ChainContext) bool {
			_, ok := ctx.Get("missing")
			return ok
		},
	)

	result, _ := chain.Execute(context.Background())

	if len(result.Results) != 2 {
		t.Errorf("Expected 2 results (1 skipped), got %d", len(result.Results))
	}
}

func TestChain_VariableResolution(t *testing.T) {
	executor := NewMapExecutor()
	executor.Register("check", func(ctx *ChainContext, params map[string]any) (any, error) {
		return params["value"], nil
	})

	chain := NewChain(executor)

	// This step stores the value to be resolved
	chain.AddWithTransform(
		ToolCall{Name: "check", Params: map[string]any{"value": "initial"}},
		func(ctx *ChainContext, result *ToolResult) error {
			ctx.Set("myVar", "resolved_value")
			return nil
		},
	)

	// This step uses the resolved variable
	chain.Add(ToolCall{
		Name:   "check",
		Params: map[string]any{"value": "{{myVar}}"},
	})

	result, _ := chain.Execute(context.Background())

	if len(result.Results) != 2 {
		t.Fatalf("Expected 2 results, got %d", len(result.Results))
	}

	// Second result should have resolved value
	data := result.Results[1].Data
	if data != "resolved_value" {
		t.Errorf("Variable not resolved: got %v", data)
	}
}

func TestChain_ContextCancellation(t *testing.T) {
	executor := NewMapExecutor()
	executor.Register("slow", func(ctx *ChainContext, params map[string]any) (any, error) {
		time.Sleep(100 * time.Millisecond)
		return "done", nil
	})

	chain := NewChain(executor)
	chain.Add(ToolCall{Name: "slow", Params: nil})
	chain.Add(ToolCall{Name: "slow", Params: nil})

	ctx, cancel := context.WithCancel(context.Background())

	// Cancel immediately
	cancel()

	result, err := chain.Execute(ctx)

	if err == nil {
		t.Error("Expected context cancellation error")
	}

	if !result.IsStopped() {
		t.Error("Chain should be stopped on cancellation")
	}
}

func TestChainSummary(t *testing.T) {
	ctx := NewChainContext(context.Background())
	ctx.Results = []ToolResult{
		{Success: true, Duration: "10ms", TokenCost: 100},
		{Success: true, Duration: "20ms", TokenCost: 200},
		{Success: false, Duration: "5ms", TokenCost: 50, Error: "failed"},
	}
	ctx.StopReason = "error"

	summary := ctx.Summary()

	if summary.TotalSteps != 3 {
		t.Errorf("TotalSteps: expected 3, got %d", summary.TotalSteps)
	}

	if summary.SuccessSteps != 2 {
		t.Errorf("SuccessSteps: expected 2, got %d", summary.SuccessSteps)
	}

	if summary.FailedSteps != 1 {
		t.Errorf("FailedSteps: expected 1, got %d", summary.FailedSteps)
	}

	if summary.TotalTokens != 350 {
		t.Errorf("TotalTokens: expected 350, got %d", summary.TotalTokens)
	}

	if summary.StopReason != "error" {
		t.Errorf("StopReason: expected 'error', got '%s'", summary.StopReason)
	}
}

func TestMapExecutor_UnknownTool(t *testing.T) {
	executor := NewMapExecutor()

	result := executor.Execute(
		NewChainContext(context.Background()),
		ToolCall{Name: "unknown", Params: nil},
	)

	if result.Success {
		t.Error("Unknown tool should fail")
	}

	if result.Error != "unknown tool: unknown" {
		t.Errorf("Unexpected error: %s", result.Error)
	}
}

func TestFuncExecutor(t *testing.T) {
	executor := NewFuncExecutor(func(ctx *ChainContext, call ToolCall) ToolResult {
		return ToolResult{
			ToolName: call.Name,
			Success:  true,
			Data:     "custom",
		}
	})

	result := executor.Execute(
		NewChainContext(context.Background()),
		ToolCall{Name: "test", Params: nil},
	)

	if !result.Success || result.Data != "custom" {
		t.Error("FuncExecutor didn't work correctly")
	}
}
