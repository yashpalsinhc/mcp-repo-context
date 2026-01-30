package analyzer

import (
	"fmt"
	"go/ast"
	"go/token"
	"strings"

	ctxpkg "github.com/yashpalc/mcp-repo-context/internal/context"
)

// BehaviorExtractor extracts semantic behavior from function AST.
type BehaviorExtractor struct {
	fset *token.FileSet
}

// NewBehaviorExtractor creates a new behavior extractor.
func NewBehaviorExtractor(fset *token.FileSet) *BehaviorExtractor {
	return &BehaviorExtractor{fset: fset}
}

// Extract extracts behavior from a function declaration.
func (e *BehaviorExtractor) Extract(fn *ast.FuncDecl, imports []ctxpkg.Import) *ctxpkg.FunctionBehavior {
	if fn.Body == nil {
		return nil
	}

	behavior := &ctxpkg.FunctionBehavior{
		Steps:      make([]string, 0),
		InputUsage: make(map[string]string),
		Patterns:   make([]string, 0),
	}

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

	// Extract parameter names for tracking usage
	paramNames := make(map[string]bool)
	if fn.Type.Params != nil {
		for _, param := range fn.Type.Params.List {
			for _, name := range param.Names {
				paramNames[name.Name] = true
			}
		}
	}

	// Analyze the function body
	steps := e.extractSteps(fn.Body, importMap, paramNames)
	behavior.Steps = steps

	// Generate summary from steps
	behavior.Summary = e.generateSummary(fn.Name.Name, steps)

	// Extract input usage
	behavior.InputUsage = e.extractInputUsage(fn.Body, paramNames)

	// Extract return source
	behavior.OutputSource = e.extractOutputSource(fn.Body)

	// Detect patterns
	behavior.Patterns = e.detectPatterns(fn, steps)

	return behavior
}

// extractSteps extracts key operations from function body.
func (e *BehaviorExtractor) extractSteps(body *ast.BlockStmt, importMap map[string]string, params map[string]bool) []string {
	steps := make([]string, 0)
	stepNum := 1

	for _, stmt := range body.List {
		step := e.analyzeStatement(stmt, importMap, params, &stepNum)
		if step != "" {
			steps = append(steps, step)
		}
	}

	return steps
}

func (e *BehaviorExtractor) analyzeStatement(stmt ast.Stmt, importMap map[string]string, _ map[string]bool, stepNum *int) string {
	switch s := stmt.(type) {
	case *ast.AssignStmt:
		return e.analyzeAssignment(s, importMap, stepNum)

	case *ast.ExprStmt:
		return e.analyzeExprStmt(s, importMap, stepNum)

	case *ast.IfStmt:
		return e.analyzeIfStmt(s, stepNum)

	case *ast.ForStmt:
		return e.analyzeForStmt(s, stepNum)

	case *ast.RangeStmt:
		return e.analyzeRangeStmt(s, stepNum)

	case *ast.ReturnStmt:
		return e.analyzeReturnStmt(s, stepNum)

	case *ast.DeferStmt:
		return e.analyzeDeferStmt(s, importMap, stepNum)

	case *ast.GoStmt:
		return e.analyzeGoStmt(s, importMap, stepNum)

	case *ast.SwitchStmt:
		return e.analyzeSwitchStmt(s, stepNum)

	default:
		return ""
	}
}

func (e *BehaviorExtractor) analyzeAssignment(s *ast.AssignStmt, importMap map[string]string, stepNum *int) string {
	// Check if RHS is a function call
	for _, rhs := range s.Rhs {
		if call, ok := rhs.(*ast.CallExpr); ok {
			action := e.describeCall(call, importMap)
			if action != "" {
				step := fmt.Sprintf("%d. %s", *stepNum, action)
				*stepNum++
				return step
			}
		}
	}
	return ""
}

