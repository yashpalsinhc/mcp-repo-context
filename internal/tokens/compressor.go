package tokens

import (
	"sort"
	"strings"

	ctxpkg "github.com/yashpalc/mcp-repo-context/internal/context"
)

// Compressor provides context compression strategies.
type Compressor struct {
	counter *TokenCounter
}

// NewCompressor creates a new context compressor.
func NewCompressor() *Compressor {
	return &Compressor{
		counter: NewTokenCounter(),
	}
}

// NewCompressorWithCounter creates a compressor with a custom counter.
func NewCompressorWithCounter(counter *TokenCounter) *Compressor {
	return &Compressor{
		counter: counter,
	}
}

// RelevanceScorer scores functions by relevance to a query.
type RelevanceScorer interface {
	Score(fn *ctxpkg.FunctionDef, query string) float64
}

// DefaultScorer implements basic keyword-based scoring.
type DefaultScorer struct{}

// Score calculates relevance based on keyword matching.
func (s DefaultScorer) Score(fn *ctxpkg.FunctionDef, query string) float64 {
	if fn == nil || query == "" {
		return 0
	}

	queryLower := strings.ToLower(query)
	words := strings.Fields(queryLower)
	score := 0.0

	// Name matching (highest weight)
	nameLower := strings.ToLower(fn.Name)
	for _, word := range words {
		if len(word) < 3 {
			continue
		}
		if strings.Contains(nameLower, word) {
			score += 3.0
		}
	}

	// Signature matching
	sigLower := strings.ToLower(fn.Signature)
	for _, word := range words {
		if len(word) < 3 {
			continue
		}
		if strings.Contains(sigLower, word) {
			score += 1.0
		}
	}

	// Description matching
	if fn.Description != "" {
		descLower := strings.ToLower(fn.Description)
		for _, word := range words {
			if len(word) < 3 {
				continue
			}
			if strings.Contains(descLower, word) {
				score += 1.5
			}
		}
	}

	// Behavior summary matching
	if fn.Behavior != nil && fn.Behavior.Summary != "" {
		summaryLower := strings.ToLower(fn.Behavior.Summary)
		for _, word := range words {
			if len(word) < 3 {
				continue
			}
			if strings.Contains(summaryLower, word) {
				score += 2.0
			}
		}
	}

	// Boost public functions
	if fn.IsPublic {
		score *= 1.2
	}

	return score
}

// FilterRelevant returns the most relevant functions within a token budget.
// This is coarse-grained filtering - removes entire functions.
func (c *Compressor) FilterRelevant(query string, funcs []ctxpkg.FunctionDef, budget int) []ctxpkg.FunctionDef {
	if len(funcs) == 0 || budget <= 0 {
		return nil
	}

	scorer := DefaultScorer{}

	// Score all functions
	type scoredFunc struct {
		fn    ctxpkg.FunctionDef
		score float64
	}
	scored := make([]scoredFunc, 0, len(funcs))

	for _, fn := range funcs {
		s := scorer.Score(&fn, query)
		if s > 0 { // Only include functions with some relevance
			scored = append(scored, scoredFunc{fn: fn, score: s})
		}
	}

	// Sort by score descending
	sort.Slice(scored, func(i, j int) bool {
		return scored[i].score > scored[j].score
	})

	// Take functions within budget
	var result []ctxpkg.FunctionDef
	tokens := 0

	for _, sf := range scored {
		funcTokens := c.counter.CountJSON(sf.fn)
		if tokens+funcTokens > budget {
			// Try summarized version
			summarized := c.Summarize(sf.fn)
			summaryTokens := c.counter.CountJSON(summarized)
			if tokens+summaryTokens <= budget {
				result = append(result, summarized)
				tokens += summaryTokens
			}
			continue
		}
		result = append(result, sf.fn)
		tokens += funcTokens
	}

	return result
}

