package orchestrator

import (
	"testing"
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
