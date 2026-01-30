package mcp

import (
	"encoding/json"
	"fmt"

	ctxpkg "github.com/yashpalc/mcp-repo-context/internal/context"
)

// Progressive Disclosure Types
// These provide references instead of full content, allowing agents to drill down.

// FunctionSummary provides a compact reference to a function.
// Agents can use DetailRef with get_function_context for full details.
type FunctionSummary struct {
	Name      string `json:"name"`
	Signature string `json:"signature"`
	Summary   string `json:"summary,omitempty"`    // 1-line summary
	File      string `json:"file"`
	Line      int    `json:"line"`
	DetailRef string `json:"detail_ref,omitempty"` // Reference for get_function_context
}

// TypeSummary provides a compact reference to a type.
type TypeSummary struct {
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	File      string `json:"file"`
	Line      int    `json:"line"`
	DetailRef string `json:"detail_ref,omitempty"` // Reference for get_context scope=file
}

// FileSummary provides a compact reference to a file.
type FileSummary struct {
	Path          string   `json:"path"`
	Package       string   `json:"package,omitempty"`
	FunctionCount int      `json:"function_count"`
	TypeCount     int      `json:"type_count"`
	Concepts      []string `json:"concepts,omitempty"`
	DetailRef     string   `json:"detail_ref,omitempty"` // Reference for get_context scope=file
}

// SearchResultCompact is a compact search result with progressive disclosure.
type SearchResultCompact struct {
	Query       string            `json:"query"`
	TotalCount  int               `json:"total_count"`
	ReturnedCount int             `json:"returned_count"`
	Functions   []FunctionSummary `json:"functions,omitempty"`
	Types       []TypeSummary     `json:"types,omitempty"`
	Files       []FileSummary     `json:"files,omitempty"`
	TokenEstimate int             `json:"token_estimate"`
	HasMore     bool              `json:"has_more"`
	NextOffset  int               `json:"next_offset,omitempty"`
}

// OutputBudget controls the output size.
type OutputBudget struct {
	MaxResults   int    `json:"max_results"`   // Max items to return
	MaxTokens    int    `json:"max_tokens"`    // Target token limit
	Format       string `json:"format"`        // "minimal" | "summary" | "detailed"
	IncludeRefs  bool   `json:"include_refs"`  // Include detail references
}

// DefaultBudget returns sensible defaults.
func DefaultBudget() OutputBudget {
	return OutputBudget{
		MaxResults:  20,
		MaxTokens:   2000,
		Format:      "summary",
		IncludeRefs: true,
	}
}

// MakeFunctionDetailRef creates a reference string for get_function_context.
func MakeFunctionDetailRef(repoID, filePath, funcName string) string {
	return fmt.Sprintf("func:%s:%s:%s", repoID, filePath, funcName)
}

// MakeFileDetailRef creates a reference string for get_context scope=file.
func MakeFileDetailRef(repoID, filePath string) string {
	return fmt.Sprintf("file:%s:%s", repoID, filePath)
}

// MakeTypeDetailRef creates a reference string for type lookup.
func MakeTypeDetailRef(repoID, filePath, typeName string) string {
	return fmt.Sprintf("type:%s:%s:%s", repoID, filePath, typeName)
}

// ToFunctionSummary converts a FunctionDef to a compact summary.
func ToFunctionSummary(fn *ctxpkg.FunctionDef, repoID, filePath string) FunctionSummary {
	summary := ""
	if fn.Behavior != nil && fn.Behavior.Summary != "" {
		summary = fn.Behavior.Summary
		// Truncate to ~100 chars if needed
		if len(summary) > 100 {
			summary = summary[:97] + "..."
		}
	}

	return FunctionSummary{
		Name:      fn.Name,
		Signature: fn.Signature,
		Summary:   summary,
		File:      filePath,
		Line:      fn.LineStart,
		DetailRef: MakeFunctionDetailRef(repoID, filePath, fn.Name),
	}
}

// ToTypeSummary converts a TypeDef to a compact summary.
func ToTypeSummary(t *ctxpkg.TypeDef, repoID, filePath string) TypeSummary {
	return TypeSummary{
		Name:      t.Name,
		Kind:      t.Kind,
		File:      filePath,
		Line:      t.LineStart,
		DetailRef: MakeTypeDetailRef(repoID, filePath, t.Name),
	}
}

