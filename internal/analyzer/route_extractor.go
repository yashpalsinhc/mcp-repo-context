package analyzer

import (
	"go/ast"
	"go/token"
	"regexp"
	"strings"

	ctxpkg "github.com/yashpalc/mcp-repo-context/internal/context"
)

// RouteExtractor extracts HTTP routes from Go source code.
type RouteExtractor struct {
	fset               *token.FileSet
	routes             []ctxpkg.Route
	file               string
	pendingRoutes      map[token.Pos]*ctxpkg.Route // For gorilla/mux HandleFunc + Methods chain
	prefixStack        []string                      // For Chi nested routing
	processedHandles   map[token.Pos]bool           // Track which HandleFunc calls have been processed
	processedRoutes    map[string]bool              // Track processed route positions to avoid duplicates
	currentPrefixDepth int                          // Track nesting depth
}

// NewRouteExtractor creates a new route extractor.
func NewRouteExtractor(fset *token.FileSet, file string) *RouteExtractor {
	return &RouteExtractor{
		fset:             fset,
		routes:           make([]ctxpkg.Route, 0),
		file:             file,
		pendingRoutes:    make(map[token.Pos]*ctxpkg.Route),
		prefixStack:      make([]string, 0),
		processedHandles: make(map[token.Pos]bool),
		processedRoutes:  make(map[string]bool),
	}
}

// Extract extracts routes from an AST.
func (e *RouteExtractor) Extract(node ast.Node) []ctxpkg.Route {
	// Don't use ast.Inspect - instead manually walk through declarations
	// This allows us to properly handle nested contexts
	if file, ok := node.(*ast.File); ok {
		for _, decl := range file.Decls {
			e.walkDecl(decl)
		}
	}

	// Flush any pending routes at the end
	for _, route := range e.pendingRoutes {
		if route != nil {
			e.routes = append(e.routes, *route)
		}
	}
	return e.routes
}

// walkDecl manually walks through a declaration
func (e *RouteExtractor) walkDecl(decl ast.Decl) {
	if funcDecl, ok := decl.(*ast.FuncDecl); ok {
		if funcDecl.Body != nil {
			e.walkStmts(funcDecl.Body.List)
		}
	}
}

// walkStmts walks through a list of statements
func (e *RouteExtractor) walkStmts(stmts []ast.Stmt) {
	for _, stmt := range stmts {
		e.walkStmt(stmt)
	}
}

// walkStmt walks through a single statement
func (e *RouteExtractor) walkStmt(stmt ast.Stmt) {
	if exprStmt, ok := stmt.(*ast.ExprStmt); ok {
		if call, ok := exprStmt.X.(*ast.CallExpr); ok {
			e.visit(call)
		}
	} else if blockStmt, ok := stmt.(*ast.BlockStmt); ok {
		e.walkStmts(blockStmt.List)
	} else if ifStmt, ok := stmt.(*ast.IfStmt); ok {
		if ifStmt.Body != nil {
			e.walkStmts(ifStmt.Body.List)
		}
		if ifStmt.Else != nil {
			e.walkStmt(ifStmt.Else)
		}
	} else if forStmt, ok := stmt.(*ast.ForStmt); ok {
		if forStmt.Body != nil {
			e.walkStmts(forStmt.Body.List)
		}
	} else if rangeStmt, ok := stmt.(*ast.RangeStmt); ok {
		if rangeStmt.Body != nil {
			e.walkStmts(rangeStmt.Body.List)
		}
	}
}

