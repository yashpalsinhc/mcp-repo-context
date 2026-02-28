# Section 03: Domain-Aware Gap Analysis

## Overview

This section adds concept similarity scoring to gap detection in `internal/comparison/comparer.go`. Currently, gap analysis flags every function present in source repos but absent from the target repo as a "gap." This produces nonsensical results when comparing complementary packages (e.g., gorilla/handlers vs gorilla/mux produces 159 false gaps like `CORS()` and `CompressHandler()`).

The fix scores each potential gap by how related it is to the target repo's domain. Only gaps with similarity above a configurable threshold (default 0.3) are reported, ranked by similarity score.

## Dependencies

- **section-01-shared-infra**: Provides the `internal/nlp/` package with `Stem()`, `ConceptSimilarity()`, `SplitCamelCase()`, and stop word filtering. This section assumes those are already implemented.
- **section-02-comparison-keys**: Provides receiver-aware `normalizeFunctionKey()` and `normalizeTypeKey()`. The `FindGaps` method should already be using `normalizeFunctionKey()` for the target function map instead of raw `fn.Name`. This section modifies the same `FindGaps` method to add similarity filtering on top of the receiver-aware keys.

## Background: Current Code

The `FindGaps` method lives in `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/comparison/comparer.go` (currently at line 288). It builds a target inventory of function names and type names as `map[string]bool`, then iterates source repos and flags any function/type not present in the target as a gap.

The `Gap` struct is defined in the comparison package types and has fields: `Type`, `Name`, `SourceRepos`, `FilePath`, `Description`, `Priority`.

The `FunctionDef` struct (in `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/context/types.go` at line 172) has `Name`, `Signature`, `Description`, `Receiver`, and other fields relevant for building domain profiles.

The `comparer` struct currently has a `defaultThreshold float64` field (set to 0.8 in `NewComparer()`). The package imports `context`, `sort`, `strings`, and the internal context package aliased as `ctxpkg`.

Gap detection only runs when `TargetRepoID` is specified -- the `if targetContext == nil` guard at line 292 returns early. This behavior is unchanged.

## Tests First

Write these tests in `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/comparison/comparer_test.go` (append to existing file or create it). All tests use the standard `testing` package with inline fixtures.

```go
// === Domain-Aware Gap Analysis Tests ===

// Test: Gap analysis with concept similarity -- CORS() from gorilla/handlers is NOT a gap for gorilla/mux
//
// Setup: Create a target RepoContext mimicking gorilla/mux (functions like Route, Handle,
// ServeHTTP, NewRouter; types like Router, Route). Create a source RepoContext mimicking
// gorilla/handlers (functions like CORS, CompressHandler, LoggingHandler).
// Call FindGaps with these contexts.
// Assert: CORS, CompressHandler, LoggingHandler are NOT in the returned gaps
// (their similarity to the mux domain is below 0.3).

// Test: Gap analysis -- function with high similarity to target domain IS reported as gap
//
// Setup: Target repo has routing-related functions (Route, Handle, ServeHTTP).
// Source repo has a function called "SubrouteHandler" with description "handles sub-routes".
// Call FindGaps.
// Assert: "SubrouteHandler" IS in the returned gaps because its stemmed words
// ("subrout", "handler", "handl", "sub", "rout") overlap with the target domain.

// Test: Gap similarity threshold is respected (below 0.3 = not reported)
//
// Setup: Create target and source repos where source has a function whose concept
// similarity to the target domain is exactly at various thresholds.
// Assert: Functions below 0.3 similarity are excluded. Functions at or above 0.3 are included.

// Test: Gap results are ranked by similarity score (highest first)
//
// Setup: Source repo has 3 functions with varying similarity to target domain.
// Assert: Returned gaps are sorted by similarity score descending.

// Test: Stop words do not inflate similarity scores
//
// Setup: Source function named "GetNewHandler" -- all three words (Get, New, Handle/Handler)
// are stop words. Target domain has route-related terms.
// Assert: After stop word removal, the function has no meaningful words to score,
// resulting in 0.0 similarity and exclusion from gaps.

// Test: Gap analysis with empty source repo returns no gaps
//
// Setup: Target repo has functions. Source repos slice is empty (no source contexts).
// Assert: Empty gaps slice returned.

// Test: Gap analysis with identical repos returns no gaps
//
// Setup: Target and source have the exact same functions.
// Assert: No gaps (every source function exists in target by key).

// Test: Gap analysis only runs when TargetRepoID is specified
//
// Setup: Call Compare with IncludeGaps=true but TargetRepoID="".
// Assert: Gaps slice is empty (existing behavior, verify it is preserved).
```

## Implementation Details

### File to Modify

`/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/comparison/comparer.go`

### What Changes

The `FindGaps` method needs to be modified to:

1. Build a domain profile for the target repo before checking gaps.
2. Score each potential gap against the domain profile using concept similarity.
3. Filter out low-similarity gaps and rank remaining gaps by score.

### Step-by-Step Implementation

#### 1. Add import for the NLP package

Add to the imports block at the top of `comparer.go`:

```go
import (
    // ... existing imports ...
    "github.com/yashpalc/mcp-repo-context/internal/nlp"
)
```

#### 2. Add a helper method to build a domain profile

Create a new method on `comparer` that builds a `map[string]bool` domain profile from a `RepoContext`. This method collects all function names, type names, package names, and description words from the target repo, stems them, and stores them in a set for O(1) lookup.

