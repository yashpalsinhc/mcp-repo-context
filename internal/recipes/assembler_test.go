package recipes

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockSearchManager extends mockManager with configurable search results.
type mockSearchManager struct {
	searchResults   []interface{}
	funcContexts    map[string]interface{} // funcName -> context
	repoContext     interface{}
}

func (m *mockSearchManager) GetContext(repoID string, scope string) (interface{}, error) {
	if m.repoContext != nil {
		return m.repoContext, nil
	}
	return map[string]interface{}{"overview": "Test repo overview"}, nil
}

func (m *mockSearchManager) ListRepos() []string { return []string{} }

func (m *mockSearchManager) SearchContext(query string, repoID string) ([]interface{}, error) {
	if m.searchResults != nil {
		return m.searchResults, nil
	}
	return []interface{}{}, nil
}

func (m *mockSearchManager) GetFunctionContext(repoID string, filePath string, functionName string) (interface{}, error) {
	if m.funcContexts != nil {
		if ctx, ok := m.funcContexts[functionName]; ok {
			return ctx, nil
		}
	}
	return nil, nil
}

func (m *mockSearchManager) GetCallers(repoID string, functionName string) ([]interface{}, error) {
	return []interface{}{}, nil
}

func (m *mockSearchManager) GetPRContext(repoID string, changedFiles []interface{}) (interface{}, error) {
	return map[string]interface{}{}, nil
}

// mockVecSearcher implements VectorSearcher for testing.
type mockVecSearcher struct {
	results []SearchResult
}

func (m *mockVecSearcher) SearchFunctions(query string, repoID string, limit int) ([]SearchResult, error) {
	return m.results, nil
}

func (m *mockVecSearcher) SearchTypes(query string, repoID string, limit int) ([]SearchResult, error) {
	return []SearchResult{}, nil
}

func TestAssembleWithAllSourcesAvailable(t *testing.T) {
	mgr := &mockSearchManager{
		searchResults: []interface{}{
			map[string]interface{}{"name": "FuncA", "file_path": "a.go", "summary": "Does A"},
			map[string]interface{}{"name": "FuncB", "file_path": "b.go", "summary": "Does B"},
			map[string]interface{}{"name": "FuncC", "file_path": "c.go", "summary": "Does C"},
			map[string]interface{}{"name": "FuncD", "file_path": "d.go", "summary": "Does D"},
			map[string]interface{}{"name": "FuncE", "file_path": "e.go", "summary": "Does E"},
		},
	}
	vecs := &mockVecSearcher{
		results: []SearchResult{
			{Name: "VecFunc1", Type: "function", FilePath: "v1.go", Score: 0.9},
			{Name: "VecFunc2", Type: "function", FilePath: "v2.go", Score: 0.8},
			{Name: "VecFunc3", Type: "function", FilePath: "v3.go", Score: 0.7},
		},
	}

	assembler := NewContextAssembler(mgr, vecs)
	result, err := assembler.Assemble(context.Background(), AssemblyRequest{
		RepoID:     "test-repo",
		Query:      "test query",
		Budget:     4000,
		Allocation: DefaultBudgetAllocation(),
	})

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, len(result.Items) > 0, "should have items")
	assert.True(t, result.TotalTokens <= 4000, "should respect budget")

	// Check sources
	sources := make(map[string]bool)
	for _, item := range result.Items {
		for _, s := range splitSources(item.Source) {
			sources[s] = true
		}
	}
	assert.True(t, sources["structural"], "should have structural items")
	assert.True(t, sources["vector"], "should have vector items")
}

func TestAssembleWithoutVectorSearch(t *testing.T) {
	mgr := &mockSearchManager{
		searchResults: []interface{}{
			map[string]interface{}{"name": "FuncA", "file_path": "a.go", "summary": "Does A"},
		},
	}

	assembler := NewContextAssembler(mgr, nil) // no vectors
	result, err := assembler.Assemble(context.Background(), AssemblyRequest{
		RepoID:     "test-repo",
		Query:      "test query",
		Budget:     4000,
		Allocation: DefaultBudgetAllocation(),
	})

	require.NoError(t, err)
	require.NotNil(t, result)

	// No vector-tagged items
	for _, item := range result.Items {
		assert.NotEqual(t, "vector", item.Source, "should have no vector-only items")
	}

	// No vector key in breakdown
	_, hasVector := result.PassBreakdown["vector"]
	assert.False(t, hasVector, "should have no vector pass in breakdown")
}

func TestAssembleDeduplicatesAcrossPasses(t *testing.T) {
	mgr := &mockSearchManager{
		searchResults: []interface{}{
			map[string]interface{}{"name": "CreateUser", "file_path": "user.go", "summary": "Creates a user"},
		},
	}
	vecs := &mockVecSearcher{
		results: []SearchResult{
			{Name: "CreateUser", Type: "function", FilePath: "user.go", Score: 0.95},
		},
	}

	assembler := NewContextAssembler(mgr, vecs)
	result, err := assembler.Assemble(context.Background(), AssemblyRequest{
		RepoID:     "test-repo",
		Query:      "CreateUser",
		Budget:     4000,
		Allocation: DefaultBudgetAllocation(),
	})

	require.NoError(t, err)

	// Count how many CreateUser items
	count := 0
	for _, item := range result.Items {
		if item.Name == "CreateUser" {
			count++
			// Source should note both passes
			assert.True(t, len(item.Source) > 0)
		}
	}
	assert.Equal(t, 1, count, "should deduplicate CreateUser to single item")
}

