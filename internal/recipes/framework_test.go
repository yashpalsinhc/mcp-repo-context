package recipes

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRecipeInputValidateRequired tests validation of required fields.
func TestRecipeInputValidateRequired(t *testing.T) {
	schema := map[string]FieldSpec{
		"repo_id": {
			Name:     "repo_id",
			Type:     "string",
			Required: true,
		},
		"optional": {
			Name:     "optional",
			Type:     "string",
			Required: false,
		},
	}

	input := NewRecipeInput(map[string]any{})
	err := input.Validate(schema)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing required fields")
	assert.Contains(t, err.Error(), "repo_id")
}

// TestRecipeInputGetString tests string retrieval.
func TestRecipeInputGetString(t *testing.T) {
	input := NewRecipeInput(map[string]any{
		"repo_id": "github.com/org/repo",
	})

	assert.Equal(t, "github.com/org/repo", input.GetString("repo_id"))
	assert.Equal(t, "", input.GetString("missing"))
	assert.Equal(t, "", input.GetString("wrong_type"))
}

// TestRecipeInputGetStringSlice tests slice coercion.
func TestRecipeInputGetStringSlice(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]any
		key      string
		expected []string
	}{
		{
			name: "[]string directly",
			input: map[string]any{
				"repos": []string{"a", "b"},
			},
			key:      "repos",
			expected: []string{"a", "b"},
		},
		{
			name: "[]interface{} coercion",
			input: map[string]any{
				"repos": []interface{}{"a", "b"},
			},
			key:      "repos",
			expected: []string{"a", "b"},
		},
		{
			name: "missing key",
			input: map[string]any{},
			key:  "repos",
			expected: []string{},
		},
		{
			name: "single string",
			input: map[string]any{
				"repos": "single",
			},
			key:      "repos",
			expected: []string{"single"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			input := NewRecipeInput(tc.input)
			result := input.GetStringSlice(tc.key)
			assert.Equal(t, tc.expected, result)
		})
	}
}

// TestRecipeInputGetInt tests integer retrieval with default.
func TestRecipeInputGetInt(t *testing.T) {
	input := NewRecipeInput(map[string]any{
		"budget": float64(4000), // JSON gives float64
	})

	assert.Equal(t, 4000, input.GetInt("budget", 8000))
	assert.Equal(t, 8000, input.GetInt("missing", 8000))
}

// TestRecipeInputGetBool tests boolean retrieval with default.
func TestRecipeInputGetBool(t *testing.T) {
	input := NewRecipeInput(map[string]any{
		"include_ai": true,
	})

	assert.True(t, input.GetBool("include_ai", false))
	assert.False(t, input.GetBool("missing", false))
	assert.True(t, input.GetBool("missing", true))
}

// TestRegistryRegisterAndGet tests recipe registration.
func TestRegistryRegisterAndGet(t *testing.T) {
	registry := NewRegistry()
	recipe := &mockRecipe{name: "test_recipe"}

	registry.Register(recipe)

	retrieved, ok := registry.Get("test_recipe")
	require.True(t, ok)
	assert.Equal(t, recipe, retrieved)

	_, ok = registry.Get("unknown")
	assert.False(t, ok)
}

// TestRegistryListReturnsAllRecipes tests listing recipes.
func TestRegistryListReturnsAllRecipes(t *testing.T) {
	registry := NewRegistry()
	recipes := []*mockRecipe{
		{name: "recipe1", desc: "First recipe"},
		{name: "recipe2", desc: "Second recipe"},
		{name: "recipe3", desc: "Third recipe"},
	}

	for _, r := range recipes {
		registry.Register(r)
	}

	list := registry.List()
	assert.Len(t, list, 3)

	names := make(map[string]bool)
	for _, info := range list {
		names[info.Name] = true
	}

	assert.True(t, names["recipe1"])
	assert.True(t, names["recipe2"])
	assert.True(t, names["recipe3"])
}

// TestRecipeRunnerExecuteCallsCorrectRecipe tests recipe execution.
func TestRecipeRunnerExecuteCallsCorrectRecipe(t *testing.T) {
	registry := NewRegistry()
	mockRecipe := &mockRecipe{
		name: "test_recipe",
		schema: map[string]FieldSpec{
			"repo_id": {
				Name:     "repo_id",
				Type:     "string",
				Required: true,
			},
		},
	}
	registry.Register(mockRecipe)

	runner := NewRecipeRunner(&mockManager{}, &mockOrgManager{}, WithRegistry(registry))

	input := NewRecipeInput(map[string]any{
		"repo_id": "github.com/org/repo",
	})

	result, err := runner.Execute(context.Background(), "test_recipe", input)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "test_recipe", result.Recipe)
}

