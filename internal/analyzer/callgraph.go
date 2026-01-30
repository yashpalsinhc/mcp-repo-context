package analyzer

import (
	"go/ast"
	"go/token"
	"strings"

	ctxpkg "github.com/yashpalc/mcp-repo-context/internal/context"
)

// CallGraphBuilder builds a call graph from analyzed files.
type CallGraphBuilder struct {
	fset     *token.FileSet
	nodes    map[string]*ctxpkg.CallGraphNode
	edges    []ctxpkg.CallGraphEdge
	fileFunc map[string][]string // file -> functions defined in it
	funcFile map[string]string   // function -> file
}

// NewCallGraphBuilder creates a new call graph builder.
func NewCallGraphBuilder() *CallGraphBuilder {
	return &CallGraphBuilder{
		nodes:    make(map[string]*ctxpkg.CallGraphNode),
		edges:    make([]ctxpkg.CallGraphEdge, 0),
		fileFunc: make(map[string][]string),
		funcFile: make(map[string]string),
	}
}

// BuildFromFiles builds a call graph from file contexts.
func (b *CallGraphBuilder) BuildFromFiles(files map[string]*ctxpkg.FileContext) *ctxpkg.CallGraph {
	// First pass: register all functions
	for path, fileCtx := range files {
		for _, fn := range fileCtx.Functions {
			nodeID := b.makeNodeID(path, fn.Name)
			b.nodes[nodeID] = &ctxpkg.CallGraphNode{
				ID:        nodeID,
				File:      path,
				Function:  fn.Name,
				Signature: fn.Signature,
				Package:   b.extractPackage(path),
				IsPublic:  fn.IsPublic,
				Calls:     make([]string, 0),
				CalledBy:  make([]string, 0),
			}
			b.fileFunc[path] = append(b.fileFunc[path], fn.Name)
			b.funcFile[fn.Name] = path
		}
	}

	// Second pass: build edges from Calls data
	for path, fileCtx := range files {
		for _, fn := range fileCtx.Functions {
			callerID := b.makeNodeID(path, fn.Name)
			for _, call := range fn.Calls {
				// Try to resolve the call to a known function
				calleeID := b.resolveCall(call)
				if calleeID != "" {
					b.addEdge(callerID, calleeID, call.Line, call.Type)
				}
			}
		}
	}

	return &ctxpkg.CallGraph{
		Nodes: b.nodes,
		Edges: b.edges,
	}
}

// BuildFromAST extracts call information from AST during analysis.
func (b *CallGraphBuilder) BuildFromAST(fset *token.FileSet, file *ast.File, filePath string) []ctxpkg.CallRef {
	b.fset = fset
	calls := make([]ctxpkg.CallRef, 0)
	seenCalls := make(map[string]bool) // Deduplicate calls

	ast.Inspect(file, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.FuncDecl:
			// Extract calls from function body
			if node.Body != nil {
				fnCalls := b.extractCallsFromBlock(node.Body, filePath)
				for _, call := range fnCalls {
					key := call.Function + ":" + call.Package
					if !seenCalls[key] {
						calls = append(calls, call)
						seenCalls[key] = true
					}
				}
			}
		}
		return true
	})

	return calls
}

// ExtractFunctionCalls extracts calls from a specific function declaration.
func (b *CallGraphBuilder) ExtractFunctionCalls(fset *token.FileSet, fn *ast.FuncDecl, filePath string, imports []ctxpkg.Import) []ctxpkg.CallRef {
	b.fset = fset
	if fn.Body == nil {
		return nil
	}

	calls := make([]ctxpkg.CallRef, 0)
	seenCalls := make(map[string]bool)

	// Build import map for resolving packages
	importMap := make(map[string]string) // alias/name -> path
	for _, imp := range imports {
		alias := imp.Alias
		if alias == "" {
			// Use last part of path as name
			parts := strings.Split(imp.Path, "/")
			alias = parts[len(parts)-1]
		}
		importMap[alias] = imp.Path
	}

	ast.Inspect(fn.Body, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.CallExpr:
			call := b.extractCall(node, filePath, importMap)
			if call != nil {
				key := call.Function + ":" + call.Package
				if !seenCalls[key] {
					calls = append(calls, *call)
					seenCalls[key] = true
				}
			}
		case *ast.GoStmt:
			// go func() call
			if callExpr, ok := node.Call.Fun.(*ast.FuncLit); ok {
				_ = callExpr // Anonymous function, skip for now
			} else {
				call := b.extractCall(node.Call, filePath, importMap)
				if call != nil {
					call.Type = "go"
					key := call.Function + ":" + call.Package
					if !seenCalls[key] {
						calls = append(calls, *call)
						seenCalls[key] = true
					}
				}
			}
		case *ast.DeferStmt:
			// defer func() call
			call := b.extractCall(node.Call, filePath, importMap)
			if call != nil {
				call.Type = "defer"
				key := call.Function + ":" + call.Package
				if !seenCalls[key] {
					calls = append(calls, *call)
					seenCalls[key] = true
				}
			}
		}
		return true
	})

	return calls
}

