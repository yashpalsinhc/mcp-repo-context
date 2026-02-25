package analyzer

import (
	"encoding/json"
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

// --- Section 06: New tests ---

func TestQualifiedFuncName(t *testing.T) {
	tests := []struct {
		receiver string
		name     string
		want     string
	}{
		{"", "ServeHTTP", "ServeHTTP"},
		{"Router", "ServeHTTP", "Router.ServeHTTP"},
		{"*Router", "ServeHTTP", "Router.ServeHTTP"},
		{"*cors", "ServeHTTP", "cors.ServeHTTP"},
	}
	for _, tt := range tests {
		got := qualifiedFuncName(tt.receiver, tt.name)
		if got != tt.want {
			t.Errorf("qualifiedFuncName(%q, %q) = %q, want %q", tt.receiver, tt.name, got, tt.want)
		}
	}
}

func TestMakeNodeID_WithReceiver(t *testing.T) {
	b := NewCallGraphBuilder()
	id := b.makeNodeID("router.go", "*Router", "ServeHTTP")
	if id != "router.go:Router.ServeHTTP" {
		t.Errorf("makeNodeID = %q, want %q", id, "router.go:Router.ServeHTTP")
	}
}

func TestMakeNodeID_WithoutReceiver(t *testing.T) {
	b := NewCallGraphBuilder()
	id := b.makeNodeID("router.go", "", "NewRouter")
	if id != "router.go:NewRouter" {
		t.Errorf("makeNodeID = %q, want %q", id, "router.go:NewRouter")
	}
}

func TestMakeNodeID_NoDuplicateIDs(t *testing.T) {
	b := NewCallGraphBuilder()
	id1 := b.makeNodeID("handler.go", "*Router", "ServeHTTP")
	id2 := b.makeNodeID("handler.go", "*cors", "ServeHTTP")
	if id1 == id2 {
		t.Errorf("different receivers should produce different node IDs: %q == %q", id1, id2)
	}
}

func TestFuncFileMap_NoCollision(t *testing.T) {
	files := map[string]*ctxpkg.FileContext{
		"router.go": {
			Path: "router.go",
			Functions: []ctxpkg.FunctionDef{
				{Name: "ServeHTTP", Receiver: "*Router", Signature: "func (r *Router) ServeHTTP(w, r)"},
			},
		},
		"cors.go": {
			Path: "cors.go",
			Functions: []ctxpkg.FunctionDef{
				{Name: "ServeHTTP", Receiver: "*cors", Signature: "func (c *cors) ServeHTTP(w, r)"},
			},
		},
	}

	builder := NewCallGraphBuilder()
	graph := builder.BuildFromFiles(files)

	// Should have 2 distinct nodes
	if len(graph.Nodes) != 2 {
		t.Errorf("Expected 2 nodes, got %d", len(graph.Nodes))
	}

	// Both should exist with receiver-qualified IDs
	if _, ok := graph.Nodes["router.go:Router.ServeHTTP"]; !ok {
		t.Error("Expected Router.ServeHTTP node")
	}
	if _, ok := graph.Nodes["cors.go:cors.ServeHTTP"]; !ok {
		t.Error("Expected cors.ServeHTTP node")
	}
}

func TestFuncFileMap_DifferentReceivers(t *testing.T) {
	builder := NewCallGraphBuilder()
	files := map[string]*ctxpkg.FileContext{
		"router.go": {
			Path: "router.go",
			Functions: []ctxpkg.FunctionDef{
				{Name: "ServeHTTP", Receiver: "*Router"},
			},
		},
		"cors.go": {
			Path: "cors.go",
			Functions: []ctxpkg.FunctionDef{
				{Name: "ServeHTTP", Receiver: "*cors"},
			},
		},
	}
	builder.BuildFromFiles(files)

	routerFile := builder.funcFile["Router.ServeHTTP"]
	corsFile := builder.funcFile["cors.ServeHTTP"]

	if routerFile == corsFile {
		t.Errorf("funcFile should map to different files: Router=%q, cors=%q", routerFile, corsFile)
	}
	if routerFile != "router.go" {
		t.Errorf("Router.ServeHTTP should be in router.go, got %q", routerFile)
	}
	if corsFile != "cors.go" {
		t.Errorf("cors.ServeHTTP should be in cors.go, got %q", corsFile)
	}
}

func TestMethodCallResolution_FunctionParameter(t *testing.T) {
	src := `
package main

type Router struct{}

func (r *Router) HandleFunc(path string) {}

func setup(r *Router) {
	r.HandleFunc("/api")
}
`
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "test.go", src, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	builder := NewCallGraphBuilder()
	for _, decl := range f.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Name.Name == "setup" {
			calls := builder.ExtractFunctionCalls(fset, fn, "test.go", nil)
			found := false
			for _, call := range calls {
				if call.Function == "HandleFunc" {
					found = true
					if call.Receiver != "Router" {
						t.Errorf("Receiver = %q, want %q", call.Receiver, "Router")
					}
					if call.Type != "method" {
						t.Errorf("Type = %q, want %q", call.Type, "method")
					}
				}
			}
			if !found {
				t.Error("expected to find HandleFunc call")
			}
		}
	}
}

func TestMethodCallResolution_CompositeLiteral(t *testing.T) {
	src := `
package main

type Config struct{}

func (c Config) Validate() error { return nil }

func create() {
	cfg := Config{}
	cfg.Validate()
}
`
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "test.go", src, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	builder := NewCallGraphBuilder()
	for _, decl := range f.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Name.Name == "create" {
			calls := builder.ExtractFunctionCalls(fset, fn, "test.go", nil)
			found := false
			for _, call := range calls {
				if call.Function == "Validate" {
					found = true
					if call.Receiver != "Config" {
						t.Errorf("Receiver = %q, want %q", call.Receiver, "Config")
					}
				}
			}
			if !found {
				t.Error("expected to find Validate call")
			}
		}
	}
}

