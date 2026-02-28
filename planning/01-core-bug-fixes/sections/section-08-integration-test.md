# Section 08: Integration Test

## Overview

This section implements end-to-end integration tests that verify all bug fixes from sections 01 through 07 work together correctly. The integration test analyzes fixture data mimicking gorilla/mux and gorilla/handlers, runs `compare_repos` (including `FindDuplicates`, `FindConflicts`, and `FindGaps`), and asserts that:

- Gap count is significantly reduced from the original 159 false positives
- No false duplicates arise from receiver-less keys (e.g., `Router.ServeHTTP` vs `cors.ServeHTTP`)
- No false conflicts from interface implementations with same method names but different receiver types
- `Router.ServeHTTP` and `cors.ServeHTTP` are treated as unrelated

## Dependencies

This section depends on all prior sections being implemented:

- **section-01-shared-infra**: The `internal/nlp/` package (stemmer, distance, similarity)
- **section-02-comparison-keys**: Receiver-aware `normalizeFunctionKey()` and `normalizeTypeKey()`, lazy migration
- **section-03-gap-analysis**: Domain-aware gap analysis with concept similarity scoring
- **section-04-smart-query**: Smart query NLP improvements (not directly tested here, but part of overall fix)
- **section-05-pattern-execution**: Pattern execution fixes (not directly tested here)
- **section-06-call-graph**: Call graph callee extraction fixes (not directly tested here)
- **section-07-package-structure**: Package structure grouping (not directly tested here)

## Tests FIRST

### File: `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/comparison/integration_test.go`

```go
//go:build integration

package comparison

import (
	"context"
	"testing"
	"time"

	ctxpkg "github.com/yashpalc/mcp-repo-context/internal/context"
)

// buildMuxFixture creates a RepoContext mimicking gorilla/mux.
// Contains types: Router, Route, RouteMatch
// Contains methods: Router.ServeHTTP, Router.HandleFunc, Router.Use, Route.Handler, Route.Path
// Contains package-level functions: NewRouter, Vars, SetURLVars
func buildMuxFixture() *ctxpkg.RepoContext {
	// Populate a RepoContext with Version=0 (pre-migration) and realistic
	// function/type data. Each FunctionDef must include the Receiver field
	// where appropriate.
	//
	// Key entries:
	//   - FunctionDef{Name: "ServeHTTP", Receiver: "*Router", Signature: "func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request)"}
	//   - FunctionDef{Name: "HandleFunc", Receiver: "*Router", ...}
	//   - FunctionDef{Name: "NewRouter", Receiver: "", ...}  (package-level)
	//   - TypeDef{Name: "Router", Kind: "struct"}
	//   - TypeDef{Name: "Route", Kind: "struct"}
	//   - TypeDef{Name: "RouteMatch", Kind: "struct"}
	panic("implement fixture")
}

// buildHandlersFixture creates a RepoContext mimicking gorilla/handlers.
// Contains types: cors (unexported), CompressResponseWriter
// Contains methods: cors.ServeHTTP, CompressResponseWriter.Write
// Contains package-level functions: CORS, CompressHandler, LoggingHandler,
//     RecoveryHandler, ProxyHeaders, CanonicalHost, ContentTypeHandler
func buildHandlersFixture() *ctxpkg.RepoContext {
	// Populate a RepoContext with Version=0 (pre-migration) and realistic
	// function/type data. Each FunctionDef must include the Receiver field.
	//
	// Key entries:
	//   - FunctionDef{Name: "ServeHTTP", Receiver: "*cors", Signature: "func (ch *cors) ServeHTTP(w http.ResponseWriter, r *http.Request)"}
	//   - FunctionDef{Name: "Write", Receiver: "*CompressResponseWriter", ...}
	//   - FunctionDef{Name: "CORS", Receiver: "", ...}  (package-level)
	//   - FunctionDef{Name: "CompressHandler", Receiver: "", ...}
	//   - TypeDef{Name: "cors", Kind: "struct"}
	//   - TypeDef{Name: "CompressResponseWriter", Kind: "struct"}
	panic("implement fixture")
}

// Test: Analyze gorilla/mux and gorilla/handlers fixtures
// Verifies that fixtures can be constructed with correct structure.
func TestIntegration_FixturesAreValid(t *testing.T) {
	// Build both fixtures and verify they have non-zero files, functions, types.
}

// Test: Run compare_repos — gap count is significantly less than 159
// The original bug reported 159 false gaps. After domain-aware gap analysis
// with concept similarity, most handler-specific functions (CORS, CompressHandler,
// LoggingHandler, etc.) should be filtered out because they have low similarity
// to the mux domain profile.
func TestIntegration_GapCountReduced(t *testing.T) {
	// Build fixtures, set mux as TargetRepoID, run Compare with IncludeGaps=true.
	// Assert gap count is significantly less than the total function count from handlers.
	// The exact threshold depends on implementation, but should be well below 159.
}

// Test: Run find_duplicates — no false duplicates from receiver-less keys
// Router.ServeHTTP and cors.ServeHTTP must NOT be flagged as duplicates
// because they have different receiver types.
func TestIntegration_NoFalseDuplicates(t *testing.T) {
	// Build fixtures, run FindDuplicates.
	// Assert that no DuplicateGroup contains instances from both repos
	// for ServeHTTP (different receivers = different keys).
	// Assert that no DuplicateGroup contains instances for Write
	// (different receivers: CompressResponseWriter vs none in mux).
}

// Test: Run find_conflicts — no false conflicts from interface implementations
// Methods with the same name but different receiver types are NOT conflicts.
func TestIntegration_NoFalseConflicts(t *testing.T) {
	// Build fixtures, set mux as target, run FindConflicts.
	// Assert no conflict entry exists for "ServeHTTP" across different receivers.
	// Assert no conflict entry exists for "Write" across different receivers.
}

// Test: Router.ServeHTTP and cors.ServeHTTP are NOT treated as related
// This is the core acceptance test. After receiver-aware keying, these two
// methods produce different normalized keys and should never appear in the
// same duplicate group, conflict, or gap entry.
func TestIntegration_DifferentReceiversAreUnrelated(t *testing.T) {
	// Build fixtures, run full Compare (duplicates + conflicts + gaps).
	// Verify that "Router.ServeHTTP" and "cors.ServeHTTP" never co-appear
	// in any DuplicateGroup.Instances, any Conflict, or any Gap.
}

// Test: End-to-end: analyze -> compare -> verify all tools return correct results
// Runs the full pipeline: build fixtures, run Compare with all options enabled,
// and verify the overall result is sane.
func TestIntegration_EndToEnd(t *testing.T) {
	// Build both fixtures.
	// Run Compare with all options: IncludeDuplicates, IncludeConflicts,
	//   IncludeGaps, TargetRepoID = mux fixture ID.
	// Verify:
	//   - result.Repos has 2 entries
	//   - result.Duplicates has no false positives from same-name different-receiver methods
	//   - result.Conflicts has no false positives from interface methods
	//   - result.Gaps length is reasonable (well below total handler functions)
	//   - Any gaps that DO appear have similarity > 0.3 to the mux domain
}
```

