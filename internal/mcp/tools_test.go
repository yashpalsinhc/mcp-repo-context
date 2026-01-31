package mcp

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/yashpalc/mcp-repo-context/internal/compose"
	ctxpkg "github.com/yashpalc/mcp-repo-context/internal/context"
	"github.com/yashpalc/mcp-repo-context/internal/orchestrator"
	"github.com/yashpalc/mcp-repo-context/internal/prreview"
	"github.com/yashpalc/mcp-repo-context/internal/tokens"
)

// testManager is a mock implementation of orchestrator.Manager for testing
type testManager struct {
	repos    map[string]*ctxpkg.RepoContext
	repoList []ctxpkg.ContextMetadata
}

func (m *testManager) AnalyzeRepo(ctx context.Context, repoURL string, opts orchestrator.AnalyzeOptions) (*orchestrator.AnalyzeResult, error) {
	return nil, nil
}

func (m *testManager) GetContext(ctx context.Context, repoID string) (*ctxpkg.RepoContext, error) {
	if rc, ok := m.repos[repoID]; ok {
		return rc, nil
	}
	return nil, nil
}

func (m *testManager) GetFileContext(ctx context.Context, repoID, filePath string) (*ctxpkg.FileContext, error) {
	return nil, nil
}

func (m *testManager) GetFunctionContext(ctx context.Context, repoID, filePath, funcName string) (*orchestrator.FunctionContextResult, error) {
	return nil, nil
}

func (m *testManager) SearchFunctions(ctx context.Context, repoID, query string) ([]ctxpkg.FunctionRef, error) {
	return nil, nil
}

func (m *testManager) SearchByConcept(ctx context.Context, repoID, concept string) ([]ctxpkg.FunctionRef, error) {
	return nil, nil
}

func (m *testManager) SearchBySideEffect(ctx context.Context, repoID, effect string) ([]ctxpkg.FunctionRef, error) {
	return nil, nil
}

func (m *testManager) ListRepos(ctx context.Context) ([]ctxpkg.ContextMetadata, error) {
	return m.repoList, nil
}

func (m *testManager) GenerateAISummary(ctx context.Context, repoID string) (*ctxpkg.AISummary, error) {
	return nil, nil
}

func (m *testManager) GenerateAIArchAnalysis(ctx context.Context, repoID string) (*ctxpkg.AIArchAnalysis, error) {
	return nil, nil
}

func (m *testManager) IsAIEnabled() bool {
	return false
}

func (m *testManager) Ask(ctx context.Context, query string, repoIDs []string) (*orchestrator.QueryResult, error) {
	return nil, nil
}

func (m *testManager) RefreshAIContext(ctx context.Context, repoIDs []string, force bool) (*orchestrator.RefreshResult, error) {
	return nil, nil
}

func (m *testManager) ReviewPR(ctx context.Context, prURL string, opts prreview.ReviewOptions) (*prreview.ReviewResult, error) {
	return nil, nil
}

func (m *testManager) CheckPRContext(ctx context.Context, prURL string) (*prreview.ContextStatus, error) {
	return nil, nil
}

func (m *testManager) AnalyzeLocal(ctx context.Context, dirPath string, opts orchestrator.AnalyzeLocalOptions) (*orchestrator.AnalyzeLocalResult, error) {
	return nil, nil
}

func (m *testManager) GetOrAnalyzeLocal(ctx context.Context, dirPath string) (*ctxpkg.RepoContext, error) {
	return nil, nil
}

func (m *testManager) SmartQuery(ctx context.Context, query string, projectID string) (*orchestrator.SmartQueryResult, error) {
	return nil, nil
}

func (m *testManager) RefreshFile(ctx context.Context, projectID, filePath string, opts orchestrator.RefreshFileOptions) (*orchestrator.RefreshFileResult, error) {
	return nil, nil
}

func (m *testManager) RefreshChangedFiles(ctx context.Context, projectID string) ([]orchestrator.RefreshFileResult, error) {
	return nil, nil
}

func (m *testManager) CheckFileStale(ctx context.Context, projectID, filePath string) (bool, error) {
	return false, nil
}

func (m *testManager) GetPRContext(ctx context.Context, repoID string, changedFiles []orchestrator.ChangedFile) (*orchestrator.PRContextResult, error) {
	return nil, nil
}

