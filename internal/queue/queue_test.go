package queue

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	ctxpkg "github.com/yashpalc/mcp-repo-context/internal/context"
	"github.com/yashpalc/mcp-repo-context/internal/logging"
	"github.com/yashpalc/mcp-repo-context/internal/orchestrator"
	"github.com/yashpalc/mcp-repo-context/internal/org"
	"github.com/yashpalc/mcp-repo-context/internal/prreview"
)

// --- Minimal mock manager for queue tests ---

type testManager struct {
	analyzeCount atomic.Int64
	shouldFail   bool
}

func (m *testManager) AnalyzeRepo(ctx context.Context, repoURL string, opts orchestrator.AnalyzeOptions) (*orchestrator.AnalyzeResult, error) {
	m.analyzeCount.Add(1)
	if m.shouldFail {
		return nil, fmt.Errorf("analysis failed")
	}
	return &orchestrator.AnalyzeResult{RepoID: "test/repo"}, nil
}

func (m *testManager) GetContext(ctx context.Context, repoID string) (*ctxpkg.RepoContext, error) {
	return nil, fmt.Errorf("not found")
}
func (m *testManager) GetFileContext(ctx context.Context, repoID, filePath string) (*ctxpkg.FileContext, error) {
	return nil, fmt.Errorf("not found")
}
func (m *testManager) GetFunctionContext(ctx context.Context, repoID, filePath, funcName string) (*orchestrator.FunctionContextResult, error) {
	return nil, fmt.Errorf("not found")
}
func (m *testManager) SearchFunctions(ctx context.Context, repoID, query string) ([]ctxpkg.FunctionRef, error) {
	return nil, nil
}
func (m *testManager) SearchByConcept(ctx context.Context, repoID, concept string) ([]ctxpkg.FunctionRef, error) {
	return nil, nil
}
func (m *testManager) SearchBySideEffect(ctx context.Context, repoID, effect string) ([]ctxpkg.FunctionRef, error) {
	return nil, nil
}
func (m *testManager) ListRepos(ctx context.Context) ([]ctxpkg.ContextMetadata, error) {
	return nil, nil
}
func (m *testManager) GenerateAISummary(ctx context.Context, repoID string) (*ctxpkg.AISummary, error) {
	return nil, nil
}
func (m *testManager) GenerateAIArchAnalysis(ctx context.Context, repoID string) (*ctxpkg.AIArchAnalysis, error) {
	return nil, nil
}
func (m *testManager) IsAIEnabled() bool { return false }
func (m *testManager) Ask(ctx context.Context, query string, repoIDs []string) (*orchestrator.QueryResult, error) {
	return nil, nil
}
func (m *testManager) RefreshAIContext(ctx context.Context, repoIDs []string, force bool) (*orchestrator.RefreshResult, error) {
	return nil, nil
}
func (m *testManager) ReviewPR(ctx context.Context, prURL string, opts prreview.ReviewOptions) (*prreview.ReviewResult, error) {
	return nil, nil
}
func (m *testManager) CheckPRContext(ctx context.Context, prURL string) (*prreview.ContextStatus, error) {
	return nil, nil
}
func (m *testManager) AnalyzeLocal(ctx context.Context, dirPath string, opts orchestrator.AnalyzeLocalOptions) (*orchestrator.AnalyzeLocalResult, error) {
	return nil, nil
}
func (m *testManager) GetOrAnalyzeLocal(ctx context.Context, dirPath string) (*ctxpkg.RepoContext, error) {
	return nil, nil
}
func (m *testManager) SmartQuery(ctx context.Context, query string, projectID string) (*orchestrator.SmartQueryResult, error) {
	return nil, nil
}
func (m *testManager) RefreshFile(ctx context.Context, projectID, filePath string, opts orchestrator.RefreshFileOptions) (*orchestrator.RefreshFileResult, error) {
	return nil, nil
}
func (m *testManager) RefreshChangedFiles(ctx context.Context, projectID string) ([]orchestrator.RefreshFileResult, error) {
	return nil, nil
}
func (m *testManager) CheckFileStale(ctx context.Context, projectID, filePath string) (bool, error) {
	return false, nil
}
func (m *testManager) GetPRContext(ctx context.Context, repoID string, changedFiles []orchestrator.ChangedFile) (*orchestrator.PRContextResult, error) {
	return nil, nil
}
func (m *testManager) DeleteRepoContext(ctx context.Context, repoID string) error { return nil }
func (m *testManager) GetDependencyGraph(ctx context.Context, repoIDs []string, includeExternal bool) (*ctxpkg.DependencyGraph, error) {
	return nil, nil
}

type testOrgManager struct{}

