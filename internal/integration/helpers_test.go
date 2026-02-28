//go:build integration

package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yashpalc/mcp-repo-context/internal/api"
	"github.com/yashpalc/mcp-repo-context/internal/logging"
	"github.com/yashpalc/mcp-repo-context/internal/org"
	"github.com/yashpalc/mcp-repo-context/internal/orchestrator"
	"github.com/yashpalc/mcp-repo-context/internal/queue"
	"github.com/yashpalc/mcp-repo-context/internal/repo"
	"github.com/yashpalc/mcp-repo-context/internal/storage"

	"database/sql"
	"path/filepath"

	_ "github.com/mattn/go-sqlite3"
)

// setupTestServer creates a full-stack test server with real SQLite and job queue.
func setupTestServer(t *testing.T) (*httptest.Server, *api.APIServer, func()) {
	t.Helper()

	tmpDir := t.TempDir()
	logger := logging.New(io.Discard, logging.INFO, "integration")

	// Create real SQLite storage
	dbPath := filepath.Join(tmpDir, "contexts.db")
	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_foreign_keys=ON")
	require.NoError(t, err)
	db.SetMaxOpenConns(5)

	store, err := storage.NewSQLiteStoreWithDB(db)
	require.NoError(t, err)

	// Create cloner and scanner
	cloner, err := repo.NewCloner(filepath.Join(tmpDir, "repos"))
	require.NoError(t, err)

	scanner := repo.NewScanner([]string{".git/**", "node_modules/**"}, 1024*1024)

	// Create managers
	manager := orchestrator.NewManager(store, cloner, scanner)

	orgStore, err := org.NewSQLiteStore(db)
	require.NoError(t, err)
	orgManager := org.NewManager(orgStore, manager)

	// Create API config
	config := api.DefaultAPIConfig()
	config.GithubWebhookSecret = "test-integration-secret"
	config.GitlabWebhookSecret = "test-integration-secret"
	config.RateLimitPerMinute = 1000 // High limit for tests

	apiServer := api.NewAPIServer(manager, orgManager, config, logger)

	// Create job queue
	jobsDBPath := filepath.Join(tmpDir, "jobs.db")
	jq, err := queue.NewJobQueue(jobsDBPath, manager, orgManager, 1, logger)
	require.NoError(t, err)
	apiServer.SetJobQueue(jq)

	// Start queue workers (use background context that we'll cancel in cleanup)
	// We don't start workers for integration tests since we don't want async execution
	// Unless the test specifically tests job lifecycle

	ts := httptest.NewServer(apiServer.Router())

	cleanup := func() {
		ts.Close()
		jq.Stop()
		db.Close()
	}

	return ts, apiServer, cleanup
}

// testClient is a thin wrapper around http.Client for test convenience.
type testClient struct {
	baseURL string
	client  *http.Client
}

func newTestClient(baseURL string) *testClient {
	return &testClient{
		baseURL: baseURL,
		client:  &http.Client{},
	}
}

func (c *testClient) get(path string) (*http.Response, error) {
	return c.client.Get(c.baseURL + path)
}

func (c *testClient) post(path string, body interface{}) (*http.Response, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	return c.client.Post(c.baseURL+path, "application/json", bytes.NewReader(data))
}

func (c *testClient) delete(path string) (*http.Response, error) {
	req, err := http.NewRequest("DELETE", c.baseURL+path, nil)
	if err != nil {
		return nil, err
	}
	return c.client.Do(req)
}

func (c *testClient) parseResponse(resp *http.Response) (*api.APIResponse, error) {
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var apiResp api.APIResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w (body: %s)", err, string(body))
	}
	return &apiResp, nil
}
