package recipes

// AssemblyRequest specifies what context to assemble.
type AssemblyRequest struct {
	RepoID     string
	Query      string
	Budget     int              // total token budget
	Allocation BudgetAllocation // per-pass allocation
	Filters    AssemblyFilters  // optional narrowing
}

// AssemblyFilters narrows context assembly to specific items.
type AssemblyFilters struct {
	FilePaths     []string // limit to specific files
	FunctionNames []string // limit to specific functions
	Concept       string   // filter by concept
}

// AssembledContext holds the assembled context from all passes.
type AssembledContext struct {
	Items         []ContextItem
	TotalTokens   int
	PassBreakdown map[string]int // "structural": 1500, "vector": 800, etc.
}

// ContextItem is a single piece of context (function, type, or file).
type ContextItem struct {
	Type      string  // "function", "type", "file"
	Name      string
	FilePath  string
	Line      int
	Summary   string  // behavior summary or description
	Details   string  // additional context (signature, side effects, etc.)
	Source    string  // "structural", "vector", "keyword", "metadata"
	Relevance float64 // 0-1 score
	TokenCost int     // estimated tokens for this item
}

// itemKey returns the deduplication key for a ContextItem.
func itemKey(name, filePath string) string {
	return name + "|" + filePath
}

// estimateTokens provides a rough token estimate for a ContextItem.
// Uses 4 chars per token as approximation.
func estimateTokens(item ContextItem) int {
	text := item.Name + " " + item.Summary + " " + item.Details + " " + item.FilePath
	return (len(text) + 3) / 4
}
