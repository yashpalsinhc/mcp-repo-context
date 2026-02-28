# TDD Plan: Core Bug Fixes & Quality

Test stubs to write BEFORE implementing each section. Uses the standard Go `testing` package with inline fixtures matching the existing codebase pattern. Run with `go test ./...`.

---

## Section 7: Shared Infrastructure

### `internal/nlp/stemmer_test.go`

```go
// Test: Stem("routing") returns "rout"
// Test: Stem("handlers") returns "handler"
// Test: Stem("structures") returns "structur"
// Test: Stem("validation") returns "valid"
// Test: Stem("") returns "" (empty input)
// Test: Stem("x") returns "x" (single character unchanged)
// Test: Stem("ServeHTTP") splits camelCase before stemming → ["serve", "http"]
// Test: Stem("CompressHandler") splits camelCase → ["compress", "handler"]
// Test: SplitCamelCase("ServeHTTP") returns ["Serve", "HTTP"]
// Test: SplitCamelCase("getURL") returns ["get", "URL"]
// Test: SplitCamelCase("simple") returns ["simple"]
// Test: Stem preserves Unicode identifiers (passes through unchanged)
// Test: Stem lowercases ASCII input
```

### `internal/nlp/distance_test.go`

```go
// Test: LevenshteinDistance("kitten", "sitting") returns 3
// Test: LevenshteinDistance("", "abc") returns 3
// Test: LevenshteinDistance("abc", "") returns 3
// Test: LevenshteinDistance("same", "same") returns 0
// Test: LevenshteinDistance terminates early when distance exceeds max threshold
// Test: FuzzyMatch("route", ["router", "handler", "routing"], 2) returns ["router", "routing"]
// Test: FuzzyMatch("xyz", ["abc", "def"], 1) returns empty slice
// Test: FuzzyMatch with empty candidates returns empty slice
// Test: FuzzyMatch with empty input returns candidates within maxDistance of ""
```

### `internal/nlp/similarity_test.go`

```go
// Test: ConceptSimilarity with all words in domain returns 1.0
// Test: ConceptSimilarity with no words in domain returns 0.0
// Test: ConceptSimilarity with half words in domain returns ~0.5
// Test: ConceptSimilarity with empty word list returns 0.0 (no division by zero)
// Test: ConceptSimilarity with empty domain profile returns 0.0
// Test: Stop words ("Get", "Set", "New", "Handle", "Error") are excluded from scoring
// Test: ConceptSimilarity uses stemmer internally (routing matches rout in domain)
// Test: Domain profile uses map[string]bool for O(1) lookup (verify interface, not timing)
```

---

## Section 1: Receiver-Aware Comparison Keys

### `internal/comparison/comparer_test.go`

```go
// Test: normalizeFunctionKey with receiver "*Router" and name "ServeHTTP" returns "Router.ServeHTTP"
// Test: normalizeFunctionKey with receiver "Router" (no pointer) returns "Router.ServeHTTP"
// Test: normalizeFunctionKey with empty receiver returns "ServeHTTP" (package-level function)
// Test: normalizeFunctionKey strips only leading "*" from receiver

// Test: normalizeTypeKey with different packages produces different keys
// Test: normalizeTypeKey for "Handler" in package "mux" vs "Handler" in package "cors" are distinct

// Test: FindDuplicates does NOT flag Router.ServeHTTP and cors.ServeHTTP as duplicates
// Test: FindDuplicates DOES flag two Router.ServeHTTP from different repos as duplicates
// Test: FindDuplicates with package-level functions (no receiver) still works correctly

// Test: FindConflicts does NOT flag methods on different receiver types as conflicts
// Test: FindConflicts DOES flag methods on same receiver type with different signatures as conflicts
// Test: FindConflicts uses normalizeFunctionKey (not raw fn.Name)

// Test: FindGaps does NOT flag Router.ServeHTTP as filling gap for cors.ServeHTTP
// Test: FindGaps uses normalizeFunctionKey for target function map (not raw fn.Name)

// Test: Lazy migration triggers when Version == 0 and functions have receivers
// Test: Lazy migration sets Version to 1 after re-keying
// Test: Lazy migration is idempotent (running twice does not corrupt data)
// Test: Version 1 contexts skip migration entirely
```

---

## Section 2: Domain-Aware Gap Analysis

### `internal/comparison/comparer_test.go` (additional tests)

```go
// Test: Gap analysis with concept similarity — CORS() from gorilla/handlers is NOT a gap for gorilla/mux
// Test: Gap analysis — function with high similarity to target domain IS reported as gap
// Test: Gap similarity threshold is respected (below 0.3 = not reported)
// Test: Gap results are ranked by similarity score (highest first)
// Test: Stop words do not inflate similarity scores
// Test: Gap analysis with empty source repo returns no gaps
// Test: Gap analysis with identical repos returns no gaps
// Test: Gap analysis only runs when TargetRepoID is specified
```