func (m *testOrgManager) Register(ctx context.Context, orgID string, repoIDs []string, config *org.OrgConfig) (*org.Org, error) {
	return nil, nil
}
func (m *testOrgManager) List(ctx context.Context) ([]org.OrgWithCount, error) { return nil, nil }
func (m *testOrgManager) Get(ctx context.Context, orgID string) (*org.Org, error) {
	return nil, nil
}
func (m *testOrgManager) AddRepos(ctx context.Context, orgID string, repoIDs []string) error {
	return nil
}
func (m *testOrgManager) RemoveRepos(ctx context.Context, orgID string, repoIDs []string) error {
	return nil
}
func (m *testOrgManager) Delete(ctx context.Context, orgID string) error { return nil }
func (m *testOrgManager) GetEffectiveConfig(ctx context.Context, orgID, repoID string) (*org.OrgConfig, error) {
	return nil, nil
}
func (m *testOrgManager) SetRepoConfigOverride(ctx context.Context, orgID, repoID string, config *org.OrgConfig) error {
	return nil
}
func (m *testOrgManager) AnalyzeOrg(ctx context.Context, orgID string, force bool, concurrency int) (*org.AnalysisResult, error) {
	return nil, nil
}
func (m *testOrgManager) IndexOrg(ctx context.Context, orgID string, force bool, concurrency int) (*org.IndexOrgResult, error) {
	return nil, nil
}

// --- Test helpers ---

func newTestQueue(t *testing.T) (*JobQueue, context.CancelFunc) {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test-jobs.db")
	logger := logging.New(io.Discard, logging.ERROR, "test")

	mgr := &testManager{}
	jq, err := NewJobQueue(dbPath, mgr, &testOrgManager{}, 1, logger)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	jq.Start(ctx)
	t.Cleanup(func() {
		cancel()
		jq.Stop()
	})

	return jq, cancel
}

func newTestQueueWithManager(t *testing.T, mgr orchestrator.Manager) (*JobQueue, context.CancelFunc) {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test-jobs.db")
	logger := logging.New(io.Discard, logging.ERROR, "test")

	jq, err := NewJobQueue(dbPath, mgr, &testOrgManager{}, 1, logger)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	jq.Start(ctx)
	t.Cleanup(func() {
		cancel()
		jq.Stop()
	})

	return jq, cancel
}

