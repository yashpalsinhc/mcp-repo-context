package integration

import (
	"context"
	"encoding/json"
	"math"
	"os"
	"testing"

	_ "github.com/mattn/go-sqlite3"
	ctxpkg "github.com/yashpalc/mcp-repo-context/internal/context"
	"github.com/yashpalc/mcp-repo-context/internal/storage"
	"github.com/yashpalc/mcp-repo-context/internal/vectors"
)

// setupTestDBs creates temporary main storage DB and vector store DB.
func setupSemanticTestDBs(t *testing.T) (*storage.SQLiteStore, *vectors.SQLiteVectorStore, func()) {
	t.Helper()

	mainDB, err := os.CreateTemp("", "semantic_main_*.db")
	if err != nil {
		t.Fatalf("create temp main db: %v", err)
	}
	mainDB.Close()

	vectorDB, err := os.CreateTemp("", "semantic_vec_*.db")
	if err != nil {
		os.Remove(mainDB.Name())
		t.Fatalf("create temp vector db: %v", err)
	}
	vectorDB.Close()

	mainStore, err := storage.NewSQLiteStore(mainDB.Name())
	if err != nil {
		os.Remove(mainDB.Name())
		os.Remove(vectorDB.Name())
		t.Fatalf("create main store: %v", err)
	}

	vecStore, err := vectors.NewSQLiteVectorStore(vectorDB.Name(), 256)
	if err != nil {
		mainStore.Close()
		os.Remove(mainDB.Name())
		os.Remove(vectorDB.Name())
		t.Fatalf("create vector store: %v", err)
	}

	cleanup := func() {
		vecStore.Close()
		mainStore.Close()
		os.Remove(mainDB.Name())
		os.Remove(vectorDB.Name())
	}

	return mainStore, vecStore, cleanup
}

func makeFunction(name, description string, sideEffects []string) ctxpkg.FunctionDef {
	return ctxpkg.FunctionDef{
		Name:        name,
		Signature:   "func " + name + "()",
		Description: description,
		Behavior: &ctxpkg.FunctionBehavior{
			Summary: description,
			Steps:   sideEffects,
		},
		SideEffects: sideEffects,
		IsPublic:    true,
		LineStart:   1,
		LineEnd:     10,
	}
}

func makeRepo(repoID string, files map[string][]ctxpkg.FunctionDef) *ctxpkg.RepoContext {
	rc := &ctxpkg.RepoContext{
		ID:    repoID,
		Files: make(map[string]*ctxpkg.FileContext),
	}
	for path, fns := range files {
		rc.Files[path] = &ctxpkg.FileContext{Path: path, Functions: fns}
	}
	return rc
}

