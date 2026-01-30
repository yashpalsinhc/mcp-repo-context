package analyzer

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"

	ctxpkg "github.com/yashpalc/mcp-repo-context/internal/context"
)

func TestCallGraphBuilder_ExtractFunctionCalls(t *testing.T) {
	src := `
package main

import (
	"fmt"
	"net/http"
)

func main() {
	result := processData("input")
	fmt.Println(result)
}

func processData(input string) string {
	resp, err := http.Get("http://example.com")
	if err != nil {
		return handleError(err)
	}
	defer resp.Body.Close()
	return formatResult(input)
}

func handleError(err error) string {
	return err.Error()
}

func formatResult(s string) string {
	return "Formatted: " + s
}
`
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "test.go", src, parser.ParseComments)
	if err != nil {
		t.Fatalf("Failed to parse source: %v", err)
	}

	imports := []ctxpkg.Import{
		{Path: "fmt"},
		{Path: "net/http"},
	}

	builder := NewCallGraphBuilder()

	// Extract calls from processData function
	for _, decl := range f.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Name.Name == "processData" {
			calls := builder.ExtractFunctionCalls(fset, fn, "test.go", imports)

			// Should find: http.Get, handleError, resp.Body.Close, formatResult
			if len(calls) < 3 {
				t.Errorf("Expected at least 3 calls, got %d", len(calls))
			}

			// Check for specific calls
			foundHTTPGet := false
			foundHandleError := false
			foundFormatResult := false

			for _, call := range calls {
				if call.Package == "http" && call.Function == "Get" {
					foundHTTPGet = true
					if call.Type != "stdlib" {
						t.Errorf("http.Get should be classified as stdlib, got %s", call.Type)
					}
				}
				if call.Function == "handleError" {
					foundHandleError = true
					if call.Type != "internal" {
						t.Errorf("handleError should be classified as internal, got %s", call.Type)
					}
				}
				if call.Function == "formatResult" {
					foundFormatResult = true
				}
			}

			if !foundHTTPGet {
				t.Error("Expected to find http.Get call")
			}
			if !foundHandleError {
				t.Error("Expected to find handleError call")
			}
			if !foundFormatResult {
				t.Error("Expected to find formatResult call")
			}
		}
	}
}

func TestCallGraphBuilder_BuildFromFiles(t *testing.T) {
	files := map[string]*ctxpkg.FileContext{
		"main.go": {
			Path:     "main.go",
			Language: "go",
			Functions: []ctxpkg.FunctionDef{
				{
					Name:      "main",
					Signature: "main()",
					LineStart: 10,
					Calls: []ctxpkg.CallRef{
						{Function: "processData", Type: "internal", Line: 12},
					},
				},
				{
					Name:      "processData",
					Signature: "processData(input string) string",
					LineStart: 20,
					Calls: []ctxpkg.CallRef{
						{Function: "Get", Package: "http", Type: "stdlib", Line: 22},
						{Function: "formatResult", Type: "internal", Line: 25},
					},
				},
				{
					Name:      "formatResult",
					Signature: "formatResult(s string) string",
					LineStart: 30,
					Calls:     []ctxpkg.CallRef{},
				},
			},
		},
	}

	builder := NewCallGraphBuilder()
	graph := builder.BuildFromFiles(files)

	// Should have 3 nodes
	if len(graph.Nodes) != 3 {
		t.Errorf("Expected 3 nodes, got %d", len(graph.Nodes))
	}

	// Check main node
	mainNode, ok := graph.Nodes["main.go:main"]
	if !ok {
		t.Fatal("Expected to find main.go:main node")
	}
	if len(mainNode.Calls) != 1 {
		t.Errorf("main should call 1 function, got %d", len(mainNode.Calls))
	}

	// Check processData node
	processNode, ok := graph.Nodes["main.go:processData"]
	if !ok {
		t.Fatal("Expected to find main.go:processData node")
	}
	// Only internal calls are resolved to nodes
	if len(processNode.Calls) != 1 {
		t.Errorf("processData should call 1 internal function, got %d", len(processNode.Calls))
	}

	// formatResult should be called by processData
	formatNode, ok := graph.Nodes["main.go:formatResult"]
	if !ok {
		t.Fatal("Expected to find main.go:formatResult node")
	}
	if len(formatNode.CalledBy) != 1 {
		t.Errorf("formatResult should be called by 1 function, got %d", len(formatNode.CalledBy))
	}
}

func TestPopulateCalledBy(t *testing.T) {
	files := map[string]*ctxpkg.FileContext{
		"main.go": {
			Path: "main.go",
			Functions: []ctxpkg.FunctionDef{
				{
					Name: "main",
					Calls: []ctxpkg.CallRef{
						{Function: "helper", Type: "internal", Line: 10},
					},
				},
				{
					Name:  "helper",
					Calls: []ctxpkg.CallRef{},
				},
			},
		},
		"util.go": {
			Path: "util.go",
			Functions: []ctxpkg.FunctionDef{
				{
					Name: "util",
					Calls: []ctxpkg.CallRef{
						{Function: "helper", Type: "internal", Line: 5},
					},
				},
			},
		},
	}

	PopulateCalledBy(files)

	// helper should be called by main and util
	helperFunc := files["main.go"].Functions[1]
	if len(helperFunc.CalledBy) != 2 {
		t.Errorf("helper should be called by 2 functions, got %d", len(helperFunc.CalledBy))
	}
}