// Summarize creates a compact summary of a function.
// This is fine-grained compression - removes details within a function.
func (c *Compressor) Summarize(fn ctxpkg.FunctionDef) ctxpkg.FunctionDef {
	summary := ctxpkg.FunctionDef{
		Name:        fn.Name,
		Signature:   fn.Signature,
		IsPublic:    fn.IsPublic,
		LineStart:   fn.LineStart,
		SideEffects: fn.SideEffects,
	}

	// Keep only behavior summary
	if fn.Behavior != nil && fn.Behavior.Summary != "" {
		summary.Behavior = &ctxpkg.FunctionBehavior{
			Summary: fn.Behavior.Summary,
		}
	}

	// Keep only top 5 calls
	if len(fn.Calls) > 5 {
		summary.Calls = fn.Calls[:5]
	} else {
		summary.Calls = fn.Calls
	}

	return summary
}

// CompressRepo compresses a repository context to fit within budget.
func (c *Compressor) CompressRepo(repo *ctxpkg.RepoContext, budget int) *ctxpkg.RepoContext {
	if repo == nil || budget <= 0 {
		return nil
	}

	compressed := &ctxpkg.RepoContext{
		ID:          repo.ID,
		URL:         repo.URL,
		Branch:      repo.Branch,
		CommitHash:  repo.CommitHash,
		AnalyzedAt:  repo.AnalyzedAt,
		Statistics:  repo.Statistics,
		Version:     repo.Version,
		Files:       make(map[string]*ctxpkg.FileContext),
	}

	// Keep architecture summary
	if repo.Architecture != nil {
		compressed.Architecture = &ctxpkg.ArchitectureContext{
			Overview:    repo.Architecture.Overview,
			BuildSystem: repo.Architecture.BuildSystem,
		}
	}

	// Keep AI summary if available
	if repo.AISummary != nil {
		compressed.AISummary = &ctxpkg.AISummary{
			Overview: repo.AISummary.Overview,
			Purpose:  repo.AISummary.Purpose,
		}
	}

	// Calculate base tokens
	baseTokens := c.counter.CountJSON(compressed)
	remainingBudget := budget - baseTokens

	if remainingBudget <= 0 {
		return compressed
	}

	// Add files until budget exhausted
	// Sort files by importance (handlers first, then models, then others)
	type fileEntry struct {
		path string
		ctx  *ctxpkg.FileContext
		prio int
	}
	files := make([]fileEntry, 0, len(repo.Files))

	for path, fc := range repo.Files {
		prio := 3 // default
		pathLower := strings.ToLower(path)
		if strings.Contains(pathLower, "handler") || strings.Contains(pathLower, "controller") {
			prio = 1
		} else if strings.Contains(pathLower, "model") || strings.Contains(pathLower, "entity") {
			prio = 2
		}
		files = append(files, fileEntry{path: path, ctx: fc, prio: prio})
	}

	sort.Slice(files, func(i, j int) bool {
		return files[i].prio < files[j].prio
	})

	usedTokens := 0
	for _, entry := range files {
		// Create compressed file
		compressedFile := c.CompressFile(entry.ctx)
		fileTokens := c.counter.CountJSON(compressedFile)

		if usedTokens+fileTokens > remainingBudget {
			break
		}

		compressed.Files[entry.path] = compressedFile
		usedTokens += fileTokens
	}

	return compressed
}

// CompressFile creates a compressed version of a file context.
func (c *Compressor) CompressFile(fc *ctxpkg.FileContext) *ctxpkg.FileContext {
	if fc == nil {
		return nil
	}

	compressed := &ctxpkg.FileContext{
		Path:     fc.Path,
		Package:  fc.Package,
		Language: fc.Language,
		Purpose:  fc.Purpose,
		Concepts: fc.Concepts,
	}

	// Include only public functions with summaries
	for _, fn := range fc.Functions {
		if fn.IsPublic {
			compressed.Functions = append(compressed.Functions, c.Summarize(fn))
		}
	}

	// Include only public types with basic info
	for _, t := range fc.Types {
		if t.IsPublic {
			compressed.Types = append(compressed.Types, ctxpkg.TypeDef{
				Name:        t.Name,
				Kind:        t.Kind,
				Description: t.Description,
				IsPublic:    t.IsPublic,
			})
		}
	}

	return compressed
}

// EstimateTokens returns the token count for any value.
func (c *Compressor) EstimateTokens(v any) int {
	return c.counter.CountJSON(v)
}