---

## Section 3: Smart Query NLP Improvements

### `internal/orchestrator/smart_query_test.go`

```go
// Test: "How does routing work?" routes to concept search (not function lookup)
// Test: "What is the project structure?" routes to architecture query
// Test: "What does ServeHTTP do?" routes to function lookup (actual function name)
// Test: "Find all database functions" routes to side-effect search
// Test: "Who calls ValidateToken?" routes to caller search

// Test: Logic reorder — extracted word that's not a function falls through to concept search
// Test: Logic reorder — extracted word that IS a function name stays as function lookup

// Test: Confidence — exact regex match produces confidence >= 0.9
// Test: Confidence — stemmed match produces confidence >= 0.75
// Test: Confidence — below 0.6 sets NeedsAI = true
// Test: Handler confidence cannot raise above parsing confidence

// Test: Package path "http" does NOT match file path containing "https"
// Test: Package path "mux" matches "gorilla/mux" but not "gorilla/muxer"
// Test: File query "router.go" does NOT match "subrouter.go" (handleFileQuery fix)

// Test: Stemming — "handlers" query matches "handler" function
// Test: Stemming — "routing" query matches route-related concepts
```

---

## Section 4: Pattern Execution Fixes

### `internal/compose/chain_test.go`

```go
// Test: Chain with all steps executed returns status "executed" for each
// Test: Chain with conditional step skipped returns status "skipped" with reason
// Test: Chain where step 1 fails marks remaining steps as "not_reached"
// Test: Partial completion output includes results from executed steps
// Test: Partial completion output explains why skipped steps were skipped
// Test: StepResults slice has entry for every step (executed, skipped, or not_reached)
```

### `internal/compose/patterns_test.go`

```go
// Test: impact_analysis resolves file_path via search step before get_function_context
// Test: impact_analysis with unknown function returns clear error (search found nothing)
// Test: impact_analysis with multi-result search uses highest-confidence result
// Test: impact_analysis includes chosen file_path in result message

// Test: search_with_context handles non-array result format gracefully
// Test: search_with_context marks step 2 as "skipped" when result parsing fails (with reason)
// Test: search_with_context completes successfully with well-formed results
```

---

## Section 5: Call Graph Callee Extraction

### `internal/analyzer/callgraph_test.go`

```go
// Test: Method call x.Method() where x is a function parameter with known type → resolves to Type.Method
// Test: Method call x.Method() where x is declared as "var x SomeType" → resolves to SomeType.Method
// Test: Method call x.Method() where x is composite literal "x := SomeType{}" → resolves to SomeType.Method
// Test: Method call x.Method() where x type is unknown → recorded as "unresolved method" (not dropped)
// Test: CalledBy includes method calls (Type == "method" no longer filtered out)
// Test: ServeHTTP in test fixture shows both callers AND callees

// Test: makeNodeID with receiver "Router" and function "ServeHTTP" → "file:Router.ServeHTTP"
// Test: makeNodeID without receiver → "file:ServeHTTP"
// Test: Two methods with same name, different receivers, same file → different node IDs

// Test: funcFile map with receiver-qualified keys — no collision for same-name methods
// Test: funcFile["Router.ServeHTTP"] != funcFile["cors.ServeHTTP"]

// Test: CallRef Receiver field populated for method calls
// Test: CallRef Receiver field empty for package-level calls
// Test: Old CallRef data (no Receiver field) deserializes with empty Receiver (omitempty)
```

### `internal/analyzer/go_analyzer_test.go` (type-checker mode)

```go
// Test: Type-checker mode resolves method calls via go/types
// Test: Type-checker mode falls back to heuristic when module resolution fails
// Test: Type-checker mode produces warning log on fallback
// Test: Flag --use-type-checker=false uses heuristic mode (default)
```

---

## Section 6: Package Structure Grouping

### `internal/orchestrator/smart_query_test.go` (additional tests)

```go
// Test: Flat package (single directory) shows flat file list, not grouped
// Test: Flat package groups by purpose: source, tests, docs, config
// Test: Multi-directory package groups by directory first, then purpose
// Test: Extension-based grouping (.go/, .md/) does NOT appear in output
// Test: Deeply nested package (3+ levels) collapses to top 2 levels
// Test: Single-file package still shows properly
```

---

## Integration Test

### `internal/comparison/integration_test.go` (or test/integration/)

```go
// Test: Analyze gorilla/mux and gorilla/handlers fixtures
// Test: Run compare_repos — gap count is significantly less than 159
// Test: Run find_duplicates — no false duplicates from receiver-less keys
// Test: Run find_conflicts — no false conflicts from interface implementations
// Test: Router.ServeHTTP and cors.ServeHTTP are NOT treated as related
// Test: End-to-end: analyze → compare → verify all tools return correct results
```