func (b *CallGraphBuilder) extractCall(call *ast.CallExpr, filePath string, importMap map[string]string) *ctxpkg.CallRef {
	var funcName, pkgName, callType string
	callType = "direct"

	switch fn := call.Fun.(type) {
	case *ast.Ident:
		// Simple function call: funcName()
		funcName = fn.Name
		pkgName = "" // Same package
		callType = b.classifyCallType("")

	case *ast.SelectorExpr:
		// Package or method call: pkg.Func() or obj.Method()
		switch x := fn.X.(type) {
		case *ast.Ident:
			// Could be package.Func or variable.Method
			pkgName = x.Name
			funcName = fn.Sel.Name

			// Check if it's an imported package
			if importPath, ok := importMap[pkgName]; ok {
				callType = b.classifyCallType(importPath)
			} else {
				// Likely a method call on a variable
				callType = "method"
			}
		default:
			// Chain call like a.b.c()
			funcName = fn.Sel.Name
			callType = "method"
		}

	case *ast.FuncLit:
		// Anonymous function call
		return nil

	default:
		return nil
	}

	if funcName == "" {
		return nil
	}

	pos := b.fset.Position(call.Pos())
	return &ctxpkg.CallRef{
		Function: funcName,
		Package:  pkgName,
		File:     filePath,
		Line:     pos.Line,
		Type:     callType,
	}
}

func (b *CallGraphBuilder) classifyCallType(importPath string) string {
	// Check if it's a stdlib call
	if importPath != "" && !strings.Contains(importPath, ".") {
		return "stdlib"
	}
	// Check if it's an external package
	if strings.Contains(importPath, ".") {
		return "external"
	}
	return "internal"
}

func (b *CallGraphBuilder) extractCallsFromBlock(block *ast.BlockStmt, filePath string) []ctxpkg.CallRef {
	calls := make([]ctxpkg.CallRef, 0)

	ast.Inspect(block, func(n ast.Node) bool {
		if callExpr, ok := n.(*ast.CallExpr); ok {
			call := b.extractCall(callExpr, filePath, nil)
			if call != nil {
				calls = append(calls, *call)
			}
		}
		return true
	})

	return calls
}

func (b *CallGraphBuilder) makeNodeID(file, function string) string {
	return file + ":" + function
}

func (b *CallGraphBuilder) extractPackage(path string) string {
	// Extract package name from path
	parts := strings.Split(path, "/")
	if len(parts) > 0 {
		dir := parts[len(parts)-1]
		if strings.HasSuffix(dir, ".go") && len(parts) > 1 {
			return parts[len(parts)-2]
		}
		return dir
	}
	return ""
}

func (b *CallGraphBuilder) resolveCall(call ctxpkg.CallRef) string {
	// If it's a same-package call, look in the same directory
	if call.Package == "" || call.Type == "internal" {
		// Check if function exists in same file
		if file, ok := b.funcFile[call.Function]; ok {
			return b.makeNodeID(file, call.Function)
		}
	}

	// For now, only track internal calls
	// External and stdlib calls are tracked but not resolved to nodes
	return ""
}

func (b *CallGraphBuilder) addEdge(from, to string, line int, callType string) {
	if callType == "" {
		callType = "direct"
	}

	edge := ctxpkg.CallGraphEdge{
		From:     from,
		To:       to,
		Line:     line,
		CallType: callType,
	}
	b.edges = append(b.edges, edge)

	// Update node references
	if fromNode, ok := b.nodes[from]; ok {
		fromNode.Calls = append(fromNode.Calls, to)
	}
	if toNode, ok := b.nodes[to]; ok {
		toNode.CalledBy = append(toNode.CalledBy, from)
	}
}

// GetCallersOf returns all functions that call the given function.
func (b *CallGraphBuilder) GetCallersOf(funcID string) []string {
	if node, ok := b.nodes[funcID]; ok {
		return node.CalledBy
	}
	return nil
}

// GetCalleesOf returns all functions called by the given function.
func (b *CallGraphBuilder) GetCalleesOf(funcID string) []string {
	if node, ok := b.nodes[funcID]; ok {
		return node.Calls
	}
	return nil
}

// PopulateCalledBy updates CalledBy for all functions in file contexts.
func PopulateCalledBy(files map[string]*ctxpkg.FileContext) {
	// Build a map of all functions and their callers
	calledBy := make(map[string][]ctxpkg.CallRef) // func name -> callers

	// First pass: collect all calls
	for path, fileCtx := range files {
		for _, fn := range fileCtx.Functions {
			for _, call := range fn.Calls {
				if call.Type == "internal" || call.Type == "" || call.Package == "" {
					// This is a call to a function in the same package
					ref := ctxpkg.CallRef{
						Function: fn.Name,
						File:     path,
						Line:     call.Line,
						Type:     "internal",
					}
					calledBy[call.Function] = append(calledBy[call.Function], ref)
				}
			}
		}
	}

	// Second pass: populate CalledBy
	for _, fileCtx := range files {
		for i := range fileCtx.Functions {
			fn := &fileCtx.Functions[i]
			if callers, ok := calledBy[fn.Name]; ok {
				fn.CalledBy = callers
			}
		}
	}
}
