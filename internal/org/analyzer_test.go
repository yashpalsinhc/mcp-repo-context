package org

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
	ctxpkg "github.com/yashpalc/mcp-repo-context/internal/context"
	"github.com/yashpalc/mcp-repo-context/internal/orchestrator"
	"github.com/yashpalc/mcp-repo-context/internal/prreview"
	"github.com/yashpalc/mcp-repo-context/internal/storage"
)

// mockOrch implements orchestrator.Manager for testing.
type mockOrch struct {
	mu            sync.Mutex
	failRepos     map[string]int    // repo → number of consecutive failures before success (0 = always succeed)
	callCounts    map[string]int    // repo → times called
	nonRetryable  map[string]bool   // repo → error is non-retryable
	latency       time.Duration
	maxConcurrent int64
	curConcurrent int64
	forceFlag     bool // last-seen force value
}

func newMockOrch() *mockOrch {
	return &mockOrch{
		failRepos:  make(map[string]int),
		callCounts: make(map[string]int),
		nonRetryable: make(map[string]bool),
	}
}

func (m *mockOrch) analyzeCall(repoID string, force bool) error {
	cur := atomic.AddInt64(&m.curConcurrent, 1)
	defer atomic.AddInt64(&m.curConcurrent, -1)

	// Track max concurrent
	for {
		old := atomic.LoadInt64(&m.maxConcurrent)
		if cur <= old || atomic.CompareAndSwapInt64(&m.maxConcurrent, old, cur) {
			break
		}
	}

	if m.latency > 0 {
		time.Sleep(m.latency)
	}

	m.mu.Lock()
	m.forceFlag = force
	m.callCounts[repoID]++
	calls := m.callCounts[repoID]
	failCount := m.failRepos[repoID]
	isNonRetryable := m.nonRetryable[repoID]
	m.mu.Unlock()

	if failCount > 0 && calls <= failCount {
		if isNonRetryable {
			return fmt.Errorf("repo not found: %s", repoID)
		}
		return fmt.Errorf("transient error for %s", repoID)
	}
	return nil
}

func (m *mockOrch) AnalyzeRepo(_ context.Context, repoURL string, opts orchestrator.AnalyzeOptions) (*orchestrator.AnalyzeResult, error) {
	err := m.analyzeCall(repoURL, opts.Force)
	if err != nil {
		return nil, err
	}
	return &orchestrator.AnalyzeResult{RepoID: repoURL}, nil
}

func (m *mockOrch) AnalyzeLocal(_ context.Context, dirPath string, opts orchestrator.AnalyzeLocalOptions) (*orchestrator.AnalyzeLocalResult, error) {
	repoID := "local:" + dirPath
	err := m.analyzeCall(repoID, opts.Force)
	if err != nil {
		return nil, err
	}
	return &orchestrator.AnalyzeLocalResult{ProjectID: repoID}, nil
}

