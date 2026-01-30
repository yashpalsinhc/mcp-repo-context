package analyzer

import (
	"go/ast"
	"go/token"
	"strings"

	ctxpkg "github.com/yashpalc/mcp-repo-context/internal/context"
)

// ErrorHandlingExtractor extracts error handling patterns from functions.
type ErrorHandlingExtractor struct {
	fset *token.FileSet
}

// NewErrorHandlingExtractor creates a new error handling extractor.
func NewErrorHandlingExtractor(fset *token.FileSet) *ErrorHandlingExtractor {
	return &ErrorHandlingExtractor{fset: fset}
}

// Extract extracts error handling information from a function declaration.
func (e *ErrorHandlingExtractor) Extract(fn *ast.FuncDecl) *ctxpkg.ErrorHandling {
	if fn.Body == nil {
		return nil
	}

	eh := &ctxpkg.ErrorHandling{
		ErrorTypes: make([]string, 0),
	}

	// Check if function returns error
	if fn.Type.Results != nil {
		for _, result := range fn.Type.Results.List {
			if e.isErrorType(result.Type) {
				eh.ReturnsError = true
				break
			}
		}
	}

	// Analyze function body for error handling patterns
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.IfStmt:
			if e.isErrorCheck(node.Cond) {
				eh.ErrorChecks++
				e.analyzeErrorHandling(node, eh)
			}

		case *ast.CallExpr:
			// Check for panic calls
			if ident, ok := node.Fun.(*ast.Ident); ok {
				if ident.Name == "panic" {
					eh.PanicsOnError = true
				}
			}
			// Check for error wrapping (fmt.Errorf with %w)
			if sel, ok := node.Fun.(*ast.SelectorExpr); ok {
				if x, ok := sel.X.(*ast.Ident); ok {
					if x.Name == "fmt" && sel.Sel.Name == "Errorf" {
						// Check if format string contains %w
						if len(node.Args) > 0 {
							if lit, ok := node.Args[0].(*ast.BasicLit); ok {
								if strings.Contains(lit.Value, "%w") {
									eh.WrapsErrors = true
								}
							}
						}
					}
					// Check for errors.Wrap
					if x.Name == "errors" && (sel.Sel.Name == "Wrap" || sel.Sel.Name == "Wrapf") {
						eh.WrapsErrors = true
					}
				}
			}

			// Check for logging
			if sel, ok := node.Fun.(*ast.SelectorExpr); ok {
				if x, ok := sel.X.(*ast.Ident); ok {
					if x.Name == "log" || x.Name == "logger" || x.Name == "zap" || x.Name == "logrus" {
						eh.LogsErrors = true
					}
				}
			}
		}
		return true
	})

	// Determine if error is propagated (returned up)
	eh.PropagatesError = eh.ReturnsError && eh.ErrorChecks > 0

	return eh
}

func (e *ErrorHandlingExtractor) isErrorType(expr ast.Expr) bool {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name == "error"
	case *ast.InterfaceType:
		// Check if it's an empty interface or error interface
		return false
	}
	return false
}

func (e *ErrorHandlingExtractor) isErrorCheck(cond ast.Expr) bool {
	binExpr, ok := cond.(*ast.BinaryExpr)
	if !ok {
		return false
	}

	// Check for "err != nil" or "err == nil" or "error != nil"
	if e.isErrorIdent(binExpr.X) || e.isErrorIdent(binExpr.Y) {
		return true
	}

	return false
}

func (e *ErrorHandlingExtractor) isErrorIdent(expr ast.Expr) bool {
	ident, ok := expr.(*ast.Ident)
	if !ok {
		return false
	}
	name := strings.ToLower(ident.Name)
	return name == "err" || strings.HasSuffix(name, "err") || strings.HasSuffix(name, "error")
}

func (e *ErrorHandlingExtractor) analyzeErrorHandling(ifStmt *ast.IfStmt, eh *ctxpkg.ErrorHandling) {
	// Analyze what happens in the if body when error is detected
	if ifStmt.Body == nil {
		return
	}

	ast.Inspect(ifStmt.Body, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.ReturnStmt:
			// Error is returned/propagated
			eh.PropagatesError = true

			// Check what type of error is returned
			for _, result := range node.Results {
				errorType := e.extractErrorType(result)
				if errorType != "" && !contains(eh.ErrorTypes, errorType) {
					eh.ErrorTypes = append(eh.ErrorTypes, errorType)
				}
			}

		case *ast.ExprStmt:
			// Check for panic or log calls
			if call, ok := node.X.(*ast.CallExpr); ok {
				if ident, ok := call.Fun.(*ast.Ident); ok {
					if ident.Name == "panic" {
						eh.PanicsOnError = true
					}
				}
			}
		}
		return true
	})
}

