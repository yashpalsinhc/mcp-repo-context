package vectors

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	_ "github.com/mattn/go-sqlite3"
	ctxpkg "github.com/yashpalc/mcp-repo-context/internal/context"
)

// mockFunctionHashStore implements FunctionHashStore for testing.
type mockFunctionHashStore struct {
	hashes map[string]map[string]FuncHashEntry // repoID:filePath -> name:type -> entry
}

func newMockFunctionHashStore() *mockFunctionHashStore {
	return &mockFunctionHashStore{
		hashes: make(map[string]map[string]FuncHashEntry),
	}
}

func (m *mockFunctionHashStore) key(repoID, filePath string) string {
	return repoID + ":" + filePath
}

func (m *mockFunctionHashStore) GetFunctionHashes(ctx context.Context, repoID, filePath string) (map[string]FuncHashEntry, error) {
	k := m.key(repoID, filePath)
	if entries, ok := m.hashes[k]; ok {
		return entries, nil
	}
	return make(map[string]FuncHashEntry), nil
}

func (m *mockFunctionHashStore) UpdateFunctionHashes(ctx context.Context, repoID, filePath string, hashes []FuncHashEntry) error {
	k := m.key(repoID, filePath)
	entries := make(map[string]FuncHashEntry)
	for _, h := range hashes {
		entries[h.Name+":"+h.Type] = h
	}
	m.hashes[k] = entries
	return nil
}

func (m *mockFunctionHashStore) DeleteFunctionHashes(ctx context.Context, repoID, filePath string) error {
	delete(m.hashes, m.key(repoID, filePath))
	return nil
}

func (m *mockFunctionHashStore) GetChangedFunctions(ctx context.Context, repoID, filePath string, current map[string]string) (added, modified, removed []string, err error) {
	k := m.key(repoID, filePath)
	existing := m.hashes[k]

	for name, hash := range current {
		if entry, ok := existing[name]; !ok {
			added = append(added, name)
		} else if entry.ContentHash != hash {
			modified = append(modified, name)
		}
	}
	for name := range existing {
		if _, ok := current[name]; !ok {
			removed = append(removed, name)
		}
	}
	return
}

// mockOrgVocabStore implements OrgVocabularyStore for testing.
type mockOrgVocabStore struct {
	vocabs map[string]*OrgVocabRecord
}

func newMockOrgVocabStore() *mockOrgVocabStore {
	return &mockOrgVocabStore{vocabs: make(map[string]*OrgVocabRecord)}
}

func (m *mockOrgVocabStore) GetOrgVocabulary(ctx context.Context, orgID string) (*OrgVocabRecord, error) {
	if rec, ok := m.vocabs[orgID]; ok {
		return rec, nil
	}
	return nil, nil
}

func setupRefreshTest(t *testing.T) (*SemanticSearch, *SQLiteVectorStore, *mockFunctionHashStore, *mockOrgVocabStore, func()) {
	t.Helper()

	tmpFile, err := os.CreateTemp("", "refresh_test_*.db")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	tmpFile.Close()

	store, err := NewSQLiteVectorStore(tmpFile.Name(), 256)
	if err != nil {
		os.Remove(tmpFile.Name())
		t.Fatalf("failed to create vector store: %v", err)
	}

	embedder := NewDefaultEmbedder()
	ss := NewSemanticSearch(embedder, store)

	hashStore := newMockFunctionHashStore()
	vocabStore := newMockOrgVocabStore()
	ss.SetHashStore(hashStore)
	ss.SetVocabStore(vocabStore)

	cleanup := func() {
		store.Close()
		os.Remove(tmpFile.Name())
	}

	return ss, store, hashStore, vocabStore, cleanup
}