func newToolsTestServer() *server {
	testMgr := &testManager{
		repos: map[string]*ctxpkg.RepoContext{
			"test-repo": {
				ID:         "test-repo",
				URL:        "https://github.com/test/repo",
				Branch:     "main",
				CommitHash: "abc123",
				AnalyzedAt: time.Now(),
				Statistics: ctxpkg.RepoStatistics{
					TotalFiles:    10,
					TotalLines:    1000,
					FunctionCount: 50,
					TypeCount:     20,
				},
				Files: map[string]*ctxpkg.FileContext{
					"pkg/auth/auth.go": {
						Path:     "pkg/auth/auth.go",
						Language: "go",
						Package:  "auth",
						Functions: []ctxpkg.FunctionDef{
							{
								Name:        "ValidateToken",
								Signature:   "func ValidateToken(token string) (bool, error)",
								Description: "Validates an authentication token",
								IsPublic:    true,
								LineStart:   10,
								LineEnd:     25,
								Behavior: &ctxpkg.FunctionBehavior{
									Summary: "Validates a JWT token and returns whether it's valid",
								},
								SideEffects: []string{"logging"},
							},
							{
								Name:        "CreateToken",
								Signature:   "func CreateToken(userID string) (string, error)",
								Description: "Creates a new authentication token",
								IsPublic:    true,
								LineStart:   30,
								LineEnd:     50,
								Behavior: &ctxpkg.FunctionBehavior{
									Summary: "Generates a new JWT token for the given user",
								},
							},
						},
						Types: []ctxpkg.TypeDef{
							{
								Name:        "TokenClaims",
								Kind:        "struct",
								Description: "JWT claims structure",
								IsPublic:    true,
								LineStart:   5,
								Fields: []ctxpkg.Field{
									{Name: "UserID", Type: "string"},
									{Name: "ExpiresAt", Type: "time.Time"},
								},
							},
						},
					},
					"pkg/db/db.go": {
						Path:     "pkg/db/db.go",
						Language: "go",
						Package:  "db",
						Functions: []ctxpkg.FunctionDef{
							{
								Name:        "QueryUser",
								Signature:   "func QueryUser(id string) (*User, error)",
								Description: "Queries a user from the database",
								IsPublic:    true,
								LineStart:   10,
								LineEnd:     30,
								Behavior: &ctxpkg.FunctionBehavior{
									Summary: "Fetches user data from the database by ID",
								},
								SideEffects: []string{"db_query"},
							},
						},
					},
				},
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
		manager:         testMgr,
		config:          &ServerConfig{Name: "test", Version: "1.0"},
		patternRegistry: compose.DefaultRegistry(),
	}
}

// ============================================================================
// Tests for get_context_budgeted tool
// ============================================================================

func TestToolGetContextBudgeted_ValidRequest(t *testing.T) {
	s := newToolsTestServer()
	ctx := context.Background()

	args := map[string]any{
		"repo_id":      "test-repo",
		"query":        "authentication token validation",
		"token_budget": float64(4000),
	}

	result := s.toolGetContextBudgeted(ctx, args)

	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content[0].Text)
	}

	content := result.Content[0].Text

	// Should contain the query
	if !strings.Contains(content, "authentication token validation") {
		t.Error("result should contain the query")
	}

	// Should contain token budget info
	if !strings.Contains(content, "Token Budget") {
		t.Error("result should contain token budget info")
	}

	// Should contain relevant function (ValidateToken)
	if !strings.Contains(content, "ValidateToken") {
		t.Error("result should contain ValidateToken function (highly relevant)")
	}
}

func TestToolGetContextBudgeted_MissingRepoID(t *testing.T) {
	s := newToolsTestServer()
	ctx := context.Background()

	args := map[string]any{
		"query":        "authentication",
		"token_budget": float64(4000),
	}

	result := s.toolGetContextBudgeted(ctx, args)

	if !result.IsError {
		t.Error("expected error for missing repo_id")
	}

	if !strings.Contains(result.Content[0].Text, "repo_id is required") {
		t.Error("error should mention repo_id is required")
	}
}

func TestToolGetContextBudgeted_MissingQuery(t *testing.T) {
	s := newToolsTestServer()
	ctx := context.Background()

	args := map[string]any{
		"repo_id":      "test-repo",
		"token_budget": float64(4000),
	}

	result := s.toolGetContextBudgeted(ctx, args)

	if !result.IsError {
		t.Error("expected error for missing query")
	}

	if !strings.Contains(result.Content[0].Text, "query is required") {
		t.Error("error should mention query is required")
	}
}

