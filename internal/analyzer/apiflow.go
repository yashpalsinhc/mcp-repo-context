package analyzer

import (
	"go/ast"
	"go/token"
	"regexp"
	"strings"

	ctxpkg "github.com/yashpalc/mcp-repo-context/internal/context"
)

// APIFlowExtractor extracts complete API flow information from functions.
type APIFlowExtractor struct {
	fset *token.FileSet
}

// NewAPIFlowExtractor creates a new API flow extractor.
func NewAPIFlowExtractor(fset *token.FileSet) *APIFlowExtractor {
	return &APIFlowExtractor{fset: fset}
}

// ExtractAPIFlow extracts complete API flow information from a function.
func (e *APIFlowExtractor) ExtractAPIFlow(fn *ast.FuncDecl, imports []ctxpkg.Import) *ctxpkg.APIFlow {
	flow := &ctxpkg.APIFlow{
		Steps:          []ctxpkg.FlowStep{},
		DBQueries:      []ctxpkg.DBQuery{},
		ExternalCalls:  []ctxpkg.ExternalCall{},
		ValidationSteps: []string{},
	}

	if fn.Body == nil {
		return flow
	}

	// Detect if this is an HTTP handler
	flow.IsHTTPHandler = e.isHTTPHandler(fn)
	if flow.IsHTTPHandler {
		flow.RequestPayload = e.extractRequestPayload(fn)
		flow.ResponsePayload = e.extractResponsePayload(fn)
	}

	// Extract flow steps
	stepNum := 1
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		switch stmt := n.(type) {
		case *ast.AssignStmt:
			step := e.analyzeAssignment(stmt, imports, stepNum)
			if step != nil {
				flow.Steps = append(flow.Steps, *step)
				stepNum++
			}

		case *ast.ExprStmt:
			step := e.analyzeExpression(stmt.X, imports, stepNum)
			if step != nil {
				flow.Steps = append(flow.Steps, *step)
				stepNum++
			}

		case *ast.IfStmt:
			// Check for validation patterns
			if validation := e.extractValidation(stmt); validation != "" {
				flow.ValidationSteps = append(flow.ValidationSteps, validation)
			}

		case *ast.CallExpr:
			// Extract DB queries
			if query := e.extractDBQuery(stmt, imports); query != nil {
				flow.DBQueries = append(flow.DBQueries, *query)
			}
			// Extract external HTTP calls
			if extCall := e.extractExternalCall(stmt, imports); extCall != nil {
				flow.ExternalCalls = append(flow.ExternalCalls, *extCall)
			}
		}
		return true
	})

	return flow
}

// isHTTPHandler checks if the function is an HTTP handler.
func (e *APIFlowExtractor) isHTTPHandler(fn *ast.FuncDecl) bool {
	if fn.Type.Params == nil || len(fn.Type.Params.List) == 0 {
		return false
	}

	params := fn.Type.Params.List

	// Check for gin.Context, echo.Context, fiber.Ctx patterns (single param handlers)
	for _, param := range params {
		paramType := e.typeToString(param.Type)
		if strings.Contains(paramType, "gin.Context") ||
			strings.Contains(paramType, "echo.Context") ||
			strings.Contains(paramType, "fiber.Ctx") {
			return true
		}
	}

	// Check for (w http.ResponseWriter, r *http.Request) pattern (needs 2+ params)
	if len(params) >= 2 {
		for i := 0; i < len(params)-1; i++ {
			param1Type := e.typeToString(params[i].Type)
			param2Type := e.typeToString(params[i+1].Type)

			if strings.Contains(param1Type, "ResponseWriter") &&
				strings.Contains(param2Type, "Request") {
				return true
			}
		}
	}

	return false
}