func (e *RouteExtractor) visit(n ast.Node) bool {
	call, ok := n.(*ast.CallExpr)
	if !ok {
		return true
	}

	// Check for method call patterns
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return true
	}

	method := sel.Sel.Name

	// Chi nested routing: r.Route("/prefix", func(r chi.Router) {...}) - MUST BE BEFORE OTHER METHOD CHECKS
	if method == "Route" {
		e.extractChiNestedRoute(call)
		return true
	}

	// Chi-style: r.Get, r.Post, etc. (check camelCase BEFORE uppercase to avoid false matches)
	if isHTTPMethodCamelCase(method) {
		e.extractChiRoute(call, method)
		return true
	}

	// Gin/Echo-style routes: r.GET, r.POST, etc.
	if isHTTPMethod(method) {
		e.extractGinEchoRoute(call, method)
		return true
	}

	// Gorilla/mux .Methods() chained call (check this BEFORE HandleFunc)
	if method == "Methods" {
		// Check if this is chained after HandleFunc
		if callExpr, ok := sel.X.(*ast.CallExpr); ok {
			if innerSel, ok := callExpr.Fun.(*ast.SelectorExpr); ok {
				if innerMethod := innerSel.Sel.Name; innerMethod == "HandleFunc" || innerMethod == "Handle" {
					e.extractGorillaMuxChainedRoute(call, callExpr)
					// Mark the inner HandleFunc as processed so we don't add it again
					e.processedHandles[callExpr.Pos()] = true
					return true
				}
			}
		}
	}

	// Gorilla/mux HandleFunc or Handle, or Go 1.22+ pattern
	if method == "HandleFunc" || method == "Handle" {
		// Skip if we've already processed this in a chain
		if e.processedHandles[call.Pos()] {
			return true
		}

		// Try Go 1.22 pattern first (looks for "METHOD /path" in first arg)
		if len(call.Args) > 0 {
			if firstArg := extractStringArg(call.Args[0]); firstArg != "" && strings.Contains(firstArg, " ") {
				// Looks like "GET /path" pattern
				e.extractGo122Pattern(call)
				return true
			}
		}
		// Treat as gorilla/mux (might be chained with .Methods(), but add with ANY for now)
		e.extractGorillaMuxRoute(call, sel)
		return true
	}

	return true
}

// extractGinEchoRoute handles Gin and Echo style routes: r.GET("/path", handler)
func (e *RouteExtractor) extractGinEchoRoute(call *ast.CallExpr, method string) {
	if len(call.Args) < 2 {
		return
	}

	// First arg is path
	path := extractStringArg(call.Args[0])
	if path == "" {
		return
	}

	// Last arg is handler
	handler := extractHandlerName(call.Args[len(call.Args)-1])

	// Middle args are middleware
	var middleware []string
	for i := 1; i < len(call.Args)-1; i++ {
		if mw := extractHandlerName(call.Args[i]); mw != "" {
			middleware = append(middleware, mw)
		}
	}

	pos := e.fset.Position(call.Pos())
	normalizedPath := e.normalizePathParams(path)

	route := ctxpkg.Route{
		Method:      strings.ToUpper(method),
		Path:        normalizedPath,
		RawPath:     path,
		Handler:     handler,
		File:        e.file,
		Line:        pos.Line,
		Framework:   e.detectFramework(call),
		Middleware:  middleware,
	}
	e.routes = append(e.routes, route)
}

// extractChiRoute handles Chi style routes: r.Get("/path", handler)
func (e *RouteExtractor) extractChiRoute(call *ast.CallExpr, method string) {
	if len(call.Args) < 2 {
		return
	}

	path := extractStringArg(call.Args[0])
	handler := extractHandlerName(call.Args[1])

	if path == "" {
		return
	}

	// Build full path with any active prefix from nested routing
	prefixStr := strings.Join(e.prefixStack, "")
	fullPath := prefixStr + path
	normalizedPath := e.normalizePathParams(fullPath)

	pos := e.fset.Position(call.Pos())

	// Create a unique key for deduplication
	key := normalizedPath + ":" + strings.ToUpper(method) + ":" + handler + ":" + string(rune(pos.Line))
	if e.processedRoutes[key] {
		return // Skip if already processed
	}
	e.processedRoutes[key] = true

	route := ctxpkg.Route{
		Method:      strings.ToUpper(method), // Chi uses camelCase so convert to uppercase
		Path:        normalizedPath,
		RawPath:     fullPath,
		Handler:     handler,
		File:        e.file,
		Line:        pos.Line,
		Framework:   "chi",
	}
	e.routes = append(e.routes, route)
}

