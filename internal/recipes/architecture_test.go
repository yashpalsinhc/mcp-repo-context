package recipes

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// archMockManager mocks the manager for architecture tests.
type archMockManager struct {
	repoContexts map[string]interface{}
}

func (m *archMockManager) GetContext(repoID string, scope string) (interface{}, error) {
	if m.repoContexts != nil {
		if ctx, ok := m.repoContexts[repoID]; ok {
			return ctx, nil
		}
	}
	return nil, fmt.Errorf("repo not found: %s", repoID)
}
func (m *archMockManager) ListRepos() []string { return []string{} }
func (m *archMockManager) SearchContext(query string, repoID string) ([]interface{}, error) {
	return []interface{}{}, nil
}
func (m *archMockManager) GetFunctionContext(repoID string, filePath string, functionName string) (interface{}, error) {
	return nil, nil
}
func (m *archMockManager) GetCallers(repoID string, functionName string) ([]interface{}, error) {
	return []interface{}{}, nil
}
func (m *archMockManager) GetPRContext(repoID string, changedFiles []interface{}) (interface{}, error) {
	return map[string]interface{}{}, nil
}

// archMockOrgManager returns configured org data.
type archMockOrgManager struct {
	orgs map[string]interface{}
}

func (m *archMockOrgManager) GetOrg(orgID string) (interface{}, error) {
	if m.orgs != nil {
		if org, ok := m.orgs[orgID]; ok {
			return org, nil
		}
	}
	return nil, fmt.Errorf("org not found: %s", orgID)
}

func (m *archMockOrgManager) ListOrgs() []string { return []string{} }

func makeRepoContext(name, framework, lang string, fileCount, funcCount, testFileCount int) interface{} {
	return map[string]interface{}{
		"id": name,
		"statistics": map[string]interface{}{
			"total_files":    fileCount,
			"function_count": funcCount,
			"test_file_count": testFileCount,
			"languages": map[string]interface{}{
				lang: fileCount,
			},
		},
		"architecture": map[string]interface{}{
			"dependencies": []interface{}{framework},
		},
		"files": map[string]interface{}{},
	}
}

func TestArchSingleRepoReturnsServiceInventory(t *testing.T) {
	mgr := &archMockManager{
		repoContexts: map[string]interface{}{
			"test-repo": makeRepoContext("test-repo", "gorilla/mux", "go", 50, 100, 5),
		},
	}

	registry := NewRegistry()
	registry.Register(&ArchitectureRecipe{})
	runner := NewRecipeRunner(mgr, &mockOrgManager{}, WithRegistry(registry))

	input := NewRecipeInput(map[string]any{
		"repo_ids": []string{"test-repo"},
	})

	result, err := runner.Execute(context.Background(), "review_architecture", input)
	require.NoError(t, err)

	services, ok := result.Data["services"].([]ServiceInfo)
	require.True(t, ok)
	assert.Len(t, services, 1)
	assert.Equal(t, "test-repo", services[0].RepoID)
	assert.Equal(t, 100, services[0].FunctionCount)
}

func TestArchMultiRepoReturnsComparison(t *testing.T) {
	mgr := &archMockManager{
		repoContexts: map[string]interface{}{
			"repo-1": makeRepoContext("repo-1", "gorilla/mux", "go", 50, 100, 5),
			"repo-2": makeRepoContext("repo-2", "go-chi/chi", "go", 30, 60, 3),
			"repo-3": makeRepoContext("repo-3", "gorilla/mux", "go", 40, 80, 4),
		},
	}

	registry := NewRegistry()
	registry.Register(&ArchitectureRecipe{})
	runner := NewRecipeRunner(mgr, &mockOrgManager{}, WithRegistry(registry))

	input := NewRecipeInput(map[string]any{
		"repo_ids": []string{"repo-1", "repo-2", "repo-3"},
	})

	result, err := runner.Execute(context.Background(), "review_architecture", input)
	require.NoError(t, err)

	services, ok := result.Data["services"].([]ServiceInfo)
	require.True(t, ok)
	assert.Len(t, services, 3)

	// Check patterns detect both gorilla and chi
	patterns, ok := result.Data["shared_patterns"].([]SharedPattern)
	require.True(t, ok)
	assert.True(t, len(patterns) > 0, "should detect patterns")

	// Should detect gorilla/mux used by 2 repos
	found := false
	for _, p := range patterns {
		if p.Value == "gorilla/mux" && p.RepoCount == 2 {
			found = true
		}
	}
	assert.True(t, found, "should detect gorilla/mux in 2 repos")
}

func TestArchWithOrgResolvesRepoList(t *testing.T) {
	mgr := &archMockManager{
		repoContexts: map[string]interface{}{
			"org-repo-1": makeRepoContext("org-repo-1", "gorilla/mux", "go", 50, 100, 5),
			"org-repo-2": makeRepoContext("org-repo-2", "gorilla/mux", "go", 30, 60, 3),
		},
	}

	orgMgr := &archMockOrgManager{
		orgs: map[string]interface{}{
			"test-org": map[string]interface{}{
				"repos": []interface{}{"org-repo-1", "org-repo-2"},
			},
		},
	}

	registry := NewRegistry()
	registry.Register(&ArchitectureRecipe{})
	runner := NewRecipeRunner(mgr, orgMgr, WithRegistry(registry))

	input := NewRecipeInput(map[string]any{
		"org_id": "test-org",
	})

	result, err := runner.Execute(context.Background(), "review_architecture", input)
	require.NoError(t, err)

	services, ok := result.Data["services"].([]ServiceInfo)
	require.True(t, ok)
	assert.Len(t, services, 2)
}