// Stub implementations for the rest of orchestrator.Manager
func (m *mockOrch) GetContext(context.Context, string) (*ctxpkg.RepoContext, error)             { return nil, nil }
func (m *mockOrch) GetFileContext(context.Context, string, string) (*ctxpkg.FileContext, error)  { return nil, nil }
func (m *mockOrch) GetFunctionContext(context.Context, string, string, string) (*orchestrator.FunctionContextResult, error) { return nil, nil }
func (m *mockOrch) SearchFunctions(context.Context, string, string) ([]ctxpkg.FunctionRef, error) { return nil, nil }
func (m *mockOrch) SearchByConcept(context.Context, string, string) ([]ctxpkg.FunctionRef, error) { return nil, nil }
func (m *mockOrch) SearchBySideEffect(context.Context, string, string) ([]ctxpkg.FunctionRef, error) { return nil, nil }
func (m *mockOrch) ListRepos(context.Context) ([]ctxpkg.ContextMetadata, error)                 { return nil, nil }
func (m *mockOrch) GenerateAISummary(context.Context, string) (*ctxpkg.AISummary, error)        { return nil, nil }
func (m *mockOrch) GenerateAIArchAnalysis(context.Context, string) (*ctxpkg.AIArchAnalysis, error) { return nil, nil }
func (m *mockOrch) IsAIEnabled() bool                                                           { return false }
func (m *mockOrch) Ask(context.Context, string, []string) (*orchestrator.QueryResult, error)    { return nil, nil }
func (m *mockOrch) RefreshAIContext(context.Context, []string, bool) (*orchestrator.RefreshResult, error) { return nil, nil }
func (m *mockOrch) ReviewPR(context.Context, string, prreview.ReviewOptions) (*prreview.ReviewResult, error) { return nil, nil }
func (m *mockOrch) CheckPRContext(context.Context, string) (*prreview.ContextStatus, error)     { return nil, nil }
func (m *mockOrch) GetOrAnalyzeLocal(context.Context, string) (*ctxpkg.RepoContext, error)      { return nil, nil }
func (m *mockOrch) SmartQuery(context.Context, string, string) (*orchestrator.SmartQueryResult, error) { return nil, nil }
func (m *mockOrch) RefreshFile(context.Context, string, string, orchestrator.RefreshFileOptions) (*orchestrator.RefreshFileResult, error) { return nil, nil }
func (m *mockOrch) RefreshChangedFiles(context.Context, string) ([]orchestrator.RefreshFileResult, error) { return nil, nil }
func (m *mockOrch) CheckFileStale(context.Context, string, string) (bool, error)                { return false, nil }
func (m *mockOrch) GetPRContext(context.Context, string, []orchestrator.ChangedFile) (*orchestrator.PRContextResult, error) { return nil, nil }
func (m *mockOrch) DeleteRepoContext(context.Context, string) error                             { return nil }

