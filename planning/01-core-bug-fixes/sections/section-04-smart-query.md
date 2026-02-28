# Section 04: Smart Query NLP Improvements

## Overview

This section fixes the NLP-based query parsing in `internal/orchestrator/smart_query.go`. The current implementation uses regex-only pattern matching with a hardcoded 0.8 confidence value, causing common natural language queries like "How does routing work?" to be misclassified as function lookups (returning low-confidence wrong results instead of routing to concept search).

Five specific improvements are implemented:

1. Logic reordering for ambiguous queries (fall through to concept search when extracted word is not a function)
2. Word stemming as a secondary signal (using the Porter stemmer from `internal/nlp/`)
3. Common question pattern expansion
4. Two-level confidence contract (parsing confidence + handler confidence)
5. Path/file substring boundary matching fix

## Dependencies

- **section-01-shared-infra** must be completed first. This section uses the `internal/nlp/` package (specifically `Stem()` and `SplitCamelCase()` from `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/nlp/stemmer.go`) created in that section.

## File to Modify

- `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/orchestrator/smart_query.go` -- query parsing, logic reordering, confidence scoring, fallback logic, package matching, file matching

## Tests (Write First)

Create `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/orchestrator/smart_query_test.go`.

All tests use the standard Go `testing` package with inline fixtures. No external test frameworks.