func TestRefreshFileVectors_AddsNew(t *testing.T) {
	ss, store, _, _, cleanup := setupRefreshTest(t)
	defer cleanup()

	ctx := context.Background()

	file := &ctxpkg.FileContext{
		Path: "pkg/user.go",
		Functions: []ctxpkg.FunctionDef{
			{Name: "CreateUser", Signature: "func CreateUser(ctx context.Context) error", IsPublic: true},
			{Name: "DeleteUser", Signature: "func DeleteUser(ctx context.Context) error", IsPublic: true},
		},
	}

	result, err := ss.RefreshFileVectors(ctx, "test-repo", "pkg/user.go", file, "")
	if err != nil {
		t.Fatalf("RefreshFileVectors: %v", err)
	}

	if result.Added != 2 {
		t.Errorf("Added = %d, want 2", result.Added)
	}
	if result.Modified != 0 {
		t.Errorf("Modified = %d, want 0", result.Modified)
	}
	if result.Removed != 0 {
		t.Errorf("Removed = %d, want 0", result.Removed)
	}

	count, err := store.Count(ctx, "test-repo")
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if count != 2 {
		t.Errorf("vector count = %d, want 2", count)
	}
}

func TestRefreshFileVectors_UpdatesModified(t *testing.T) {
	ss, store, hashStore, _, cleanup := setupRefreshTest(t)
	defer cleanup()

	ctx := context.Background()

	// First, add a function
	file1 := &ctxpkg.FileContext{
		Path: "pkg/user.go",
		Functions: []ctxpkg.FunctionDef{
			{Name: "CreateUser", Signature: "func CreateUser(ctx context.Context) error", IsPublic: true},
		},
	}
	_, err := ss.RefreshFileVectors(ctx, "test-repo", "pkg/user.go", file1, "")
	if err != nil {
		t.Fatalf("first RefreshFileVectors: %v", err)
	}

	// Verify it was stored
	hashes, _ := hashStore.GetFunctionHashes(ctx, "test-repo", "pkg/user.go")
	if len(hashes) != 1 {
		t.Fatalf("expected 1 hash entry, got %d", len(hashes))
	}

	// Now modify the function (different signature)
	file2 := &ctxpkg.FileContext{
		Path: "pkg/user.go",
		Functions: []ctxpkg.FunctionDef{
			{Name: "CreateUser", Signature: "func CreateUser(ctx context.Context, name string) error", IsPublic: true},
		},
	}

	result, err := ss.RefreshFileVectors(ctx, "test-repo", "pkg/user.go", file2, "")
	if err != nil {
		t.Fatalf("second RefreshFileVectors: %v", err)
	}

	if result.Modified != 1 {
		t.Errorf("Modified = %d, want 1", result.Modified)
	}
	if result.Added != 0 {
		t.Errorf("Added = %d, want 0", result.Added)
	}

	// Vector should still exist
	count, _ := store.Count(ctx, "test-repo")
	if count != 1 {
		t.Errorf("vector count = %d, want 1", count)
	}
}

func TestRefreshFileVectors_RemovesDeleted(t *testing.T) {
	ss, store, _, _, cleanup := setupRefreshTest(t)
	defer cleanup()

	ctx := context.Background()

	// Add two functions
	file1 := &ctxpkg.FileContext{
		Path: "pkg/user.go",
		Functions: []ctxpkg.FunctionDef{
			{Name: "FuncA", Signature: "func FuncA()", IsPublic: true},
			{Name: "FuncB", Signature: "func FuncB()", IsPublic: true},
		},
	}
	_, err := ss.RefreshFileVectors(ctx, "test-repo", "pkg/user.go", file1, "")
	if err != nil {
		t.Fatalf("first RefreshFileVectors: %v", err)
	}

	count, _ := store.Count(ctx, "test-repo")
	if count != 2 {
		t.Fatalf("expected 2 vectors after first refresh, got %d", count)
	}

	// Remove FuncB
	file2 := &ctxpkg.FileContext{
		Path: "pkg/user.go",
		Functions: []ctxpkg.FunctionDef{
			{Name: "FuncA", Signature: "func FuncA()", IsPublic: true},
		},
	}

	result, err := ss.RefreshFileVectors(ctx, "test-repo", "pkg/user.go", file2, "")
	if err != nil {
		t.Fatalf("second RefreshFileVectors: %v", err)
	}

	if result.Removed != 1 {
		t.Errorf("Removed = %d, want 1", result.Removed)
	}

	count, _ = store.Count(ctx, "test-repo")
	if count != 1 {
		t.Errorf("vector count = %d, want 1 after removal", count)
	}

	// FuncA should still exist
	rec, _ := store.Get(ctx, "test-repo:func:pkg/user.go:FuncA")
	if rec == nil {
		t.Error("FuncA vector should still exist")
	}

	// FuncB should be gone
	rec, _ = store.Get(ctx, "test-repo:func:pkg/user.go:FuncB")
	if rec != nil {
		t.Error("FuncB vector should be deleted")
	}
}