// newAnalyzerTestStore creates an in-memory store + manager for testing the analyzer.
func newAnalyzerTestSetup(t *testing.T, mo *mockOrch) (Manager, *SQLiteStore) {
	t.Helper()
	dsn := nextTestDBName()
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		t.Fatalf("Failed to open DB: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })

	_, err = storage.NewSQLiteStoreWithDB(db)
	if err != nil {
		t.Fatalf("Failed to run migrations: %v", err)
	}

	store, err := NewSQLiteStore(db)
	if err != nil {
		t.Fatalf("Failed to create org store: %v", err)
	}

	mgr := NewManager(store, mo)
	return mgr, store
}

// --- Analyzer tests ---

func TestAnalyzeOrg_AllSucceed(t *testing.T) {
	mo := newMockOrch()
	mgr, _ := newAnalyzerTestSetup(t, mo)
	ctx := context.Background()

	mgr.Register(ctx, "org-1", []string{"repo-a", "repo-b", "repo-c"}, nil)

	result, err := mgr.AnalyzeOrg(ctx, "org-1", false, 3)
	if err != nil {
		t.Fatalf("AnalyzeOrg failed: %v", err)
	}
	if result.Total != 3 {
		t.Errorf("Total = %d, want 3", result.Total)
	}
	if result.Succeeded != 3 {
		t.Errorf("Succeeded = %d, want 3", result.Succeeded)
	}
	if result.Failed != 0 {
		t.Errorf("Failed = %d, want 0", result.Failed)
	}
	if len(result.Errors) != 0 {
		t.Errorf("Errors = %v, want empty", result.Errors)
	}
}

func TestAnalyzeOrg_RetrySucceeds(t *testing.T) {
	mo := newMockOrch()
	mo.failRepos["repo-b"] = 1 // fail first call, succeed on retry
	mgr, _ := newAnalyzerTestSetup(t, mo)
	ctx := context.Background()

	mgr.Register(ctx, "org-1", []string{"repo-a", "repo-b"}, nil)

	result, err := mgr.AnalyzeOrg(ctx, "org-1", false, 3)
	if err != nil {
		t.Fatalf("AnalyzeOrg failed: %v", err)
	}
	if result.Succeeded != 2 {
		t.Errorf("Succeeded = %d, want 2 (retry should succeed)", result.Succeeded)
	}
	if result.Failed != 0 {
		t.Errorf("Failed = %d, want 0", result.Failed)
	}
}

func TestAnalyzeOrg_RetryFails(t *testing.T) {
	mo := newMockOrch()
	mo.failRepos["repo-b"] = 99 // always fail
	mgr, _ := newAnalyzerTestSetup(t, mo)
	ctx := context.Background()

	mgr.Register(ctx, "org-1", []string{"repo-a", "repo-b"}, nil)

	result, err := mgr.AnalyzeOrg(ctx, "org-1", false, 3)
	if err != nil {
		t.Fatalf("AnalyzeOrg failed: %v", err)
	}
	if result.Succeeded != 1 {
		t.Errorf("Succeeded = %d, want 1", result.Succeeded)
	}
	if result.Failed != 1 {
		t.Errorf("Failed = %d, want 1", result.Failed)
	}
	if len(result.Errors) != 1 {
		t.Fatalf("Errors len = %d, want 1", len(result.Errors))
	}
	if result.Errors[0].RepoID != "repo-b" {
		t.Errorf("Errors[0].RepoID = %q, want repo-b", result.Errors[0].RepoID)
	}
}

func TestAnalyzeOrg_ConcurrencyLimit(t *testing.T) {
	mo := newMockOrch()
	mo.latency = 50 * time.Millisecond
	mgr, _ := newAnalyzerTestSetup(t, mo)
	ctx := context.Background()

	repos := make([]string, 10)
	for i := range repos {
		repos[i] = fmt.Sprintf("repo-%d", i)
	}
	mgr.Register(ctx, "org-1", repos, nil)

	result, err := mgr.AnalyzeOrg(ctx, "org-1", false, 2)
	if err != nil {
		t.Fatalf("AnalyzeOrg failed: %v", err)
	}
	if result.Succeeded != 10 {
		t.Errorf("Succeeded = %d, want 10", result.Succeeded)
	}
	if atomic.LoadInt64(&mo.maxConcurrent) > 2 {
		t.Errorf("maxConcurrent = %d, want <= 2", mo.maxConcurrent)
	}
}

func TestAnalyzeOrg_ContextCancellation(t *testing.T) {
	mo := newMockOrch()
	mo.latency = 200 * time.Millisecond
	mgr, _ := newAnalyzerTestSetup(t, mo)

	repos := make([]string, 20)
	for i := range repos {
		repos[i] = fmt.Sprintf("repo-%d", i)
	}
	ctx := context.Background()
	mgr.Register(ctx, "org-1", repos, nil)

	ctx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()

	result, err := mgr.AnalyzeOrg(ctx, "org-1", false, 1) // concurrency=1, each takes 200ms
	if err != nil {
		t.Fatalf("AnalyzeOrg failed: %v", err)
	}
	// With 1 concurrency and 200ms latency, 100ms timeout should complete 0-1 repos
	if result.Succeeded >= len(repos) {
		t.Errorf("Succeeded = %d, want < %d (should have been cancelled)", result.Succeeded, len(repos))
	}
}

func TestAnalyzeOrg_ForceFlag(t *testing.T) {
	mo := newMockOrch()
	mgr, _ := newAnalyzerTestSetup(t, mo)
	ctx := context.Background()

	mgr.Register(ctx, "org-1", []string{"repo-a"}, nil)

	_, err := mgr.AnalyzeOrg(ctx, "org-1", true, 3)
	if err != nil {
		t.Fatalf("AnalyzeOrg failed: %v", err)
	}

	mo.mu.Lock()
	gotForce := mo.forceFlag
	mo.mu.Unlock()
	if !gotForce {
		t.Error("force flag was not passed to orchestrator")
	}
}

func TestAnalyzeOrg_EmptyOrg(t *testing.T) {
	mo := newMockOrch()
	mgr, _ := newAnalyzerTestSetup(t, mo)
	ctx := context.Background()

	mgr.Register(ctx, "org-1", []string{}, nil)

	result, err := mgr.AnalyzeOrg(ctx, "org-1", false, 3)
	if err != nil {
		t.Fatalf("AnalyzeOrg failed: %v", err)
	}
	if result.Total != 0 {
		t.Errorf("Total = %d, want 0", result.Total)
	}
	if result.Succeeded != 0 {
		t.Errorf("Succeeded = %d, want 0", result.Succeeded)
	}
	if result.Duration <= 0 {
		t.Error("Duration should be > 0")
	}
}

func TestAnalyzeOrg_NonExistentOrg(t *testing.T) {
	mo := newMockOrch()
	mgr, _ := newAnalyzerTestSetup(t, mo)
	ctx := context.Background()

	_, err := mgr.AnalyzeOrg(ctx, "ghost-org", false, 3)
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("AnalyzeOrg error = %v, want ErrNotFound", err)
	}
}

