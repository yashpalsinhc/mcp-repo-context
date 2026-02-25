package compose

import (
	"context"
	"testing"
)

func TestImpactAnalysis_ResolvesFilePath(t *testing.T) {
	executor := NewMapExecutor()

	executor.Register("search_context", func(ctx *ChainContext, params map[string]any) (any, error) {
		return []map[string]any{
			{"file": "pkg/router.go", "name": "ServeHTTP"},
		}, nil
	})

	var receivedFilePath string
	executor.Register("get_function_context", func(ctx *ChainContext, params map[string]any) (any, error) {
		receivedFilePath, _ = params["file_path"].(string)
		return map[string]any{"name": "ServeHTTP"}, nil
	})
	executor.Register("get_callers", func(ctx *ChainContext, params map[string]any) (any, error) {
		return []map[string]any{}, nil
	})
	executor.Register("search_by_concept", func(ctx *ChainContext, params map[string]any) (any, error) {
		return []map[string]any{}, nil
	})

	pattern := NewImpactAnalysis()
	// No file_path in params -- should be resolved via search
	params := map[string]any{
		"repo_id":       "test-repo",
		"function_name": "ServeHTTP",
	}

	_, err := ExecutePattern(context.Background(), executor, pattern, params)
	if err != nil {
		t.Fatalf("ExecutePattern failed: %v", err)
	}

	if receivedFilePath != "pkg/router.go" {
		t.Errorf("file_path = %q, want %q", receivedFilePath, "pkg/router.go")
	}
}

func TestImpactAnalysis_WithFilePathSkipsSearch(t *testing.T) {
	executor := NewMapExecutor()

	searchCalled := false
	executor.Register("search_context", func(ctx *ChainContext, params map[string]any) (any, error) {
		searchCalled = true
		return nil, nil
	})
	executor.Register("get_function_context", func(ctx *ChainContext, params map[string]any) (any, error) {
		return map[string]any{"name": "ServeHTTP"}, nil
	})
	executor.Register("get_callers", func(ctx *ChainContext, params map[string]any) (any, error) {
		return []map[string]any{}, nil
	})
	executor.Register("search_by_concept", func(ctx *ChainContext, params map[string]any) (any, error) {
		return []map[string]any{}, nil
	})

	pattern := NewImpactAnalysis()
	params := map[string]any{
		"repo_id":       "test-repo",
		"function_name": "ServeHTTP",
		"file_path":     "pkg/router.go",
	}

	_, err := ExecutePattern(context.Background(), executor, pattern, params)
	if err != nil {
		t.Fatalf("ExecutePattern failed: %v", err)
	}

	if searchCalled {
		t.Error("search_context should not be called when file_path is provided")
	}
}

func TestSearchWithContext_Success(t *testing.T) {
	executor := NewMapExecutor()

	executor.Register("search_context", func(ctx *ChainContext, params map[string]any) (any, error) {
		return []map[string]any{
			{"file": "pkg/handler.go", "name": "HandleRequest"},
		}, nil
	})
	executor.Register("get_function_context", func(ctx *ChainContext, params map[string]any) (any, error) {
		return map[string]any{"name": "HandleRequest", "behavior": "handles requests"}, nil
	})

	pattern := NewSearchWithContext()
	params := map[string]any{
		"repo_id": "test-repo",
		"query":   "HandleRequest",
	}

	result, err := ExecutePattern(context.Background(), executor, pattern, params)
	if err != nil {
		t.Fatalf("ExecutePattern failed: %v", err)
	}

	// Both steps should have executed
	executed := 0
	for _, sr := range result.StepResults {
		if sr.Status == StepExecuted {
			executed++
		}
	}
	if executed != 2 {
		t.Errorf("expected 2 executed steps, got %d", executed)
	}
}

