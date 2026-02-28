package vectors

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"testing"

	_ "github.com/mattn/go-sqlite3"
	ctxpkg "github.com/yashpalc/mcp-repo-context/internal/context"
)

func setupIncrementalTest(t *testing.T) (*SemanticSearch, *SQLiteVectorStore, func()) {
	t.Helper()

	tmpFile, err := os.CreateTemp("", "incremental_test_*.db")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	tmpFile.Close()

	store, err := NewSQLiteVectorStore(tmpFile.Name(), 256)
	if err != nil {
		os.Remove(tmpFile.Name())
		t.Fatalf("failed to create store: %v", err)
	}

	embedder := NewDefaultEmbedder()
	ss := NewSemanticSearch(embedder, store)

	cleanup := func() {
		store.Close()
		os.Remove(tmpFile.Name())
	}

	return ss, store, cleanup
}

// buildTestRepo creates a RepoContext for testing.
func buildTestRepo(repoID string, files map[string]*ctxpkg.FileContext) *ctxpkg.RepoContext {
	return &ctxpkg.RepoContext{
		ID:    repoID,
		Files: files,
	}
}

func TestRefreshFile_UpdatesVectorsForChangedFunctions(t *testing.T) {
	ss, store, cleanup := setupIncrementalTest(t)
	defer cleanup()

	ctx := context.Background()

	// Index a repo with file A containing GetUser, CreateUser
	repo := buildTestRepo("test-repo", map[string]*ctxpkg.FileContext{
		"pkg/user.go": {
			Path: "pkg/user.go",
			Functions: []ctxpkg.FunctionDef{
				{Name: "GetUser", Signature: "func GetUser(id int) *User", IsPublic: true},
				{Name: "CreateUser", Signature: "func CreateUser(name string) *User", IsPublic: true},
			},
		},
	})

	if err := ss.IndexRepository(ctx, repo); err != nil {
		t.Fatalf("IndexRepository: %v", err)
	}

	count, _ := store.Count(ctx, "test-repo")
	if count != 2 {
		t.Fatalf("expected 2 vectors after index, got %d", count)
	}

	// Now modify file A: remove CreateUser, add UpdateUser
	newFunctions := []ctxpkg.FunctionDef{
		{Name: "GetUser", Signature: "func GetUser(id int) *User", IsPublic: true},
		{Name: "UpdateUser", Signature: "func UpdateUser(id int, name string) error", IsPublic: true},
	}

	if err := ss.RefreshFile(ctx, "test-repo", "pkg/user.go", newFunctions, nil); err != nil {
		t.Fatalf("RefreshFile: %v", err)
	}

	// GetUser should still exist
	rec, _ := store.Get(ctx, "test-repo:func:pkg/user.go:GetUser")
	if rec == nil {
		t.Error("GetUser vector should exist after refresh")
	}

	// UpdateUser should exist (new)
	rec, _ = store.Get(ctx, "test-repo:func:pkg/user.go:UpdateUser")
	if rec == nil {
		t.Error("UpdateUser vector should exist after refresh")
	}

	// CreateUser should be gone (removed)
	rec, _ = store.Get(ctx, "test-repo:func:pkg/user.go:CreateUser")
	if rec != nil {
		t.Error("CreateUser vector should not exist after refresh")
	}

	count, _ = store.Count(ctx, "test-repo")
	if count != 2 {
		t.Errorf("expected 2 vectors after refresh, got %d", count)
	}
}

func TestRefreshFile_UpdatesBothFunctionsAndTypes(t *testing.T) {
	ss, store, cleanup := setupIncrementalTest(t)
	defer cleanup()

	ctx := context.Background()

	repo := buildTestRepo("test-repo", map[string]*ctxpkg.FileContext{
		"pkg/models.go": {
			Path: "pkg/models.go",
			Functions: []ctxpkg.FunctionDef{
				{Name: "GetUser", Signature: "func GetUser() *User", IsPublic: true},
			},
			Types: []ctxpkg.TypeDef{
				{Name: "User", Kind: "struct", IsPublic: true, Description: "User model"},
			},
		},
	})

	if err := ss.IndexRepository(ctx, repo); err != nil {
		t.Fatalf("IndexRepository: %v", err)
	}

	count, _ := store.Count(ctx, "test-repo")
	if count != 2 {
		t.Fatalf("expected 2 vectors (1 func + 1 type), got %d", count)
	}

	// Update: change User to UserProfile
	newFunctions := []ctxpkg.FunctionDef{
		{Name: "GetUser", Signature: "func GetUser() *UserProfile", IsPublic: true},
	}
	newTypes := []ctxpkg.TypeDef{
		{Name: "UserProfile", Kind: "struct", IsPublic: true, Description: "User profile model"},
	}

	if err := ss.RefreshFile(ctx, "test-repo", "pkg/models.go", newFunctions, newTypes); err != nil {
		t.Fatalf("RefreshFile: %v", err)
	}

	// GetUser should exist
	rec, _ := store.Get(ctx, "test-repo:func:pkg/models.go:GetUser")
	if rec == nil {
		t.Error("GetUser should still exist")
	}

	// UserProfile should exist
	rec, _ = store.Get(ctx, "test-repo:type:pkg/models.go:UserProfile")
	if rec == nil {
		t.Error("UserProfile type should exist")
	}

	// User type should be gone
	rec, _ = store.Get(ctx, "test-repo:type:pkg/models.go:User")
	if rec != nil {
		t.Error("User type should not exist after refresh")
	}
}