// TestRecipeRunnerExecuteUnknownRecipe tests error handling.
func TestRecipeRunnerExecuteUnknownRecipe(t *testing.T) {
	registry := NewRegistry()
	registry.Register(&mockRecipe{name: "existing"})

	runner := NewRecipeRunner(&mockManager{}, &mockOrgManager{}, WithRegistry(registry))
	input := NewRecipeInput(map[string]any{})

	_, err := runner.Execute(context.Background(), "unknown", input)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "recipe not found")
	assert.Contains(t, err.Error(), "existing")
}

// TestConfidenceScoringAllSteps tests confidence calculation.
func TestConfidenceScoringAllSteps(t *testing.T) {
	tests := []struct {
		name    string
		hasAI   bool
		gaps    int
		minConf float64
		maxConf float64
	}{
		{"All steps, AI available", true, 0, 0.99, 1.01},
		{"No AI available", false, 0, 0.69, 0.71},
		{"One gap, AI available", true, 1, 0.68, 0.72},
		{"One gap, no AI", false, 1, 0.48, 0.52},
		{"Two gaps, both unavailable", false, 2, 0.33, 0.37},
		{"Floors at 0.1", true, 10, 0.09, 0.11},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			conf := CalculateConfidence(tc.hasAI, tc.gaps)
			assert.Greater(t, conf, tc.minConf)
			assert.Less(t, conf, tc.maxConf)
		})
	}
}

// TestProviderInterfaceIncludesCompleteRaw verifies CompleteRaw is on AIProvider.
func TestProviderInterfaceIncludesCompleteRaw(t *testing.T) {
	// This is a compile-time test, but we can verify the interface exists
	var _ AIProvider = &mockAIProvider{}
	// If this compiles, CompleteRaw exists on AIProvider interface
}

// TestVectorSearcherInterface verifies the interface is defined.
func TestVectorSearcherInterface(t *testing.T) {
	var _ VectorSearcher = &mockVectorSearcher{}
	// If this compiles, VectorSearcher interface exists
}

// Test helper types

type mockRecipe struct {
	name   string
	desc   string
	schema map[string]FieldSpec
}

func (m *mockRecipe) Name() string {
	return m.name
}

func (m *mockRecipe) Description() string {
	return m.desc
}

func (m *mockRecipe) InputSchema() map[string]FieldSpec {
	return m.schema
}

func (m *mockRecipe) Run(ctx context.Context, runner *RecipeRunner, input RecipeInput) (*RecipeResult, error) {
	return &RecipeResult{
		Recipe: m.name,
		Data:   map[string]any{},
	}, nil
}

type mockManager struct{}

func (m *mockManager) GetContext(repoID string, scope string) (interface{}, error) {
	return map[string]interface{}{}, nil
}

func (m *mockManager) ListRepos() []string {
	return []string{}
}

func (m *mockManager) SearchContext(query string, repoID string) ([]interface{}, error) {
	return []interface{}{}, nil
}

func (m *mockManager) GetFunctionContext(repoID string, filePath string, functionName string) (interface{}, error) {
	return map[string]interface{}{}, nil
}

func (m *mockManager) GetCallers(repoID string, functionName string) ([]interface{}, error) {
	return []interface{}{}, nil
}

func (m *mockManager) GetPRContext(repoID string, changedFiles []interface{}) (interface{}, error) {
	return map[string]interface{}{}, nil
}

type mockOrgManager struct{}

func (m *mockOrgManager) GetOrg(orgID string) (interface{}, error) {
	return map[string]interface{}{}, nil
}

func (m *mockOrgManager) ListOrgs() []string {
	return []string{}
}

type mockAIProvider struct{}

func (m *mockAIProvider) CompleteRaw(ctx context.Context, prompt string, maxTokens int) (string, int, error) {
	return "response", 100, nil
}

func (m *mockAIProvider) IsConfigured() bool {
	return true
}

type mockVectorSearcher struct{}

func (m *mockVectorSearcher) SearchFunctions(query string, repoID string, limit int) ([]SearchResult, error) {
	return []SearchResult{}, nil
}

func (m *mockVectorSearcher) SearchTypes(query string, repoID string, limit int) ([]SearchResult, error) {
	return []SearchResult{}, nil
}

type mockBudgeter struct{}

func (m *mockBudgeter) Available() int {
	return 8000
}

func (m *mockBudgeter) Reserve(tokens int) bool {
	return true
}

func (m *mockBudgeter) Use(tokens int) bool {
	return true
}

type mockLogger struct{}

func (m *mockLogger) Info(msg string, fields ...interface{})  {}
func (m *mockLogger) Warn(msg string, fields ...interface{})  {}
func (m *mockLogger) Error(msg string, fields ...interface{}) {}