// TestFullPipeline_IndexOrgThenSearch verifies end-to-end: build org vocab,
// index repos, search across repos.
func TestFullPipeline_IndexOrgThenSearch(t *testing.T) {
	_, vecStore, cleanup := setupSemanticTestDBs(t)
	defer cleanup()

	ctx := context.Background()

	embedder := vectors.NewDefaultEmbedder()
	ss := vectors.NewSemanticSearch(embedder, vecStore)

	repoA := makeRepo("repo-a", map[string][]ctxpkg.FunctionDef{
		"pkg/user.go": {
			makeFunction("CreateUser", "Creates a new user in the database with validation", []string{"db_query"}),
			makeFunction("ValidateEmail", "Validates email format using regex", nil),
		},
	})
	repoB := makeRepo("repo-b", map[string][]ctxpkg.FunctionDef{
		"pkg/notify.go": {
			makeFunction("SendNotification", "Sends email notifications to users via SMTP", []string{"http_call"}),
			makeFunction("FormatMessage", "Formats notification message templates", nil),
		},
	})

	// Build org vocabulary
	vocabData, err := ss.BuildOrgVocabulary([]*ctxpkg.RepoContext{repoA, repoB})
	if err != nil {
		t.Fatalf("BuildOrgVocabulary: %v", err)
	}
	if vocabData.DocCount == 0 {
		t.Fatal("expected non-zero doc count")
	}

	// Store vocab in mock vocabStore for SemanticSearch
	vocabJSON, _ := json.Marshal(vocabData.WordIDF)
	mockVocab := &mockVocabStore{
		vocabs: map[string]*vectors.OrgVocabRecord{
			"test-org": {
				OrgID:          "test-org",
				VocabularyJSON: string(vocabJSON),
				VersionHash:    vocabData.VersionHash,
				DocCount:       vocabData.DocCount,
			},
		},
	}
	ss.SetVocabStore(mockVocab)

	// Index repos with org
	if err := ss.IndexRepositoryWithOrg(ctx, repoA, "test-org"); err != nil {
		t.Fatalf("IndexRepositoryWithOrg repoA: %v", err)
	}
	if err := ss.IndexRepositoryWithOrg(ctx, repoB, "test-org"); err != nil {
		t.Fatalf("IndexRepositoryWithOrg repoB: %v", err)
	}

	// Verify total count
	count, err := ss.CountByOrg(ctx, "test-org")
	if err != nil {
		t.Fatalf("CountByOrg: %v", err)
	}
	if count != 4 {
		t.Errorf("expected 4 vectors, got %d", count)
	}

	// Search for user-related terms
	results, err := ss.SearchByOrg(ctx, "user creation database", "test-org", 10)
	if err != nil {
		t.Fatalf("SearchByOrg: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected search results for 'user creation database'")
	}
	// First result should be CreateUser
	if results[0].Record.Name != "CreateUser" {
		t.Errorf("expected first result to be CreateUser, got %s", results[0].Record.Name)
	}

	// Search for notification-related terms
	results2, err := ss.SearchByOrg(ctx, "notification email SMTP", "test-org", 10)
	if err != nil {
		t.Fatalf("SearchByOrg: %v", err)
	}
	if len(results2) == 0 {
		t.Fatal("expected search results for 'notification email'")
	}
	if results2[0].Record.Name != "SendNotification" {
		t.Errorf("expected first result to be SendNotification, got %s", results2[0].Record.Name)
	}
}

// TestIncrementalUpdate_ModifyFunction verifies RefreshFileVectors detects modified function.
func TestIncrementalUpdate_ModifyFunction(t *testing.T) {
	mainStore, vecStore, cleanup := setupSemanticTestDBs(t)
	defer cleanup()

	ctx := context.Background()

	embedder := vectors.NewDefaultEmbedder()
	ss := vectors.NewSemanticSearch(embedder, vecStore)

	// Create hash store adapter from main storage
	hashStore := &storageHashAdapter{store: mainStore}
	ss.SetHashStore(hashStore)

	repo := makeRepo("repo-test", map[string][]ctxpkg.FunctionDef{
		"pkg/api.go": {
			makeFunction("FuncA", "Function A does something", nil),
			makeFunction("FuncB", "Function B does something else", nil),
			makeFunction("FuncC", "Function C is a third function", nil),
		},
	})

	// Initial index
	if err := ss.IndexRepository(ctx, repo); err != nil {
		t.Fatalf("IndexRepository: %v", err)
	}

	// Store initial hashes
	file := repo.Files["pkg/api.go"]
	refreshResult, err := ss.RefreshFileVectors(ctx, "repo-test", "pkg/api.go", file, "")
	if err != nil {
		t.Fatalf("RefreshFileVectors initial: %v", err)
	}
	// First refresh should add 3 (since no hashes existed before)
	if refreshResult.Added != 3 {
		t.Logf("initial refresh: added=%d, modified=%d, removed=%d", refreshResult.Added, refreshResult.Modified, refreshResult.Removed)
	}

	// Modify FuncB
	modifiedFile := &ctxpkg.FileContext{
		Path: "pkg/api.go",
		Functions: []ctxpkg.FunctionDef{
			makeFunction("FuncA", "Function A does something", nil),
			makeFunction("FuncB", "Function B now does COMPLETELY DIFFERENT THINGS with database queries", []string{"db_query"}),
			makeFunction("FuncC", "Function C is a third function", nil),
		},
	}

	refreshResult, err = ss.RefreshFileVectors(ctx, "repo-test", "pkg/api.go", modifiedFile, "")
	if err != nil {
		t.Fatalf("RefreshFileVectors modified: %v", err)
	}

	if refreshResult.Modified != 1 {
		t.Errorf("expected 1 modified, got %d", refreshResult.Modified)
	}
	if refreshResult.Added != 0 {
		t.Errorf("expected 0 added, got %d", refreshResult.Added)
	}
	if refreshResult.Removed != 0 {
		t.Errorf("expected 0 removed, got %d", refreshResult.Removed)
	}

	// Verify total count unchanged
	count, _ := vecStore.Count(ctx, "repo-test")
	if count != 3 {
		t.Errorf("expected 3 vectors, got %d", count)
	}
}

