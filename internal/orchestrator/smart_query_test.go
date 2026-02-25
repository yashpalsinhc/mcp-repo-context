package orchestrator

import (
	"strings"
	"testing"

	ctxpkg "github.com/yashpalc/mcp-repo-context/internal/context"
)

// --- parseQuery tests ---

func TestParseQuery_HowDoesRoutingWork_RoutesToFunction(t *testing.T) {
	// parseQuery still returns QueryTypeFunction for "how does X work" patterns
	qt, extracted, conf := parseQuery("how does routing work?")
	if qt != QueryTypeFunction {
		t.Errorf("parseQuery type = %s, want %s", qt, QueryTypeFunction)
	}
	if extracted["function"] != "routing" {
		t.Errorf("extracted function = %q, want %q", extracted["function"], "routing")
	}
	if conf < 0.8 {
		t.Errorf("confidence = %f, want >= 0.8", conf)
	}
}

func TestParseQuery_WhatDoesServeHTTPDo_RoutesToFunction(t *testing.T) {
	qt, extracted, conf := parseQuery("what does ServeHTTP do?")
	if qt != QueryTypeFunction {
		t.Errorf("parseQuery type = %s, want %s", qt, QueryTypeFunction)
	}
	if extracted["function"] != "servehttp" {
		t.Errorf("extracted function = %q, want %q", extracted["function"], "servehttp")
	}
	if conf < 0.9 {
		t.Errorf("confidence = %f, want >= 0.9", conf)
	}
}

func TestParseQuery_FindDatabaseFunctions_RoutesToSideEffect(t *testing.T) {
	qt, extracted, _ := parseQuery("find all database functions")
	if qt != QueryTypeSideEffect {
		t.Errorf("parseQuery type = %s, want %s", qt, QueryTypeSideEffect)
	}
	if extracted["effect"] != "db_query" {
		t.Errorf("extracted effect = %q, want %q", extracted["effect"], "db_query")
	}
}

func TestParseQuery_WhoCallsValidateToken_RoutesToCallers(t *testing.T) {
	qt, extracted, _ := parseQuery("who calls ValidateToken?")
	if qt != QueryTypeCallers {
		t.Errorf("parseQuery type = %s, want %s", qt, QueryTypeCallers)
	}
	if extracted["function"] != "validatetoken" {
		t.Errorf("extracted function = %q, want %q", extracted["function"], "validatetoken")
	}
}

func TestParseQuery_ProjectStructure_RoutesToArchitecture(t *testing.T) {
	// "what is the structure" should NOT match as type lookup for "structure"
	qt, _, _ := parseQuery("what is the project structure?")
	if qt != QueryTypeArchitecture {
		t.Errorf("parseQuery type = %s, want %s", qt, QueryTypeArchitecture)
	}
}

func TestParseQuery_ArchitectureGuard_PreventsMisclassification(t *testing.T) {
	tests := []struct {
		query string
		want  QueryType
	}{
		{"what is the architecture?", QueryTypeArchitecture},
		{"what is the layout of this project?", QueryTypeArchitecture},
		{"what is the overview?", QueryTypeArchitecture},
		{"what is the codebase structure?", QueryTypeArchitecture},
		// Actual type names should still match as type queries
		{"what is the UserConfig?", QueryTypeType},
	}
	for _, tt := range tests {
		qt, _, _ := parseQuery(tt.query)
		if qt != tt.want {
			t.Errorf("parseQuery(%q) type = %s, want %s", tt.query, qt, tt.want)
		}
	}
}

func TestParseQuery_ConfidenceLevels(t *testing.T) {
	// Exact regex match -> 0.9
	_, _, conf := parseQuery("what does ServeHTTP do?")
	if conf < 0.9 {
		t.Errorf("exact regex confidence = %f, want >= 0.9", conf)
	}

	// Side effect keyword match -> 0.8
	_, _, conf = parseQuery("find all database functions")
	if conf < 0.7 || conf > 0.9 {
		t.Errorf("keyword confidence = %f, want ~0.8", conf)
	}

	// Architecture keyword -> 0.7
	_, _, conf = parseQuery("show me the project overview")
	if conf < 0.6 || conf > 0.8 {
		t.Errorf("architecture keyword confidence = %f, want ~0.7", conf)
	}
}

