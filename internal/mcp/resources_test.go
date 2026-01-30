package mcp

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	ctxpkg "github.com/yashpalc/mcp-repo-context/internal/context"
	"github.com/yashpalc/mcp-repo-context/internal/orchestrator"
	"github.com/yashpalc/mcp-repo-context/internal/prreview"
)

// mockManager implements orchestrator.Manager for testing
type mockManager struct {
	repos    map[string]*ctxpkg.RepoContext
	repoList []ctxpkg.ContextMetadata
}

func (m *mockManager) AnalyzeRepo(ctx context.Context, repoURL string, opts orchestrator.AnalyzeOptions) (*orchestrator.AnalyzeResult, error) {
	return nil, nil
}

func (m *mockManager) GetContext(ctx context.Context, repoID string) (*ctxpkg.RepoContext, error) {
	if rc, ok := m.repos[repoID]; ok {
		return rc, nil
	}
	return nil, nil
}

func (m *mockManager) GetFileContext(ctx context.Context, repoID, filePath string) (*ctxpkg.FileContext, error) {
	return nil, nil
}

func (m *mockManager) GetFunctionContext(ctx context.Context, repoID, filePath, funcName string) (*orchestrator.FunctionContextResult, error) {
	return nil, nil
}

func (m *mockManager) SearchFunctions(ctx context.Context, repoID, query string) ([]ctxpkg.FunctionRef, error) {
	return nil, nil
}

func (m *mockManager) SearchByConcept(ctx context.Context, repoID, concept string) ([]ctxpkg.FunctionRef, error) {
	return nil, nil
}

func (m *mockManager) SearchBySideEffect(ctx context.Context, repoID, effect string) ([]ctxpkg.FunctionRef, error) {
	return nil, nil
}

func (m *mockManager) ListRepos(ctx context.Context) ([]ctxpkg.ContextMetadata, error) {
	return m.repoList, nil
}

func (m *mockManager) GenerateAISummary(ctx context.Context, repoID string) (*ctxpkg.AISummary, error) {
	return nil, nil
}

func (m *mockManager) GenerateAIArchAnalysis(ctx context.Context, repoID string) (*ctxpkg.AIArchAnalysis, error) {
	return nil, nil
}

func (m *mockManager) IsAIEnabled() bool {
	return false
}

func (m *mockManager) Ask(ctx context.Context, query string, repoIDs []string) (*orchestrator.QueryResult, error) {
	return nil, nil
}

func (m *mockManager) RefreshAIContext(ctx context.Context, repoIDs []string, force bool) (*orchestrator.RefreshResult, error) {
	return nil, nil
}

func (m *mockManager) ReviewPR(ctx context.Context, prURL string, opts prreview.ReviewOptions) (*prreview.ReviewResult, error) {
	return nil, nil
}

func (m *mockManager) CheckPRContext(ctx context.Context, prURL string) (*prreview.ContextStatus, error) {
	return nil, nil
}

func (m *mockManager) AnalyzeLocal(ctx context.Context, dirPath string, opts orchestrator.AnalyzeLocalOptions) (*orchestrator.AnalyzeLocalResult, error) {
	return nil, nil
}

func (m *mockManager) GetOrAnalyzeLocal(ctx context.Context, dirPath string) (*ctxpkg.RepoContext, error) {
	return nil, nil
}

func (m *mockManager) SmartQuery(ctx context.Context, query string, projectID string) (*orchestrator.SmartQueryResult, error) {
	return nil, nil
}

func (m *mockManager) RefreshFile(ctx context.Context, projectID, filePath string, opts orchestrator.RefreshFileOptions) (*orchestrator.RefreshFileResult, error) {
	return nil, nil
}

func (m *mockManager) RefreshChangedFiles(ctx context.Context, projectID string) ([]orchestrator.RefreshFileResult, error) {
	return nil, nil
}

func (m *mockManager) CheckFileStale(ctx context.Context, projectID, filePath string) (bool, error) {
	return false, nil
}

func (m *mockManager) GetPRContext(ctx context.Context, repoID string, changedFiles []orchestrator.ChangedFile) (*orchestrator.PRContextResult, error) {
	return nil, nil
}

func newTestServer() *server {
	mockMgr := &mockManager{
		repos: map[string]*ctxpkg.RepoContext{
			"test-repo": {
				ID:         "test-repo",
				URL:        "https://github.com/test/repo",
				Branch:     "main",
				CommitHash: "abc123",
				AnalyzedAt: time.Now(),
				Statistics: ctxpkg.RepoStatistics{TotalFiles: 10, TotalLines: 1000, FunctionCount: 50, TypeCount: 20},
				Files:      map[string]*ctxpkg.FileContext{},
				Architecture: &ctxpkg.ArchitectureContext{
					Overview:    "Test architecture",
					BuildSystem: "go",
				},
			},
		},
		repoList: []ctxpkg.ContextMetadata{
			{RepoID: "test-repo"},
		},
	}

	return &server{
		manager: mockMgr,
		config:  &ServerConfig{Name: "test", Version: "1.0"},
	}
}

