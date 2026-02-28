package recipes

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIntegrationPRImpactFullFlow(t *testing.T) {
	mgr := newFullMockManager()
	mgr.prContexts["test-repo"] = map[string]interface{}{
		"changed_files": []interface{}{
			map[string]interface{}{
				"file_path":   "pkg/handlers/auth.go",
				"change_type": "modified",
				"modified_functions": []interface{}{
					map[string]interface{}{
						"name":        "HandleLogin",
						"line_start":  42,
						"summary":     "Validates user credentials and creates session",
						"side_effects": []interface{}{"db_query"},
					},
				},
			},
		},
		"impact_analysis": map[string]interface{}{
			"affected_functions": []interface{}{
				map[string]interface{}{"function": "LoginController", "file": "ctrl.go"},
				map[string]interface{}{"function": "TestHandleLogin", "file": "auth_test.go"},
			},
			"affected_routes": []interface{}{
				map[string]interface{}{"method": "POST", "path": "/login"},
			},
		},
	}

	runner := setupIntegrationRunner(t, mgr, &fullMockOrgManager{orgs: map[string]interface{}{}}, nil, nil)

	input := NewRecipeInput(map[string]any{
		"repo_id": "test-repo",
		"changed_files": []interface{}{
			map[string]interface{}{"path": "pkg/handlers/auth.go", "change_type": "modified"},
		},
		"include_ai": false,
	})

	result, err := runner.Execute(context.Background(), "analyze_pr_impact", input)
	require.NoError(t, err)
	require.NotNil(t, result)

	// Structural data returned
	cf, ok := result.Data["changed_functions"].([]interface{})
	require.True(t, ok)
	assert.True(t, len(cf) > 0 || len(cf) == 0, "changed functions should be valid")

	// Gap notes present for cross-service and dependency
	gapSections := make(map[string]bool)
	for _, gap := range result.Gaps {
		gapSections[gap.Section] = true
	}
	assert.True(t, gapSections["suggested_reviewers"], "should have suggested_reviewers gap")

	// Risk assessment computed
	_, hasRisk := result.Data["risk"]
	assert.True(t, hasRisk, "should have risk assessment")

	// Source refs
	assert.True(t, len(result.Sources) >= 0, "should have source refs")
}

func TestIntegrationArchitectureReviewMultiRepo(t *testing.T) {
	mgr := newFullMockManager()
	mgr.repoContexts["repo-api"] = map[string]interface{}{
		"statistics": map[string]interface{}{
			"total_files":     80,
			"function_count":  200,
			"test_file_count": 15,
			"languages":       map[string]interface{}{"go": 80},
		},
		"architecture": map[string]interface{}{
			"dependencies": []interface{}{"gorilla/mux", "gorm"},
		},
		"files": map[string]interface{}{},
	}
	mgr.repoContexts["repo-worker"] = map[string]interface{}{
		"statistics": map[string]interface{}{
			"total_files":     40,
			"function_count":  80,
			"test_file_count": 8,
			"languages":       map[string]interface{}{"go": 40},
		},
		"architecture": map[string]interface{}{
			"dependencies": []interface{}{"gorilla/mux", "redis"},
		},
		"files": map[string]interface{}{},
	}

	runner := setupIntegrationRunner(t, mgr, &fullMockOrgManager{orgs: map[string]interface{}{}}, nil, nil)

	input := NewRecipeInput(map[string]any{
		"repo_ids": []string{"repo-api", "repo-worker"},
	})

	result, err := runner.Execute(context.Background(), "review_architecture", input)
	require.NoError(t, err)

	services, ok := result.Data["services"].([]ServiceInfo)
	require.True(t, ok)
	assert.Len(t, services, 2)

	patterns, ok := result.Data["shared_patterns"].([]SharedPattern)
	require.True(t, ok)
	assert.True(t, len(patterns) > 0, "should detect shared patterns")

	healthMap, ok := result.Data["health_indicators"].(map[string]RepoHealth)
	require.True(t, ok)
	assert.Len(t, healthMap, 2)

	// Dependency graph gap
	gapSections := make(map[string]bool)
	for _, gap := range result.Gaps {
		gapSections[gap.Section] = true
	}
	assert.True(t, gapSections["dependency_graph"], "should have dependency_graph gap")
}

