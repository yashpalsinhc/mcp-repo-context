package recipes

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// flowMockManager mocks the manager for API flow tests.
type flowMockManager struct {
	funcContexts map[string]interface{}
}

func (m *flowMockManager) GetContext(repoID string, scope string) (interface{}, error) {
	return map[string]interface{}{}, nil
}
func (m *flowMockManager) ListRepos() []string { return []string{} }
func (m *flowMockManager) SearchContext(query string, repoID string) ([]interface{}, error) {
	return []interface{}{}, nil
}
func (m *flowMockManager) GetFunctionContext(repoID string, filePath string, functionName string) (interface{}, error) {
	if m.funcContexts != nil {
		if ctx, ok := m.funcContexts[functionName]; ok {
			return ctx, nil
		}
	}
	return nil, fmt.Errorf("function not found: %s", functionName)
}
func (m *flowMockManager) GetCallers(repoID string, functionName string) ([]interface{}, error) {
	return []interface{}{}, nil
}
func (m *flowMockManager) GetPRContext(repoID string, changedFiles []interface{}) (interface{}, error) {
	return map[string]interface{}{}, nil
}

func TestAPIFlowKnownFunctionTracesFlow(t *testing.T) {
	mgr := &flowMockManager{
		funcContexts: map[string]interface{}{
			"HandleLogin": map[string]interface{}{
				"file_path":  "auth.go",
				"line_start": 42,
				"summary":    "Handles user login",
				"callees": []interface{}{
					map[string]interface{}{"function": "ValidateCredentials"},
					map[string]interface{}{"function": "GenerateToken"},
				},
			},
			"ValidateCredentials": map[string]interface{}{
				"file_path":    "validate.go",
				"line_start":   10,
				"summary":      "Validates user credentials",
				"side_effects": []interface{}{"db_query: SELECT user"},
			},
			"GenerateToken": map[string]interface{}{
				"file_path":  "token.go",
				"line_start": 20,
				"summary":    "Generates JWT token",
			},
		},
	}

	registry := NewRegistry()
	registry.Register(&APIFlowRecipe{})
	runner := NewRecipeRunner(mgr, &mockOrgManager{}, WithRegistry(registry))

	input := NewRecipeInput(map[string]any{
		"function_name": "HandleLogin",
		"repo_id":       "test-repo",
	})

	result, err := runner.Execute(context.Background(), "explain_api_flow", input)
	require.NoError(t, err)
	require.NotNil(t, result)

	// Check flow steps
	steps, ok := result.Data["flow_steps"].([]FlowStep)
	require.True(t, ok)
	assert.Len(t, steps, 3, "entry + 2 callees")

	// Check entry point
	entry, ok := result.Data["entry_point"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "HandleLogin", entry["function"])
}

func TestAPIFlowUnknownFunctionReturnsError(t *testing.T) {
	mgr := &flowMockManager{
		funcContexts: map[string]interface{}{},
	}

	registry := NewRegistry()
	registry.Register(&APIFlowRecipe{})
	runner := NewRecipeRunner(mgr, &mockOrgManager{}, WithRegistry(registry))

	input := NewRecipeInput(map[string]any{
		"function_name": "NonExistent",
		"repo_id":       "test-repo",
	})

	_, err := runner.Execute(context.Background(), "explain_api_flow", input)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "function not found")
}

func TestAPIFlowEndpointWithoutFlowDataGivesGapNote(t *testing.T) {
	mgr := &flowMockManager{
		funcContexts: map[string]interface{}{},
	}

	registry := NewRegistry()
	registry.Register(&APIFlowRecipe{})
	runner := NewRecipeRunner(mgr, &mockOrgManager{}, WithRegistry(registry))

	input := NewRecipeInput(map[string]any{
		"function_name": "",
		"endpoint":      "POST /login",
		"repo_id":       "test-repo",
	})

	_, err := runner.Execute(context.Background(), "explain_api_flow", input)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "endpoint-to-handler lookup not available")
}

func TestAPIFlowMermaidDiagramGenerated(t *testing.T) {
	mgr := &flowMockManager{
		funcContexts: map[string]interface{}{
			"HandleLogin": map[string]interface{}{
				"file_path":  "auth.go",
				"line_start": 42,
				"summary":    "Handles login",
				"callees": []interface{}{
					map[string]interface{}{"function": "ValidateUser"},
					map[string]interface{}{"function": "CreateSession"},
				},
			},
			"ValidateUser": map[string]interface{}{
				"file_path":    "validate.go",
				"line_start":   10,
				"summary":      "Validates user",
				"side_effects": []interface{}{"db_query: SELECT user WHERE email=?"},
			},
			"CreateSession": map[string]interface{}{
				"file_path":  "session.go",
				"line_start": 30,
				"summary":    "Creates session",
			},
		},
	}

	registry := NewRegistry()
	registry.Register(&APIFlowRecipe{})
	runner := NewRecipeRunner(mgr, &mockOrgManager{}, WithRegistry(registry))

	input := NewRecipeInput(map[string]any{
		"function_name": "HandleLogin",
		"repo_id":       "test-repo",
	})

	result, err := runner.Execute(context.Background(), "explain_api_flow", input)
	require.NoError(t, err)

	mermaid, ok := result.Data["mermaid"].(string)
	require.True(t, ok)
	assert.Contains(t, mermaid, "sequenceDiagram")
	assert.Contains(t, mermaid, "HandleLogin")
	assert.Contains(t, mermaid, "ValidateUser")
}

