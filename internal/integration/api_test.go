//go:build integration

package integration

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIntegration_HealthAndStatus(t *testing.T) {
	ts, _, cleanup := setupTestServer(t)
	defer cleanup()

	c := newTestClient(ts.URL)

	// Health
	resp, err := c.get("/health")
	require.NoError(t, err)

	apiResp, err := c.parseResponse(resp)
	require.NoError(t, err)
	data := apiResp.Data.(map[string]interface{})
	assert.Equal(t, "ok", data["status"])

	// Status
	resp, err = c.get("/api/v1/status")
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	apiResp, err = c.parseResponse(resp)
	require.NoError(t, err)
	data = apiResp.Data.(map[string]interface{})
	assert.Equal(t, "ok", data["status"])
	assert.NotNil(t, data["uptime"])
}

func TestIntegration_OrgCRUD(t *testing.T) {
	ts, _, cleanup := setupTestServer(t)
	defer cleanup()

	c := newTestClient(ts.URL)

	// Create org
	resp, err := c.post("/api/v1/orgs", map[string]interface{}{
		"id":    "test-org",
		"repos": []string{},
	})
	require.NoError(t, err)
	assert.Equal(t, http.StatusCreated, resp.StatusCode)
	resp.Body.Close()

	// List orgs
	resp, err = c.get("/api/v1/orgs")
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	apiResp, err := c.parseResponse(resp)
	require.NoError(t, err)
	assert.Equal(t, 1, apiResp.Meta.Total)

	// Get org
	resp, err = c.get("/api/v1/orgs/test-org")
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()

	// Delete org
	resp, err = c.delete("/api/v1/orgs/test-org")
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()

	// Verify deleted
	resp, err = c.get("/api/v1/orgs")
	require.NoError(t, err)
	apiResp, err = c.parseResponse(resp)
	require.NoError(t, err)
	assert.Equal(t, 0, apiResp.Meta.Total)
}

func TestIntegration_ErrorHandling(t *testing.T) {
	ts, _, cleanup := setupTestServer(t)
	defer cleanup()

	c := newTestClient(ts.URL)

	// 404 for unknown repo
	resp, err := c.get("/api/v1/repos/nonexistent%2Frepo")
	require.NoError(t, err)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)

	apiResp, err := c.parseResponse(resp)
	require.NoError(t, err)
	assert.NotNil(t, apiResp.Error)
	assert.Equal(t, "not_found", apiResp.Error.Code)

	// 400 for missing URL
	resp, err = c.post("/api/v1/repos/analyze", map[string]interface{}{})
	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	resp.Body.Close()

	// 400 for invalid JSON
	resp, err = http.Post(ts.URL+"/api/v1/repos/analyze", "application/json",
		nil)
	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	resp.Body.Close()
}

func TestIntegration_Pagination(t *testing.T) {
	ts, _, cleanup := setupTestServer(t)
	defer cleanup()

	c := newTestClient(ts.URL)

	// Create 5 orgs
	for i := 0; i < 5; i++ {
		resp, err := c.post("/api/v1/orgs", map[string]interface{}{
			"id":    "org-" + string(rune('a'+i)),
			"repos": []string{},
		})
		require.NoError(t, err)
		assert.Equal(t, http.StatusCreated, resp.StatusCode)
		resp.Body.Close()
	}

	// Page 1
	resp, err := c.get("/api/v1/orgs?limit=2&offset=0")
	require.NoError(t, err)
	apiResp, err := c.parseResponse(resp)
	require.NoError(t, err)
	assert.Equal(t, 5, apiResp.Meta.Total)
	assert.Equal(t, 2, apiResp.Meta.Limit)

	data, ok := apiResp.Data.([]interface{})
	require.True(t, ok)
	assert.Len(t, data, 2)

	// Page 2
	resp, err = c.get("/api/v1/orgs?limit=2&offset=2")
	require.NoError(t, err)
	apiResp, err = c.parseResponse(resp)
	require.NoError(t, err)
	data, ok = apiResp.Data.([]interface{})
	require.True(t, ok)
	assert.Len(t, data, 2)

	// Last page
	resp, err = c.get("/api/v1/orgs?limit=2&offset=4")
	require.NoError(t, err)
	apiResp, err = c.parseResponse(resp)
	require.NoError(t, err)
	data, ok = apiResp.Data.([]interface{})
	require.True(t, ok)
	assert.Len(t, data, 1)
}

func TestIntegration_WebhookFlow(t *testing.T) {
	ts, _, cleanup := setupTestServer(t)
	defer cleanup()

	secret := "test-integration-secret"

	// Create push payload
	payload := map[string]interface{}{
		"ref": "refs/heads/main",
		"repository": map[string]interface{}{
			"clone_url":      "https://github.com/test/repo.git",
			"default_branch": "main",
			"full_name":      "test/repo",
		},
		"commits": []map[string]interface{}{
			{
				"added":    []string{"main.go"},
				"modified": []string{},
				"removed":  []string{},
			},
		},
	}

	body, _ := json.Marshal(payload)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	sig := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	req, _ := http.NewRequest("POST", ts.URL+"/api/v1/webhooks/github", nil)
	req.Body = io.NopCloser(bytes.NewReader(body))
	req.Header.Set("X-Hub-Signature-256", sig)
	req.Header.Set("X-GitHub-Event", "push")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	c := newTestClient(ts.URL)
	apiResp, err := c.parseResponse(resp)
	require.NoError(t, err)
	data := apiResp.Data.(map[string]interface{})
	assert.Equal(t, "accepted", data["status"])
	assert.NotEmpty(t, data["job_id"])

	// Verify job was created
	jobID := data["job_id"].(string)
	resp, err = c.get("/api/v1/jobs/" + jobID)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()
}

func TestIntegration_AnalyzeRepoReturns202(t *testing.T) {
	ts, _, cleanup := setupTestServer(t)
	defer cleanup()

	c := newTestClient(ts.URL)

	resp, err := c.post("/api/v1/repos/analyze", map[string]interface{}{
		"repo_url": "https://github.com/test/small-repo",
	})
	require.NoError(t, err)
	assert.Equal(t, http.StatusAccepted, resp.StatusCode)

	apiResp, err := c.parseResponse(resp)
	require.NoError(t, err)
	data := apiResp.Data.(map[string]interface{})
	assert.NotEmpty(t, data["job_id"])
}

func TestIntegration_JobList(t *testing.T) {
	ts, _, cleanup := setupTestServer(t)
	defer cleanup()

	c := newTestClient(ts.URL)

	// Create a job
	resp, err := c.post("/api/v1/repos/analyze", map[string]interface{}{
		"repo_url": "https://github.com/test/repo1",
	})
	require.NoError(t, err)
	assert.Equal(t, http.StatusAccepted, resp.StatusCode)
	resp.Body.Close()

	// List jobs
	resp, err = c.get("/api/v1/jobs")
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	apiResp, err := c.parseResponse(resp)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, apiResp.Meta.Total, 1)

	// Filter by status
	resp, err = c.get("/api/v1/jobs?status=pending")
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()
}