func (e *ErrorHandlingExtractor) extractErrorType(expr ast.Expr) string {
	switch node := expr.(type) {
	case *ast.CallExpr:
		// Check for fmt.Errorf, errors.New, etc.
		if sel, ok := node.Fun.(*ast.SelectorExpr); ok {
			if x, ok := sel.X.(*ast.Ident); ok {
				if x.Name == "fmt" && sel.Sel.Name == "Errorf" {
					return "formatted_error"
				}
				if x.Name == "errors" && sel.Sel.Name == "New" {
					return "simple_error"
				}
				if x.Name == "errors" && (sel.Sel.Name == "Wrap" || sel.Sel.Name == "Wrapf") {
					return "wrapped_error"
				}
			}
		}
		// Check for custom error type constructors
		if ident, ok := node.Fun.(*ast.Ident); ok {
			if strings.HasPrefix(ident.Name, "New") && strings.Contains(strings.ToLower(ident.Name), "error") {
				return ident.Name
			}
		}

	case *ast.Ident:
		if node.Name == "err" {
			return "propagated_error"
		}

	case *ast.UnaryExpr:
		// &CustomError{}
		if node.Op == token.AND {
			if comp, ok := node.X.(*ast.CompositeLit); ok {
				if ident, ok := comp.Type.(*ast.Ident); ok {
					return ident.Name
				}
			}
		}
	}

	return ""
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// ExtractSideEffects detects side effects from a function declaration.
func ExtractSideEffects(fn *ast.FuncDecl, imports []ctxpkg.Import) []string {
	if fn.Body == nil {
		return nil
	}

	effects := make(map[string]bool)

	// Build import map
	importMap := make(map[string]string)
	for _, imp := range imports {
		alias := imp.Alias
		if alias == "" {
			parts := strings.Split(imp.Path, "/")
			alias = parts[len(parts)-1]
		}
		importMap[alias] = imp.Path
	}

	ast.Inspect(fn.Body, func(n ast.Node) bool {
		if call, ok := n.(*ast.CallExpr); ok {
			effect := detectSideEffect(call, importMap)
			if effect != "" {
				effects[effect] = true
			}
		}
		return true
	})

	result := make([]string, 0, len(effects))
	for effect := range effects {
		result = append(result, effect)
	}
	return result
}

func detectSideEffect(call *ast.CallExpr, importMap map[string]string) string {
	switch fn := call.Fun.(type) {
	case *ast.SelectorExpr:
		if x, ok := fn.X.(*ast.Ident); ok {
			pkgName := x.Name
			funcName := fn.Sel.Name
			importPath := importMap[pkgName]

			// HTTP calls
			if pkgName == "http" || strings.Contains(importPath, "http") {
				switch funcName {
				case "Get", "Post", "Put", "Delete", "Do", "NewRequest":
					return "http_call"
				}
			}

			// File I/O
			if pkgName == "os" || pkgName == "ioutil" || pkgName == "io" {
				switch funcName {
				case "Create", "Open", "OpenFile", "WriteFile", "ReadFile", "Remove", "Mkdir":
					return "file_io"
				case "Write", "WriteString", "Read", "ReadAll", "Copy":
					return "io_operation"
				}
			}

			// Database
			if pkgName == "db" || pkgName == "sql" || strings.Contains(importPath, "database") || strings.Contains(importPath, "sql") {
				switch funcName {
				case "Query", "QueryRow", "Exec", "Prepare":
					return "db_query"
				case "Begin", "Commit", "Rollback":
					return "db_transaction"
				}
			}

			// Logging
			if pkgName == "log" || pkgName == "logger" || strings.Contains(importPath, "log") {
				return "logging"
			}

			// Redis
			if strings.Contains(importPath, "redis") {
				return "redis_call"
			}

			// Kafka
			if strings.Contains(importPath, "kafka") {
				return "kafka_call"
			}

			// gRPC
			if strings.Contains(importPath, "grpc") {
				return "grpc_call"
			}

			// Context
			if pkgName == "context" {
				switch funcName {
				case "WithCancel", "WithTimeout", "WithDeadline":
					return "context_management"
				}
			}

			// Time
			if pkgName == "time" && funcName == "Sleep" {
				return "time_delay"
			}
		}

	case *ast.Ident:
		// Check for panic
		if fn.Name == "panic" {
			return "panic"
		}
		// Check for print functions
		if fn.Name == "print" || fn.Name == "println" {
			return "console_output"
		}
	}

	return ""
}