func (e *BehaviorExtractor) analyzeExprStmt(s *ast.ExprStmt, importMap map[string]string, stepNum *int) string {
	if call, ok := s.X.(*ast.CallExpr); ok {
		action := e.describeCall(call, importMap)
		if action != "" {
			step := fmt.Sprintf("%d. %s", *stepNum, action)
			*stepNum++
			return step
		}
	}
	return ""
}

func (e *BehaviorExtractor) analyzeIfStmt(s *ast.IfStmt, stepNum *int) string {
	// Check for error handling pattern
	if e.isErrorCheck(s.Cond) {
		return "" // Skip error checks, they're captured in ErrorHandling
	}

	// Check for nil check
	if e.isNilCheck(s.Cond) {
		step := fmt.Sprintf("%d. Validate input is not nil", *stepNum)
		*stepNum++
		return step
	}

	return ""
}

func (e *BehaviorExtractor) analyzeForStmt(_ *ast.ForStmt, stepNum *int) string {
	step := fmt.Sprintf("%d. Loop iteration", *stepNum)
	*stepNum++
	return step
}

func (e *BehaviorExtractor) analyzeRangeStmt(s *ast.RangeStmt, stepNum *int) string {
	var target string
	switch x := s.X.(type) {
	case *ast.Ident:
		target = x.Name
	default:
		target = "collection"
	}
	step := fmt.Sprintf("%d. Iterate over %s", *stepNum, target)
	*stepNum++
	return step
}

func (e *BehaviorExtractor) analyzeReturnStmt(ret *ast.ReturnStmt, stepNum *int) string {
	if len(ret.Results) == 0 {
		return ""
	}
	step := fmt.Sprintf("%d. Return result", *stepNum)
	*stepNum++
	return step
}

func (e *BehaviorExtractor) analyzeDeferStmt(s *ast.DeferStmt, importMap map[string]string, stepNum *int) string {
	action := e.describeCall(s.Call, importMap)
	if action != "" {
		step := fmt.Sprintf("%d. Defer: %s", *stepNum, action)
		*stepNum++
		return step
	}
	return ""
}

func (e *BehaviorExtractor) analyzeGoStmt(s *ast.GoStmt, importMap map[string]string, stepNum *int) string {
	action := e.describeCall(s.Call, importMap)
	if action != "" {
		step := fmt.Sprintf("%d. Spawn goroutine: %s", *stepNum, action)
		*stepNum++
		return step
	}
	return ""
}

func (e *BehaviorExtractor) analyzeSwitchStmt(_ *ast.SwitchStmt, stepNum *int) string {
	step := fmt.Sprintf("%d. Switch on condition", *stepNum)
	*stepNum++
	return step
}

func (e *BehaviorExtractor) describeCall(call *ast.CallExpr, importMap map[string]string) string {
	switch fn := call.Fun.(type) {
	case *ast.Ident:
		return e.describeSimpleCall(fn.Name)

	case *ast.SelectorExpr:
		if x, ok := fn.X.(*ast.Ident); ok {
			pkgName := x.Name
			funcName := fn.Sel.Name
			return e.describePackageCall(pkgName, funcName, importMap)
		}
		return ""

	default:
		return ""
	}
}

