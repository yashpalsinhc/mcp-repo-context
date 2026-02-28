package api

import (
	"crypto/subtle"
)

// GitLabPushPayload represents a GitLab push webhook event.
type GitLabPushPayload struct {
	ObjectKind string `json:"object_kind"`
	Ref        string `json:"ref"`
	Project    struct {
		GitHTTPURL    string `json:"git_http_url"`
		DefaultBranch string `json:"default_branch"`
		PathWithNS    string `json:"path_with_namespace"`
	} `json:"project"`
}

// verifyGitLabToken validates the X-Gitlab-Token header using constant-time comparison.
func verifyGitLabToken(token, secret string) bool {
	if token == "" || secret == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(token), []byte(secret)) == 1
}
