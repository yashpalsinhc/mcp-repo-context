# Section 01: Shared Infrastructure

## Overview

This section creates the `internal/nlp/` package -- a shared NLP utility package used by multiple subsequent sections (comparison keys, gap analysis, smart query). It also establishes schema versioning on `RepoContext` to support lazy data migration.

This section has **no dependencies** on other sections and must be completed first, as it blocks sections 02, 03, and 04.

## Background

The MCP repo-context server analyzes Go codebases and exposes tools for querying function metadata, call graphs, and cross-repo comparisons. Several bugs require NLP capabilities (stemming, fuzzy matching, similarity scoring) that do not currently exist in the codebase. Rather than adding external NLP dependencies (which stakeholders have vetoed), this section implements minimal custom NLP utilities.

The server is written in Go. Tests use the standard `testing` package with inline fixtures. There are no external test frameworks. The test command is `go test ./...`.

## Files to Create

- `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/nlp/stemmer.go`
- `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/nlp/stemmer_test.go`
- `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/nlp/distance.go`
- `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/nlp/distance_test.go`
- `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/nlp/similarity.go`
- `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/nlp/similarity_test.go`

## Files to Modify

- The file containing `RepoContext` struct (likely under a `context/` or similar package with `types.go`) -- ensure the existing `Version` field is present and serialized via JSON. If `Version` is already declared but not tagged, add `json:"version"`. No new fields are needed; just verify the field exists and is usable.

## Tests (Write First)

All tests use the standard Go `testing` package with inline fixtures. No external test frameworks.

### `internal/nlp/stemmer_test.go`

```go
package nlp

import "testing"

// TestStemBasicWords verifies the Porter stemmer reduces common English words to expected stems.
// Test cases:
//   Stem("routing") returns "rout"
//   Stem("handlers") returns "handler"
//   Stem("structures") returns "structur"
//   Stem("validation") returns "valid"
func TestStemBasicWords(t *testing.T) {}

// TestStemEdgeCases verifies edge case handling.
// Test cases:
//   Stem("") returns "" (empty input)
//   Stem("x") returns "x" (single character unchanged)
//   Stem lowercases ASCII input
//   Stem preserves Unicode identifiers (passes through unchanged)
func TestStemEdgeCases(t *testing.T) {}

// TestStemCamelCase verifies camelCase splitting before stemming.
// Test cases:
//   Stem("ServeHTTP") splits camelCase → stems of ["serve", "http"]
//   Stem("CompressHandler") splits camelCase → stems of ["compress", "handler"]
func TestStemCamelCase(t *testing.T) {}

// TestSplitCamelCase verifies identifier splitting.
// Test cases:
//   SplitCamelCase("ServeHTTP") returns ["Serve", "HTTP"]
//   SplitCamelCase("getURL") returns ["get", "URL"]
//   SplitCamelCase("simple") returns ["simple"]
func TestSplitCamelCase(t *testing.T) {}
```

### `internal/nlp/distance_test.go`

```go
package nlp

import "testing"

// TestLevenshteinDistance verifies edit distance calculations.
// Test cases:
//   LevenshteinDistance("kitten", "sitting") returns 3
//   LevenshteinDistance("", "abc") returns 3
//   LevenshteinDistance("abc", "") returns 3
//   LevenshteinDistance("same", "same") returns 0
func TestLevenshteinDistance(t *testing.T) {}

// TestLevenshteinEarlyTermination verifies the function terminates
// early when accumulated cost exceeds a max threshold.
// Use LevenshteinDistanceMax(a, b, maxDist) variant or verify behavior
// with very different strings where early termination should kick in.
func TestLevenshteinEarlyTermination(t *testing.T) {}

// TestFuzzyMatch verifies candidate filtering by edit distance.
// Test cases:
//   FuzzyMatch("route", ["router", "handler", "routing"], 2) returns ["router", "routing"]
//   FuzzyMatch("xyz", ["abc", "def"], 1) returns empty slice
//   FuzzyMatch with empty candidates returns empty slice
//   FuzzyMatch with empty input returns candidates within maxDistance of ""
func TestFuzzyMatch(t *testing.T) {}
```

### `internal/nlp/similarity_test.go`

```go
package nlp

import "testing"

// TestConceptSimilarityBasic verifies fraction-based similarity scoring.
// Test cases:
//   ConceptSimilarity with all words in domain returns 1.0
//   ConceptSimilarity with no words in domain returns 0.0
//   ConceptSimilarity with half words in domain returns ~0.5
func TestConceptSimilarityBasic(t *testing.T) {}

// TestConceptSimilarityEdgeCases verifies safety for degenerate inputs.
// Test cases:
//   ConceptSimilarity with empty word list returns 0.0 (no division by zero)
//   ConceptSimilarity with empty domain profile returns 0.0
func TestConceptSimilarityEdgeCases(t *testing.T) {}

// TestConceptSimilarityStopWords verifies stop word filtering.
// Test cases:
//   Stop words ("Get", "Set", "New", "Handle", "Error") are excluded from scoring
//   A word list containing only stop words returns 0.0
func TestConceptSimilarityStopWords(t *testing.T) {}

// TestConceptSimilarityUseStemmer verifies internal stemmer usage.
// Test cases:
//   "routing" matches "rout" in domain profile (stemmer normalizes both)
func TestConceptSimilarityUseStemmer(t *testing.T) {}
```

## Implementation Details

### Package: `internal/nlp`

All three files live in the `nlp` package. No external dependencies -- only the Go standard library.

### `stemmer.go` -- Custom Porter Stemmer

**Exported Functions:**

