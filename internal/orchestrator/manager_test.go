package orchestrator

import (
	"context"
	"errors"
	"testing"
	"time"

	ctxpkg "github.com/yashpalc/mcp-repo-context/internal/context"
	"github.com/yashpalc/mcp-repo-context/internal/repo"
	"github.com/yashpalc/mcp-repo-context/internal/storage"
)

// mockSource implements repo.Source for testing
type mockSource struct {
	cloneFn     func(ctx context.Context, url string, opts repo.CloneOptions) (string, func(), error)
	commitHash  string
	branch      string
	commitErr   error
	branchErr   error
}

func (m *mockSource) Clone(ctx context.Context, url string, opts repo.CloneOptions) (string, func(), error) {
	if m.cloneFn != nil {
		return m.cloneFn(ctx, url, opts)
	}
	return "", func() {}, nil
}

func (m *mockSource) GetCurrentCommit(ctx context.Context, path string) (string, error) {
	return m.commitHash, m.commitErr
}

func (m *mockSource) GetBranch(ctx context.Context, path string) (string, error) {
	return m.branch, m.branchErr
}

// mockScanner implements repo.FileScanner for testing
type mockScanner struct {
	files []repo.FileInfo
	err   error
}

func (m *mockScanner) Scan(ctx context.Context, root string, fn func(repo.FileInfo) error) error {
	if m.err != nil {
		return m.err
	}
	for _, f := range m.files {
		if err := fn(f); err != nil {
			return err
		}
	}
	return nil
}

// mockStore implements storage.ContextStore for testing
type mockStore struct {
	contexts     map[string]*ctxpkg.RepoContext
	storeErr     error
	getErr       error
	existsResult bool
	existsTime   time.Time
}

func newMockStore() *mockStore {
	return &mockStore{
		contexts: make(map[string]*ctxpkg.RepoContext),
	}
}

func (m *mockStore) StoreRepoContext(ctx context.Context, repoID string, repoCtx *ctxpkg.RepoContext) error {
	if m.storeErr != nil {
		return m.storeErr
	}
	m.contexts[repoID] = repoCtx
	return nil
}

func (m *mockStore) GetRepoContext(ctx context.Context, repoID string) (*ctxpkg.RepoContext, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	if c, ok := m.contexts[repoID]; ok {
		return c, nil
	}
	return nil, storage.ErrNotFound
}

func (m *mockStore) GetFileContext(ctx context.Context, repoID, filePath string) (*ctxpkg.FileContext, error) {
	repoCtx, err := m.GetRepoContext(ctx, repoID)
	if err != nil {
		return nil, err
	}
	if fc, ok := repoCtx.Files[filePath]; ok {
		return fc, nil
	}
	return nil, storage.ErrNotFound
}

func (m *mockStore) ContextExists(ctx context.Context, repoID string, maxAge time.Duration) (bool, time.Time, error) {
	return m.existsResult, m.existsTime, nil
}

func (m *mockStore) DeleteContext(ctx context.Context, repoID string) error {
	delete(m.contexts, repoID)
	return nil
}

func (m *mockStore) ListContexts(ctx context.Context) ([]ctxpkg.ContextMetadata, error) {
	var result []ctxpkg.ContextMetadata
	for _, c := range m.contexts {
		result = append(result, ctxpkg.ContextMetadata{
			RepoID:     c.ID,
			URL:        c.URL,
			Branch:     c.Branch,
			CommitHash: c.CommitHash,
			FileCount:  len(c.Files),
			AnalyzedAt: c.AnalyzedAt,
		})
	}
	return result, nil
}

func TestNewManager(t *testing.T) {
	store := newMockStore()
	source := &mockSource{}
	scanner := &mockScanner{}

	m := NewManager(store, source, scanner)
	if m == nil {
		t.Fatal("expected non-nil manager")
	}
}

