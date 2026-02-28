package recipes

import (
	"context"
	"sort"
	"strings"
)

// ContextAssembler combines structural, vector, and keyword search into unified context.
type ContextAssembler struct {
	manager  OrchestratorManager
	vectors  VectorSearcher // optional
}

// NewContextAssembler creates a new ContextAssembler.
func NewContextAssembler(manager OrchestratorManager, vectors VectorSearcher) *ContextAssembler {
	return &ContextAssembler{
		manager: manager,
		vectors: vectors,
	}
}

// Assemble gathers context from multiple passes within the given token budget.
func (a *ContextAssembler) Assemble(ctx context.Context, req AssemblyRequest) (*AssembledContext, error) {
	result := &AssembledContext{
		Items:         []ContextItem{},
		PassBreakdown: make(map[string]int),
	}

	if req.Budget <= 0 {
		req.Budget = 4000
	}

	alloc := req.Allocation
	if a.vectors == nil {
		// Redistribute vector budget: structural +60%, keyword +40%
		extra := alloc.Vector
		alloc.Structural += extra * 0.6
		alloc.Keyword += extra * 0.4
		alloc.Vector = 0
	}

	structBudget := int(float64(req.Budget) * alloc.Structural)
	vectorBudget := int(float64(req.Budget) * alloc.Vector)
	keywordBudget := int(float64(req.Budget) * alloc.Keyword)
	metadataBudget := int(float64(req.Budget) * alloc.Metadata)

	seen := make(map[string]*ContextItem) // dedup key -> item

	// Pass 1: Structural
	structItems := a.structuralPass(ctx, req)
	structTokens := a.selectWithinBudget(structItems, structBudget, seen, "structural")
	result.PassBreakdown["structural"] = structTokens

	// Pass 2: Vector (if available)
	if a.vectors != nil && req.Query != "" {
		vectorItems := a.vectorPass(req)
		vectorTokens := a.selectWithinBudget(vectorItems, vectorBudget, seen, "vector")
		result.PassBreakdown["vector"] = vectorTokens
	}

	// Pass 3: Keyword
	if req.Query != "" {
		keywordItems := a.keywordPass(ctx, req)
		keywordTokens := a.selectWithinBudget(keywordItems, keywordBudget, seen, "keyword")
		result.PassBreakdown["keyword"] = keywordTokens
	}

	// Pass 4: Metadata
	metaItems := a.metadataPass(ctx, req)
	metaTokens := a.selectWithinBudget(metaItems, metadataBudget, seen, "metadata")
	result.PassBreakdown["metadata"] = metaTokens

	// Collect all items, sort by relevance descending
	for _, item := range seen {
		result.Items = append(result.Items, *item)
		result.TotalTokens += item.TokenCost
	}

	sort.Slice(result.Items, func(i, j int) bool {
		return result.Items[i].Relevance > result.Items[j].Relevance
	})

	return result, nil
}

// structuralPass gets function/type contexts from the manager.
func (a *ContextAssembler) structuralPass(ctx context.Context, req AssemblyRequest) []ContextItem {
	var items []ContextItem

	// If specific function names requested, get those
	if len(req.Filters.FunctionNames) > 0 {
		for _, name := range req.Filters.FunctionNames {
			funcCtx, err := a.manager.GetFunctionContext(req.RepoID, "", name)
			if err != nil || funcCtx == nil {
				continue
			}
			items = append(items, contextItemFromInterface(name, funcCtx, "structural", 0.9))
		}
		return items
	}

	// Otherwise search by query
	if req.Query != "" {
		results, err := a.manager.SearchContext(req.Query, req.RepoID)
		if err != nil {
			return items
		}
		for i, r := range results {
			relevance := 1.0 - float64(i)*0.05
			if relevance < 0.1 {
				relevance = 0.1
			}
			items = append(items, contextItemFromInterface("", r, "structural", relevance))
		}
	}

	return items
}

