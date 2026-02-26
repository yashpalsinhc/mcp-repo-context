package org

import (
	"context"
	"fmt"
	"os"
	"testing"

	_ "github.com/mattn/go-sqlite3"
	ctxpkg "github.com/yashpalc/mcp-repo-context/internal/context"
	"github.com/yashpalc/mcp-repo-context/internal/vectors"
)

// mockContextLoader implements ContextLoader for testing.
type mockContextLoader struct {
	contexts map[string]*ctxpkg.RepoContext
}

func (m *mockContextLoader) GetContext(_ context.Context, repoID string) (*ctxpkg.RepoContext, error) {
	rc, ok := m.contexts[repoID]
	if !ok {
		return nil, fmt.Errorf("repo not found: %s", repoID)
	}
	return rc, nil
}

// mockVocabStorer implements VocabStorer for testing.
type mockVocabStorer struct {
	stored map[string][]byte
}

func newMockVocabStorer() *mockVocabStorer {
	return &mockVocabStorer{stored: make(map[string][]byte)}
}

func (m *mockVocabStorer) StoreOrgVocabulary(_ context.Context, orgID string, vocabJSON []byte, _ string, _, _ int) error {
	m.stored[orgID] = vocabJSON
	return nil
}

// mockOrgManager implements Manager for testing.
type mockOrgManager struct {
	orgs map[string]*Org
}

func (m *mockOrgManager) Register(_ context.Context, _ string, _ []string, _ *OrgConfig) (*Org, error) {
	return nil, nil
}
func (m *mockOrgManager) List(_ context.Context) ([]OrgWithCount, error)    { return nil, nil }
func (m *mockOrgManager) AddRepos(_ context.Context, _ string, _ []string) error   { return nil }
func (m *mockOrgManager) RemoveRepos(_ context.Context, _ string, _ []string) error { return nil }
func (m *mockOrgManager) Delete(_ context.Context, _ string) error           { return nil }
func (m *mockOrgManager) GetEffectiveConfig(_ context.Context, _, _ string) (*OrgConfig, error) {
	return nil, nil
}
func (m *mockOrgManager) SetRepoConfigOverride(_ context.Context, _, _ string, _ *OrgConfig) error {
	return nil
}
func (m *mockOrgManager) AnalyzeOrg(_ context.Context, _ string, _ bool, _ int) (*AnalysisResult, error) {
	return nil, nil
}
func (m *mockOrgManager) IndexOrg(_ context.Context, _ string, _ bool, _ int) (*IndexOrgResult, error) {
	return nil, nil
}

func (m *mockOrgManager) Get(_ context.Context, orgID string) (*Org, error) {
	o, ok := m.orgs[orgID]
	if !ok {
		return nil, fmt.Errorf("org not found: %s", orgID)
	}
	return o, nil
}

func setupIndexerTest(t *testing.T) (*Indexer, *vectors.SQLiteVectorStore, *mockVocabStorer, func()) {
	t.Helper()

	tmpFile, err := os.CreateTemp("", "indexer_test_*.db")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	tmpFile.Close()

	store, err := vectors.NewSQLiteVectorStore(tmpFile.Name(), 256)
	if err != nil {
		os.Remove(tmpFile.Name())
		t.Fatalf("failed to create vector store: %v", err)
	}

	embedder := vectors.NewDefaultEmbedder()
	ss := vectors.NewSemanticSearch(embedder, store)

	vocabStorer := newMockVocabStorer()

	// Build mock org manager and context loader
	orgMgr := &mockOrgManager{orgs: make(map[string]*Org)}
	ctxLoader := &mockContextLoader{contexts: make(map[string]*ctxpkg.RepoContext)}

	indexer := NewIndexer(orgMgr, ctxLoader, ss, vocabStorer)

	cleanup := func() {
		store.Close()
		os.Remove(tmpFile.Name())
	}

	// Return access to mock org manager and context loader via closure
	return indexer, store, vocabStorer, cleanup
}

func makeTestRepo(repoID string, files map[string][]ctxpkg.FunctionDef) *ctxpkg.RepoContext {
	rc := &ctxpkg.RepoContext{
		ID:    repoID,
		Files: make(map[string]*ctxpkg.FileContext),
	}
	for path, fns := range files {
		fc := &ctxpkg.FileContext{Path: path, Functions: fns}
		rc.Files[path] = fc
	}
	return rc
}

