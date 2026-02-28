package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	ctxpkg "github.com/yashpalc/mcp-repo-context/internal/context"
	"github.com/yashpalc/mcp-repo-context/internal/logging"
	"github.com/yashpalc/mcp-repo-context/internal/orchestrator"
	"github.com/yashpalc/mcp-repo-context/internal/org"
	"github.com/yashpalc/mcp-repo-context/internal/prreview"
)

// --- Mock Manager ---

type mockManager struct {
	repos     []ctxpkg.ContextMetadata
	repoCtx   map[string]*ctxpkg.RepoContext
	analyzeErr error
}

func (m *mockManager) AnalyzeRepo(ctx context.Context, repoURL string, opts orchestrator.AnalyzeOptions) (*orchestrator.AnalyzeResult, error) {
	if m.analyzeErr != nil {
		return nil, m.analyzeErr
	}
	return &orchestrator.AnalyzeResult{RepoID: "test/repo"}, nil
}

func (m *mockManager) GetContext(ctx context.Context, repoID string) (*ctxpkg.RepoContext, error) {
	if rc, ok := m.repoCtx[repoID]; ok {
		return rc, nil
	}
	return nil, fmt.Errorf("context not found for %s", repoID)
}

func (m *mockManager) GetFileContext(ctx context.Context, repoID, filePath string) (*ctxpkg.FileContext, error) {
	return nil, fmt.Errorf("not found")
}

func (m *mockManager) GetFunctionContext(ctx context.Context, repoID, filePath, funcName string) (*orchestrator.FunctionContextResult, error) {
	return nil, fmt.Errorf("function not found: %s", funcName)
}

func (m *mockManager) SearchFunctions(ctx context.Context, repoID, query string) ([]ctxpkg.FunctionRef, error) {
	return []ctxpkg.FunctionRef{{Function: "TestFunc", File: "test.go"}}, nil
}

func (m *mockManager) SearchByConcept(ctx context.Context, repoID, concept string) ([]ctxpkg.FunctionRef, error) {
	return nil, nil
}

func (m *mockManager) SearchBySideEffect(ctx context.Context, repoID, effect string) ([]ctxpkg.FunctionRef, error) {
	return nil, nil
}

func (m *mockManager) ListRepos(ctx context.Context) ([]ctxpkg.ContextMetadata, error) {
	return m.repos, nil
}

func (m *mockManager) GenerateAISummary(ctx context.Context, repoID string) (*ctxpkg.AISummary, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *mockManager) GenerateAIArchAnalysis(ctx context.Context, repoID string) (*ctxpkg.AIArchAnalysis, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *mockManager) IsAIEnabled() bool { return false }

func (m *mockManager) Ask(ctx context.Context, query string, repoIDs []string) (*orchestrator.QueryResult, error) {
	return nil, fmt.Errorf("AI not enabled")
}

func (m *mockManager) RefreshAIContext(ctx context.Context, repoIDs []string, force bool) (*orchestrator.RefreshResult, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *mockManager) ReviewPR(ctx context.Context, prURL string, opts prreview.ReviewOptions) (*prreview.ReviewResult, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *mockManager) CheckPRContext(ctx context.Context, prURL string) (*prreview.ContextStatus, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *mockManager) AnalyzeLocal(ctx context.Context, dirPath string, opts orchestrator.AnalyzeLocalOptions) (*orchestrator.AnalyzeLocalResult, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *mockManager) GetOrAnalyzeLocal(ctx context.Context, dirPath string) (*ctxpkg.RepoContext, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *mockManager) SmartQuery(ctx context.Context, query string, projectID string) (*orchestrator.SmartQueryResult, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *mockManager) RefreshFile(ctx context.Context, projectID, filePath string, opts orchestrator.RefreshFileOptions) (*orchestrator.RefreshFileResult, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *mockManager) RefreshChangedFiles(ctx context.Context, projectID string) ([]orchestrator.RefreshFileResult, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *mockManager) CheckFileStale(ctx context.Context, projectID, filePath string) (bool, error) {
	return false, nil
}

