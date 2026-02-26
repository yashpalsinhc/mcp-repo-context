package analyzer

import (
	"context"
	"testing"

	ctxpkg "github.com/yashpalc/mcp-repo-context/internal/context"
	"github.com/yashpalc/mcp-repo-context/internal/repo"
)

// mockAnalyzer for testing
type mockAnalyzer struct {
	name      string
	languages []string
}

func (m *mockAnalyzer) Name() string                   { return m.name }
func (m *mockAnalyzer) Languages() []string           { return m.languages }
func (m *mockAnalyzer) AnalyzeFile(ctx context.Context, file repo.FileInfo, content []byte) (*ctxpkg.FileContext, error) {
	return nil, nil
}

// Test: NewGoAnalyzer returns analyzer with Name "go"
func TestNewGoAnalyzer_Name(t *testing.T) {
	a := NewGoAnalyzer()
	if a.Name() != "go" {
		t.Errorf("expected Name() == 'go', got %q", a.Name())
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
		t.Errorf("expected go analyzer to handle 'go' language, got %v", langs)
	}
}

// Test: NewGenericAnalyzer returns analyzer with Name "generic"
func TestNewGenericAnalyzer_Name(t *testing.T) {
	a := NewGenericAnalyzer()
	if a.Name() != "generic" {
		t.Errorf("expected Name() == 'generic', got %q", a.Name())
	}
}

// Test: NewRegistry with no args returns empty registry (generic fallback only)
func TestNewRegistry_NoArgs(t *testing.T) {
	r := NewRegistry()
	if r == nil {
		t.Fatal("expected non-nil registry")
	}

	// Get "go" should return generic (not registered)
	goAnalyzer := r.Get("go")
	if goAnalyzer.Name() != "generic" {
		t.Errorf("expected fallback 'generic' for 'go', got %q", goAnalyzer.Name())
	}

	// Get "python" should return generic
	pyAnalyzer := r.Get("python")
	if pyAnalyzer.Name() != "generic" {
		t.Errorf("expected fallback 'generic' for 'python', got %q", pyAnalyzer.Name())
	}
}

// Test: DefaultRegistry returns Go analyzer for "go"
func TestDefaultRegistry_Go(t *testing.T) {
	r := DefaultRegistry()
	goAnalyzer := r.Get("go")

	if goAnalyzer.Name() != "go" {
		t.Errorf("expected Name() == 'go', got %q", goAnalyzer.Name())
	}
}

// Test: DefaultRegistry falls back to generic for unknown
func TestDefaultRegistry_Unknown(t *testing.T) {
	r := DefaultRegistry()
	pyAnalyzer := r.Get("python")

	if pyAnalyzer.Name() != "generic" {
		t.Errorf("expected fallback 'generic' for 'python', got %q", pyAnalyzer.Name())
	}
}

// Test: NewRegistry with custom analyzer
func TestNewRegistry_CustomAnalyzer(t *testing.T) {
	mockPy := &mockAnalyzer{name: "python", languages: []string{"py", "python"}}
	r := NewRegistry(mockPy)

	// Should find python analyzer for both extensions
	pyByExt := r.Get("py")
	if pyByExt.Name() != "python" {
		t.Errorf("expected 'python' analyzer for 'py', got %q", pyByExt.Name())
	}

	pyByName := r.Get("python")
	if pyByName.Name() != "python" {
		t.Errorf("expected 'python' analyzer for 'python', got %q", pyByName.Name())
	}

	// Go should fallback to generic (not registered)
	goAnalyzer := r.Get("go")
	if goAnalyzer.Name() != "generic" {
		t.Errorf("expected fallback 'generic' for 'go', got %q", goAnalyzer.Name())
	}
}

// Test: Duplicate language last wins
func TestNewRegistry_DuplicateLanguage(t *testing.T) {
	analyzer1 := &mockAnalyzer{name: "first", languages: []string{"go"}}
	analyzer2 := &mockAnalyzer{name: "second", languages: []string{"go"}}
	r := NewRegistry(analyzer1, analyzer2)

	goAnalyzer := r.Get("go")
	if goAnalyzer.Name() != "second" {
		t.Errorf("expected last analyzer 'second' to win, got %q", goAnalyzer.Name())
	}
}

// Test: Multiple analyzers registered
func TestNewRegistry_MultipleAnalyzers(t *testing.T) {
	mockPy := &mockAnalyzer{name: "python", languages: []string{"py", "python"}}
	r := NewRegistry(NewGoAnalyzer(), mockPy)

	goAnalyzer := r.Get("go")
	if goAnalyzer.Name() != "go" {
		t.Errorf("expected 'go' analyzer, got %q", goAnalyzer.Name())
	}

	pyAnalyzer := r.Get("py")
	if pyAnalyzer.Name() != "python" {
		t.Errorf("expected 'python' analyzer, got %q", pyAnalyzer.Name())
	}

	rustAnalyzer := r.Get("rust")
	if rustAnalyzer.Name() != "generic" {
		t.Errorf("expected fallback 'generic' for 'rust', got %q", rustAnalyzer.Name())
	}
}

// Legacy tests (for backward compatibility - use DefaultRegistry)

func TestNewRegistry(t *testing.T) {
	r := NewRegistry()
	if r == nil {
		t.Fatal("expected non-nil registry")
	}
}

func TestRegistry_Get_Go(t *testing.T) {
	r := DefaultRegistry()
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
	r := DefaultRegistry()
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
	r := DefaultRegistry()

	// Test various unknown languages return same fallback
	languages := []string{"rust", "python", "javascript", "xyz"}
	for _, lang := range languages {
		a := r.Get(lang)
		if a == nil {
			t.Errorf("expected non-nil analyzer for %s", lang)
		}
	}
}