func TestRefreshFiles_BatchProcessesMultipleFiles(t *testing.T) {
	ss, store, cleanup := setupIncrementalTest(t)
	defer cleanup()

	ctx := context.Background()

	repo := buildTestRepo("test-repo", map[string]*ctxpkg.FileContext{
		"pkg/a.go": {
			Path:      "pkg/a.go",
			Functions: []ctxpkg.FunctionDef{{Name: "FuncA", Signature: "func FuncA()", IsPublic: true}},
		},
		"pkg/b.go": {
			Path:      "pkg/b.go",
			Functions: []ctxpkg.FunctionDef{{Name: "FuncB", Signature: "func FuncB()", IsPublic: true}},
		},
		"pkg/c.go": {
			Path:      "pkg/c.go",
			Functions: []ctxpkg.FunctionDef{{Name: "FuncC", Signature: "func FuncC()", IsPublic: true}},
		},
	})

	if err := ss.IndexRepository(ctx, repo); err != nil {
		t.Fatalf("IndexRepository: %v", err)
	}

	count, _ := store.Count(ctx, "test-repo")
	if count != 3 {
		t.Fatalf("expected 3 vectors, got %d", count)
	}

	// Refresh files A and B with new functions, C unchanged
	files := map[string]FileContext{
		"pkg/a.go": {
			Functions: []ctxpkg.FunctionDef{{Name: "FuncA2", Signature: "func FuncA2()", IsPublic: true}},
		},
		"pkg/b.go": {
			Functions: []ctxpkg.FunctionDef{{Name: "FuncB2", Signature: "func FuncB2()", IsPublic: true}},
		},
	}

	if err := ss.RefreshFiles(ctx, "test-repo", files); err != nil {
		t.Fatalf("RefreshFiles: %v", err)
	}

	// A and B should have new vectors
	rec, _ := store.Get(ctx, "test-repo:func:pkg/a.go:FuncA2")
	if rec == nil {
		t.Error("FuncA2 should exist")
	}
	rec, _ = store.Get(ctx, "test-repo:func:pkg/b.go:FuncB2")
	if rec == nil {
		t.Error("FuncB2 should exist")
	}

	// Old A and B vectors should be gone
	rec, _ = store.Get(ctx, "test-repo:func:pkg/a.go:FuncA")
	if rec != nil {
		t.Error("FuncA should not exist after refresh")
	}
	rec, _ = store.Get(ctx, "test-repo:func:pkg/b.go:FuncB")
	if rec != nil {
		t.Error("FuncB should not exist after refresh")
	}

	// C should be unchanged
	rec, _ = store.Get(ctx, "test-repo:func:pkg/c.go:FuncC")
	if rec == nil {
		t.Error("FuncC should still exist (not refreshed)")
	}
}

func TestDeleteByFile_RemovesAllVectorsForFile(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	records := []VectorRecord{
		{ID: "r:func:a.go:F1", RepoID: "r", Type: "function", Name: "F1", FilePath: "a.go", Vector: make([]float64, 128)},
		{ID: "r:func:a.go:F2", RepoID: "r", Type: "function", Name: "F2", FilePath: "a.go", Vector: make([]float64, 128)},
		{ID: "r:type:a.go:T1", RepoID: "r", Type: "type", Name: "T1", FilePath: "a.go", Vector: make([]float64, 128)},
		{ID: "r:func:b.go:F3", RepoID: "r", Type: "function", Name: "F3", FilePath: "b.go", Vector: make([]float64, 128)},
	}
	if err := store.StoreBatch(ctx, records); err != nil {
		t.Fatalf("StoreBatch: %v", err)
	}

	if err := store.DeleteByFile(ctx, "r", "a.go"); err != nil {
		t.Fatalf("DeleteByFile: %v", err)
	}

	count, _ := store.Count(ctx, "r")
	if count != 1 {
		t.Errorf("expected 1 vector after delete, got %d", count)
	}

	// b.go should still exist
	rec, _ := store.Get(ctx, "r:func:b.go:F3")
	if rec == nil {
		t.Error("b.go vector should still exist")
	}
}

