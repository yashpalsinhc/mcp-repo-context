package graph

import (
	"context"
	"strings"
	"testing"
	"time"

	ctxpkg "github.com/yashpalc/mcp-repo-context/internal/context"
	"github.com/yashpalc/mcp-repo-context/internal/orchestrator"
	"github.com/yashpalc/mcp-repo-context/internal/prreview"
)

// mockManager implements orchestrator.Manager for testing.
type mockManager struct {
	repoCtx *ctxpkg.RepoContext
}

func (m *mockManager) AnalyzeRepo(ctx context.Context, repoURL string, opts orchestrator.AnalyzeOptions) (*orchestrator.AnalyzeResult, error) {
	return nil, nil
}

func (m *mockManager) GetContext(ctx context.Context, repoID string) (*ctxpkg.RepoContext, error) {
	return m.repoCtx, nil
}

func (m *mockManager) GetFileContext(ctx context.Context, repoID, filePath string) (*ctxpkg.FileContext, error) {
	if fileCtx, ok := m.repoCtx.Files[filePath]; ok {
		return fileCtx, nil
	}
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
	return nil, nil
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

func (m *mockManager) DeleteRepoContext(ctx context.Context, repoID string) error {
	return nil
}

func (m *mockManager) GetDependencyGraph(ctx context.Context, repoIDs []string, includeExternal bool) (*ctxpkg.DependencyGraph, error) {
	return nil, nil
}

// createTestRepoContext creates a mock repository context for testing.
func createTestRepoContext() *ctxpkg.RepoContext {
	return &ctxpkg.RepoContext{
		ID:         "test/repo",
		URL:        "https://github.com/test/repo",
		Branch:     "main",
		CommitHash: "abc123",
		AnalyzedAt: time.Now(),
		Files: map[string]*ctxpkg.FileContext{
			"pkg/handlers/user.go": {
				Path:     "pkg/handlers/user.go",
				Language: "go",
				Functions: []ctxpkg.FunctionDef{
					{
						Name:      "CreateUser",
						Signature: "func CreateUser(ctx context.Context, req CreateUserRequest) (*User, error)",
						IsPublic:  true,
						LineStart: 10,
						LineEnd:   50,
						Calls: []ctxpkg.CallRef{
							{Function: "ValidateUser", Type: "internal", Line: 15},
							{Function: "SaveUser", Type: "internal", Line: 30},
						},
					},
					{
						Name:      "ValidateUser",
						Signature: "func ValidateUser(user *User) error",
						IsPublic:  true,
						LineStart: 55,
						LineEnd:   80,
						Calls: []ctxpkg.CallRef{
							{Function: "CheckEmail", Type: "internal", Line: 60},
						},
					},
				},
			},
			"pkg/repository/user.go": {
				Path:     "pkg/repository/user.go",
				Language: "go",
				Functions: []ctxpkg.FunctionDef{
					{
						Name:      "SaveUser",
						Signature: "func (r *UserRepo) SaveUser(ctx context.Context, user *User) error",
						IsPublic:  true,
						LineStart: 20,
						LineEnd:   45,
						Calls: []ctxpkg.CallRef{
							{Function: "Query", Package: "db", Type: "external", Line: 35},
						},
					},
				},
			},
			"pkg/validators/email.go": {
				Path:     "pkg/validators/email.go",
				Language: "go",
				Functions: []ctxpkg.FunctionDef{
					{
						Name:      "CheckEmail",
						Signature: "func CheckEmail(email string) error",
						IsPublic:  true,
						LineStart: 5,
						LineEnd:   20,
						Calls:     []ctxpkg.CallRef{},
					},
				},
			},
			"cmd/api/main.go": {
				Path:     "cmd/api/main.go",
				Language: "go",
				Functions: []ctxpkg.FunctionDef{
					{
						Name:      "main",
						Signature: "func main()",
						IsPublic:  false,
						LineStart: 10,
						LineEnd:   30,
						Calls: []ctxpkg.CallRef{
							{Function: "CreateUser", Type: "internal", Line: 25},
						},
					},
				},
			},
		},
	}
}

func TestGenerateMermaid(t *testing.T) {
	manager := &mockManager{repoCtx: createTestRepoContext()}
	visualizer := NewGraphVisualizer(manager)

	tests := []struct {
		name         string
		functionName string
		depth        int
		wantContains []string
		wantErr      bool
	}{
		{
			name:         "CreateUser with callers and callees",
			functionName: "CreateUser",
			depth:        2,
			wantContains: []string{
				"```mermaid",
				"flowchart TD",
				"CreateUser",
				"ValidateUser",
				"SaveUser",
				"main", // caller
				"-->",
			},
			wantErr: false,
		},
		{
			name:         "ValidateUser with callees",
			functionName: "ValidateUser",
			depth:        2,
			wantContains: []string{
				"```mermaid",
				"ValidateUser",
				"CheckEmail", // callee
				"CreateUser", // caller
			},
			wantErr: false,
		},
		{
			name:         "function not found",
			functionName: "NonExistent",
			depth:        2,
			wantErr:      true,
		},
		{
			name:         "default depth",
			functionName: "CreateUser",
			depth:        0, // should default to 2
			wantContains: []string{
				"```mermaid",
				"CreateUser",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := visualizer.GenerateMermaid(context.Background(), "test/repo", tt.functionName, tt.depth)

			if tt.wantErr {
				if err == nil {
					t.Errorf("GenerateMermaid() expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("GenerateMermaid() unexpected error: %v", err)
				return
			}

			for _, want := range tt.wantContains {
				if !strings.Contains(result, want) {
					t.Errorf("GenerateMermaid() result missing expected content '%s'. Got:\n%s", want, result)
				}
			}
		})
	}
}

func TestGenerateDOT(t *testing.T) {
	manager := &mockManager{repoCtx: createTestRepoContext()}
	visualizer := NewGraphVisualizer(manager)

	tests := []struct {
		name         string
		functionName string
		depth        int
		wantContains []string
		wantErr      bool
	}{
		{
			name:         "CreateUser DOT graph",
			functionName: "CreateUser",
			depth:        2,
			wantContains: []string{
				"digraph CallGraph",
				"rankdir=TB",
				"CreateUser",
				"ValidateUser",
				"SaveUser",
				"->",
			},
			wantErr: false,
		},
		{
			name:         "function not found",
			functionName: "NonExistent",
			depth:        2,
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := visualizer.GenerateDOT(context.Background(), "test/repo", tt.functionName, tt.depth)

			if tt.wantErr {
				if err == nil {
					t.Errorf("GenerateDOT() expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("GenerateDOT() unexpected error: %v", err)
				return
			}

			for _, want := range tt.wantContains {
				if !strings.Contains(result, want) {
					t.Errorf("GenerateDOT() result missing expected content '%s'. Got:\n%s", want, result)
				}
			}
		})
	}
}

func TestGetCallTree(t *testing.T) {
	manager := &mockManager{repoCtx: createTestRepoContext()}
	visualizer := NewGraphVisualizer(manager)

	tests := []struct {
		name           string
		functionName   string
		depth          int
		wantCallers    int
		wantCallees    int
		wantErr        bool
	}{
		{
			name:         "CreateUser call tree",
			functionName: "CreateUser",
			depth:        2,
			wantCallers:  1, // main calls CreateUser
			wantCallees:  2, // ValidateUser and SaveUser
			wantErr:      false,
		},
		{
			name:         "ValidateUser call tree",
			functionName: "ValidateUser",
			depth:        2,
			wantCallers:  1, // CreateUser calls ValidateUser
			wantCallees:  1, // CheckEmail
			wantErr:      false,
		},
		{
			name:         "CheckEmail call tree (leaf node)",
			functionName: "CheckEmail",
			depth:        2,
			wantCallers:  1, // ValidateUser calls CheckEmail
			wantCallees:  0, // no internal callees
			wantErr:      false,
		},
		{
			name:         "function not found",
			functionName: "NonExistent",
			depth:        2,
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := visualizer.GetCallTree(context.Background(), "test/repo", tt.functionName, tt.depth)

			if tt.wantErr {
				if err == nil {
					t.Errorf("GetCallTree() expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("GetCallTree() unexpected error: %v", err)
				return
			}

			if result == nil {
				t.Errorf("GetCallTree() returned nil result")
				return
			}

			if result.Name != tt.functionName {
				t.Errorf("GetCallTree() Name = %v, want %v", result.Name, tt.functionName)
			}

			if len(result.Callers) != tt.wantCallers {
				t.Errorf("GetCallTree() Callers count = %d, want %d", len(result.Callers), tt.wantCallers)
			}

			if len(result.Callees) != tt.wantCallees {
				t.Errorf("GetCallTree() Callees count = %d, want %d", len(result.Callees), tt.wantCallees)
			}
		})
	}
}

func TestDepthLimit(t *testing.T) {
	manager := &mockManager{repoCtx: createTestRepoContext()}
	visualizer := NewGraphVisualizer(manager)

	// Test that depth is clamped to max 5
	result, err := visualizer.GetCallTree(context.Background(), "test/repo", "CreateUser", 10)
	if err != nil {
		t.Fatalf("GetCallTree() error: %v", err)
	}

	if result == nil {
		t.Fatal("GetCallTree() returned nil")
	}

	// The result should still work with depth clamped to 5
	if result.Name != "CreateUser" {
		t.Errorf("GetCallTree() Name = %v, want CreateUser", result.Name)
	}
}

func TestEscapeForMermaid(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"SimpleFunction", "SimpleFunction"},
		{"Function<T>", "Function&lt;T&gt;"},
		{"Function\"Name\"", "Function#quot;Name#quot;"},
	}

	for _, tt := range tests {
		result := escapeForMermaid(tt.input)
		if result != tt.expected {
			t.Errorf("escapeForMermaid(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestEscapeForDOT(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"SimpleFunction", "SimpleFunction"},
		{"Function<T>", "Function\\<T\\>"},
		{"Function\"Name\"", "Function\\\"Name\\\""},
	}

	for _, tt := range tests {
		result := escapeForDOT(tt.input)
		if result != tt.expected {
			t.Errorf("escapeForDOT(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}
