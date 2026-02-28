package vectors

import (
	"sort"
	"testing"
)

// mockEmbedder for testing the embedder registry.
type mockEmbedder struct {
	name string
	dim  int
}

func (e *mockEmbedder) Name() string      { return e.name }
func (e *mockEmbedder) Dimension() int     { return e.dim }
func (e *mockEmbedder) Embed(text string) []float64 {
	vec := make([]float64, e.dim)
	for i, c := range text {
		vec[i%e.dim] += float64(c) / 1000.0
	}
	return vec
}
func (e *mockEmbedder) EmbedBatch(texts []string) [][]float64 {
	result := make([][]float64, len(texts))
	for i, t := range texts {
		result[i] = e.Embed(t)
	}
	return result
}

func TestLocalEmbedder_Name(t *testing.T) {
	e := NewDefaultEmbedder()
	if e.Name() != "local-tfidf" {
		t.Errorf("expected Name() == 'local-tfidf', got %q", e.Name())
	}
}

func TestNewEmbedderRegistry_WithDefault(t *testing.T) {
	reg, err := NewEmbedderRegistry(NewDefaultEmbedder())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if reg.Default().Name() != "local-tfidf" {
		t.Errorf("expected default name 'local-tfidf', got %q", reg.Default().Name())
	}
}

func TestNewEmbedderRegistry_GetByName(t *testing.T) {
	custom := &mockEmbedder{name: "custom", dim: 256}
	reg, err := NewEmbedderRegistry(NewDefaultEmbedder(), custom)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if reg.Get("custom") == nil {
		t.Fatal("expected to find 'custom' embedder")
	}
	if reg.Get("custom").Name() != "custom" {
		t.Errorf("expected 'custom', got %q", reg.Get("custom").Name())
	}
	if reg.Get("unknown") != nil {
		t.Errorf("expected nil for unknown embedder, got %v", reg.Get("unknown"))
	}
}

func TestNewEmbedderRegistry_Names(t *testing.T) {
	emb1 := &mockEmbedder{name: "alpha", dim: 256}
	emb2 := &mockEmbedder{name: "beta", dim: 256}
	reg, err := NewEmbedderRegistry(NewDefaultEmbedder(), emb1, emb2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	names := reg.Names()
	if len(names) != 3 {
		t.Fatalf("expected 3 names, got %d: %v", len(names), names)
	}
	// Names should be sorted
	if !sort.StringsAreSorted(names) {
		t.Errorf("names should be sorted, got %v", names)
	}
}

func TestNewEmbedderRegistry_NilDefault(t *testing.T) {
	_, err := NewEmbedderRegistry(nil)
	if err == nil {
		t.Fatal("expected error for nil default")
	}
	if err.Error() != "default embedder required" {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestNewEmbedderRegistry_DimensionMismatch(t *testing.T) {
	emb256 := &mockEmbedder{name: "a", dim: 256}
	emb384 := &mockEmbedder{name: "b", dim: 384}
	_, err := NewEmbedderRegistry(emb256, emb384)
	if err == nil {
		t.Fatal("expected error for dimension mismatch")
	}
	if err.Error() != "dimension mismatch: embedder 'b' has dimension 384, expected 256" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestNewEmbedderRegistry_SameDimension(t *testing.T) {
	emb1 := &mockEmbedder{name: "a", dim: 256}
	emb2 := &mockEmbedder{name: "b", dim: 256}
	reg, err := NewEmbedderRegistry(emb1, emb2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if reg == nil {
		t.Fatal("expected non-nil registry")
	}
}

func TestDefaultEmbedderRegistry(t *testing.T) {
	reg := DefaultEmbedderRegistry()
	if reg.Default().Name() != "local-tfidf" {
		t.Errorf("expected default name 'local-tfidf', got %q", reg.Default().Name())
	}
	if reg.Default().Dimension() != 256 {
		t.Errorf("expected dimension 256, got %d", reg.Default().Dimension())
	}
}

func TestNewEmbedderRegistry_DuplicateNameLastWins(t *testing.T) {
	defaultEmb := &mockEmbedder{name: "default", dim: 256}
	emb1 := &mockEmbedder{name: "test", dim: 256}
	emb2 := &mockEmbedder{name: "test", dim: 256}
	reg, err := NewEmbedderRegistry(defaultEmb, emb1, emb2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := reg.Get("test")
	if got == nil {
		t.Fatal("expected non-nil embedder for 'test'")
	}
	// Both have same name, last should win
	if got != emb2 {
		t.Error("expected last embedder to win for duplicate name")
	}
}
