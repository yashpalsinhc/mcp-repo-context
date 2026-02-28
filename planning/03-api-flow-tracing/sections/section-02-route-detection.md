Now I have all the context I need. Let me compose the section content:

# Section 02: Enhanced Route Registration Detection

## Overview

This section improves the existing `RouteExtractor` in `internal/analyzer/route_extractor.go` to handle gorilla/mux, Chi nested routing, Go 1.22+ patterns, and thorough path parameter normalization. It also consolidates the two duplicate `Route` structs and fixes an existing Gin/Echo bug in the extractor.

This section has no dependencies on other sections and can be implemented in parallel with sections 01, 03, and 08.

## Background

### Current State

Two `Route` structs currently exist:

1. `analyzer.Route` — defined in `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/analyzer/route_extractor.go`

```go
type Route struct {
    Method      string   `json:"method"`
    Path        string   `json:"path"`
    Handler     string   `json:"handler"`
    File        string   `json:"file"`
    Line        int      `json:"line"`
    Description string   `json:"description"`
    Middleware  []string `json:"middleware,omitempty"`
}
```

2. `context.Route` — defined in `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/context/types.go`

```go
type Route struct {
    Method      string   `json:"method"`
    Path        string   `json:"path"`
    Handler     string   `json:"handler"`
    Line        int      `json:"line"`
    Description string   `json:"description,omitempty"`
    Middleware  []string `json:"middleware,omitempty"`
}
```

The analyzer currently maps from `analyzer.Route` to `context.Route` manually in `go_analyzer.go` at lines 91–100:

```go
for _, r := range routes {
    fileCtx.Routes = append(fileCtx.Routes, ctxpkg.Route{
        Method:      r.Method,
        Path:        r.Path,
        Handler:     r.Handler,
        Line:        r.Line,
        Description: r.Description,
        Middleware:  r.Middleware,
    })
}
```

### Known Existing Bug

`route_extractor.go` has identical duplicate detection blocks for Gin/Echo (lines 57–62 and lines 63–68 are the same check). The second `isHTTPMethod` block is dead code because the first one will always match first. This means Echo-specific logic can never be reached separately. Fix this as part of this section.

### Current Framework Support

| Framework | Status |
|-----------|--------|
| Gin-style (r.GET, r.POST) | Supported (upper-case method names) |
| Echo-style (e.GET, e.POST) | Duplicate dead code - effectively same as Gin |
| Chi-style (r.Get, r.Post) | Supported (camelCase method names) |
| net/http HandleFunc | Supported (no method, ANY) |
| gorilla/mux HandleFunc + Methods() | NOT supported |
| Chi nested r.Route() | NOT supported |
| Go 1.22+ `"GET /path"` pattern | NOT supported |

## Goals

1. Add gorilla/mux `HandleFunc(...).Methods(...)` chaining detection
2. Add Chi nested `r.Route("/prefix", func(r chi.Router) { ... })` path concatenation
3. Parse Go 1.22+ `http.HandleFunc("GET /task/{id}", handler)` patterns
4. Normalize all path parameter syntax to `{param}`:
   - gorilla `{id:[0-9]+}` → `{id}` (strip regex constraint)
   - gin `:id` → `{id}`
   - gorilla `{id}` already correct
5. Consolidate `analyzer.Route` → delete it and use `context.Route` as the canonical type throughout
6. Fix the duplicate Gin/Echo detection bug

## Tests First

### File: `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/analyzer/route_extractor_test.go`

Extend (or create) this test file. All tests use the Go standard `testing` package. Parse test Go source as in-memory AST using `go/parser` and `go/token`, then call the extractor directly.

```go
// Test: detect gorilla/mux HandleFunc with Methods chain
// Setup: parse source containing r.HandleFunc("/users/{id}", handler).Methods("GET")
// Assert: Route with Path="/users/{id}", Method="GET", Framework="gorilla/mux"

// Test: detect gorilla/mux HandleFunc without Methods (ANY)
// Setup: r.HandleFunc("/health", handler)
// Assert: Route with Method="ANY", Framework="gorilla/mux"

// Test: path parameter normalization strips regex
// Setup: r.HandleFunc("/users/{id:[0-9]+}", handler)
// Assert: Route.Path == "/users/{id}", Route.RawPath == "/users/{id:[0-9]+}"

// Test: gin colon params normalized to braces
// Setup: r.GET("/users/:id", handler)
// Assert: Route.Path == "/users/{id}"

// Test: Go 1.22+ pattern parsing
// Setup: mux.HandleFunc("GET /tasks/{id}", handler)
// Assert: Route.Method == "GET", Route.Path == "/tasks/{id}"

// Test: chi nested routing path concatenation
// Setup: r.Route("/api", func(r chi.Router) { r.Get("/users", handler) })
// Assert: Route.Path == "/api/users"

// Test: duplicate Gin/Echo blocks fixed (existing bug)
// Assert: No duplicate routes registered for same path/handler call

// Test: Route struct consolidation — context.Route has all fields
// Assert: context.Route includes RawPath, HandlerFile, Framework fields
```