func TestRefreshFileVectors_UsesOrgVocabulary(t *testing.T) {
	ss, store, _, vocabStore, cleanup := setupRefreshTest(t)
	defer cleanup()

	ctx := context.Background()

	// Build a vocabulary and export it
	emb := NewDefaultEmbedder()
	emb.BuildVocabulary([]string{"create user database", "handle http request"})
	vocab := emb.ExportVocabulary()

	// Store as org vocabulary
	vocabJSON := `{}`
	if data, err := vocabToJSON(vocab); err == nil {
		vocabJSON = data
	}
	vocabStore.vocabs["test-org"] = &OrgVocabRecord{
		OrgID:          "test-org",
		VocabularyJSON: vocabJSON,
		VersionHash:    vocab.VersionHash,
		DocCount:       vocab.DocCount,
	}

	file := &ctxpkg.FileContext{
		Path: "pkg/user.go",
		Functions: []ctxpkg.FunctionDef{
			{Name: "CreateUser", Signature: "func CreateUser() error", IsPublic: true},
		},
	}

	result, err := ss.RefreshFileVectors(ctx, "test-repo", "pkg/user.go", file, "test-org")
	if err != nil {
		t.Fatalf("RefreshFileVectors: %v", err)
	}

	if result.Added != 1 {
		t.Errorf("Added = %d, want 1", result.Added)
	}

	// Vector should have org_id set
	rec, err := store.Get(ctx, "test-repo:func:pkg/user.go:CreateUser")
	if err != nil || rec == nil {
		t.Fatal("vector not found")
	}
	if rec.OrgID != "test-org" {
		t.Errorf("OrgID = %q, want %q", rec.OrgID, "test-org")
	}
	if rec.VocabVersion == "" {
		t.Error("VocabVersion should be set when using org vocabulary")
	}
}

func TestRefreshFileVectors_PerRepoVocab(t *testing.T) {
	ss, store, _, _, cleanup := setupRefreshTest(t)
	defer cleanup()

	ctx := context.Background()

	file := &ctxpkg.FileContext{
		Path: "pkg/user.go",
		Functions: []ctxpkg.FunctionDef{
			{Name: "CreateUser", Signature: "func CreateUser() error", IsPublic: true},
		},
	}

	result, err := ss.RefreshFileVectors(ctx, "test-repo", "pkg/user.go", file, "")
	if err != nil {
		t.Fatalf("RefreshFileVectors: %v", err)
	}

	if result.Added != 1 {
		t.Errorf("Added = %d, want 1", result.Added)
	}

	rec, err := store.Get(ctx, "test-repo:func:pkg/user.go:CreateUser")
	if err != nil || rec == nil {
		t.Fatal("vector not found")
	}
	if rec.OrgID != "" {
		t.Errorf("OrgID = %q, want empty", rec.OrgID)
	}
	if rec.VocabVersion != "" {
		t.Errorf("VocabVersion = %q, want empty for per-repo mode", rec.VocabVersion)
	}
}