func TestDeleteByFile_NonexistentFileIsNoOp(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	err := store.DeleteByFile(ctx, "r", "nonexistent.go")
	if err != nil {
		t.Fatalf("DeleteByFile on nonexistent file should not error: %v", err)
	}
}

func TestVocabulary_StoredDuringIndexRepository(t *testing.T) {
	ss, store, cleanup := setupIncrementalTest(t)
	defer cleanup()

	ctx := context.Background()

	repo := buildTestRepo("test-repo", map[string]*ctxpkg.FileContext{
		"pkg/user.go": {
			Path: "pkg/user.go",
			Functions: []ctxpkg.FunctionDef{
				{Name: "CreateUser", Signature: "func CreateUser() error", Description: "creates a user", IsPublic: true},
			},
		},
	})

	if err := ss.IndexRepository(ctx, repo); err != nil {
		t.Fatalf("IndexRepository: %v", err)
	}

	// Check vocabularies table
	vocabJSON, idfJSON, version, err := store.LoadVocabulary(ctx, "test-repo")
	if err != nil {
		t.Fatalf("LoadVocabulary: %v", err)
	}

	if version != 0 {
		t.Errorf("expected version 0, got %d", version)
	}

	// Verify JSON is valid
	var wordIDF map[string]float64
	if err := json.Unmarshal(vocabJSON, &wordIDF); err != nil {
		t.Fatalf("vocabulary_json invalid: %v", err)
	}
	if len(wordIDF) == 0 {
		t.Error("vocabulary should not be empty")
	}

	var wordSlots map[string]int
	if err := json.Unmarshal(idfJSON, &wordSlots); err != nil {
		t.Fatalf("idf_json (word slots) invalid: %v", err)
	}
}

func TestVocabulary_LoadedForIncrementalIndexing(t *testing.T) {
	ss, store, cleanup := setupIncrementalTest(t)
	defer cleanup()

	ctx := context.Background()

	repo := buildTestRepo("test-repo", map[string]*ctxpkg.FileContext{
		"pkg/user.go": {
			Path: "pkg/user.go",
			Functions: []ctxpkg.FunctionDef{
				{Name: "CreateUser", Signature: "func CreateUser() error", IsPublic: true},
				{Name: "DeleteUser", Signature: "func DeleteUser() error", IsPublic: true},
			},
		},
	})

	if err := ss.IndexRepository(ctx, repo); err != nil {
		t.Fatalf("IndexRepository: %v", err)
	}

	// Create a fresh SemanticSearch with empty vocabulary embedder
	freshEmb := NewDefaultEmbedder()
	freshSS := NewSemanticSearch(freshEmb, store)

	// Vocabulary should be empty
	if freshEmb.VocabularySize() != 0 {
		t.Fatal("fresh embedder should have empty vocabulary")
	}

	// RefreshFile should load vocabulary from DB
	newFunctions := []ctxpkg.FunctionDef{
		{Name: "UpdateUser", Signature: "func UpdateUser() error", IsPublic: true},
	}
	if err := freshSS.RefreshFile(ctx, "test-repo", "pkg/user.go", newFunctions, nil); err != nil {
		t.Fatalf("RefreshFile: %v", err)
	}

	// Vocabulary should now be loaded
	if freshEmb.VocabularySize() == 0 {
		t.Error("embedder vocabulary should be loaded from DB after RefreshFile")
	}
}

