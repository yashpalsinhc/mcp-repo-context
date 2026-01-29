package ai

import (
	"sort"
	"strings"

	ctxpkg "github.com/yashpalc/mcp-repo-context/internal/context"
)

// ContextExtractor extracts relevant context for a query.
type ContextExtractor struct {
	maxFiles     int
	maxFunctions int
	maxTypes     int
	maxTokens    int
}

// NewContextExtractor creates a new context extractor.
func NewContextExtractor() *ContextExtractor {
	return &ContextExtractor{
		maxFiles:     10,
		maxFunctions: 15,
		maxTypes:     10,
		maxTokens:    6000,
	}
}

// WithMaxFiles sets the maximum number of files to include.
func (e *ContextExtractor) WithMaxFiles(n int) *ContextExtractor {
	e.maxFiles = n
	return e
}

// WithMaxFunctions sets the maximum number of functions to include.
func (e *ContextExtractor) WithMaxFunctions(n int) *ContextExtractor {
	e.maxFunctions = n
	return e
}

// WithMaxTokens sets the maximum token budget.
func (e *ContextExtractor) WithMaxTokens(n int) *ContextExtractor {
	e.maxTokens = n
	return e
}

// Extract extracts relevant context for a query from repositories.
func (e *ContextExtractor) Extract(query string, repos []*ctxpkg.RepoContext) *RelevantContext {
	keywords := ExtractKeywords(query)
	queryType := ClassifyQuery(query)

	ctx := &RelevantContext{
		Query:         query,
		RepoSummaries: make([]RepoSummary, 0),
		RelevantFiles: make([]FileSnippet, 0),
		RelevantFuncs: make([]FuncSnippet, 0),
		RelevantTypes: make([]TypeSnippet, 0),
		SearchResults: make([]SearchMatch, 0),
	}

	// Score and collect all candidates
	type scoredFile struct {
		snippet FileSnippet
		score   float64
	}
	type scoredFunc struct {
		snippet FuncSnippet
		score   float64
	}
	type scoredType struct {
		snippet TypeSnippet
		score   float64
	}

	var allFiles []scoredFile
	var allFuncs []scoredFunc
	var allTypes []scoredType

	for _, repo := range repos {
		// Add repo summary
		ctx.RepoSummaries = append(ctx.RepoSummaries, RepoSummary{
			ID:           repo.ID,
			URL:          repo.URL,
			Purpose:      e.getRepoPurpose(repo),
			MainPackages: e.getMainPackages(repo),
			FileCount:    repo.Statistics.TotalFiles,
			Languages:    repo.Statistics.LanguageBreakdown,
		})

		// Score files
		for path, file := range repo.Files {
			score := e.scoreFile(file, path, keywords, queryType)
			if score > 0 {
				exports := make([]string, 0)
				for _, exp := range file.Exports {
					exports = append(exports, exp.Name)
				}
				allFiles = append(allFiles, scoredFile{
					snippet: FileSnippet{
						RepoID:   repo.ID,
						Path:     path,
						Purpose:  file.Purpose,
						Language: file.Language,
						Exports:  exports,
					},
					score: score,
				})
			}

			// Score functions
			for _, fn := range file.Functions {
				score := e.scoreFunction(fn, path, keywords, queryType)
				if score > 0 {
					allFuncs = append(allFuncs, scoredFunc{
						snippet: FuncSnippet{
							RepoID:      repo.ID,
							FilePath:    path,
							Name:        fn.Name,
							Signature:   fn.Signature,
							Description: fn.Description,
							LineStart:   fn.LineStart,
						},
						score: score,
					})
				}
			}

			// Score types
			for _, t := range file.Types {
				score := e.scoreType(t, path, keywords, queryType)
				if score > 0 {
					fields := make([]string, 0)
					for _, f := range t.Fields {
						fields = append(fields, f.Name)
					}
					allTypes = append(allTypes, scoredType{
						snippet: TypeSnippet{
							RepoID:      repo.ID,
							FilePath:    path,
							Name:        t.Name,
							Kind:        t.Kind,
							Description: t.Description,
							Fields:      fields,
							Methods:     t.Methods,
						},
						score: score,
					})
				}
			}
		}
	}

	// Sort by score and take top results
	sort.Slice(allFiles, func(i, j int) bool {
		return allFiles[i].score > allFiles[j].score
	})
	sort.Slice(allFuncs, func(i, j int) bool {
		return allFuncs[i].score > allFuncs[j].score
	})
	sort.Slice(allTypes, func(i, j int) bool {
		return allTypes[i].score > allTypes[j].score
	})

	// Add top results respecting limits
	for i := 0; i < len(allFiles) && i < e.maxFiles; i++ {
		ctx.RelevantFiles = append(ctx.RelevantFiles, allFiles[i].snippet)
	}
	for i := 0; i < len(allFuncs) && i < e.maxFunctions; i++ {
		ctx.RelevantFuncs = append(ctx.RelevantFuncs, allFuncs[i].snippet)
	}
	for i := 0; i < len(allTypes) && i < e.maxTypes; i++ {
		ctx.RelevantTypes = append(ctx.RelevantTypes, allTypes[i].snippet)
	}

	// Add search matches for keywords
	for _, kw := range keywords {
		for _, f := range allFuncs {
			if strings.Contains(strings.ToLower(f.snippet.Name), kw) {
				ctx.SearchResults = append(ctx.SearchResults, SearchMatch{
					RepoID:   f.snippet.RepoID,
					FilePath: f.snippet.FilePath,
					Name:     f.snippet.Name,
					Type:     "function",
					Match:    kw,
					Score:    f.score,
				})
			}
		}
		for _, t := range allTypes {
			if strings.Contains(strings.ToLower(t.snippet.Name), kw) {
				ctx.SearchResults = append(ctx.SearchResults, SearchMatch{
					RepoID:   t.snippet.RepoID,
					FilePath: t.snippet.FilePath,
					Name:     t.snippet.Name,
					Type:     "type",
					Match:    kw,
					Score:    t.score,
				})
			}
		}
	}

	// Limit search results
	if len(ctx.SearchResults) > 20 {
		ctx.SearchResults = ctx.SearchResults[:20]
	}

	return ctx
}

