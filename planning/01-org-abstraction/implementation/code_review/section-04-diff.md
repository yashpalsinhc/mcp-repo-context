diff --git a/internal/orchestrator/interface.go b/internal/orchestrator/interface.go
index 7812957..5fd4cda 100644
--- a/internal/orchestrator/interface.go
+++ b/internal/orchestrator/interface.go
@@ -76,6 +76,9 @@ type Manager interface {
 
 	// GetPRContext extracts rich context for PR changes (no AI required).
 	GetPRContext(ctx context.Context, repoID string, changedFiles []ChangedFile) (*PRContextResult, error)
+
+	// DeleteRepoContext removes all stored context for a repository.
+	DeleteRepoContext(ctx context.Context, repoID string) error
 }
 
 // RefreshResult contains results of refreshing AI context.
diff --git a/internal/orchestrator/manager.go b/internal/orchestrator/manager.go
index bcbf41b..6bcb492 100644
--- a/internal/orchestrator/manager.go
+++ b/internal/orchestrator/manager.go
@@ -900,3 +900,8 @@ func (m *manager) ReviewPR(ctx context.Context, prURL string, opts prreview.Revi
 
 	return result, nil
 }
+
+// DeleteRepoContext removes all stored context for a repository.
+func (m *manager) DeleteRepoContext(ctx context.Context, repoID string) error {
+	return m.store.DeleteContext(ctx, repoID)
+}
diff --git a/internal/org/analyzer.go b/internal/org/analyzer.go
new file mode 100644
index 0000000..113eb81
--- /dev/null
+++ b/internal/org/analyzer.go
@@ -0,0 +1,136 @@
+package org
+
+import (
+	"context"
+	"strings"
+	"sync"
+	"time"
+
+	"github.com/yashpalc/mcp-repo-context/internal/orchestrator"
+)
+
+// Analyzer orchestrates concurrent analysis of all repos in an org.
+type Analyzer struct {
+	orgManager Manager
+	orch       orchestrator.Manager
+}
+
+// NewAnalyzer creates an Analyzer.
+func NewAnalyzer(orgManager Manager, orch orchestrator.Manager) *Analyzer {
+	return &Analyzer{orgManager: orgManager, orch: orch}
+}
+
+// AnalyzeOrg analyzes all repos in an org with bounded concurrency and single-retry.
+func (a *Analyzer) AnalyzeOrg(ctx context.Context, orgID string, force bool, concurrency int) (*AnalysisResult, error) {
+	// Clamp concurrency
+	if concurrency <= 0 {
+		concurrency = 3
+	}
+	if concurrency > 10 {
+		concurrency = 10
+	}
+
+	org, err := a.orgManager.Get(ctx, orgID)
+	if err != nil {
+		return nil, err
+	}
+
+	start := time.Now()
+	result := &AnalysisResult{
+		OrgID: orgID,
+		Total: len(org.Repos),
+	}
+
+	if len(org.Repos) == 0 {
+		result.Duration = time.Since(start)
+		return result, nil
+	}
+
+	sem := make(chan struct{}, concurrency)
+	var mu sync.Mutex
+	var wg sync.WaitGroup
+
+	for _, repoID := range org.Repos {
+		// Check cancellation before launching
+		select {
+		case <-ctx.Done():
+			mu.Lock()
+			result.Duration = time.Since(start)
+			mu.Unlock()
+			return result, nil
+		default:
+		}
+
+		wg.Add(1)
+		go func(repoID string) {
+			defer wg.Done()
+
+			// Acquire semaphore or bail on cancellation
+			select {
+			case sem <- struct{}{}:
+			case <-ctx.Done():
+				return
+			}
+			defer func() { <-sem }() // release
+
+			err := a.analyzeRepo(ctx, orgID, repoID, force)
+
+			mu.Lock()
+			defer mu.Unlock()
+			if err != nil {
+				result.Failed++
+				result.Errors = append(result.Errors, RepoError{
+					RepoID: repoID,
+					Error:  err.Error(),
+				})
+			} else {
+				result.Succeeded++
+			}
+		}(repoID)
+	}
+
+	wg.Wait()
+	result.Duration = time.Since(start)
+	return result, nil
+}
+
+// analyzeRepo analyzes a single repo with single-retry for transient errors.
+func (a *Analyzer) analyzeRepo(ctx context.Context, orgID, repoID string, force bool) error {
+	err := a.callOrchestrator(ctx, repoID, force)
+	if err == nil {
+		return nil
+	}
+
+	// Skip retry for non-retryable errors
+	if isNonRetryable(err) {
+		return err
+	}
+
+	// Wait and retry once
+	select {
+	case <-ctx.Done():
+		return err
+	case <-time.After(1 * time.Second):
+	}
+
+	return a.callOrchestrator(ctx, repoID, force)
+}
+
+// callOrchestrator routes to AnalyzeLocal or AnalyzeRepo based on prefix.
+func (a *Analyzer) callOrchestrator(ctx context.Context, repoID string, force bool) error {
+	if strings.HasPrefix(repoID, "local:") {
+		path := strings.TrimPrefix(repoID, "local:")
+		_, err := a.orch.AnalyzeLocal(ctx, path, orchestrator.AnalyzeLocalOptions{Force: force})
+		return err
+	}
+	_, err := a.orch.AnalyzeRepo(ctx, repoID, orchestrator.AnalyzeOptions{Force: force})
+	return err
+}
+
+// isNonRetryable returns true for errors that should not be retried.
+func isNonRetryable(err error) bool {
+	msg := strings.ToLower(err.Error())
+	return strings.Contains(msg, "not found") ||
+		strings.Contains(msg, "no such file") ||
+		strings.Contains(msg, "invalid")
+}
diff --git a/internal/org/analyzer_test.go b/internal/org/analyzer_test.go
new file mode 100644
index 0000000..7d69dd2
--- /dev/null
+++ b/internal/org/analyzer_test.go
@@ -0,0 +1,414 @@
+package org
+
+import (
+	"context"
+	"database/sql"
+	"errors"
+	"fmt"
+	"strings"
+	"sync"
+	"sync/atomic"
+	"testing"
+	"time"
+
+	_ "github.com/mattn/go-sqlite3"
+	ctxpkg "github.com/yashpalc/mcp-repo-context/internal/context"
+	"github.com/yashpalc/mcp-repo-context/internal/orchestrator"
+	"github.com/yashpalc/mcp-repo-context/internal/prreview"
+	"github.com/yashpalc/mcp-repo-context/internal/storage"
+)
+
+// mockOrch implements orchestrator.Manager for testing.
+type mockOrch struct {
+	mu            sync.Mutex
+	failRepos     map[string]int    // repo → number of consecutive failures before success (0 = always succeed)
+	callCounts    map[string]int    // repo → times called
+	nonRetryable  map[string]bool   // repo → error is non-retryable
+	latency       time.Duration
+	maxConcurrent int64
+	curConcurrent int64
+	forceFlag     bool // last-seen force value
+}
+
+func newMockOrch() *mockOrch {
+	return &mockOrch{
+		failRepos:  make(map[string]int),
+		callCounts: make(map[string]int),
+		nonRetryable: make(map[string]bool),
+	}
+}
+
+func (m *mockOrch) analyzeCall(repoID string, force bool) error {
+	cur := atomic.AddInt64(&m.curConcurrent, 1)
+	defer atomic.AddInt64(&m.curConcurrent, -1)
+
+	// Track max concurrent
+	for {
+		old := atomic.LoadInt64(&m.maxConcurrent)
+		if cur <= old || atomic.CompareAndSwapInt64(&m.maxConcurrent, old, cur) {
+			break
+		}
+	}
+
+	if m.latency > 0 {
+		time.Sleep(m.latency)
+	}
+
+	m.mu.Lock()
+	m.forceFlag = force
+	m.callCounts[repoID]++
+	calls := m.callCounts[repoID]
+	failCount := m.failRepos[repoID]
+	isNonRetryable := m.nonRetryable[repoID]
+	m.mu.Unlock()
+
+	if failCount > 0 && calls <= failCount {
+		if isNonRetryable {
+			return fmt.Errorf("repo not found: %s", repoID)
+		}
+		return fmt.Errorf("transient error for %s", repoID)
+	}
+	return nil
+}
+
+func (m *mockOrch) AnalyzeRepo(_ context.Context, repoURL string, opts orchestrator.AnalyzeOptions) (*orchestrator.AnalyzeResult, error) {
+	err := m.analyzeCall(repoURL, opts.Force)
+	if err != nil {
+		return nil, err
+	}
+	return &orchestrator.AnalyzeResult{RepoID: repoURL}, nil
+}
+
+func (m *mockOrch) AnalyzeLocal(_ context.Context, dirPath string, opts orchestrator.AnalyzeLocalOptions) (*orchestrator.AnalyzeLocalResult, error) {
+	repoID := "local:" + dirPath
+	err := m.analyzeCall(repoID, opts.Force)
+	if err != nil {
+		return nil, err
+	}
+	return &orchestrator.AnalyzeLocalResult{ProjectID: repoID}, nil
+}
+
+// Stub implementations for the rest of orchestrator.Manager
+func (m *mockOrch) GetContext(context.Context, string) (*ctxpkg.RepoContext, error)             { return nil, nil }
+func (m *mockOrch) GetFileContext(context.Context, string, string) (*ctxpkg.FileContext, error)  { return nil, nil }
+func (m *mockOrch) GetFunctionContext(context.Context, string, string, string) (*orchestrator.FunctionContextResult, error) { return nil, nil }
+func (m *mockOrch) SearchFunctions(context.Context, string, string) ([]ctxpkg.FunctionRef, error) { return nil, nil }
+func (m *mockOrch) SearchByConcept(context.Context, string, string) ([]ctxpkg.FunctionRef, error) { return nil, nil }
+func (m *mockOrch) SearchBySideEffect(context.Context, string, string) ([]ctxpkg.FunctionRef, error) { return nil, nil }
+func (m *mockOrch) ListRepos(context.Context) ([]ctxpkg.ContextMetadata, error)                 { return nil, nil }
+func (m *mockOrch) GenerateAISummary(context.Context, string) (*ctxpkg.AISummary, error)        { return nil, nil }
+func (m *mockOrch) GenerateAIArchAnalysis(context.Context, string) (*ctxpkg.AIArchAnalysis, error) { return nil, nil }
+func (m *mockOrch) IsAIEnabled() bool                                                           { return false }
+func (m *mockOrch) Ask(context.Context, string, []string) (*orchestrator.QueryResult, error)    { return nil, nil }
+func (m *mockOrch) RefreshAIContext(context.Context, []string, bool) (*orchestrator.RefreshResult, error) { return nil, nil }
+func (m *mockOrch) ReviewPR(context.Context, string, prreview.ReviewOptions) (*prreview.ReviewResult, error) { return nil, nil }
+func (m *mockOrch) CheckPRContext(context.Context, string) (*prreview.ContextStatus, error)     { return nil, nil }
+func (m *mockOrch) GetOrAnalyzeLocal(context.Context, string) (*ctxpkg.RepoContext, error)      { return nil, nil }
+func (m *mockOrch) SmartQuery(context.Context, string, string) (*orchestrator.SmartQueryResult, error) { return nil, nil }
+func (m *mockOrch) RefreshFile(context.Context, string, string, orchestrator.RefreshFileOptions) (*orchestrator.RefreshFileResult, error) { return nil, nil }
+func (m *mockOrch) RefreshChangedFiles(context.Context, string) ([]orchestrator.RefreshFileResult, error) { return nil, nil }
+func (m *mockOrch) CheckFileStale(context.Context, string, string) (bool, error)                { return false, nil }
+func (m *mockOrch) GetPRContext(context.Context, string, []orchestrator.ChangedFile) (*orchestrator.PRContextResult, error) { return nil, nil }
+func (m *mockOrch) DeleteRepoContext(context.Context, string) error                             { return nil }
+
+// newAnalyzerTestStore creates an in-memory store + manager for testing the analyzer.
+func newAnalyzerTestSetup(t *testing.T, mo *mockOrch) (Manager, *SQLiteStore) {
+	t.Helper()
+	dsn := nextTestDBName()
+	db, err := sql.Open("sqlite3", dsn)
+	if err != nil {
+		t.Fatalf("Failed to open DB: %v", err)
+	}
+	db.SetMaxOpenConns(1)
+	t.Cleanup(func() { db.Close() })
+
+	_, err = storage.NewSQLiteStoreWithDB(db)
+	if err != nil {
+		t.Fatalf("Failed to run migrations: %v", err)
+	}
+
+	store, err := NewSQLiteStore(db)
+	if err != nil {
+		t.Fatalf("Failed to create org store: %v", err)
+	}
+
+	mgr := NewManager(store, mo)
+	return mgr, store
+}
+
+// --- Analyzer tests ---
+
+func TestAnalyzeOrg_AllSucceed(t *testing.T) {
+	mo := newMockOrch()
+	mgr, _ := newAnalyzerTestSetup(t, mo)
+	ctx := context.Background()
+
+	mgr.Register(ctx, "org-1", []string{"repo-a", "repo-b", "repo-c"}, nil)
+
+	result, err := mgr.AnalyzeOrg(ctx, "org-1", false, 3)
+	if err != nil {
+		t.Fatalf("AnalyzeOrg failed: %v", err)
+	}
+	if result.Total != 3 {
+		t.Errorf("Total = %d, want 3", result.Total)
+	}
+	if result.Succeeded != 3 {
+		t.Errorf("Succeeded = %d, want 3", result.Succeeded)
+	}
+	if result.Failed != 0 {
+		t.Errorf("Failed = %d, want 0", result.Failed)
+	}
+	if len(result.Errors) != 0 {
+		t.Errorf("Errors = %v, want empty", result.Errors)
+	}
+}
+
+func TestAnalyzeOrg_RetrySucceeds(t *testing.T) {
+	mo := newMockOrch()
+	mo.failRepos["repo-b"] = 1 // fail first call, succeed on retry
+	mgr, _ := newAnalyzerTestSetup(t, mo)
+	ctx := context.Background()
+
+	mgr.Register(ctx, "org-1", []string{"repo-a", "repo-b"}, nil)
+
+	result, err := mgr.AnalyzeOrg(ctx, "org-1", false, 3)
+	if err != nil {
+		t.Fatalf("AnalyzeOrg failed: %v", err)
+	}
+	if result.Succeeded != 2 {
+		t.Errorf("Succeeded = %d, want 2 (retry should succeed)", result.Succeeded)
+	}
+	if result.Failed != 0 {
+		t.Errorf("Failed = %d, want 0", result.Failed)
+	}
+}
+
+func TestAnalyzeOrg_RetryFails(t *testing.T) {
+	mo := newMockOrch()
+	mo.failRepos["repo-b"] = 99 // always fail
+	mgr, _ := newAnalyzerTestSetup(t, mo)
+	ctx := context.Background()
+
+	mgr.Register(ctx, "org-1", []string{"repo-a", "repo-b"}, nil)
+
+	result, err := mgr.AnalyzeOrg(ctx, "org-1", false, 3)
+	if err != nil {
+		t.Fatalf("AnalyzeOrg failed: %v", err)
+	}
+	if result.Succeeded != 1 {
+		t.Errorf("Succeeded = %d, want 1", result.Succeeded)
+	}
+	if result.Failed != 1 {
+		t.Errorf("Failed = %d, want 1", result.Failed)
+	}
+	if len(result.Errors) != 1 {
+		t.Fatalf("Errors len = %d, want 1", len(result.Errors))
+	}
+	if result.Errors[0].RepoID != "repo-b" {
+		t.Errorf("Errors[0].RepoID = %q, want repo-b", result.Errors[0].RepoID)
+	}
+}
+
+func TestAnalyzeOrg_ConcurrencyLimit(t *testing.T) {
+	mo := newMockOrch()
+	mo.latency = 50 * time.Millisecond
+	mgr, _ := newAnalyzerTestSetup(t, mo)
+	ctx := context.Background()
+
+	repos := make([]string, 10)
+	for i := range repos {
+		repos[i] = fmt.Sprintf("repo-%d", i)
+	}
+	mgr.Register(ctx, "org-1", repos, nil)
+
+	result, err := mgr.AnalyzeOrg(ctx, "org-1", false, 2)
+	if err != nil {
+		t.Fatalf("AnalyzeOrg failed: %v", err)
+	}
+	if result.Succeeded != 10 {
+		t.Errorf("Succeeded = %d, want 10", result.Succeeded)
+	}
+	if atomic.LoadInt64(&mo.maxConcurrent) > 2 {
+		t.Errorf("maxConcurrent = %d, want <= 2", mo.maxConcurrent)
+	}
+}
+
+func TestAnalyzeOrg_ContextCancellation(t *testing.T) {
+	mo := newMockOrch()
+	mo.latency = 200 * time.Millisecond
+	mgr, _ := newAnalyzerTestSetup(t, mo)
+
+	repos := make([]string, 20)
+	for i := range repos {
+		repos[i] = fmt.Sprintf("repo-%d", i)
+	}
+	ctx := context.Background()
+	mgr.Register(ctx, "org-1", repos, nil)
+
+	ctx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
+	defer cancel()
+
+	result, err := mgr.AnalyzeOrg(ctx, "org-1", false, 1) // concurrency=1, each takes 200ms
+	if err != nil {
+		t.Fatalf("AnalyzeOrg failed: %v", err)
+	}
+	// With 1 concurrency and 200ms latency, 100ms timeout should complete 0-1 repos
+	if result.Succeeded >= len(repos) {
+		t.Errorf("Succeeded = %d, want < %d (should have been cancelled)", result.Succeeded, len(repos))
+	}
+}
+
+func TestAnalyzeOrg_ForceFlag(t *testing.T) {
+	mo := newMockOrch()
+	mgr, _ := newAnalyzerTestSetup(t, mo)
+	ctx := context.Background()
+
+	mgr.Register(ctx, "org-1", []string{"repo-a"}, nil)
+
+	_, err := mgr.AnalyzeOrg(ctx, "org-1", true, 3)
+	if err != nil {
+		t.Fatalf("AnalyzeOrg failed: %v", err)
+	}
+
+	mo.mu.Lock()
+	gotForce := mo.forceFlag
+	mo.mu.Unlock()
+	if !gotForce {
+		t.Error("force flag was not passed to orchestrator")
+	}
+}
+
+func TestAnalyzeOrg_EmptyOrg(t *testing.T) {
+	mo := newMockOrch()
+	mgr, _ := newAnalyzerTestSetup(t, mo)
+	ctx := context.Background()
+
+	mgr.Register(ctx, "org-1", []string{}, nil)
+
+	result, err := mgr.AnalyzeOrg(ctx, "org-1", false, 3)
+	if err != nil {
+		t.Fatalf("AnalyzeOrg failed: %v", err)
+	}
+	if result.Total != 0 {
+		t.Errorf("Total = %d, want 0", result.Total)
+	}
+	if result.Succeeded != 0 {
+		t.Errorf("Succeeded = %d, want 0", result.Succeeded)
+	}
+	if result.Duration <= 0 {
+		t.Error("Duration should be > 0")
+	}
+}
+
+func TestAnalyzeOrg_NonExistentOrg(t *testing.T) {
+	mo := newMockOrch()
+	mgr, _ := newAnalyzerTestSetup(t, mo)
+	ctx := context.Background()
+
+	_, err := mgr.AnalyzeOrg(ctx, "ghost-org", false, 3)
+	if !errors.Is(err, ErrNotFound) {
+		t.Errorf("AnalyzeOrg error = %v, want ErrNotFound", err)
+	}
+}
+
+func TestAnalyzeOrg_RoutesLocalPrefix(t *testing.T) {
+	mo := newMockOrch()
+	mgr, _ := newAnalyzerTestSetup(t, mo)
+	ctx := context.Background()
+
+	mgr.Register(ctx, "org-1", []string{"local:/tmp/myrepo", "github.com/foo/bar"}, nil)
+
+	result, err := mgr.AnalyzeOrg(ctx, "org-1", false, 3)
+	if err != nil {
+		t.Fatalf("AnalyzeOrg failed: %v", err)
+	}
+	if result.Succeeded != 2 {
+		t.Errorf("Succeeded = %d, want 2", result.Succeeded)
+	}
+
+	mo.mu.Lock()
+	// local: prefix should route to AnalyzeLocal (which uses "local:" + dirPath as key)
+	localCalls := mo.callCounts["local:/tmp/myrepo"]
+	remoteCalls := mo.callCounts["github.com/foo/bar"]
+	mo.mu.Unlock()
+
+	if localCalls != 1 {
+		t.Errorf("local repo callCount = %d, want 1", localCalls)
+	}
+	if remoteCalls != 1 {
+		t.Errorf("remote repo callCount = %d, want 1", remoteCalls)
+	}
+}
+
+func TestAnalyzeOrg_Duration(t *testing.T) {
+	mo := newMockOrch()
+	mgr, _ := newAnalyzerTestSetup(t, mo)
+	ctx := context.Background()
+
+	mgr.Register(ctx, "org-1", []string{"repo-a"}, nil)
+
+	result, err := mgr.AnalyzeOrg(ctx, "org-1", false, 3)
+	if err != nil {
+		t.Fatalf("AnalyzeOrg failed: %v", err)
+	}
+	if result.Duration <= 0 {
+		t.Error("Duration should be > 0")
+	}
+}
+
+func TestAnalyzeOrg_ClampsConcurrency(t *testing.T) {
+	mo := newMockOrch()
+	mgr, _ := newAnalyzerTestSetup(t, mo)
+	ctx := context.Background()
+
+	mgr.Register(ctx, "org-1", []string{"repo-a"}, nil)
+
+	// 0 should become 3, 15 should become 10 — both should just work
+	result, err := mgr.AnalyzeOrg(ctx, "org-1", false, 0)
+	if err != nil {
+		t.Fatalf("AnalyzeOrg (concurrency=0) failed: %v", err)
+	}
+	if result.Succeeded != 1 {
+		t.Errorf("Succeeded = %d, want 1", result.Succeeded)
+	}
+
+	result, err = mgr.AnalyzeOrg(ctx, "org-1", false, 15)
+	if err != nil {
+		t.Fatalf("AnalyzeOrg (concurrency=15) failed: %v", err)
+	}
+	if result.Succeeded != 1 {
+		t.Errorf("Succeeded = %d, want 1", result.Succeeded)
+	}
+}
+
+func TestAnalyzeOrg_SkipsRetryForNonRetryable(t *testing.T) {
+	mo := newMockOrch()
+	mo.failRepos["repo-b"] = 99       // always fail
+	mo.nonRetryable["repo-b"] = true   // "not found" error
+	mgr, _ := newAnalyzerTestSetup(t, mo)
+	ctx := context.Background()
+
+	mgr.Register(ctx, "org-1", []string{"repo-b"}, nil)
+
+	result, err := mgr.AnalyzeOrg(ctx, "org-1", false, 3)
+	if err != nil {
+		t.Fatalf("AnalyzeOrg failed: %v", err)
+	}
+	if result.Failed != 1 {
+		t.Errorf("Failed = %d, want 1", result.Failed)
+	}
+
+	mo.mu.Lock()
+	calls := mo.callCounts["repo-b"]
+	mo.mu.Unlock()
+
+	// Non-retryable: should only be called once (no retry)
+	if calls != 1 {
+		t.Errorf("callCount for repo-b = %d, want 1 (no retry for non-retryable)", calls)
+	}
+
+	// Error message should contain "not found"
+	if len(result.Errors) != 1 || !strings.Contains(result.Errors[0].Error, "not found") {
+		t.Errorf("Errors = %v, want 'not found' error", result.Errors)
+	}
+}
diff --git a/internal/org/manager.go b/internal/org/manager.go
index 4eb9877..f421b04 100644
--- a/internal/org/manager.go
+++ b/internal/org/manager.go
@@ -4,6 +4,8 @@ import (
 	"context"
 	"fmt"
 	"time"
+
+	"github.com/yashpalc/mcp-repo-context/internal/orchestrator"
 )
 
 // Manager manages organizations.
