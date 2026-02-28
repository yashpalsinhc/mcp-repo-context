# Complete Specification: Core Bug Fixes & Quality

## Overview

Fix 7 bugs in the MCP repo-context server that cause existing tools to produce incorrect or misleading results. These bugs span comparison logic, NLP query parsing, pattern execution, call graph extraction, and UI grouping. This is the foundation split — all other splits depend on reliable tool output.

## Priority Order (by severity)

1. **normalizeFunctionKey()** — Critical
2. **find_conflicts interface detection** — Critical
3. **find_gaps semantic model** — High
4. **smart_query NLP** — High
5. **execute_pattern step execution** — Medium
6. **Call graph callee extraction** — Medium
7. **get_package_structure grouping** — Low

## Bug 1: normalizeFunctionKey() (Critical)

**File:** `internal/comparison/comparer.go:481`
**Root cause:** Returns only `fn.Name`, ignoring receiver type, package, and parameters.
**Impact:** `compare_repos`, `find_duplicates` produce completely wrong results — unrelated methods like `(*Router).ServeHTTP` and `(*cors).ServeHTTP` flagged as duplicates.

**Fix:** Include receiver type in key: when `fn.Receiver` is non-empty, key becomes `ReceiverType.FunctionName` (strip pointer `*` prefix). Example: `Router.ServeHTTP`, `cors.ServeHTTP`.

**Migration:** Lazy migration on first comparison tool use. When `compare_repos` or `find_duplicates` is called, detect old name-only keys and re-key using the new format. No startup cost.

**Data model note:** `FunctionDef.Receiver` field (types.go:178) already stores receiver type as string (e.g., `"*User"`). This is available but unused in comparison.

## Bug 2: find_conflicts Interface Implementation Detection (Critical)

**File:** `internal/comparison/comparer.go` (conflict detection, lines 195-286)
**Root cause:** Uses `fn.Name` at line 208 for key lookup. Methods on different receiver types implementing the same interface are treated as conflicts.
**Impact:** 22 false "conflicts" between gorilla repos — all are independent `http.Handler` implementations.

**Fix:** Conflict detection must include receiver type in the key (same fix as Bug 1). After receiver-aware keying, two methods are only conflicts if they have the same receiver type AND different signatures.

## Bug 3: find_gaps Semantic Model (High)

**File:** `internal/comparison/comparer.go` (gap detection, lines 289-365)
**Root cause:** Uses `targetFuncs[fn.Name] = true` — assumes target should contain EVERYTHING from source repos.
**Impact:** 159 nonsensical "gaps" when comparing complementary packages.

**Fix (heuristic approach — architecture YAML deferred to later split):**
- Only flag gaps for functions where the source and target share domain concepts
- Use package name similarity and function name concept matching
- Custom Porter stemmer implementation for normalizing function/package names
- Weight gaps by concept similarity score, not just name absence
- Reject gaps with similarity below a threshold

**NOT in scope:** User-provided architecture YAML configuration for repo responsibility mapping (deferred to 02-dependency-graph or dedicated split).

## Bug 4: smart_query NLP (High)

**File:** `internal/orchestrator/smart_query.go`
**Root cause:** Regex-only pattern matching. Hardcoded 0.8 confidence. No stemming. Naive substring matching for package paths.

**Fix:**
1. **Add word stemming:** Custom Porter stemmer implementation (no external deps). Normalize query words: routing→rout, handlers→handler, structures→structur.
2. **Add common question patterns:** "what is the structure" → architecture, "how does X work" → concept search for X.
3. **Raise confidence threshold:** Reject matches below 60% confidence.
4. **Automatic AI fallback:** When confidence < 60%, automatically route to `ask` tool instead of returning wrong results.
5. **Fix package path matching:** Replace `strings.Contains` with proper path boundary checking (must match at `/` boundaries).

**No external dependencies.** Custom Porter stemmer and Levenshtein distance implementations.

## Bug 5: execute_pattern Step Execution (Medium)

**Files:** `internal/compose/patterns.go`, `internal/compose/chain.go`
**Root cause:** Conditional steps skip silently. Impact analysis has no search step.

**Fix:**
1. **Add logging:** When a conditional step is skipped, include reason in chain output (e.g., "Step 2 skipped: no search results").
2. **Fix impact_analysis:** Add a search step (position 0) before `get_function_context` to resolve `file_path` from function name.
3. **Fix search_with_context:** Improve result parsing robustness — handle different result formats gracefully.
4. **Return partial completion message:** When a pattern partially completes, return what succeeded and what was skipped/failed.

## Bug 6: Call Graph Callee Extraction (Medium)

**File:** `internal/analyzer/callgraph.go`
**Root cause:** Method calls have `Type == "method"` which is filtered out of resolution (line 265) and CalledBy population (line 324). Only `Type == "internal"` is resolved.

**Fix (configurable go/types integration):**
1. **Add `--use-type-checker` flag** (defaults to OFF / heuristic mode)
2. **Heuristic mode (default):** Improve method call resolution by:
   - Tracking local variable declarations to infer receiver types
   - Matching method calls to known types within the same package
   - Including `Type == "method"` in CalledBy population
3. **Type-checker mode (opt-in):** Use `go/types.Checker` for full type resolution:
   - Run module resolution (`go mod download` equivalent)
   - Use `types.Info.Uses` for accurate symbol resolution
   - Distinguish package functions from method calls definitively
4. **Graceful fallback:** If type-checker mode is enabled but module resolution fails, fall back to heuristic mode with a warning.

**CallRef.Package field:** Currently overloaded (stores both package paths and receiver type names). Add a `Receiver` field to disambiguate.

## Bug 7: get_package_structure Grouping (Low)

**File:** `internal/orchestrator/smart_query.go:933-950`
**Root cause:** Groups files by path segments (first two path components), producing odd extension-based groupings.

**Fix:** Group by logical purpose:
- **Source code:** `.go` files (excluding tests)
- **Tests:** `_test.go` files
- **Documentation:** `.md` files
- **Configuration:** `.mod`, `.sum`, `.yml`, `.yaml`, `.json`, `.toml` files
- For flat packages (single directory), use a flat file list instead of subdirectory grouping.

## Cross-Cutting Concerns

### Testing Strategy
- Run existing gorilla/* test suite to verify regressions
- Each fix includes regression tests targeting the specific bug
- Standard Go `testing` package, inline fixtures (matches existing patterns)
- Test command: `go test ./...`

### Backward Compatibility
- All existing MCP tool interfaces remain unchanged
- Migration logic handles old stored data seamlessly
- New `--use-type-checker` flag defaults to OFF (no behavioral change unless opted in)

### Shared Code
- Custom Porter stemmer: used by both smart_query (Bug 4) and gap analysis (Bug 3)
- Receiver-aware key generation: shared by duplicates (Bug 1), conflicts (Bug 2), and gaps (Bug 3)

### Performance
- Stemmer and Levenshtein are lightweight O(n) operations — negligible overhead
- go/types integration (when enabled) adds module resolution time — handled by making it opt-in
- Lazy migration avoids startup cost