// TestIncrementalUpdate_DeleteFunction verifies RefreshFileVectors removes deleted function.
func TestIncrementalUpdate_DeleteFunction(t *testing.T) {
	mainStore, vecStore, cleanup := setupSemanticTestDBs(t)
	defer cleanup()

	ctx := context.Background()

	embedder := vectors.NewDefaultEmbedder()
	ss := vectors.NewSemanticSearch(embedder, vecStore)

	hashStore := &storageHashAdapter{store: mainStore}
	ss.SetHashStore(hashStore)

	repo := makeRepo("repo-del", map[string][]ctxpkg.FunctionDef{
		"pkg/api.go": {
			makeFunction("FuncA", "Function A", nil),
			makeFunction("FuncB", "Function B", nil),
			makeFunction("FuncC", "Function C", nil),
		},
	})

	if err := ss.IndexRepository(ctx, repo); err != nil {
		t.Fatalf("IndexRepository: %v", err)
	}

	// Initial hash setup
	file := repo.Files["pkg/api.go"]
	ss.RefreshFileVectors(ctx, "repo-del", "pkg/api.go", file, "")

	// Delete FuncC
	reducedFile := &ctxpkg.FileContext{
		Path: "pkg/api.go",
		Functions: []ctxpkg.FunctionDef{
			makeFunction("FuncA", "Function A", nil),
			makeFunction("FuncB", "Function B", nil),
		},
	}

	result, err := ss.RefreshFileVectors(ctx, "repo-del", "pkg/api.go", reducedFile, "")
	if err != nil {
		t.Fatalf("RefreshFileVectors: %v", err)
	}

	if result.Removed != 1 {
		t.Errorf("expected 1 removed, got %d", result.Removed)
	}

	// Verify FuncC vector is gone
	vec, _ := vecStore.Get(ctx, "repo-del:func:pkg/api.go:FuncC")
	if vec != nil {
		t.Error("expected FuncC vector to be deleted")
	}

	// FuncA and FuncB should still exist
	vecA, _ := vecStore.Get(ctx, "repo-del:func:pkg/api.go:FuncA")
	vecB, _ := vecStore.Get(ctx, "repo-del:func:pkg/api.go:FuncB")
	if vecA == nil {
		t.Error("expected FuncA vector to exist")
	}
	if vecB == nil {
		t.Error("expected FuncB vector to exist")
	}
}