func (e *ContextExtractor) getRepoPurpose(repo *ctxpkg.RepoContext) string {
	if repo.AISummary != nil && repo.AISummary.Purpose != "" {
		return repo.AISummary.Purpose
	}
	if repo.Architecture != nil && repo.Architecture.Overview != "" {
		return repo.Architecture.Overview
	}
	return ""
}

func (e *ContextExtractor) getMainPackages(repo *ctxpkg.RepoContext) []string {
	if repo.Architecture != nil {
		return repo.Architecture.MainPackages
	}
	return nil
}

func (e *ContextExtractor) scoreFile(file *ctxpkg.FileContext, path string, keywords []string, queryType QueryType) float64 {
	score := 0.0

	pathLower := strings.ToLower(path)
	purposeLower := strings.ToLower(file.Purpose)

	// Keyword matching
	for _, kw := range keywords {
		if strings.Contains(pathLower, kw) {
			score += 2.0
		}
		if strings.Contains(purposeLower, kw) {
			score += 1.5
		}
		for _, exp := range file.Exports {
			if strings.Contains(strings.ToLower(exp.Name), kw) {
				score += 1.0
			}
		}
	}

	// Query type bonuses
	switch queryType {
	case QueryTypeArchitecture:
		// Prefer main files, configs
		if strings.Contains(pathLower, "main") {
			score += 2.0
		}
		if strings.Contains(pathLower, "config") || strings.Contains(pathLower, "server") {
			score += 1.5
		}
	case QueryTypeSearch:
		// Just keyword matching is fine
	case QueryTypeExplain:
		// Prefer files with more exports (main logic)
		score += float64(len(file.Exports)) * 0.1
	}

	// Bonus for important file types
	if strings.HasSuffix(path, "_test.go") {
		score *= 0.5 // Lower priority for tests unless explicitly searching
	}
	if strings.Contains(pathLower, "vendor/") || strings.Contains(pathLower, "node_modules/") {
		score = 0 // Exclude vendor
	}

	return score
}

func (e *ContextExtractor) scoreFunction(fn ctxpkg.FunctionDef, path string, keywords []string, queryType QueryType) float64 {
	score := 0.0

	nameLower := strings.ToLower(fn.Name)
	descLower := strings.ToLower(fn.Description)
	sigLower := strings.ToLower(fn.Signature)

	// Keyword matching
	for _, kw := range keywords {
		if strings.Contains(nameLower, kw) {
			score += 3.0 // Name match is very important
		}
		if strings.Contains(descLower, kw) {
			score += 1.5
		}
		if strings.Contains(sigLower, kw) {
			score += 1.0
		}
	}

	// Public functions are more important
	if fn.IsPublic {
		score += 1.0
	}

	// Main entry points
	if fn.Name == "main" || fn.Name == "Main" {
		score += 2.0
	}

	// Handler/Server functions for architecture queries
	if queryType == QueryTypeArchitecture {
		if strings.Contains(nameLower, "handler") || strings.Contains(nameLower, "server") ||
			strings.Contains(nameLower, "init") || strings.Contains(nameLower, "new") {
			score += 1.5
		}
	}

	return score
}

func (e *ContextExtractor) scoreType(t ctxpkg.TypeDef, path string, keywords []string, queryType QueryType) float64 {
	score := 0.0

	nameLower := strings.ToLower(t.Name)
	descLower := strings.ToLower(t.Description)

	// Keyword matching
	for _, kw := range keywords {
		if strings.Contains(nameLower, kw) {
			score += 3.0
		}
		if strings.Contains(descLower, kw) {
			score += 1.5
		}
		for _, f := range t.Fields {
			if strings.Contains(strings.ToLower(f.Name), kw) {
				score += 0.5
			}
		}
		for _, m := range t.Methods {
			if strings.Contains(strings.ToLower(m), kw) {
				score += 0.5
			}
		}
	}

	// Public types are more important
	if t.IsPublic {
		score += 1.0
	}

	// Interfaces and structs with many fields/methods
	if t.Kind == "interface" {
		score += 1.0
	}
	if len(t.Methods) > 3 {
		score += 0.5
	}

	return score
}
