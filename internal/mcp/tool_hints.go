package mcp

// ToolHints provides metadata to help AI agents choose and use tools effectively.
// This implements the agent-friendly design patterns from Phase 3.
type ToolHints struct {
	// OutputSize estimates the typical output size
	// "tiny" (<100 tokens), "small" (<500), "medium" (<2000), "large" (>2000)
	OutputSize string `json:"output_size"`

	// CostLevel indicates the computational cost
	// "free" (no AI, instant), "cheap" (cached/indexed), "expensive" (AI call)
	CostLevel string `json:"cost_level"`

	// UseCases describes when to use this tool
	UseCases []string `json:"use_cases,omitempty"`

	// Prerequisites lists tools that should be run first
	Prerequisites []string `json:"prerequisites,omitempty"`

	// ComposableWith lists tools that work well together
	ComposableWith []string `json:"composable_with,omitempty"`

	// RequiresAI indicates if this tool needs AI provider
	RequiresAI bool `json:"requires_ai"`

	// Idempotent indicates if repeated calls return the same result
	Idempotent bool `json:"idempotent"`
}

// ExtendedToolDefinition extends the basic tool definition with hints.
type ExtendedToolDefinition struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"inputSchema"`
	Hints       *ToolHints             `json:"hints,omitempty"`
}

// GetToolHints returns hints for all tools.
// This helps AI agents make informed decisions about tool usage.
func GetToolHints() map[string]*ToolHints {
	return map[string]*ToolHints{
		// Repository analysis tools
		"analyze_repo": {
			OutputSize:     "small",
			CostLevel:      "expensive", // Network + analysis
			UseCases:       []string{"initial setup", "index new repository"},
			Prerequisites:  []string{},
			ComposableWith: []string{"get_context", "search_context"},
			RequiresAI:     false,
			Idempotent:     false,
		},
		"analyze_local": {
			OutputSize:     "small",
			CostLevel:      "cheap", // Local only, no network
			UseCases:       []string{"analyze local codebase", "index project on disk"},
			Prerequisites:  []string{},
			ComposableWith: []string{"smart_query", "get_function_context"},
			RequiresAI:     false,
			Idempotent:     false,
		},

		// Context retrieval tools
		"get_context": {
			OutputSize:     "large",
			CostLevel:      "cheap",
			UseCases:       []string{"get full repo overview", "understand repository structure"},
			Prerequisites:  []string{"analyze_repo OR analyze_local"},
			ComposableWith: []string{"search_context"},
			RequiresAI:     false,
			Idempotent:     true,
		},
		"list_repos": {
			OutputSize:     "small",
			CostLevel:      "free",
			UseCases:       []string{"see available repositories", "check what's analyzed"},
			Prerequisites:  []string{},
			ComposableWith: []string{"get_context"},
			RequiresAI:     false,
			Idempotent:     true,
		},

		// Search tools (no AI required)
		"search_context": {
			OutputSize:     "medium",
			CostLevel:      "free",
			UseCases:       []string{"find functions by name", "search for types"},
			Prerequisites:  []string{"analyze_repo OR analyze_local"},
			ComposableWith: []string{"get_function_context"},
			RequiresAI:     false,
			Idempotent:     true,
		},
		"search_by_concept": {
			OutputSize:     "medium",
			CostLevel:      "free",
			UseCases:       []string{"find authentication code", "locate validation logic", "find handlers"},
			Prerequisites:  []string{"analyze_repo OR analyze_local"},
			ComposableWith: []string{"get_function_context"},
			RequiresAI:     false,
			Idempotent:     true,
		},
		"search_by_side_effect": {
			OutputSize:     "medium",
			CostLevel:      "free",
			UseCases:       []string{"find database queries", "locate HTTP calls", "find file I/O"},
			Prerequisites:  []string{"analyze_repo OR analyze_local"},
			ComposableWith: []string{"get_function_context"},
			RequiresAI:     false,
			Idempotent:     true,
		},
		"get_callers": {
			OutputSize:     "small",
			CostLevel:      "free",
			UseCases:       []string{"find who calls a function", "understand dependencies"},
			Prerequisites:  []string{"analyze_repo OR analyze_local"},
			ComposableWith: []string{"get_function_context"},
			RequiresAI:     false,
			Idempotent:     true,
		},
		"smart_query": {
			OutputSize:     "medium",
			CostLevel:      "free",
			UseCases:       []string{"natural language questions", "quick lookups"},
			Prerequisites:  []string{"analyze_repo OR analyze_local"},
			ComposableWith: []string{"get_function_context"},
			RequiresAI:     false,
			Idempotent:     true,
		},

		// Deep context tools
		"get_function_context": {
			OutputSize:     "medium",
			CostLevel:      "free",
			UseCases:       []string{"understand function behavior", "see what a function does"},
			Prerequisites:  []string{"analyze_repo OR analyze_local"},
			ComposableWith: []string{"get_callers"},
			RequiresAI:     false,
			Idempotent:     true,
		},

		// Comparison tools
		"compare_repos": {
			OutputSize:     "large",
			CostLevel:      "cheap",
			UseCases:       []string{"compare repositories", "find differences"},
			Prerequisites:  []string{"analyze_repo (for each repo)"},
			ComposableWith: []string{"find_duplicates", "find_conflicts"},
			RequiresAI:     false,
			Idempotent:     true,
		},
		"find_duplicates": {
			OutputSize:     "medium",
			CostLevel:      "cheap",
			UseCases:       []string{"find duplicate code", "identify copy-paste"},
			Prerequisites:  []string{"analyze_repo (for each repo)"},
			ComposableWith: []string{"compare_repos"},
			RequiresAI:     false,
			Idempotent:     true,
		},
		"find_conflicts": {
			OutputSize:     "medium",
			CostLevel:      "cheap",
			UseCases:       []string{"find conflicting implementations", "pre-merge analysis"},
			Prerequisites:  []string{"analyze_repo (for each repo)"},
			ComposableWith: []string{"compare_repos"},
			RequiresAI:     false,
			Idempotent:     true,
		},
		"find_gaps": {
			OutputSize:     "medium",
			CostLevel:      "cheap",
			UseCases:       []string{"find missing functionality", "gap analysis"},
			Prerequisites:  []string{"analyze_repo (for each repo)"},
			ComposableWith: []string{"compare_repos"},
			RequiresAI:     false,
			Idempotent:     true,
		},

		// AI-powered tools
		"generate_ai_summary": {
			OutputSize:     "medium",
			CostLevel:      "expensive",
			UseCases:       []string{"get AI-generated overview", "understand unfamiliar repo"},
			Prerequisites:  []string{"analyze_repo"},
			ComposableWith: []string{"generate_ai_arch_analysis"},
			RequiresAI:     true,
			Idempotent:     false,
		},
		"generate_ai_arch_analysis": {
			OutputSize:     "medium",
			CostLevel:      "expensive",
			UseCases:       []string{"get AI architecture analysis", "understand system design"},
			Prerequisites:  []string{"analyze_repo"},
			ComposableWith: []string{"generate_ai_summary"},
			RequiresAI:     true,
			Idempotent:     false,
		},
		"ask": {
			OutputSize:     "medium",
			CostLevel:      "expensive",
			UseCases:       []string{"ask questions about code", "get AI explanations"},
			Prerequisites:  []string{"analyze_repo OR analyze_local"},
			ComposableWith: []string{},
			RequiresAI:     true,
			Idempotent:     false,
		},
		"refresh_ai_context": {
			OutputSize:     "small",
			CostLevel:      "expensive",
			UseCases:       []string{"update AI summaries", "regenerate AI context"},
			Prerequisites:  []string{"analyze_repo"},
			ComposableWith: []string{},
			RequiresAI:     true,
			Idempotent:     false,
		},
		"review_pr": {
			OutputSize:     "large",
			CostLevel:      "expensive",
			UseCases:       []string{"review pull request", "get code review feedback"},
			Prerequisites:  []string{"analyze_repo (optional but recommended)"},
			ComposableWith: []string{"get_pr_context"},
			RequiresAI:     true,
			Idempotent:     false,
		},

		// PR context (no AI)
		"get_pr_context": {
			OutputSize:     "large",
			CostLevel:      "cheap",
			UseCases:       []string{"understand PR changes", "pre-review analysis"},
			Prerequisites:  []string{"analyze_repo"},
			ComposableWith: []string{"review_pr"},
			RequiresAI:     false,
			Idempotent:     true,
		},

		// Incremental update tools
		"refresh_file": {
			OutputSize:     "tiny",
			CostLevel:      "free",
			UseCases:       []string{"update single file after edit", "keep context fresh"},
			Prerequisites:  []string{"analyze_local"},
			ComposableWith: []string{"smart_query"},
			RequiresAI:     false,
			Idempotent:     true,
		},
		"refresh_changed": {
			OutputSize:     "small",
			CostLevel:      "cheap",
			UseCases:       []string{"update all changed files", "sync after refactoring"},
			Prerequisites:  []string{"analyze_local"},
			ComposableWith: []string{"smart_query"},
			RequiresAI:     false,
			Idempotent:     true,
		},

		// Skills
		"list_skills": {
			OutputSize:     "small",
			CostLevel:      "free",
			UseCases:       []string{"see available skills", "find specialized prompts"},
			Prerequisites:  []string{},
			ComposableWith: []string{"get_skill"},
			RequiresAI:     false,
			Idempotent:     true,
		},
		"get_skill": {
			OutputSize:     "medium",
			CostLevel:      "free",
			UseCases:       []string{"get skill prompt", "use specialized guidance"},
			Prerequisites:  []string{},
			ComposableWith: []string{},
			RequiresAI:     false,
			Idempotent:     true,
		},

		// NEW: Semantic search tools (using internal/vectors)
		"semantic_search": {
			OutputSize:     "medium",
			CostLevel:      "cheap",
			UseCases:       []string{"find similar code", "semantic code search", "find conceptually related functions"},
			Prerequisites:  []string{"index_repository"},
			ComposableWith: []string{"get_function_context"},
			RequiresAI:     false,
			Idempotent:     true,
		},
		"index_repository": {
			OutputSize:     "small",
			CostLevel:      "cheap",
			UseCases:       []string{"enable semantic search", "create vector embeddings"},
			Prerequisites:  []string{"analyze_repo OR analyze_local"},
			ComposableWith: []string{"semantic_search"},
			RequiresAI:     false,
			Idempotent:     false,
		},

		// NEW: Token-budgeted context (using internal/tokens)
		"get_context_budgeted": {
			OutputSize:     "medium",
			CostLevel:      "free",
			UseCases:       []string{"get context within token limit", "fit context in prompt", "efficient context retrieval"},
			Prerequisites:  []string{"analyze_repo OR analyze_local"},
			ComposableWith: []string{"ask"},
			RequiresAI:     false,
			Idempotent:     true,
		},

		// NEW: Compose pattern tools (using internal/compose)
		"execute_pattern": {
			OutputSize:     "large",
			CostLevel:      "cheap",
			UseCases:       []string{"automate tool workflows", "chain tool calls", "complex analysis"},
			Prerequisites:  []string{"analyze_repo OR analyze_local"},
			ComposableWith: []string{},
			RequiresAI:     false,
			Idempotent:     false,
		},
		"list_patterns": {
			OutputSize:     "small",
			CostLevel:      "free",
			UseCases:       []string{"see available patterns", "discover workflows"},
			Prerequisites:  []string{},
			ComposableWith: []string{"execute_pattern"},
			RequiresAI:     false,
			Idempotent:     true,
		},
	}
}

// GetToolsByOutputSize groups tools by their output size.
func GetToolsByOutputSize() map[string][]string {
	hints := GetToolHints()
	result := map[string][]string{
		"tiny":   {},
		"small":  {},
		"medium": {},
		"large":  {},
	}

	for name, hint := range hints {
		result[hint.OutputSize] = append(result[hint.OutputSize], name)
	}

	return result
}

// GetFreeTools returns tools that don't require AI (instant, no cost).
func GetFreeTools() []string {
	hints := GetToolHints()
	var result []string

	for name, hint := range hints {
		if hint.CostLevel == "free" {
			result = append(result, name)
		}
	}

	return result
}

// GetToolsForUseCase finds tools suitable for a given use case.
func GetToolsForUseCase(useCase string) []string {
	hints := GetToolHints()
	var result []string

	for name, hint := range hints {
		for _, uc := range hint.UseCases {
			if containsIgnoreCase(uc, useCase) {
				result = append(result, name)
				break
			}
		}
	}

	return result
}

func containsIgnoreCase(s, substr string) bool {
	return len(s) >= len(substr) &&
		(s == substr || len(substr) == 0 ||
			(len(s) > 0 && containsIgnoreCaseHelper(s, substr)))
}

func containsIgnoreCaseHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if equalIgnoreCase(s[i:i+len(substr)], substr) {
			return true
		}
	}
	return false
}

func equalIgnoreCase(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		ca, cb := a[i], b[i]
		if ca >= 'A' && ca <= 'Z' {
			ca += 'a' - 'A'
		}
		if cb >= 'A' && cb <= 'Z' {
			cb += 'a' - 'A'
		}
		if ca != cb {
			return false
		}
	}
	return true
}
