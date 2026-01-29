package ai

import (
	"context"
	"fmt"
	"strings"
)

// QueryRequest represents a natural language query about repositories.
type QueryRequest struct {
	Query      string   // The natural language question
	RepoIDs    []string // Repos to search (empty = all)
	MaxContext int      // Max tokens for context (default: 6000)
}

// QueryResponse contains the AI's answer.
type QueryResponse struct {
	Answer       string           `json:"answer"`
	Sources      []SourceRef      `json:"sources"`
	Confidence   float64          `json:"confidence"`
	TokensUsed   int              `json:"tokens_used"`
	ContextUsed  []string         `json:"context_used"` // What files/functions were included
}

// SourceRef references a specific location in the code.
type SourceRef struct {
	RepoID   string `json:"repo_id"`
	FilePath string `json:"file_path"`
	Function string `json:"function,omitempty"`
	Line     int    `json:"line,omitempty"`
}

// RelevantContext holds context extracted for a query.
type RelevantContext struct {
	Query           string
	RepoSummaries   []RepoSummary
	RelevantFiles   []FileSnippet
	RelevantFuncs   []FuncSnippet
	RelevantTypes   []TypeSnippet
	SearchResults   []SearchMatch
	TotalTokens     int
}

// RepoSummary is a condensed repository overview.
type RepoSummary struct {
	ID           string
	URL          string
	Purpose      string
	MainPackages []string
	FileCount    int
	Languages    map[string]int
}

// FileSnippet is relevant file information.
type FileSnippet struct {
	RepoID   string
	Path     string
	Purpose  string
	Language string
	Exports  []string
}

// FuncSnippet is relevant function information.
type FuncSnippet struct {
	RepoID      string
	FilePath    string
	Name        string
	Signature   string
	Description string
	LineStart   int
}

// TypeSnippet is relevant type information.
type TypeSnippet struct {
	RepoID      string
	FilePath    string
	Name        string
	Kind        string
	Description string
	Fields      []string
	Methods     []string
}

// SearchMatch represents a search result.
type SearchMatch struct {
	RepoID   string
	FilePath string
	Name     string
	Type     string // function, type, file
	Match    string // What matched
	Score    float64
}

// QueryHandler handles intelligent queries.
type QueryHandler struct {
	provider Provider
}

// NewQueryHandler creates a new query handler.
func NewQueryHandler(provider Provider) *QueryHandler {
	return &QueryHandler{provider: provider}
}

// Answer processes a query with the given context.
func (h *QueryHandler) Answer(ctx context.Context, query string, relevantCtx *RelevantContext) (*QueryResponse, error) {
	if !h.provider.IsConfigured() {
		return nil, ErrProviderNotConfigured
	}

	prompt := h.buildQueryPrompt(query, relevantCtx)

	// Use the provider's complete method (we need to expose it or use GenerateDescription)
	// For now, let's create a custom method
	answer, tokensUsed, err := h.askAI(ctx, prompt)
	if err != nil {
		return nil, fmt.Errorf("AI query failed: %w", err)
	}

	// Extract sources from the context
	sources := h.extractSources(relevantCtx)

	return &QueryResponse{
		Answer:      answer,
		Sources:     sources,
		Confidence:  0.8, // Could be parsed from AI response
		TokensUsed:  tokensUsed,
		ContextUsed: h.getContextList(relevantCtx),
	}, nil
}