## Implementation Details

### Build Tag

The integration test file uses the `//go:build integration` build tag. This keeps it separate from unit tests during normal `go test ./...` runs. To run integration tests:

```bash
go test -tags=integration ./internal/comparison/...
```

### Fixture Construction

Each fixture function builds a `*ctxpkg.RepoContext` with:

- A unique `ID` (e.g., `"github.com/gorilla/mux"`, `"github.com/gorilla/handlers"`)
- `Version: 0` to test that lazy migration (from section-02) triggers automatically
- A `Files` map containing at least one `*ctxpkg.FileContext` per logical source file
- Each `FileContext` contains realistic `Functions` and `Types` slices

The mux fixture should include functions and types that represent the routing domain: `Router`, `Route`, `RouteMatch`, `NewRouter`, `HandleFunc`, `Use`, `Vars`, `ServeHTTP` (on `*Router`), `Handler` (on `*Route`), `Path` (on `*Route`).

The handlers fixture should include functions and types from the middleware domain: `cors` (struct), `CompressResponseWriter` (struct), `CORS` (package-level), `CompressHandler`, `LoggingHandler`, `RecoveryHandler`, `ProxyHeaders`, `CanonicalHost`, `ContentTypeHandler`, `ServeHTTP` (on `*cors`), `Write` (on `*CompressResponseWriter`).

Key principle: the fixtures must have at least one function name collision across repos with different receivers (`ServeHTTP` on `*Router` vs `*cors`, `Write` on `*CompressResponseWriter` vs none). This is what triggers the original bugs and validates the fix.

### Relevant Data Types

The following types from `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/context/types.go` are used to build fixtures:

```go
type RepoContext struct {
    ID           string                  `json:"id"`
    Version      int                     `json:"version"`
    Files        map[string]*FileContext `json:"files"`
    Statistics   RepoStatistics          `json:"statistics"`
    // ... other fields
}

type FileContext struct {
    Path      string        `json:"path"`
    Package   string        `json:"package,omitempty"`
    Functions []FunctionDef `json:"functions"`
    Types     []TypeDef     `json:"types"`
    // ... other fields
}

type FunctionDef struct {
    Name      string `json:"name"`
    Signature string `json:"signature"`
    Receiver  string `json:"receiver,omitempty"`
    IsPublic  bool   `json:"is_public"`
    LineStart int    `json:"line_start"`
    LineEnd   int    `json:"line_end"`
    // ... other fields
}

type TypeDef struct {
    Name     string  `json:"name"`
    Kind     string  `json:"kind"`
    Fields   []Field `json:"fields,omitempty"`
    IsPublic bool    `json:"is_public"`
    // ... other fields
}
```