## Implementation Details

### Step 1: Extend `context.Route` (Canonical Type)

File: `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/context/types.go`

Replace the existing `context.Route` struct with the expanded canonical version:

```go
// Route represents an HTTP route/endpoint.
type Route struct {
    Method      string   `json:"method"`
    Path        string   `json:"path"`        // normalized path with {param} placeholders
    RawPath     string   `json:"raw_path,omitempty"` // original path as written in code
    Handler     string   `json:"handler"`
    HandlerFile string   `json:"handler_file,omitempty"` // file containing the handler (if determinable)
    File        string   `json:"file,omitempty"`         // file containing the route registration
    Line        int      `json:"line"`
    Framework   string   `json:"framework,omitempty"` // "gorilla/mux", "chi", "net/http", "gin", "echo"
    Description string   `json:"description,omitempty"`
    Middleware  []string `json:"middleware,omitempty"`
}
```

The `File` field is new to `context.Route` (previously only in `analyzer.Route`). The `RawPath`, `HandlerFile`, and `Framework` fields are entirely new.

### Step 2: Delete `analyzer.Route` and Update `RouteExtractor` Return Type

File: `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/analyzer/route_extractor.go`

- Remove the `analyzer.Route` struct definition entirely (lines 11–19)
- Update `RouteExtractor.routes` field from `[]Route` to `[]ctxpkg.Route` (importing `ctxpkg "github.com/yashpalc/mcp-repo-context/internal/context"`)
- Update all method signatures that reference `Route` to use `ctxpkg.Route`

Since `analyzer.Route` is removed, the manual mapping loop in `go_analyzer.go` (lines 91–100) simplifies — routes can be appended directly to `fileCtx.Routes` without struct conversion.

### Step 3: Fix the Duplicate Gin/Echo Bug

File: `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/analyzer/route_extractor.go`

In the `visit` method, replace the two identical `isHTTPMethod` blocks (lines 57–67) with a single combined block that detects both Gin and Echo. Echo and Gin share the same call pattern (uppercase method name), so a single block is correct. The old second block at lines 63–68 should be deleted.

The corrected visit method section should look like:

```go
// Gin/Echo-style routes: r.GET, r.POST, e.GET, etc.
if isHTTPMethod(method) {
    e.extractGinEchoRoute(call, method)
    return true
}

// Chi-style: r.Get, r.Post, etc.
if isHTTPMethodCamelCase(method) {
    e.extractChiRoute(call, method)
    return true
}
```

Rename `extractGinRoute` to `extractGinEchoRoute` to accurately reflect it handles both.

### Step 4: Add Gorilla/mux Detection

The gorilla/mux pattern is:
```go
r.HandleFunc("/users/{id}", handler).Methods("GET", "POST")
```

This is an outer `CallExpr` (the `.Methods(...)` call) wrapping an inner `CallExpr` (`r.HandleFunc(...)`). The current code already has a branch for `HandleFunc` in the visit method, but it only handles the plain `http.HandleFunc` pattern, not gorilla's chained version.

Changes needed in `route_extractor.go`:

1. Modify the `HandleFunc` branch in `visit` to detect whether the `HandleFunc` call is itself the receiver of a further `.Methods()` chain. This requires inspecting the parent call context — the AST `Inspect` traversal visits child before parent, so track pending routes as "awaiting Methods".

Alternative approach (simpler): Use a two-pass strategy. On the first pass, collect all `HandleFunc` calls with path and handler. On a second `ast.Inspect`, look for `Methods` calls whose receiver matches a `HandleFunc` call AST node. Match by position (the `Pos()` of the inner call).

The simplest correct approach: store `HandleFunc` routes in a pending map keyed by their `Pos()`. In the same traversal, when a `Methods` call is seen, check if its receiver expression is a `CallExpr` whose `Pos()` is in the pending map. If so, update that route's method and set framework to `"gorilla/mux"`. After traversal, flush all pending routes (those without a Methods call default to `Method="ANY"`).

```go
// RouteExtractor fields to add:
type RouteExtractor struct {
    fset         *token.FileSet
    routes       []ctxpkg.Route
    file         string
    pendingRoutes map[token.Pos]*ctxpkg.Route  // routes awaiting Methods() chaining
}
```

The `extractGorillaMuxRoute` helper should:
- Extract path from first arg (call `normalizePathParams`)
- Extract handler from second arg
- Store `RawPath` as the raw extracted string before normalization
- Set `Framework = "gorilla/mux"`
- Register in `pendingRoutes`

When a `Methods(...)` call is detected (selector method name is `"Methods"`), walk its receiver to find the underlying `HandleFunc` call, look it up in `pendingRoutes`, and set the `Method` field.

### Step 5: Path Parameter Normalization

Add a `normalizePathParams(path string) string` function to `route_extractor.go`:

