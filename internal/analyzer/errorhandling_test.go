package analyzer

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"

	ctxpkg "github.com/yashpalc/mcp-repo-context/internal/context"
)

func TestErrorHandlingExtractor_ReturnsError(t *testing.T) {
	src := `
package main

import "errors"

func doSomething() error {
	return errors.New("something failed")
}
`
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "test.go", src, parser.ParseComments)
	if err != nil {
		t.Fatalf("Failed to parse source: %v", err)
	}

	extractor := NewErrorHandlingExtractor(fset)
	for _, decl := range f.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Name.Name == "doSomething" {
			errHandling := extractor.Extract(fn)

			if !errHandling.ReturnsError {
				t.Error("Expected ReturnsError to be true")
			}
		}
	}
}

func TestErrorHandlingExtractor_ErrorChecks(t *testing.T) {
	src := `
package main

import "os"

func readFile(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return data, nil
}
`
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "test.go", src, parser.ParseComments)
	if err != nil {
		t.Fatalf("Failed to parse source: %v", err)
	}

	extractor := NewErrorHandlingExtractor(fset)
	for _, decl := range f.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Name.Name == "readFile" {
			errHandling := extractor.Extract(fn)

			if errHandling.ErrorChecks < 1 {
				t.Errorf("Expected at least 1 error check, got %d", errHandling.ErrorChecks)
			}
			if !errHandling.PropagatesError {
				t.Error("Expected PropagatesError to be true")
			}
		}
	}
}

func TestErrorHandlingExtractor_WrapsErrors(t *testing.T) {
	src := `
package main

import "fmt"

func process(input string) error {
	err := validate(input)
	if err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}
	return nil
}
`
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "test.go", src, parser.ParseComments)
	if err != nil {
		t.Fatalf("Failed to parse source: %v", err)
	}

	extractor := NewErrorHandlingExtractor(fset)
	for _, decl := range f.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Name.Name == "process" {
			errHandling := extractor.Extract(fn)

			if !errHandling.WrapsErrors {
				t.Error("Expected WrapsErrors to be true")
			}
		}
	}
}

func TestErrorHandlingExtractor_LogsErrors(t *testing.T) {
	src := `
package main

import "log"

func handle(data []byte) {
	err := process(data)
	if err != nil {
		log.Printf("error processing: %v", err)
	}
}
`
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "test.go", src, parser.ParseComments)
	if err != nil {
		t.Fatalf("Failed to parse source: %v", err)
	}

	extractor := NewErrorHandlingExtractor(fset)
	for _, decl := range f.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Name.Name == "handle" {
			errHandling := extractor.Extract(fn)

			if !errHandling.LogsErrors {
				t.Error("Expected LogsErrors to be true")
			}
		}
	}
}

func TestErrorHandlingExtractor_Panics(t *testing.T) {
	src := `
package main

func mustInit() {
	err := initialize()
	if err != nil {
		panic(err)
	}
}
`
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "test.go", src, parser.ParseComments)
	if err != nil {
		t.Fatalf("Failed to parse source: %v", err)
	}

	extractor := NewErrorHandlingExtractor(fset)
	for _, decl := range f.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Name.Name == "mustInit" {
			errHandling := extractor.Extract(fn)

			if !errHandling.PanicsOnError {
				t.Error("Expected PanicsOnError to be true")
			}
		}
	}
}

func TestExtractSideEffects_HTTPCall(t *testing.T) {
	src := `
package main

import "net/http"

func fetchData(url string) (*http.Response, error) {
	return http.Get(url)
}
`
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "test.go", src, parser.ParseComments)
	if err != nil {
		t.Fatalf("Failed to parse source: %v", err)
	}

	imports := []ctxpkg.Import{
		{Path: "net/http"},
	}

	for _, decl := range f.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Name.Name == "fetchData" {
			sideEffects := ExtractSideEffects(fn, imports)

			found := false
			for _, effect := range sideEffects {
				if effect == "http_call" {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("Expected http_call side effect, got: %v", sideEffects)
			}
		}
	}
}

func TestExtractSideEffects_DBQuery(t *testing.T) {
	src := `
package main

import "database/sql"

func queryUsers(db *sql.DB) (*sql.Rows, error) {
	return db.Query("SELECT * FROM users")
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

	for _, decl := range f.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Name.Name == "queryUsers" {
			sideEffects := ExtractSideEffects(fn, imports)

			found := false
			for _, effect := range sideEffects {
				if effect == "db_query" {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("Expected db_query side effect, got: %v", sideEffects)
			}
		}
	}
}

func TestExtractSideEffects_FileIO(t *testing.T) {
	src := `
package main

import "os"

func writeData(path string, data []byte) error {
	return os.WriteFile(path, data, 0644)
}
`
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "test.go", src, parser.ParseComments)
	if err != nil {
		t.Fatalf("Failed to parse source: %v", err)
	}

	imports := []ctxpkg.Import{
		{Path: "os"},
	}

	for _, decl := range f.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Name.Name == "writeData" {
			sideEffects := ExtractSideEffects(fn, imports)

			found := false
			for _, effect := range sideEffects {
				if effect == "file_write" || effect == "file_io" {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("Expected file_write or file_io side effect, got: %v", sideEffects)
			}
		}
	}
}

func TestExtractSideEffects_Logging(t *testing.T) {
	src := `
package main

import "log"

func logMessage(msg string) {
	log.Printf("Message: %s", msg)
}
`
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "test.go", src, parser.ParseComments)
	if err != nil {
		t.Fatalf("Failed to parse source: %v", err)
	}

	imports := []ctxpkg.Import{
		{Path: "log"},
	}

	for _, decl := range f.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Name.Name == "logMessage" {
			sideEffects := ExtractSideEffects(fn, imports)

			found := false
			for _, effect := range sideEffects {
				if effect == "logging" {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("Expected logging side effect, got: %v", sideEffects)
			}
		}
	}
}