func TestAPIFlowDepthLimitPreventsInfiniteLoop(t *testing.T) {
	// Circular: A -> B -> A
	mgr := &flowMockManager{
		funcContexts: map[string]interface{}{
			"FuncA": map[string]interface{}{
				"file_path":  "a.go",
				"line_start": 1,
				"summary":    "Function A",
				"callees":    []interface{}{map[string]interface{}{"function": "FuncB"}},
			},
			"FuncB": map[string]interface{}{
				"file_path":  "b.go",
				"line_start": 1,
				"summary":    "Function B",
				"callees":    []interface{}{map[string]interface{}{"function": "FuncA"}},
			},
		},
	}

	registry := NewRegistry()
	registry.Register(&APIFlowRecipe{})
	runner := NewRecipeRunner(mgr, &mockOrgManager{}, WithRegistry(registry))

	input := NewRecipeInput(map[string]any{
		"function_name": "FuncA",
		"repo_id":       "test-repo",
	})

	result, err := runner.Execute(context.Background(), "explain_api_flow", input)
	require.NoError(t, err)

	steps, ok := result.Data["flow_steps"].([]FlowStep)
	require.True(t, ok)
	// Should have exactly 2 steps (A and B), not infinite
	assert.Len(t, steps, 2)

	// Verify no repeated functions
	seen := make(map[string]bool)
	for _, step := range steps {
		assert.False(t, seen[step.Function], "should not repeat: %s", step.Function)
		seen[step.Function] = true
	}
}

func TestAPIFlowCrossServiceWithoutFlowData(t *testing.T) {
	mgr := &flowMockManager{
		funcContexts: map[string]interface{}{
			"HandleRequest": map[string]interface{}{
				"file_path":  "handler.go",
				"line_start": 1,
				"summary":    "Handles request",
			},
		},
	}

	registry := NewRegistry()
	registry.Register(&APIFlowRecipe{})
	runner := NewRecipeRunner(mgr, &mockOrgManager{}, WithRegistry(registry))

	input := NewRecipeInput(map[string]any{
		"function_name": "HandleRequest",
		"repo_id":       "test-repo",
		"org_id":        "test-org",
	})

	result, err := runner.Execute(context.Background(), "explain_api_flow", input)
	require.NoError(t, err)

	hasGap := false
	for _, gap := range result.Gaps {
		if gap.Section == "cross_service_hops" {
			hasGap = true
			assert.Contains(t, gap.Reason, "03-api-flow-tracing")
		}
	}
	assert.True(t, hasGap, "should have cross_service_hops gap")
}

func TestAPIFlowAIExplanationGenerated(t *testing.T) {
	mgr := &flowMockManager{
		funcContexts: map[string]interface{}{
			"HandleLogin": map[string]interface{}{
				"file_path":  "auth.go",
				"line_start": 42,
				"summary":    "Handles login",
			},
		},
	}

	aiProvider := &mockAIProviderWithResponse{
		response: "When a request hits HandleLogin, it validates the user credentials and generates a JWT token.",
	}

	registry := NewRegistry()
	registry.Register(&APIFlowRecipe{})
	runner := NewRecipeRunner(mgr, &mockOrgManager{},
		WithRegistry(registry),
		WithAI(aiProvider),
	)

	input := NewRecipeInput(map[string]any{
		"function_name": "HandleLogin",
		"repo_id":       "test-repo",
	})

	result, err := runner.Execute(context.Background(), "explain_api_flow", input)
	require.NoError(t, err)
	assert.NotEmpty(t, result.Analysis)
	assert.True(t, strings.Contains(result.Analysis, "HandleLogin"))
}

func TestAPIFlowWithoutAIStructuralDataOnly(t *testing.T) {
	mgr := &flowMockManager{
		funcContexts: map[string]interface{}{
			"HandleLogin": map[string]interface{}{
				"file_path":  "auth.go",
				"line_start": 42,
				"summary":    "Handles login",
			},
		},
	}

	registry := NewRegistry()
	registry.Register(&APIFlowRecipe{})
	runner := NewRecipeRunner(mgr, &mockOrgManager{}, WithRegistry(registry))

	input := NewRecipeInput(map[string]any{
		"function_name": "HandleLogin",
		"repo_id":       "test-repo",
	})

	result, err := runner.Execute(context.Background(), "explain_api_flow", input)
	require.NoError(t, err)
	assert.Empty(t, result.Analysis, "should have no AI analysis")
	assert.NotNil(t, result.Data["flow_steps"], "should have flow steps")
	assert.NotNil(t, result.Data["mermaid"], "should have mermaid diagram")
}