func (e *BehaviorExtractor) describeSimpleCall(funcName string) string {
	// Common function patterns
	lower := strings.ToLower(funcName)

	switch {
	case strings.HasPrefix(lower, "get"):
		return fmt.Sprintf("Get %s", camelToWords(funcName[3:]))
	case strings.HasPrefix(lower, "set"):
		return fmt.Sprintf("Set %s", camelToWords(funcName[3:]))
	case strings.HasPrefix(lower, "create"):
		return fmt.Sprintf("Create %s", camelToWords(funcName[6:]))
	case strings.HasPrefix(lower, "delete"):
		return fmt.Sprintf("Delete %s", camelToWords(funcName[6:]))
	case strings.HasPrefix(lower, "update"):
		return fmt.Sprintf("Update %s", camelToWords(funcName[6:]))
	case strings.HasPrefix(lower, "validate"):
		return fmt.Sprintf("Validate %s", camelToWords(funcName[8:]))
	case strings.HasPrefix(lower, "parse"):
		return fmt.Sprintf("Parse %s", camelToWords(funcName[5:]))
	case strings.HasPrefix(lower, "build"):
		return fmt.Sprintf("Build %s", camelToWords(funcName[5:]))
	case strings.HasPrefix(lower, "init"):
		return "Initialize"
	case strings.HasPrefix(lower, "new"):
		return fmt.Sprintf("Create new %s", camelToWords(funcName[3:]))
	case strings.HasPrefix(lower, "handle"):
		return fmt.Sprintf("Handle %s", camelToWords(funcName[6:]))
	case strings.HasPrefix(lower, "process"):
		return fmt.Sprintf("Process %s", camelToWords(funcName[7:]))
	case strings.HasPrefix(lower, "send"):
		return fmt.Sprintf("Send %s", camelToWords(funcName[4:]))
	case strings.HasPrefix(lower, "read"):
		return fmt.Sprintf("Read %s", camelToWords(funcName[4:]))
	case strings.HasPrefix(lower, "write"):
		return fmt.Sprintf("Write %s", camelToWords(funcName[5:]))
	case strings.HasPrefix(lower, "load"):
		return fmt.Sprintf("Load %s", camelToWords(funcName[4:]))
	case strings.HasPrefix(lower, "save"):
		return fmt.Sprintf("Save %s", camelToWords(funcName[4:]))
	case strings.HasPrefix(lower, "fetch"):
		return fmt.Sprintf("Fetch %s", camelToWords(funcName[5:]))
	case strings.HasPrefix(lower, "convert"):
		return fmt.Sprintf("Convert %s", camelToWords(funcName[7:]))
	case strings.HasPrefix(lower, "format"):
		return fmt.Sprintf("Format %s", camelToWords(funcName[6:]))
	case strings.HasPrefix(lower, "check"):
		return fmt.Sprintf("Check %s", camelToWords(funcName[5:]))
	case strings.HasPrefix(lower, "is"):
		return fmt.Sprintf("Check if %s", camelToWords(funcName[2:]))
	case strings.HasPrefix(lower, "has"):
		return fmt.Sprintf("Check has %s", camelToWords(funcName[3:]))
	case strings.HasPrefix(lower, "can"):
		return fmt.Sprintf("Check can %s", camelToWords(funcName[3:]))
	default:
		return fmt.Sprintf("Call %s", funcName)
	}
}

func (e *BehaviorExtractor) describePackageCall(pkgName, funcName string, importMap map[string]string) string {
	importPath := importMap[pkgName]

	// Standard library patterns
	switch pkgName {
	case "http":
		return e.describeHTTPCall(funcName)
	case "json":
		return e.describeJSONCall(funcName)
	case "fmt":
		return e.describeFmtCall(funcName)
	case "os":
		return e.describeOSCall(funcName)
	case "io":
		return e.describeIOCall(funcName)
	case "strings":
		return e.describeStringsCall(funcName)
	case "log":
		return e.describeLogCall(funcName)
	case "context":
		return e.describeContextCall(funcName)
	case "time":
		return e.describeTimeCall(funcName)
	case "sql", "db":
		return e.describeSQLCall(funcName)
	case "errors":
		return e.describeErrorsCall(funcName)
	}

	// Check import path for more context
	if strings.Contains(importPath, "database") || strings.Contains(importPath, "sql") {
		return e.describeSQLCall(funcName)
	}
	if strings.Contains(importPath, "redis") {
		return fmt.Sprintf("Redis: %s", funcName)
	}
	if strings.Contains(importPath, "kafka") {
		return fmt.Sprintf("Kafka: %s", funcName)
	}
	if strings.Contains(importPath, "grpc") {
		return fmt.Sprintf("gRPC: %s", funcName)
	}

	// Generic package.method description
	return fmt.Sprintf("%s.%s", pkgName, funcName)
}

