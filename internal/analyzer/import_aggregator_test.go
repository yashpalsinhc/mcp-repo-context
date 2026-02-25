package analyzer

import (
	"testing"

	ctxpkg "github.com/yashpalc/mcp-repo-context/internal/context"
)

func makeFiles(imports ...string) map[string]*ctxpkg.FileContext {
	fc := &ctxpkg.FileContext{}
	for _, imp := range imports {
		fc.Imports = append(fc.Imports, ctxpkg.Import{Path: imp})
	}
	return map[string]*ctxpkg.FileContext{"main.go": fc}
}

func TestAggregateImports_Stdlib(t *testing.T) {
	files := makeFiles("fmt", "net/http", "context", "os")
	mod := &ctxpkg.ModuleInfo{ModulePath: "github.com/example/myapp"}

	summary := AggregateImports(files, mod)

	if len(summary.Stdlib) != 4 {
		t.Fatalf("Stdlib count = %d, want 4", len(summary.Stdlib))
	}
	if len(summary.Internal) != 0 {
		t.Errorf("Internal count = %d, want 0", len(summary.Internal))
	}
	if len(summary.External) != 0 {
		t.Errorf("External count = %d, want 0", len(summary.External))
	}
}

func TestAggregateImports_Internal(t *testing.T) {
	files := makeFiles(
		"github.com/example/myapp/pkg/handlers",
		"github.com/example/myapp/internal/db",
	)
	mod := &ctxpkg.ModuleInfo{ModulePath: "github.com/example/myapp"}

	summary := AggregateImports(files, mod)

	if len(summary.Internal) != 2 {
		t.Fatalf("Internal count = %d, want 2", len(summary.Internal))
	}
	if len(summary.Stdlib) != 0 {
		t.Errorf("Stdlib count = %d, want 0", len(summary.Stdlib))
	}
	if len(summary.External) != 0 {
		t.Errorf("External count = %d, want 0", len(summary.External))
	}
}

func TestAggregateImports_External(t *testing.T) {
	files := makeFiles("github.com/gorilla/mux", "github.com/lib/pq")
	mod := &ctxpkg.ModuleInfo{
		ModulePath: "github.com/example/myapp",
		Dependencies: []ctxpkg.ModuleDependency{
			{Path: "github.com/gorilla/mux", Version: "v1.8.1"},
			{Path: "github.com/lib/pq", Version: "v1.10.9"},
		},
	}

	summary := AggregateImports(files, mod)

	if len(summary.External) != 2 {
		t.Fatalf("External count = %d, want 2", len(summary.External))
	}
	for _, ext := range summary.External {
		if ext.ModulePath == "" || ext.Version == "" {
			t.Errorf("External import %q missing module_path or version", ext.ImportPath)
		}
	}
}

func TestAggregateImports_LongestPrefixMatch(t *testing.T) {
	files := makeFiles("github.com/gorilla/mux/middleware")
	mod := &ctxpkg.ModuleInfo{
		ModulePath: "github.com/example/myapp",
		Dependencies: []ctxpkg.ModuleDependency{
			{Path: "github.com/gorilla/mux", Version: "v1.8.1"},
		},
	}

	summary := AggregateImports(files, mod)

	if len(summary.External) != 1 {
		t.Fatalf("External count = %d, want 1", len(summary.External))
	}
	ext := summary.External[0]
	if ext.ModulePath != "github.com/gorilla/mux" {
		t.Errorf("ModulePath = %q, want %q", ext.ModulePath, "github.com/gorilla/mux")
	}
	if ext.Version != "v1.8.1" {
		t.Errorf("Version = %q, want %q", ext.Version, "v1.8.1")
	}
}

func TestAggregateImports_ReplaceDirective(t *testing.T) {
	files := makeFiles("github.com/original/pkg/foo")
	mod := &ctxpkg.ModuleInfo{
		ModulePath: "github.com/example/myapp",
		Dependencies: []ctxpkg.ModuleDependency{
			{Path: "github.com/fork/pkg", Version: "v1.2.0"},
		},
		Replaces: []ctxpkg.ModuleReplace{
			{Old: "github.com/original/pkg", New: "github.com/fork/pkg", Version: "v1.2.0"},
		},
	}

	summary := AggregateImports(files, mod)

	if len(summary.External) != 1 {
		t.Fatalf("External count = %d, want 1", len(summary.External))
	}
	ext := summary.External[0]
	if ext.ModulePath != "github.com/fork/pkg" {
		t.Errorf("ModulePath = %q, want %q", ext.ModulePath, "github.com/fork/pkg")
	}
	if ext.Version != "v1.2.0" {
		t.Errorf("Version = %q, want %q", ext.Version, "v1.2.0")
	}
}