// extractRequestPayload extracts the request payload structure.
func (e *APIFlowExtractor) extractRequestPayload(fn *ast.FuncDecl) *ctxpkg.PayloadInfo {
	if fn.Body == nil {
		return nil
	}

	payload := &ctxpkg.PayloadInfo{
		Fields: []ctxpkg.FieldInfo{},
	}

	ast.Inspect(fn.Body, func(n ast.Node) bool {
		switch stmt := n.(type) {
		case *ast.CallExpr:
			// Look for json.Decode, c.Bind, c.ShouldBind patterns
			if sel, ok := stmt.Fun.(*ast.SelectorExpr); ok {
				methodName := sel.Sel.Name
				if methodName == "Decode" || methodName == "Bind" ||
					methodName == "ShouldBind" || methodName == "ShouldBindJSON" ||
					methodName == "BindJSON" || methodName == "ReadJSON" {
					// Try to find the target variable
					if len(stmt.Args) > 0 {
						if unary, ok := stmt.Args[0].(*ast.UnaryExpr); ok {
							if ident, ok := unary.X.(*ast.Ident); ok {
								payload.TypeName = ident.Name
								payload.Source = "json_body"
							}
						}
					}
				}
			}

		case *ast.AssignStmt:
			// Look for variable declarations that might be request structs
			for _, rhs := range stmt.Rhs {
				if comp, ok := rhs.(*ast.CompositeLit); ok {
					if ident, ok := comp.Type.(*ast.Ident); ok {
						name := strings.ToLower(ident.Name)
						if strings.Contains(name, "request") ||
							strings.Contains(name, "req") ||
							strings.Contains(name, "input") {
							payload.TypeName = ident.Name
						}
					}
				}
			}
		}
		return true
	})

	if payload.TypeName == "" {
		return nil
	}
	return payload
}

// extractResponsePayload extracts the response payload structure.
func (e *APIFlowExtractor) extractResponsePayload(fn *ast.FuncDecl) *ctxpkg.PayloadInfo {
	if fn.Body == nil {
		return nil
	}

	payload := &ctxpkg.PayloadInfo{
		Fields: []ctxpkg.FieldInfo{},
	}

	ast.Inspect(fn.Body, func(n ast.Node) bool {
		if call, ok := n.(*ast.CallExpr); ok {
			if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
				methodName := sel.Sel.Name
				// Look for json.Encode, c.JSON, w.Write patterns
				if methodName == "Encode" || methodName == "JSON" ||
					methodName == "WriteJSON" || methodName == "SendJSON" {
					// Try to find what's being encoded
					for _, arg := range call.Args {
						switch v := arg.(type) {
						case *ast.Ident:
							payload.TypeName = v.Name
						case *ast.CompositeLit:
							if ident, ok := v.Type.(*ast.Ident); ok {
								payload.TypeName = ident.Name
							}
						case *ast.CallExpr:
							// Could be a function returning the response
							if sel, ok := v.Fun.(*ast.SelectorExpr); ok {
								payload.TypeName = sel.Sel.Name + "()"
							}
						}
					}
				}
			}
		}
		return true
	})

	if payload.TypeName == "" {
		return nil
	}
	return payload
}

// analyzeAssignment analyzes an assignment statement for flow information.
func (e *APIFlowExtractor) analyzeAssignment(stmt *ast.AssignStmt, imports []ctxpkg.Import, stepNum int) *ctxpkg.FlowStep {
	step := &ctxpkg.FlowStep{
		StepNumber: stepNum,
		Line:       e.fset.Position(stmt.Pos()).Line,
	}

	// Get variable names being assigned
	var varNames []string
	for _, lhs := range stmt.Lhs {
		if ident, ok := lhs.(*ast.Ident); ok {
			varNames = append(varNames, ident.Name)
		}
	}

	// Analyze the RHS
	for _, rhs := range stmt.Rhs {
		switch v := rhs.(type) {
		case *ast.CallExpr:
			step.Action = e.describeCall(v, imports)
			step.Type = e.classifyCallType(v, imports)
			if len(varNames) > 0 {
				step.Output = strings.Join(varNames, ", ")
			}
			return step

		case *ast.CompositeLit:
			if ident, ok := v.Type.(*ast.Ident); ok {
				step.Action = "Create " + ident.Name + " struct"
				step.Type = "struct_creation"
				return step
			}
		}
	}

	return nil
}