```go
package orchestrator

import (
    "testing"
)

// --- Logic reordering tests ---

// Test: "How does routing work?" routes to concept search (not function lookup)
// parseQuery returns QueryTypeFunction with "routing", but handleFunctionQuery
// should detect "routing" is not a real function and fall through to concept search.
func TestParseQuery_HowDoesRoutingWork_RoutesToConceptSearch(t *testing.T) {
    // Setup: mock manager with repo context containing NO function named "routing"
    // but with functions whose names/descriptions relate to routing concepts.
    // Query: "How does routing work?"
    // Assert: result.QueryType should NOT be QueryTypeFunction with a "not found" answer.
    // Assert: result should contain concept-related results or NeedsAI == true.
}

// Test: "What is the project structure?" routes to architecture query
func TestParseQuery_ProjectStructure_RoutesToArchitecture(t *testing.T) {
    // parseQuery("what is the project structure?") should return QueryTypeArchitecture
    // NOT QueryTypeType with "project" as the type name.
}

// Test: "What does ServeHTTP do?" routes to function lookup (actual function name)
func TestParseQuery_WhatDoesServeHTTPDo_RoutesToFunction(t *testing.T) {
    // parseQuery("what does ServeHTTP do?") should return QueryTypeFunction
    // with extracted["function"] == "servehttp"
}

// Test: "Find all database functions" routes to side-effect search
func TestParseQuery_FindDatabaseFunctions_RoutesToSideEffect(t *testing.T) {
    // parseQuery("find all database functions") should return QueryTypeSideEffect
    // with extracted["effect"] == "db_query"
}

// Test: "Who calls ValidateToken?" routes to caller search
func TestParseQuery_WhoCallsValidateToken_RoutesToCallers(t *testing.T) {
    // parseQuery("who calls ValidateToken?") should return QueryTypeCallers
    // with extracted["function"] == "validatetoken"
}

// --- Logic reorder: function fallthrough ---

// Test: Extracted word that is NOT a function falls through to concept search
func TestSmartQuery_ExtractedWordNotFunction_FallsThroughToConcept(t *testing.T) {
    // Setup: mock manager with repo context that has NO function named "routing"
    // but has route-related functions (e.g., "HandleRoute", "NewRouter").
    // Query: "how does routing work?"
    // Assert: result does NOT say "Function `routing` not found"
    // Assert: result contains concept-related matches or NeedsAI == true
}

// Test: Extracted word that IS a function name stays as function lookup
func TestSmartQuery_ExtractedWordIsFunction_StaysAsFunctionLookup(t *testing.T) {
    // Setup: mock manager with repo context that HAS a function named "ServeHTTP"
    // Query: "how does ServeHTTP work?"
    // Assert: result.QueryType == QueryTypeFunction
    // Assert: result.Answer contains function details for ServeHTTP
}

// --- Confidence contract tests ---

// Test: Exact regex match produces confidence >= 0.9
func TestConfidence_ExactRegexMatch_AtLeast09(t *testing.T) {
    // Query: "what does ServeHTTP do?" (exact function pattern match)
    // Assert: parsing confidence >= 0.9
}

// Test: Stemmed match produces confidence >= 0.75
func TestConfidence_StemmedMatch_AtLeast075(t *testing.T) {
    // Query where function name is found via stemming (e.g., "handlers" -> "handler")
    // Assert: result.Confidence >= 0.75
}

// Test: Below 0.6 combined confidence sets NeedsAI = true
func TestConfidence_BelowThreshold_SetsNeedsAI(t *testing.T) {
    // Query that produces very low confidence (no match found at all)
    // Assert: result.NeedsAI == true
    // Assert: result.Confidence < 0.6
}

// Test: Handler confidence cannot raise above parsing confidence
func TestConfidence_HandlerCannotRaiseAboveParsing(t *testing.T) {
    // If parsing confidence is 0.75 (stemmed match), handler should not
    // set result.Confidence to 0.95 even if it finds the function.
    // Assert: result.Confidence <= parsing confidence
}

// --- Path/file boundary matching tests ---

// Test: Package path "http" does NOT match file path containing "https"
func TestPackagePathMatching_HttpDoesNotMatchHttps(t *testing.T) {
    // matchesPackagePath("net/https/client.go", "http") should return false
    // matchesPackagePath("net/http/server.go", "http") should return true
}

// Test: Package path "mux" matches "gorilla/mux" but not "gorilla/muxer"
func TestPackagePathMatching_MuxDoesNotMatchMuxer(t *testing.T) {
    // matchesPackagePath("gorilla/mux/router.go", "mux") should return true
    // matchesPackagePath("gorilla/muxer/router.go", "mux") should return false
}

// Test: File query "router.go" does NOT match "subrouter.go" (handleFileQuery fix)
func TestFileQuery_RouterGoDoesNotMatchSubrouterGo(t *testing.T) {
    // matchesFileName("gorilla/mux/subrouter.go", "router.go") should return false
    // matchesFileName("gorilla/mux/router.go", "router.go") should return true
}

// --- Stemming integration tests ---

// Test: "handlers" query matches "handler" function via stemming
func TestStemming_HandlersQueryMatchesHandlerFunction(t *testing.T) {
    // Setup: repo with function named "handler" (not "handlers")
    // Query: "what does handlers do?"
    // Assert: function "handler" is found (stemmed match)
}

// Test: "routing" query matches route-related concepts
func TestStemming_RoutingQueryMatchesRouteConcepts(t *testing.T) {
    // Setup: repo with functions like "NewRouter", "HandleRoute"
    // Query: "how does routing work?" (falls through to concept search)
    // Assert: results contain route-related functions
}
```

## Implementation Details

### Current Code Structure

The `parseQuery` function (lines 120-308 of `smart_query.go`) processes patterns in this order:

1. Function patterns (lines 125-139) -- includes `how does (\w+) work`
2. Type/struct patterns (lines 142-153) -- includes `what is (\w+)`
3. Side effect patterns (lines 156-179)
4. Concept patterns (lines 183-201)
5. Caller patterns (lines 204-217)
6. Calls patterns (lines 220-231)
7. Flow patterns (lines 234-250)
8. File patterns (lines 253-263)
9. Package patterns (lines 267-296)
10. Architecture patterns (lines 299-305)
11. Default: General (line 307)

The `SmartQueryResult` struct already has `NeedsAI` (line 40) and `Confidence` (line 39) fields. The `SmartQuery` method (line 66) hardcodes `Confidence: 0.8` at line 79.

### Improvement 1: Logic Reordering for Ambiguous Queries

