package api

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yashpalc/mcp-repo-context/internal/logging"
	"github.com/yashpalc/mcp-repo-context/internal/queue"
)

func computeGitHubSignature(body []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func newWebhookTestServer(t *testing.T, secret string) (*APIServer, *queue.JobQueue) {
	t.Helper()

	logger := logging.New(bytes.NewBuffer(nil), logging.INFO, "test")
	config := DefaultAPIConfig()
	config.GithubWebhookSecret = secret
	config.GitlabWebhookSecret = secret

	s := NewAPIServer(&mockManager{}, &mockOrgManager{}, config, logger)

	// Create temp job queue
	tmpDir := t.TempDir()
	jq, err := queue.NewJobQueue(filepath.Join(tmpDir, "test-jobs.db"), &mockManager{}, &mockOrgManager{}, 0, logger)
	require.NoError(t, err)
	s.SetJobQueue(jq)

	return s, jq
}

func TestGitHubWebhook_ValidSignature_TriggersJob(t *testing.T) {
	secret := "test-secret-123"
	s, jq := newWebhookTestServer(t, secret)
	defer jq.Stop()

	payload := GitHubPushPayload{
		Ref: "refs/heads/main",
	}
	payload.Repository.CloneURL = "https://github.com/org/repo.git"
	payload.Repository.DefaultBranch = "main"
	payload.Repository.FullName = "org/repo"

	body, _ := json.Marshal(payload)
	sig := computeGitHubSignature(body, secret)

	req := httptest.NewRequest("POST", "/api/v1/webhooks/github", bytes.NewReader(body))
	req.Header.Set("X-Hub-Signature-256", sig)
	req.Header.Set("X-GitHub-Event", "push")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp APIResponse
	json.NewDecoder(w.Body).Decode(&resp)
	data := resp.Data.(map[string]interface{})
	assert.Equal(t, "accepted", data["status"])
	assert.NotEmpty(t, data["job_id"])
}

func TestGitHubWebhook_InvalidSignature_Returns401(t *testing.T) {
	secret := "test-secret-123"
	s, jq := newWebhookTestServer(t, secret)
	defer jq.Stop()

	body := []byte(`{"ref": "refs/heads/main"}`)

	req := httptest.NewRequest("POST", "/api/v1/webhooks/github", bytes.NewReader(body))
	req.Header.Set("X-Hub-Signature-256", "sha256=invalid")
	req.Header.Set("X-GitHub-Event", "push")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestGitHubWebhook_MissingSignature_Returns401(t *testing.T) {
	secret := "test-secret-123"
	s, jq := newWebhookTestServer(t, secret)
	defer jq.Stop()

	body := []byte(`{"ref": "refs/heads/main"}`)

	req := httptest.NewRequest("POST", "/api/v1/webhooks/github", bytes.NewReader(body))
	req.Header.Set("X-GitHub-Event", "push")
	// No signature header
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestGitHubWebhook_NonDefaultBranch_Skipped(t *testing.T) {
	secret := "test-secret-123"
	s, jq := newWebhookTestServer(t, secret)
	defer jq.Stop()

	payload := GitHubPushPayload{
		Ref: "refs/heads/feature-x",
	}
	payload.Repository.CloneURL = "https://github.com/org/repo.git"
	payload.Repository.DefaultBranch = "main"
	payload.Repository.FullName = "org/repo"

	body, _ := json.Marshal(payload)
	sig := computeGitHubSignature(body, secret)

	req := httptest.NewRequest("POST", "/api/v1/webhooks/github", bytes.NewReader(body))
	req.Header.Set("X-Hub-Signature-256", sig)
	req.Header.Set("X-GitHub-Event", "push")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp APIResponse
	json.NewDecoder(w.Body).Decode(&resp)
	data := resp.Data.(map[string]interface{})
	assert.Equal(t, "skipped", data["status"])
}

func TestGitHubWebhook_PREvent_TriggersJob(t *testing.T) {
	secret := "test-secret-123"
	s, jq := newWebhookTestServer(t, secret)
	defer jq.Stop()

	payload := GitHubPRPayload{
		Action: "opened",
	}
	payload.PullRequest.Number = 42
	payload.PullRequest.Head.Repo.CloneURL = "https://github.com/org/repo.git"
	payload.PullRequest.Head.Repo.FullName = "org/repo"

	body, _ := json.Marshal(payload)
	sig := computeGitHubSignature(body, secret)

	req := httptest.NewRequest("POST", "/api/v1/webhooks/github", bytes.NewReader(body))
	req.Header.Set("X-Hub-Signature-256", sig)
	req.Header.Set("X-GitHub-Event", "pull_request")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestGitLabWebhook_ValidToken(t *testing.T) {
	secret := "gitlab-token-123"
	s, jq := newWebhookTestServer(t, secret)
	defer jq.Stop()

	payload := map[string]interface{}{
		"object_kind": "push",
		"ref":         "refs/heads/main",
		"project": map[string]interface{}{
			"git_http_url":       "https://gitlab.com/org/repo.git",
			"default_branch":     "main",
			"path_with_namespace": "org/repo",
		},
	}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest("POST", "/api/v1/webhooks/gitlab", bytes.NewReader(body))
	req.Header.Set("X-Gitlab-Token", secret)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestGitLabWebhook_InvalidToken_Returns401(t *testing.T) {
	secret := "gitlab-token-123"
	s, jq := newWebhookTestServer(t, secret)
	defer jq.Stop()

	body := []byte(`{"object_kind": "push"}`)
	req := httptest.NewRequest("POST", "/api/v1/webhooks/gitlab", bytes.NewReader(body))
	req.Header.Set("X-Gitlab-Token", "wrong-token")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestVerifyGitHubSignature_Unit(t *testing.T) {
	secret := "mysecret"
	body := []byte("hello world")

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	validSig := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	assert.True(t, verifyGitHubSignature(body, validSig, secret))
	assert.False(t, verifyGitHubSignature(body, "sha256=invalid", secret))
	assert.False(t, verifyGitHubSignature(body, "", secret))
}

func TestWebhookNotConfigured_Returns503(t *testing.T) {
	logger := logging.New(os.Stderr, logging.INFO, "test")
	config := DefaultAPIConfig()
	// No secrets configured
	s := NewAPIServer(&mockManager{}, &mockOrgManager{}, config, logger)

	body := []byte(`{"ref": "refs/heads/main"}`)

	req := httptest.NewRequest("POST", "/api/v1/webhooks/github", bytes.NewReader(body))
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusServiceUnavailable, w.Code)

	req = httptest.NewRequest("POST", "/api/v1/webhooks/gitlab", bytes.NewReader(body))
	w = httptest.NewRecorder()
	s.router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}
