package prreview

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/yashpalc/mcp-repo-context/internal/ai"
	ctxpkg "github.com/yashpalc/mcp-repo-context/internal/context"
)

// Reviewer handles PR review logic.
type Reviewer struct {
	aiProvider ai.Provider
	ghClient   *GitHubClient
}

// NewReviewer creates a new PR reviewer.
func NewReviewer(aiProvider ai.Provider, ghToken string) *Reviewer {
	return &Reviewer{
		aiProvider: aiProvider,
		ghClient:   NewGitHubClient(ghToken),
	}
}

// Review performs a comprehensive review of the PR.
func (r *Reviewer) Review(ctx context.Context, prInfo *PRInfo, repoCtx *ctxpkg.RepoContext, opts ReviewOptions) (*ReviewResult, error) {
	startTime := time.Now()

	result := &ReviewResult{
		PRInfo:     *prInfo,
		HasContext: repoCtx != nil,
		ReviewedAt: startTime,
	}

	if repoCtx != nil {
		result.RepoID = repoCtx.ID
	}

	// Build context summary for AI
	var contextSummary *RepoContextSummary
	if repoCtx != nil {
		contextSummary = r.buildContextSummary(repoCtx, prInfo)
	}

	// Create AI review request
	reviewReq := AIReviewRequest{
		PRInfo:      *prInfo,
		RepoContext: contextSummary,
		FocusAreas:  opts.FocusAreas,
	}

	// Get AI review
	aiResp, err := r.getAIReview(ctx, reviewReq)
	if err != nil {
		return nil, fmt.Errorf("AI review failed: %w", err)
	}

	result.Comments = aiResp.Comments
	result.Summary = aiResp.Summary
	result.TokensUsed = aiResp.TokensUsed
	result.Provider = r.aiProvider.Name()

	// Add comments to GitHub if requested
	if opts.AddComments && len(result.Comments) > 0 {
		r.addCommentsToGitHub(ctx, prInfo, result, opts)
	}

	return result, nil
}

// buildContextSummary creates a condensed context summary for the AI.
func (r *Reviewer) buildContextSummary(repoCtx *ctxpkg.RepoContext, prInfo *PRInfo) *RepoContextSummary {
	summary := &RepoContextSummary{
		RepoID: repoCtx.ID,
	}

	// Add AI-generated summary if available
	if repoCtx.AISummary != nil {
		summary.Overview = repoCtx.AISummary.Overview
		summary.Purpose = repoCtx.AISummary.Purpose
		if repoCtx.AISummary.ArchitectureStyle != "" {
			summary.Architecture = repoCtx.AISummary.ArchitectureStyle
		}
	}

	// Add architecture patterns
	if repoCtx.Architecture != nil && repoCtx.Architecture.AIAnalysis != nil {
		summary.Architecture = repoCtx.Architecture.AIAnalysis.Pattern
		// Infer patterns from strengths/recommendations
		for _, s := range repoCtx.Architecture.AIAnalysis.Strengths {
			if len(summary.Patterns) < 5 {
				summary.Patterns = append(summary.Patterns, s)
			}
		}
	}

	// Find related files - files in same directories as changed files
	changedDirs := make(map[string]bool)
	for _, f := range prInfo.Files {
		dir := getDir(f.Path)
		changedDirs[dir] = true
	}

	// Add context for files in related directories
	for path, fileCtx := range repoCtx.Files {
		dir := getDir(path)
		if changedDirs[dir] && len(summary.RelatedFiles) < 10 {
			exports := make([]string, 0)
			for _, exp := range fileCtx.Exports {
				if len(exports) < 5 {
					exports = append(exports, exp.Name)
				}
			}
			summary.RelatedFiles = append(summary.RelatedFiles, RelatedFileSummary{
				Path:    path,
				Purpose: fileCtx.Purpose,
				Exports: exports,
			})
		}

		// Add related functions
		for _, fn := range fileCtx.Functions {
			if fn.IsPublic && len(summary.RelatedFuncs) < 15 {
				summary.RelatedFuncs = append(summary.RelatedFuncs, RelatedFuncSummary{
					Name:      fn.Name,
					File:      path,
					Signature: fn.Signature,
					Purpose:   fn.Description,
				})
			}
		}
	}

	return summary
}