@@ -16,16 +18,20 @@ type Manager interface {
 	Delete(ctx context.Context, orgID string) error
 	GetEffectiveConfig(ctx context.Context, orgID, repoID string) (*OrgConfig, error)
 	SetRepoConfigOverride(ctx context.Context, orgID, repoID string, config *OrgConfig) error
+	AnalyzeOrg(ctx context.Context, orgID string, force bool, concurrency int) (*AnalysisResult, error)
 }
 
 // manager implements Manager.
 type manager struct {
-	store Store
+	store    Store
+	analyzer *Analyzer
 }
 
 // NewManager creates a new OrgManager.
-func NewManager(store Store) Manager {
-	return &manager{store: store}
+func NewManager(store Store, orch orchestrator.Manager) Manager {
+	m := &manager{store: store}
+	m.analyzer = NewAnalyzer(m, orch)
+	return m
 }
 
 func (m *manager) Register(ctx context.Context, orgID string, repoIDs []string, config *OrgConfig) (*Org, error) {
@@ -84,6 +90,10 @@ func (m *manager) SetRepoConfigOverride(ctx context.Context, orgID, repoID strin
 	return m.store.SetRepoConfigOverride(ctx, orgID, repoID, config)
 }
 
+func (m *manager) AnalyzeOrg(ctx context.Context, orgID string, force bool, concurrency int) (*AnalysisResult, error) {
+	return m.analyzer.AnalyzeOrg(ctx, orgID, force, concurrency)
+}
+
 func uniqueStrings(ss []string) []string {
 	seen := make(map[string]bool)
 	var result []string
diff --git a/internal/org/store_test.go b/internal/org/store_test.go
index d1ec3ff..4cdb97b 100644
--- a/internal/org/store_test.go
+++ b/internal/org/store_test.go
@@ -606,7 +606,7 @@ func TestConcurrent_ReadsAndWrites(t *testing.T) {
 func TestManager_GetEffectiveConfig(t *testing.T) {
 	store := newTestStore(t)
 	ctx := context.Background()
-	mgr := NewManager(store)
+	mgr := NewManager(store, newMockOrch())
 
 	// Register org with config
 	_, err := mgr.Register(ctx, "org-1", []string{"repo-a"}, &OrgConfig{
@@ -651,7 +651,7 @@ func TestManager_GetEffectiveConfig(t *testing.T) {
 func TestManager_DelegatesAtomicOps(t *testing.T) {
 	store := newTestStore(t)
 	ctx := context.Background()
-	mgr := NewManager(store)
+	mgr := NewManager(store, newMockOrch())
 
 	mgr.Register(ctx, "org-1", []string{"repo-a"}, nil)
 
diff --git a/internal/org/types.go b/internal/org/types.go
index e4f6cce..9c47dd1 100644
--- a/internal/org/types.go
+++ b/internal/org/types.go
@@ -26,3 +26,20 @@ type OrgWithCount struct {
 	Org
 	RepoCount int `json:"repo_count"`
 }
+
+// AnalysisResult holds the outcome of analyzing all repos in an org.
+type AnalysisResult struct {
+	OrgID     string        `json:"org_id"`
+	Total     int           `json:"total"`
+	Succeeded int           `json:"succeeded"`
+	Failed    int           `json:"failed"`
+	Skipped   int           `json:"skipped"`
+	Errors    []RepoError   `json:"errors,omitempty"`
+	Duration  time.Duration `json:"duration"`
+}
+
+// RepoError records a per-repo analysis failure.
+type RepoError struct {
+	RepoID string `json:"repo_id"`
+	Error  string `json:"error"`
+}
diff --git a/planning/01-org-abstraction/implementation/deep_implement_config.json b/planning/01-org-abstraction/implementation/deep_implement_config.json
index e463485..425e972 100644
--- a/planning/01-org-abstraction/implementation/deep_implement_config.json
+++ b/planning/01-org-abstraction/implementation/deep_implement_config.json
@@ -23,6 +23,10 @@
     "section-02-config-inheritance": {
       "status": "complete",
       "commit_hash": "018a8a8"
+    },
+    "section-03-store-interface-sqlite": {
+      "status": "complete",
+      "commit_hash": "7646eec"
     }
   },
   "pre_commit": {
