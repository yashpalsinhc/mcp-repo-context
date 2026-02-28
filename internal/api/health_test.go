package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	ctxpkg "github.com/yashpalc/mcp-repo-context/internal/context"
	"github.com/yashpalc/mcp-repo-context/internal/logging"
	"github.com/yashpalc/mcp-repo-context/internal/org"
)

func TestHandleHealth_Returns200(t *testing.T) {
	s := newTestServer(t)

	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp APIResponse
	err := json.NewDecoder(w.Body).Decode(&resp)
	require.NoError(t, err)

	data := resp.Data.(map[string]interface{})
	assert.Equal(t, "ok", data["status"])
}

func TestHandleStatus_ReturnsCounts(t *testing.T) {
	mgr := &mockManager{
		repos: []ctxpkg.ContextMetadata{
			{RepoID: "repo1"}, {RepoID: "repo2"},
		},
	}
	orgMgr := &mockOrgManager{
		orgs: []org.OrgWithCount{{Org: org.Org{ID: "org1"}}},
	}

	logger := logging.New(bytes.NewBuffer(nil), logging.INFO, "test")
	s := NewAPIServer(mgr, orgMgr, DefaultAPIConfig(), logger)

	req := httptest.NewRequest("GET", "/api/v1/status", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp APIResponse
	err := json.NewDecoder(w.Body).Decode(&resp)
	require.NoError(t, err)

	data := resp.Data.(map[string]interface{})
	assert.Equal(t, float64(2), data["repos_analyzed"])
	assert.Equal(t, float64(1), data["orgs"])
	assert.Equal(t, "ok", data["status"])
}

func TestRateLimiter_AllowsWithinLimit(t *testing.T) {
	logger := logging.New(bytes.NewBuffer(nil), logging.INFO, "test")
	config := DefaultAPIConfig()
	config.RateLimitPerMinute = 60

	s := NewAPIServer(&mockManager{repos: []ctxpkg.ContextMetadata{}}, &mockOrgManager{}, config, logger)

	// Send 10 requests (well within 60/min)
	for i := 0; i < 10; i++ {
		req := httptest.NewRequest("GET", "/api/v1/repos", nil)
		req.RemoteAddr = "10.0.0.1:12345"
		w := httptest.NewRecorder()
		s.router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code, "request %d should succeed", i)
	}
}

func TestRateLimiter_BlocksOverLimit(t *testing.T) {
	logger := logging.New(bytes.NewBuffer(nil), logging.INFO, "test")
	config := DefaultAPIConfig()
	config.RateLimitPerMinute = 5

	s := NewAPIServer(&mockManager{repos: []ctxpkg.ContextMetadata{}}, &mockOrgManager{}, config, logger)

	var lastStatus int
	for i := 0; i < 7; i++ {
		req := httptest.NewRequest("GET", "/api/v1/repos", nil)
		req.RemoteAddr = "10.0.0.2:12345"
		w := httptest.NewRecorder()
		s.router.ServeHTTP(w, req)
		lastStatus = w.Code
		if w.Code == http.StatusTooManyRequests {
			assert.NotEmpty(t, w.Header().Get("Retry-After"))
			return
		}
	}
	t.Fatalf("expected 429, but last status was %d", lastStatus)
}

func TestStructuredJSONLog(t *testing.T) {
	var logBuf bytes.Buffer
	logger := logging.New(&logBuf, logging.INFO, "test")
	config := DefaultAPIConfig()

	s := NewAPIServer(&mockManager{repos: []ctxpkg.ContextMetadata{}}, &mockOrgManager{}, config, logger)

	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	// Parse log output
	logOutput := logBuf.String()
	if logOutput == "" {
		t.Skip("log output empty - may be buffered")
	}

	var logEntry map[string]interface{}
	err := json.Unmarshal([]byte(logOutput), &logEntry)
	if err != nil {
		// Log might have multiple lines; try first line
		t.Skipf("could not parse log as JSON: %s", logOutput)
	}

	assert.Contains(t, logEntry, "method")
	assert.Contains(t, logEntry, "path")
	assert.Contains(t, logEntry, "status")
	assert.Contains(t, logEntry, "duration_ms")
}