// vectorPass performs semantic search via the VectorSearcher.
func (a *ContextAssembler) vectorPass(req AssemblyRequest) []ContextItem {
	var items []ContextItem

	results, err := a.vectors.SearchFunctions(req.Query, req.RepoID, 20)
	if err != nil {
		return items
	}

	for _, r := range results {
		items = append(items, ContextItem{
			Type:      r.Type,
			Name:      r.Name,
			FilePath:  r.FilePath,
			Line:      r.Line,
			Summary:   "",
			Source:    "vector",
			Relevance: r.Score,
			TokenCost: estimateTokens(ContextItem{Name: r.Name, FilePath: r.FilePath}),
		})
	}

	return items
}

// keywordPass splits query into terms and searches individually.
func (a *ContextAssembler) keywordPass(ctx context.Context, req AssemblyRequest) []ContextItem {
	var items []ContextItem

	terms := strings.Fields(req.Query)
	if len(terms) == 0 {
		return items
	}

	seen := make(map[string]bool)
	for _, term := range terms {
		if len(term) < 2 {
			continue
		}
		results, err := a.manager.SearchContext(term, req.RepoID)
		if err != nil {
			continue
		}
		for i, r := range results {
			item := contextItemFromInterface("", r, "keyword", 0.6-float64(i)*0.05)
			key := itemKey(item.Name, item.FilePath)
			if seen[key] {
				continue
			}
			seen[key] = true
			items = append(items, item)
		}
	}

	return items
}

// metadataPass includes repo architecture summary.
func (a *ContextAssembler) metadataPass(ctx context.Context, req AssemblyRequest) []ContextItem {
	var items []ContextItem

	repoCtx, err := a.manager.GetContext(req.RepoID, "architecture")
	if err != nil || repoCtx == nil {
		return items
	}

	// Extract a summary string from whatever the manager returns
	summary := extractSummaryFromContext(repoCtx)
	if summary != "" {
		item := ContextItem{
			Type:      "metadata",
			Name:      "architecture_overview",
			FilePath:  "",
			Summary:   summary,
			Source:    "metadata",
			Relevance: 0.3,
			TokenCost: (len(summary) + 3) / 4,
		}
		items = append(items, item)
	}

	return items
}

// selectWithinBudget selects items greedily within the given token budget.
// Deduplicates against items already in the seen map.
// Returns total tokens used in this pass.
func (a *ContextAssembler) selectWithinBudget(items []ContextItem, budget int, seen map[string]*ContextItem, passName string) int {
	used := 0
	for i := range items {
		item := &items[i]
		if item.TokenCost == 0 {
			item.TokenCost = estimateTokens(*item)
		}
		if used+item.TokenCost > budget {
			continue
		}

		key := itemKey(item.Name, item.FilePath)
		if existing, ok := seen[key]; ok {
			// Dedup: keep higher relevance, note combined source
			if item.Relevance > existing.Relevance {
				existing.Relevance = item.Relevance
			}
			if !strings.Contains(existing.Source, passName) {
				existing.Source = existing.Source + "+" + passName
			}
			continue
		}

		item.Source = passName
		seen[key] = item
		used += item.TokenCost
	}
	return used
}

// contextItemFromInterface converts a manager search result to a ContextItem.
func contextItemFromInterface(name string, data interface{}, source string, relevance float64) ContextItem {
	item := ContextItem{
		Type:      "function",
		Source:    source,
		Relevance: relevance,
	}

	switch v := data.(type) {
	case map[string]interface{}:
		if n, ok := v["name"].(string); ok && name == "" {
			item.Name = n
		} else {
			item.Name = name
		}
		if fp, ok := v["file_path"].(string); ok {
			item.FilePath = fp
		} else if fp, ok := v["file"].(string); ok {
			item.FilePath = fp
		}
		if s, ok := v["summary"].(string); ok {
			item.Summary = s
		}
		if l, ok := v["line"].(int); ok {
			item.Line = l
		} else if l, ok := v["line_start"].(int); ok {
			item.Line = l
		}
		if t, ok := v["type"].(string); ok {
			item.Type = t
		}
	default:
		item.Name = name
	}

	item.TokenCost = estimateTokens(item)
	return item
}

// extractSummaryFromContext pulls a summary string from a context result.
func extractSummaryFromContext(data interface{}) string {
	switch v := data.(type) {
	case map[string]interface{}:
		if overview, ok := v["overview"].(string); ok {
			return overview
		}
		if summary, ok := v["summary"].(string); ok {
			return summary
		}
	case string:
		return v
	}
	return ""
}