The root cause: the regex `how does (\w+) work` in `parseQuery()` greedily extracts any word as a function name. When "How does routing work?" is parsed, "routing" is extracted and `QueryTypeFunction` is returned. The `handleFunctionQuery` handler then searches for a function named "routing", fails to find it, and returns a low-confidence "not found" result at line 338.

**Fix location:** `handleFunctionQuery` method, in the `foundFunc == nil` block (currently at lines 337-341).

Instead of immediately returning "Function not found" with confidence 0.5, the handler should:

1. Take the extracted word (e.g., "routing") from `extracted["function"]`
2. Use the Porter stemmer from `internal/nlp` to stem it (e.g., "routing" becomes "rout")
3. Set `extracted["concept"]` to the original word
4. Call `m.handleConceptQuery(ctx, repoCtx, extracted, result)` as a fallback
5. If concept search also returns no results, THEN set `result.NeedsAI = true` and return a message suggesting the `ask` tool, rather than returning a misleading "Function not found" answer

This change is in `handleFunctionQuery`, NOT in `parseQuery`. The `parseQuery` function can still return `QueryTypeFunction` for "how does X work" -- the reordering happens at the handler level. This preserves correct behavior for queries like "how does ServeHTTP work?" where ServeHTTP IS a real function.

### Improvement 2: Word Stemming as Secondary Signal

Import the `internal/nlp` package (created in section-01-shared-infra). The key function is `nlp.Stem(word string) string`.

Use stemming in two places:

**a) In `handleFunctionQuery` function search loop (around lines 323-335):**

The current search does `strings.EqualFold(fn.Name, funcName)`. After this exact match loop, add a second pass that stems both the query function name and actual function names:

- Stem the query: `nlp.Stem(funcName)` (e.g., "handlers" becomes "handler")
- For each function in the repo, stem its name and compare
- If a stemmed match is found, use it but note this is a stemmed match (affects confidence -- see improvement 4)

Exact match takes priority over stemmed match.

**b) In the concept search fallthrough (from improvement 1):**

When falling through to concept search after function lookup fails, stem the query word before searching. "routing" stems to "rout" which will match "route"-related functions via the concept search mechanism.

### Improvement 3: Common Question Pattern Expansion

Add new patterns to `parseQuery()`. Be careful about ordering -- more specific patterns must come before more general ones.

**Architecture patterns -- prevent "structure"/"architecture" from being caught as type names:**

Currently "what is the structure of the project?" gets caught by type patterns (lines 142-153) as a type lookup for "structure". The fix: add an early check BEFORE the type patterns block. If the extracted word from a "what is X" pattern is a known architecture word ("structure", "architecture", "layout", "overview", "project"), skip the type match and return `QueryTypeArchitecture` instead.

Concretely, add this check right before the type patterns loop:

```go
// Architecture guard: prevent architecture words from matching as type names
architectureWords := map[string]bool{
    "structure": true, "architecture": true, "layout": true,
    "overview": true, "project": true, "codebase": true,
}
```

Then in the type patterns loop, after extracting a match, check if the extracted word is in `architectureWords`. If so, return `QueryTypeArchitecture` instead of `QueryTypeType`.

**New concept search patterns:**

Add regex patterns before the concept keyword check (around lines 183-201):

- `find all (\w+) (?:handlers|functions|methods)` routing to `QueryTypeConcept`
- `show (\w+) related (?:code|functions)` routing to `QueryTypeConcept`

### Improvement 4: Two-Level Confidence Contract

Replace the hardcoded `Confidence: 0.8` at line 79.

**Modify `parseQuery` to return a confidence value.** Change the signature:

```go
func parseQuery(query string) (QueryType, map[string]string, float64)
```

Confidence levels from `parseQuery`:

| Match Type | Confidence | Example |
|---|---|---|
| Exact regex match | 0.9 | "what does ServeHTTP do?" matches function pattern |
| Keyword-based match | 0.8 | Side effect or concept keyword matched |
| Fuzzy/partial match | 0.6 | Architecture pattern by keyword containment |