func TestHandleListResources(t *testing.T) {
	s := newTestServer()
	ctx := context.Background()

	req := &jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "resources/list",
	}

	resp := s.handleListResources(ctx, req)

	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error.Message)
	}

	result, ok := resp.Result.(listResourcesResult)
	if !ok {
		t.Fatalf("unexpected result type: %T", resp.Result)
	}

	// Should have 4 resources per repo + 1 tool hints resource
	expectedCount := 4*1 + 1 // 4 per repo (overview, architecture, functions, types) + tool hints
	if len(result.Resources) != expectedCount {
		t.Errorf("expected %d resources, got %d", expectedCount, len(result.Resources))
	}

	// Verify resource URIs
	foundOverview := false
	foundToolHints := false
	for _, r := range result.Resources {
		if r.URI == "repo://test-repo/overview" {
			foundOverview = true
		}
		if r.URI == "mcp://tool-hints" {
			foundToolHints = true
		}
	}

	if !foundOverview {
		t.Error("missing overview resource")
	}
	if !foundToolHints {
		t.Error("missing tool-hints resource")
	}
}

func TestHandleReadResource_ToolHints(t *testing.T) {
	s := newTestServer()

	params, _ := json.Marshal(readResourceParams{URI: "mcp://tool-hints"})
	req := &jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "resources/read",
		Params:  params,
	}

	resp := s.handleReadResource(context.Background(), req)

	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error.Message)
	}

	result, ok := resp.Result.(readResourceResult)
	if !ok {
		t.Fatalf("unexpected result type: %T", resp.Result)
	}

	if len(result.Contents) != 1 {
		t.Fatalf("expected 1 content, got %d", len(result.Contents))
	}

	content := result.Contents[0]
	if content.MimeType != "application/json" {
		t.Errorf("expected application/json mime type, got %s", content.MimeType)
	}

	// Verify content is valid JSON
	var guide map[string]any
	if err := json.Unmarshal([]byte(content.Text), &guide); err != nil {
		t.Errorf("content is not valid JSON: %v", err)
	}

	// Verify guide has expected keys
	if _, ok := guide["output_sizes"]; !ok {
		t.Error("missing output_sizes in guide")
	}
	if _, ok := guide["free_tools"]; !ok {
		t.Error("missing free_tools in guide")
	}
}

func TestHandleReadResource_Overview(t *testing.T) {
	s := newTestServer()

	params, _ := json.Marshal(readResourceParams{URI: "repo://test-repo/overview"})
	req := &jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "resources/read",
		Params:  params,
	}

	resp := s.handleReadResource(context.Background(), req)

	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error.Message)
	}

	result, ok := resp.Result.(readResourceResult)
	if !ok {
		t.Fatalf("unexpected result type: %T", resp.Result)
	}

	if len(result.Contents) != 1 {
		t.Fatalf("expected 1 content, got %d", len(result.Contents))
	}

	// Verify content is valid JSON with expected fields
	var overview map[string]any
	if err := json.Unmarshal([]byte(result.Contents[0].Text), &overview); err != nil {
		t.Errorf("content is not valid JSON: %v", err)
	}

	if overview["repo_id"] != "test-repo" {
		t.Errorf("expected repo_id=test-repo, got %v", overview["repo_id"])
	}
}

func TestHandleReadResource_InvalidURI(t *testing.T) {
	s := newTestServer()

	params, _ := json.Marshal(readResourceParams{URI: "invalid://resource"})
	req := &jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "resources/read",
		Params:  params,
	}

	resp := s.handleReadResource(context.Background(), req)

	if resp.Error == nil {
		t.Error("expected error for invalid URI")
	}
	if resp.Error.Code != -32602 {
		t.Errorf("expected error code -32602, got %d", resp.Error.Code)
	}
}

func TestHandleReadResource_UnknownResourceType(t *testing.T) {
	s := newTestServer()

	params, _ := json.Marshal(readResourceParams{URI: "repo://test-repo/unknown"})
	req := &jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "resources/read",
		Params:  params,
	}

	resp := s.handleReadResource(context.Background(), req)

	if resp.Error == nil {
		t.Error("expected error for unknown resource type")
	}
}
