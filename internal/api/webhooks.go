package api

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
)

// handleGitHubWebhook processes GitHub webhook events.
func (s *APIServer) handleGitHubWebhook(w http.ResponseWriter, r *http.Request) {
	if s.config.GithubWebhookSecret == "" {
		writeError(w, r, http.StatusServiceUnavailable, "not_configured", "GitHub webhook secret not configured")
		return
	}

	// Read body for HMAC verification
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "read_error", "failed to read request body")
		return
	}

	// Verify signature
	sig := r.Header.Get("X-Hub-Signature-256")
	if sig == "" {
		writeError(w, r, http.StatusUnauthorized, "unauthorized", "missing signature")
		return
	}
	if !verifyGitHubSignature(body, sig, s.config.GithubWebhookSecret) {
		writeError(w, r, http.StatusUnauthorized, "unauthorized", "invalid signature")
		return
	}

	// Route by event type
	event := r.Header.Get("X-GitHub-Event")
	switch event {
	case "push":
		s.handleGitHubPush(w, r, body)
	case "pull_request":
		s.handleGitHubPR(w, r, body)
	default:
		writeJSON(w, r, http.StatusOK, map[string]string{"status": "ignored"}, nil)
	}
}

func (s *APIServer) handleGitHubPush(w http.ResponseWriter, r *http.Request, body []byte) {
	var payload GitHubPushPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_json", "invalid push payload")
		return
	}

	// Filter by branch
	branch := branchFromRef(payload.Ref)
	if branch != payload.Repository.DefaultBranch && !s.isAllowedBranch(branch) {
		writeJSON(w, r, http.StatusOK, map[string]string{"status": "skipped", "reason": "non-default branch"}, nil)
		return
	}

	// Skip doc-only changes
	if len(payload.Commits) > 0 && isDocOnlyChange(&payload) {
		writeJSON(w, r, http.StatusOK, map[string]string{"status": "skipped", "reason": "docs only"}, nil)
		return
	}

	// Enqueue analysis job
	if s.jobQueue != nil {
		payloadJSON, _ := json.Marshal(map[string]string{
			"repo_url": payload.Repository.CloneURL,
		})
		repoID := payload.Repository.FullName
		jobID, err := s.jobQueue.Enqueue(r.Context(), "analyze_repo", "", repoID, string(payloadJSON))
		if err != nil {
			writeError(w, r, http.StatusInternalServerError, "queue_error", err.Error())
			return
		}
		writeJSON(w, r, http.StatusOK, map[string]string{"status": "accepted", "job_id": jobID}, nil)
		return
	}

	writeJSON(w, r, http.StatusOK, map[string]string{"status": "accepted"}, nil)
}

func (s *APIServer) handleGitHubPR(w http.ResponseWriter, r *http.Request, body []byte) {
	var payload GitHubPRPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_json", "invalid PR payload")
		return
	}

	// Only process opened and synchronize actions
	if payload.Action != "opened" && payload.Action != "synchronize" {
		writeJSON(w, r, http.StatusOK, map[string]string{"status": "ignored"}, nil)
		return
	}

	if s.jobQueue != nil {
		payloadJSON, _ := json.Marshal(map[string]interface{}{
			"repo_id":  payload.PullRequest.Head.Repo.FullName,
			"pr_number": payload.PullRequest.Number,
		})
		repoID := payload.PullRequest.Head.Repo.FullName
		jobID, err := s.jobQueue.Enqueue(r.Context(), "pr_context", "", repoID, string(payloadJSON))
		if err != nil {
			writeError(w, r, http.StatusInternalServerError, "queue_error", err.Error())
			return
		}
		writeJSON(w, r, http.StatusOK, map[string]string{"status": "accepted", "job_id": jobID}, nil)
		return
	}

	writeJSON(w, r, http.StatusOK, map[string]string{"status": "accepted"}, nil)
}

// handleGitLabWebhook processes GitLab webhook events.
func (s *APIServer) handleGitLabWebhook(w http.ResponseWriter, r *http.Request) {
	if s.config.GitlabWebhookSecret == "" {
		writeError(w, r, http.StatusServiceUnavailable, "not_configured", "GitLab webhook secret not configured")
		return
	}

	// Verify token
	token := r.Header.Get("X-Gitlab-Token")
	if token == "" {
		writeError(w, r, http.StatusUnauthorized, "unauthorized", "missing token")
		return
	}
	if !verifyGitLabToken(token, s.config.GitlabWebhookSecret) {
		writeError(w, r, http.StatusUnauthorized, "unauthorized", "invalid token")
		return
	}

	// Parse body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "read_error", "failed to read request body")
		return
	}

	// Determine event type from payload
	var base struct {
		ObjectKind string `json:"object_kind"`
	}
	if err := json.Unmarshal(body, &base); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_json", "invalid payload")
		return
	}

	switch base.ObjectKind {
	case "push":
		s.handleGitLabPush(w, r, body)
	case "merge_request":
		writeJSON(w, r, http.StatusOK, map[string]string{"status": "accepted"}, nil)
	default:
		writeJSON(w, r, http.StatusOK, map[string]string{"status": "ignored"}, nil)
	}
}

func (s *APIServer) handleGitLabPush(w http.ResponseWriter, r *http.Request, body []byte) {
	var payload GitLabPushPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_json", "invalid push payload")
		return
	}

	// Filter by branch
	branch := branchFromRef(payload.Ref)
	if branch != payload.Project.DefaultBranch && !s.isAllowedBranch(branch) {
		writeJSON(w, r, http.StatusOK, map[string]string{"status": "skipped", "reason": "non-default branch"}, nil)
		return
	}

	if s.jobQueue != nil {
		payloadJSON, _ := json.Marshal(map[string]string{
			"repo_url": payload.Project.GitHTTPURL,
		})
		repoID := payload.Project.PathWithNS
		jobID, err := s.jobQueue.Enqueue(r.Context(), "analyze_repo", "", repoID, string(payloadJSON))
		if err != nil {
			writeError(w, r, http.StatusInternalServerError, "queue_error", err.Error())
			return
		}
		writeJSON(w, r, http.StatusOK, map[string]string{"status": "accepted", "job_id": jobID}, nil)
		return
	}

	writeJSON(w, r, http.StatusOK, map[string]string{"status": "accepted"}, nil)
}

// isAllowedBranch checks if the branch is in the configured allowed list.
func (s *APIServer) isAllowedBranch(branch string) bool {
	// For now, only default branch is allowed unless configured
	// This could be extended with WEBHOOK_BRANCHES env var
	_ = branch
	return false
}

// webhookBranches parses a comma-separated list of allowed branches.
func webhookBranches(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	var result []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}