The comparison is driven by types in `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/comparison/types.go`:

```go
type CompareOptions struct {
    TargetRepoID        string
    IncludeDuplicates   bool
    IncludeConflicts    bool
    IncludeGaps         bool
    IncludeConsistency  bool
    SimilarityThreshold float64
}
```

### What Each Test Validates

**TestIntegration_FixturesAreValid**: Sanity check that fixtures are properly constructed. Assert `len(rc.Files) > 0`, iterate files and assert `len(fc.Functions) > 0` or `len(fc.Types) > 0` for at least one file.

**TestIntegration_GapCountReduced**: Build both fixtures. Create a `CompareOptions` with `IncludeGaps: true` and `TargetRepoID` set to the mux fixture's ID. Call `comparer.Compare(ctx, []*ctxpkg.RepoContext{muxCtx, handlersCtx}, opts)`. Count the number of gaps in `result.Gaps`. Assert the count is well below the total number of functions in the handlers fixture. The exact threshold depends on the similarity scoring from section-03, but a reasonable assertion is that the gap count is less than 50% of the handler function count (since most handler functions like `CORS`, `CompressHandler`, `LoggingHandler` are middleware-domain concepts unrelated to routing).

**TestIntegration_NoFalseDuplicates**: Call `FindDuplicates` with both fixtures. Iterate all `DuplicateGroup` entries. For any group where `Name` contains "ServeHTTP" or "Write", assert that all instances come from the same repo (no cross-repo false matches). With receiver-aware keys, `Router.ServeHTTP` and `cors.ServeHTTP` are different keys and will never be grouped together.

**TestIntegration_NoFalseConflicts**: Call `FindConflicts` with handlers as source and mux as target. Iterate all `Conflict` entries. Assert no conflict has `Name == "ServeHTTP"` or `Name == "Write"` unless the receiver types actually match. With receiver-aware keying in `FindConflicts`, `Router.ServeHTTP` and `cors.ServeHTTP` produce different keys and never match.

**TestIntegration_DifferentReceiversAreUnrelated**: This is the umbrella acceptance test. Run full `Compare` and verify that the string `"Router.ServeHTTP"` and `"cors.ServeHTTP"` never appear together in any single result entry (duplicate group, conflict, or gap). Use helper functions to scan result structures.

**TestIntegration_EndToEnd**: The comprehensive test. Run `Compare` with all options enabled. Make broad assertions about result sanity:
- `len(result.Repos) == 2`
- No duplicate group has instances from both repos for same-name-different-receiver methods
- No conflict exists for same-name-different-receiver methods
- Gap count is reasonable
- If any gaps exist, they should have been scored by concept similarity (verify via the gap's presence -- only gaps with similarity above threshold appear)

### Files to Create

| File | Purpose |
|------|---------|
| `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/comparison/integration_test.go` | Integration test with build tag, fixture builders, and 6 test functions |

### Files Referenced (read-only, from other sections)

| File | What It Provides |
|------|-----------------|
| `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/comparison/comparer.go` | `Compare`, `FindDuplicates`, `FindConflicts`, `FindGaps` methods with receiver-aware keys (section-02) and domain-aware gap analysis (section-03) |
| `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/comparison/types.go` | `CompareOptions`, `CompareResult`, `DuplicateGroup`, `Conflict`, `Gap` types |
| `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/context/types.go` | `RepoContext`, `FileContext`, `FunctionDef`, `TypeDef` types |
| `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/nlp/similarity.go` | `ConceptSimilarity` used by gap analysis (section-01) |

### Helper Functions to Include in Test File

The test file should include two private helper functions:

- `containsCrossRepoInstances(group DuplicateGroup) bool` -- returns true if a duplicate group has instances from more than one repo ID. Used to identify cross-repo false positives.
- `findGapByName(items []Gap, name string) *Gap`, `findConflictByName(items []Conflict, name string) *Conflict`, and `findDuplicateByName(items []DuplicateGroup, name string) *DuplicateGroup` -- simple linear scan helpers to find entries by name for specific assertions.

### Running the Tests

```bash
# Run integration tests only
go test -tags=integration -v -run TestIntegration ./internal/comparison/...

# Run all tests including integration
go test -tags=integration ./...
```

### Acceptance Criteria

The integration test suite passes when:

1. Zero false duplicates from same-name methods on different receivers
2. Zero false conflicts from interface implementation methods on different receivers
3. Gap count for gorilla/handlers functions missing from gorilla/mux is significantly below the pre-fix count of 159 (most middleware-specific functions are filtered by concept similarity)
4. `Router.ServeHTTP` and `cors.ServeHTTP` are treated as completely unrelated entities across all comparison tools