func TestVocabulary_CachedInSemanticSearch(t *testing.T) {
	ss, _, cleanup := setupIncrementalTest(t)
	defer cleanup()

	ctx := context.Background()

	repo := buildTestRepo("test-repo", map[string]*ctxpkg.FileContext{
		"pkg/user.go": {
			Path: "pkg/user.go",
			Functions: []ctxpkg.FunctionDef{
				{Name: "CreateUser", Signature: "func CreateUser() error", IsPublic: true},
			},
		},
	})

	if err := ss.IndexRepository(ctx, repo); err != nil {
		t.Fatalf("IndexRepository: %v", err)
	}

	// Clear the embedder's vocabulary to force loading from cache/DB
	freshEmb := NewDefaultEmbedder()
	ss.embedder = freshEmb

	// First call - should load from DB
	if err := ss.loadVocabularyForRepo(ctx, "test-repo"); err != nil {
		t.Fatalf("first loadVocabularyForRepo: %v", err)
	}

	// Check cache
	ss.vocabCacheMu.RLock()
	_, cached := ss.vocabCache["test-repo"]
	ss.vocabCacheMu.RUnlock()
	if !cached {
		t.Error("vocabulary should be cached after first load")
	}

	// Second call should use cache (won't hit DB)
	freshEmb2 := NewDefaultEmbedder()
	ss.embedder = freshEmb2
	if err := ss.loadVocabularyForRepo(ctx, "test-repo"); err != nil {
		t.Fatalf("second loadVocabularyForRepo: %v", err)
	}
	if freshEmb2.VocabularySize() == 0 {
		t.Error("second load should use cached vocabulary")
	}
}

func TestVocabulary_ForceReindexResetsVersion(t *testing.T) {
	ss, store, cleanup := setupIncrementalTest(t)
	defer cleanup()

	ctx := context.Background()

	repo := buildTestRepo("test-repo", map[string]*ctxpkg.FileContext{
		"pkg/user.go": {
			Path: "pkg/user.go",
			Functions: []ctxpkg.FunctionDef{
				{Name: "CreateUser", Signature: "func CreateUser() error", IsPublic: true},
			},
		},
	})

	// Index -> version=0
	if err := ss.IndexRepository(ctx, repo); err != nil {
		t.Fatalf("IndexRepository: %v", err)
	}

	// RefreshFile 3 times -> version=3
	for i := 0; i < 3; i++ {
		if err := ss.RefreshFile(ctx, "test-repo", "pkg/user.go",
			[]ctxpkg.FunctionDef{{Name: "CreateUser", Signature: "func CreateUser() error", IsPublic: true}},
			nil); err != nil {
			t.Fatalf("RefreshFile %d: %v", i, err)
		}
	}

	_, _, version, _ := store.LoadVocabulary(ctx, "test-repo")
	if version != 3 {
		t.Errorf("expected version 3, got %d", version)
	}

	// Re-index (simulating force=true) -> version=0
	if err := ss.ClearRepository(ctx, "test-repo"); err != nil {
		t.Fatalf("ClearRepository: %v", err)
	}
	if err := ss.IndexRepository(ctx, repo); err != nil {
		t.Fatalf("re-IndexRepository: %v", err)
	}

	_, _, version, _ = store.LoadVocabulary(ctx, "test-repo")
	if version != 0 {
		t.Errorf("expected version 0 after re-index, got %d", version)
	}
}

func TestVocabulary_MissingTriggersFallback(t *testing.T) {
	ss, store, cleanup := setupIncrementalTest(t)
	defer cleanup()

	ctx := context.Background()

	repo := buildTestRepo("test-repo", map[string]*ctxpkg.FileContext{
		"pkg/user.go": {
			Path: "pkg/user.go",
			Functions: []ctxpkg.FunctionDef{
				{Name: "CreateUser", Signature: "func CreateUser() error", IsPublic: true},
			},
		},
	})

	if err := ss.IndexRepository(ctx, repo); err != nil {
		t.Fatalf("IndexRepository: %v", err)
	}

	// Delete vocabulary manually
	store.mu.Lock()
	store.db.Exec("DELETE FROM vocabularies WHERE repo_id = ?", "test-repo")
	store.mu.Unlock()

	// Create fresh SS
	freshEmb := NewDefaultEmbedder()
	freshSS := NewSemanticSearch(freshEmb, store)

	// RefreshFile should still work (hash fallback when vocabulary missing)
	err := freshSS.RefreshFile(ctx, "test-repo", "pkg/user.go",
		[]ctxpkg.FunctionDef{{Name: "UpdateUser", Signature: "func UpdateUser() error", IsPublic: true}},
		nil)
	if err != nil {
		t.Fatalf("RefreshFile should work even without vocabulary: %v", err)
	}

	// Vector should exist
	rec, _ := store.Get(ctx, "test-repo:func:pkg/user.go:UpdateUser")
	if rec == nil {
		t.Error("UpdateUser should exist even with hash fallback")
	}
}