func (e *BehaviorExtractor) describeHTTPCall(funcName string) string {
	switch funcName {
	case "Get":
		return "HTTP GET request"
	case "Post":
		return "HTTP POST request"
	case "NewRequest":
		return "Create HTTP request"
	case "ListenAndServe":
		return "Start HTTP server"
	case "HandleFunc":
		return "Register HTTP handler"
	case "Do":
		return "Execute HTTP request"
	default:
		return fmt.Sprintf("HTTP: %s", funcName)
	}
}

func (e *BehaviorExtractor) describeJSONCall(funcName string) string {
	switch funcName {
	case "Marshal":
		return "Encode to JSON"
	case "Unmarshal":
		return "Decode from JSON"
	case "NewEncoder":
		return "Create JSON encoder"
	case "NewDecoder":
		return "Create JSON decoder"
	case "Encode":
		return "Write JSON"
	case "Decode":
		return "Read JSON"
	default:
		return fmt.Sprintf("JSON: %s", funcName)
	}
}

func (e *BehaviorExtractor) describeFmtCall(funcName string) string {
	switch funcName {
	case "Printf", "Println", "Print":
		return "Print output"
	case "Sprintf":
		return "Format string"
	case "Errorf":
		return "Create formatted error"
	case "Fprintf":
		return "Write formatted output"
	default:
		return fmt.Sprintf("Format: %s", funcName)
	}
}

func (e *BehaviorExtractor) describeOSCall(funcName string) string {
	switch funcName {
	case "Open":
		return "Open file"
	case "Create":
		return "Create file"
	case "Remove":
		return "Delete file"
	case "Mkdir":
		return "Create directory"
	case "Getenv":
		return "Read environment variable"
	case "Exit":
		return "Exit program"
	case "ReadFile":
		return "Read file contents"
	case "WriteFile":
		return "Write file contents"
	default:
		return fmt.Sprintf("OS: %s", funcName)
	}
}

func (e *BehaviorExtractor) describeIOCall(funcName string) string {
	switch funcName {
	case "Copy":
		return "Copy data stream"
	case "ReadAll":
		return "Read all data"
	case "WriteString":
		return "Write string"
	default:
		return fmt.Sprintf("IO: %s", funcName)
	}
}

func (e *BehaviorExtractor) describeStringsCall(funcName string) string {
	switch funcName {
	case "Contains":
		return "Check string contains"
	case "Split":
		return "Split string"
	case "Join":
		return "Join strings"
	case "TrimSpace":
		return "Trim whitespace"
	case "ToLower":
		return "Convert to lowercase"
	case "ToUpper":
		return "Convert to uppercase"
	case "Replace":
		return "Replace in string"
	case "HasPrefix":
		return "Check string prefix"
	case "HasSuffix":
		return "Check string suffix"
	default:
		return fmt.Sprintf("String: %s", funcName)
	}
}

func (e *BehaviorExtractor) describeLogCall(funcName string) string {
	switch funcName {
	case "Printf", "Println", "Print":
		return "Log message"
	case "Fatal", "Fatalf":
		return "Log fatal error and exit"
	case "Panic", "Panicf":
		return "Log and panic"
	default:
		return fmt.Sprintf("Log: %s", funcName)
	}
}

func (e *BehaviorExtractor) describeContextCall(funcName string) string {
	switch funcName {
	case "Background":
		return "Create background context"
	case "WithCancel":
		return "Create cancellable context"
	case "WithTimeout":
		return "Create context with timeout"
	case "WithDeadline":
		return "Create context with deadline"
	case "WithValue":
		return "Create context with value"
	default:
		return fmt.Sprintf("Context: %s", funcName)
	}
}

func (e *BehaviorExtractor) describeTimeCall(funcName string) string {
	switch funcName {
	case "Now":
		return "Get current time"
	case "Sleep":
		return "Sleep/wait"
	case "Parse":
		return "Parse time"
	case "Since":
		return "Calculate duration"
	case "After":
		return "Wait for duration"
	default:
		return fmt.Sprintf("Time: %s", funcName)
	}
}

