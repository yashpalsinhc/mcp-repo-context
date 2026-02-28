package recipes

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

// fullMockManager provides rich configurable mock data for integration tests.
type fullMockManager struct {
	repoContexts map[string]interface{}
	funcContexts map[string]interface{} // "repoID:funcName" -> context
	searchResults map[string][]interface{} // "repoID:query" -> results
	prContexts   map[string]interface{} // repoID -> PR context
}

func newFullMockManager() *fullMockManager {
	return &fullMockManager{
		repoContexts:  make(map[string]interface{}),
		funcContexts:   make(map[string]interface{}),
		searchResults:  make(map[string][]interface{}),
		prContexts:     make(map[string]interface{}),
	}
}

func (m *fullMockManager) GetContext(repoID string, scope string) (interface{}, error) {
	if ctx, ok := m.repoContexts[repoID]; ok {
		return ctx, nil
	}
	return nil, fmt.Errorf("repo not found: %s", repoID)
}

func (m *fullMockManager) ListRepos() []string {
	var repos []string
	for k := range m.repoContexts {
		repos = append(repos, k)
	}
	return repos
}

func (m *fullMockManager) SearchContext(query string, repoID string) ([]interface{}, error) {
	key := repoID + ":" + query
	if results, ok := m.searchResults[key]; ok {
		return results, nil
	}
	// Try partial match
	for k, results := range m.searchResults {
		if strings.Contains(k, query) || strings.Contains(k, repoID) {
			return results, nil
		}
	}
	return []interface{}{}, nil
}

func (m *fullMockManager) GetFunctionContext(repoID string, filePath string, functionName string) (interface{}, error) {
	key := repoID + ":" + functionName
	if ctx, ok := m.funcContexts[key]; ok {
		return ctx, nil
	}
	return nil, fmt.Errorf("function not found: %s", functionName)
}

func (m *fullMockManager) GetCallers(repoID string, functionName string) ([]interface{}, error) {
	return []interface{}{}, nil
}

func (m *fullMockManager) GetPRContext(repoID string, changedFiles []interface{}) (interface{}, error) {
	if ctx, ok := m.prContexts[repoID]; ok {
		return ctx, nil
	}
	return map[string]interface{}{
		"changed_files":   []interface{}{},
		"impact_analysis": map[string]interface{}{},
	}, nil
}

// fullMockOrgManager provides configurable org data.
type fullMockOrgManager struct {
	orgs map[string]interface{}
}

func (m *fullMockOrgManager) GetOrg(orgID string) (interface{}, error) {
	if org, ok := m.orgs[orgID]; ok {
		return org, nil
	}
	return nil, fmt.Errorf("org not found: %s", orgID)
}

func (m *fullMockOrgManager) ListOrgs() []string {
	var orgs []string
	for k := range m.orgs {
		orgs = append(orgs, k)
	}
	return orgs
}

// configMockAI records all calls and returns configurable responses.
type configMockAI struct {
	responses map[string]string // prompt substring -> response
	calls     []string
}

func (m *configMockAI) CompleteRaw(ctx context.Context, prompt string, maxTokens int) (string, int, error) {
	m.calls = append(m.calls, prompt)
	for substr, response := range m.responses {
		if strings.Contains(prompt, substr) {
			return response, 100, nil
		}
	}
	return "AI response", 50, nil
}

func (m *configMockAI) IsConfigured() bool { return true }

// configMockVectors returns configurable vector search results.
type configMockVectors struct {
	funcResults map[string][]SearchResult // query -> results
	typeResults map[string][]SearchResult
}

func (m *configMockVectors) SearchFunctions(query string, repoID string, limit int) ([]SearchResult, error) {
	if results, ok := m.funcResults[query]; ok {
		return results, nil
	}
	return []SearchResult{}, nil
}

func (m *configMockVectors) SearchTypes(query string, repoID string, limit int) ([]SearchResult, error) {
	if results, ok := m.typeResults[query]; ok {
		return results, nil
	}
	return []SearchResult{}, nil
}

// setupIntegrationRunner creates a fully configured RecipeRunner for integration tests.
func setupIntegrationRunner(t *testing.T, mgr OrchestratorManager, orgMgr OrgManager, ai AIProvider, vecs VectorSearcher) *RecipeRunner {
	t.Helper()

	opts := []RunnerOption{
		WithRegistry(DefaultRegistry()),
	}

	if ai != nil {
		opts = append(opts, WithAI(ai))
	}
	if vecs != nil {
		opts = append(opts, WithVectors(vecs))
	}

	return NewRecipeRunner(mgr, orgMgr, opts...)
}