// getDir returns the directory portion of a path.
func getDir(path string) string {
	lastSlash := strings.LastIndex(path, "/")
	if lastSlash == -1 {
		return "."
	}
	return path[:lastSlash]
}

// getAIReview sends the review request to the AI provider.
func (r *Reviewer) getAIReview(ctx context.Context, req AIReviewRequest) (*AIReviewResponse, error) {
	prompt := buildReviewPrompt(req)

	// Use the AI provider's complete method (we'll need to add this to the interface)
	response, tokensUsed, err := r.completeAI(ctx, prompt, 4096)
	if err != nil {
		return nil, err
	}

	// Parse the response
	aiResp, err := parseReviewResponse(response)
	if err != nil {
		// If parsing fails, create a basic response
		aiResp = &AIReviewResponse{
			Summary: ReviewSummary{
				Overall: response,
			},
		}
	}
	aiResp.TokensUsed = tokensUsed

	return aiResp, nil
}

// completeAI sends a completion request to the AI provider.
// This is a wrapper that adapts to the existing provider interface.
func (r *Reviewer) completeAI(ctx context.Context, prompt string, maxTokens int) (string, int, error) {
	// Use GenerateDescription as a general completion method
	// (In a real implementation, we'd add a Complete method to the Provider interface)
	req := ai.DescriptionRequest{
		CodeType:  "pull_request_review",
		Name:      "PR Review",
		Signature: "",
		Content:   prompt,
		Context:   "",
	}

	// We need to use the raw API - let's add a method to handle this
	// For now, use the anthropic provider directly
	if anthro, ok := r.aiProvider.(*ai.AnthropicProvider); ok {
		return anthro.CompleteRaw(ctx, prompt, maxTokens)
	}

	// Fallback: use GenerateDescription (limited)
	desc, err := r.aiProvider.GenerateDescription(ctx, req)
	return desc, 0, err
}

// addCommentsToGitHub adds review comments to the GitHub PR.
func (r *Reviewer) addCommentsToGitHub(ctx context.Context, prInfo *PRInfo, result *ReviewResult, opts ReviewOptions) {
	// Filter comments by severity level
	commentsToAdd := filterCommentsBySeverity(result.Comments, opts.SeverityLevel)

	for _, comment := range commentsToAdd {
		err := r.ghClient.AddReviewComment(ctx, prInfo.Owner, prInfo.Repo, prInfo.Number, prInfo.CommitSHA, comment)
		if err != nil {
			result.CommentsFailed++
			result.CommentErrors = append(result.CommentErrors, fmt.Sprintf("%s:%d - %v", comment.Path, comment.Line, err))
		} else {
			result.CommentsAdded++
		}
	}

	// Add summary comment
	summaryBody := formatSummaryComment(result.Summary)
	if err := r.ghClient.AddPRComment(ctx, prInfo.Owner, prInfo.Repo, prInfo.Number, summaryBody); err != nil {
		result.CommentErrors = append(result.CommentErrors, fmt.Sprintf("summary comment: %v", err))
	}
}

// filterCommentsBySeverity filters comments based on severity level.
func filterCommentsBySeverity(comments []ReviewComment, level string) []ReviewComment {
	if level == "all" {
		return comments
	}

	var filtered []ReviewComment
	for _, c := range comments {
		switch level {
		case "critical":
			if c.Severity == "critical" {
				filtered = append(filtered, c)
			}
		case "important":
			if c.Severity == "critical" || c.Severity == "important" {
				filtered = append(filtered, c)
			}
		}
	}
	return filtered
}

