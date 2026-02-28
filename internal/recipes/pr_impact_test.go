package recipes

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// prMockManager mocks the manager for PR impact tests.
type prMockManager struct {
	prContext interface{}
	prErr     error
}

func (m *prMockManager) GetContext(repoID string, scope string) (interface{}, error) {
	return map[string]interface{}{}, nil
}
func (m *prMockManager) ListRepos() []string { return []string{} }
func (m *prMockManager) SearchContext(query string, repoID string) ([]interface{}, error) {
	return []interface{}{}, nil
}
func (m *prMockManager) GetFunctionContext(repoID string, filePath string, functionName string) (interface{}, error) {
	return nil, nil
}
func (m *prMockManager) GetCallers(repoID string, functionName string) ([]interface{}, error) {
	return []interface{}{}, nil
}
func (m *prMockManager) GetPRContext(repoID string, changedFiles []interface{}) (interface{}, error) {
	if m.prErr != nil {
		return nil, m.prErr
	}
	return m.prContext, nil
}

func TestPRImpactBasicSingleFileChange(t *testing.T) {
	mgr := &prMockManager{
		prContext: map[string]interface{}{
			"changed_files": []interface{}{
				map[string]interface{}{
					"file_path":   "auth.go",
					"change_type": "modified",
					"modified_functions": []interface{}{
						map[string]interface{}{"name": "HandleLogin", "line_start": 42, "summary": "Handles login"},
						map[string]interface{}{"name": "ValidateToken", "line_start": 87, "summary": "Validates token"},
					},
				},
			},
			"impact_analysis": map[string]interface{}{
				"affected_functions": []interface{}{
					map[string]interface{}{"function": "LoginController", "file": "ctrl.go"},
					map[string]interface{}{"function": "TestHandleLogin", "file": "auth_test.go"},
					map[string]interface{}{"function": "Middleware", "file": "middleware.go"},
				},
				"affected_routes": []interface{}{},
			},
		},
	}

	registry := NewRegistry()
	registry.Register(&PRImpactRecipe{})
	runner := NewRecipeRunner(mgr, &mockOrgManager{}, WithRegistry(registry))

	input := NewRecipeInput(map[string]any{
		"repo_id": "test-repo",
		"changed_files": []interface{}{
			map[string]interface{}{"path": "auth.go", "change_type": "modified"},
		},
		"include_ai": false,
	})

	result, err := runner.Execute(context.Background(), "analyze_pr_impact", input)
	require.NoError(t, err)
	require.NotNil(t, result)

	// Check changed functions
	cf, ok := result.Data["changed_functions"].([]interface{})
	require.True(t, ok)
	assert.Len(t, cf, 2)

	// Check affected callers
	ac, ok := result.Data["affected_callers"].([]interface{})
	require.True(t, ok)
	assert.Len(t, ac, 3)
}

func TestPRImpactNoChangedFunctions(t *testing.T) {
	mgr := &prMockManager{
		prContext: map[string]interface{}{
			"changed_files":   []interface{}{},
			"impact_analysis": map[string]interface{}{},
		},
	}

	registry := NewRegistry()
	registry.Register(&PRImpactRecipe{})
	runner := NewRecipeRunner(mgr, &mockOrgManager{}, WithRegistry(registry))

	input := NewRecipeInput(map[string]any{
		"repo_id":       "test-repo",
		"changed_files": []interface{}{},
		"include_ai":    false,
	})

	result, err := runner.Execute(context.Background(), "analyze_pr_impact", input)
	require.NoError(t, err)
	require.NotNil(t, result)

	cf, ok := result.Data["changed_functions"].([]interface{})
	require.True(t, ok)
	assert.Len(t, cf, 0)
}

func TestPRImpactWithOrgIDCrossServiceGap(t *testing.T) {
	mgr := &prMockManager{
		prContext: map[string]interface{}{
			"changed_files":   []interface{}{},
			"impact_analysis": map[string]interface{}{},
		},
	}

	registry := NewRegistry()
	registry.Register(&PRImpactRecipe{})
	runner := NewRecipeRunner(mgr, &mockOrgManager{}, WithRegistry(registry))

	input := NewRecipeInput(map[string]any{
		"repo_id":       "test-repo",
		"changed_files": []interface{}{},
		"org_id":        "test-org",
		"include_ai":    false,
	})

	result, err := runner.Execute(context.Background(), "analyze_pr_impact", input)
	require.NoError(t, err)

	// Should have cross_service_impact gap
	hasGap := false
	for _, gap := range result.Gaps {
		if gap.Section == "cross_service_impact" {
			hasGap = true
			assert.Contains(t, gap.Reason, "03-api-flow-tracing")
		}
	}
	assert.True(t, hasGap, "should have cross_service_impact gap")
}

