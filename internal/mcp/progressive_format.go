package mcp

import (
	"fmt"
	"strings"

	"github.com/yashpalc/mcp-repo-context/internal/search"
)

// MakeOrgFunctionDetailRef creates a pipe-separated detail_ref for org search results.
// Uses pipe instead of colon to avoid ambiguity with colons in repo IDs like "github.com/org/repo".
func MakeOrgFunctionDetailRef(repoID, filePath, funcName string) string {
	return "func|" + repoID + "|" + filePath + "|" + funcName
}

// ExpandDetailRef parses a pipe-separated detail_ref into its components.
func ExpandDetailRef(ref string) (refType, repoID, filePath, funcName string, err error) {
	parts := strings.Split(ref, "|")
	if len(parts) != 4 {
		return "", "", "", "", fmt.Errorf("invalid detail_ref format: expected 4 pipe-separated parts, got %d", len(parts))
	}
	return parts[0], parts[1], parts[2], parts[3], nil
}

// shortRepoName extracts the last path segment from a repo ID.
// "github.com/org/user-service" → "user-service"
func shortRepoName(repoID string) string {
	parts := strings.Split(repoID, "/")
	if len(parts) == 0 {
		return repoID
	}
	return parts[len(parts)-1]
}

// FormatOrgSearchResult formats ranked search results as token-budgeted markdown.
func FormatOrgSearchResult(query, orgID string, results []search.RankedResult, tokenBudget int) string {
	if tokenBudget <= 0 {
		tokenBudget = 4000
	}

	if len(results) == 0 {
		return fmt.Sprintf("## Search Results: %q across %s\n\nNo results found for %q.", query, orgID, query)
	}

	// Count unique repos
	repoSet := make(map[string]bool)
	for _, r := range results {
		repoSet[r.FunctionRef.RepoID] = true
	}

	var sb strings.Builder

	// Header
	header := fmt.Sprintf("## Search Results: %q across %s\n\nFound %d results across %d repos\n\n",
		query, orgID, len(results), len(repoSet))
	sb.WriteString(header)

	headerTokens := len(header) / 4
	remainingBudget := tokenBudget - headerTokens
	shown := 0

	for i, r := range results {
		line := formatRankedResultLine(i+1, r)
		lineTokens := len(line) / 4

		if lineTokens > remainingBudget && shown > 0 {
			remaining := len(results) - shown
			sb.WriteString(fmt.Sprintf("\n... and %d more results. Use detail_ref to expand individual results.\n", remaining))
			sb.WriteString(fmt.Sprintf("\nToken budget: %d/%d\n", tokenBudget-remainingBudget, tokenBudget))
			return sb.String()
		}

		sb.WriteString(line)
		remainingBudget -= lineTokens
		shown++
	}

	return sb.String()
}

// formatRankedResultLine formats a single ranked result as a markdown line.
func formatRankedResultLine(rank int, rr search.RankedResult) string {
	ref := rr.FunctionRef
	shortRepo := shortRepoName(ref.RepoID)
	detailRef := MakeOrgFunctionDetailRef(ref.RepoID, ref.File, ref.Function)

	var location string
	if ref.Line > 0 {
		location = fmt.Sprintf("`%s:%d`", ref.File, ref.Line)
	} else {
		location = fmt.Sprintf("`%s`", ref.File)
	}

	summary := ref.Summary
	if summary == "" {
		summary = "(no summary)"
	}

	return fmt.Sprintf("%d. **%s** (%s) - %s\n   %s\n   → detail: `%s`\n\n",
		rank, ref.Function, shortRepo, location, summary, detailRef)
}