// formatSummaryComment formats the review summary as a GitHub comment.
func formatSummaryComment(summary ReviewSummary) string {
	var sb strings.Builder

	sb.WriteString("## 🤖 AI-Powered Code Review Summary\n\n")
	sb.WriteString(summary.Overall + "\n\n")

	sb.WriteString("### Overview\n\n")
	sb.WriteString(fmt.Sprintf("| Metric | Count |\n"))
	sb.WriteString(fmt.Sprintf("|--------|-------|\n"))
	sb.WriteString(fmt.Sprintf("| 🔴 Critical Issues | %d |\n", summary.CriticalIssues))
	sb.WriteString(fmt.Sprintf("| 🟡 Important Issues | %d |\n", summary.ImportantIssues))
	sb.WriteString(fmt.Sprintf("| 💡 Suggestions | %d |\n", summary.Suggestions))
	sb.WriteString(fmt.Sprintf("| ❓ Questions | %d |\n\n", summary.Questions))

	if len(summary.Strengths) > 0 {
		sb.WriteString("### ✅ Strengths\n\n")
		for _, s := range summary.Strengths {
			sb.WriteString(fmt.Sprintf("- %s\n", s))
		}
		sb.WriteString("\n")
	}

	if len(summary.Concerns) > 0 {
		sb.WriteString("### ⚠️ Concerns\n\n")
		for _, c := range summary.Concerns {
			sb.WriteString(fmt.Sprintf("- %s\n", c))
		}
		sb.WriteString("\n")
	}

	if summary.ImpactAnalysis != "" {
		sb.WriteString("### 📊 Impact Analysis\n\n")
		sb.WriteString(summary.ImpactAnalysis + "\n\n")
	}

	if summary.PatternNotes != "" {
		sb.WriteString("### 📐 Pattern Adherence\n\n")
		sb.WriteString(summary.PatternNotes + "\n\n")
	}

	if len(summary.Recommendations) > 0 {
		sb.WriteString("### 📝 Recommendations\n\n")
		for i, r := range summary.Recommendations {
			sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, r))
		}
		sb.WriteString("\n")
	}

	sb.WriteString("---\n")
	sb.WriteString("*Reviewed by MCP Repo Context AI*\n")

	return sb.String()
}

