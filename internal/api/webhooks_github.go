package api

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// GitHubPushPayload represents a GitHub push webhook event.
type GitHubPushPayload struct {
	Ref        string `json:"ref"`
	Repository struct {
		CloneURL      string `json:"clone_url"`
		DefaultBranch string `json:"default_branch"`
		FullName      string `json:"full_name"`
	} `json:"repository"`
	Commits []struct {
		Added    []string `json:"added"`
		Modified []string `json:"modified"`
		Removed  []string `json:"removed"`
	} `json:"commits"`
}

// GitHubPRPayload represents a GitHub pull_request webhook event.
type GitHubPRPayload struct {
	Action      string `json:"action"`
	PullRequest struct {
		Number int `json:"number"`
		Head   struct {
			Repo struct {
				CloneURL string `json:"clone_url"`
				FullName string `json:"full_name"`
			} `json:"repo"`
		} `json:"head"`
	} `json:"pull_request"`
}

// verifyGitHubSignature validates HMAC-SHA256 signature from GitHub.
func verifyGitHubSignature(body []byte, signature string, secret string) bool {
	if signature == "" || secret == "" {
		return false
	}

	// Parse the sha256= prefix
	if !strings.HasPrefix(signature, "sha256=") {
		return false
	}
	sigHex := strings.TrimPrefix(signature, "sha256=")

	// Compute expected MAC
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	expectedMAC := hex.EncodeToString(mac.Sum(nil))

	return hmac.Equal([]byte(sigHex), []byte(expectedMAC))
}

// branchFromRef extracts the branch name from a git ref.
func branchFromRef(ref string) string {
	return strings.TrimPrefix(ref, "refs/heads/")
}

// isDocOnlyChange returns true if all changed files are .md files.
func isDocOnlyChange(payload *GitHubPushPayload) bool {
	for _, commit := range payload.Commits {
		for _, f := range commit.Added {
			if !strings.HasSuffix(f, ".md") {
				return false
			}
		}
		for _, f := range commit.Modified {
			if !strings.HasSuffix(f, ".md") {
				return false
			}
		}
		for _, f := range commit.Removed {
			if !strings.HasSuffix(f, ".md") {
				return false
			}
		}
	}
	return true
}