func TestIndexOrg_IndexesAllRepos(t *testing.T) {
	indexer, store, _, cleanup := setupIndexerTest(t)
	defer cleanup()

	ctx := context.Background()

	repoA := makeTestRepo("repo-a", map[string][]ctxpkg.FunctionDef{
		"pkg/user.go": {
			{Name: "CreateUser", Signature: "func CreateUser() error", Description: "Creates user", IsPublic: true},
		},
	})
	repoB := makeTestRepo("repo-b", map[string][]ctxpkg.FunctionDef{
		"pkg/notify.go": {
			{Name: "SendNotification", Signature: "func SendNotification() error", Description: "Sends notification", IsPublic: true},
		},
	})

	// Wire up mock data
	indexer.orgManager.(*mockOrgManager).orgs["test-org"] = &Org{
		ID:    "test-org",
		Repos: []string{"repo-a", "repo-b"},
	}
	indexer.ctxLoader.(*mockContextLoader).contexts["repo-a"] = repoA
	indexer.ctxLoader.(*mockContextLoader).contexts["repo-b"] = repoB

	result, err := indexer.IndexOrg(ctx, "test-org", false, 3)
	if err != nil {
		t.Fatalf("IndexOrg: %v", err)
	}

	if result.ReposIndexed != 2 {
		t.Errorf("expected 2 repos indexed, got %d", result.ReposIndexed)
	}
	if result.ReposFailed != 0 {
		t.Errorf("expected 0 failures, got %d", result.ReposFailed)
	}
	if result.TotalVectors < 2 {
		t.Errorf("expected at least 2 vectors, got %d", result.TotalVectors)
	}

	// Verify vectors exist
	vec, _ := store.Get(ctx, "repo-a:func:pkg/user.go:CreateUser")
	if vec == nil {
		t.Error("expected repo-a vector to exist")
	}
	vec, _ = store.Get(ctx, "repo-b:func:pkg/notify.go:SendNotification")
	if vec == nil {
		t.Error("expected repo-b vector to exist")
	}
}

func TestIndexOrg_BuildsVocabulary(t *testing.T) {
	indexer, _, vocabStore, cleanup := setupIndexerTest(t)
	defer cleanup()

	ctx := context.Background()

	repoA := makeTestRepo("repo-a", map[string][]ctxpkg.FunctionDef{
		"a.go": {{Name: "FuncA", Signature: "func FuncA()", Description: "Function A", IsPublic: true}},
	})

	indexer.orgManager.(*mockOrgManager).orgs["org1"] = &Org{ID: "org1", Repos: []string{"repo-a"}}
	indexer.ctxLoader.(*mockContextLoader).contexts["repo-a"] = repoA

	_, err := indexer.IndexOrg(ctx, "org1", false, 3)
	if err != nil {
		t.Fatalf("IndexOrg: %v", err)
	}

	if _, ok := vocabStore.stored["org1"]; !ok {
		t.Error("expected vocabulary to be stored for org1")
	}
}

func TestIndexOrg_HandlesPartialFailure(t *testing.T) {
	indexer, _, _, cleanup := setupIndexerTest(t)
	defer cleanup()

	ctx := context.Background()

	repoA := makeTestRepo("repo-a", map[string][]ctxpkg.FunctionDef{
		"a.go": {{Name: "FuncA", Signature: "func FuncA()", IsPublic: true}},
	})

	// repo-b has no context (will fail to load)
	indexer.orgManager.(*mockOrgManager).orgs["org1"] = &Org{ID: "org1", Repos: []string{"repo-a", "repo-b"}}
	indexer.ctxLoader.(*mockContextLoader).contexts["repo-a"] = repoA
	// repo-b intentionally missing from contexts

	result, err := indexer.IndexOrg(ctx, "org1", false, 3)
	if err != nil {
		t.Fatalf("IndexOrg: %v", err)
	}

	if result.ReposIndexed != 1 {
		t.Errorf("expected 1 repo indexed, got %d", result.ReposIndexed)
	}
	if result.ReposFailed != 1 {
		t.Errorf("expected 1 failure, got %d", result.ReposFailed)
	}
	if len(result.Failures) != 1 {
		t.Errorf("expected 1 failure entry, got %d", len(result.Failures))
	}
}

func TestIndexOrg_EmptyOrg(t *testing.T) {
	indexer, _, _, cleanup := setupIndexerTest(t)
	defer cleanup()

	ctx := context.Background()

	indexer.orgManager.(*mockOrgManager).orgs["empty-org"] = &Org{ID: "empty-org", Repos: []string{}}

	result, err := indexer.IndexOrg(ctx, "empty-org", false, 3)
	if err != nil {
		t.Fatalf("IndexOrg: %v", err)
	}

	if result.ReposIndexed != 0 {
		t.Errorf("expected 0 repos indexed, got %d", result.ReposIndexed)
	}
}

func TestIndexOrg_UnknownOrg(t *testing.T) {
	indexer, _, _, cleanup := setupIndexerTest(t)
	defer cleanup()

	ctx := context.Background()

	_, err := indexer.IndexOrg(ctx, "nonexistent", false, 3)
	if err == nil {
		t.Error("expected error for unknown org")
	}
}