func TestArchNoDependencyDataAddsGapNote(t *testing.T) {
	mgr := &archMockManager{
		repoContexts: map[string]interface{}{
			"test-repo": makeRepoContext("test-repo", "gorilla/mux", "go", 50, 100, 5),
		},
	}

	registry := NewRegistry()
	registry.Register(&ArchitectureRecipe{})
	runner := NewRecipeRunner(mgr, &mockOrgManager{}, WithRegistry(registry))

	input := NewRecipeInput(map[string]any{
		"repo_ids": []string{"test-repo"},
	})

	result, err := runner.Execute(context.Background(), "review_architecture", input)
	require.NoError(t, err)

	hasGap := false
	for _, gap := range result.Gaps {
		if gap.Section == "dependency_graph" {
			hasGap = true
			assert.Contains(t, gap.Reason, "02-dependency-graph")
		}
	}
	assert.True(t, hasGap, "should have dependency_graph gap")
}

func TestArchHealthIndicatorsComputed(t *testing.T) {
	mgr := &archMockManager{
		repoContexts: map[string]interface{}{
			"test-repo": map[string]interface{}{
				"statistics": map[string]interface{}{
					"total_files":     50,
					"function_count":  100,
					"test_file_count": 5,
					"languages":       map[string]interface{}{"go": 50},
				},
				"architecture": map[string]interface{}{
					"dependencies": []interface{}{},
				},
				"files": map[string]interface{}{},
			},
		},
	}

	registry := NewRegistry()
	registry.Register(&ArchitectureRecipe{})
	runner := NewRecipeRunner(mgr, &mockOrgManager{}, WithRegistry(registry))

	input := NewRecipeInput(map[string]any{
		"repo_ids": []string{"test-repo"},
	})

	result, err := runner.Execute(context.Background(), "review_architecture", input)
	require.NoError(t, err)

	healthMap, ok := result.Data["health_indicators"].(map[string]RepoHealth)
	require.True(t, ok)
	health, ok := healthMap["test-repo"]
	require.True(t, ok)
	assert.Equal(t, 5, health.TestFiles)
	assert.InDelta(t, 0.1, health.TestRatio, 0.01)
}

func TestArchAIRecommendationsGenerated(t *testing.T) {
	mgr := &archMockManager{
		repoContexts: map[string]interface{}{
			"test-repo": makeRepoContext("test-repo", "gorilla/mux", "go", 50, 100, 5),
		},
	}

	aiProvider := &mockAIProviderWithResponse{
		response: "Recommendation: Standardize on a single router framework across all services.",
	}

	registry := NewRegistry()
	registry.Register(&ArchitectureRecipe{})
	runner := NewRecipeRunner(mgr, &mockOrgManager{},
		WithRegistry(registry),
		WithAI(aiProvider),
	)

	input := NewRecipeInput(map[string]any{
		"repo_ids": []string{"test-repo"},
	})

	result, err := runner.Execute(context.Background(), "review_architecture", input)
	require.NoError(t, err)
	assert.NotEmpty(t, result.Analysis)
}

func TestArchInputValidationRequiresOrgIDOrRepoIDs(t *testing.T) {
	registry := NewRegistry()
	registry.Register(&ArchitectureRecipe{})
	runner := NewRecipeRunner(&archMockManager{repoContexts: map[string]interface{}{}}, &mockOrgManager{}, WithRegistry(registry))

	input := NewRecipeInput(map[string]any{})

	_, err := runner.Execute(context.Background(), "review_architecture", input)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "org_id or repo_ids")
}

func TestArchFailedRepoDoesntBreakRecipe(t *testing.T) {
	mgr := &archMockManager{
		repoContexts: map[string]interface{}{
			"repo-1": makeRepoContext("repo-1", "gorilla/mux", "go", 50, 100, 5),
			// repo-2 not in map -> will fail
			"repo-3": makeRepoContext("repo-3", "go-chi/chi", "go", 40, 80, 4),
		},
	}

	registry := NewRegistry()
	registry.Register(&ArchitectureRecipe{})
	runner := NewRecipeRunner(mgr, &mockOrgManager{}, WithRegistry(registry))

	input := NewRecipeInput(map[string]any{
		"repo_ids": []string{"repo-1", "repo-2", "repo-3"},
	})

	result, err := runner.Execute(context.Background(), "review_architecture", input)
	require.NoError(t, err)

	services, ok := result.Data["services"].([]ServiceInfo)
	require.True(t, ok)
	assert.Len(t, services, 2, "should have 2 services (1 failed)")

	// Should have gap note for failed repo
	hasGap := false
	for _, gap := range result.Gaps {
		if gap.Section == "service_inventory" {
			hasGap = true
			assert.Contains(t, gap.Reason, "repo-2")
		}
	}
	assert.True(t, hasGap, "should have gap for failed repo")
}
