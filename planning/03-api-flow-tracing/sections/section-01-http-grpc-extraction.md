Now I have all the context needed to write the section. Let me compose the implementation section.

# Section 01: Enhanced HTTP/gRPC Client Call Extraction

## Overview

This section enhances the Go static analyzer's ability to extract endpoint URLs and service hints from HTTP and gRPC client calls. Currently, `extractExternalCall()` in `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/analyzer/apiflow.go` only captures URLs from `*ast.BasicLit` string literals. This section adds support for file-scope constants, `fmt.Sprintf` patterns, string concatenation, `http.NewRequest` method extraction, and a heuristic for detecting HTTP client wrapper structs. It also adds gRPC client call detection.

This section has no dependencies on other sections and can be implemented in parallel with sections 02, 03, and 08.

## Background

### Current State

`ExternalCall` in `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/context/types.go` is:

```go
type ExternalCall struct {
    Method      string `json:"method"`
    URL         string `json:"url,omitempty"`
    Description string `json:"description,omitempty"`
    Line        int    `json:"line"`
}
```

The current `extractExternalCall()` in `apiflow.go` only detects calls where the receiver is `http`, `client`, or contains "client", and only extracts the URL when the argument is a `*ast.BasicLit` string. Any URL stored in a constant, built with `fmt.Sprintf`, or concatenated via `+` is silently dropped. The `Method` field is also never set for `http.NewRequest` calls.

`FunctionDef` in `types.go` has `APIFlow *APIFlow` which in turn has `ExternalCalls []ExternalCall`. These are serialized to JSON in the `functions` table. The new fields added to `ExternalCall` in this section will be backward-compatible: old stored data will deserialize with zero values for the new fields.

### What This Section Adds

1. **Two new fields on `ExternalCall`**: `URLExpression string` (original expression text when URL is dynamic) and `ServiceHint string` (guessed destination service name).

2. **Enhanced `extractExternalCall()`** with five new URL resolution strategies beyond `*ast.BasicLit`.

3. **A new `resolveURLExpr()` helper** on `APIFlowExtractor` that walks AST expressions to resolve URLs.

4. **A new `resolveFileConstant()` helper** that scans `*ast.GenDecl` nodes in the file for `const` blocks matching a given identifier.

5. **gRPC client detection** for `pb.NewXxxClient(conn).MethodName(ctx, req)` protobuf patterns.

6. **HTTP client wrapper heuristic** that flags method calls on struct fields whose names contain "client", "service", or "api".

## Files to Modify

- `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/context/types.go` — add `URLExpression` and `ServiceHint` fields to `ExternalCall`
- `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/analyzer/apiflow.go` — enhance `extractExternalCall()` and `ExtractAPIFlow()`

## Files to Extend (Tests)

- `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/analyzer/apiflow_test.go` — add new test cases

## Tests First

Add the following test cases to `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/analyzer/apiflow_test.go`. The existing test file already imports `go/ast`, `go/parser`, `go/token`, and `testing`, and defines `type funcDeclType = ast.FuncDecl`.

Each test follows the same pattern: parse a Go source snippet, run `ExtractAPIFlow`, assert on the resulting `ExternalCalls`.

```go
// TestExtractExternalCall_StringLiteral
// Verifies the existing baseline still works.
// Setup: http.Get("https://api.example.com/users")
// Assert: ExternalCall.URL == "https://api.example.com/users", Method == "GET"

// TestExtractExternalCall_FileConstant
// Setup: A file that declares const baseURL = "https://api.example.com"
//        and a function that calls http.Get(baseURL)
// Assert: ExternalCall.URL == "https://api.example.com"
// Note:   The extractor must receive the file's *ast.File (or its Decls slice)
//         so it can walk the constant declarations.

// TestExtractExternalCall_FmtSprintf
// Setup: http.Get(fmt.Sprintf("https://api.example.com/users/%d", id))
// Assert: ExternalCall.URL == "https://api.example.com/users/{param}"

// TestExtractExternalCall_StringConcatenation
// Setup: http.Get("https://api.example.com" + "/users/" + id)
// Assert: ExternalCall.URL contains "https://api.example.com/users/{dynamic}"

// TestExtractExternalCall_DynamicURL
// Setup: http.Get(getURL())
// Assert: ExternalCall.URL == "<dynamic>", URLExpression contains "getURL()"

// TestExtractExternalCall_NewRequest
// Setup: http.NewRequest("POST", "https://api.example.com/data", body)
// Assert: ExternalCall.Method == "POST", ExternalCall.URL == "https://api.example.com/data"

// TestExtractExternalCall_ServiceHintFromHostname
// Setup: http.Get("http://auth-service:8080/validate")
// Assert: ExternalCall.ServiceHint == "auth-service"

// TestHTTPClientWrapper_ServiceFieldDetection
// Setup: a function body containing s.authService.ValidateToken(ctx, token)
//        where authService is a struct field (not a top-level var)
// Assert: ExternalCalls contains an entry with ServiceHint == "authService"
//         and Method reflecting the heuristic (e.g. "gRPC" or "http")

// TestGRPCClientDetection_ProtobufPattern
// Setup: pb.NewUserServiceClient(conn).GetUser(ctx, req)
//        with import "somemodule/pb"
// Assert: ExternalCall with Method == "gRPC", URL == "/UserService/GetUser"
```