func TestManager_GetContext(t *testing.T) {
	store := newMockStore()
	repoID := "github.com/test/repo"
	store.contexts[repoID] = &ctxpkg.RepoContext{
		ID:     repoID,
		URL:    "https://github.com/test/repo",
		Branch: "main",
	}

	m := NewManager(store, &mockSource{}, &mockScanner{})
	ctx := context.Background()

	result, err := m.GetContext(ctx, repoID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.ID != repoID {
		t.Errorf("expected ID %s, got %s", repoID, result.ID)
	}
}

func TestManager_GetContext_NotFound(t *testing.T) {
	store := newMockStore()
	m := NewManager(store, &mockSource{}, &mockScanner{})
	ctx := context.Background()

	_, err := m.GetContext(ctx, "nonexistent")
	if err != storage.ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestManager_GetFileContext(t *testing.T) {
	store := newMockStore()
	repoID := "github.com/test/repo"
	store.contexts[repoID] = &ctxpkg.RepoContext{
		ID: repoID,
		Files: map[string]*ctxpkg.FileContext{
			"main.go": {
				Path:      "main.go",
				Language:  "go",
				LineCount: 50,
			},
		},
	}

	m := NewManager(store, &mockSource{}, &mockScanner{})
	ctx := context.Background()

	result, err := m.GetFileContext(ctx, repoID, "main.go")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Path != "main.go" {
		t.Errorf("expected path main.go, got %s", result.Path)
	}
}

func TestManager_ListRepos(t *testing.T) {
	store := newMockStore()
	store.contexts["repo1"] = &ctxpkg.RepoContext{ID: "repo1", URL: "url1"}
	store.contexts["repo2"] = &ctxpkg.RepoContext{ID: "repo2", URL: "url2"}

	m := NewManager(store, &mockSource{}, &mockScanner{})
	ctx := context.Background()

	result, err := m.ListRepos(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result) != 2 {
		t.Errorf("expected 2 repos, got %d", len(result))
	}
}

func TestManager_AnalyzeRepo_UsesCache(t *testing.T) {
	store := newMockStore()
	store.existsResult = true
	store.existsTime = time.Now()

	source := &mockSource{
		cloneFn: func(ctx context.Context, url string, opts repo.CloneOptions) (string, func(), error) {
			t.Error("Clone should not be called when cache exists")
			return "", func() {}, nil
		},
	}

	m := NewManager(store, source, &mockScanner{})
	ctx := context.Background()

	opts := AnalyzeOptions{
		Force:  false,
		MaxAge: time.Hour,
	}

	result, err := m.AnalyzeRepo(ctx, "https://github.com/test/repo", opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Warnings) == 0 {
		t.Error("expected cache warning")
	}
}

func TestManager_AnalyzeRepo_ForceBypassesCache(t *testing.T) {
	store := newMockStore()
	store.existsResult = true
	store.existsTime = time.Now()

	cloneCalled := false
	source := &mockSource{
		cloneFn: func(ctx context.Context, url string, opts repo.CloneOptions) (string, func(), error) {
			cloneCalled = true
			return t.TempDir(), func() {}, nil
		},
		commitHash: "abc123",
		branch:     "main",
	}

	m := NewManager(store, source, &mockScanner{})
	ctx := context.Background()

	opts := AnalyzeOptions{
		Force: true,
	}

	_, err := m.AnalyzeRepo(ctx, "https://github.com/test/repo", opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !cloneCalled {
		t.Error("expected Clone to be called with Force=true")
	}
}

func TestManager_AnalyzeRepo_CloneError(t *testing.T) {
	store := newMockStore()
	source := &mockSource{
		cloneFn: func(ctx context.Context, url string, opts repo.CloneOptions) (string, func(), error) {
			return "", nil, errors.New("clone failed")
		},
	}

	m := NewManager(store, source, &mockScanner{})
	ctx := context.Background()

	_, err := m.AnalyzeRepo(ctx, "https://github.com/test/repo", AnalyzeOptions{Force: true})
	if err == nil {
		t.Error("expected error")
	}
}