func (m *mockManager) GetPRContext(ctx context.Context, repoID string, changedFiles []orchestrator.ChangedFile) (*orchestrator.PRContextResult, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *mockManager) DeleteRepoContext(ctx context.Context, repoID string) error {
	return nil
}

func (m *mockManager) GetDependencyGraph(ctx context.Context, repoIDs []string, includeExternal bool) (*ctxpkg.DependencyGraph, error) {
	return nil, fmt.Errorf("not implemented")
}

// --- Mock OrgManager ---

type mockOrgManager struct {
	orgs []org.OrgWithCount
}

func (m *mockOrgManager) Register(ctx context.Context, orgID string, repoIDs []string, config *org.OrgConfig) (*org.Org, error) {
	return &org.Org{ID: orgID, Repos: repoIDs, Created: time.Now()}, nil
}

func (m *mockOrgManager) List(ctx context.Context) ([]org.OrgWithCount, error) {
	return m.orgs, nil
}

func (m *mockOrgManager) Get(ctx context.Context, orgID string) (*org.Org, error) {
	for _, o := range m.orgs {
		if o.ID == orgID {
			return &org.Org{ID: o.ID}, nil
		}
	}
	return nil, fmt.Errorf("org not found: %s", orgID)
}

func (m *mockOrgManager) AddRepos(ctx context.Context, orgID string, repoIDs []string) error {
	return nil
}

func (m *mockOrgManager) RemoveRepos(ctx context.Context, orgID string, repoIDs []string) error {
	return nil
}

func (m *mockOrgManager) Delete(ctx context.Context, orgID string) error {
	return nil
}

func (m *mockOrgManager) GetEffectiveConfig(ctx context.Context, orgID, repoID string) (*org.OrgConfig, error) {
	return nil, nil
}

func (m *mockOrgManager) SetRepoConfigOverride(ctx context.Context, orgID, repoID string, config *org.OrgConfig) error {
	return nil
}

func (m *mockOrgManager) AnalyzeOrg(ctx context.Context, orgID string, force bool, concurrency int) (*org.AnalysisResult, error) {
	return nil, nil
}

func (m *mockOrgManager) IndexOrg(ctx context.Context, orgID string, force bool, concurrency int) (*org.IndexOrgResult, error) {
	return nil, nil
}

// --- Test helpers ---

func newTestAPIServer(t *testing.T, mgr orchestrator.Manager, orgMgr org.Manager) *APIServer {
	t.Helper()
	logger := logging.New(bytes.NewBuffer(nil), logging.INFO, "test")
	config := DefaultAPIConfig()
	return NewAPIServer(mgr, orgMgr, config, logger)
}

// --- Handler Tests ---

