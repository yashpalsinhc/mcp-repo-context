# Implementation Plan: Core Bug Fixes & Quality

## Context

The MCP repo-context server analyzes Go codebases and exposes tools for querying function metadata, call graphs, and cross-repo comparisons. Testing against gorilla/mux, gorilla/handlers, and gorilla/sessions revealed 7 bugs where existing tools produce incorrect or misleading output. This plan fixes all 7, ordered by severity.

The server is written in Go. Tests use the standard `testing` package with inline fixtures. There are no external test frameworks. The test command is `go test ./...`.

---

## Section 1: Receiver-Aware Comparison Keys

### Problem

`normalizeFunctionKey()` in `internal/comparison/comparer.go:481` returns only `fn.Name`. This causes every method named `ServeHTTP`, `Write`, `Get`, etc. across different types to be treated as duplicates, conflicts, or gaps. Three tools are broken: `compare_repos`, `find_duplicates`, and `find_conflicts`.

Additionally, `normalizeTypeKey()` at line 486 has the same bug — it returns `td.Name`, causing types like `Handler`, `Config`, `Option` across different repos to be flagged as false duplicates.

### Current Data Model

`FunctionDef` (in `context/types.go:172`) has a `Receiver` field (string, e.g. `"*Router"`) that is populated during analysis but ignored in comparison.

### Approach

**Modify `normalizeFunctionKey()`** to produce keys in the format `ReceiverType.FunctionName` when a receiver exists, and plain `FunctionName` for package-level functions. Strip the pointer prefix `*` from receiver types so `*Router` and `Router` produce the same key.

**Modify `normalizeTypeKey()`** similarly — include package name or qualifying context to prevent false type duplicates across repos.

**Refactor all three comparison paths to use normalize functions:**

1. **Duplicate detection (lines 128-192):** The `funcMap` keyed by `normalizeFunctionKey()` will now naturally separate methods on different types. Two functions are duplicates only if they share the same receiver-qualified key.

2. **Conflict detection (`FindConflicts`, lines 203-212):** Currently uses `key := fn.Name` directly — must be refactored to call `normalizeFunctionKey()`. After receiver-aware keying, two methods are conflicts only if they have the same receiver type AND different signatures.

3. **Gap detection (`FindGaps`, lines 297-309):** Currently uses `targetFuncs[fn.Name] = true` directly — must be refactored to call `normalizeFunctionKey()`. With receiver-qualified keys, `Router.ServeHTTP` existing in the target no longer satisfies a gap check for `cors.ServeHTTP` in the source.

### Migration

Stored context data uses old name-only keys. Implement **idempotent lazy migration** triggered on first comparison tool use:

- When `compare_repos`, `find_duplicates`, or `find_conflicts` is called, check the `Version` field on the loaded repo context.
- If `Version < 1`, trigger migration: re-key all functions in memory using the new receiver-qualified format.
- Version guard: only persist if version was actually bumped (check-then-write), making migration idempotent and safe under concurrent access.
- Reuse the existing `Version` field on `RepoContext` (in `context/types.go:15`). This is a data-content version (per-repo-context), distinct from the SQLite `schema_migrations` table which tracks DB-schema-level changes.
- Current (pre-fix) data has `Version == 0`. After migration, version becomes `1`.

### Files to Modify