func TestAssembleRespectsBudgetAllocation(t *testing.T) {
	// Create many results to test budget limits
	searchResults := make([]interface{}, 50)
	for i := 0; i < 50; i++ {
		searchResults[i] = map[string]interface{}{
			"name":      "Func" + string(rune('A'+i%26)),
			"file_path": "file" + string(rune('0'+i%10)) + ".go",
			"summary":   "This function does something important and interesting which uses many tokens in its summary text",
		}
	}

	mgr := &mockSearchManager{searchResults: searchResults}
	assembler := NewContextAssembler(mgr, nil)
	result, err := assembler.Assemble(context.Background(), AssemblyRequest{
		RepoID:     "test-repo",
		Query:      "test query",
		Budget:     4000,
		Allocation: BudgetAllocation{0.5, 0.0, 0.3, 0.2},
	})

	require.NoError(t, err)
	assert.True(t, result.TotalTokens <= 4000, "should respect total budget")
}

func TestAssembleWithFiltersNarrowsResults(t *testing.T) {
	mgr := &mockSearchManager{
		funcContexts: map[string]interface{}{
			"HandleLogin": map[string]interface{}{
				"name":      "HandleLogin",
				"file_path": "auth.go",
				"summary":   "Handles login",
			},
		},
	}

	assembler := NewContextAssembler(mgr, nil)
	result, err := assembler.Assemble(context.Background(), AssemblyRequest{
		RepoID: "test-repo",
		Query:  "login",
		Budget: 4000,
		Allocation: DefaultBudgetAllocation(),
		Filters: AssemblyFilters{
			FunctionNames: []string{"HandleLogin"},
		},
	})

	require.NoError(t, err)

	// Should have HandleLogin-related items
	found := false
	for _, item := range result.Items {
		if item.Name == "HandleLogin" {
			found = true
		}
	}
	assert.True(t, found, "should have HandleLogin item")
}

func TestAssembleTagsItemsBySourceType(t *testing.T) {
	mgr := &mockSearchManager{
		searchResults: []interface{}{
			map[string]interface{}{"name": "FuncA", "file_path": "a.go", "summary": "Does A"},
		},
	}

	assembler := NewContextAssembler(mgr, nil)
	result, err := assembler.Assemble(context.Background(), AssemblyRequest{
		RepoID:     "test-repo",
		Query:      "test",
		Budget:     4000,
		Allocation: DefaultBudgetAllocation(),
	})

	require.NoError(t, err)
	for _, item := range result.Items {
		assert.NotEmpty(t, item.Source, "each item should have a source")
		// Source should be one of the valid sources
		validSources := []string{"structural", "vector", "keyword", "metadata"}
		hasValid := false
		for _, vs := range validSources {
			if containsSource(item.Source, vs) {
				hasValid = true
				break
			}
		}
		assert.True(t, hasValid, "source should be valid: %s", item.Source)
	}
}

func TestAssembleEmptyQueryReturnsMetadataOnly(t *testing.T) {
	mgr := &mockSearchManager{
		repoContext: map[string]interface{}{"overview": "Test architecture overview"},
	}

	assembler := NewContextAssembler(mgr, nil)
	result, err := assembler.Assemble(context.Background(), AssemblyRequest{
		RepoID:     "test-repo",
		Query:      "",
		Budget:     4000,
		Allocation: DefaultBudgetAllocation(),
	})

	require.NoError(t, err)
	// With empty query, only metadata pass runs
	for _, item := range result.Items {
		assert.True(t, containsSource(item.Source, "metadata"), "should be metadata source: %s", item.Source)
	}
}

// helpers

func splitSources(source string) []string {
	return splitBy(source, "+")
}

func splitBy(s, sep string) []string {
	if s == "" {
		return nil
	}
	parts := make([]string, 0)
	for _, p := range splitString(s, sep) {
		if p != "" {
			parts = append(parts, p)
		}
	}
	return parts
}

func splitString(s, sep string) []string {
	var result []string
	for {
		idx := indexOf(s, sep)
		if idx < 0 {
			result = append(result, s)
			break
		}
		result = append(result, s[:idx])
		s = s[idx+len(sep):]
	}
	return result
}

func indexOf(s, sep string) int {
	for i := 0; i+len(sep) <= len(s); i++ {
		if s[i:i+len(sep)] == sep {
			return i
		}
	}
	return -1
}

func containsSource(source, target string) bool {
	for _, s := range splitSources(source) {
		if s == target {
			return true
		}
	}
	return false
}