```go
// buildDomainProfile creates a stemmed word set from a repo's function names,
// type names, and descriptions. Used for concept similarity scoring in gap analysis.
func (c *comparer) buildDomainProfile(rc *ctxpkg.RepoContext) map[string]bool {
    // Collect words from: function names (split camelCase), type names (split camelCase),
    // function descriptions, package path components.
    // Stem each word using nlp.Stem().
    // Filter out stop words (the nlp.ConceptSimilarity function handles stop words
    // internally, but also exclude them from the domain profile to avoid inflating matches).
    // Return map[string]bool with all stemmed words.
}
```

The profile should include:
- All function names from `rc.Files[*].Functions[*].Name`, split via `nlp.SplitCamelCase` then stemmed.
- All type names from `rc.Files[*].Types[*].Name`, split via `nlp.SplitCamelCase` then stemmed.
- Words from function descriptions (`fn.Description`), split on whitespace, stemmed.
- The repo ID or URL path components, stemmed (e.g., "gorilla/mux" yields "gorilla", "mux").

#### 3. Add a helper to extract stemmed words from a function

```go
// functionWords returns the stemmed, non-stop-word terms for a function
// based on its name and description. Used to compute similarity against a domain profile.
func functionWords(fn *ctxpkg.FunctionDef) []string {
    // Split fn.Name via nlp.SplitCamelCase, stem each part.
    // Split fn.Description on whitespace, stem each word.
    // Filter out stop words (Get, Set, New, Handle, Error, etc.).
    // Return deduplicated slice of stemmed words.
}
```

#### 4. Modify FindGaps to use similarity scoring

The current `FindGaps` method does a simple boolean check: `if !targetFuncs[fn.Name]`. The modified version should:

**a)** After building `targetFuncs` and `targetTypes` maps (using receiver-aware keys from section-02), also build the target domain profile:

```go
domainProfile := c.buildDomainProfile(targetContext)
```

**b)** When iterating source functions and finding one not in `targetFuncs`, compute similarity before recording it as a gap:

```go
words := functionWords(&fn)
similarity := nlp.ConceptSimilarity(words, domainProfile)
if similarity < c.gapSimilarityThreshold() {
    continue // skip -- not relevant to target domain
}
```

Store the similarity score alongside the gap for later sorting and output.

**c)** The `Gap` struct needs a `Similarity` field added. Find the `Gap` struct definition in the comparison package and add:

```go
Similarity float64 `json:"similarity,omitempty"` // concept similarity to target domain
```

**d)** After collecting all gaps, sort by similarity descending (highest first), then by existing priority:

```go
priorityOrder := map[string]int{"high": 0, "medium": 1, "low": 2}
sort.Slice(gaps, func(i, j int) bool {
    if gaps[i].Similarity != gaps[j].Similarity {
        return gaps[i].Similarity > gaps[j].Similarity
    }
    return priorityOrder[gaps[i].Priority] < priorityOrder[gaps[j].Priority]
})
```

**e)** Apply the same similarity filtering to type gaps (the "Find missing types" block at lines 337-356). Build type words similarly by splitting the type name via `SplitCamelCase` and stemming.

#### 5. Add configurable threshold

Add a method on `comparer` for the gap similarity threshold. Default value is `0.3`.

```go
func (c *comparer) gapSimilarityThreshold() float64 {
    // Could be made configurable via CompareOptions in the future.
    return 0.3
}
```

Optionally, the threshold could be added as a field on `comparer` initialized in `NewComparer()`, or as a field on `CompareOptions`. For this implementation, a method returning the constant is sufficient.

### NLP Package API Used

This section calls the following from `internal/nlp/` (implemented in section-01-shared-infra):

- `nlp.Stem(word string) string` -- Porter stemmer reducing English words to stems
- `nlp.SplitCamelCase(s string) []string` -- splits "ServeHTTP" into ["Serve", "HTTP"]
- `nlp.ConceptSimilarity(words []string, domainProfile map[string]bool) float64` -- fraction of words found in domain profile, with stop word filtering built in; returns 0.0 for empty word lists (no division by zero)

The stop word list (internal to the NLP package): `Get`, `Set`, `New`, `Make`, `Handle`, `Error`, `Init`, `Close`, `Open`, `Read`, `Write`, `String`, `Reset`, `Is`, `Has`.

### Existing Behavior Preserved

- Gap detection still only runs when `TargetRepoID` is specified (the `if targetContext == nil` guard is unchanged).
- The `assessGapPriority` method continues to be used for the `Priority` field.
- The `Description` field on `Gap` can be enhanced to include the similarity score, e.g., `fmt.Sprintf("Function not present in target repo (similarity: %.2f)", similarity)`.
- Gaps with similarity exactly at the threshold (0.3) should be included (use `<` not `<=` for the exclusion check).

### Summary of Files Changed

| File | Change |
|------|--------|
| `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/comparison/comparer.go` | Modify `FindGaps` to build domain profile and filter by similarity; add `buildDomainProfile` and `functionWords` helpers; add `gapSimilarityThreshold` method; add import for `internal/nlp` |
| `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/comparison/comparer_test.go` | Add 8 test cases for domain-aware gap analysis |
| Gap struct definition file (in comparison package) | Add `Similarity float64` field to `Gap` struct |