// extractChiNestedRoute handles Chi nested routing: r.Route("/prefix", func(r chi.Router) {...})
func (e *RouteExtractor) extractChiNestedRoute(call *ast.CallExpr) {
	if len(call.Args) < 2 {
		return
	}

	// First arg is the prefix path
	prefix := extractStringArg(call.Args[0])
	if prefix == "" {
		return
	}

	// Push prefix onto stack
	e.prefixStack = append(e.prefixStack, prefix)
	e.currentPrefixDepth++

	// Second arg should be a function literal with the nested router definition
	if len(call.Args) > 1 {
		if fn, ok := call.Args[1].(*ast.FuncLit); ok {
			if fn.Body != nil {
				// Walk the statements in the function body (don't use ast.Inspect)
				e.walkStmts(fn.Body.List)
			}
		}
	}

	// Pop prefix from stack
	e.currentPrefixDepth--
	if len(e.prefixStack) > 0 {
		e.prefixStack = e.prefixStack[:len(e.prefixStack)-1]
	}
}

// extractGorillaMuxRoute handles gorilla/mux HandleFunc with optional .Methods() chain
// Example: r.HandleFunc("/path", handler).Methods("GET")
func (e *RouteExtractor) extractGorillaMuxRoute(call *ast.CallExpr, sel *ast.SelectorExpr) {
	if len(call.Args) < 2 {
		return
	}

	path := extractStringArg(call.Args[0])
	handler := extractHandlerName(call.Args[1])

	if path == "" {
		return
	}

	pos := e.fset.Position(call.Pos())

	// Create a pending route - will be filled in if .Methods() is chained
	route := ctxpkg.Route{
		Method:    "ANY", // Default if no Methods() chain
		Path:      e.normalizePathParams(path),
		RawPath:   path,
		Handler:   handler,
		File:      e.file,
		Line:      pos.Line,
		Framework: "gorilla/mux",
	}

	e.routes = append(e.routes, route)
}

// extractGorillaMuxChainedRoute handles gorilla/mux .Methods() chained after HandleFunc
// Example: r.HandleFunc("/path", handler).Methods("GET")
// methodsCall is the .Methods(...) call, handleCall is the .HandleFunc(...) call
func (e *RouteExtractor) extractGorillaMuxChainedRoute(methodsCall *ast.CallExpr, handleCall *ast.CallExpr) {
	if len(handleCall.Args) < 2 {
		return
	}

	path := extractStringArg(handleCall.Args[0])
	handler := extractHandlerName(handleCall.Args[1])

	if path == "" {
		return
	}

	// Extract methods from the Methods() call args
	var methods []string
	for _, arg := range methodsCall.Args {
		if method := extractStringArg(arg); method != "" {
			methods = append(methods, strings.ToUpper(method))
		}
	}

	pos := e.fset.Position(handleCall.Pos())

	// Create a route for each method
	for _, method := range methods {
		route := ctxpkg.Route{
			Method:    method,
			Path:      e.normalizePathParams(path),
			RawPath:   path,
			Handler:   handler,
			File:      e.file,
			Line:      pos.Line,
			Framework: "gorilla/mux",
		}
		e.routes = append(e.routes, route)
	}

	// If no methods specified, still add with ANY
	if len(methods) == 0 {
		route := ctxpkg.Route{
			Method:    "ANY",
			Path:      e.normalizePathParams(path),
			RawPath:   path,
			Handler:   handler,
			File:      e.file,
			Line:      pos.Line,
			Framework: "gorilla/mux",
		}
		e.routes = append(e.routes, route)
	}
}

// extractGo122Pattern handles Go 1.22+ http.HandleFunc("GET /path/{id}", handler)
// The method and path are combined in the first argument
func (e *RouteExtractor) extractGo122Pattern(call *ast.CallExpr) {
	if len(call.Args) < 1 {
		return
	}

	// First arg contains "METHOD /path"
	patternStr := extractStringArg(call.Args[0])
	if patternStr == "" {
		return
	}

	// Handler is typically the second arg (or might not be present)
	var handler string
	if len(call.Args) > 1 {
		handler = extractHandlerName(call.Args[1])
	}

	pos := e.fset.Position(call.Pos())

	// Split "GET /path" into method and path
	parts := strings.SplitN(patternStr, " ", 2)
	var method, path string

	if len(parts) == 2 {
		method = strings.ToUpper(parts[0])
		path = parts[1]
	} else {
		// Only path provided, no method
		method = "ANY"
		path = patternStr
	}

	route := ctxpkg.Route{
		Method:      method,
		Path:        e.normalizePathParams(path),
		RawPath:     path,
		Handler:     handler,
		File:        e.file,
		Line:        pos.Line,
		Framework:   "http",
	}

	e.routes = append(e.routes, route)
}

