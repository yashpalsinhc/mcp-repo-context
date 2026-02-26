package recipes

// VectorSearcher defines the interface for vector search operations.
type VectorSearcher interface {
	// SearchFunctions performs semantic search on functions.
	SearchFunctions(query string, repoID string, limit int) ([]SearchResult, error)

	// SearchTypes performs semantic search on types.
	SearchTypes(query string, repoID string, limit int) ([]SearchResult, error)
}

// SearchResult represents a vector search result.
type SearchResult struct {
	Name     string
	Type     string
	FilePath string
	Line     int
	Score    float64
}
