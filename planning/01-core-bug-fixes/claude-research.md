# Research Findings: Core Bug Fixes

## Codebase Research

### 1. Comparison Logic (internal/comparison/comparer.go)

#### normalizeFunctionKey() Bug (Line 481-484)
**Current implementation returns ONLY `fn.Name`**, completely ignoring receiver type, parameters, and package. The `FunctionDef` struct has a `Receiver` field (types.go:178) that is available but unused.

**Impact on duplicates (lines 128-192):** All functions with the same name across repos are treated as duplicates regardless of receiver type or signature. `(*Router).ServeHTTP` and `(*cors).ServeHTTP` become "duplicates."

**Impact on conflicts (lines 195-286):** Uses `fn.Name` at line 208 for key lookup, then does string equality on signatures at line 220. No receiver-type awareness. Severity assessment only checks for error return type (line 237).

**Impact on gaps (lines 289-365):** Uses `targetFuncs[fn.Name] = true` at line 304. Boolean map with name-only keys. Priority ranking (lines 322-334) works correctly but underlying detection is flawed.

#### Data Structures

```go
// FunctionDef (context/types.go:172-192)
type FunctionDef struct {
    Name        string
    Signature   string
    Receiver    string   // Available but NOT used in comparison
    Parameters  []Param
    Returns     []string
    IsPublic    bool
    // ... plus deep context fields
}

// CallRef (context/types.go:204-210)
type CallRef struct {
    Function string  // Name only
    Package  string  // OVERLOADED: package path OR receiver type
    File     string
    Line     int
    Type     string  // "internal", "stdlib", "external", "method"
}
```

**Critical issue:** `CallRef.Package` field is overloaded — stores both package paths and receiver type names with no way to distinguish.

### 2. Smart Query NLP (internal/orchestrator/smart_query.go)

**Pattern matching:** Uses `regexp.MustCompile()` with sequential pattern testing (lines 125-139).

**Confidence scoring:** Hardcoded to 0.8 for all query types (line 78). Only adjusts to:
- 0.5 when function not found (line 339)
- 0.3 for general query with no results (line 1174)

**Fallback:** `handleGeneralQuery()` (line 1137-1178) uses word matching on function names/summaries. Sets `NeedsAI = true` only when confidence is low (line 1168).

**Package query matching (lines 911-916):** Uses naive `strings.Contains(path, packagePath)` — no boundary checking. Package `http` would match `https` (false positive).

### 3. Pattern/Chain Execution (internal/compose/)

**Chain execution (chain.go:156-206):** Steps execute sequentially. Conditional steps skip silently (line 180-181) with no output.

**SearchWithContext pattern (patterns.go:91-159):** Two-step. Step 2 conditionally extracts first result with `r[0]` access. Assumes specific result structure (lines 144-150). Fails silently if different format.

**ImpactAnalysis pattern (patterns.go:226-285):** Three steps, all unconditional. Step 1 calls `get_function_context` but requires `file_path` parameter — no preceding search step to resolve it.

**Error handling (chain.go:199-202):** Breaks immediately on first error. No retry, no conditional error handling.

### 4. Call Graph / Callee Extraction (internal/analyzer/callgraph.go)

**Function call extraction (lines 99-162):** Handles direct calls (line 122), goroutine calls (131-145), deferred calls (146-156), and method calls via SelectorExpr.

**Method vs package detection (lines 176-194):**
```go
case *ast.SelectorExpr:
    if importPath, ok := importMap[pkgName]; ok {
        callType = b.classifyCallType(importPath)
    } else {
        callType = "method"  // Heuristic: if not in imports, it's a method
    }
```
**Fails when:** variable name matches import alias, or vice versa.

**Call resolution (lines 263-275):** Only resolves `Type == "internal"` or empty package. Method calls have `Type == "method"` (set at line 188) and are **never resolved**.

**Caller population (lines 316-347):** Only processes `Type == "internal"`. Method calls (`Type == "method"`) are filtered out (line 324), so methods never have `CalledBy` populated.

### 5. Package Structure Grouping (smart_query.go:933-950)

Groups by path segments (first two path components), not by file purpose. For flat packages, this creates odd groupings by file extension.

### 6. Testing Setup

