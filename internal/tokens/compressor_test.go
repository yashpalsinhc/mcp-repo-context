package tokens

import (
	"testing"

	ctxpkg "github.com/yashpalc/mcp-repo-context/internal/context"
)

func TestDefaultScorer_Score(t *testing.T) {
	scorer := DefaultScorer{}

	fn := &ctxpkg.FunctionDef{
		Name:        "CreateUser",
		Signature:   "func CreateUser(ctx context.Context, user *User) error",
		Description: "Creates a new user in the database",
		IsPublic:    true,
		Behavior: &ctxpkg.FunctionBehavior{
			Summary: "Validates user data and inserts into users table",
		},
	}

	tests := []struct {
		query    string
		minScore float64
	}{
		{"CreateUser", 3.0},  // Exact name match
		{"user", 3.0},        // Name contains user
		{"database", 1.5},    // Description contains database
		{"validate", 2.0},    // Behavior contains validate
		{"xyz", 0.0},         // No match
		{"", 0.0},            // Empty query
	}

	for _, tc := range tests {
		t.Run(tc.query, func(t *testing.T) {
			score := scorer.Score(fn, tc.query)
			if score < tc.minScore {
				t.Errorf("Score(%q) = %f, expected at least %f", tc.query, score, tc.minScore)
			}
		})
	}
}

func TestDefaultScorer_ScoreNil(t *testing.T) {
	scorer := DefaultScorer{}

	if scorer.Score(nil, "test") != 0 {
		t.Error("Score with nil function should be 0")
	}
}

func TestCompressor_Summarize(t *testing.T) {
	compressor := NewCompressor()

	fn := ctxpkg.FunctionDef{
		Name:        "GetUser",
		Signature:   "func GetUser(id int) (*User, error)",
		Description: "Retrieves a user by ID",
		IsPublic:    true,
		LineStart:   10,
		LineEnd:     50,
		Complexity:  5,
		Behavior: &ctxpkg.FunctionBehavior{
			Summary: "Fetches user from database",
			Steps:   []string{"validate id", "query db", "return user"},
		},
		Calls: make([]ctxpkg.CallRef, 10), // 10 calls
		SideEffects: []string{"db_query"},
	}

	summary := compressor.Summarize(fn)

	// Should keep essential fields
	if summary.Name != fn.Name {
		t.Error("Name should be preserved")
	}
	if summary.Signature != fn.Signature {
		t.Error("Signature should be preserved")
	}
	if summary.LineStart != fn.LineStart {
		t.Error("LineStart should be preserved")
	}

	// Should have behavior summary but not steps
	if summary.Behavior == nil || summary.Behavior.Summary != "Fetches user from database" {
		t.Error("Behavior summary should be preserved")
	}
	if summary.Behavior != nil && len(summary.Behavior.Steps) > 0 {
		t.Error("Behavior steps should be removed")
	}

	// Should truncate calls to 5
	if len(summary.Calls) > 5 {
		t.Errorf("Calls should be truncated to 5, got %d", len(summary.Calls))
	}

	// Should remove description and complexity
	if summary.Description != "" {
		t.Error("Description should be removed in summary")
	}
}

func TestCompressor_FilterRelevant(t *testing.T) {
	compressor := NewCompressor()

	funcs := []ctxpkg.FunctionDef{
		{Name: "CreateUser", Signature: "func CreateUser()", IsPublic: true},
		{Name: "GetUser", Signature: "func GetUser()", IsPublic: true},
		{Name: "DeleteUser", Signature: "func DeleteUser()", IsPublic: true},
		{Name: "ValidateEmail", Signature: "func ValidateEmail()", IsPublic: true},
	}

	// Large budget - should include all relevant
	result := compressor.FilterRelevant("user", funcs, 10000)
	if len(result) < 3 { // CreateUser, GetUser, DeleteUser match "user"
		t.Errorf("expected at least 3 functions matching 'user', got %d", len(result))
	}

	// Small budget - should truncate
	result2 := compressor.FilterRelevant("user", funcs, 50)
	if len(result2) > 2 {
		t.Error("should truncate with small budget")
	}

	// Empty query - no results
	result3 := compressor.FilterRelevant("", funcs, 10000)
	if len(result3) != 0 {
		t.Error("empty query should return no results")
	}
}

func TestCompressor_CompressFile(t *testing.T) {
	compressor := NewCompressor()

	fc := &ctxpkg.FileContext{
		Path:     "pkg/handlers/user.go",
		Package:  "handlers",
		Language: "go",
		Purpose:  "User HTTP handlers",
		Concepts: []string{"http", "handler"},
		Functions: []ctxpkg.FunctionDef{
			{Name: "CreateUser", IsPublic: true, Signature: "func CreateUser()"},
			{Name: "helper", IsPublic: false, Signature: "func helper()"},
		},
		Types: []ctxpkg.TypeDef{
			{Name: "UserRequest", Kind: "struct", IsPublic: true},
			{Name: "internal", Kind: "struct", IsPublic: false},
		},
	}

	compressed := compressor.CompressFile(fc)

	// Should preserve basic info
	if compressed.Path != fc.Path {
		t.Error("Path should be preserved")
	}
	if compressed.Package != fc.Package {
		t.Error("Package should be preserved")
	}

	// Should only include public functions
	if len(compressed.Functions) != 1 {
		t.Errorf("expected 1 public function, got %d", len(compressed.Functions))
	}

	// Should only include public types
	if len(compressed.Types) != 1 {
		t.Errorf("expected 1 public type, got %d", len(compressed.Types))
	}
}

func TestCompressor_CompressFile_Nil(t *testing.T) {
	compressor := NewCompressor()

	if compressor.CompressFile(nil) != nil {
		t.Error("CompressFile(nil) should return nil")
	}
}

func TestCompressor_EstimateTokens(t *testing.T) {
	compressor := NewCompressor()

	fn := ctxpkg.FunctionDef{
		Name:      "Test",
		Signature: "func Test()",
	}

	tokens := compressor.EstimateTokens(fn)
	if tokens < 5 || tokens > 50 {
		t.Errorf("unexpected token estimate: %d", tokens)
	}
}