// buildReviewPrompt creates the prompt for the AI review.
func buildReviewPrompt(req AIReviewRequest) string {
	var sb strings.Builder

	sb.WriteString("You are an expert code reviewer. Review the following pull request and provide detailed feedback.\n\n")

	// PR Info
	sb.WriteString("## Pull Request\n\n")
	sb.WriteString(fmt.Sprintf("**Title:** %s\n", req.PRInfo.Title))
	sb.WriteString(fmt.Sprintf("**Author:** %s\n", req.PRInfo.Author))
	sb.WriteString(fmt.Sprintf("**Branch:** %s → %s\n\n", req.PRInfo.HeadBranch, req.PRInfo.BaseBranch))

	if req.PRInfo.Description != "" {
		sb.WriteString("**Description:**\n")
		sb.WriteString(req.PRInfo.Description + "\n\n")
	}

	// Repository Context (if available)
	if req.RepoContext != nil {
		sb.WriteString("## Repository Context\n\n")
		if req.RepoContext.Overview != "" {
			sb.WriteString(fmt.Sprintf("**Overview:** %s\n\n", req.RepoContext.Overview))
		}
		if req.RepoContext.Purpose != "" {
			sb.WriteString(fmt.Sprintf("**Purpose:** %s\n\n", req.RepoContext.Purpose))
		}
		if req.RepoContext.Architecture != "" {
			sb.WriteString(fmt.Sprintf("**Architecture:** %s\n\n", req.RepoContext.Architecture))
		}
		if len(req.RepoContext.Patterns) > 0 {
			sb.WriteString("**Patterns Used:**\n")
			for _, p := range req.RepoContext.Patterns {
				sb.WriteString(fmt.Sprintf("- %s\n", p))
			}
			sb.WriteString("\n")
		}
		if len(req.RepoContext.RelatedFuncs) > 0 {
			sb.WriteString("**Related Functions in Codebase:**\n")
			for _, fn := range req.RepoContext.RelatedFuncs[:min(10, len(req.RepoContext.RelatedFuncs))] {
				sb.WriteString(fmt.Sprintf("- `%s` in %s: %s\n", fn.Name, fn.File, fn.Signature))
			}
			sb.WriteString("\n")
		}
	}

	// Files changed
	sb.WriteString("## Changed Files\n\n")
	for _, f := range req.PRInfo.Files {
		sb.WriteString(fmt.Sprintf("- `%s` (+%d/-%d)\n", f.Path, f.Additions, f.Deletions))
	}
	sb.WriteString("\n")

	// Diff
	sb.WriteString("## Diff\n\n")
	sb.WriteString("```diff\n")
	// Truncate diff if too long
	diff := req.PRInfo.Diff
	if len(diff) > 15000 {
		diff = diff[:15000] + "\n... (truncated)"
	}
	sb.WriteString(diff)
	sb.WriteString("\n```\n\n")

	// Focus areas
	if len(req.FocusAreas) > 0 {
		sb.WriteString("**Focus Areas:** " + strings.Join(req.FocusAreas, ", ") + "\n\n")
	}

	// Instructions
	sb.WriteString(`## Review Instructions

Please analyze this PR and provide your review in the following JSON format:

{
  "comments": [
    {
      "path": "file/path.go",
      "line": 42,
      "side": "RIGHT",
      "body": "**Issue Type:** Description of the issue\n\nExplanation...\n\n**Suggestion:**\n` + "```" + `go\n// suggested code\n` + "```" + `",
      "severity": "critical|important|suggestion|question",
      "category": "bug|security|performance|style|architecture"
    }
  ],
  "summary": {
    "overall": "Overall assessment of the PR (2-3 sentences)",
    "pr_quality": "good|needs_work|problematic",
    "critical_issues": 0,
    "important_issues": 0,
    "suggestions": 0,
    "questions": 0,
    "strengths": ["Good thing 1", "Good thing 2"],
    "concerns": ["Concern 1", "Concern 2"],
    "recommendations": ["Before merging, fix...", "Consider..."],
    "impact_analysis": "How this change affects the codebase",
    "pattern_notes": "Notes about adherence to existing patterns (if context available)"
  }
}

## Review Checklist

Consider these aspects:
1. **Correctness**: Does the code do what it's supposed to do?
2. **Security**: Any security vulnerabilities?
3. **Performance**: Any performance issues?
4. **Error Handling**: Are errors handled properly?
5. **Code Style**: Is it consistent with the codebase?
6. **Architecture**: Does it fit the existing architecture?
7. **Tests**: Are there adequate tests?
8. **Documentation**: Is it documented appropriately?

## Comment Format Guidelines

Use these severity levels:
- **critical**: Must fix before merge (bugs, security issues, data loss)
- **important**: Should fix (error handling, performance, incomplete implementation)
- **suggestion**: Nice to have (style improvements, minor optimizations)
- **question**: Need clarification

Return ONLY the JSON, no additional text.`)

	return sb.String()
}

// parseReviewResponse parses the AI response into a ReviewResponse.
func parseReviewResponse(response string) (*AIReviewResponse, error) {
	response = strings.TrimSpace(response)

	// Remove markdown code blocks if present
	if strings.HasPrefix(response, "```") {
		lines := strings.Split(response, "\n")
		var jsonLines []string
		inJSON := false
		for _, line := range lines {
			if strings.HasPrefix(line, "```") {
				inJSON = !inJSON
				continue
			}
			if inJSON || !strings.HasPrefix(line, "```") {
				jsonLines = append(jsonLines, line)
			}
		}
		response = strings.Join(jsonLines, "\n")
	}

	var aiResp AIReviewResponse
	if err := json.Unmarshal([]byte(response), &aiResp); err != nil {
		return nil, fmt.Errorf("failed to parse review response: %w", err)
	}

	return &aiResp, nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