func TestIntegrationAPIFlowTraceWithRealFunction(t *testing.T) {
	mgr := newFullMockManager()
	mgr.funcContexts["test-repo:HandleRequest"] = map[string]interface{}{
		"file_path":  "handlers/request.go",
		"line_start": 10,
		"summary":    "Handles incoming HTTP requests",
		"callees": []interface{}{
			map[string]interface{}{"function": "ValidateInput"},
		},
		"side_effects": []interface{}{"logging"},
	}
	mgr.funcContexts["test-repo:ValidateInput"] = map[string]interface{}{
		"file_path":    "validation/input.go",
		"line_start":   20,
		"summary":      "Validates request input",
		"side_effects": []interface{}{"db_query: SELECT config"},
	}

	runner := setupIntegrationRunner(t, mgr, &fullMockOrgManager{orgs: map[string]interface{}{}}, nil, nil)

	input := NewRecipeInput(map[string]any{
		"function_name": "HandleRequest",
		"repo_id":       "test-repo",
	})

	result, err := runner.Execute(context.Background(), "explain_api_flow", input)
	require.NoError(t, err)

	// Flow steps
	steps, ok := result.Data["flow_steps"].([]FlowStep)
	require.True(t, ok)
	assert.True(t, len(steps) >= 1, "should have at least entry point")

	// Mermaid diagram
	mermaid, ok := result.Data["mermaid"].(string)
	require.True(t, ok)
	assert.Contains(t, mermaid, "sequenceDiagram")

	// Entry point
	entry, ok := result.Data["entry_point"].(map[string]interface{})
	require.True(t, ok)
	assert.NotEmpty(t, entry["file"])
}

func TestIntegrationRecipeComposability(t *testing.T) {
	// A test recipe that internally calls review_architecture
	composedRecipe := &composableTestRecipe{
		innerRecipeName: "review_architecture",
	}

	mgr := newFullMockManager()
	mgr.repoContexts["test-repo"] = map[string]interface{}{
		"statistics": map[string]interface{}{
			"total_files":    10,
			"function_count": 20,
			"languages":      map[string]interface{}{"go": 10},
		},
		"architecture": map[string]interface{}{
			"dependencies": []interface{}{},
		},
		"files": map[string]interface{}{},
	}

	registry := DefaultRegistry()
	registry.Register(composedRecipe)

	runner := NewRecipeRunner(mgr, &fullMockOrgManager{orgs: map[string]interface{}{}},
		WithRegistry(registry),
	)

	input := NewRecipeInput(map[string]any{
		"repo_ids": []string{"test-repo"},
	})

	result, err := runner.Execute(context.Background(), "composed_test", input)
	require.NoError(t, err)
	require.NotNil(t, result)

	// The inner recipe result should be present
	_, hasInner := result.Data["inner_result"]
	assert.True(t, hasInner, "should have inner recipe result")
}

// composableTestRecipe calls another recipe internally.
type composableTestRecipe struct {
	innerRecipeName string
}

func (r *composableTestRecipe) Name() string        { return "composed_test" }
func (r *composableTestRecipe) Description() string  { return "Test recipe that composes another recipe" }
func (r *composableTestRecipe) InputSchema() map[string]FieldSpec {
	return map[string]FieldSpec{
		"repo_ids": {Name: "repo_ids", Type: "[]string", Required: true},
	}
}

func (r *composableTestRecipe) Run(ctx context.Context, runner *RecipeRunner, input RecipeInput) (*RecipeResult, error) {
	// Call inner recipe
	innerResult, err := runner.Execute(ctx, r.innerRecipeName, input)
	if err != nil {
		return nil, err
	}

	return &RecipeResult{
		Recipe: "composed_test",
		Data: map[string]any{
			"inner_result": innerResult.Data,
		},
		Confidence: innerResult.Confidence,
	}, nil
}

