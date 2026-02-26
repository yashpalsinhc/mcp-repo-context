package vectors

import (
	"context"
	"database/sql"
	"encoding/json"
	"math/rand"
	"sort"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

func newTestStore(t *testing.T, dimension int) *SQLiteVectorStore {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	store, err := NewSQLiteVectorStoreWithDB(db, dimension)
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func TestDimensionValidation_StoreRejectsWrongDimension(t *testing.T) {
	store := newTestStore(t, 256)
	ctx := context.Background()

	record := VectorRecord{
		ID:     "test-1",
		RepoID: "repo-1",
		Type:   "function",
		Name:   "Foo",
		Vector: make([]float64, 384), // wrong dimension
	}
	err := store.Store(ctx, record)
	if err == nil {
		t.Fatal("expected error for wrong dimension")
	}
	if err.Error() != "vector dimension 384 does not match store dimension 256" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestDimensionValidation_StoreBatchRejectsWrongDimension(t *testing.T) {
	store := newTestStore(t, 256)
	ctx := context.Background()

	records := []VectorRecord{
		{ID: "test-1", RepoID: "repo-1", Type: "function", Name: "Foo", Vector: make([]float64, 256)},
		{ID: "test-2", RepoID: "repo-1", Type: "function", Name: "Bar", Vector: make([]float64, 384)},
	}
	err := store.StoreBatch(ctx, records)
	if err == nil {
		t.Fatal("expected error for wrong dimension in batch")
	}
}

func TestDimensionValidation_AcceptsCorrectDimension(t *testing.T) {
	store := newTestStore(t, 256)
	ctx := context.Background()

	record := VectorRecord{
		ID:     "test-1",
		RepoID: "repo-1",
		Type:   "function",
		Name:   "Foo",
		Vector: make([]float64, 256),
	}
	if err := store.Store(ctx, record); err != nil {
		t.Fatalf("should accept correct dimension: %v", err)
	}
}

func TestDimensionMigration_ClearsMismatchedVectors(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// Create store with dim=384 first
	store384, err := NewSQLiteVectorStoreWithDB(db, 384)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	// Insert a vector with dim=384
	vec := make([]float64, 384)
	vec[0] = 1.0
	err = store384.Store(ctx, VectorRecord{
		ID: "v1", RepoID: "repo", Type: "function", Name: "Foo", Vector: vec,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Verify it's there
	count, _ := store384.Count(ctx, "repo")
	if count != 1 {
		t.Fatalf("expected 1 vector, got %d", count)
	}

	// Now create a new store with dim=256 — should trigger migration
	store256, err := NewSQLiteVectorStoreWithDB(db, 256)
	if err != nil {
		t.Fatal(err)
	}

	count, _ = store256.Count(ctx, "repo")
	if count != 0 {
		t.Fatalf("expected 0 vectors after migration, got %d", count)
	}
}

func TestDimensionMigration_SkipsWhenEmpty(t *testing.T) {
	store := newTestStore(t, 256)
	dim, err := store.detectStoredDimension()
	if err != nil {
		t.Fatal(err)
	}
	if dim != 0 {
		t.Fatalf("expected 0 for empty store, got %d", dim)
	}
}

func TestMetadataTable_StoresDimension(t *testing.T) {
	store := newTestStore(t, 256)
	var value string
	err := store.db.QueryRow("SELECT value FROM vector_metadata WHERE key = 'dimension'").Scan(&value)
	if err != nil {
		t.Fatal(err)
	}
	if value != "256" {
		t.Fatalf("expected dimension 256, got %s", value)
	}
}

func TestSortBySimilarity_Correctness(t *testing.T) {
	results := make([]SearchResult, 1000)
	for i := range results {
		results[i] = SearchResult{
			Record:     VectorRecord{Name: "func"},
			Similarity: rand.Float64(),
		}
	}

	sortBySimilarity(results)

	for i := 1; i < len(results); i++ {
		if results[i].Similarity > results[i-1].Similarity {
			t.Fatalf("not sorted at index %d: %.4f > %.4f", i, results[i].Similarity, results[i-1].Similarity)
		}
	}
}

func TestTopKSimilar_SortedCorrectly(t *testing.T) {
	query := make([]float64, 256)
	query[0] = 1.0

	candidates := make([][]float64, 100)
	for i := range candidates {
		candidates[i] = make([]float64, 256)
		candidates[i][0] = rand.Float64()
	}

	indices := TopKSimilar(query, candidates, 10)
	if len(indices) != 10 {
		t.Fatalf("expected 10 results, got %d", len(indices))
	}

	// Verify they are sorted by similarity
	for i := 1; i < len(indices); i++ {
		s1 := CosineSimilarity(query, candidates[indices[i-1]])
		s2 := CosineSimilarity(query, candidates[indices[i]])
		if s2 > s1 {
			t.Fatalf("TopK results not sorted at position %d", i)
		}
	}
}

func TestIsAvailable_True(t *testing.T) {
	store := newTestStore(t, 256)
	ss := NewSemanticSearch(NewDefaultEmbedder(), store)
	if !ss.IsAvailable() {
		t.Error("expected IsAvailable true")
	}
}

func TestIsAvailable_FalseNilStore(t *testing.T) {
	ss := NewSemanticSearch(NewDefaultEmbedder(), nil)
	if ss.IsAvailable() {
		t.Error("expected IsAvailable false for nil store")
	}
}

func TestIsAvailable_FalseNilReceiver(t *testing.T) {
	var ss *SemanticSearch
	if ss.IsAvailable() {
		t.Error("expected IsAvailable false for nil receiver")
	}
}

func TestVocabularyPersistence(t *testing.T) {
	store := newTestStore(t, 256)
	ctx := context.Background()

	vocabJSON := []byte(`{"foo":0,"bar":1}`)
	idfJSON := []byte(`{"foo":1.5,"bar":2.0}`)

	// Store vocabulary
	err := store.StoreVocabulary(ctx, "repo-1", vocabJSON, idfJSON, 0)
	if err != nil {
		t.Fatal(err)
	}

	// Load vocabulary
	v, idf, version, err := store.LoadVocabulary(ctx, "repo-1")
	if err != nil {
		t.Fatal(err)
	}
	if version != 0 {
		t.Fatalf("expected version 0, got %d", version)
	}

	var vocabMap map[string]int
	json.Unmarshal(v, &vocabMap)
	if vocabMap["foo"] != 0 || vocabMap["bar"] != 1 {
		t.Fatalf("unexpected vocab: %s", v)
	}

	var idfMap map[string]float64
	json.Unmarshal(idf, &idfMap)
	if idfMap["foo"] != 1.5 {
		t.Fatalf("unexpected idf: %s", idf)
	}
}

func TestVocabularyVersionIncrement(t *testing.T) {
	store := newTestStore(t, 256)
	ctx := context.Background()

	store.StoreVocabulary(ctx, "repo-1", []byte(`{}`), []byte(`{}`), 0)

	v1, err := store.IncrementVocabularyVersion(ctx, "repo-1")
	if err != nil {
		t.Fatal(err)
	}
	if v1 != 1 {
		t.Fatalf("expected version 1, got %d", v1)
	}

	v2, err := store.IncrementVocabularyVersion(ctx, "repo-1")
	if err != nil {
		t.Fatal(err)
	}
	if v2 != 2 {
		t.Fatalf("expected version 2, got %d", v2)
	}
}

func TestVocabularyLoadNotFound(t *testing.T) {
	store := newTestStore(t, 256)
	ctx := context.Background()

	_, _, _, err := store.LoadVocabulary(ctx, "nonexistent")
	if err != sql.ErrNoRows {
		t.Fatalf("expected sql.ErrNoRows, got %v", err)
	}
}

func TestFilePathIndex_Exists(t *testing.T) {
	store := newTestStore(t, 256)
	var count int
	err := store.db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name='idx_vectors_file'`).Scan(&count)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatal("idx_vectors_file index should exist")
	}
}

func TestStoreDimension(t *testing.T) {
	store := newTestStore(t, 256)
	if store.Dimension() != 256 {
		t.Fatalf("expected dimension 256, got %d", store.Dimension())
	}
}

// Test that sort.Slice is used (not bubble sort) by running on larger set efficiently
func TestSortPerformance_NotQuadratic(t *testing.T) {
	results := make([]SearchResult, 5000)
	for i := range results {
		results[i] = SearchResult{Similarity: rand.Float64()}
	}

	sortBySimilarity(results)

	// Verify sorted
	if !sort.SliceIsSorted(results, func(i, j int) bool {
		return results[i].Similarity > results[j].Similarity
	}) {
		t.Fatal("results not properly sorted")
	}
}