func waitForJobStatus(t *testing.T, jq *JobQueue, jobID string, status string, timeout time.Duration) *Job {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		job, err := jq.GetJob(context.Background(), jobID)
		require.NoError(t, err)
		if job.Status == status {
			return job
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("job %s did not reach status %s within %v", jobID, status, timeout)
	return nil
}

// --- Tests ---

func TestEnqueue_CreatesPendingJob(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	logger := logging.New(io.Discard, logging.ERROR, "test")

	jq, err := NewJobQueue(dbPath, &testManager{}, &testOrgManager{}, 0, logger)
	require.NoError(t, err)
	defer jq.db.Close()

	jobID, err := jq.Enqueue(context.Background(), "analyze_repo", "", "repo1", `{"repo_url":"https://github.com/org/repo"}`)
	require.NoError(t, err)
	assert.NotEmpty(t, jobID)

	job, err := jq.GetJob(context.Background(), jobID)
	require.NoError(t, err)
	assert.Equal(t, StatusPending, job.Status)
}

func TestWorker_CompletesJob(t *testing.T) {
	jq, _ := newTestQueue(t)

	jobID, err := jq.Enqueue(context.Background(), "analyze_repo", "", "", `{"repo_url":"https://github.com/org/repo"}`)
	require.NoError(t, err)

	job := waitForJobStatus(t, jq, jobID, StatusCompleted, 10*time.Second)
	assert.NotEmpty(t, job.Result)
}

func TestWorker_FailedJobHasError(t *testing.T) {
	mgr := &testManager{shouldFail: true}
	jq, _ := newTestQueueWithManager(t, mgr)

	// Set max_retries=1 so it fails fast
	jobID, err := jq.Enqueue(context.Background(), "analyze_repo", "", "", `{"repo_url":"https://github.com/org/repo"}`)
	require.NoError(t, err)

	// Manually set max_retries=1 to speed up test
	jq.db.ExecContext(context.Background(), "UPDATE jobs SET max_retries = 1 WHERE id = ?", jobID)

	job := waitForJobStatus(t, jq, jobID, StatusFailed, 10*time.Second)
	assert.NotEmpty(t, job.Error)
}

func TestConcurrentWorkers_ClaimDifferentJobs(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	logger := logging.New(io.Discard, logging.ERROR, "test")

	var processedJobs sync.Map
	var processCount atomic.Int64

	mgr := &testManager{}
	jq, err := NewJobQueue(dbPath, mgr, &testOrgManager{}, 5, logger)
	require.NoError(t, err)

	// Enqueue 10 jobs
	var jobIDs []string
	for i := 0; i < 10; i++ {
		id, err := jq.Enqueue(context.Background(), "analyze_repo", "", fmt.Sprintf("repo-%d", i),
			fmt.Sprintf(`{"repo_url":"https://github.com/org/repo-%d"}`, i))
		require.NoError(t, err)
		jobIDs = append(jobIDs, id)
	}

	ctx, cancel := context.WithCancel(context.Background())
	jq.Start(ctx)

	// Wait for all jobs to complete (SQLite concurrent access can be slow)
	for _, id := range jobIDs {
		waitForJobStatus(t, jq, id, StatusCompleted, 60*time.Second)
		if _, loaded := processedJobs.LoadOrStore(id, true); loaded {
			t.Fatalf("job %s processed more than once", id)
		}
		processCount.Add(1)
	}

	cancel()
	jq.Stop()

	assert.Equal(t, int64(10), processCount.Load())
}

func TestDedup_PreventsDuplicate(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	logger := logging.New(io.Discard, logging.ERROR, "test")

	jq, err := NewJobQueue(dbPath, &testManager{}, &testOrgManager{}, 0, logger)
	require.NoError(t, err)
	defer jq.db.Close()

	id1, err := jq.Enqueue(context.Background(), "analyze_repo", "", "repo1", `{"repo_url":"https://github.com/org/repo"}`)
	require.NoError(t, err)

	id2, err := jq.Enqueue(context.Background(), "analyze_repo", "", "repo1", `{"repo_url":"https://github.com/org/repo"}`)
	require.NoError(t, err)

	assert.Equal(t, id1, id2, "second enqueue should return existing job ID")
}

func TestDedup_AllowsAfter5Min(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	logger := logging.New(io.Discard, logging.ERROR, "test")

	jq, err := NewJobQueue(dbPath, &testManager{}, &testOrgManager{}, 0, logger)
	require.NoError(t, err)
	defer jq.db.Close()

	id1, err := jq.Enqueue(context.Background(), "analyze_repo", "", "repo1", `{"repo_url":"https://github.com/org/repo"}`)
	require.NoError(t, err)

	// Move job creation back 10 minutes
	jq.db.ExecContext(context.Background(), "UPDATE jobs SET created_at = datetime('now', '-10 minutes') WHERE id = ?", id1)

	id2, err := jq.Enqueue(context.Background(), "analyze_repo", "", "repo1", `{"repo_url":"https://github.com/org/repo"}`)
	require.NoError(t, err)

	assert.NotEqual(t, id1, id2, "should create new job after 5 min dedup window")
}

func TestListJobs_Paginated(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	logger := logging.New(io.Discard, logging.ERROR, "test")

	jq, err := NewJobQueue(dbPath, &testManager{}, &testOrgManager{}, 0, logger)
	require.NoError(t, err)
	defer jq.db.Close()

	for i := 0; i < 5; i++ {
		_, err := jq.Enqueue(context.Background(), "analyze_repo", "", fmt.Sprintf("repo%d", i),
			fmt.Sprintf(`{"repo_url":"repo%d"}`, i))
		require.NoError(t, err)
	}

	jobs, total, err := jq.ListJobs(context.Background(), "", "", 2, 0)
	require.NoError(t, err)
	assert.Len(t, jobs, 2)
	assert.Equal(t, 5, total)
}

func TestListJobs_FilterByStatus(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	logger := logging.New(io.Discard, logging.ERROR, "test")

	jq, err := NewJobQueue(dbPath, &testManager{}, &testOrgManager{}, 0, logger)
	require.NoError(t, err)
	defer jq.db.Close()

	// Create 3 jobs
	for i := 0; i < 3; i++ {
		_, err := jq.Enqueue(context.Background(), "analyze_repo", "", fmt.Sprintf("repo%d", i), `{}`)
		require.NoError(t, err)
	}

	// Mark one as completed
	jq.db.ExecContext(context.Background(), "UPDATE jobs SET status = 'completed' WHERE repo_id = 'repo0'")

	jobs, _, err := jq.ListJobs(context.Background(), "", "completed", 10, 0)
	require.NoError(t, err)
	assert.Len(t, jobs, 1)
}

func TestStaleJobRecovery(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	logger := logging.New(io.Discard, logging.ERROR, "test")

	jq, err := NewJobQueue(dbPath, &testManager{}, &testOrgManager{}, 0, logger)
	require.NoError(t, err)
	defer jq.db.Close()

	id, err := jq.Enqueue(context.Background(), "analyze_repo", "", "repo1", `{}`)
	require.NoError(t, err)

	// Simulate stale: set running with old started_at
	jq.db.ExecContext(context.Background(),
		"UPDATE jobs SET status = 'running', started_at = datetime('now', '-31 minutes') WHERE id = ?", id)

	// Run sweeper
	jq.recoverStaleJobs(context.Background())

	job, err := jq.GetJob(context.Background(), id)
	require.NoError(t, err)
	assert.Equal(t, StatusPending, job.Status, "stale job should be back to pending")
	assert.Equal(t, 1, job.Retries)
}

func TestCleanupOldJobs(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	logger := logging.New(io.Discard, logging.ERROR, "test")

	jq, err := NewJobQueue(dbPath, &testManager{}, &testOrgManager{}, 0, logger)
	require.NoError(t, err)
	defer jq.db.Close()

	id, err := jq.Enqueue(context.Background(), "analyze_repo", "", "repo1", `{}`)
	require.NoError(t, err)

	// Set as completed 8 days ago
	jq.db.ExecContext(context.Background(),
		"UPDATE jobs SET status = 'completed', completed_at = datetime('now', '-8 days') WHERE id = ?", id)

	jq.cleanupOldJobs(context.Background())

	_, err = jq.GetJob(context.Background(), id)
	assert.Error(t, err, "old job should be deleted")
}