These tests should be written as table-driven sub-tests under `TestExtractExternalCall_Enhanced` or as individual top-level functions following the existing `TestAPIFlowExtractor_*` naming convention in the file.

## Implementation

### Step 1: Update `ExternalCall` struct in `types.go`

In `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/context/types.go`, extend the `ExternalCall` struct (currently at approximately line 254):

```go
// ExternalCall represents an external HTTP or gRPC call made from a function.
type ExternalCall struct {
    Method        string `json:"method"`                    // GET, POST, PUT, DELETE, PATCH, gRPC
    URL           string `json:"url,omitempty"`             // Extracted URL or template; "<dynamic>" when unresolvable
    URLExpression string `json:"url_expression,omitempty"` // Original expression text when URL is dynamic
    Description   string `json:"description,omitempty"`
    Line          int    `json:"line"`
    ServiceHint   string `json:"service_hint,omitempty"`   // Guessed destination service name
}
```

The two new fields are `URLExpression` and `ServiceHint`. These are omitempty so existing JSON blobs round-trip correctly.

### Step 2: Update `APIFlowExtractor` to carry file-level declarations

`extractExternalCall()` currently receives only a `*ast.CallExpr` and `[]ctxpkg.Import`. To resolve file-scope constants, the extractor needs access to the file's `[]ast.Decl`. Update `APIFlowExtractor` to hold file-level declarations and the file set:

```go
// APIFlowExtractor extracts complete API flow information from functions.
type APIFlowExtractor struct {
    fset     *token.FileSet
    fileDecls []ast.Decl // top-level declarations for constant resolution
}
```

Update `ExtractAPIFlow()` (or add a new entry point `ExtractAPIFlowFromFile(fn, file, imports)`) so that the file's `Decls` can be set before walking the function body.

Alternatively, add a `SetFileDecls(decls []ast.Decl)` method that the caller (the main `goAnalyzer.AnalyzeFile()` pipeline) calls once per file before extracting functions. The existing call in `AnalyzeFile()` is something like:

```go
flow := apiFlowExtractor.ExtractAPIFlow(fnDecl, imports)
```

This should become:

```go
apiFlowExtractor.SetFileDecls(file.Decls)
flow := apiFlowExtractor.ExtractAPIFlow(fnDecl, imports)
```

### Step 3: Implement `resolveFileConstant()`

Add a helper on `APIFlowExtractor` that searches the file's top-level `*ast.GenDecl` nodes for a `token.CONST` block containing an identifier with the given name:

```go
// resolveFileConstant resolves a file-scope constant by name.
// It walks fileDecls looking for const blocks and returns the string value
// of the named constant if found, or ("", false) if not.
func (e *APIFlowExtractor) resolveFileConstant(name string) (string, bool) {
    // Walk e.fileDecls looking for *ast.GenDecl with Tok == token.CONST.
    // For each spec (*ast.ValueSpec), if the spec name matches `name`
    // and the value is a *ast.BasicLit of token.STRING, return the
    // trimmed string value.
}
```

### Step 4: Implement `resolveURLExpr()`

Add a recursive helper that attempts to extract or construct a URL string from any `ast.Expr`:

```go
// resolveURLExpr attempts to resolve a URL from an AST expression.
// Returns (url, expression_text, resolved bool).
// When resolved is false, url is "<dynamic>" and expression_text is the
// best-effort string representation of the expression.
func (e *APIFlowExtractor) resolveURLExpr(expr ast.Expr) (url string, exprText string, resolved bool) {
    // Cases to handle:
    //
    // *ast.BasicLit (token.STRING):
    //   Return trimmed string value, resolved=true
    //
    // *ast.Ident:
    //   Attempt resolveFileConstant(ident.Name).
    //   If found, return resolved=true.
    //   Otherwise return "<dynamic>", ident.Name, resolved=false.
    //
    // *ast.CallExpr where Fun is fmt.Sprintf:
    //   Extract first arg (format string via BasicLit).
    //   Replace %s, %d, %v, %f, %t with "{param}".
    //   Return the resulting template, resolved=true.
    //
    // *ast.BinaryExpr with Op == token.ADD:
    //   Recursively resolve left and right.
    //   If left resolved and right resolved: concatenate.
    //   If left resolved and right not resolved: left + "{dynamic}"
    //   If neither resolved: "<dynamic>", exprText, resolved=false
    //
    // Default:
    //   Return "<dynamic>", best-effort expr string, resolved=false
}
```