- `internal/comparison/comparer.go` — normalizeFunctionKey, normalizeTypeKey, duplicate, conflict, gap logic (refactor FindConflicts and FindGaps to use normalize functions)
- `context/types.go` — use existing Version field (ensure it's serialized)
- `internal/storage/` — update storage read path to check version for lazy migration

---

## Section 2: Domain-Aware Gap Analysis

### Problem

Gap analysis assumes the target repo should contain everything from source repos. Comparing complementary packages (gorilla/handlers vs gorilla/mux) produces 159 nonsensical "gaps" because functions like `CORS()` and `CompressHandler()` belong to handlers, not mux.

**Note:** Gap detection only runs when a `TargetRepoID` is specified. When comparing repos without a target, gap analysis is skipped entirely — this is existing behavior and unchanged by this fix.

### Approach

Add **concept similarity scoring** to gap detection. Instead of a boolean "is this function name present in target?", score each potential gap by how related it is to the target repo's domain.

**Concept similarity heuristic:**

1. Build a domain profile for the target repo: collect all package names, function names, type names, and descriptions. Stem these words using the custom Porter stemmer. Store in a `map[string]bool` for O(1) lookup.
2. **Stop word filtering:** Exclude common Go function name prefixes from the domain profile and gap scoring: `Get`, `Set`, `New`, `Make`, `Handle`, `Error`, `Init`, `Close`, `Open`, `Read`, `Write`, `String`, `Reset`, `Is`, `Has`. These appear in nearly every Go repo and would cause false similarity.
3. For each potential gap function from a source repo, stem its name and description words (after removing stop words).
4. Compute a similarity score: what fraction of the gap function's stemmed words appear in the target's domain profile map.
5. Only report gaps with similarity above a configurable threshold (default: 0.3).
6. Rank reported gaps by similarity score (higher = more likely a genuine gap).

**Custom Porter stemmer:** Implement a minimal Porter stemmer in a new `internal/nlp/` package. This is shared with smart_query (Section 3). The stemmer handles common Go identifier patterns: routing→rout, handlers→handler, structures→structur, validation→valid.

**Custom Levenshtein distance:** Implement Levenshtein distance in the same `internal/nlp/` package for fuzzy matching. Used when exact stemmed matches aren't found — catches typos and minor naming variations. Include early termination optimization: bail out of distance calculation once accumulated cost exceeds `maxDistance`.

No external NLP dependencies per stakeholder decision. (`golang.org/x/tools` used in Section 5 is Go's quasi-standard toolchain, a different category.)

### Files to Create

- `internal/nlp/stemmer.go` — custom Porter stemmer
- `internal/nlp/distance.go` — Levenshtein distance with early termination
- `internal/nlp/similarity.go` — concept similarity scoring with stop words and O(1) domain profile

### Files to Modify

- `internal/comparison/comparer.go` — gap detection logic to use similarity scoring

---

## Section 3: Smart Query NLP Improvements

### Problem

`internal/orchestrator/smart_query.go` uses regex-only pattern matching with hardcoded 0.8 confidence. Common questions like "How does routing work?" get misclassified as function lookups at 50% confidence and return wrong results.

### Approach

Five improvements:

**1. Logic reordering for ambiguous queries:**
The root cause for "How does routing work?" misclassification is that the regex `how does (\w+) work` greedily extracts any word as a function name. Stemming alone doesn't fix this. The fix: when `handleFunctionQuery` extracts a word but finds no matching function in the repo, fall through to concept search instead of returning a low-confidence "not found" result. This is a logic reordering, not just stemming.

**2. Word stemming as secondary signal:**
Use the custom Porter stemmer (from Section 2) to normalize query words. Stemming helps after the logic reorder — when falling through to concept search, "routing" stems to "rout" which matches "route"-related functions. Stemming also improves exact function lookups by matching `handlers` to `handler`.

**3. Common question pattern expansion:**
Add explicit patterns for common query shapes that currently fail:
- "what is the structure/architecture" → architecture query
- "how does X work" → concept search for X (when X is not a function name)
- "find all Y handlers/functions" → concept or side-effect search for Y
- "what calls X" / "who calls X" → caller search

**4. Confidence contract:**
Replace the hardcoded 0.8 confidence with a two-level system:
- **Parsing confidence** determines routing (which handler runs): exact regex match → 0.9, stemmed match → 0.75, fuzzy/partial → 0.6, below 0.6 → reject
- **Handler confidence** determines result quality: handler can lower confidence (e.g., function not found → 0.5) but cannot raise above parsing confidence
- Below 0.6 combined confidence: automatically route to the `ask` tool (AI-powered). Set `NeedsAI = true`. Do not return low-confidence wrong results.

**5. Fix path substring matching:**
Replace `strings.Contains(path, packagePath)` with proper boundary checking at `/` boundaries so `http` does not match `https`. Also fix the same bug in `handleFileQuery` (line 831) where `strings.HasSuffix(path, fileName) || strings.Contains(path, fileName)` has the same substring matching problem.

### Files to Modify

- `internal/orchestrator/smart_query.go` — query parsing, logic reordering, confidence scoring, fallback logic, package matching, file matching

### Files Used (from Section 2)

- `internal/nlp/stemmer.go` — Porter stemmer

---

## Section 4: Pattern Execution Fixes

### Problem

`internal/compose/patterns.go` and `internal/compose/chain.go` have two issues:
1. Conditional steps skip silently — no output explains why step 2 didn't run.
2. The impact_analysis pattern calls `get_function_context` without first resolving `file_path`.

### Approach

**Add step status tracking to ChainContext:**
Add a `StepResults` slice to `ChainContext` that records, for each step: name and one of three statuses:
- `executed` — step ran and produced results
- `skipped` — step's condition evaluated to false (with reason)
- `not_reached` — earlier step failed, this step never ran

Add a `Status` field to `ChainStep` to track this. After chain completion, include the full step results in the output.

**Fix impact_analysis pattern:**
Insert a search step (position 0) that takes the function name, calls `search_context` to find the file containing the function, then stores `file_path` in the chain context for the subsequent `get_function_context` step.

**Disambiguation for multi-result searches:** When `search_context` returns multiple results, use the first result with the highest confidence score. Include the chosen file path in the result message so the user knows which was selected. If the user needs a different file, they can specify `file_path` explicitly.

**Improve search_with_context result parsing:**
The transform function (patterns.go:144-150) assumes results are `[]map[string]any`. Add a type switch to handle other possible result formats gracefully. If result parsing fails, log the reason and mark step 2 as `skipped` with an explicit message.

**Partial completion output:**
When a chain completes with some steps skipped or failed, return a structured result that includes:
- Which steps ran successfully and their results
- Which steps were skipped and why (condition was false)
- Which steps were not reached and why (earlier step failed)
- Which steps failed and the error

### Files to Modify

- `internal/compose/chain.go` — add StepResults tracking with three-state status, partial completion output
- `internal/compose/patterns.go` — fix impact_analysis (add search step with disambiguation), improve result parsing

---

## Section 5: Call Graph Callee Extraction

### Problem

Method calls in Go source have `Type == "method"` in `CallRef` (set at `callgraph.go:188`). The resolution logic (line 265) only handles `Type == "internal"`, and CalledBy population (line 324) filters out methods because the condition `call.Type == "internal" || call.Type == "" || call.Package == ""` excludes methods (which have `Type == "method"` and `Package` set to the variable name like `"r"`). Result: `ServeHTTP` in gorilla/mux shows 32 callers but only 2 callees — the 13 internal method calls are invisible.

Additionally, `funcFile` map at `callgraph.go:47` uses `fn.Name` as key, causing collisions when two functions have the same name (different receivers). And `makeNodeID` produces `"file:function"` without receiver info, causing node map collisions.

### Approach

**Two-mode call graph extraction:**

**Mode 1: Heuristic (default)**
Improve method call resolution without `go/types`:
- Track local variable declarations in function bodies to infer receiver types.
- **Handled patterns:** function parameters with typed receivers, `var x SomeType` declarations, `x := SomeType{}` composite literals.
- **NOT handled (documented):** function return types (`x := SomeFunc()`), type assertions (`x.(Type)`), shadowed variables in nested scopes. These require `go/types` for correct resolution.
- When a method call `x.Method()` is found and `x` was declared as a handled pattern, resolve the call to `SomeType.Method`.
- Include `Type == "method"` calls in CalledBy population (expand the filter at line 324).
- For unresolvable method calls (receiver type unknown), still record them in the call graph as "unresolved method" rather than silently dropping them.

**Mode 2: go/types (opt-in via `--use-type-checker` flag)**
Full type-checked call graph:
- Use `go/types.Config` and `types.Check()` to type-check the package.
- Use `types.Info.Uses` to resolve every identifier to its declaration.
- For method calls, `Uses` gives the exact receiver type and method.
- For interface method calls, record the interface type.
- Requires module resolution — call `packages.Load()` or equivalent to resolve imports.
- If module resolution fails (private deps, network issues), fall back to heuristic mode with a warning log.

**Note on dependencies:** `golang.org/x/tools/go/packages` is from Go's quasi-standard toolchain. The "no external dependencies" constraint from Section 2 applies to third-party NLP libraries specifically. `golang.org/x` packages are acceptable as Go ecosystem tooling.

**CallRef data model improvement:**
Add a `Receiver` field to `CallRef` to disambiguate the overloaded `Package` field:

```go
type CallRef struct {
    Function string
    Package  string   // Package path for external calls
    Receiver string   // Receiver type for method calls (new field)
    File     string
    Line     int
    Type     string   // "internal", "stdlib", "external", "method"
}
```

**Fix node ID collisions:**
- Update `makeNodeID` to include receiver type: `"file:ReceiverType.function"` (or `"file:function"` for package-level functions).
- Update `funcFile` map to use receiver-qualified keys instead of plain function name.

### Files to Modify

- `internal/analyzer/callgraph.go` — call extraction, resolution, CalledBy population, makeNodeID, funcFile map
- `context/types.go` — add Receiver field to CallRef
- `internal/analyzer/go_analyzer.go` — add type-checker mode with flag
- `cmd/` or server config — add `--use-type-checker` flag

### New Dependencies

- `golang.org/x/tools/go/packages` — for loading typed packages in type-checker mode (Go ecosystem tooling, not third-party)

---

## Section 6: Package Structure Grouping

### Problem

`internal/orchestrator/smart_query.go:933-950` groups files by the first two path components of their relative path. For flat Go packages (no subdirectories), the file path IS the filename including extension, producing odd groupings like `.go/`, `.md/`, `.mod/`.

### Approach

Replace path-based grouping with **purpose-based grouping:**

| Group | Matching Rule |
|-------|---------------|
| Source Code | `.go` files not ending in `_test.go` |
| Tests | `_test.go` files |
| Documentation | `.md` files |
| Configuration | `.mod`, `.sum`, `.yml`, `.yaml`, `.json`, `.toml`, `Makefile`, `Dockerfile` |
| Other | Everything else |

**Flat package optimization:** If all files are in a single directory (no subdirectories), present a flat list instead of creating groups. This is the common case for Go packages.

**Subdirectory preservation:** If files span multiple directories, group first by directory, then within each directory by purpose. Show top 2 levels of nesting; collapse deeper levels into their 2nd-level parent.

### Files to Modify

- `internal/orchestrator/smart_query.go` — `handlePackageQuery` function, file grouping logic

---

## Section 7: Shared Infrastructure

### Custom NLP Package (`internal/nlp/`)

This is a new package shared by Sections 2, 3, and potentially other future features.

**stemmer.go:**
- `Stem(word string) string` — Porter stemmer reducing English words to stems
- Handles common Go identifier patterns (camelCase splitting before stemming)
- Handles common programming terms (e.g., "handlers" → "handler", "routing" → "rout")
- Edge cases: empty strings return empty, single-character words return unchanged, ASCII lowercased, Unicode identifiers passed through unchanged

**distance.go:**
- `LevenshteinDistance(a, b string) int` — edit distance between two strings, with early termination when accumulated cost exceeds a threshold
- `FuzzyMatch(input string, candidates []string, maxDistance int) []string` — find candidates within distance

**similarity.go:**
- `ConceptSimilarity(words []string, domainProfile map[string]bool) float64` — fraction of words present in domain profile map (O(1) per lookup)
- Uses stemmer internally for normalization
- Returns 0.0 for empty word lists (avoids division by zero)
- Includes a stop word list for common Go prefixes (Get, Set, New, Handle, Error, etc.)

### Schema Versioning

Reuse the existing `Version` field on `RepoContext` (in `context/types.go:15`). This is a data-content version, distinct from the SQLite `schema_migrations` table.

Current (pre-fix) data has `Version == 0`. After this fix, version becomes `1`. Migration from 0→1 re-keys functions with receiver-aware keys. Migration is idempotent (version guard prevents double-write).

---

## Implementation Order

```
Section 7 (shared infra)
    ↓
Section 1 (comparison keys + migration)
    ↓
Section 2 (gap analysis — uses nlp + new keys)
    ↓
Section 3 (smart_query — uses nlp)
    ↓
Section 4 (pattern execution — independent)
    ↓
Section 5 (call graph — independent, largest change)
    ↓
Section 6 (package structure — smallest change)
```

Sections 4, 5, and 6 are independent of each other and could be parallelized after Section 3 completes.

---

## Testing Strategy

Each section includes regression tests targeting the specific bug:

- **Section 1:** Tests with functions that have same name but different receivers. Verify they are NOT flagged as duplicates/conflicts. Also test normalizeTypeKey with same-name types across repos.
- **Section 2:** Tests comparing complementary packages. Verify low-similarity functions are NOT flagged as gaps. Test stop word filtering.
- **Section 3:** Tests with natural language queries that previously failed. Verify correct routing (logic reorder), AI fallback, and path boundary matching. Test handleFileQuery fix.
- **Section 4:** Tests that patterns log skipped steps with three-state status. Tests that impact_analysis resolves file_path. Test multi-result disambiguation.
- **Section 5:** Tests that method calls appear in callee lists. Tests for type-checker mode with sample Go module. Test makeNodeID with receiver-qualified IDs. Test funcFile map with receiver-qualified keys.
- **Section 6:** Tests that flat packages show logical grouping, not extension-based. Test deeply nested packages (2+ levels).
- **Section 7:** Unit tests for stemmer (including edge cases: empty, single-char, Unicode), Levenshtein (including early termination), similarity scoring (including stop words and empty inputs).

**Integration test:** End-to-end test analyzing gorilla/mux and gorilla/handlers, running `compare_repos`, and verifying gap count drops from 159 to a reasonable number. This is the acceptance criterion for the overall fix.

All tests use inline fixtures matching the existing test pattern (no external test framework).

---

## Risk Assessment

| Risk | Likelihood | Mitigation |
|------|-----------|------------|
| Migration corrupts stored data | Low | Idempotent migration with version guard; check-then-write pattern |
| go/types integration is complex | Medium | It's opt-in (flag defaults to OFF). Heuristic mode works without it. |
| Custom stemmer has edge cases | Medium | Porter stemmer is well-documented algorithm. Test with known word lists. |
| Pattern changes break existing chains | Low | Step status tracking is additive, not changing execution flow |
| CallRef schema change breaks storage | Medium | Add Receiver field as `omitempty` JSON — old data reads fine with empty field |
| Node ID format change breaks existing graphs | Low | Only affects in-memory representation; regenerated on next analysis |