// analyzeExpression analyzes an expression for flow information.
func (e *APIFlowExtractor) analyzeExpression(expr ast.Expr, imports []ctxpkg.Import, stepNum int) *ctxpkg.FlowStep {
	if call, ok := expr.(*ast.CallExpr); ok {
		return &ctxpkg.FlowStep{
			StepNumber: stepNum,
			Line:       e.fset.Position(expr.Pos()).Line,
			Action:     e.describeCall(call, imports),
			Type:       e.classifyCallType(call, imports),
		}
	}
	return nil
}

// describeCall creates a human-readable description of a function call.
func (e *APIFlowExtractor) describeCall(call *ast.CallExpr, _ []ctxpkg.Import) string {
	switch fn := call.Fun.(type) {
	case *ast.SelectorExpr:
		pkg := ""
		method := fn.Sel.Name
		if ident, ok := fn.X.(*ast.Ident); ok {
			pkg = ident.Name
		}

		// Describe based on known patterns
		return e.describeMethod(pkg, method, call.Args)

	case *ast.Ident:
		return "Call " + fn.Name + "()"
	}

	return "Function call"
}

// describeMethod creates a description for a method call.
func (e *APIFlowExtractor) describeMethod(pkg, method string, _ []ast.Expr) string {
	pkgLower := strings.ToLower(pkg)
	methodLower := strings.ToLower(method)

	// Database operations
	if pkgLower == "db" || strings.Contains(pkgLower, "repo") || strings.Contains(pkgLower, "store") {
		switch {
		case strings.HasPrefix(methodLower, "find") || strings.HasPrefix(methodLower, "get"):
			return "Query database: " + method
		case strings.HasPrefix(methodLower, "create") || strings.HasPrefix(methodLower, "insert"):
			return "Insert into database: " + method
		case strings.HasPrefix(methodLower, "update"):
			return "Update database: " + method
		case strings.HasPrefix(methodLower, "delete") || strings.HasPrefix(methodLower, "remove"):
			return "Delete from database: " + method
		case methodLower == "query" || methodLower == "queryrow":
			return "Execute SQL query"
		case methodLower == "exec":
			return "Execute SQL statement"
		}
	}

	// HTTP client operations
	if pkgLower == "http" || pkgLower == "client" {
		switch methodLower {
		case "get":
			return "HTTP GET request"
		case "post":
			return "HTTP POST request"
		case "put":
			return "HTTP PUT request"
		case "delete":
			return "HTTP DELETE request"
		case "do":
			return "Execute HTTP request"
		}
	}

	// JSON operations
	if pkgLower == "json" {
		switch methodLower {
		case "marshal":
			return "Serialize to JSON"
		case "unmarshal":
			return "Deserialize from JSON"
		case "encode":
			return "Encode response as JSON"
		case "decode":
			return "Decode JSON request body"
		}
	}

	// Context/framework operations
	if strings.Contains(pkgLower, "ctx") || pkgLower == "c" {
		switch {
		case methodLower == "json":
			return "Send JSON response"
		case methodLower == "bind" || methodLower == "shouldbind":
			return "Bind request body to struct"
		case methodLower == "param" || methodLower == "params":
			return "Get URL parameter"
		case methodLower == "query":
			return "Get query parameter"
		case methodLower == "set":
			return "Set context value"
		case methodLower == "get":
			return "Get context value"
		}
	}

	// Logging
	if pkgLower == "log" || pkgLower == "logger" || strings.Contains(pkgLower, "log") {
		return "Log: " + method
	}

	// Error handling
	if strings.Contains(methodLower, "error") || methodLower == "errorf" {
		return "Create/handle error"
	}

	// Generic
	return pkg + "." + method + "()"
}

// classifyCallType classifies the type of a function call.
func (e *APIFlowExtractor) classifyCallType(call *ast.CallExpr, _ []ctxpkg.Import) string {
	if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
		if ident, ok := sel.X.(*ast.Ident); ok {
			pkg := strings.ToLower(ident.Name)
			method := strings.ToLower(sel.Sel.Name)

			if pkg == "db" || strings.Contains(pkg, "repo") || strings.Contains(pkg, "store") {
				return "database"
			}
			if pkg == "http" || pkg == "client" {
				return "http_client"
			}
			if pkg == "json" {
				return "serialization"
			}
			if strings.Contains(pkg, "log") {
				return "logging"
			}
			if strings.Contains(method, "valid") {
				return "validation"
			}
			if pkg == "ctx" || pkg == "c" {
				return "context"
			}
		}
	}
	return "internal"
}