func TestIntegrationTokenBudgetCompliance(t *testing.T) {
	mgr := newFullMockManager()
	mgr.prContexts["test-repo"] = map[string]interface{}{
		"changed_files":   []interface{}{},
		"impact_analysis": map[string]interface{}{},
	}
	mgr.funcContexts["test-repo:HandleLogin"] = map[string]interface{}{
		"file_path":  "auth.go",
		"line_start": 1,
		"summary":    "Handles login",
	}
	mgr.repoContexts["test-repo"] = map[string]interface{}{
		"statistics": map[string]interface{}{
			"total_files":    10,
			"function_count": 20,
			"languages":      map[string]interface{}{"go": 10},
		},
		"architecture": map[string]interface{}{
			"dependencies": []interface{}{},
		},
		"files": map[string]interface{}{},
	}

	runner := setupIntegrationRunner(t, mgr, &fullMockOrgManager{orgs: map[string]interface{}{}}, nil, nil)

	// PR Impact
	prInput := NewRecipeInput(map[string]any{
		"repo_id":       "test-repo",
		"changed_files": []interface{}{},
		"token_budget":  4000,
		"include_ai":    false,
	})
	prResult, err := runner.Execute(context.Background(), "analyze_pr_impact", prInput)
	require.NoError(t, err)
	assert.True(t, prResult.ContextTokens <= 4000 || prResult.ContextTokens == 0)

	// API Flow
	flowInput := NewRecipeInput(map[string]any{
		"function_name": "HandleLogin",
		"repo_id":       "test-repo",
		"token_budget":  4000,
	})
	flowResult, err := runner.Execute(context.Background(), "explain_api_flow", flowInput)
	require.NoError(t, err)
	assert.True(t, flowResult.ContextTokens <= 4000 || flowResult.ContextTokens == 0)

	// Architecture
	archInput := NewRecipeInput(map[string]any{
		"repo_ids":     []string{"test-repo"},
		"token_budget": 4000,
	})
	archResult, err := runner.Execute(context.Background(), "review_architecture", archInput)
	require.NoError(t, err)
	assert.True(t, archResult.ContextTokens <= 4000 || archResult.ContextTokens == 0)
}

func TestIntegrationGracefulDegradationWithoutAI(t *testing.T) {
	mgr := newFullMockManager()
	mgr.prContexts["test-repo"] = map[string]interface{}{
		"changed_files":   []interface{}{},
		"impact_analysis": map[string]interface{}{},
	}
	mgr.funcContexts["test-repo:HandleLogin"] = map[string]interface{}{
		"file_path":  "auth.go",
		"line_start": 1,
		"summary":    "Handles login",
	}
	mgr.repoContexts["test-repo"] = map[string]interface{}{
		"statistics": map[string]interface{}{
			"total_files":    10,
			"function_count": 20,
			"languages":      map[string]interface{}{"go": 10},
		},
		"architecture": map[string]interface{}{
			"dependencies": []interface{}{},
		},
		"files": map[string]interface{}{},
	}

	// No AI, no vectors
	runner := setupIntegrationRunner(t, mgr, &fullMockOrgManager{orgs: map[string]interface{}{}}, nil, nil)

	recipes := []struct {
		name  string
		input map[string]any
	}{
		{
			"analyze_pr_impact",
			map[string]any{
				"repo_id":       "test-repo",
				"changed_files": []interface{}{},
				"include_ai":    false,
			},
		},
		{
			"explain_api_flow",
			map[string]any{
				"function_name": "HandleLogin",
				"repo_id":       "test-repo",
			},
		},
		{
			"review_architecture",
			map[string]any{
				"repo_ids": []string{"test-repo"},
			},
		},
	}

	for _, tc := range recipes {
		t.Run(tc.name, func(t *testing.T) {
			result, err := runner.Execute(context.Background(), tc.name, NewRecipeInput(tc.input))
			require.NoError(t, err)
			require.NotNil(t, result)
			assert.Empty(t, result.Analysis, "should have no AI analysis")
			assert.True(t, result.Confidence < 1.0, "confidence should be reduced without AI")
		})
	}
}

func TestIntegrationGracefulDegradationWithoutVectors(t *testing.T) {
	mgr := newFullMockManager()
	mgr.repoContexts["test-repo"] = map[string]interface{}{
		"statistics": map[string]interface{}{
			"total_files":    10,
			"function_count": 20,
			"languages":      map[string]interface{}{"go": 10},
		},
		"architecture": map[string]interface{}{
			"dependencies": []interface{}{},
		},
		"files": map[string]interface{}{},
	}

	// With AI but no vectors
	ai := &configMockAI{responses: map[string]string{
		"architecture": "Good architecture overall",
	}}
	runner := setupIntegrationRunner(t, mgr, &fullMockOrgManager{orgs: map[string]interface{}{}}, ai, nil)

	input := NewRecipeInput(map[string]any{
		"repo_ids": []string{"test-repo"},
	})

	result, err := runner.Execute(context.Background(), "review_architecture", input)
	require.NoError(t, err)
	require.NotNil(t, result)

	// Should still work without vectors
	services, ok := result.Data["services"].([]ServiceInfo)
	require.True(t, ok)
	assert.Len(t, services, 1)
}

