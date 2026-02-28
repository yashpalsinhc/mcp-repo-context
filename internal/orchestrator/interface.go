package orchestrator

import (
	"context"
	"time"

	ctxpkg "github.com/yashpalc/mcp-repo-context/internal/context"
	"github.com/yashpalc/mcp-repo-context/internal/prreview"
)

// Manager orchestrates repository analysis and context management.
type Manager interface {
	// AnalyzeRepo analyzes a repository and stores the context.
	AnalyzeRepo(ctx context.Context, repoURL string, opts AnalyzeOptions) (*AnalyzeResult, error)

	// GetContext retrieves repository context.
	GetContext(ctx context.Context, repoID string) (*ctxpkg.RepoContext, error)

	// GetFileContext retrieves context for a specific file.
	GetFileContext(ctx context.Context, repoID, filePath string) (*ctxpkg.FileContext, error)

	// GetFunctionContext retrieves comprehensive context for a specific function.
	GetFunctionContext(ctx context.Context, repoID, filePath, funcName string) (*FunctionContextResult, error)

	// SearchFunctions searches for functions by query.
	SearchFunctions(ctx context.Context, repoID, query string) ([]ctxpkg.FunctionRef, error)

	// SearchByConcept searches for functions related to a concept.
	SearchByConcept(ctx context.Context, repoID, concept string) ([]ctxpkg.FunctionRef, error)

	// SearchBySideEffect searches for functions with specific side effects.
	SearchBySideEffect(ctx context.Context, repoID, effect string) ([]ctxpkg.FunctionRef, error)

	// ListRepos returns all analyzed repositories.
	ListRepos(ctx context.Context) ([]ctxpkg.ContextMetadata, error)

	// GenerateAISummary generates AI-powered summary for a repository.
	GenerateAISummary(ctx context.Context, repoID string) (*ctxpkg.AISummary, error)

	// GenerateAIArchAnalysis generates AI-powered architecture analysis.
	GenerateAIArchAnalysis(ctx context.Context, repoID string) (*ctxpkg.AIArchAnalysis, error)

	// IsAIEnabled returns true if AI features are available.
	IsAIEnabled() bool

	// Ask answers a natural language query about repositories using AI.
	Ask(ctx context.Context, query string, repoIDs []string) (*QueryResult, error)

	// RefreshAIContext updates all repositories with AI-generated summaries.
	// If force is true, re-generates AI context even if it already exists.
	RefreshAIContext(ctx context.Context, repoIDs []string, force bool) (*RefreshResult, error)

	// ReviewPR reviews a pull request using AI and repository context.
	ReviewPR(ctx context.Context, prURL string, opts prreview.ReviewOptions) (*prreview.ReviewResult, error)

	// CheckPRContext checks if context exists for the PR's repository.
	CheckPRContext(ctx context.Context, prURL string) (*prreview.ContextStatus, error)

	// AnalyzeLocal analyzes a local directory and stores the context.
	AnalyzeLocal(ctx context.Context, dirPath string, opts AnalyzeLocalOptions) (*AnalyzeLocalResult, error)

	// GetOrAnalyzeLocal gets existing context or analyzes if not present.
	GetOrAnalyzeLocal(ctx context.Context, dirPath string) (*ctxpkg.RepoContext, error)

	// SmartQuery routes queries to appropriate tools without AI.
	SmartQuery(ctx context.Context, query string, projectID string) (*SmartQueryResult, error)

	// RefreshFile re-analyzes a single file (fast incremental update).
	RefreshFile(ctx context.Context, projectID, filePath string, opts RefreshFileOptions) (*RefreshFileResult, error)

	// RefreshChangedFiles checks all files and refreshes only changed ones.
	RefreshChangedFiles(ctx context.Context, projectID string) ([]RefreshFileResult, error)

	// CheckFileStale checks if a file's context is stale without refreshing.
	CheckFileStale(ctx context.Context, projectID, filePath string) (bool, error)

	// GetPRContext extracts rich context for PR changes (no AI required).
	GetPRContext(ctx context.Context, repoID string, changedFiles []ChangedFile) (*PRContextResult, error)

	// DeleteRepoContext removes all stored context for a repository.
	DeleteRepoContext(ctx context.Context, repoID string) error

	// GetDependencyGraph builds a cross-repo dependency graph from stored ModuleInfo.
	GetDependencyGraph(ctx context.Context, repoIDs []string, includeExternal bool) (*ctxpkg.DependencyGraph, error)
}

// RefreshResult contains results of refreshing AI context.
type RefreshResult struct {
	Updated   []string `json:"updated"`   // Repos that were updated
	Skipped   []string `json:"skipped"`   // Repos that already had AI context
	Failed    []string `json:"failed"`    // Repos that failed
	Errors    []string `json:"errors"`    // Error messages
	TokensUsed int     `json:"tokens_used"`
}

// QueryResult contains the answer to a natural language query.
type QueryResult struct {
	Answer      string       `json:"answer"`
	Sources     []SourceRef  `json:"sources"`
	Confidence  float64      `json:"confidence"`
	TokensUsed  int          `json:"tokens_used"`
	ContextUsed []string     `json:"context_used"`
	QueryType   string       `json:"query_type"`
}

// SourceRef references a location in the code.
type SourceRef struct {
	RepoID   string `json:"repo_id"`
	FilePath string `json:"file_path"`
	Function string `json:"function,omitempty"`
	Line     int    `json:"line,omitempty"`
}

// AnalyzeOptions configures repository analysis.
type AnalyzeOptions struct {
	Branch        string
	Force         bool
	GitHubToken   string
	MaxAge        time.Duration
	EnableAI      bool   // Enable AI-powered analysis
	AIProvider    string // Specific AI provider to use (default: auto-detect)
	AnalyzerName  string // Override analyzer selection by name (empty = default)
}

// AnalyzeResult contains analysis results.
type AnalyzeResult struct {
	RepoID       string
	FileCount    int
	NewFiles     int
	UpdatedFiles int
	Duration     time.Duration
	Warnings     []string
}
