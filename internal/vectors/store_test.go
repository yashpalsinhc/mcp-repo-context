package vectors

import (
	"context"
	"database/sql"
	"os"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

func setupTestDB(t *testing.T) (*SQLiteVectorStore, func()) {
	t.Helper()

	tmpFile, err := os.CreateTemp("", "vectors_test_*.db")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	tmpFile.Close()

	store, err := NewSQLiteVectorStore(tmpFile.Name(), 128)
	if err != nil {
		os.Remove(tmpFile.Name())
		t.Fatalf("failed to create store: %v", err)
	}

	cleanup := func() {
		store.Close()
		os.Remove(tmpFile.Name())
	}

	return store, cleanup
}

func TestSQLiteVectorStore_StoreAndGet(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	record := VectorRecord{
		ID:       "test:func:main.go:CreateUser",
		RepoID:   "test-repo",
		Type:     "function",
		Name:     "CreateUser",
		FilePath: "main.go",
		Vector:   make([]float64, 128),
		Metadata: map[string]string{
			"signature": "func CreateUser(ctx context.Context) error",
		},
	}

	// Set some non-zero values
	record.Vector[0] = 1.0
	record.Vector[10] = 0.5

	// Store
	err := store.Store(ctx, record)
	if err != nil {
		t.Fatalf("Store failed: %v", err)
	}

	// Get
	retrieved, err := store.Get(ctx, record.ID)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if retrieved == nil {
		t.Fatal("Get returned nil")
	}

	if retrieved.ID != record.ID {
		t.Errorf("ID mismatch: got %s, want %s", retrieved.ID, record.ID)
	}

	if retrieved.Name != record.Name {
		t.Errorf("Name mismatch: got %s, want %s", retrieved.Name, record.Name)
	}

	if len(retrieved.Vector) != 128 {
		t.Errorf("Vector length: got %d, want 128", len(retrieved.Vector))
	}

	if retrieved.Vector[0] != 1.0 {
		t.Errorf("Vector[0]: got %f, want 1.0", retrieved.Vector[0])
	}

	if retrieved.Metadata["signature"] != record.Metadata["signature"] {
		t.Error("Metadata mismatch")
	}
}

func TestSQLiteVectorStore_StoreBatch(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	records := []VectorRecord{
		{
			ID:     "test:1",
			RepoID: "test-repo",
			Type:   "function",
			Name:   "Func1",
			Vector: make([]float64, 128),
		},
		{
			ID:     "test:2",
			RepoID: "test-repo",
			Type:   "function",
			Name:   "Func2",
			Vector: make([]float64, 128),
		},
		{
			ID:     "test:3",
			RepoID: "test-repo",
			Type:   "type",
			Name:   "Type1",
			Vector: make([]float64, 128),
		},
	}

	err := store.StoreBatch(ctx, records)
	if err != nil {
		t.Fatalf("StoreBatch failed: %v", err)
	}

	count, err := store.Count(ctx, "test-repo")
	if err != nil {
		t.Fatalf("Count failed: %v", err)
	}

	if count != 3 {
		t.Errorf("Count: got %d, want 3", count)
	}
}

func TestSQLiteVectorStore_Delete(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	record := VectorRecord{
		ID:     "to-delete",
		RepoID: "test-repo",
		Type:   "function",
		Name:   "ToDelete",
		Vector: make([]float64, 128),
	}

	store.Store(ctx, record)

	// Verify it exists
	retrieved, _ := store.Get(ctx, record.ID)
	if retrieved == nil {
		t.Fatal("record should exist before delete")
	}

	// Delete
	err := store.Delete(ctx, record.ID)
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// Verify it's gone
	retrieved, _ = store.Get(ctx, record.ID)
	if retrieved != nil {
		t.Error("record should not exist after delete")
	}
}

func TestSQLiteVectorStore_DeleteByRepo(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	records := []VectorRecord{
		{ID: "repo1:1", RepoID: "repo1", Type: "function", Name: "F1", Vector: make([]float64, 128)},
		{ID: "repo1:2", RepoID: "repo1", Type: "function", Name: "F2", Vector: make([]float64, 128)},
		{ID: "repo2:1", RepoID: "repo2", Type: "function", Name: "F1", Vector: make([]float64, 128)},
	}

	store.StoreBatch(ctx, records)

	// Delete repo1
	err := store.DeleteByRepo(ctx, "repo1")
	if err != nil {
		t.Fatalf("DeleteByRepo failed: %v", err)
	}

	// Repo1 should be empty
	count1, _ := store.Count(ctx, "repo1")
	if count1 != 0 {
		t.Errorf("repo1 should have 0 vectors, got %d", count1)
	}

	// Repo2 should still have vectors
	count2, _ := store.Count(ctx, "repo2")
	if count2 != 1 {
		t.Errorf("repo2 should have 1 vector, got %d", count2)
	}
}

func TestSQLiteVectorStore_Search(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create vectors with known similarity patterns
	vec1 := make([]float64, 128)
	vec1[0] = 1.0 // Primary dimension

	vec2 := make([]float64, 128)
	vec2[0] = 0.9
	vec2[1] = 0.1 // Similar to vec1

	vec3 := make([]float64, 128)
	vec3[10] = 1.0 // Different direction

	records := []VectorRecord{
		{ID: "1", RepoID: "test", Type: "function", Name: "Similar", Vector: vec1},
		{ID: "2", RepoID: "test", Type: "function", Name: "AlsoSimilar", Vector: vec2},
		{ID: "3", RepoID: "test", Type: "function", Name: "Different", Vector: vec3},
	}

	store.StoreBatch(ctx, records)

	// Search with vec1 as query
	results, err := store.Search(ctx, vec1, "test", 2)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	if len(results) != 2 {
		t.Errorf("expected 2 results, got %d", len(results))
	}

	// First result should be exact match (similarity = 1.0)
	if results[0].Record.ID != "1" {
		t.Errorf("first result should be '1', got '%s'", results[0].Record.ID)
	}

	if results[0].Similarity < 0.99 {
		t.Errorf("first similarity should be ~1.0, got %f", results[0].Similarity)
	}

	// Second should be similar vec2
	if results[1].Record.ID != "2" {
		t.Errorf("second result should be '2', got '%s'", results[1].Record.ID)
	}
}

func TestSQLiteVectorStore_SearchByType(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	vec := make([]float64, 128)
	vec[0] = 1.0

	records := []VectorRecord{
		{ID: "f1", RepoID: "test", Type: "function", Name: "Func", Vector: vec},
		{ID: "t1", RepoID: "test", Type: "type", Name: "Type", Vector: vec},
	}

	store.StoreBatch(ctx, records)

	// Search only functions
	results, err := store.SearchByType(ctx, vec, "test", "function", 10)
	if err != nil {
		t.Fatalf("SearchByType failed: %v", err)
	}

	if len(results) != 1 {
		t.Errorf("expected 1 function result, got %d", len(results))
	}

	if results[0].Record.Type != "function" {
		t.Errorf("result should be function type, got %s", results[0].Record.Type)
	}
}

func TestSQLiteVectorStore_GetNotFound(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	result, err := store.Get(ctx, "nonexistent")
	if err != nil {
		t.Fatalf("Get should not error for missing: %v", err)
	}

	if result != nil {
		t.Error("Get should return nil for missing record")
	}
}

func TestNewSQLiteVectorStoreWithDB(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "vectors_test_*.db")
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

	store, err := NewSQLiteVectorStoreWithDB(db, 256)
	if err != nil {
		t.Fatalf("NewSQLiteVectorStoreWithDB failed: %v", err)
	}

	if store.dimension != 256 {
		t.Errorf("dimension should be 256, got %d", store.dimension)
	}
}
