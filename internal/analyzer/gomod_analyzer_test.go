package analyzer

import (
	"testing"
)

func TestParseGoMod_ValidWithDeps(t *testing.T) {
	content := []byte(`module github.com/example/myapp

go 1.21

require (
	github.com/gorilla/mux v1.8.1
	github.com/lib/pq v1.10.9
	golang.org/x/text v0.14.0 // indirect
)
`)
	info, err := ParseGoMod(content)
	if err != nil {
		t.Fatalf("ParseGoMod: %v", err)
	}
	if len(info.Dependencies) != 3 {
		t.Fatalf("got %d deps, want 3", len(info.Dependencies))
	}
	// Check direct vs indirect
	for _, dep := range info.Dependencies {
		if dep.Path == "golang.org/x/text" && dep.IsDirect {
			t.Error("golang.org/x/text should be indirect")
		}
		if dep.Path == "github.com/gorilla/mux" && !dep.IsDirect {
			t.Error("gorilla/mux should be direct")
		}
	}
}

func TestParseGoMod_ReplaceVersionSpecific(t *testing.T) {
	content := []byte(`module github.com/example/myapp

go 1.21

require github.com/old/pkg v1.0.0

replace github.com/old/pkg v1.0.0 => github.com/new/pkg v1.1.0
`)
	info, err := ParseGoMod(content)
	if err != nil {
		t.Fatalf("ParseGoMod: %v", err)
	}
	if len(info.Replaces) != 1 {
		t.Fatalf("got %d replaces, want 1", len(info.Replaces))
	}
	r := info.Replaces[0]
	if r.Old != "github.com/old/pkg" || r.New != "github.com/new/pkg" || r.Version != "v1.1.0" {
		t.Errorf("unexpected replace: %+v", r)
	}
	// Check IsReplaced on dependency
	for _, dep := range info.Dependencies {
		if dep.Path == "github.com/old/pkg" && !dep.IsReplaced {
			t.Error("old/pkg should be marked as replaced")
		}
	}
}

func TestParseGoMod_ReplaceLocalPath(t *testing.T) {
	content := []byte(`module github.com/example/myapp

go 1.21

require github.com/foo/bar v1.0.0

replace github.com/foo/bar => ../local-foo
`)
	info, err := ParseGoMod(content)
	if err != nil {
		t.Fatalf("ParseGoMod: %v", err)
	}
	if len(info.Replaces) != 1 {
		t.Fatalf("got %d replaces, want 1", len(info.Replaces))
	}
	r := info.Replaces[0]
	if r.New != "../local-foo" {
		t.Errorf("replace New = %q, want %q", r.New, "../local-foo")
	}
	if r.Version != "" {
		t.Errorf("replace Version = %q, want empty", r.Version)
	}
}

func TestParseGoMod_NoDependencies(t *testing.T) {
	content := []byte(`module github.com/example/minimal

go 1.21
`)
	info, err := ParseGoMod(content)
	if err != nil {
		t.Fatalf("ParseGoMod: %v", err)
	}
	if info.ModulePath != "github.com/example/minimal" {
		t.Errorf("ModulePath = %q, want %q", info.ModulePath, "github.com/example/minimal")
	}
	if info.GoVersion != "1.21" {
		t.Errorf("GoVersion = %q, want %q", info.GoVersion, "1.21")
	}
	if len(info.Dependencies) != 0 {
		t.Errorf("got %d deps, want 0", len(info.Dependencies))
	}
}

func TestParseGoMod_Malformed(t *testing.T) {
	content := []byte(`this is not a valid go.mod file!!!`)
	info, err := ParseGoMod(content)
	if err == nil {
		t.Fatal("expected error for malformed go.mod")
	}
	if info != nil {
		t.Error("expected nil info for malformed go.mod")
	}
}

func TestParseGoMod_ModulePath(t *testing.T) {
	content := []byte(`module github.com/gorilla/mux

go 1.20
`)
	info, err := ParseGoMod(content)
	if err != nil {
		t.Fatalf("ParseGoMod: %v", err)
	}
	if info.ModulePath != "github.com/gorilla/mux" {
		t.Errorf("ModulePath = %q, want %q", info.ModulePath, "github.com/gorilla/mux")
	}
}

func TestParseGoMod_GoVersion(t *testing.T) {
	content := []byte(`module example.com/test

go 1.22.0
`)
	info, err := ParseGoMod(content)
	if err != nil {
		t.Fatalf("ParseGoMod: %v", err)
	}
	if info.GoVersion != "1.22.0" {
		t.Errorf("GoVersion = %q, want %q", info.GoVersion, "1.22.0")
	}
}

func TestParseGoMod_DirectVsIndirect(t *testing.T) {
	content := []byte(`module github.com/example/app

go 1.21

require (
	github.com/direct/one v1.0.0
	github.com/indirect/two v0.5.0 // indirect
	github.com/direct/three v0.3.0
)
`)
	info, err := ParseGoMod(content)
	if err != nil {
		t.Fatalf("ParseGoMod: %v", err)
	}
	directCount := 0
	for _, dep := range info.Dependencies {
		if dep.IsDirect {
			directCount++
		}
	}
	if directCount != 2 {
		t.Errorf("direct count = %d, want 2", directCount)
	}
}