// extractValidation extracts validation logic from if statements.
func (e *APIFlowExtractor) extractValidation(stmt *ast.IfStmt) string {
	// Look for validation patterns in the condition
	var validation string
	ast.Inspect(stmt.Cond, func(n ast.Node) bool {
		switch v := n.(type) {
		case *ast.BinaryExpr:
			// Check for nil checks, empty checks, etc.
			if v.Op.String() == "==" || v.Op.String() == "!=" {
				if ident, ok := v.X.(*ast.Ident); ok {
					if basic, ok := v.Y.(*ast.BasicLit); ok {
						if basic.Value == "\"\"" {
							validation = "Validate " + ident.Name + " is not empty"
						} else if basic.Value == "nil" {
							validation = "Check " + ident.Name + " is not nil"
						} else if basic.Value == "0" {
							validation = "Validate " + ident.Name + " is not zero"
						}
					}
				}
			}
		case *ast.CallExpr:
			if sel, ok := v.Fun.(*ast.SelectorExpr); ok {
				if strings.Contains(strings.ToLower(sel.Sel.Name), "valid") {
					validation = "Run validation: " + sel.Sel.Name
				}
			}
		}
		return true
	})
	return validation
}

// extractDBQuery extracts database query information.
func (e *APIFlowExtractor) extractDBQuery(call *ast.CallExpr, imports []ctxpkg.Import) *ctxpkg.DBQuery {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return nil
	}

	method := sel.Sel.Name
	methodLower := strings.ToLower(method)

	// Check if this is a DB method
	isDBMethod := false
	if ident, ok := sel.X.(*ast.Ident); ok {
		pkg := strings.ToLower(ident.Name)
		isDBMethod = pkg == "db" || strings.Contains(pkg, "repo") ||
			strings.Contains(pkg, "store") || strings.Contains(pkg, "sql")
	}

	if !isDBMethod {
		// Check for chained calls like db.Model().Where().Find()
		if inner, ok := sel.X.(*ast.CallExpr); ok {
			return e.extractDBQuery(inner, imports)
		}
		return nil
	}

	query := &ctxpkg.DBQuery{
		Method: method,
		Line:   e.fset.Position(call.Pos()).Line,
	}

	// Determine operation type
	switch {
	case methodLower == "query" || methodLower == "queryrow" || methodLower == "select" ||
		strings.HasPrefix(methodLower, "find") || strings.HasPrefix(methodLower, "get"):
		query.Operation = "SELECT"
	case methodLower == "exec" || methodLower == "insert" || strings.HasPrefix(methodLower, "create"):
		query.Operation = "INSERT"
	case strings.HasPrefix(methodLower, "update"):
		query.Operation = "UPDATE"
	case strings.HasPrefix(methodLower, "delete") || strings.HasPrefix(methodLower, "remove"):
		query.Operation = "DELETE"
	default:
		query.Operation = "UNKNOWN"
	}

	// Try to extract the actual SQL query string
	for _, arg := range call.Args {
		if lit, ok := arg.(*ast.BasicLit); ok && lit.Kind == token.STRING {
			query.RawQuery = strings.Trim(lit.Value, "\"'`")
			// Extract table name from query
			query.Table = extractTableFromQuery(query.RawQuery)
			break
		}
	}

	return query
}

