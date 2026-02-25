package compose

import (
	"context"
)

// Pattern represents a pre-defined chain of tool calls.
type Pattern interface {
	// Name returns the pattern name.
	Name() string

	// Description returns what this pattern does.
	Description() string

	// Build creates a chain for this pattern.
	Build(executor ToolExecutor, params map[string]any) *Chain
}

// FindAndExpand finds items then expands details.
type FindAndExpand struct {
	searchTool string
	expandTool string
	idField    string
}

// NewFindAndExpand creates a find-and-expand pattern.
func NewFindAndExpand(searchTool, expandTool, idField string) *FindAndExpand {
	return &FindAndExpand{
		searchTool: searchTool,
		expandTool: expandTool,
		idField:    idField,
	}
}

func (p *FindAndExpand) Name() string {
	return "find_and_expand"
}

func (p *FindAndExpand) Description() string {
	return "Search for items then expand details for top results"
}

func (p *FindAndExpand) Build(executor ToolExecutor, params map[string]any) *Chain {
	chain := NewChain(executor)

	// Step 1: Search
	chain.Add(ToolCall{
		Name:   p.searchTool,
		Params: params,
	})

	// Step 2: Expand first result (conditional)
	chain.AddConditional(
		ToolCall{
			Name:   p.expandTool,
			Params: map[string]any{p.idField: "{{first_result_id}}"},
		},
		func(ctx *ChainContext) bool {
			// Extract first result ID and store for expansion
			lastData := ctx.LastData()
			if lastData == nil {
				return false
			}

			// Try to extract ID from results
			switch results := lastData.(type) {
			case []map[string]any:
				if len(results) > 0 {
					if id, ok := results[0][p.idField]; ok {
						ctx.Set("first_result_id", id)
						return true
					}
				}
			case map[string]any:
				if items, ok := results["items"].([]any); ok && len(items) > 0 {
					if item, ok := items[0].(map[string]any); ok {
						if id, ok := item[p.idField]; ok {
							ctx.Set("first_result_id", id)
							return true
						}
					}
				}
			}
			return false
		},
	)

	return chain
}

// SearchWithContext searches then gets full context for results.
type SearchWithContext struct{}

func NewSearchWithContext() *SearchWithContext {
	return &SearchWithContext{}
}

func (p *SearchWithContext) Name() string {
	return "search_with_context"
}

func (p *SearchWithContext) Description() string {
	return "Search for functions and get detailed context for top results"
}

func (p *SearchWithContext) Build(executor ToolExecutor, params map[string]any) *Chain {
	chain := NewChain(executor)

	// Step 1: Search
	chain.AddWithTransform(
		ToolCall{
			Name:   "search_context",
			Params: params,
		},
		func(ctx *ChainContext, result *ToolResult) error {
			if !result.Success {
				return nil
			}
			// Store search results for later use
			ctx.Set("search_results", result.Data)
			return nil
		},
	)

	// Step 2: Get function context for first result
	chain.AddConditional(
		ToolCall{
			Name: "get_function_context",
			Params: map[string]any{
				"repo_id":       "{{repo_id}}",
				"file_path":     "{{first_file}}",
				"function_name": "{{first_func}}",
			},
		},
		func(ctx *ChainContext) bool {
			results := ctx.LastData()
			if results == nil {
				ctx.Set("_skip_reason", "no search results to expand")
				return false
			}

			// Extract first function info from various result formats
			if extractFirstResult(ctx, results) {
				return true
			}
			ctx.Set("_skip_reason", "could not parse search results: unexpected format")
			return false
		},
	)

	return chain
}

// MultiRepoSearch searches across multiple repositories.
type MultiRepoSearch struct {
	repos []string
}

func NewMultiRepoSearch(repos []string) *MultiRepoSearch {
	return &MultiRepoSearch{repos: repos}
}

func (p *MultiRepoSearch) Name() string {
	return "multi_repo_search"
}

func (p *MultiRepoSearch) Description() string {
	return "Search across multiple repositories and aggregate results"
}

func (p *MultiRepoSearch) Build(executor ToolExecutor, params map[string]any) *Chain {
	chain := NewChain(executor)

	for i, repo := range p.repos {
		repoParams := make(map[string]any)
		for k, v := range params {
			repoParams[k] = v
		}
		repoParams["repo_id"] = repo

		// Add search for each repo
		chain.AddWithTransform(
			ToolCall{
				Name:   "search_context",
				Params: repoParams,
			},
			func(ctx *ChainContext, result *ToolResult) error {
				if !result.Success {
					return nil
				}

				// Aggregate results
				key := "all_results"
				existing, _ := ctx.Get(key)
				var allResults []any
				if existing != nil {
					allResults, _ = existing.([]any)
				}

				if result.Data != nil {
					allResults = append(allResults, map[string]any{
						"repo":    repo,
						"results": result.Data,
					})
				}

				ctx.Set(key, allResults)
				return nil
			},
		)

		// Store index for closure
		_ = i
	}

	return chain
}

