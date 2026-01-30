package mcp

import (
	"testing"
)

func TestGetToolHints(t *testing.T) {
	hints := GetToolHints()

	// Verify all expected tools have hints
	expectedTools := []string{
		"analyze_repo", "analyze_local", "get_context", "list_repos",
		"search_context", "search_by_concept", "search_by_side_effect",
		"get_callers", "smart_query", "get_function_context",
		"compare_repos", "find_duplicates", "find_conflicts", "find_gaps",
		"generate_ai_summary", "generate_ai_arch_analysis", "ask",
		"refresh_ai_context", "review_pr", "get_pr_context",
		"refresh_file", "refresh_changed", "list_skills", "get_skill",
	}

	for _, tool := range expectedTools {
		hint, ok := hints[tool]
		if !ok {
			t.Errorf("missing hints for tool: %s", tool)
			continue
		}

		// Verify required fields
		if hint.OutputSize == "" {
			t.Errorf("tool %s missing OutputSize", tool)
		}
		if hint.CostLevel == "" {
			t.Errorf("tool %s missing CostLevel", tool)
		}

		// Verify valid output size
		validSizes := map[string]bool{"tiny": true, "small": true, "medium": true, "large": true}
		if !validSizes[hint.OutputSize] {
			t.Errorf("tool %s has invalid OutputSize: %s", tool, hint.OutputSize)
		}

		// Verify valid cost level
		validCosts := map[string]bool{"free": true, "cheap": true, "expensive": true}
		if !validCosts[hint.CostLevel] {
			t.Errorf("tool %s has invalid CostLevel: %s", tool, hint.CostLevel)
		}
	}
}

func TestGetToolsByOutputSize(t *testing.T) {
	sizes := GetToolsByOutputSize()

	// Verify all size categories exist
	for _, size := range []string{"tiny", "small", "medium", "large"} {
		if _, ok := sizes[size]; !ok {
			t.Errorf("missing size category: %s", size)
		}
	}

	// Verify total count matches hints
	hints := GetToolHints()
	total := 0
	for _, tools := range sizes {
		total += len(tools)
	}

	if total != len(hints) {
		t.Errorf("size categories contain %d tools, but hints has %d", total, len(hints))
	}
}

func TestGetFreeTools(t *testing.T) {
	freeTools := GetFreeTools()

	if len(freeTools) == 0 {
		t.Error("expected at least some free tools")
	}

	// Verify all free tools actually have "free" cost level
	hints := GetToolHints()
	for _, tool := range freeTools {
		hint, ok := hints[tool]
		if !ok {
			t.Errorf("free tool %s not in hints", tool)
			continue
		}
		if hint.CostLevel != "free" {
			t.Errorf("free tool %s has cost level %s", tool, hint.CostLevel)
		}
	}
}

func TestGetToolsForUseCase(t *testing.T) {
	tests := []struct {
		useCase  string
		expected []string
	}{
		{"database", []string{"search_by_side_effect"}},
		{"function", []string{"search_context", "get_function_context"}},
		{"authentication", []string{"search_by_concept"}},
	}

	for _, tc := range tests {
		tools := GetToolsForUseCase(tc.useCase)
		if len(tools) == 0 {
			t.Errorf("no tools found for use case: %s", tc.useCase)
		}
	}
}

func TestToolHintsRequiresAI(t *testing.T) {
	hints := GetToolHints()

	aiTools := []string{"generate_ai_summary", "generate_ai_arch_analysis", "ask", "refresh_ai_context", "review_pr"}
	nonAITools := []string{"search_context", "get_callers", "list_repos", "smart_query"}

	for _, tool := range aiTools {
		hint := hints[tool]
		if hint == nil {
			t.Errorf("missing hint for AI tool: %s", tool)
			continue
		}
		if !hint.RequiresAI {
			t.Errorf("AI tool %s should have RequiresAI=true", tool)
		}
	}

	for _, tool := range nonAITools {
		hint := hints[tool]
		if hint == nil {
			t.Errorf("missing hint for non-AI tool: %s", tool)
			continue
		}
		if hint.RequiresAI {
			t.Errorf("non-AI tool %s should have RequiresAI=false", tool)
		}
	}
}

func TestToolHintsIdempotent(t *testing.T) {
	hints := GetToolHints()

	idempotentTools := []string{"get_context", "list_repos", "search_context", "get_function_context"}
	nonIdempotentTools := []string{"analyze_repo", "generate_ai_summary"}

	for _, tool := range idempotentTools {
		hint := hints[tool]
		if hint == nil {
			t.Errorf("missing hint for idempotent tool: %s", tool)
			continue
		}
		if !hint.Idempotent {
			t.Errorf("tool %s should be idempotent", tool)
		}
	}

	for _, tool := range nonIdempotentTools {
		hint := hints[tool]
		if hint == nil {
			t.Errorf("missing hint for non-idempotent tool: %s", tool)
			continue
		}
		if hint.Idempotent {
			t.Errorf("tool %s should not be idempotent", tool)
		}
	}
}