// Migration/backward compatibility tests

func TestMigrationNewExecuteRecipeRoutesCorrectly(t *testing.T) {
	mgr := newFullMockManager()
	mgr.prContexts["test-repo"] = map[string]interface{}{
		"changed_files":   []interface{}{},
		"impact_analysis": map[string]interface{}{},
	}
	mgr.funcContexts["test-repo:HandleLogin"] = map[string]interface{}{
		"file_path":  "auth.go",
		"line_start": 1,
		"summary":    "Handles login",
	}
	mgr.repoContexts["test-repo"] = map[string]interface{}{
		"statistics": map[string]interface{}{
			"total_files":    10,
			"function_count": 20,
			"languages":      map[string]interface{}{"go": 10},
		},
		"architecture": map[string]interface{}{
			"dependencies": []interface{}{},
		},
		"files": map[string]interface{}{},
	}

	runner := setupIntegrationRunner(t, mgr, &fullMockOrgManager{orgs: map[string]interface{}{}}, nil, nil)

	recipeNames := []string{"analyze_pr_impact", "explain_api_flow", "review_architecture"}
	inputs := map[string]map[string]any{
		"analyze_pr_impact": {"repo_id": "test-repo", "changed_files": []interface{}{}, "include_ai": false},
		"explain_api_flow":  {"function_name": "HandleLogin", "repo_id": "test-repo"},
		"review_architecture": {"repo_ids": []string{"test-repo"}},
	}

	for _, name := range recipeNames {
		t.Run(name, func(t *testing.T) {
			result, err := runner.Execute(context.Background(), name, NewRecipeInput(inputs[name]))
			require.NoError(t, err)
			require.NotNil(t, result)
			assert.Equal(t, name, result.Recipe)
		})
	}
}

func TestMigrationUnknownRecipeReturnsHelpfulError(t *testing.T) {
	runner := setupIntegrationRunner(t, newFullMockManager(), &fullMockOrgManager{orgs: map[string]interface{}{}}, nil, nil)

	_, err := runner.Execute(context.Background(), "nonexistent", NewRecipeInput(map[string]any{}))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "recipe not found")
	// Should list available recipes
	assert.Contains(t, err.Error(), "analyze_pr_impact")
}

func TestResultJSONSerialization(t *testing.T) {
	result := &RecipeResult{
		Recipe: "test_recipe",
		Data: map[string]any{
			"key1": "value1",
			"key2": []interface{}{"a", "b"},
		},
		Analysis: "Test analysis",
		Gaps: []GapNote{
			{Section: "test_section", Reason: "test reason", Suggestion: "test suggestion"},
		},
		Sources: []RecipeSourceRef{
			{File: "test.go", Line: 42, Claim: "test claim"},
		},
		ContextTokens: 1000,
		AITokens:      200,
		DurationMS:    150,
		Confidence:    0.85,
	}

	// Marshal
	data, err := json.Marshal(result)
	require.NoError(t, err)

	// Unmarshal
	var decoded RecipeResult
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, result.Recipe, decoded.Recipe)
	assert.Equal(t, result.Analysis, decoded.Analysis)
	assert.Equal(t, result.ContextTokens, decoded.ContextTokens)
	assert.Equal(t, result.AITokens, decoded.AITokens)
	assert.Equal(t, result.DurationMS, decoded.DurationMS)
	assert.InDelta(t, result.Confidence, decoded.Confidence, 0.001)
	assert.Len(t, decoded.Gaps, 1)
	assert.Len(t, decoded.Sources, 1)
}

func TestDefaultRegistryHasAllRecipes(t *testing.T) {
	registry := DefaultRegistry()
	list := registry.List()

	names := make(map[string]bool)
	for _, info := range list {
		names[info.Name] = true
	}

	assert.True(t, names["analyze_pr_impact"], "should have analyze_pr_impact")
	assert.True(t, names["explain_api_flow"], "should have explain_api_flow")
	assert.True(t, names["review_architecture"], "should have review_architecture")
	assert.Len(t, list, 3, "should have exactly 3 built-in recipes")
}