// extractTableFromQuery extracts the table name from a SQL query.
func extractTableFromQuery(query string) string {
	query = strings.ToUpper(query)

	// FROM table pattern
	fromRegex := regexp.MustCompile(`FROM\s+(\w+)`)
	if matches := fromRegex.FindStringSubmatch(query); len(matches) > 1 {
		return strings.ToLower(matches[1])
	}

	// INSERT INTO table pattern
	insertRegex := regexp.MustCompile(`INSERT\s+INTO\s+(\w+)`)
	if matches := insertRegex.FindStringSubmatch(query); len(matches) > 1 {
		return strings.ToLower(matches[1])
	}

	// UPDATE table pattern
	updateRegex := regexp.MustCompile(`UPDATE\s+(\w+)`)
	if matches := updateRegex.FindStringSubmatch(query); len(matches) > 1 {
		return strings.ToLower(matches[1])
	}

	// DELETE FROM table pattern
	deleteRegex := regexp.MustCompile(`DELETE\s+FROM\s+(\w+)`)
	if matches := deleteRegex.FindStringSubmatch(query); len(matches) > 1 {
		return strings.ToLower(matches[1])
	}

	return ""
}

// extractExternalCall extracts external HTTP call information.
func (e *APIFlowExtractor) extractExternalCall(call *ast.CallExpr, _ []ctxpkg.Import) *ctxpkg.ExternalCall {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return nil
	}

	method := sel.Sel.Name
	methodLower := strings.ToLower(method)

	// Check if this is an HTTP client call
	isHTTPCall := false
	if ident, ok := sel.X.(*ast.Ident); ok {
		pkg := strings.ToLower(ident.Name)
		isHTTPCall = pkg == "http" || pkg == "client" || strings.Contains(pkg, "client")
	}

	if !isHTTPCall {
		return nil
	}

	extCall := &ctxpkg.ExternalCall{
		Line: e.fset.Position(call.Pos()).Line,
	}

	// Determine HTTP method
	switch methodLower {
	case "get":
		extCall.Method = "GET"
	case "post":
		extCall.Method = "POST"
	case "put":
		extCall.Method = "PUT"
	case "delete":
		extCall.Method = "DELETE"
	case "patch":
		extCall.Method = "PATCH"
	case "do":
		extCall.Method = "DO" // Generic request
	default:
		return nil
	}

	// Try to extract URL
	for _, arg := range call.Args {
		if lit, ok := arg.(*ast.BasicLit); ok && lit.Kind == token.STRING {
			extCall.URL = strings.Trim(lit.Value, "\"'`")
			break
		}
	}

	return extCall
}

// typeToString converts an AST type to a string representation.
func (e *APIFlowExtractor) typeToString(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.SelectorExpr:
		if x, ok := t.X.(*ast.Ident); ok {
			return x.Name + "." + t.Sel.Name
		}
		return t.Sel.Name
	case *ast.StarExpr:
		return "*" + e.typeToString(t.X)
	case *ast.ArrayType:
		return "[]" + e.typeToString(t.Elt)
	default:
		return ""
	}
}

// ExtractRouteInfo extracts HTTP route information from common frameworks.
func ExtractRouteInfo(fn *ast.FuncDecl, fileContent string) *ctxpkg.RouteInfo {
	// Look for route registration patterns in comments or nearby code
	// This requires analyzing the file content for patterns like:
	// - r.GET("/users", HandleUsers)
	// - app.Post("/api/v1/users", handler.CreateUser)
	// - http.HandleFunc("/users", HandleUsers)

	info := &ctxpkg.RouteInfo{}

	// Extract from function name conventions
	fnName := fn.Name.Name
	fnNameLower := strings.ToLower(fnName)

	// Guess HTTP method from function name
	switch {
	case strings.HasPrefix(fnNameLower, "get") || strings.HasPrefix(fnNameLower, "list") ||
		strings.HasPrefix(fnNameLower, "fetch"):
		info.Method = "GET"
	case strings.HasPrefix(fnNameLower, "create") || strings.HasPrefix(fnNameLower, "post") ||
		strings.HasPrefix(fnNameLower, "add"):
		info.Method = "POST"
	case strings.HasPrefix(fnNameLower, "update") || strings.HasPrefix(fnNameLower, "put") ||
		strings.HasPrefix(fnNameLower, "edit"):
		info.Method = "PUT"
	case strings.HasPrefix(fnNameLower, "delete") || strings.HasPrefix(fnNameLower, "remove"):
		info.Method = "DELETE"
	case strings.HasPrefix(fnNameLower, "handle"):
		info.Method = "ANY"
	}

	return info
}