// --- matchesPackagePath tests ---

func TestMatchesPackagePath_HttpDoesNotMatchHttps(t *testing.T) {
	if matchesPackagePath("net/https/client.go", "http") {
		t.Error("matchesPackagePath should not match http in https")
	}
	if !matchesPackagePath("net/http/server.go", "http") {
		t.Error("matchesPackagePath should match http in net/http/server.go")
	}
}

func TestMatchesPackagePath_MuxDoesNotMatchMuxer(t *testing.T) {
	if !matchesPackagePath("gorilla/mux/router.go", "mux") {
		t.Error("matchesPackagePath should match mux in gorilla/mux/router.go")
	}
	if matchesPackagePath("gorilla/muxer/router.go", "mux") {
		t.Error("matchesPackagePath should not match mux in muxer")
	}
}

func TestMatchesPackagePath_MultiSegment(t *testing.T) {
	if !matchesPackagePath("internal/orchestrator/smart_query.go", "internal/orchestrator") {
		t.Error("should match multi-segment path")
	}
	if matchesPackagePath("internal/orchestration/smart_query.go", "internal/orchestrator") {
		t.Error("should not match partial segment")
	}
}

// --- matchesFileName tests ---

func TestMatchesFileName_RouterGoDoesNotMatchSubrouterGo(t *testing.T) {
	if matchesFileName("gorilla/mux/subrouter.go", "router.go") {
		t.Error("matchesFileName should not match subrouter.go for router.go")
	}
	if !matchesFileName("gorilla/mux/router.go", "router.go") {
		t.Error("matchesFileName should match router.go")
	}
}

func TestMatchesFileName_ExactMatch(t *testing.T) {
	if !matchesFileName("router.go", "router.go") {
		t.Error("matchesFileName should match exact file name")
	}
}

func TestMatchesFileName_NoFalsePositive(t *testing.T) {
	if matchesFileName("pkg/myrouter.go", "router.go") {
		t.Error("matchesFileName should not match myrouter.go for router.go")
	}
}

// --- classifyFilePurpose tests ---

func TestClassifyFilePurpose(t *testing.T) {
	tests := []struct {
		filename string
		want     string
	}{
		{"router.go", "Source Code"},
		{"router_test.go", "Tests"},
		{"README.md", "Documentation"},
		{"config.yaml", "Configuration"},
		{"config.yml", "Configuration"},
		{"config.json", "Configuration"},
		{"config.toml", "Configuration"},
		{"go.mod", "Configuration"},
		{"go.sum", "Configuration"},
		{"Makefile", "Configuration"},
		{"Dockerfile", "Configuration"},
		{"script.sh", "Other"},
		{"pkg/handler_test.go", "Tests"},
		{"pkg/handler.go", "Source Code"},
	}
	for _, tt := range tests {
		got := classifyFilePurpose(tt.filename)
		if got != tt.want {
			t.Errorf("classifyFilePurpose(%q) = %q, want %q", tt.filename, got, tt.want)
		}
	}
}

// --- handlePackageQuery grouping tests ---

func TestPackageStructure_FlatPackage_NoDotGoGrouping(t *testing.T) {
	// A flat package should NOT produce headers like ".go/"
	result := buildPackageQueryOutput(t, "mux", map[string]*fileInfoForTest{
		"mux/router.go":   {purpose: "Source Code"},
		"mux/route.go":    {purpose: "Source Code"},
		"mux/mux.go":      {purpose: "Source Code"},
	})

	if containsString(result, ".go/") {
		t.Errorf("flat package should not contain '.go/' grouping\n%s", result)
	}
}

