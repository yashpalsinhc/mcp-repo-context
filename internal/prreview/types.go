package prreview

import "time"

// PRInfo contains information about a pull request.
type PRInfo struct {
	URL         string     `json:"url"`
	Owner       string     `json:"owner"`
	Repo        string     `json:"repo"`
	Number      int        `json:"number"`
	Title       string     `json:"title"`
	Description string     `json:"description"`
	Author      string     `json:"author"`
	BaseBranch  string     `json:"base_branch"`
	HeadBranch  string     `json:"head_branch"`
	CommitSHA   string     `json:"commit_sha"`
	Files       []PRFile   `json:"files"`
	Diff        string     `json:"diff"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// PRFile represents a file changed in the PR.
type PRFile struct {
	Path      string `json:"path"`
	Status    string `json:"status"` // added, modified, deleted, renamed
	Additions int    `json:"additions"`
	Deletions int    `json:"deletions"`
	Patch     string `json:"patch"`
}

// ReviewOptions configures the PR review.
type ReviewOptions struct {
	// GitHub authentication
	GitHubToken string

	// Review settings
	AddComments   bool     // Whether to add comments to the PR
	SeverityLevel string   // "critical", "important", "all" - what level of issues to report
	FocusAreas    []string // Optional areas to focus on: "security", "performance", "architecture", etc.

	// Context settings
	UseRepoContext    bool // Use stored repo context for review
	GenerateIfMissing bool // Offer to generate context if missing
}

// DefaultReviewOptions returns sensible defaults.
func DefaultReviewOptions() ReviewOptions {
	return ReviewOptions{
		AddComments:       true,
		SeverityLevel:     "all",
		UseRepoContext:    true,
		GenerateIfMissing: true,
	}
}

// ReviewComment represents a comment to add to the PR.
type ReviewComment struct {
	Path       string `json:"path"`
	Line       int    `json:"line"`      // Line in the new version of the file
	Side       string `json:"side"`      // "LEFT" or "RIGHT"
	Body       string `json:"body"`
	Severity   string `json:"severity"`  // "critical", "important", "suggestion", "question"
	Category   string `json:"category"`  // "bug", "security", "performance", "style", "architecture"
	Suggestion string `json:"suggestion,omitempty"` // Suggested fix if applicable
}

// ReviewResult contains the complete review output.
type ReviewResult struct {
	PRInfo     PRInfo          `json:"pr_info"`
	RepoID     string          `json:"repo_id"`
	HasContext bool            `json:"has_context"`

	// Review findings
	Comments     []ReviewComment `json:"comments"`
	Summary      ReviewSummary   `json:"summary"`

	// Comments added to GitHub
	CommentsAdded   int      `json:"comments_added"`
	CommentsFailed  int      `json:"comments_failed"`
	CommentErrors   []string `json:"comment_errors,omitempty"`

	// Metadata
	TokensUsed   int       `json:"tokens_used"`
	ReviewedAt   time.Time `json:"reviewed_at"`
	Provider     string    `json:"provider"`
}

// ReviewSummary provides a high-level summary of the review.
type ReviewSummary struct {
	Overall         string   `json:"overall"`           // Overall assessment
	PRQuality       string   `json:"pr_quality"`        // Quality assessment: "good", "needs_work", "problematic"
	CriticalIssues  int      `json:"critical_issues"`
	ImportantIssues int      `json:"important_issues"`
	Suggestions     int      `json:"suggestions"`
	Questions       int      `json:"questions"`
	Strengths       []string `json:"strengths"`         // What's good about this PR
	Concerns        []string `json:"concerns"`          // Main concerns
	Recommendations []string `json:"recommendations"`   // What to do next
	ImpactAnalysis  string   `json:"impact_analysis"`   // How this change affects the codebase
	PatternNotes    string   `json:"pattern_notes"`     // Notes about pattern adherence
}

// ContextStatus indicates whether repo context is available.
type ContextStatus struct {
	RepoID    string    `json:"repo_id"`
	HasContext bool     `json:"has_context"`
	LastAnalyzed time.Time `json:"last_analyzed,omitempty"`
	Message    string    `json:"message"`
}

// AIReviewRequest is sent to the AI provider for review.
type AIReviewRequest struct {
	PRInfo      PRInfo                `json:"pr_info"`
	RepoContext *RepoContextSummary   `json:"repo_context,omitempty"`
	FocusAreas  []string              `json:"focus_areas,omitempty"`
}

// RepoContextSummary is a condensed version of repo context for AI.
type RepoContextSummary struct {
	RepoID         string                `json:"repo_id"`
	Overview       string                `json:"overview,omitempty"`
	Purpose        string                `json:"purpose,omitempty"`
	Architecture   string                `json:"architecture,omitempty"`
	Patterns       []string              `json:"patterns,omitempty"`
	Conventions    []string              `json:"conventions,omitempty"`
	RelatedFiles   []RelatedFileSummary  `json:"related_files,omitempty"`
	RelatedFuncs   []RelatedFuncSummary  `json:"related_functions,omitempty"`
}

// RelatedFileSummary is a summary of a file relevant to the PR.
type RelatedFileSummary struct {
	Path     string   `json:"path"`
	Purpose  string   `json:"purpose"`
	Exports  []string `json:"exports,omitempty"`
}

// RelatedFuncSummary is a summary of a function relevant to the PR.
type RelatedFuncSummary struct {
	Name      string `json:"name"`
	File      string `json:"file"`
	Signature string `json:"signature"`
	Purpose   string `json:"purpose,omitempty"`
}

// AIReviewResponse is the parsed AI response.
type AIReviewResponse struct {
	Comments        []ReviewComment `json:"comments"`
	Summary         ReviewSummary   `json:"summary"`
	TokensUsed      int             `json:"tokens_used"`
}