func TestPRImpactWithOrgIDDependencyGap(t *testing.T) {
	mgr := &prMockManager{
		prContext: map[string]interface{}{
			"changed_files":   []interface{}{},
			"impact_analysis": map[string]interface{}{},
		},
	}

	registry := NewRegistry()
	registry.Register(&PRImpactRecipe{})
	runner := NewRecipeRunner(mgr, &mockOrgManager{}, WithRegistry(registry))

	input := NewRecipeInput(map[string]any{
		"repo_id":       "test-repo",
		"changed_files": []interface{}{},
		"org_id":        "test-org",
		"include_ai":    false,
	})

	result, err := runner.Execute(context.Background(), "analyze_pr_impact", input)
	require.NoError(t, err)

	hasGap := false
	for _, gap := range result.Gaps {
		if gap.Section == "dependency_impact" {
			hasGap = true
			assert.Contains(t, gap.Reason, "02-dependency-graph")
		}
	}
	assert.True(t, hasGap, "should have dependency_impact gap")
}

func TestPRImpactAIRiskAssessment(t *testing.T) {
	mgr := &prMockManager{
		prContext: map[string]interface{}{
			"changed_files": []interface{}{
				map[string]interface{}{
					"file_path":   "auth.go",
					"change_type": "modified",
					"modified_functions": []interface{}{
						map[string]interface{}{"name": "HandleLogin", "line_start": 42},
					},
				},
			},
			"impact_analysis": map[string]interface{}{
				"affected_functions": []interface{}{
					map[string]interface{}{"function": "Caller1"},
					map[string]interface{}{"function": "Caller2"},
					map[string]interface{}{"function": "Caller3"},
					map[string]interface{}{"function": "Caller4"},
					map[string]interface{}{"function": "Caller5"},
				},
			},
		},
	}

	aiProvider := &mockAIProviderWithResponse{
		response: "Risk: medium\nReasoning: 5 callers affected with potential side effects",
	}

	registry := NewRegistry()
	registry.Register(&PRImpactRecipe{})
	runner := NewRecipeRunner(mgr, &mockOrgManager{},
		WithRegistry(registry),
		WithAI(aiProvider),
	)

	input := NewRecipeInput(map[string]any{
		"repo_id":       "test-repo",
		"changed_files": []interface{}{},
		"include_ai":    true,
	})

	result, err := runner.Execute(context.Background(), "analyze_pr_impact", input)
	require.NoError(t, err)

	risk, ok := result.Data["risk"].(RecipeRiskAssessment)
	require.True(t, ok)
	assert.Equal(t, "medium", risk.Level)
	assert.Equal(t, 0.8, risk.Confidence)
}

func TestPRImpactHeuristicRiskWithoutAI(t *testing.T) {
	// Create 10 callers
	callers := make([]interface{}, 10)
	for i := range callers {
		callers[i] = map[string]interface{}{"function": "Caller"}
	}

	mgr := &prMockManager{
		prContext: map[string]interface{}{
			"changed_files":   []interface{}{},
			"impact_analysis": map[string]interface{}{
				"affected_functions": callers,
			},
		},
	}

	registry := NewRegistry()
	registry.Register(&PRImpactRecipe{})
	runner := NewRecipeRunner(mgr, &mockOrgManager{}, WithRegistry(registry))

	input := NewRecipeInput(map[string]any{
		"repo_id":       "test-repo",
		"changed_files": []interface{}{},
		"include_ai":    false,
	})

	result, err := runner.Execute(context.Background(), "analyze_pr_impact", input)
	require.NoError(t, err)

	risk, ok := result.Data["risk"].(RecipeRiskAssessment)
	require.True(t, ok)
	assert.Equal(t, 0.5, risk.Confidence)
	// 10 callers * 2 = 20 > 15 -> "high"
	assert.Equal(t, "high", risk.Level)
}