func TestPackageStructure_FlatPackage_GroupsByPurpose(t *testing.T) {
	result := buildPackageQueryOutput(t, "pkg", map[string]*fileInfoForTest{
		"pkg/router.go":      {purpose: "Source Code"},
		"pkg/router_test.go": {purpose: "Tests"},
		"pkg/README.md":      {purpose: "Documentation"},
		"pkg/config.yaml":    {purpose: "Configuration"},
	})

	if !containsString(result, "### Source Code") {
		t.Errorf("expected 'Source Code' header\n%s", result)
	}
	if !containsString(result, "### Tests") {
		t.Errorf("expected 'Tests' header\n%s", result)
	}
	if !containsString(result, "### Documentation") {
		t.Errorf("expected 'Documentation' header\n%s", result)
	}
	if !containsString(result, "### Configuration") {
		t.Errorf("expected 'Configuration' header\n%s", result)
	}
}

func TestPackageStructure_MultiDirectory_GroupsByDir(t *testing.T) {
	result := buildPackageQueryOutput(t, "pkg", map[string]*fileInfoForTest{
		"pkg/handlers/auth.go":      {purpose: "Source Code"},
		"pkg/handlers/auth_test.go": {purpose: "Tests"},
		"pkg/models/user.go":        {purpose: "Source Code"},
	})

	if !containsString(result, "### `handlers/`") {
		t.Errorf("expected 'handlers/' directory header\n%s", result)
	}
	if !containsString(result, "### `models/`") {
		t.Errorf("expected 'models/' directory header\n%s", result)
	}
}

func TestPackageStructure_NoExtensionGrouping(t *testing.T) {
	result := buildPackageQueryOutput(t, "pkg", map[string]*fileInfoForTest{
		"pkg/router.go":  {purpose: "Source Code"},
		"pkg/README.md":  {purpose: "Documentation"},
		"pkg/go.mod":     {purpose: "Configuration"},
	})

	if containsString(result, ".go/") || containsString(result, ".md/") || containsString(result, ".mod/") {
		t.Errorf("should not contain extension-based grouping\n%s", result)
	}
}

func TestPackageStructure_DeepNesting_CollapsesTo2Levels(t *testing.T) {
	result := buildPackageQueryOutput(t, "pkg", map[string]*fileInfoForTest{
		"pkg/a/b/c/deep.go":    {purpose: "Source Code"},
		"pkg/a/b/shallow.go":   {purpose: "Source Code"},
		"pkg/a/top.go":         {purpose: "Source Code"},
	})

	// Deep files should be collapsed into 2-level groups
	if containsString(result, "### `a/b/c/`") {
		t.Errorf("should collapse a/b/c/ to a/b/\n%s", result)
	}
}

func TestPackageStructure_SingleFile(t *testing.T) {
	result := buildPackageQueryOutput(t, "pkg", map[string]*fileInfoForTest{
		"pkg/main.go": {purpose: "Source Code"},
	})

	if !containsString(result, "main.go") {
		t.Errorf("single file should still appear\n%s", result)
	}
}

// --- helpers for package structure tests ---

type fileInfoForTest struct {
	purpose string
}

func buildPackageQueryOutput(t *testing.T, packagePath string, files map[string]*fileInfoForTest) string {
	t.Helper()
	// Build mock RepoContext
	ctxFiles := make(map[string]*ctxpkg.FileContext)
	for path := range files {
		ctxFiles[path] = &ctxpkg.FileContext{
			Path:     path,
			Language: "go",
		}
	}

	repoCtx := &ctxpkg.RepoContext{
		ID:    "test-repo",
		Files: ctxFiles,
	}

	mgr := &manager{}
	result := &SmartQueryResult{}
	extracted := map[string]string{"package": packagePath}

	out, err := mgr.handlePackageQuery(nil, repoCtx, extracted, result)
	if err != nil {
		t.Fatalf("handlePackageQuery error: %v", err)
	}
	return out.Answer
}

func containsString(s, substr string) bool {
	return strings.Contains(s, substr)
}