func TestSearchWithContext_NonArrayResults(t *testing.T) {
	executor := NewMapExecutor()

	// Return a non-standard format
	executor.Register("search_context", func(ctx *ChainContext, params map[string]any) (any, error) {
		return "just a string", nil
	})
	executor.Register("get_function_context", func(ctx *ChainContext, params map[string]any) (any, error) {
		return nil, nil
	})

	pattern := NewSearchWithContext()
	params := map[string]any{"repo_id": "test-repo", "query": "test"}

	result, err := ExecutePattern(context.Background(), executor, pattern, params)
	if err != nil {
		t.Fatalf("ExecutePattern failed: %v", err)
	}

	// Step 2 should be skipped due to unparseable results
	if len(result.StepResults) < 2 {
		t.Fatalf("expected at least 2 StepResults, got %d", len(result.StepResults))
	}
	if result.StepResults[1].Status != StepSkipped {
		t.Errorf("step 2 status = %s, want skipped", result.StepResults[1].Status)
	}
	if result.StepResults[1].Reason == "" {
		t.Error("skipped step should have a reason")
	}
}

func TestSearchWithContext_WrappedResults(t *testing.T) {
	executor := NewMapExecutor()

	// Return results wrapped in a map with "items" key
	executor.Register("search_context", func(ctx *ChainContext, params map[string]any) (any, error) {
		return map[string]any{
			"items": []any{
				map[string]any{"file": "pkg/auth.go", "name": "ValidateToken"},
			},
			"count": 1,
		}, nil
	})

	var receivedFile string
	executor.Register("get_function_context", func(ctx *ChainContext, params map[string]any) (any, error) {
		receivedFile, _ = params["file_path"].(string)
		return map[string]any{"name": "ValidateToken"}, nil
	})

	pattern := NewSearchWithContext()
	params := map[string]any{"repo_id": "test-repo", "query": "ValidateToken"}

	result, err := ExecutePattern(context.Background(), executor, pattern, params)
	if err != nil {
		t.Fatalf("ExecutePattern failed: %v", err)
	}

	// Both steps should execute
	executed := 0
	for _, sr := range result.StepResults {
		if sr.Status == StepExecuted {
			executed++
		}
	}
	if executed != 2 {
		t.Errorf("expected 2 executed steps, got %d", executed)
	}
	if receivedFile != "pkg/auth.go" {
		t.Errorf("file_path = %q, want %q", receivedFile, "pkg/auth.go")
	}
}

func TestExtractFirstResult_SliceMapFormat(t *testing.T) {
	ctx := NewChainContext(context.Background())
	data := []map[string]any{{"file": "a.go", "name": "Foo"}}
	if !extractFirstResult(ctx, data) {
		t.Fatal("expected true")
	}
	if ctx.GetString("first_file") != "a.go" {
		t.Errorf("first_file = %q", ctx.GetString("first_file"))
	}
}

func TestExtractFirstResult_WrappedItems(t *testing.T) {
	ctx := NewChainContext(context.Background())
	data := map[string]any{
		"items": []any{map[string]any{"file": "b.go", "name": "Bar"}},
	}
	if !extractFirstResult(ctx, data) {
		t.Fatal("expected true")
	}
	if ctx.GetString("first_file") != "b.go" {
		t.Errorf("first_file = %q", ctx.GetString("first_file"))
	}
}

func TestExtractFirstResult_SliceAny(t *testing.T) {
	ctx := NewChainContext(context.Background())
	data := []any{map[string]any{"file": "c.go", "name": "Baz"}}
	if !extractFirstResult(ctx, data) {
		t.Fatal("expected true")
	}
	if ctx.GetString("first_file") != "c.go" {
		t.Errorf("first_file = %q", ctx.GetString("first_file"))
	}
}

func TestExtractFirstResult_UnknownFormat(t *testing.T) {
	ctx := NewChainContext(context.Background())
	if extractFirstResult(ctx, "just a string") {
		t.Error("expected false for unknown format")
	}
}
