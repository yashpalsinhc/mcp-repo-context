package prreview

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	githubAPIURL = "https://api.github.com"
)

// ParsePRURL extracts owner, repo, and PR number from a GitHub PR URL.
func ParsePRURL(prURL string) (owner, repo string, number int, err error) {
	// Parse URL
	u, err := url.Parse(prURL)
	if err != nil {
		return "", "", 0, fmt.Errorf("invalid URL: %w", err)
	}

	// Expected format: github.com/owner/repo/pull/123
	if u.Host != "github.com" && u.Host != "www.github.com" {
		return "", "", 0, fmt.Errorf("not a GitHub URL: %s", u.Host)
	}

	// Parse path
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) < 4 || parts[2] != "pull" {
		return "", "", 0, fmt.Errorf("invalid PR URL format, expected: github.com/owner/repo/pull/number")
	}

	owner = parts[0]
	repo = parts[1]
	number, err = strconv.Atoi(parts[3])
	if err != nil {
		return "", "", 0, fmt.Errorf("invalid PR number: %s", parts[3])
	}

	return owner, repo, number, nil
}

// GitHubClient interacts with GitHub using the REST API.
type GitHubClient struct {
	token  string
	client *http.Client
}

// NewGitHubClient creates a new GitHub client.
func NewGitHubClient(token string) *GitHubClient {
	return &GitHubClient{
		token: token,
		client: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

// FetchPRInfo fetches pull request information using GitHub REST API.
func (c *GitHubClient) FetchPRInfo(ctx context.Context, owner, repo string, number int) (*PRInfo, error) {
	// Fetch PR metadata
	prURL := fmt.Sprintf("%s/repos/%s/%s/pulls/%d", githubAPIURL, owner, repo, number)

	var prData struct {
		Title string `json:"title"`
		Body  string `json:"body"`
		User  struct {
			Login string `json:"login"`
		} `json:"user"`
		Base struct {
			Ref string `json:"ref"`
		} `json:"base"`
		Head struct {
			Ref string `json:"ref"`
			SHA string `json:"sha"`
		} `json:"head"`
		CreatedAt time.Time `json:"created_at"`
		UpdatedAt time.Time `json:"updated_at"`
	}

	if err := c.doRequest(ctx, "GET", prURL, nil, &prData); err != nil {
		return nil, fmt.Errorf("failed to fetch PR info: %w", err)
	}

	// Fetch PR files
	filesURL := fmt.Sprintf("%s/repos/%s/%s/pulls/%d/files", githubAPIURL, owner, repo, number)
	var filesData []struct {
		Filename  string `json:"filename"`
		Status    string `json:"status"`
		Additions int    `json:"additions"`
		Deletions int    `json:"deletions"`
		Patch     string `json:"patch"`
	}

	if err := c.doRequest(ctx, "GET", filesURL, nil, &filesData); err != nil {
		return nil, fmt.Errorf("failed to fetch PR files: %w", err)
	}

	// Build diff from patches
	var diffBuilder strings.Builder
	var files []PRFile
	for _, f := range filesData {
		files = append(files, PRFile{
			Path:      f.Filename,
			Status:    f.Status,
			Additions: f.Additions,
			Deletions: f.Deletions,
			Patch:     f.Patch,
		})

		// Build combined diff
		if f.Patch != "" {
			diffBuilder.WriteString(fmt.Sprintf("diff --git a/%s b/%s\n", f.Filename, f.Filename))
			diffBuilder.WriteString(f.Patch)
			diffBuilder.WriteString("\n")
		}
	}

	// Build PR info
	prInfo := &PRInfo{
		URL:         fmt.Sprintf("https://github.com/%s/%s/pull/%d", owner, repo, number),
		Owner:       owner,
		Repo:        repo,
		Number:      number,
		Title:       prData.Title,
		Description: prData.Body,
		Author:      prData.User.Login,
		BaseBranch:  prData.Base.Ref,
		HeadBranch:  prData.Head.Ref,
		CommitSHA:   prData.Head.SHA,
		Diff:        diffBuilder.String(),
		Files:       files,
		CreatedAt:   prData.CreatedAt,
		UpdatedAt:   prData.UpdatedAt,
	}

	return prInfo, nil
}

// GetExistingComments fetches existing review comments on the PR.
func (c *GitHubClient) GetExistingComments(ctx context.Context, owner, repo string, number int) ([]string, error) {
	commentsURL := fmt.Sprintf("%s/repos/%s/%s/pulls/%d/comments", githubAPIURL, owner, repo, number)

	var commentsData []struct {
		Body string `json:"body"`
	}

	if err := c.doRequest(ctx, "GET", commentsURL, nil, &commentsData); err != nil {
		// Return empty if error
		return nil, nil
	}

	var comments []string
	for _, c := range commentsData {
		comments = append(comments, c.Body)
	}

	return comments, nil
}

// AddReviewComment adds a review comment to a specific line in the PR.
func (c *GitHubClient) AddReviewComment(ctx context.Context, owner, repo string, number int, commitSHA string, comment ReviewComment) error {
	commentsURL := fmt.Sprintf("%s/repos/%s/%s/pulls/%d/comments", githubAPIURL, owner, repo, number)

	// Calculate position in diff
	position := comment.Line
	if position <= 0 {
		position = 1
	}

	payload := map[string]interface{}{
		"commit_id": commitSHA,
		"path":      comment.Path,
		"position":  position,
		"body":      comment.Body,
	}

	return c.doRequest(ctx, "POST", commentsURL, payload, nil)
}

// AddPRComment adds a general comment to the PR (not line-specific).
func (c *GitHubClient) AddPRComment(ctx context.Context, owner, repo string, number int, body string) error {
	// Use issues API for general PR comments
	commentsURL := fmt.Sprintf("%s/repos/%s/%s/issues/%d/comments", githubAPIURL, owner, repo, number)

	payload := map[string]interface{}{
		"body": body,
	}

	return c.doRequest(ctx, "POST", commentsURL, payload, nil)
}

// SubmitReview submits a formal review with a summary and optional approval/request changes.
func (c *GitHubClient) SubmitReview(ctx context.Context, owner, repo string, number int, body string, event string) error {
	reviewsURL := fmt.Sprintf("%s/repos/%s/%s/pulls/%d/reviews", githubAPIURL, owner, repo, number)

	payload := map[string]interface{}{
		"body":  body,
		"event": event, // APPROVE, REQUEST_CHANGES, COMMENT
	}

	return c.doRequest(ctx, "POST", reviewsURL, payload, nil)
}

// doRequest performs an HTTP request to the GitHub API.
func (c *GitHubClient) doRequest(ctx context.Context, method, url string, body interface{}, result interface{}) error {
	var bodyReader io.Reader
	if body != nil {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("failed to marshal request body: %w", err)
		}
		bodyReader = bytes.NewReader(jsonBody)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "mcp-repo-context")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	// Check for error status
	if resp.StatusCode >= 400 {
		var errResp struct {
			Message string `json:"message"`
		}
		if json.Unmarshal(respBody, &errResp) == nil && errResp.Message != "" {
			return fmt.Errorf("GitHub API error (%d): %s", resp.StatusCode, errResp.Message)
		}
		return fmt.Errorf("GitHub API error (%d): %s", resp.StatusCode, string(respBody))
	}

	// Parse result if provided
	if result != nil && len(respBody) > 0 {
		if err := json.Unmarshal(respBody, result); err != nil {
			return fmt.Errorf("failed to parse response: %w", err)
		}
	}

	return nil
}

// enrichFilesWithPatches adds patch content to each file from the diff.
func (c *GitHubClient) enrichFilesWithPatches(files []PRFile, diff string) []PRFile {
	// Parse diff into file patches
	filePatchRegex := regexp.MustCompile(`(?m)^diff --git a/(.+) b/(.+)$`)
	matches := filePatchRegex.FindAllStringSubmatchIndex(diff, -1)

	patches := make(map[string]string)
	for i, match := range matches {
		if len(match) >= 6 {
			filePath := diff[match[4]:match[5]] // b/path
			startIdx := match[0]
			endIdx := len(diff)
			if i < len(matches)-1 {
				endIdx = matches[i+1][0]
			}
			patches[filePath] = diff[startIdx:endIdx]
		}
	}

	// Enrich files
	for i := range files {
		if patch, ok := patches[files[i].Path]; ok {
			files[i].Patch = patch
		}
	}

	return files
}

// RepoIDFromPR generates a repo ID from PR info (matches the format used by analyze_repo).
func RepoIDFromPR(owner, repo string) string {
	return fmt.Sprintf("github.com/%s/%s", owner, repo)
}