func (e *BehaviorExtractor) describeSQLCall(funcName string) string {
	switch funcName {
	case "Query", "QueryRow":
		return "Execute SQL query"
	case "Exec":
		return "Execute SQL command"
	case "Prepare":
		return "Prepare SQL statement"
	case "Begin":
		return "Begin transaction"
	case "Commit":
		return "Commit transaction"
	case "Rollback":
		return "Rollback transaction"
	case "Scan":
		return "Read query result"
	case "Close":
		return "Close database connection"
	default:
		return fmt.Sprintf("Database: %s", funcName)
	}
}

func (e *BehaviorExtractor) describeErrorsCall(funcName string) string {
	switch funcName {
	case "New":
		return "Create error"
	case "Is":
		return "Check error type"
	case "As":
		return "Cast error"
	case "Wrap":
		return "Wrap error with context"
	case "Unwrap":
		return "Unwrap error"
	default:
		return fmt.Sprintf("Error: %s", funcName)
	}
}

func (e *BehaviorExtractor) isErrorCheck(cond ast.Expr) bool {
	binExpr, ok := cond.(*ast.BinaryExpr)
	if !ok {
		return false
	}

	// Check for "err != nil" or "err == nil"
	if ident, ok := binExpr.X.(*ast.Ident); ok {
		if ident.Name == "err" {
			return true
		}
	}
	if ident, ok := binExpr.Y.(*ast.Ident); ok {
		if ident.Name == "err" {
			return true
		}
	}

	return false
}

func (e *BehaviorExtractor) isNilCheck(cond ast.Expr) bool {
	binExpr, ok := cond.(*ast.BinaryExpr)
	if !ok {
		return false
	}

	hasNil := false
	hasIdent := false

	if ident, ok := binExpr.X.(*ast.Ident); ok {
		if ident.Name == "nil" {
			hasNil = true
		} else {
			hasIdent = true
		}
	}
	if ident, ok := binExpr.Y.(*ast.Ident); ok {
		if ident.Name == "nil" {
			hasNil = true
		} else {
			hasIdent = true
		}
	}

	return hasNil && hasIdent
}

func (e *BehaviorExtractor) generateSummary(funcName string, steps []string) string {
	if len(steps) == 0 {
		return e.describeSimpleCall(funcName)
	}

	// Take first few meaningful steps
	var meaningful []string
	for _, step := range steps {
		// Remove step number
		parts := strings.SplitN(step, ". ", 2)
		if len(parts) == 2 {
			meaningful = append(meaningful, parts[1])
		}
		if len(meaningful) >= 3 {
			break
		}
	}

	if len(meaningful) == 0 {
		return e.describeSimpleCall(funcName)
	}

	return strings.Join(meaningful, ", then ")
}

func (e *BehaviorExtractor) extractInputUsage(body *ast.BlockStmt, params map[string]bool) map[string]string {
	usage := make(map[string]string)

	ast.Inspect(body, func(n ast.Node) bool {
		if ident, ok := n.(*ast.Ident); ok {
			if params[ident.Name] {
				// Find how this parameter is used
				parent := e.findParentCall(body, ident)
				if parent != "" && usage[ident.Name] == "" {
					usage[ident.Name] = parent
				}
			}
		}
		return true
	})

	return usage
}

func (e *BehaviorExtractor) findParentCall(body *ast.BlockStmt, target *ast.Ident) string {
	var result string

	ast.Inspect(body, func(n ast.Node) bool {
		if call, ok := n.(*ast.CallExpr); ok {
			for _, arg := range call.Args {
				if ident, ok := arg.(*ast.Ident); ok && ident.Name == target.Name {
					// Found the call that uses this identifier
					switch fn := call.Fun.(type) {
					case *ast.SelectorExpr:
						if x, ok := fn.X.(*ast.Ident); ok {
							result = fmt.Sprintf("Passed to %s.%s", x.Name, fn.Sel.Name)
						}
					case *ast.Ident:
						result = fmt.Sprintf("Passed to %s", fn.Name)
					}
					return false
				}
			}
		}
		return true
	})

	return result
}