- `Stem(word string) string` -- Porter stemmer reducing English words to stems
- `SplitCamelCase(s string) []string` -- splits Go-style identifiers into word parts
- `StemAll(words []string) []string` -- convenience: applies Stem to each word

**Behavior:**

- Empty strings return empty. Single-character words return unchanged.
- ASCII input is lowercased before stemming. Unicode identifiers pass through unchanged.
- CamelCase splitting happens before stemming: `"ServeHTTP"` splits to `["Serve", "HTTP"]`, each part is stemmed individually. The `Stem` function itself should handle single words; provide `StemAll` or `StemIdentifier` for camelCase-aware stemming of identifiers.
- Implement the standard Porter stemmer steps (step 1a, 1b, 1c, 2, 3, 4, 5a, 5b). The algorithm is well-documented at https://tartarus.org/martin/PorterStemmer/. Key suffixes to handle:
  - Step 1a: `sses` -> `ss`, `ies` -> `i`, `s` -> (remove if preceded by vowel-containing stem)
  - Step 1b: `eed` -> `ee` (if measure > 0), `ed` -> (remove if stem has vowel), `ing` -> (remove if stem has vowel)
  - Step 2: `ational` -> `ate`, `tion` -> `tion`, `izer` -> `ize`, etc.
  - Step 3: `icate` -> `ic`, `ful` -> (remove), `ness` -> (remove), etc.
  - Step 4: `al` -> (remove), `ance` -> (remove), `tion` -> (remove if preceded by `t`), etc.
  - Step 5: final `e` removal, double `l` reduction

- Expected outputs for key test words: `routing` -> `rout`, `handlers` -> `handler`, `structures` -> `structur`, `validation` -> `valid`

**`SplitCamelCase` algorithm:**

Iterate through the string tracking transitions:
- lowercase-to-uppercase boundary: split before the uppercase letter
- uppercase-to-uppercase-to-lowercase boundary (like `URL` followed by `Parser`): split before the last uppercase that precedes a lowercase
- Example: `"ServeHTTP"` -> `["Serve", "HTTP"]`, `"getURL"` -> `["get", "URL"]`, `"simple"` -> `["simple"]`

### `distance.go` -- Levenshtein Distance

**Exported Functions:**

- `LevenshteinDistance(a, b string) int` -- standard edit distance
- `LevenshteinDistanceMax(a, b string, maxDist int) int` -- edit distance with early termination; returns `maxDist + 1` if actual distance exceeds `maxDist`
- `FuzzyMatch(input string, candidates []string, maxDistance int) []string` -- returns all candidates within `maxDistance` of `input`

**Behavior:**

- Standard dynamic programming implementation with a 2-row optimization (only keep current and previous rows, not the full matrix).
- Early termination in `LevenshteinDistanceMax`: after filling each row, check if the minimum value in that row exceeds `maxDist`. If so, return `maxDist + 1` immediately.
- `FuzzyMatch` uses `LevenshteinDistanceMax` internally for each candidate, collecting those where distance <= `maxDistance`.
- Empty string handling: distance from empty to a string of length N is N.

### `similarity.go` -- Concept Similarity Scoring

**Exported Functions:**

- `ConceptSimilarity(words []string, domainProfile map[string]bool) float64` -- fraction of non-stop words present in domain profile
- `BuildDomainProfile(names []string) map[string]bool` -- builds a stemmed domain profile from a list of identifiers
- `IsStopWord(word string) bool` -- checks if a word is in the stop word list

**Behavior:**

- **Stop word list** (hardcoded): `Get`, `Set`, `New`, `Make`, `Handle`, `Error`, `Init`, `Close`, `Open`, `Read`, `Write`, `String`, `Reset`, `Is`, `Has`. These are lowercased before comparison.
- **ConceptSimilarity algorithm:**
  1. Filter `words` to remove stop words (case-insensitive comparison against stop list)
  2. If no non-stop words remain, return 0.0
  3. Stem each remaining word using `Stem()`
  4. Count how many stemmed words appear in `domainProfile` (which is also pre-stemmed)
  5. Return `matchCount / totalNonStopWords` as float64
- **BuildDomainProfile:** takes a slice of identifier strings (package names, function names, type names, description words), splits camelCase, stems each part, and stores all stems in a `map[string]bool`. This gives O(1) lookup during similarity scoring.
- Division by zero protection: if `words` is empty or all words are stop words, return 0.0.

### Schema Versioning

The `RepoContext` struct (in the types file, likely at a path like `context/types.go` or similar) already has a `Version` field. Verify that:

1. The field exists: `Version int` (or similar integer type)
2. It is serialized: has a `json:"version"` tag (or `json:"version,omitempty"`)
3. Current data has `Version == 0` (the zero value for int, which is the default for existing data)

No new migration code is needed in this section -- the actual migration logic (re-keying functions with receiver-aware keys when `Version == 0`) is implemented in section-02-comparison-keys. This section only ensures the `Version` field is ready for use.

If the `Version` field does not have a JSON tag, add one: `json:"version"`. This ensures it survives serialization/deserialization. Use `json:"version"` (not `omitempty`) so that Version 0 is explicitly written, making it distinguishable from "field missing."

## Dependencies

- **None.** This section is the foundation; it has no dependencies on other sections.
- Sections 02, 03, and 04 depend on the `internal/nlp/` package created here.

## Verification

After implementation, run:

```bash
cd /Users/yashpalc/yashpalc-mcp/mcp-repo-context && go test ./internal/nlp/...
```

All tests in `stemmer_test.go`, `distance_test.go`, and `similarity_test.go` must pass. The package must compile cleanly with no external dependencies beyond the Go standard library.