func (h *QueryHandler) buildQueryPrompt(query string, ctx *RelevantContext) string {
	var sb strings.Builder

	sb.WriteString("You are a code assistant analyzing repositories. Answer the user's question based ONLY on the provided context.\n\n")
	sb.WriteString("## User Question\n")
	sb.WriteString(query)
	sb.WriteString("\n\n")

	// Add repository summaries
	if len(ctx.RepoSummaries) > 0 {
		sb.WriteString("## Repositories\n\n")
		for _, repo := range ctx.RepoSummaries {
			fmt.Fprintf(&sb, "### %s\n", repo.ID)
			if repo.Purpose != "" {
				fmt.Fprintf(&sb, "Purpose: %s\n", repo.Purpose)
			}
			fmt.Fprintf(&sb, "Files: %d\n", repo.FileCount)
			if len(repo.Languages) > 0 {
				sb.WriteString("Languages: ")
				langs := []string{}
				for l, c := range repo.Languages {
					langs = append(langs, fmt.Sprintf("%s(%d)", l, c))
				}
				sb.WriteString(strings.Join(langs, ", "))
				sb.WriteString("\n")
			}
			sb.WriteString("\n")
		}
	}

	// Add relevant files
	if len(ctx.RelevantFiles) > 0 {
		sb.WriteString("## Relevant Files\n\n")
		for _, f := range ctx.RelevantFiles {
			fmt.Fprintf(&sb, "### %s:%s\n", f.RepoID, f.Path)
			if f.Purpose != "" {
				fmt.Fprintf(&sb, "Purpose: %s\n", f.Purpose)
			}
			if len(f.Exports) > 0 {
				fmt.Fprintf(&sb, "Exports: %s\n", strings.Join(f.Exports, ", "))
			}
			sb.WriteString("\n")
		}
	}

	// Add relevant functions
	if len(ctx.RelevantFuncs) > 0 {
		sb.WriteString("## Relevant Functions\n\n")
		for _, fn := range ctx.RelevantFuncs {
			fmt.Fprintf(&sb, "### %s (in %s:%s)\n", fn.Name, fn.RepoID, fn.FilePath)
			fmt.Fprintf(&sb, "```\n%s\n```\n", fn.Signature)
			if fn.Description != "" {
				fmt.Fprintf(&sb, "%s\n", fn.Description)
			}
			sb.WriteString("\n")
		}
	}

	// Add relevant types
	if len(ctx.RelevantTypes) > 0 {
		sb.WriteString("## Relevant Types\n\n")
		for _, t := range ctx.RelevantTypes {
			fmt.Fprintf(&sb, "### %s (%s in %s:%s)\n", t.Name, t.Kind, t.RepoID, t.FilePath)
			if t.Description != "" {
				fmt.Fprintf(&sb, "%s\n", t.Description)
			}
			if len(t.Fields) > 0 {
				fmt.Fprintf(&sb, "Fields: %s\n", strings.Join(t.Fields, ", "))
			}
			if len(t.Methods) > 0 {
				fmt.Fprintf(&sb, "Methods: %s\n", strings.Join(t.Methods, ", "))
			}
			sb.WriteString("\n")
		}
	}

	// Add search results
	if len(ctx.SearchResults) > 0 {
		sb.WriteString("## Search Matches\n\n")
		for _, m := range ctx.SearchResults {
			fmt.Fprintf(&sb, "- %s `%s` in %s:%s\n", m.Type, m.Name, m.RepoID, m.FilePath)
		}
		sb.WriteString("\n")
	}

	sb.WriteString("## Instructions\n\n")
	sb.WriteString("1. Answer the question directly and concisely based on the context above\n")
	sb.WriteString("2. Reference specific files, functions, or types when relevant\n")
	sb.WriteString("3. If you cannot answer from the provided context, say so\n")
	sb.WriteString("4. Format code references as `repo:path` or `repo:path:function`\n")
	sb.WriteString("5. Keep your answer focused and practical\n\n")
	sb.WriteString("Answer:")

	return sb.String()
}

func (h *QueryHandler) askAI(ctx context.Context, prompt string) (string, int, error) {
	// Type assert to access the complete method
	if ap, ok := h.provider.(*AnthropicProvider); ok {
		return ap.complete(ctx, prompt, 2048)
	}

	// Fallback to using GenerateDescription
	desc, err := h.provider.GenerateDescription(ctx, DescriptionRequest{
		CodeType: "query",
		Content:  prompt,
	})
	return desc, 0, err
}