The function should:
1. Replace `{name:[^}]+}` (gorilla regex constraints) with `{name}` — strip everything after the `:` inside braces
2. Replace `:name` segments (gin-style colon params) with `{name}` — a path segment starting with `:` that is not `://`
3. Leave `{name}` (chi/gorilla without regex) unchanged

The caller stores both the raw path and the normalized path:
- `RawPath` = original string extracted from AST
- `Path` = `normalizePathParams(RawPath)`

Normalization should be applied in all framework extraction helpers (`extractGinEchoRoute`, `extractChiRoute`, `extractHTTPRoute`, and the new gorilla/mux helper).

### Step 6: Add Chi Nested Routing

Chi nested routing:
```go
r.Route("/api", func(r chi.Router) {
    r.Get("/users", handler)
})
```

The `r.Route(prefix, fn)` call takes a path prefix and an inline function. Routes registered inside that function should inherit the prefix. This requires tracking prefix scope during AST traversal.

Implementation approach:

Add a `prefixStack []string` field to `RouteExtractor`. In the `visit` method, when a `Route` method is seen (i.e., `sel.Sel.Name == "Route"`) and it has at least 2 args where the second is a `*ast.FuncLit`:

1. Extract the prefix string from the first argument
2. Push the prefix onto `prefixStack`
3. Recurse: call `ast.Inspect` on the function literal body with the extractor's visitor
4. Pop the prefix from `prefixStack`

When appending routes from any extraction helper, prepend the current joined prefix from `prefixStack`.

Add a helper `currentPrefix() string` that returns `strings.Join(e.prefixStack, "")`.

### Step 7: Add Go 1.22+ Pattern Parsing

Go 1.22 net/http allows:
```go
mux.HandleFunc("GET /tasks/{id}", handler)
```

In `extractHTTPRoute`, after extracting the path string, check whether it begins with an HTTP method token followed by a space. If so:
1. Split on the first space
2. First part becomes `Route.Method`
3. Second part becomes the path (run through `normalizePathParams`)
4. Set `Framework = "net/http"`

The check: `strings.HasPrefix(path, "GET ") || strings.HasPrefix(path, "POST ")` etc. (or use `isHTTPMethodPrefix`).

### Step 8: Update `go_analyzer.go` Route Mapping

File: `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/analyzer/go_analyzer.go`

The manual mapping loop (lines 88–100) can be simplified since `RouteExtractor.Extract` now returns `[]ctxpkg.Route` directly:

```go
// Extract HTTP routes
routeExtractor := NewRouteExtractor(fset, file.Path)
fileCtx.Routes = routeExtractor.Extract(f)
```

This replaces the previous loop entirely.

## File Summary

| File | Action | Description |
|------|--------|-------------|
| `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/context/types.go` | Modify | Extend `Route` struct with `RawPath`, `HandlerFile`, `File`, `Framework` fields |
| `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/analyzer/route_extractor.go` | Modify | Remove `analyzer.Route`, use `ctxpkg.Route`, add gorilla/mux detection, chi nesting, Go 1.22+ patterns, param normalization, fix duplicate bug |
| `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/analyzer/go_analyzer.go` | Modify | Simplify route extraction loop since types are now unified |
| `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/analyzer/route_extractor_test.go` | Create/Extend | Tests for all new detection patterns |

## Dependencies

This section has no dependencies on other sections. It runs in parallel with:
- Section 01 (HTTP/gRPC client extraction)
- Section 03 (Kafka extraction)
- Section 08 (Multi-language detection)

Sections that depend on this section completing first:
- **Section 04** (Endpoint Storage) — requires the expanded `context.Route` struct with `Framework`, `RawPath`, `File` fields to populate the `endpoints` SQL table

## Implementation Checklist

1. Extend `context.Route` with `RawPath`, `HandlerFile`, `File`, `Framework` fields
2. Remove `analyzer.Route` struct from `route_extractor.go`
3. Add `ctxpkg` import to `route_extractor.go`
4. Update `RouteExtractor` struct: change `routes []Route` to `routes []ctxpkg.Route`, add `pendingRoutes map[token.Pos]*ctxpkg.Route`, add `prefixStack []string`
5. Update `NewRouteExtractor` constructor to initialize new fields
6. Update `Extract` return type to `[]ctxpkg.Route`
7. Fix duplicate Gin/Echo visit blocks (delete lines 63–68, rename `extractGinRoute` → `extractGinEchoRoute`)
8. Add `normalizePathParams(path string) string` function
9. Update all existing extract helpers to use `ctxpkg.Route` and set `RawPath` and normalized `Path`
10. Add gorilla/mux `HandleFunc`+`Methods` detection with `pendingRoutes` approach
11. Add chi `r.Route("/prefix", func(r chi.Router) {...})` nested routing via `prefixStack`
12. Add Go 1.22+ `"METHOD /path"` parsing in `extractHTTPRoute`
13. Simplify `go_analyzer.go` route extraction loop
14. Write tests for all new behaviors