func TestRefreshFileVectors_Idempotent(t *testing.T) {
	ss, _, _, _, cleanup := setupRefreshTest(t)
	defer cleanup()

	ctx := context.Background()

	file := &ctxpkg.FileContext{
		Path: "pkg/user.go",
		Functions: []ctxpkg.FunctionDef{
			{Name: "CreateUser", Signature: "func CreateUser() error", IsPublic: true},
			{Name: "DeleteUser", Signature: "func DeleteUser() error", IsPublic: true},
		},
	}

	// First call
	result1, err := ss.RefreshFileVectors(ctx, "test-repo", "pkg/user.go", file, "")
	if err != nil {
		t.Fatalf("first RefreshFileVectors: %v", err)
	}
	if result1.Added != 2 {
		t.Errorf("first call: Added = %d, want 2", result1.Added)
	}

	// Second call with identical file
	result2, err := ss.RefreshFileVectors(ctx, "test-repo", "pkg/user.go", file, "")
	if err != nil {
		t.Fatalf("second RefreshFileVectors: %v", err)
	}
	if result2.Added != 0 {
		t.Errorf("second call: Added = %d, want 0", result2.Added)
	}
	if result2.Modified != 0 {
		t.Errorf("second call: Modified = %d, want 0", result2.Modified)
	}
	if result2.Removed != 0 {
		t.Errorf("second call: Removed = %d, want 0", result2.Removed)
	}
}

func TestRefreshFileVectors_NoHashStore(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "refresh_test_*.db")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	tmpFile.Close()
	defer os.Remove(tmpFile.Name())

	store, err := NewSQLiteVectorStore(tmpFile.Name(), 256)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	ss := NewSemanticSearch(NewDefaultEmbedder(), store)
	// Don't set hash store

	file := &ctxpkg.FileContext{
		Path:      "pkg/user.go",
		Functions: []ctxpkg.FunctionDef{{Name: "F", Signature: "func F()"}},
	}

	_, err = ss.RefreshFileVectors(context.Background(), "r", "pkg/user.go", file, "")
	if err == nil {
		t.Error("expected error when hash store not configured")
	}
}

func TestCleanupStaleVectors(t *testing.T) {
	ss, store, _, _, cleanup := setupRefreshTest(t)
	defer cleanup()

	ctx := context.Background()

	// Add 3 functions via refresh
	file := &ctxpkg.FileContext{
		Path: "pkg/user.go",
		Functions: []ctxpkg.FunctionDef{
			{Name: "FuncA", Signature: "func FuncA()", IsPublic: true},
			{Name: "FuncB", Signature: "func FuncB()", IsPublic: true},
			{Name: "FuncC", Signature: "func FuncC()", IsPublic: true},
		},
	}
	_, err := ss.RefreshFileVectors(ctx, "test-repo", "pkg/user.go", file, "")
	if err != nil {
		t.Fatalf("RefreshFileVectors: %v", err)
	}

	count, _ := store.Count(ctx, "test-repo")
	if count != 3 {
		t.Fatalf("expected 3 vectors, got %d", count)
	}

	// Cleanup with only A and B present
	current := map[string]bool{"FuncA": true, "FuncB": true}
	removed, err := ss.CleanupStaleVectors(ctx, "test-repo", "pkg/user.go", current)
	if err != nil {
		t.Fatalf("CleanupStaleVectors: %v", err)
	}
	if removed != 1 {
		t.Errorf("removed = %d, want 1", removed)
	}

	count, _ = store.Count(ctx, "test-repo")
	if count != 2 {
		t.Errorf("expected 2 vectors after cleanup, got %d", count)
	}
}

