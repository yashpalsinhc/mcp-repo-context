package mcp

import (
	"testing"

	ctxpkg "github.com/yashpalc/mcp-repo-context/internal/context"
)

func TestMakeFunctionDetailRef(t *testing.T) {
	ref := MakeFunctionDetailRef("github.com/org/repo", "pkg/handlers/user.go", "CreateUser")
	expected := "func:github.com/org/repo:pkg/handlers/user.go:CreateUser"
	if ref != expected {
		t.Errorf("expected %s, got %s", expected, ref)
	}
}

func TestMakeFileDetailRef(t *testing.T) {
	ref := MakeFileDetailRef("github.com/org/repo", "pkg/handlers/user.go")
	expected := "file:github.com/org/repo:pkg/handlers/user.go"
	if ref != expected {
		t.Errorf("expected %s, got %s", expected, ref)
	}
}

func TestToFunctionSummary(t *testing.T) {
	fn := &ctxpkg.FunctionDef{
		Name:      "CreateUser",
		Signature: "func CreateUser(ctx context.Context, req *CreateUserRequest) (*User, error)",
		LineStart: 42,
		Behavior: &ctxpkg.FunctionBehavior{
			Summary: "Creates a new user in the database",
		},
	}

	summary := ToFunctionSummary(fn, "test-repo", "pkg/handlers/user.go")

	if summary.Name != "CreateUser" {
		t.Errorf("expected name CreateUser, got %s", summary.Name)
	}
	if summary.Summary != "Creates a new user in the database" {
		t.Errorf("unexpected summary: %s", summary.Summary)
	}
	if summary.DetailRef == "" {
		t.Error("expected DetailRef to be set")
	}
}

func TestToFunctionSummary_TruncateLongSummary(t *testing.T) {
	longSummary := "This is a very long summary that exceeds one hundred characters and should be truncated to fit within the limit for compact display"
	fn := &ctxpkg.FunctionDef{
		Name:      "LongFunc",
		Signature: "func LongFunc()",
		Behavior: &ctxpkg.FunctionBehavior{
			Summary: longSummary,
		},
	}

	summary := ToFunctionSummary(fn, "test-repo", "test.go")

	if len(summary.Summary) > 103 { // 100 + "..."
		t.Errorf("summary not truncated, length: %d", len(summary.Summary))
	}
	if summary.Summary[len(summary.Summary)-3:] != "..." {
		t.Error("truncated summary should end with ...")
	}
}

func TestDefaultBudget(t *testing.T) {
	budget := DefaultBudget()

	if budget.MaxResults != 20 {
		t.Errorf("expected MaxResults 20, got %d", budget.MaxResults)
	}
	if budget.Format != "summary" {
		t.Errorf("expected Format summary, got %s", budget.Format)
	}
	if !budget.IncludeRefs {
		t.Error("expected IncludeRefs to be true")
	}
}

func TestTruncateResults(t *testing.T) {
	results := &SearchResultCompact{
		Query:      "test",
		TotalCount: 50,
		Functions:  make([]FunctionSummary, 30),
	}

	// Fill with dummy data
	for i := 0; i < 30; i++ {
		results.Functions[i] = FunctionSummary{Name: "func"}
	}

	budget := OutputBudget{MaxResults: 10}
	TruncateResults(results, budget)

	if len(results.Functions) != 10 {
		t.Errorf("expected 10 functions, got %d", len(results.Functions))
	}
	if !results.HasMore {
		t.Error("expected HasMore to be true")
	}
	if results.NextOffset != 10 {
		t.Errorf("expected NextOffset 10, got %d", results.NextOffset)
	}
}

func TestEstimateTokens(t *testing.T) {
	// Test with a simple struct
	data := struct {
		Name string `json:"name"`
		Size int    `json:"size"`
	}{
		Name: "test",
		Size: 100,
	}

	tokens := EstimateTokens(data)
	// {"name":"test","size":100} = ~28 chars / 4 = ~7 tokens
	if tokens < 5 || tokens > 15 {
		t.Errorf("unexpected token estimate: %d", tokens)
	}
}

func TestGetQueryPatterns(t *testing.T) {
	patterns := GetQueryPatterns()

	if len(patterns) == 0 {
		t.Error("expected at least one query pattern")
	}

	// Verify all patterns have required fields
	for _, p := range patterns {
		if p.Name == "" {
			t.Error("pattern missing name")
		}
		if len(p.Examples) == 0 {
			t.Errorf("pattern %s has no examples", p.Name)
		}
		if p.ToolToUse == "" {
			t.Errorf("pattern %s has no tool recommendation", p.Name)
		}
	}

	// Verify we have both AI and non-AI patterns
	hasAI := false
	hasNonAI := false
	for _, p := range patterns {
		if p.Cost.AIRequired {
			hasAI = true
		} else {
			hasNonAI = true
		}
	}

	if !hasAI {
		t.Error("expected at least one AI-requiring pattern")
	}
	if !hasNonAI {
		t.Error("expected at least one non-AI pattern")
	}
}

func TestToTypeSummary(t *testing.T) {
	typeDef := &ctxpkg.TypeDef{
		Name:      "User",
		Kind:      "struct",
		LineStart: 10,
	}

	summary := ToTypeSummary(typeDef, "test-repo", "models/user.go")

	if summary.Name != "User" {
		t.Errorf("expected name User, got %s", summary.Name)
	}
	if summary.Kind != "struct" {
		t.Errorf("expected kind struct, got %s", summary.Kind)
	}
	if summary.DetailRef == "" {
		t.Error("expected DetailRef to be set")
	}
}

func TestToFileSummary(t *testing.T) {
	fileCtx := &ctxpkg.FileContext{
		Package:   "handlers",
		Functions: make([]ctxpkg.FunctionDef, 5),
		Types:     make([]ctxpkg.TypeDef, 2),
		Concepts:  []string{"handler", "http"},
	}

	summary := ToFileSummary(fileCtx, "test-repo", "pkg/handlers/user.go")

	if summary.Package != "handlers" {
		t.Errorf("expected package handlers, got %s", summary.Package)
	}
	if summary.FunctionCount != 5 {
		t.Errorf("expected 5 functions, got %d", summary.FunctionCount)
	}
	if summary.TypeCount != 2 {
		t.Errorf("expected 2 types, got %d", summary.TypeCount)
	}
	if len(summary.Concepts) != 2 {
		t.Errorf("expected 2 concepts, got %d", len(summary.Concepts))
	}
}