// ToFileSummary converts a FileContext to a compact summary.
func ToFileSummary(fc *ctxpkg.FileContext, repoID, filePath string) FileSummary {
	return FileSummary{
		Path:          filePath,
		Package:       fc.Package,
		FunctionCount: len(fc.Functions),
		TypeCount:     len(fc.Types),
		Concepts:      fc.Concepts,
		DetailRef:     MakeFileDetailRef(repoID, filePath),
	}
}

// EstimateTokens provides a rough token estimate for output size planning.
// Uses ~4 chars per token approximation.
func EstimateTokens(v any) int {
	data, err := json.Marshal(v)
	if err != nil {
		return 0
	}
	return len(data) / 4
}

// TruncateResults limits results to fit within budget.
func TruncateResults(results *SearchResultCompact, budget OutputBudget) {
	if budget.MaxResults <= 0 {
		budget.MaxResults = 20
	}

	// Truncate functions
	if len(results.Functions) > budget.MaxResults {
		results.HasMore = true
		results.NextOffset = budget.MaxResults
		results.Functions = results.Functions[:budget.MaxResults]
	}

	// Truncate types
	if len(results.Types) > budget.MaxResults {
		results.HasMore = true
		if results.NextOffset == 0 {
			results.NextOffset = budget.MaxResults
		}
		results.Types = results.Types[:budget.MaxResults]
	}

	// Truncate files
	if len(results.Files) > budget.MaxResults {
		results.HasMore = true
		if results.NextOffset == 0 {
			results.NextOffset = budget.MaxResults
		}
		results.Files = results.Files[:budget.MaxResults]
	}

	results.ReturnedCount = len(results.Functions) + len(results.Types) + len(results.Files)
	results.TokenEstimate = EstimateTokens(results)
}

// QueryCost represents the expected cost of a query pattern.
type QueryCost struct {
	OutputSize string // tiny, small, medium, large
	AIRequired bool
	Cached     bool
}

// QueryPattern defines a recognizable query pattern with its cost.
type QueryPattern struct {
	Name        string    // Pattern name for logging
	Examples    []string  // Example queries
	Cost        QueryCost // Expected cost
	ToolToUse   string    // Recommended tool
}

// GetQueryPatterns returns all recognized query patterns with costs.
// This helps agents understand what queries are cheap vs expensive.
func GetQueryPatterns() []QueryPattern {
	return []QueryPattern{
		{
			Name:     "function_lookup",
			Examples: []string{"what does X do", "explain function X", "how does X work"},
			Cost:     QueryCost{OutputSize: "medium", AIRequired: false, Cached: true},
			ToolToUse: "smart_query or get_function_context",
		},
		{
			Name:     "callers",
			Examples: []string{"who calls X", "where is X used", "callers of X"},
			Cost:     QueryCost{OutputSize: "small", AIRequired: false, Cached: true},
			ToolToUse: "get_callers or smart_query",
		},
		{
			Name:     "side_effect_search",
			Examples: []string{"find database functions", "show HTTP calls", "list file I/O"},
			Cost:     QueryCost{OutputSize: "medium", AIRequired: false, Cached: true},
			ToolToUse: "search_by_side_effect",
		},
		{
			Name:     "concept_search",
			Examples: []string{"find authentication code", "show validation logic", "where is caching"},
			Cost:     QueryCost{OutputSize: "medium", AIRequired: false, Cached: true},
			ToolToUse: "search_by_concept",
		},
		{
			Name:     "architecture",
			Examples: []string{"project structure", "architecture overview", "how is code organized"},
			Cost:     QueryCost{OutputSize: "large", AIRequired: false, Cached: true},
			ToolToUse: "smart_query or get_context scope=architecture",
		},
		{
			Name:     "general_question",
			Examples: []string{"how does feature X work", "explain the flow", "why is it designed this way"},
			Cost:     QueryCost{OutputSize: "large", AIRequired: true, Cached: false},
			ToolToUse: "ask",
		},
	}
}