func TestMethodCallResolution_UnknownType(t *testing.T) {
	src := `
package main

func process(x interface{}) {
	x.Method()
}
`
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "test.go", src, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	builder := NewCallGraphBuilder()
	for _, decl := range f.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Name.Name == "process" {
			calls := builder.ExtractFunctionCalls(fset, fn, "test.go", nil)
			found := false
			for _, call := range calls {
				if call.Function == "Method" {
					found = true
					// Unknown type - should still be recorded
					if call.Type != "method" {
						t.Errorf("Type = %q, want %q", call.Type, "method")
					}
					// Receiver should be empty (unresolved)
					if call.Receiver != "" {
						t.Errorf("Receiver should be empty for unresolved, got %q", call.Receiver)
					}
				}
			}
			if !found {
				t.Error("expected to find Method call even when type is unknown")
			}
		}
	}
}

func TestPopulateCalledBy_IncludesMethodCalls(t *testing.T) {
	files := map[string]*ctxpkg.FileContext{
		"router.go": {
			Path: "router.go",
			Functions: []ctxpkg.FunctionDef{
				{
					Name:     "ServeHTTP",
					Receiver: "*Router",
					Calls:    []ctxpkg.CallRef{},
				},
			},
		},
		"main.go": {
			Path: "main.go",
			Functions: []ctxpkg.FunctionDef{
				{
					Name: "main",
					Calls: []ctxpkg.CallRef{
						{Function: "ServeHTTP", Type: "method", Receiver: "Router", Line: 10},
					},
				},
			},
		},
	}

	PopulateCalledBy(files)

	serveHTTP := files["router.go"].Functions[0]
	if len(serveHTTP.CalledBy) != 1 {
		t.Errorf("Router.ServeHTTP should be called by 1 function, got %d", len(serveHTTP.CalledBy))
	}
}

func TestServeHTTP_CallersAndCallees(t *testing.T) {
	files := map[string]*ctxpkg.FileContext{
		"router.go": {
			Path: "router.go",
			Functions: []ctxpkg.FunctionDef{
				{
					Name:     "ServeHTTP",
					Receiver: "*Router",
					Calls: []ctxpkg.CallRef{
						{Function: "Match", Type: "method", Receiver: "Route", Line: 20},
						{Function: "handleNotFound", Type: "internal", Line: 25},
					},
				},
				{
					Name:     "Match",
					Receiver: "*Route",
					Calls:    []ctxpkg.CallRef{},
				},
				{
					Name:  "handleNotFound",
					Calls: []ctxpkg.CallRef{},
				},
			},
		},
		"server.go": {
			Path: "server.go",
			Functions: []ctxpkg.FunctionDef{
				{
					Name: "startServer",
					Calls: []ctxpkg.CallRef{
						{Function: "ServeHTTP", Type: "method", Receiver: "Router", Line: 5},
					},
				},
			},
		},
	}

	builder := NewCallGraphBuilder()
	graph := builder.BuildFromFiles(files)

	serveNode, ok := graph.Nodes["router.go:Router.ServeHTTP"]
	if !ok {
		t.Fatal("Expected Router.ServeHTTP node")
	}

	// Should have callees (Match and handleNotFound)
	if len(serveNode.Calls) != 2 {
		t.Errorf("ServeHTTP should have 2 callees, got %d: %v", len(serveNode.Calls), serveNode.Calls)
	}

	// Should have callers (startServer)
	if len(serveNode.CalledBy) != 1 {
		t.Errorf("ServeHTTP should have 1 caller, got %d", len(serveNode.CalledBy))
	}
}

func TestCallRef_ReceiverPopulatedForMethods(t *testing.T) {
	src := `
package main

type DB struct{}

func (d *DB) Query(sql string) {}

func handler(db *DB) {
	db.Query("SELECT 1")
}
`
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "test.go", src, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	builder := NewCallGraphBuilder()
	for _, decl := range f.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Name.Name == "handler" {
			calls := builder.ExtractFunctionCalls(fset, fn, "test.go", nil)
			for _, call := range calls {
				if call.Function == "Query" {
					if call.Receiver != "DB" {
						t.Errorf("Receiver = %q, want %q", call.Receiver, "DB")
					}
					return
				}
			}
			t.Error("expected to find Query call")
		}
	}
}

func TestCallRef_ReceiverEmptyForPackageLevel(t *testing.T) {
	src := `
package main

func helper() {}

func caller() {
	helper()
}
`
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "test.go", src, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	builder := NewCallGraphBuilder()
	for _, decl := range f.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Name.Name == "caller" {
			calls := builder.ExtractFunctionCalls(fset, fn, "test.go", nil)
			for _, call := range calls {
				if call.Function == "helper" {
					if call.Receiver != "" {
						t.Errorf("Receiver should be empty for package-level call, got %q", call.Receiver)
					}
					return
				}
			}
			t.Error("expected to find helper call")
		}
	}
}

func TestCallRef_BackwardsCompatibility(t *testing.T) {
	// Old JSON without Receiver field should deserialize with empty Receiver
	oldJSON := `{"function":"Get","package":"http","file":"test.go","line":10,"type":"stdlib"}`
	var ref ctxpkg.CallRef
	if err := json.Unmarshal([]byte(oldJSON), &ref); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if ref.Receiver != "" {
		t.Errorf("Receiver should be empty for old data, got %q", ref.Receiver)
	}
	if ref.Function != "Get" {
		t.Errorf("Function = %q, want %q", ref.Function, "Get")
	}
}
