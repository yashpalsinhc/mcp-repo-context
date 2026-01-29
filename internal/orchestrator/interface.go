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