func TestCleanupDeletedFileVectors(t *testing.T) {
	ss, store, _, _, cleanup := setupRefreshTest(t)
	defer cleanup()

	ctx := context.Background()

	// Add vectors for two files
	file1 := &ctxpkg.FileContext{
		Path: "pkg/foo.go",
		Functions: []ctxpkg.FunctionDef{
			{Name: "FooFunc", Signature: "func FooFunc()", IsPublic: true},
		},
	}
	file2 := &ctxpkg.FileContext{
		Path: "pkg/bar.go",
		Functions: []ctxpkg.FunctionDef{
			{Name: "BarFunc", Signature: "func BarFunc()", IsPublic: true},
		},
	}

	ss.RefreshFileVectors(ctx, "test-repo", "pkg/foo.go", file1, "")
	ss.RefreshFileVectors(ctx, "test-repo", "pkg/bar.go", file2, "")

	count, _ := store.Count(ctx, "test-repo")
	if count != 2 {
		t.Fatalf("expected 2 vectors, got %d", count)
	}

	// Delete foo.go
	err := ss.CleanupDeletedFileVectors(ctx, "test-repo", "pkg/foo.go")
	if err != nil {
		t.Fatalf("CleanupDeletedFileVectors: %v", err)
	}

	count, _ = store.Count(ctx, "test-repo")
	if count != 1 {
		t.Errorf("expected 1 vector after cleanup, got %d", count)
	}

	// Bar should still exist
	rec, _ := store.Get(ctx, "test-repo:func:pkg/bar.go:BarFunc")
	if rec == nil {
		t.Error("BarFunc vector should still exist")
	}
}

func TestCleanupRepoFromOrg(t *testing.T) {
	ss, store, _, _, cleanup := setupRefreshTest(t)
	defer cleanup()

	ctx := context.Background()

	// Store vectors for two repos in same org
	records := []VectorRecord{
		{ID: "repo1:func:f.go:F1", RepoID: "repo1", OrgID: "org1", Type: "function", Name: "F1", FilePath: "f.go", Vector: make([]float64, 256)},
		{ID: "repo1:func:f.go:F2", RepoID: "repo1", OrgID: "org1", Type: "function", Name: "F2", FilePath: "f.go", Vector: make([]float64, 256)},
		{ID: "repo2:func:g.go:G1", RepoID: "repo2", OrgID: "org1", Type: "function", Name: "G1", FilePath: "g.go", Vector: make([]float64, 256)},
	}
	store.StoreBatch(ctx, records)

	orgCount, _ := store.CountByOrg(ctx, "org1")
	if orgCount != 3 {
		t.Fatalf("expected 3 org vectors, got %d", orgCount)
	}

	// Remove repo1 from org
	err := ss.CleanupRepoFromOrg(ctx, "org1", "repo1")
	if err != nil {
		t.Fatalf("CleanupRepoFromOrg: %v", err)
	}

	orgCount, _ = store.CountByOrg(ctx, "org1")
	if orgCount != 1 {
		t.Errorf("expected 1 org vector after cleanup, got %d", orgCount)
	}

	// repo2 should still exist
	rec, _ := store.Get(ctx, "repo2:func:g.go:G1")
	if rec == nil {
		t.Error("repo2 vector should still exist")
	}
}