// TestVocabularyConsistency_SameTextSameEmbedding verifies identical function
// text produces identical embeddings when using the same org vocabulary.
func TestVocabularyConsistency_SameTextSameEmbedding(t *testing.T) {
	_, vecStore, cleanup := setupSemanticTestDBs(t)
	defer cleanup()

	ctx := context.Background()

	embedder := vectors.NewDefaultEmbedder()
	ss := vectors.NewSemanticSearch(embedder, vecStore)

	// Both repos have identical function
	sharedFunc := makeFunction("ParseJSON", "Parses JSON input and returns structured data", nil)

	repoA := makeRepo("repo-a", map[string][]ctxpkg.FunctionDef{
		"pkg/parser.go": {sharedFunc},
	})
	repoB := makeRepo("repo-b", map[string][]ctxpkg.FunctionDef{
		"pkg/parser.go": {sharedFunc},
	})

	// Build shared vocabulary
	vocabData, err := ss.BuildOrgVocabulary([]*ctxpkg.RepoContext{repoA, repoB})
	if err != nil {
		t.Fatalf("BuildOrgVocabulary: %v", err)
	}

	mockVocab := &mockVocabStore{
		vocabs: map[string]*vectors.OrgVocabRecord{
			"test-org": {
				OrgID:          "test-org",
				VocabularyJSON: marshalVocab(t, vocabData),
				VersionHash:    vocabData.VersionHash,
				DocCount:       vocabData.DocCount,
			},
		},
	}
	ss.SetVocabStore(mockVocab)

	if err := ss.IndexRepositoryWithOrg(ctx, repoA, "test-org"); err != nil {
		t.Fatalf("index repoA: %v", err)
	}
	if err := ss.IndexRepositoryWithOrg(ctx, repoB, "test-org"); err != nil {
		t.Fatalf("index repoB: %v", err)
	}

	// Retrieve both vectors
	vecA, _ := vecStore.Get(ctx, "repo-a:func:pkg/parser.go:ParseJSON")
	vecB, _ := vecStore.Get(ctx, "repo-b:func:pkg/parser.go:ParseJSON")

	if vecA == nil || vecB == nil {
		t.Fatal("expected both vectors to exist")
	}

	// Compute cosine similarity
	sim := cosineSimilarity(vecA.Vector, vecB.Vector)
	if math.Abs(sim-1.0) > 1e-6 {
		t.Errorf("expected cosine similarity ~1.0, got %f", sim)
	}
}

// TestVocabularyVersionTracking verifies vectors store vocab_version.
func TestVocabularyVersionTracking(t *testing.T) {
	_, vecStore, cleanup := setupSemanticTestDBs(t)
	defer cleanup()

	ctx := context.Background()

	embedder := vectors.NewDefaultEmbedder()
	ss := vectors.NewSemanticSearch(embedder, vecStore)

	repoA := makeRepo("repo-a", map[string][]ctxpkg.FunctionDef{
		"a.go": {makeFunction("FuncA", "Function A handles authentication", []string{"db_query"})},
	})
	repoB := makeRepo("repo-b", map[string][]ctxpkg.FunctionDef{
		"b.go": {
			makeFunction("FuncB", "Function B processes payments with Stripe integration", []string{"http_call"}),
			makeFunction("FuncC", "Function C validates credit card numbers", []string{"logging"}),
			makeFunction("FuncD", "Function D generates invoices for monthly billing", []string{"file_io"}),
		},
	})

	// Build v1 vocabulary (only repoA)
	v1, _ := ss.BuildOrgVocabulary([]*ctxpkg.RepoContext{repoA})
	mockVocab := &mockVocabStore{
		vocabs: map[string]*vectors.OrgVocabRecord{
			"org1": {
				OrgID:          "org1",
				VocabularyJSON: marshalVocab(t, v1),
				VersionHash:    v1.VersionHash,
				DocCount:       v1.DocCount,
			},
		},
	}
	ss.SetVocabStore(mockVocab)

	if err := ss.IndexRepositoryWithOrg(ctx, repoA, "org1"); err != nil {
		t.Fatalf("index repoA: %v", err)
	}

	vecA, _ := vecStore.Get(ctx, "repo-a:func:a.go:FuncA")
	if vecA == nil {
		t.Fatal("expected vecA to exist")
	}
	if vecA.VocabVersion != v1.VersionHash {
		t.Errorf("expected vocab_version '%s', got '%s'", v1.VersionHash, vecA.VocabVersion)
	}

	// Build v2 vocabulary (with both repos)
	v2, _ := ss.BuildOrgVocabulary([]*ctxpkg.RepoContext{repoA, repoB})

	if v1.VersionHash == v2.VersionHash {
		t.Error("expected different version hashes for different vocabularies")
	}

	// Update mock and re-index
	mockVocab.vocabs["org1"] = &vectors.OrgVocabRecord{
		OrgID:          "org1",
		VocabularyJSON: marshalVocab(t, v2),
		VersionHash:    v2.VersionHash,
		DocCount:       v2.DocCount,
	}

	// Clear and re-index
	ss.ClearRepository(ctx, "repo-a")
	if err := ss.IndexRepositoryWithOrg(ctx, repoA, "org1"); err != nil {
		t.Fatalf("re-index repoA: %v", err)
	}

	vecA2, _ := vecStore.Get(ctx, "repo-a:func:a.go:FuncA")
	if vecA2 == nil {
		t.Fatal("expected vecA to exist after re-index")
	}
	if vecA2.VocabVersion != v2.VersionHash {
		t.Errorf("expected vocab_version '%s', got '%s'", v2.VersionHash, vecA2.VocabVersion)
	}
}