func (e *BehaviorExtractor) extractOutputSource(body *ast.BlockStmt) string {
	var lastReturn string

	ast.Inspect(body, func(n ast.Node) bool {
		if ret, ok := n.(*ast.ReturnStmt); ok && len(ret.Results) > 0 {
			// Analyze the first return value
			switch r := ret.Results[0].(type) {
			case *ast.Ident:
				lastReturn = fmt.Sprintf("Returns %s", r.Name)
			case *ast.CallExpr:
				if fn, ok := r.Fun.(*ast.SelectorExpr); ok {
					if x, ok := fn.X.(*ast.Ident); ok {
						lastReturn = fmt.Sprintf("Returns result of %s.%s", x.Name, fn.Sel.Name)
					}
				} else if fn, ok := r.Fun.(*ast.Ident); ok {
					lastReturn = fmt.Sprintf("Returns result of %s", fn.Name)
				}
			case *ast.CompositeLit:
				if t, ok := r.Type.(*ast.Ident); ok {
					lastReturn = fmt.Sprintf("Returns new %s", t.Name)
				}
			}
		}
		return true
	})

	return lastReturn
}

func (e *BehaviorExtractor) detectPatterns(fn *ast.FuncDecl, steps []string) []string {
	patterns := make([]string, 0)
	name := strings.ToLower(fn.Name.Name)
	stepsStr := strings.ToLower(strings.Join(steps, " "))

	// CRUD patterns
	if strings.HasPrefix(name, "create") || strings.HasPrefix(name, "add") || strings.HasPrefix(name, "insert") {
		patterns = append(patterns, "create")
	}
	if strings.HasPrefix(name, "get") || strings.HasPrefix(name, "find") || strings.HasPrefix(name, "fetch") || strings.HasPrefix(name, "read") {
		patterns = append(patterns, "read")
	}
	if strings.HasPrefix(name, "update") || strings.HasPrefix(name, "modify") || strings.HasPrefix(name, "edit") {
		patterns = append(patterns, "update")
	}
	if strings.HasPrefix(name, "delete") || strings.HasPrefix(name, "remove") {
		patterns = append(patterns, "delete")
	}

	// Handler pattern
	if strings.HasPrefix(name, "handle") || strings.HasSuffix(name, "handler") {
		patterns = append(patterns, "handler")
	}

	// Validation pattern
	if strings.HasPrefix(name, "validate") || strings.HasPrefix(name, "check") || strings.HasPrefix(name, "is") {
		patterns = append(patterns, "validation")
	}

	// Factory pattern
	if strings.HasPrefix(name, "new") || strings.HasPrefix(name, "create") {
		if fn.Type.Results != nil && len(fn.Type.Results.List) > 0 {
			patterns = append(patterns, "factory")
		}
	}

	// Builder pattern
	if strings.Contains(stepsStr, "build") {
		patterns = append(patterns, "builder")
	}

	// HTTP pattern
	if strings.Contains(stepsStr, "http") {
		patterns = append(patterns, "http-client")
	}

	// Database pattern
	if strings.Contains(stepsStr, "database") || strings.Contains(stepsStr, "query") || strings.Contains(stepsStr, "sql") {
		patterns = append(patterns, "database")
	}

	// Async pattern
	if strings.Contains(stepsStr, "goroutine") {
		patterns = append(patterns, "async")
	}

	return patterns
}

// Helper function to convert CamelCase to words
func camelToWords(s string) string {
	if s == "" {
		return ""
	}

	var words []string
	var word strings.Builder

	for i, r := range s {
		if i > 0 && r >= 'A' && r <= 'Z' {
			if word.Len() > 0 {
				words = append(words, word.String())
				word.Reset()
			}
		}
		word.WriteRune(r)
	}
	if word.Len() > 0 {
		words = append(words, word.String())
	}

	return strings.ToLower(strings.Join(words, " "))
}