func TestAggregateImports_AliasIgnored(t *testing.T) {
	fc := &ctxpkg.FileContext{
		Imports: []ctxpkg.Import{
			{Path: "github.com/gorilla/mux", Alias: "router"},
		},
	}
	files := map[string]*ctxpkg.FileContext{"main.go": fc}
	mod := &ctxpkg.ModuleInfo{
		ModulePath: "github.com/example/myapp",
		Dependencies: []ctxpkg.ModuleDependency{
			{Path: "github.com/gorilla/mux", Version: "v1.8.1"},
		},
	}

	summary := AggregateImports(files, mod)

	if len(summary.External) != 1 {
		t.Fatalf("External count = %d, want 1", len(summary.External))
	}
	if summary.External[0].ModulePath != "github.com/gorilla/mux" {
		t.Errorf("ModulePath = %q, want github.com/gorilla/mux", summary.External[0].ModulePath)
	}
}

func TestAggregateImports_UnresolvedExternal(t *testing.T) {
	files := makeFiles("github.com/unknown/lib")
	mod := &ctxpkg.ModuleInfo{ModulePath: "github.com/example/myapp"}

	summary := AggregateImports(files, mod)

	if len(summary.External) != 1 {
		t.Fatalf("External count = %d, want 1", len(summary.External))
	}
	ext := summary.External[0]
	if ext.ModulePath != "github.com/unknown/lib" {
		t.Errorf("ModulePath = %q, want %q", ext.ModulePath, "github.com/unknown/lib")
	}
	if ext.Version != "" {
		t.Errorf("Version = %q, want empty", ext.Version)
	}
}

func TestAggregateImports_Deduplication(t *testing.T) {
	fc1 := &ctxpkg.FileContext{
		Imports: []ctxpkg.Import{
			{Path: "fmt"},
			{Path: "github.com/gorilla/mux"},
		},
	}
	fc2 := &ctxpkg.FileContext{
		Imports: []ctxpkg.Import{
			{Path: "fmt"},
			{Path: "github.com/gorilla/mux"},
		},
	}
	files := map[string]*ctxpkg.FileContext{"a.go": fc1, "b.go": fc2}
	mod := &ctxpkg.ModuleInfo{
		ModulePath: "github.com/example/myapp",
		Dependencies: []ctxpkg.ModuleDependency{
			{Path: "github.com/gorilla/mux", Version: "v1.8.1"},
		},
	}

	summary := AggregateImports(files, mod)

	if len(summary.Stdlib) != 1 {
		t.Errorf("Stdlib count = %d, want 1 (deduplicated)", len(summary.Stdlib))
	}
	if len(summary.External) != 1 {
		t.Errorf("External count = %d, want 1 (deduplicated)", len(summary.External))
	}
}

func TestAggregateImports_NilModuleInfo(t *testing.T) {
	files := makeFiles("fmt", "github.com/gorilla/mux")

	summary := AggregateImports(files, nil)

	if summary == nil {
		t.Fatal("expected non-nil summary")
	}
	if len(summary.Stdlib) != 0 && len(summary.Internal) != 0 && len(summary.External) != 0 {
		t.Error("expected empty summary for nil ModuleInfo")
	}
}

func TestIsStdlib(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"fmt", true},
		{"net/http", true},
		{"net/http/httptest", true},
		{"context", true},
		{"encoding/json", true},
		{"github.com/gorilla/mux", false},
		{"golang.org/x/text", false},
		{"C", true}, // C pseudo-package detected as stdlib by no-dots heuristic; skipped in AggregateImports
	}

	for _, tt := range tests {
		if got := isStdlib(tt.path); got != tt.want {
			t.Errorf("isStdlib(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}

func TestApplyReplaces(t *testing.T) {
	replaces := []ctxpkg.ModuleReplace{
		{Old: "github.com/old/pkg", New: "github.com/new/pkg", Version: "v1.1.0"},
	}

	// Exact match
	path, ver := applyReplaces("github.com/old/pkg", replaces)
	if path != "github.com/new/pkg" || ver != "v1.1.0" {
		t.Errorf("exact match: got (%q, %q), want (github.com/new/pkg, v1.1.0)", path, ver)
	}

	// Subpackage match
	path, ver = applyReplaces("github.com/old/pkg/sub", replaces)
	if path != "github.com/new/pkg/sub" || ver != "v1.1.0" {
		t.Errorf("subpackage: got (%q, %q), want (github.com/new/pkg/sub, v1.1.0)", path, ver)
	}

	// No match
	path, ver = applyReplaces("github.com/other/pkg", replaces)
	if path != "github.com/other/pkg" || ver != "" {
		t.Errorf("no match: got (%q, %q), want (github.com/other/pkg, \"\")", path, ver)
	}
}