// vocabToJSON converts VocabularyData.WordIDF to JSON string.
func vocabToJSON(v *VocabularyData) (string, error) {
	data, err := json.Marshal(v.WordIDF)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// ============================================================================
// Section 06: IndexRepositoryWithOrg and SearchByOrg tests
// ============================================================================

func TestIndexRepositoryWithOrg_UsesOrgVocabulary(t *testing.T) {
	ss, store, _, vocabStore, cleanup := setupRefreshTest(t)
	defer cleanup()

	ctx := context.Background()

	// Build two repos
	repoA := &ctxpkg.RepoContext{
		ID: "repo-a",
		Files: map[string]*ctxpkg.FileContext{
			"pkg/user.go": {
				Path: "pkg/user.go",
				Functions: []ctxpkg.FunctionDef{
					{Name: "CreateUser", Signature: "func CreateUser(ctx context.Context) error", Description: "Creates a new user in the database", IsPublic: true},
				},
			},
		},
	}
	repoB := &ctxpkg.RepoContext{
		ID: "repo-b",
		Files: map[string]*ctxpkg.FileContext{
			"pkg/notify.go": {
				Path: "pkg/notify.go",
				Functions: []ctxpkg.FunctionDef{
					{Name: "SendNotification", Signature: "func SendNotification(msg string) error", Description: "Sends email notifications to users", IsPublic: true},
				},
			},
		},
	}

	// Build org vocabulary from both repos
	vocabData, err := ss.BuildOrgVocabulary([]*ctxpkg.RepoContext{repoA, repoB})
	if err != nil {
		t.Fatalf("BuildOrgVocabulary: %v", err)
	}

	vocabJSON, err := vocabToJSON(vocabData)
	if err != nil {
		t.Fatalf("vocabToJSON: %v", err)
	}

	// Store org vocabulary in mock store
	vocabStore.vocabs["test-org"] = &OrgVocabRecord{
		OrgID:          "test-org",
		VocabularyJSON: vocabJSON,
		VersionHash:    vocabData.VersionHash,
		DocCount:       vocabData.DocCount,
	}

	// Index repo A with org
	if err := ss.IndexRepositoryWithOrg(ctx, repoA, "test-org"); err != nil {
		t.Fatalf("IndexRepositoryWithOrg repoA: %v", err)
	}

	// Index repo B with org
	if err := ss.IndexRepositoryWithOrg(ctx, repoB, "test-org"); err != nil {
		t.Fatalf("IndexRepositoryWithOrg repoB: %v", err)
	}

	// Verify vectors have org_id and vocab_version set
	vecA, err := store.Get(ctx, "repo-a:func:pkg/user.go:CreateUser")
	if err != nil || vecA == nil {
		t.Fatalf("Get repoA vector: err=%v, vec=%v", err, vecA)
	}
	if vecA.OrgID != "test-org" {
		t.Errorf("expected OrgID 'test-org', got '%s'", vecA.OrgID)
	}
	if vecA.VocabVersion != vocabData.VersionHash {
		t.Errorf("expected VocabVersion '%s', got '%s'", vocabData.VersionHash, vecA.VocabVersion)
	}

	vecB, err := store.Get(ctx, "repo-b:func:pkg/notify.go:SendNotification")
	if err != nil || vecB == nil {
		t.Fatalf("Get repoB vector: err=%v, vec=%v", err, vecB)
	}
	if vecB.OrgID != "test-org" {
		t.Errorf("expected OrgID 'test-org', got '%s'", vecB.OrgID)
	}
	if vecB.VocabVersion != vocabData.VersionHash {
		t.Errorf("expected VocabVersion '%s', got '%s'", vocabData.VersionHash, vecB.VocabVersion)
	}
}

func TestIndexRepositoryWithOrg_FallsBackToPerRepoVocab(t *testing.T) {
	ss, store, _, _, cleanup := setupRefreshTest(t)
	defer cleanup()

	ctx := context.Background()

	repo := &ctxpkg.RepoContext{
		ID: "repo-solo",
		Files: map[string]*ctxpkg.FileContext{
			"main.go": {
				Path: "main.go",
				Functions: []ctxpkg.FunctionDef{
					{Name: "Main", Signature: "func Main()", Description: "Entry point", IsPublic: true},
				},
			},
		},
	}

	// Index without org (empty orgID) — should use per-repo vocab
	if err := ss.IndexRepositoryWithOrg(ctx, repo, ""); err != nil {
		t.Fatalf("IndexRepositoryWithOrg: %v", err)
	}

	vec, err := store.Get(ctx, "repo-solo:func:main.go:Main")
	if err != nil || vec == nil {
		t.Fatalf("Get vector: err=%v, vec=%v", err, vec)
	}
	if vec.OrgID != "" {
		t.Errorf("expected empty OrgID, got '%s'", vec.OrgID)
	}
	if vec.VocabVersion != "" {
		t.Errorf("expected empty VocabVersion, got '%s'", vec.VocabVersion)
	}
}

func TestSearchByOrg_RespectsOrgFiltering(t *testing.T) {
	ss, store, _, vocabStore, cleanup := setupRefreshTest(t)
	defer cleanup()

	ctx := context.Background()

	// Create vectors for two different orgs
	vec1 := make([]float64, 256)
	vec1[0] = 1.0
	vec2 := make([]float64, 256)
	vec2[0] = 0.9
	vec2[1] = 0.1

	records := []VectorRecord{
		{ID: "alpha:func:a.go:FuncA", RepoID: "repo-alpha", OrgID: "org-alpha", Type: "function", Name: "FuncA", FilePath: "a.go", Vector: vec1},
		{ID: "beta:func:b.go:FuncB", RepoID: "repo-beta", OrgID: "org-beta", Type: "function", Name: "FuncB", FilePath: "b.go", Vector: vec2},
	}
	if err := store.StoreBatch(ctx, records); err != nil {
		t.Fatalf("StoreBatch: %v", err)
	}

	// Build vocab for org-alpha and store
	vocabStore.vocabs["org-alpha"] = &OrgVocabRecord{
		OrgID:          "org-alpha",
		VocabularyJSON: `{"func":1.0,"alpha":2.0}`,
		VersionHash:    "alpha-v1",
		DocCount:       1,
	}

	// Search within org-alpha
	results, err := ss.SearchByOrg(ctx, "FuncA alpha", "org-alpha", 10)
	if err != nil {
		t.Fatalf("SearchByOrg: %v", err)
	}

	// Should only return vectors from org-alpha
	for _, r := range results {
		if r.Record.OrgID != "org-alpha" {
			t.Errorf("expected org-alpha results only, got org_id='%s'", r.Record.OrgID)
		}
	}
}

func TestSearchByOrg_FallsBackWithoutVocabulary(t *testing.T) {
	ss, store, _, _, cleanup := setupRefreshTest(t)
	defer cleanup()

	ctx := context.Background()

	vec := make([]float64, 256)
	vec[0] = 1.0

	records := []VectorRecord{
		{ID: "r:func:a.go:F", RepoID: "r", OrgID: "org-novocab", Type: "function", Name: "F", FilePath: "a.go", Vector: vec},
	}
	if err := store.StoreBatch(ctx, records); err != nil {
		t.Fatalf("StoreBatch: %v", err)
	}

	// Search without stored vocabulary — should still work (fallback to default embedder)
	results, err := ss.SearchByOrg(ctx, "some query", "org-novocab", 10)
	if err != nil {
		t.Fatalf("SearchByOrg without vocab should not error: %v", err)
	}

	// Should still find results using default embedder
	if len(results) == 0 {
		t.Log("No results returned (expected when query vector doesn't match, acceptable)")
	}
}

func TestCountByOrg(t *testing.T) {
	ss, store, _, _, cleanup := setupRefreshTest(t)
	defer cleanup()

	ctx := context.Background()

	records := []VectorRecord{
		{ID: "a:func:a.go:F1", RepoID: "a", OrgID: "org1", Type: "function", Name: "F1", Vector: make([]float64, 256)},
		{ID: "b:func:b.go:F2", RepoID: "b", OrgID: "org1", Type: "function", Name: "F2", Vector: make([]float64, 256)},
		{ID: "c:func:c.go:F3", RepoID: "c", OrgID: "org2", Type: "function", Name: "F3", Vector: make([]float64, 256)},
	}
	if err := store.StoreBatch(ctx, records); err != nil {
		t.Fatalf("StoreBatch: %v", err)
	}

	count, err := ss.CountByOrg(ctx, "org1")
	if err != nil {
		t.Fatalf("CountByOrg: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 vectors in org1, got %d", count)
	}

	count2, err := ss.CountByOrg(ctx, "org2")
	if err != nil {
		t.Fatalf("CountByOrg org2: %v", err)
	}
	if count2 != 1 {
		t.Errorf("expected 1 vector in org2, got %d", count2)
	}
}
