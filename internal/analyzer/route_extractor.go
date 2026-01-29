package analyzer

import (
	"go/ast"
	"go/token"
	"regexp"
	"strings"
)

// Route represents an HTTP route/endpoint.
type Route struct {
	Method      string `json:"method"`       // GET, POST, PUT, DELETE, etc.
	Path        string `json:"path"`         // Route path like /api/v1/users
	Handler     string `json:"handler"`      // Handler function name
	File        string `json:"file"`         // Source file
	Line        int    `json:"line"`         // Line number
	Description string `json:"description"`  // Doc comment if available
	Middleware  []string `json:"middleware,omitempty"` // Middleware chain
}

// RouteExtractor extracts HTTP routes from Go source code.
type RouteExtractor struct {
	fset   *token.FileSet
	routes []Route
	file   string
}

// NewRouteExtractor creates a new route extractor.
func NewRouteExtractor(fset *token.FileSet, file string) *RouteExtractor {
	return &RouteExtractor{
		fset:   fset,
		routes: make([]Route, 0),
		file:   file,
	}
}

// Extract extracts routes from an AST.
func (e *RouteExtractor) Extract(node ast.Node) []Route {
	ast.Inspect(node, e.visit)
	return e.routes
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

	// Gin-style routes: r.GET, r.POST, etc.
	if isHTTPMethod(method) {
		e.extractGinRoute(call, method)
		return true
	}

	// Echo-style: e.GET, e.POST, etc.
	if isHTTPMethod(method) {
		e.extractGinRoute(call, method) // Same pattern as Gin
		return true
	}

	// Chi-style: r.Get, r.Post, etc.
	if isHTTPMethodCamelCase(method) {
		e.extractChiRoute(call, method)
		return true
	}

	// Standard http.HandleFunc
	if method == "HandleFunc" || method == "Handle" {
		e.extractHTTPRoute(call, method)
		return true
	}

	// Gin group: r.Group
	if method == "Group" {
		// Groups are harder to track, skip for now
		return true
	}

	return true
}

func (e *RouteExtractor) extractGinRoute(call *ast.CallExpr, method string) {
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
	e.routes = append(e.routes, Route{
		Method:     strings.ToUpper(method),
		Path:       path,
		Handler:    handler,
		File:       e.file,
		Line:       pos.Line,
		Middleware: middleware,
	})
}

func (e *RouteExtractor) extractChiRoute(call *ast.CallExpr, method string) {
	if len(call.Args) < 2 {
		return
	}

	path := extractStringArg(call.Args[0])
	handler := extractHandlerName(call.Args[1])

	if path == "" {
		return
	}

	pos := e.fset.Position(call.Pos())
	e.routes = append(e.routes, Route{
		Method:  strings.ToUpper(method),
		Path:    path,
		Handler: handler,
		File:    e.file,
		Line:    pos.Line,
	})
}

func (e *RouteExtractor) extractHTTPRoute(call *ast.CallExpr, _ string) {
	if len(call.Args) < 2 {
		return
	}

	path := extractStringArg(call.Args[0])
	handler := extractHandlerName(call.Args[1])

	if path == "" {
		return
	}

	pos := e.fset.Position(call.Pos())
	e.routes = append(e.routes, Route{
		Method:  "ANY", // http.HandleFunc doesn't specify method
		Path:    path,
		Handler: handler,
		File:    e.file,
		Line:    pos.Line,
	})
}

func isHTTPMethod(name string) bool {
	methods := map[string]bool{
		"GET": true, "POST": true, "PUT": true, "DELETE": true,
		"PATCH": true, "HEAD": true, "OPTIONS": true, "ANY": true,
	}
	return methods[strings.ToUpper(name)]
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
func ExtractRoutesFromComments(comments string) []Route {
	routes := make([]Route, 0)

	// Match @Router annotations: @Router /path [method]
	routerPattern := regexp.MustCompile(`@Router\s+(\S+)\s+\[(\w+)\]`)
	matches := routerPattern.FindAllStringSubmatch(comments, -1)

	for _, match := range matches {
		if len(match) >= 3 {
			routes = append(routes, Route{
				Path:   match[1],
				Method: strings.ToUpper(match[2]),
			})
		}
	}

	return routes
}