func (h *QueryHandler) extractSources(ctx *RelevantContext) []SourceRef {
	sources := []SourceRef{}

	for _, f := range ctx.RelevantFiles {
		sources = append(sources, SourceRef{
			RepoID:   f.RepoID,
			FilePath: f.Path,
		})
	}

	for _, fn := range ctx.RelevantFuncs {
		sources = append(sources, SourceRef{
			RepoID:   fn.RepoID,
			FilePath: fn.FilePath,
			Function: fn.Name,
			Line:     fn.LineStart,
		})
	}

	return sources
}

func (h *QueryHandler) getContextList(ctx *RelevantContext) []string {
	list := []string{}

	for _, f := range ctx.RelevantFiles {
		list = append(list, fmt.Sprintf("%s:%s", f.RepoID, f.Path))
	}

	for _, fn := range ctx.RelevantFuncs {
		list = append(list, fmt.Sprintf("%s:%s:%s", fn.RepoID, fn.FilePath, fn.Name))
	}

	return list
}

// ExtractKeywords extracts search keywords from a query.
func ExtractKeywords(query string) []string {
	// Simple keyword extraction - in production, could use NLP
	words := strings.Fields(strings.ToLower(query))
	keywords := []string{}

	// Filter out common words
	stopWords := map[string]bool{
		"what": true, "where": true, "how": true, "why": true, "when": true,
		"is": true, "are": true, "the": true, "a": true, "an": true,
		"in": true, "on": true, "at": true, "to": true, "for": true,
		"of": true, "with": true, "by": true, "from": true, "as": true,
		"this": true, "that": true, "these": true, "those": true,
		"it": true, "its": true, "and": true, "or": true, "but": true,
		"do": true, "does": true, "did": true, "will": true, "would": true,
		"can": true, "could": true, "should": true, "must": true,
		"i": true, "me": true, "my": true, "we": true, "our": true,
		"you": true, "your": true, "they": true, "their": true,
		"find": true, "show": true, "get": true, "list": true, "tell": true,
	}

	for _, word := range words {
		// Clean punctuation
		word = strings.Trim(word, ".,?!;:'\"()[]{}")
		if len(word) > 2 && !stopWords[word] {
			keywords = append(keywords, word)
		}
	}

	return keywords
}

// ClassifyQuery determines the type of query.
type QueryType string

const (
	QueryTypeSearch      QueryType = "search"      // Looking for specific code
	QueryTypeExplain     QueryType = "explain"     // Explain how something works
	QueryTypeCompare     QueryType = "compare"     // Compare repos/implementations
	QueryTypeArchitecture QueryType = "architecture" // Architecture questions
	QueryTypeGeneral     QueryType = "general"     // General questions
)

// ClassifyQuery determines what type of question is being asked.
func ClassifyQuery(query string) QueryType {
	q := strings.ToLower(query)

	// Architecture keywords
	archKeywords := []string{"architecture", "structure", "design", "pattern", "layer", "module", "component", "organized", "layout"}
	for _, kw := range archKeywords {
		if strings.Contains(q, kw) {
			return QueryTypeArchitecture
		}
	}

	// Compare keywords
	compareKeywords := []string{"compare", "difference", "different", "versus", "vs", "between", "similar"}
	for _, kw := range compareKeywords {
		if strings.Contains(q, kw) {
			return QueryTypeCompare
		}
	}

	// Explain keywords
	explainKeywords := []string{"how does", "how do", "explain", "what does", "why does", "describe", "works"}
	for _, kw := range explainKeywords {
		if strings.Contains(q, kw) {
			return QueryTypeExplain
		}
	}

	// Search keywords
	searchKeywords := []string{"where is", "find", "locate", "which file", "search", "looking for"}
	for _, kw := range searchKeywords {
		if strings.Contains(q, kw) {
			return QueryTypeSearch
		}
	}

	return QueryTypeGeneral
}