// normalizePathParams converts various path parameter syntaxes to a standard format.
// Examples:
//   - "{id:[0-9]+}" -> "{id}" (strip regex in gorilla braces)
//   - ":id" -> "{id}" (gin colon params, but not "://")
//   - "{id}" -> "{id}" (unchanged)
func (e *RouteExtractor) normalizePathParams(path string) string {
	// Skip URLs with :// (protocol markers)
	if strings.Contains(path, "://") {
		return path
	}

	// Strip regex constraints in gorilla-style braces: {id:[0-9]+} -> {id}, {id:} -> {id}
	reGorilla := regexp.MustCompile(`\{([^:}]+):[^}]*\}`)
	path = reGorilla.ReplaceAllString(path, "{$1}")

	// Convert gin-style colon params to brace params: :id -> {id}
	// But be careful not to convert URLs like http://
	reGin := regexp.MustCompile(`([/:])(:([a-zA-Z_][a-zA-Z0-9_]*))\b`)
	path = reGin.ReplaceAllString(path, "$1{$3}")

	return path
}

// detectFramework tries to detect the web framework based on the call pattern
func (e *RouteExtractor) detectFramework(call *ast.CallExpr) string {
	// Look at the receiver to determine framework
	if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
		if ident, ok := sel.X.(*ast.Ident); ok {
			varName := ident.Name
			methodName := sel.Sel.Name

			// Common patterns: "r" for router, "e" for echo, "app" for gin
			switch varName {
			case "e":
				return "echo"
			case "app":
				return "gin"
			case "r":
				// Check method name for hints
				// Chi uses camelCase methods like Get, Post, Put, Delete, Route
				if isHTTPMethodCamelCase(methodName) || methodName == "Route" {
					return "chi"
				}
				// Default to gin for uppercase HTTP methods
				return "gin"
			}
		}
	}
	return "unknown"
}

func isHTTPMethod(name string) bool {
	methods := map[string]bool{
		"GET": true, "POST": true, "PUT": true, "DELETE": true,
		"PATCH": true, "HEAD": true, "OPTIONS": true, "ANY": true,
	}
	return methods[name]
}

func isHTTPMethodCamelCase(name string) bool {
	methods := map[string]bool{
		"Get": true, "Post": true, "Put": true, "Delete": true,
		"Patch": true, "Head": true, "Options": true,
	}
	return methods[name]
}

func extractStringArg(expr ast.Expr) string {
	switch v := expr.(type) {
	case *ast.BasicLit:
		if v.Kind == token.STRING {
			return strings.Trim(v.Value, `"'`)
		}
	case *ast.Ident:
		// Variable reference - can't resolve statically
		return "${" + v.Name + "}"
	case *ast.BinaryExpr:
		// String concatenation
		left := extractStringArg(v.X)
		right := extractStringArg(v.Y)
		if left != "" && right != "" {
			return left + right
		}
	}
	return ""
}

func extractHandlerName(expr ast.Expr) string {
	switch v := expr.(type) {
	case *ast.Ident:
		return v.Name
	case *ast.SelectorExpr:
		if x, ok := v.X.(*ast.Ident); ok {
			return x.Name + "." + v.Sel.Name
		}
		return v.Sel.Name
	case *ast.CallExpr:
		// Wrapped handler like gin.HandlerFunc(myHandler)
		return extractHandlerName(v.Fun)
	case *ast.FuncLit:
		return "<anonymous>"
	}
	return ""
}

// ExtractRoutesFromComments extracts routes from doc comments like swagger annotations.
func ExtractRoutesFromComments(comments string) []ctxpkg.Route {
	routes := make([]ctxpkg.Route, 0)

	// Match @Router annotations: @Router /path [method]
	routerPattern := regexp.MustCompile(`@Router\s+(\S+)\s+\[(\w+)\]`)
	matches := routerPattern.FindAllStringSubmatch(comments, -1)

	for _, match := range matches {
		if len(match) >= 3 {
			routes = append(routes, ctxpkg.Route{
				Path:   match[1],
				Method: strings.ToUpper(match[2]),
			})
		}
	}

	return routes
}
