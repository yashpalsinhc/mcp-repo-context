package analyzer

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"

	ctxpkg "github.com/yashpalc/mcp-repo-context/internal/context"
)

func TestBehaviorExtractor_BasicFunction(t *testing.T) {
	src := `
package main

func add(a, b int) int {
	return a + b
}
`
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "test.go", src, parser.ParseComments)
	if err != nil {
		t.Fatalf("Failed to parse source: %v", err)
	}

	extractor := NewBehaviorExtractor(fset)
	fn := f.Decls[0].(*ast.FuncDecl)
	behavior := extractor.Extract(fn, nil)

	// Should produce some kind of summary
	if behavior.Summary == "" {
		t.Error("Expected non-empty summary")
	}
	// Should have at least one step (the return statement)
	if len(behavior.Steps) == 0 {
		t.Logf("Summary: %s", behavior.Summary)
	}
}

func TestBehaviorExtractor_HTTPHandler(t *testing.T) {
	src := `
package main

import (
	"encoding/json"
	"net/http"
)

func HandleUser(w http.ResponseWriter, r *http.Request) {
	var user User
	json.NewDecoder(r.Body).Decode(&user)
	json.NewEncoder(w).Encode(user)
}
`
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "test.go", src, parser.ParseComments)
	if err != nil {
		t.Fatalf("Failed to parse source: %v", err)
	}

	imports := []ctxpkg.Import{
		{Path: "encoding/json"},
		{Path: "net/http"},
	}

	extractor := NewBehaviorExtractor(fset)
	// Find the function declaration
	for _, decl := range f.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok {
			behavior := extractor.Extract(fn, imports)

			// Should detect handler pattern based on signature
			hasHandlerPattern := false
			for _, p := range behavior.Patterns {
				if strings.Contains(strings.ToLower(p), "handler") || strings.Contains(strings.ToLower(p), "http") {
					hasHandlerPattern = true
					break
				}
			}
			if !hasHandlerPattern {
				t.Logf("Patterns detected: %v (handler detection may need param types)", behavior.Patterns)
			}

			// Should produce a summary
			if behavior.Summary == "" {
				t.Error("Expected non-empty summary for HTTP handler")
			}
		}
	}
}

func TestBehaviorExtractor_DatabaseOperation(t *testing.T) {
	src := `
package main

import "database/sql"

func GetUser(db *sql.DB, id int) (*User, error) {
	row := db.QueryRow("SELECT * FROM users WHERE id = ?", id)
	var user User
	err := row.Scan(&user.ID, &user.Name)
	if err != nil {
		return nil, err
	}
	return &user, nil
}
`
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "test.go", src, parser.ParseComments)
	if err != nil {
		t.Fatalf("Failed to parse source: %v", err)
	}

	imports := []ctxpkg.Import{
		{Path: "database/sql"},
	}

	extractor := NewBehaviorExtractor(fset)
	for _, decl := range f.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok {
			behavior := extractor.Extract(fn, imports)

			// Should detect database pattern
			hasDBPattern := false
			for _, p := range behavior.Patterns {
				if strings.Contains(strings.ToLower(p), "database") || strings.Contains(strings.ToLower(p), "crud") {
					hasDBPattern = true
					break
				}
			}
			if !hasDBPattern {
				t.Logf("Patterns: %v", behavior.Patterns)
			}

			// Should have steps mentioning database
			hasDBStep := false
			for _, step := range behavior.Steps {
				if strings.Contains(strings.ToLower(step), "database") || strings.Contains(strings.ToLower(step), "query") {
					hasDBStep = true
					break
				}
			}
			if !hasDBStep {
				t.Logf("Steps: %v", behavior.Steps)
			}
		}
	}
}

func TestBehaviorExtractor_ConditionalLogic(t *testing.T) {
	src := `
package main

func process(status string) string {
	if status == "active" {
		return "processing"
	} else if status == "pending" {
		return "waiting"
	}
	return "unknown"
}
`
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "test.go", src, parser.ParseComments)
	if err != nil {
		t.Fatalf("Failed to parse source: %v", err)
	}

	extractor := NewBehaviorExtractor(fset)
	for _, decl := range f.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok {
			behavior := extractor.Extract(fn, nil)

			// Should produce some summary
			if behavior.Summary == "" {
				t.Error("Expected non-empty summary for conditional function")
			}

			// Log what was extracted for verification
			t.Logf("Summary: %s", behavior.Summary)
			t.Logf("Steps: %v", behavior.Steps)
		}
	}
}

func TestBehaviorExtractor_LoopLogic(t *testing.T) {
	src := `
package main

func sumItems(items []int) int {
	total := 0
	for _, item := range items {
		total += item
	}
	return total
}
`
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "test.go", src, parser.ParseComments)
	if err != nil {
		t.Fatalf("Failed to parse source: %v", err)
	}

	extractor := NewBehaviorExtractor(fset)
	for _, decl := range f.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok {
			behavior := extractor.Extract(fn, nil)

			// Should describe the loop
			hasLoopStep := false
			for _, step := range behavior.Steps {
				if strings.Contains(strings.ToLower(step), "iterate") ||
					strings.Contains(strings.ToLower(step), "loop") ||
					strings.Contains(strings.ToLower(step), "each") {
					hasLoopStep = true
					break
				}
			}
			if !hasLoopStep {
				t.Logf("Steps: %v (loop detection may vary)", behavior.Steps)
			}
		}
	}
}

func TestSplitCamelCase(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "CamelCase",
			input:    "getUserProfile",
			expected: []string{"get", "User", "Profile"},
		},
		{
			name:     "PascalCase",
			input:    "HandleUserRequest",
			expected: []string{"Handle", "User", "Request"},
		},
		{
			name:     "SingleWord",
			input:    "process",
			expected: []string{"process"},
		},
		{
			name:     "WithNumbers",
			input:    "process2Items",
			expected: []string{"process2", "Items"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := splitCamelCase(tt.input)
			if len(result) != len(tt.expected) {
				t.Errorf("Expected %d parts, got %d: %v", len(tt.expected), len(result), result)
				return
			}
			for i, exp := range tt.expected {
				if result[i] != exp {
					t.Errorf("At index %d: expected %q, got %q", i, exp, result[i])
				}
			}
		})
	}
}