- Standard Go `testing` package, no external frameworks
- Individual test functions (not table-driven)
- Inline fixture construction with hardcoded values
- Key test files: `comparer_test.go` (11 tests), `go_analyzer_test.go`, `callgraph_test.go`
- Run with `go test ./...`
- **Missing tests:** normalizeFunctionKey() logic, method vs function distinction in call graph, confidence scoring adjustments, package path boundary checking

---

## Web Research

### Go AST Call Extraction Best Practices

**Key packages:**
| Package | Purpose |
|---------|---------|
| `go/ast` | Core AST node types (CallExpr, SelectorExpr) |
| `go/types` | Type info and symbol resolution — **critical for method vs package** |
| `golang.org/x/tools/go/analysis` | Framework for building static analyzers |
| `golang.org/x/tools/go/ast/inspector` | Efficient AST node filtering |

**Approach for accurate call detection:**
1. Direct calls (`foo()`): `ast.CallExpr` with `ast.Ident` as `Fun`
2. Method/package calls (`x.Method()`): `ast.SelectorExpr` — use `go/types.Info` to distinguish
3. Function variable calls: check if `Ident` resolves to function type via `go/types`
4. Interface methods: requires `go/types.Checker` runtime info

**Key pitfall:** Without `go/types`, you cannot reliably distinguish `package.Function()` from `receiver.Method()`. The current codebase uses a heuristic (check import map) instead.

**Recommendation:** The current project does NOT use `go/types` (only `go/ast`). Adding `go/types` would require running the type checker, which needs module resolution. For now, improving the heuristic is more pragmatic: check if the receiver variable is declared locally, track method calls by variable type where possible.

### Go Text Stemming / NLP Libraries

**Recommended libraries:**
- `github.com/kljensen/snowball` — Snowball stemmer, 7 languages, single function API
- `github.com/reiver/go-porterstemmer` — Porter stemmer, simpler, English-only
- `github.com/MarkusZoppelt/fuzzymatch` — Levenshtein distance + closest match

**For smart_query improvement:**
- Use Snowball stemmer for query word normalization (routing→rout, handlers→handler)
- Use fuzzy match as fallback when exact match fails
- Build confidence scoring with weighted pattern matching instead of hardcoded values

### Domain-Aware Comparison Heuristics

**Key insight from tools like Sourcegraph:** Use semantic analysis (unique symbol identifiers) rather than name matching. For the MCP server, this means including receiver type + package in function keys.

**For gap analysis:** The concept of "package responsibility scope" is well-established:
- Backstage uses service catalog with domain ownership declarations
- CODEOWNERS maps files to teams
- ArgoCD ApplicationSet defines service deployments

### User-Defined Architecture Configuration

**Recommended format (Backstage-inspired YAML):**
```yaml
# .mcp/architecture.yaml
version: "1.0"
services:
  auth-service:
    description: "Authentication and authorization"
    domains: ["auth", "security"]
    responsibilities: ["User authentication", "Token management"]
    dependencies:
      - service: user-service
        type: "uses"
    apis:
      - path: "/auth/login"
        type: "http"
    repositories:
      - "https://github.com/org/auth-service"

domains:
  auth:
    label: "Authentication & Security"
    keywords: ["auth", "login", "token", "oauth", "jwt"]
    services: [auth-service]
```

**Benefits for gap analysis:** When comparing repos, consult the architecture definition to determine if repos are complementary (different domains) or overlapping (same domain). Only flag gaps within the same domain scope.

**Note:** This architecture configuration feature is OUT OF SCOPE for 01-core-bug-fixes. It belongs in a later split (possibly 02-dependency-graph or a new split). For now, gap analysis should use simpler heuristics (concept similarity, package name matching).

---

## Sources

- [go/ast package docs](https://pkg.go.dev/go/ast)
- [golang.org/x/tools/go/analysis](https://pkg.go.dev/golang.org/x/tools/go/analysis)
- [kljensen/snowball stemmer](https://github.com/kljensen/snowball)
- [MarkusZoppelt/fuzzymatch](https://github.com/MarkusZoppelt/fuzzymatch)
- [Backstage Descriptor Format](https://backstage.io/docs/features/software-catalog/descriptor-format/)
- [Sourcegraph Cross-Repository Navigation](https://sourcegraph.com/blog/cross-repository-code-navigation)