func TestToolGetContextBudgeted_DefaultBudget(t *testing.T) {
	s := newToolsTestServer()
	ctx := context.Background()

	args := map[string]any{
		"repo_id": "test-repo",
		"query":   "authentication",
		// No token_budget - should use default 4000
	}

	result := s.toolGetContextBudgeted(ctx, args)

	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content[0].Text)
	}

	content := result.Content[0].Text
	if !strings.Contains(content, "**Token Budget:** 4000") {
		t.Error("should use default token budget of 4000")
	}
}

func TestToolGetContextBudgeted_BudgetCap(t *testing.T) {
	s := newToolsTestServer()
	ctx := context.Background()

	args := map[string]any{
		"repo_id":      "test-repo",
		"query":        "authentication",
		"token_budget": float64(100000), // Should be capped at 32000
	}

	result := s.toolGetContextBudgeted(ctx, args)

	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content[0].Text)
	}

	content := result.Content[0].Text
	if !strings.Contains(content, "**Token Budget:** 32000") {
		t.Error("token budget should be capped at 32000")
	}
}

// ============================================================================
// Tests for semantic_search tool
// ============================================================================

func TestToolSemanticSearch_NoVectorStore(t *testing.T) {
	s := newToolsTestServer()
	// semanticSearch is nil by default
	ctx := context.Background()

	args := map[string]any{
		"query":   "authentication",
		"repo_id": "test-repo",
	}

	result := s.toolSemanticSearch(ctx, args)

	if !result.IsError {
		t.Error("expected error when semantic search is not enabled")
	}

	if !strings.Contains(result.Content[0].Text, "Semantic search is not enabled") {
		t.Error("error should mention semantic search is not enabled")
	}
}

func TestToolSemanticSearch_MissingQuery(t *testing.T) {
	s := newToolsTestServer()
	ctx := context.Background()

	args := map[string]any{
		"repo_id": "test-repo",
	}

	result := s.toolSemanticSearch(ctx, args)

	if !result.IsError {
		t.Error("expected error for missing query")
	}

	if !strings.Contains(result.Content[0].Text, "query is required") {
		t.Error("error should mention query is required")
	}
}

func TestToolSemanticSearch_MissingRepoID(t *testing.T) {
	s := newToolsTestServer()
	ctx := context.Background()

	args := map[string]any{
		"query": "authentication",
	}

	result := s.toolSemanticSearch(ctx, args)

	if !result.IsError {
		t.Error("expected error for missing repo_id")
	}

	if !strings.Contains(result.Content[0].Text, "repo_id is required") {
		t.Error("error should mention repo_id is required")
	}
}

// ============================================================================
// Tests for execute_pattern tool
// ============================================================================

func TestToolExecutePattern_InvalidPattern(t *testing.T) {
	s := newToolsTestServer()
	ctx := context.Background()

	args := map[string]any{
		"pattern_name": "nonexistent_pattern",
	}

	result := s.toolExecutePattern(ctx, args)

	if !result.IsError {
		t.Error("expected error for invalid pattern")
	}

	if !strings.Contains(result.Content[0].Text, "not found") {
		t.Error("error should mention pattern not found")
	}
}

func TestToolExecutePattern_MissingPatternName(t *testing.T) {
	s := newToolsTestServer()
	ctx := context.Background()

	args := map[string]any{
		"params": map[string]any{"repo_id": "test-repo"},
	}

	result := s.toolExecutePattern(ctx, args)

	if !result.IsError {
		t.Error("expected error for missing pattern_name")
	}

	if !strings.Contains(result.Content[0].Text, "pattern_name is required") {
		t.Error("error should mention pattern_name is required")
	}
}

func TestToolListPatterns(t *testing.T) {
	s := newToolsTestServer()
	ctx := context.Background()

	args := map[string]any{}

	result := s.toolListPatterns(ctx, args)

	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content[0].Text)
	}

	content := result.Content[0].Text

	// Should contain header
	if !strings.Contains(content, "Available Patterns") {
		t.Error("result should contain header")
	}

	// Should contain some default patterns
	if !strings.Contains(content, "search_with_context") {
		t.Error("result should contain search_with_context pattern")
	}

	// Should contain usage instructions
	if !strings.Contains(content, "execute_pattern") {
		t.Error("result should contain usage instructions")
	}
}

