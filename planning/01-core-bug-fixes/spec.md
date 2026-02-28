# Spec: Core Bug Fixes & Quality

## Purpose

Make all existing MCP tools produce correct, reliable results. Currently, critical tools (comparison, smart_query, patterns, call graph) produce wrong or misleading output.

## Background

See `planning/mcp-server-gaps-requirements.md` for full testing methodology and detailed findings. Testing was done against gorilla/mux, gorilla/handlers, gorilla/sessions.

## Scope

### 1. Fix Comparison Receiver-Type Bug (Critical)
**File:** `internal/comparison/comparer.go:481`

`normalizeFunctionKey()` returns only `fn.Name`, ignoring the Go receiver type. This causes completely different methods on different types (e.g., `(*Router).ServeHTTP` vs `(*cors).ServeHTTP`) to be flagged as duplicates.

**Fix:** Include receiver type in the key. When a function has a receiver, the key should be `ReceiverType.FunctionName` (e.g., `Router.ServeHTTP`).

**Affected tools:** `compare_repos`, `find_duplicates`

### 2. Fix find_conflicts Interface Implementation Detection (Critical)
**File:** `internal/comparison/comparer.go` (conflict detection)

The tool flags 22 "conflicts" between gorilla repos, all of which are different types implementing the same Go interfaces. Methods on different receiver types implementing `http.Handler`, `io.Writer`, etc. are NOT conflicts.

**Fix:** Conflict detection must compare receiver types. Methods on different receivers are independent, not conflicting.

### 3. Fix find_gaps Semantic Model (High)
**File:** `internal/comparison/comparer.go` (gap detection)

Gap analysis assumes the target repo should contain EVERYTHING from source repos. This produces 159 nonsensical "gaps" when comparing complementary packages.

**Fix options (research during /deep-plan):**
- Only flag gaps for functions in the target's domain (same package name, similar concepts)
- Require user to specify what kind of functionality to look for
- Weight gaps by concept similarity, not just function name absence

### 4. Fix smart_query NLP (High)
**File:** `internal/orchestrator/smart_query.go`

Regex-based pattern matching with no stemming. Fails on common questions:
- "How does routing work?" → misclassified as function lookup for "routing" (50% confidence)
- "What is the project structure?" → misclassified as type lookup for "project" (50% confidence)

**Fix:**
- Add word stemming (routing→route, handlers→handler, structures→structure)
- Add common question patterns ("what is the structure" → architecture)
- Raise confidence threshold (reject <60% matches)
- Fallback to `ask` (AI) when confidence is below threshold

### 5. Fix execute_pattern Step Execution (Medium)
**File:** `internal/compose/patterns.go`, `internal/compose/chain.go`

- "search_with_context" only executes 1 step (conditional step 2 silently fails)
- "impact_analysis" fails: needs to search for function first to get file_path

**Fix:**
- Add logging/output when conditional steps are skipped and why
- impact_analysis: add search step before get_function_context
- Return clear message when pattern partially completes

### 6. Fix Call Graph Callee Extraction (Medium)
**File:** Go analyzer call extraction

`ServeHTTP` in gorilla/mux shows 32 callers but only 2 callees. The function calls ~13 internal functions but most don't appear.

**Fix:** Verify AST callee extraction captures:
- Direct function calls (`cleanPath(...)`)
- Method calls on named types (`r.Match(...)`)
- Function calls via variables (`handler.ServeHTTP(...)`)

### 7. Fix get_package_structure Grouping (Low)
**File:** Package structure generation code

Files are grouped by extension (.go/, .md/, .mod/) instead of logical purpose.

**Fix:** Group by: source code, tests, documentation, configuration. Or use flat list for single-package repos.

## Dependencies

- **None.** This is the foundation split. Other splits depend on this.

## Provides to Other Splits

- **02-dependency-graph:** Reliable comparison logic for cross-repo analysis
- **All splits:** Reliable smart_query, patterns, and call graph

## Testing Strategy

- Run the same gorilla/* test suite that found these bugs
- Each fix should include regression tests
- Verify all 25+ MCP tools still work correctly after changes

## Interview Decisions

- Go-only for now (no new language analyzers)
- Interleave bugs + features (this split is the "bugs" part)
