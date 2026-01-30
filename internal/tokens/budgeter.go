package tokens

import (
	"sort"

	ctxpkg "github.com/yashpalc/mcp-repo-context/internal/context"
)

// Budgeter helps fit content within a token budget.
type Budgeter struct {
	counter *TokenCounter
}

// NewBudgeter creates a new token budgeter.
func NewBudgeter() *Budgeter {
	return &Budgeter{
		counter: NewTokenCounter(),
	}
}

// NewBudgeterWithCounter creates a budgeter with a custom counter.
func NewBudgeterWithCounter(counter *TokenCounter) *Budgeter {
	return &Budgeter{
		counter: counter,
	}
}

// ScoredItem represents an item with a relevance score.
type ScoredItem[T any] struct {
	Item      T
	Score     float64
	TokenCost int
}

// BuildFunctionContext selects functions that fit within budget.
// Functions are sorted by score and included until budget is exhausted.
func (b *Budgeter) BuildFunctionContext(functions []ScoredItem[ctxpkg.FunctionDef], budget int) []ctxpkg.FunctionDef {
	if budget <= 0 {
		return nil
	}

	// Sort by score descending
	sorted := make([]ScoredItem[ctxpkg.FunctionDef], len(functions))
	copy(sorted, functions)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Score > sorted[j].Score
	})

	var result []ctxpkg.FunctionDef
	used := 0

	for _, item := range sorted {
		cost := item.TokenCost
		if cost == 0 {
			cost = b.counter.CountJSON(item.Item)
		}

		if used+cost > budget {
			// Try to include a summarized version
			summary := b.SummarizeFunction(item.Item)
			summaryCost := b.counter.CountJSON(summary)
			if used+summaryCost <= budget {
				result = append(result, summary)
				used += summaryCost
			}
			continue
		}

		result = append(result, item.Item)
		used += cost
	}

	return result
}

// BuildFileContext selects files that fit within budget.
func (b *Budgeter) BuildFileContext(files []ScoredItem[*ctxpkg.FileContext], budget int) []*ctxpkg.FileContext {
	if budget <= 0 {
		return nil
	}

	// Sort by score descending
	sorted := make([]ScoredItem[*ctxpkg.FileContext], len(files))
	copy(sorted, files)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Score > sorted[j].Score
	})

	var result []*ctxpkg.FileContext
	used := 0

	for _, item := range sorted {
		cost := item.TokenCost
		if cost == 0 {
			cost = b.counter.CountJSON(item.Item)
		}

		if used+cost > budget {
			continue
		}

		result = append(result, item.Item)
		used += cost
	}

	return result
}

// SummarizeFunction creates a minimal version of a function.
func (b *Budgeter) SummarizeFunction(fn ctxpkg.FunctionDef) ctxpkg.FunctionDef {
	summary := ctxpkg.FunctionDef{
		Name:        fn.Name,
		Signature:   fn.Signature,
		Description: fn.Description,
		IsPublic:    fn.IsPublic,
		LineStart:   fn.LineStart,
		LineEnd:     fn.LineEnd,
	}

	// Keep only the behavior summary, not the full steps
	if fn.Behavior != nil {
		summary.Behavior = &ctxpkg.FunctionBehavior{
			Summary: fn.Behavior.Summary,
		}
	}

	// Keep only side effects, not full API flow
	summary.SideEffects = fn.SideEffects

	return summary
}

// SummarizeFile creates a minimal version of a file context.
func (b *Budgeter) SummarizeFile(fc *ctxpkg.FileContext) *ctxpkg.FileContext {
	summary := &ctxpkg.FileContext{
		Path:     fc.Path,
		Package:  fc.Package,
		Language: fc.Language,
		Purpose:  fc.Purpose,
		Concepts: fc.Concepts,
	}

	// Include only function signatures
	for _, fn := range fc.Functions {
		summary.Functions = append(summary.Functions, ctxpkg.FunctionDef{
			Name:      fn.Name,
			Signature: fn.Signature,
			IsPublic:  fn.IsPublic,
		})
	}

	// Include only type names
	for _, t := range fc.Types {
		summary.Types = append(summary.Types, ctxpkg.TypeDef{
			Name:     t.Name,
			Kind:     t.Kind,
			IsPublic: t.IsPublic,
		})
	}

	return summary
}

// EstimateCost returns the token cost of an item.
func (b *Budgeter) EstimateCost(v any) int {
	return b.counter.CountJSON(v)
}

// FitWithinBudget returns how many items from a slice fit within budget.
func (b *Budgeter) FitWithinBudget(items []any, budget int) int {
	used := 0
	count := 0

	for _, item := range items {
		cost := b.counter.CountJSON(item)
		if used+cost > budget {
			break
		}
		used += cost
		count++
	}

	return count
}