func TestPRImpactConcurrentStepsNoRace(t *testing.T) {
	mgr := &prMockManager{
		prContext: map[string]interface{}{
			"changed_files":   []interface{}{},
			"impact_analysis": map[string]interface{}{},
		},
	}

	registry := NewRegistry()
	registry.Register(&PRImpactRecipe{})
	runner := NewRecipeRunner(mgr, &mockOrgManager{}, WithRegistry(registry))

	input := NewRecipeInput(map[string]any{
		"repo_id":       "test-repo",
		"changed_files": []interface{}{},
		"org_id":        "test-org",
		"include_ai":    false,
	})

	// Run multiple times to increase chance of detecting races
	for i := 0; i < 10; i++ {
		result, err := runner.Execute(context.Background(), "analyze_pr_impact", input)
		require.NoError(t, err)
		require.NotNil(t, result)
	}
}

func TestPRImpactContextCancellationReturnsPartial(t *testing.T) {
	mgr := &prMockManager{
		prContext: map[string]interface{}{
			"changed_files": []interface{}{
				map[string]interface{}{
					"file_path":   "auth.go",
					"modified_functions": []interface{}{
						map[string]interface{}{"name": "HandleLogin"},
					},
				},
			},
			"impact_analysis": map[string]interface{}{},
		},
	}

	registry := NewRegistry()
	registry.Register(&PRImpactRecipe{})
	runner := NewRecipeRunner(mgr, &mockOrgManager{}, WithRegistry(registry))

	// Cancel context immediately after creation
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	input := NewRecipeInput(map[string]any{
		"repo_id":       "test-repo",
		"changed_files": []interface{}{},
		"include_ai":    false,
	})

	result, err := runner.Execute(ctx, "analyze_pr_impact", input)
	require.NoError(t, err)
	require.NotNil(t, result)

	// Step 1 data should be present
	_, ok := result.Data["changed_functions"]
	assert.True(t, ok, "step 1 data should be present")

	// Should have gap notes for cancelled steps
	assert.True(t, len(result.Gaps) > 0, "should have gap notes for cancelled steps")
}

func TestPRImpactInputValidationRejectsMissingRepoID(t *testing.T) {
	registry := NewRegistry()
	registry.Register(&PRImpactRecipe{})
	runner := NewRecipeRunner(&prMockManager{prContext: map[string]interface{}{}}, &mockOrgManager{}, WithRegistry(registry))

	input := NewRecipeInput(map[string]any{
		"changed_files": []interface{}{},
	})

	_, err := runner.Execute(context.Background(), "analyze_pr_impact", input)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "repo_id")
}

func TestPRImpactSourcesPopulated(t *testing.T) {
	mgr := &prMockManager{
		prContext: map[string]interface{}{
			"changed_files": []interface{}{
				map[string]interface{}{
					"file_path":   "auth.go",
					"change_type": "modified",
					"modified_functions": []interface{}{
						map[string]interface{}{"name": "HandleLogin", "line_start": 42},
					},
				},
			},
			"impact_analysis": map[string]interface{}{},
		},
	}

	registry := NewRegistry()
	registry.Register(&PRImpactRecipe{})
	runner := NewRecipeRunner(mgr, &mockOrgManager{}, WithRegistry(registry))

	input := NewRecipeInput(map[string]any{
		"repo_id":       "test-repo",
		"changed_files": []interface{}{},
		"include_ai":    false,
	})

	result, err := runner.Execute(context.Background(), "analyze_pr_impact", input)
	require.NoError(t, err)
	assert.True(t, len(result.Sources) > 0, "should have source refs")
	assert.Equal(t, "auth.go", result.Sources[0].File)
}

// mockAIProviderWithResponse returns a configurable response.
type mockAIProviderWithResponse struct {
	response string
	calls    []string
}

func (m *mockAIProviderWithResponse) CompleteRaw(ctx context.Context, prompt string, maxTokens int) (string, int, error) {
	m.calls = append(m.calls, prompt)
	return m.response, 100, nil
}

func (m *mockAIProviderWithResponse) IsConfigured() bool {
	return true
}
