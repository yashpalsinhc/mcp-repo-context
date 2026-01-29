package analyzer

import (
	"testing"
)

func TestNewRegistry(t *testing.T) {
	r := NewRegistry()
	if r == nil {
		t.Fatal("expected non-nil registry")
	}
}

func TestRegistry_Get_Go(t *testing.T) {
	r := NewRegistry()
	a := r.Get("go")

	if a == nil {
		t.Fatal("expected non-nil analyzer for go")
	}

	langs := a.Languages()
	found := false
	for _, l := range langs {
		if l == "go" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected go analyzer to handle go language, got %v", langs)
	}
}

func TestRegistry_Get_Unknown(t *testing.T) {
	r := NewRegistry()
	a := r.Get("unknown-language")

	if a == nil {
		t.Fatal("expected non-nil fallback analyzer")
	}

	// Should return generic analyzer
	langs := a.Languages()
	if len(langs) != 1 || langs[0] != "unknown" {
		t.Errorf("expected fallback analyzer with unknown language, got %v", langs)
	}
}

func TestRegistry_Get_Fallback(t *testing.T) {
	r := NewRegistry()

	// Test various unknown languages return same fallback
	languages := []string{"rust", "python", "javascript", "xyz"}
	for _, lang := range languages {
		a := r.Get(lang)
		if a == nil {
			t.Errorf("expected non-nil analyzer for %s", lang)
		}
	}
}