func TestHandleListRepos_Paginated(t *testing.T) {
	mgr := &mockManager{
		repos: []ctxpkg.ContextMetadata{
			{RepoID: "repo1"}, {RepoID: "repo2"}, {RepoID: "repo3"},
		},
	}
	s := newTestAPIServer(t, mgr, &mockOrgManager{})

	req := httptest.NewRequest("GET", "/api/v1/repos?limit=2&offset=0", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp APIResponse
	err := json.NewDecoder(w.Body).Decode(&resp)
	require.NoError(t, err)
	assert.Equal(t, 3, resp.Meta.Total)
	assert.Equal(t, 2, resp.Meta.Limit)

	// Data should be array of 2
	data, ok := resp.Data.([]interface{})
	require.True(t, ok)
	assert.Len(t, data, 2)
}

func TestHandleGetRepoContext_EncodedID(t *testing.T) {
	mgr := &mockManager{
		repoCtx: map[string]*ctxpkg.RepoContext{
			"github.com/org/repo": {ID: "github.com/org/repo"},
		},
	}
	s := newTestAPIServer(t, mgr, &mockOrgManager{})

	req := httptest.NewRequest("GET", "/api/v1/repos/github.com%2Forg%2Frepo", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestHandleGetRepoContext_NotFound(t *testing.T) {
	mgr := &mockManager{repoCtx: map[string]*ctxpkg.RepoContext{}}
	s := newTestAPIServer(t, mgr, &mockOrgManager{})

	req := httptest.NewRequest("GET", "/api/v1/repos/nonexistent%2Frepo", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestHandleAnalyzeRepo_Returns202(t *testing.T) {
	mgr := &mockManager{}
	s := newTestAPIServer(t, mgr, &mockOrgManager{})

	body := `{"repo_url": "https://github.com/org/repo"}`
	req := httptest.NewRequest("POST", "/api/v1/repos/analyze", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusAccepted, w.Code)
}

func TestHandleAnalyzeRepo_InvalidJSON(t *testing.T) {
	mgr := &mockManager{}
	s := newTestAPIServer(t, mgr, &mockOrgManager{})

	req := httptest.NewRequest("POST", "/api/v1/repos/analyze", bytes.NewBufferString("{invalid"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandleAnalyzeRepo_MissingURL(t *testing.T) {
	mgr := &mockManager{}
	s := newTestAPIServer(t, mgr, &mockOrgManager{})

	body := `{}`
	req := httptest.NewRequest("POST", "/api/v1/repos/analyze", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandleCreateOrg_Returns201(t *testing.T) {
	s := newTestAPIServer(t, &mockManager{}, &mockOrgManager{})

	body := `{"id": "test-org", "repos": []}`
	req := httptest.NewRequest("POST", "/api/v1/orgs", bytes.NewBufferString(body))
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)
}

func TestHandleListOrgs_Paginated(t *testing.T) {
	orgMgr := &mockOrgManager{
		orgs: []org.OrgWithCount{
			{Org: org.Org{ID: "org1"}}, {Org: org.Org{ID: "org2"}}, {Org: org.Org{ID: "org3"}},
		},
	}
	s := newTestAPIServer(t, &mockManager{}, orgMgr)

	req := httptest.NewRequest("GET", "/api/v1/orgs?limit=10", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp APIResponse
	err := json.NewDecoder(w.Body).Decode(&resp)
	require.NoError(t, err)
	assert.Equal(t, 3, resp.Meta.Total)
}

func TestResponseEnvelope_Format(t *testing.T) {
	s := newTestAPIServer(t, &mockManager{repos: []ctxpkg.ContextMetadata{}}, &mockOrgManager{})

	req := httptest.NewRequest("GET", "/api/v1/repos", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	var raw map[string]interface{}
	err := json.NewDecoder(w.Body).Decode(&raw)
	require.NoError(t, err)

	_, hasData := raw["data"]
	_, hasMeta := raw["meta"]
	_, hasError := raw["error"]
	assert.True(t, hasData, "response should have data field")
	assert.True(t, hasMeta, "response should have meta field")
	assert.True(t, hasError, "response should have error field")
}

func TestHandleSearchFunctions_MissingQuery(t *testing.T) {
	mgr := &mockManager{
		repoCtx: map[string]*ctxpkg.RepoContext{
			"test/repo": {ID: "test/repo"},
		},
	}
	s := newTestAPIServer(t, mgr, &mockOrgManager{})

	req := httptest.NewRequest("GET", "/api/v1/repos/test%2Frepo/search", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestPaginationDefaults_AndBounds(t *testing.T) {
	mgr := &mockManager{repos: []ctxpkg.ContextMetadata{}}
	s := newTestAPIServer(t, mgr, &mockOrgManager{})

	tests := []struct {
		query        string
		expectLimit  int
		expectOffset int
	}{
		{"", 50, 0},
		{"?limit=999", 200, 0},
		{"?limit=-1", 50, 0},
	}

	for _, tt := range tests {
		req := httptest.NewRequest("GET", "/api/v1/repos"+tt.query, nil)
		w := httptest.NewRecorder()
		s.router.ServeHTTP(w, req)

		var resp APIResponse
		json.NewDecoder(w.Body).Decode(&resp)
		assert.Equal(t, tt.expectLimit, resp.Meta.Limit, "query: %s", tt.query)
		assert.Equal(t, tt.expectOffset, resp.Meta.Offset, "query: %s", tt.query)
	}
}