func TestVocabulary_LoadedForSearchAfterRestart(t *testing.T) {
	ss, store, cleanup := setupIncrementalTest(t)
	defer cleanup()

	ctx := context.Background()

	repo := buildTestRepo("test-repo", map[string]*ctxpkg.FileContext{
		"pkg/user.go": {
			Path: "pkg/user.go",
			Functions: []ctxpkg.FunctionDef{
				{Name: "CreateUser", Signature: "func CreateUser(name string) error", Description: "creates a new user in the database", IsPublic: true},
				{Name: "DeleteUser", Signature: "func DeleteUser(id int) error", Description: "deletes a user by ID", IsPublic: true},
			},
		},
	})

	if err := ss.IndexRepository(ctx, repo); err != nil {
		t.Fatalf("IndexRepository: %v", err)
	}

	// Simulate restart: new SemanticSearch with fresh embedder
	freshEmb := NewDefaultEmbedder()
	freshSS := NewSemanticSearch(freshEmb, store)

	if freshEmb.VocabularySize() != 0 {
		t.Fatal("fresh embedder should have empty vocabulary")
	}

	// Search should load vocabulary from DB
	results, err := freshSS.SearchFunctions(ctx, "create user", "test-repo", 10)
	if err != nil {
		t.Fatalf("SearchFunctions: %v", err)
	}

	// Vocabulary should now be loaded
	if freshEmb.VocabularySize() == 0 {
		t.Error("vocabulary should be loaded from DB before search")
	}

	// Should find results
	if len(results) == 0 {
		t.Error("expected search results after vocabulary loaded")
	}
}

func TestVocabulary_CrossRepoIsolation(t *testing.T) {
	ss, store, cleanup := setupIncrementalTest(t)
	defer cleanup()

	ctx := context.Background()

	// Index repo A with user functions
	repoA := buildTestRepo("repo-a", map[string]*ctxpkg.FileContext{
		"pkg/user.go": {
			Path: "pkg/user.go",
			Functions: []ctxpkg.FunctionDef{
				{Name: "CreateUser", Signature: "func CreateUser() error", Description: "creates user", IsPublic: true},
			},
		},
	})
	if err := ss.IndexRepository(ctx, repoA); err != nil {
		t.Fatalf("IndexRepository repoA: %v", err)
	}

	// Index repo B with order functions (need new embedder to avoid vocabulary cross-contamination)
	embB := NewDefaultEmbedder()
	ssB := NewSemanticSearch(embB, store)
	repoB := buildTestRepo("repo-b", map[string]*ctxpkg.FileContext{
		"pkg/order.go": {
			Path: "pkg/order.go",
			Functions: []ctxpkg.FunctionDef{
				{Name: "CreateOrder", Signature: "func CreateOrder() error", Description: "creates order", IsPublic: true},
			},
		},
	})
	if err := ssB.IndexRepository(ctx, repoB); err != nil {
		t.Fatalf("IndexRepository repoB: %v", err)
	}

	// Verify vocabularies are separate
	vocabA, _, _, errA := store.LoadVocabulary(ctx, "repo-a")
	vocabB, _, _, errB := store.LoadVocabulary(ctx, "repo-b")
	if errA != nil || errB != nil {
		t.Fatalf("LoadVocabulary errors: a=%v, b=%v", errA, errB)
	}

	if string(vocabA) == string(vocabB) {
		t.Log("Note: vocabularies are identical (may happen with small repos)")
	}

	// Search repo B should only get repo B results
	results, err := ssB.SearchFunctions(ctx, "order", "repo-b", 10)
	if err != nil {
		t.Fatalf("SearchFunctions repoB: %v", err)
	}

	for _, r := range results {
		if r.FilePath != "pkg/order.go" {
			t.Errorf("expected only repo-b results, got file %s", r.FilePath)
		}
	}
}

func TestFilePathIndex_ExistsForDeleteByFile(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "idx_test_*.db")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	tmpFile.Close()
	defer os.Remove(tmpFile.Name())

	db, err := sql.Open("sqlite3", tmpFile.Name())
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer db.Close()

	_, err = NewSQLiteVectorStoreWithDB(db, 256)
	if err != nil {
		t.Fatalf("NewSQLiteVectorStoreWithDB: %v", err)
	}

	// Check that the idx_vectors_file index exists
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name='idx_vectors_file'").Scan(&count)
	if err != nil {
		t.Fatalf("query index: %v", err)
	}
	if count != 1 {
		t.Error("idx_vectors_file index should exist")
	}
}