// --- Helpers ---

// mockVocabStore implements vectors.OrgVocabularyStore for integration tests.
type mockVocabStore struct {
	vocabs map[string]*vectors.OrgVocabRecord
}

func (m *mockVocabStore) GetOrgVocabulary(_ context.Context, orgID string) (*vectors.OrgVocabRecord, error) {
	rec, ok := m.vocabs[orgID]
	if !ok {
		return nil, nil
	}
	return rec, nil
}

// storageHashAdapter adapts storage.SQLiteStore to vectors.FunctionHashStore.
type storageHashAdapter struct {
	store *storage.SQLiteStore
}

func (a *storageHashAdapter) GetFunctionHashes(ctx context.Context, repoID, filePath string) (map[string]vectors.FuncHashEntry, error) {
	infos, err := a.store.GetFunctionHashes(ctx, repoID, filePath)
	if err != nil {
		return nil, err
	}
	result := make(map[string]vectors.FuncHashEntry)
	for k, v := range infos {
		result[k] = vectors.FuncHashEntry{
			Name:        v.Name,
			Type:        v.Type,
			ContentHash: v.ContentHash,
			VectorID:    v.VectorID,
		}
	}
	return result, nil
}

func (a *storageHashAdapter) UpdateFunctionHashes(ctx context.Context, repoID, filePath string, hashes []vectors.FuncHashEntry) error {
	infos := make([]storage.FunctionHashInfo, len(hashes))
	for i, h := range hashes {
		infos[i] = storage.FunctionHashInfo{
			Name:        h.Name,
			Type:        h.Type,
			ContentHash: h.ContentHash,
			VectorID:    h.VectorID,
		}
	}
	return a.store.UpdateFunctionHashes(ctx, repoID, filePath, infos)
}

func (a *storageHashAdapter) DeleteFunctionHashes(ctx context.Context, repoID, filePath string) error {
	return a.store.DeleteFunctionHashes(ctx, repoID, filePath)
}

func (a *storageHashAdapter) GetChangedFunctions(ctx context.Context, repoID, filePath string, current map[string]string) (added, modified, removed []string, err error) {
	return a.store.GetChangedFunctions(ctx, repoID, filePath, current)
}

func marshalVocab(t *testing.T, v *vectors.VocabularyData) string {
	t.Helper()
	data, err := json.Marshal(v.WordIDF)
	if err != nil {
		t.Fatalf("marshal vocabulary: %v", err)
	}
	return string(data)
}

func cosineSimilarity(a, b []float64) float64 {
	if len(a) != len(b) {
		return 0
	}
	var dot, magA, magB float64
	for i := range a {
		dot += a[i] * b[i]
		magA += a[i] * a[i]
		magB += b[i] * b[i]
	}
	if magA == 0 || magB == 0 {
		return 0
	}
	return dot / (math.Sqrt(magA) * math.Sqrt(magB))
}