// ============================================================================
// Tests for index_repository tool
// ============================================================================

func TestToolIndexRepository_NoVectorStore(t *testing.T) {
	s := newToolsTestServer()
	ctx := context.Background()

	args := map[string]any{
		"repo_id": "test-repo",
	}

	result := s.toolIndexRepository(ctx, args)

	if !result.IsError {
		t.Error("expected error when semantic search is not enabled")
	}

	if !strings.Contains(result.Content[0].Text, "Semantic search is not enabled") {
		t.Error("error should mention semantic search is not enabled")
	}
}

func TestToolIndexRepository_MissingRepoID(t *testing.T) {
	s := newToolsTestServer()
	ctx := context.Background()

	args := map[string]any{}

	result := s.toolIndexRepository(ctx, args)

	if !result.IsError {
		t.Error("expected error for missing repo_id")
	}

	if !strings.Contains(result.Content[0].Text, "repo_id is required") {
		t.Error("error should mention repo_id is required")
	}
}

// ============================================================================
// Tests for helper functions
// ============================================================================

func TestScoreFunctionRelevance(t *testing.T) {
	fn := ctxpkg.FunctionDef{
		Name:        "ValidateToken",
		Signature:   "func ValidateToken(token string) (bool, error)",
		Description: "Validates an authentication token",
		Behavior: &ctxpkg.FunctionBehavior{
			Summary: "Validates a JWT token for authentication",
		},
	}

	tests := []struct {
		name     string
		keywords []string
		wantGt0  bool
	}{
		{
			name:     "matching keywords",
			keywords: []string{"validate", "token"},
			wantGt0:  true,
		},
		{
			name:     "partial match",
			keywords: []string{"auth"},
			wantGt0:  true,
		},
		{
			name:     "no match",
			keywords: []string{"database", "query"},
			wantGt0:  false,
		},
		{
			name:     "empty keywords",
			keywords: []string{},
			wantGt0:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := scoreFunctionRelevance(fn, tt.keywords)
			if tt.wantGt0 && score <= 0 {
				t.Errorf("expected score > 0, got %f", score)
			}
			if !tt.wantGt0 && score > 0 {
				t.Errorf("expected score = 0, got %f", score)
			}
		})
	}
}

func TestTokensBudgeter_Integration(t *testing.T) {
	// Test that the tokens package integrates correctly
	budgeter := tokens.NewBudgeter()

	fn := ctxpkg.FunctionDef{
		Name:        "TestFunction",
		Signature:   "func TestFunction() error",
		Description: "A test function",
	}

	cost := budgeter.EstimateCost(fn)
	if cost <= 0 {
		t.Error("expected positive token cost")
	}

	// Test summarization
	summary := budgeter.SummarizeFunction(fn)
	if summary.Name != fn.Name {
		t.Error("summary should preserve function name")
	}

	summaryCost := budgeter.EstimateCost(summary)
	// Summary should be roughly equal or smaller (no behavior details to strip)
	if summaryCost > cost*2 {
		t.Error("summary cost should not be much larger than original")
	}
}

// ============================================================================
// Tests for createToolExecutor
// ============================================================================

func TestCreateToolExecutor(t *testing.T) {
	s := newToolsTestServer()
	executor := s.createToolExecutor()

	if executor == nil {
		t.Fatal("executor should not be nil")
	}

	// Test with a simple tool call
	chainCtx := compose.NewChainContext(context.Background())
	result := executor.Execute(chainCtx, compose.ToolCall{
		Name: "list_repos",
		Params: map[string]any{},
	})

	// list_repos should work (though may return empty)
	// We just verify it doesn't crash and returns a result
	if result.ToolName != "list_repos" {
		t.Errorf("expected tool name list_repos, got %s", result.ToolName)
	}
}

func TestCreateToolExecutor_UnknownTool(t *testing.T) {
	s := newToolsTestServer()
	executor := s.createToolExecutor()

	chainCtx := compose.NewChainContext(context.Background())
	result := executor.Execute(chainCtx, compose.ToolCall{
		Name: "unknown_tool",
		Params: map[string]any{},
	})

	if result.Success {
		t.Error("expected failure for unknown tool")
	}

	if !strings.Contains(result.Error, "not supported") {
		t.Error("error should mention tool not supported")
	}
}