func TestAnalyzeOrg_RoutesLocalPrefix(t *testing.T) {
	mo := newMockOrch()
	mgr, _ := newAnalyzerTestSetup(t, mo)
	ctx := context.Background()

	mgr.Register(ctx, "org-1", []string{"local:/tmp/myrepo", "github.com/foo/bar"}, nil)

	result, err := mgr.AnalyzeOrg(ctx, "org-1", false, 3)
	if err != nil {
		t.Fatalf("AnalyzeOrg failed: %v", err)
	}
	if result.Succeeded != 2 {
		t.Errorf("Succeeded = %d, want 2", result.Succeeded)
	}

	mo.mu.Lock()
	// local: prefix should route to AnalyzeLocal (which uses "local:" + dirPath as key)
	localCalls := mo.callCounts["local:/tmp/myrepo"]
	remoteCalls := mo.callCounts["github.com/foo/bar"]
	mo.mu.Unlock()

	if localCalls != 1 {
		t.Errorf("local repo callCount = %d, want 1", localCalls)
	}
	if remoteCalls != 1 {
		t.Errorf("remote repo callCount = %d, want 1", remoteCalls)
	}
}

func TestAnalyzeOrg_Duration(t *testing.T) {
	mo := newMockOrch()
	mgr, _ := newAnalyzerTestSetup(t, mo)
	ctx := context.Background()

	mgr.Register(ctx, "org-1", []string{"repo-a"}, nil)

	result, err := mgr.AnalyzeOrg(ctx, "org-1", false, 3)
	if err != nil {
		t.Fatalf("AnalyzeOrg failed: %v", err)
	}
	if result.Duration <= 0 {
		t.Error("Duration should be > 0")
	}
}

func TestAnalyzeOrg_ClampsConcurrency(t *testing.T) {
	mo := newMockOrch()
	mgr, _ := newAnalyzerTestSetup(t, mo)
	ctx := context.Background()

	mgr.Register(ctx, "org-1", []string{"repo-a"}, nil)

	// 0 should become 3, 15 should become 10 — both should just work
	result, err := mgr.AnalyzeOrg(ctx, "org-1", false, 0)
	if err != nil {
		t.Fatalf("AnalyzeOrg (concurrency=0) failed: %v", err)
	}
	if result.Succeeded != 1 {
		t.Errorf("Succeeded = %d, want 1", result.Succeeded)
	}

	result, err = mgr.AnalyzeOrg(ctx, "org-1", false, 15)
	if err != nil {
		t.Fatalf("AnalyzeOrg (concurrency=15) failed: %v", err)
	}
	if result.Succeeded != 1 {
		t.Errorf("Succeeded = %d, want 1", result.Succeeded)
	}
}

func TestAnalyzeOrg_SkipsRetryForNonRetryable(t *testing.T) {
	mo := newMockOrch()
	mo.failRepos["repo-b"] = 99       // always fail
	mo.nonRetryable["repo-b"] = true   // "not found" error
	mgr, _ := newAnalyzerTestSetup(t, mo)
	ctx := context.Background()

	mgr.Register(ctx, "org-1", []string{"repo-b"}, nil)

	result, err := mgr.AnalyzeOrg(ctx, "org-1", false, 3)
	if err != nil {
		t.Fatalf("AnalyzeOrg failed: %v", err)
	}
	if result.Failed != 1 {
		t.Errorf("Failed = %d, want 1", result.Failed)
	}

	mo.mu.Lock()
	calls := mo.callCounts["repo-b"]
	mo.mu.Unlock()

	// Non-retryable: should only be called once (no retry)
	if calls != 1 {
		t.Errorf("callCount for repo-b = %d, want 1 (no retry for non-retryable)", calls)
	}

	// Error message should contain "not found"
	if len(result.Errors) != 1 || !strings.Contains(result.Errors[0].Error, "not found") {
		t.Errorf("Errors = %v, want 'not found' error", result.Errors)
	}
}