**In `SmartQuery` method:** Update the call site and enforce the ceiling after handler execution:

```go
queryType, extracted, parsingConfidence := parseQuery(query)
// ... set result.Confidence = parsingConfidence ...
// ... handler runs and may adjust result.Confidence ...
// Enforce ceiling:
if result.Confidence > parsingConfidence {
    result.Confidence = parsingConfidence
}
```

**Handler confidence adjustments:**

- `handleFunctionQuery`: exact function found = keep parsing confidence; stemmed match found = `min(parsingConfidence, 0.75)`; function not found (before fallthrough) = 0.5
- `handleConceptQuery`: results found = keep parsing confidence; no results = 0.5
- Other handlers follow similar patterns

**AI fallback threshold:**

After the handler returns and confidence ceiling is enforced, if `result.Confidence < 0.6`, automatically set `result.NeedsAI = true`:

```go
if result.Confidence < 0.6 {
    result.NeedsAI = true
}
```

### Improvement 5: Fix Path Substring Matching

**In `handlePackageQuery` (line 913):**

Current buggy code:
```go
if strings.Contains(path, packagePath) {
```

Replace with a helper function that checks `/` boundaries:

```go
func matchesPackagePath(filePath, packagePath string) bool {
    idx := strings.Index(filePath, packagePath)
    if idx < 0 {
        return false
    }
    // Check left boundary: must be at start or preceded by "/"
    if idx > 0 && filePath[idx-1] != '/' {
        return false
    }
    // Check right boundary: must be at end or followed by "/"
    endIdx := idx + len(packagePath)
    if endIdx < len(filePath) && filePath[endIdx] != '/' {
        return false
    }
    return true
}
```

This ensures "http" matches "net/http/server.go" but NOT "net/https/client.go", and "mux" matches "gorilla/mux/router.go" but NOT "gorilla/muxer/router.go".

**In `handleFileQuery` (line 831):**

Current buggy code:
```go
if strings.HasSuffix(path, fileName) || strings.Contains(path, fileName) {
```

Replace with a boundary-aware helper:

```go
func matchesFileName(filePath, fileName string) bool {
    // Exact filename match: path ends with "/filename" or path IS filename
    if strings.HasSuffix(filePath, "/"+fileName) || filePath == fileName {
        return true
    }
    return false
}
```

This ensures "router.go" matches "gorilla/mux/router.go" but NOT "gorilla/mux/subrouter.go".

## Summary of All Changes

All changes are in a single file: `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/orchestrator/smart_query.go`

| Change | Location | What |
|--------|----------|------|
| Logic reorder | `handleFunctionQuery`, "foundFunc == nil" block | Fall through to concept search instead of returning "not found" |
| Stemming in function search | `handleFunctionQuery`, function search loop | Add stemmed matching as second pass after exact match |
| Stemming import | File imports | Add `"github.com/yashpalc/mcp-repo-context/internal/nlp"` |
| Architecture pattern guard | `parseQuery`, before/within type patterns | Prevent "structure"/"architecture" from matching as type names |
| New concept patterns | `parseQuery`, concept patterns block | Add "find all X handlers/functions" regex patterns |
| `parseQuery` signature change | `parseQuery` function | Return `float64` confidence as third value |
| Confidence ceiling | `SmartQuery` method, after handler call | Enforce `result.Confidence <= parsingConfidence` |
| AI fallback | `SmartQuery` method, after handler call | Set `NeedsAI = true` when confidence < 0.6 |
| `matchesPackagePath` helper | New function in same file | Boundary-aware `/`-delimited path matching |
| `matchesFileName` helper | New function in same file | Boundary-aware filename matching |
| Fix package matching | `handlePackageQuery`, file loop | Use `matchesPackagePath` instead of `strings.Contains` |
| Fix file matching | `handleFileQuery`, file loop | Use `matchesFileName` instead of `HasSuffix`/`Contains` |