// ImpactAnalysis analyzes impact of changes to a function.
type ImpactAnalysis struct{}

func NewImpactAnalysis() *ImpactAnalysis {
	return &ImpactAnalysis{}
}

func (p *ImpactAnalysis) Name() string {
	return "impact_analysis"
}

func (p *ImpactAnalysis) Description() string {
	return "Analyze impact of changes by finding callers and their callers"
}

func (p *ImpactAnalysis) Build(executor ToolExecutor, params map[string]any) *Chain {
	chain := NewChain(executor)

	// If file_path is already provided, store it directly
	if fp, ok := params["file_path"].(string); ok && fp != "" {
		// Step 1: Get function context directly (file_path known)
		chain.AddWithTransform(
			ToolCall{
				Name:   "get_function_context",
				Params: params,
			},
			func(ctx *ChainContext, result *ToolResult) error {
				if result.Success {
					ctx.Set("function_context", result.Data)
				}
				return nil
			},
		)
	} else {
		// Step 0: Search for function to resolve file_path
		chain.AddWithTransform(
			ToolCall{
				Name: "search_context",
				Params: map[string]any{
					"repo_id":     params["repo_id"],
					"query":       params["function_name"],
					"search_type": "function",
				},
			},
			func(ctx *ChainContext, result *ToolResult) error {
				if !result.Success || result.Data == nil {
					return nil
				}
				// Extract file_path from search results
				if extractFirstResult(ctx, result.Data) {
					filePath := ctx.GetString("first_file")
					funcName := ctx.GetString("first_func")
					if filePath != "" {
						ctx.Set("file_path", filePath)
						ctx.Set("function_name", funcName)
						ctx.Set("resolved_file_message", "Selected file: "+filePath)
					}
				}
				return nil
			},
		)

		// Step 1: Get function context using resolved file_path
		chain.AddConditional(
			ToolCall{
				Name: "get_function_context",
				Params: map[string]any{
					"repo_id":       params["repo_id"],
					"file_path":     "{{file_path}}",
					"function_name": "{{function_name}}",
				},
			},
			func(ctx *ChainContext) bool {
				fp := ctx.GetString("file_path")
				if fp == "" {
					ctx.Set("_skip_reason", "could not resolve file_path: function not found in search")
					return false
				}
				return true
			},
		)
	}

	// Step 2: Get callers
	chain.AddWithTransform(
		ToolCall{
			Name: "get_callers",
			Params: map[string]any{
				"repo_id":       params["repo_id"],
				"function_name": params["function_name"],
			},
		},
		func(ctx *ChainContext, result *ToolResult) error {
			if result.Success {
				ctx.Set("direct_callers", result.Data)
			}
			return nil
		},
	)

	// Step 3: Search by concept for related functions
	chain.Add(ToolCall{
		Name: "search_by_concept",
		Params: map[string]any{
			"repo_id": params["repo_id"],
			"concept": params["function_name"],
		},
	})

	return chain
}

// PatternRegistry holds available patterns.
type PatternRegistry struct {
	patterns map[string]Pattern
}

// NewPatternRegistry creates a new pattern registry.
func NewPatternRegistry() *PatternRegistry {
	return &PatternRegistry{
		patterns: make(map[string]Pattern),
	}
}

// Register adds a pattern to the registry.
func (r *PatternRegistry) Register(pattern Pattern) {
	r.patterns[pattern.Name()] = pattern
}

// Get retrieves a pattern by name.
func (r *PatternRegistry) Get(name string) (Pattern, bool) {
	p, ok := r.patterns[name]
	return p, ok
}

// List returns all available patterns.
func (r *PatternRegistry) List() []PatternInfo {
	result := make([]PatternInfo, 0, len(r.patterns))
	for name, p := range r.patterns {
		result = append(result, PatternInfo{
			Name:        name,
			Description: p.Description(),
		})
	}
	return result
}

// PatternInfo describes a pattern.
type PatternInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// DefaultRegistry returns a registry with common patterns.
func DefaultRegistry() *PatternRegistry {
	r := NewPatternRegistry()
	r.Register(NewSearchWithContext())
	r.Register(NewImpactAnalysis())
	r.Register(NewFindAndExpand("search_context", "get_function_context", "name"))
	return r
}

// ExecutePattern runs a pattern with the given parameters.
func ExecutePattern(ctx context.Context, executor ToolExecutor, pattern Pattern, params map[string]any) (*ChainContext, error) {
	chain := pattern.Build(executor, params)
	return chain.Execute(ctx)
}

// extractFirstResult extracts file and function name from various result formats
// and stores them as "first_file" and "first_func" in the chain context.
func extractFirstResult(ctx *ChainContext, results any) bool {
	switch r := results.(type) {
	case []map[string]any:
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
		// Handle single result directly
		if file, ok := r["file"].(string); ok {
			ctx.Set("first_file", file)
			if name, ok := r["name"].(string); ok {
				ctx.Set("first_func", name)
			}
			return true
		}
	case []any:
		// Handle JSON-deserialized arrays
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
	return false
}