### Step 5: Enhance `extractExternalCall()`

Replace the current URL extraction logic in `extractExternalCall()`:

Current code (lines 569–575 of `apiflow.go`):
```go
// Try to extract URL
for _, arg := range call.Args {
    if lit, ok := arg.(*ast.BasicLit); ok && lit.Kind == token.STRING {
        extCall.URL = strings.Trim(lit.Value, "\"'`")
        break
    }
}
```

New logic:

1. **`http.NewRequest` special case**: Before the existing method-switch, check if the call is `http.NewRequest(method, url, body)`. If so, extract the method from the first argument (BasicLit string) and the URL from the second argument via `resolveURLExpr`.

2. **Method detection for `http.Get`, `http.Post`, etc.**: Keep existing switch on `methodLower`.

3. **URL extraction via `resolveURLExpr`**: For calls with a URL argument, find the URL argument position (first arg for `http.Get`/`http.Post`, second arg for `http.NewRequest`) and call `resolveURLExpr`. Set `extCall.URL` from the result; when not resolved also set `extCall.URLExpression`.

4. **ServiceHint from URL**: After extracting the URL, call a new `extractServiceHint(url string) string` helper. If the URL has a hostname component (e.g., `http://auth-service:8080/...`), extract the hostname and strip port. Set `extCall.ServiceHint`.

5. **HTTP client wrapper heuristic**: Expand the detection logic. After the existing `isHTTPCall` check (which already covers struct fields containing "client"), for calls on struct fields whose `sel.X` is `*ast.SelectorExpr` (i.e., `s.authService.ValidateToken`), check if the intermediate field name (case-insensitive) contains "client", "service", or "api". If so, create an `ExternalCall` entry with `ServiceHint` set to the field name.

### Step 6: Add gRPC client detection

In `extractExternalCall()`, add a separate detection branch for gRPC patterns. The protobuf pattern is `pb.NewXxxClient(conn).MethodName(ctx, req)`:

- The outer call is `callExpr.Fun` of type `*ast.SelectorExpr` where `sel.X` is itself a `*ast.CallExpr`
- The inner call's `Fun` is `*ast.SelectorExpr` where `sel.Sel.Name` starts with `"New"` and ends with `"Client"`
- The import package should contain `"pb"`, `"proto"`, or a path ending in `/pb` or `/proto`

When matched:
- Extract the service name: strip `"New"` prefix and `"Client"` suffix from the constructor name (e.g., `NewUserServiceClient` → `UserService`)
- Extract the method name: `sel.Sel.Name` from the outer call (e.g., `GetUser`)
- Create `ExternalCall{Method: "gRPC", URL: "/"+serviceName+"/"+methodName}`

A helper function `extractServiceHint(url string) string` should parse the URL's hostname for use in `ServiceHint`. Use `net/url.Parse()` if available, or simple string splitting.

### Step 7: Caller site updates in `goAnalyzer.AnalyzeFile()`

Locate where `APIFlowExtractor.ExtractAPIFlow()` is called in the analyzer pipeline (likely in `internal/analyzer/goAnalyzer.go` or `go_analyzer.go`). Before each per-function call, ensure `SetFileDecls(file.Decls)` (or the equivalent mechanism from Step 2) is called so constant resolution has access to the full file.

## Known Limitations

- **Cross-file constants**: Constants defined in a separate file (e.g., `constants.go` in the same package) are not resolved. Only constants in the same file as the calling function are visible to the extractor. Cross-package constant resolution is a follow-up item.
- **HTTP client wrapper heuristic false positives**: Struct fields with names containing "client", "service", or "api" are flagged unconditionally. Custom wrapper names not following these patterns (e.g., `s.backend.Fetch()`) will be missed.
- **gRPC stored clients**: The pattern `s.userClient.GetUser()` (gRPC stub stored as a struct field) is partially handled by the wrapper heuristic but type-aware analysis is not performed. The ServiceHint will be "userClient" rather than the resolved service name.
- **`fmt.Sprintf` with non-format-string first arg**: If the format string is in a variable rather than a literal, it falls through to the `<dynamic>` case.

## Dependencies Required Before Section 04

Section 04 (Normalized Endpoint Storage) reads `ExternalCalls` from `FunctionDef.APIFlow` to populate the `service_calls` table. The two new fields (`URLExpression`, `ServiceHint`) on `ExternalCall` must be in place before Section 04 is implemented so they are available for storage. No other sections depend on this section.