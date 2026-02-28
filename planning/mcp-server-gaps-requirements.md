# MCP Repo-Context Server: Gaps & Requirements

## Background

The MCP repo-context server provides pre-analyzed code context for repositories. It analyzes Go codebases, extracts function/type metadata, builds call graphs, and exposes tools for querying that context.

Testing was performed against three small, interconnected public Go repos: `gorilla/mux`, `gorilla/handlers`, and `gorilla/sessions`. These repos are commonly used together in Go web applications and share interface patterns (e.g., `http.Handler`).

## Current State

### What Works
- Repository analysis (analyze_repo, analyze_local, analyze_org)
- Function/type search (search_context) including cross-repo
- Deep function analysis (get_function_context) with call graphs, behavior summaries
- Caller detection (get_callers)
- Concept search (search_by_concept) and side-effect search (search_by_side_effect)
- AI-powered queries (ask) with cross-repo support
- Token-budgeted context (get_context_budgeted)
- AI summaries and architecture analysis
- Call graph visualization (Mermaid/DOT)
- Org registration and batch analysis
- Incremental file refresh (refresh_file, refresh_changed)

---

## Critical Bugs

### 1. Duplicate Detection Ignores Go Receiver Type
**Severity:** Critical
**File:** `internal/comparison/comparer.go:481`
**Impact:** compare_repos, find_duplicates produce completely wrong results

The `normalizeFunctionKey()` function returns only `fn.Name`, ignoring the receiver type. This causes every `ServeHTTP`, `Write`, `WriteHeader`, `Get`, `Name` method across different types to be flagged as a "duplicate that should be consolidated." These are unrelated interface implementations on different types.

**Example:** `(*Router).ServeHTTP`, `(*cors).ServeHTTP`, `(MethodHandler).ServeHTTP`, `(loggingHandler).ServeHTTP`, `(recoveryHandler).ServeHTTP` are all flagged as duplicates with recommendation "Consider consolidating into a shared package." This is nonsensical for Go code.

**Fix:** Include receiver type in the key: `receiver.Name` when available.

### 2. find_conflicts Flags Interface Implementations as Conflicts
**Severity:** Critical
**File:** `internal/comparison/comparer.go` (conflict detection logic)
**Impact:** find_conflicts produces misleading results

When comparing gorilla/handlers and gorilla/mux, the tool flags 22 "conflicts" — all of which are different types implementing the same Go interfaces (`http.Handler`, `http.ResponseWriter`). The tool treats `(*cors).ServeHTTP` and `(*Router).ServeHTTP` as a "signature mismatch" conflict even though they are completely independent types.

**Fix:** Conflict detection must be receiver-type-aware for Go. Methods on different receiver types are not conflicts.

### 3. find_gaps Semantic Model is Wrong
**Severity:** High
**File:** `internal/comparison/comparer.go` (gap detection logic)
**Impact:** find_gaps lists 159 "gaps" that are nonsensical

The gap analysis assumes the target repo should contain everything from source repos. When comparing gorilla/handlers + gorilla/sessions as sources against gorilla/mux as target, it lists `CORS()`, `CompressHandler()`, `NewCookieStore()`, etc. as "missing" from mux. These are separate packages with different responsibilities — they're not supposed to overlap.

**Fix:** Gap analysis needs a concept of "package responsibility scope." Consider only flagging gaps for functions that match the target's domain, or requiring the user to specify what kind of functionality to look for.

---

## High-Priority Gaps

### 4. No go.mod / Dependency Parsing
**Severity:** High
**File:** `internal/analyzer/go_analyzer.go`
**Impact:** Inter-repo dependency graph is invisible

The Go analyzer parses `.go` files via AST but completely ignores `go.mod`. This means:
- No module-level dependency information
- Cannot determine if repos import each other
- Architecture context `Dependencies` field is always empty (`internal/orchestrator/manager.go:generateArchitecture()`)
- AI queries about dependencies return "I cannot determine" because go.mod content is not indexed

**Requirements:**
- Parse go.mod for module path, Go version, require directives, replace directives
- Store dependencies in architecture context
- Enable dependency-aware cross-repo analysis ("which repos depend on each other?")
- Parse import statements from .go files to build an actual import graph

### 5. No File Content Storage for Config Files
**Severity:** High
**File:** `internal/analyzer/generic_analyzer.go`
**Impact:** Configuration files are metadata-only

The generic analyzer only stores: path, hash, language, size, line count, and a guessed "purpose" string. The actual content of go.mod, Dockerfile, docker-compose.yml, .env, Makefile, etc. is discarded.

When `get_context(scope=file)` is called for go.mod, it returns:
```
Language: go-mod, Lines: 4, Size: 39 bytes, Purpose: go-mod source file
```
No actual content.

**Requirements:**
- Store content for key configuration files (go.mod, go.sum hashes, Makefile, Dockerfile, CI configs)
- Alternatively, parse structured files into queryable metadata (go.mod -> dependencies, Dockerfile -> base images + stages)

### 6. smart_query NLP is Too Weak
**Severity:** High
**File:** `internal/orchestrator/smart_query.go`
**Impact:** Common natural language questions fail

The smart_query uses regex pattern matching with no stemming, fuzzy matching, or semantic understanding. Failures observed:
- "How does routing work?" → parsed as function lookup for "routing" (50% confidence), fails
- "What is the project structure?" → parsed as type lookup for "project" (50% confidence), fails
- "Find all HTTP handlers" → parsed as side_effect "http_call" (90%), partially correct but wrong intent

**Requirements:**
- Add word stemming (routing → route, handlers → handler)
- Add common question patterns: "what is the structure" → architecture, "how does X work" → concept search
- Improve confidence threshold — don't act on 50% confidence matches
- Fallback to `ask` (AI) when confidence is below threshold instead of giving wrong results

---

## Medium-Priority Gaps

### 7. Semantic Search Requires Manual Vector Store Setup
**Severity:** Medium
**File:** `internal/vectors/store.go`
**Impact:** Feature appears broken to users

`index_repository` fails with: "Semantic search is not enabled. Initialize the server with a vector store." No documentation explains the setup. The vector store uses SQLite with brute-force cosine similarity (loads all vectors, calculates similarity).

**Requirements:**
- Auto-initialize vector store on first use (or on server startup)
- Improve error message: explain what the user needs to do
- Consider if brute-force similarity is acceptable at scale

### 8. execute_pattern Silently Skips Steps
**Severity:** Medium
**File:** `internal/compose/patterns.go`, `internal/compose/chain.go`
**Impact:** Patterns appear to only run 1 step

The "search_with_context" pattern has a conditional second step that silently fails if result parsing fails. The "impact_analysis" pattern calls `get_function_context` without resolving `file_path` first — it needs to search for the function to find its file.

**Requirements:**
- Add logging/output when steps are skipped and why
- impact_analysis: add a search step before get_function_context to resolve file_path
- search_with_context: improve result parsing robustness
- Return clear message when a pattern partially completes

### 9. Architecture Context Missing Key Data
**Severity:** Medium
**File:** `internal/orchestrator/manager.go:generateArchitecture()`
**Impact:** get_context(scope=architecture) is sparse

The `Dependencies` field is never populated. `entry_points` only finds `main.go`. For library repos (like gorilla/mux), there are no main.go files so entry_points is null and main_packages is null.

**Requirements:**
- Populate Dependencies from go.mod (requires gap #4)
- For library repos: identify exported API surface as "entry points"
- Detect package type (library vs application) based on presence of main package

### 10. Call Graph Visualization Missing Callees
**Severity:** Medium
**File:** Call graph visualization code
**Impact:** Graphs are caller-heavy, callee-sparse

For `ServeHTTP` in gorilla/mux, the visualization shows 32 callers but only 2 callees (WriteHeader, Fprint). The actual function calls ~13 internal functions (cleanPath, requestWithVars, requestWithRouter, etc.) but these don't appear as callees.

**Requirements:**
- Verify callee extraction from function body AST
- Ensure internal function calls are included in call graph
- Check if method calls on other types are tracked

---

## Low-Priority Gaps

### 11. Go-Only Deep Analysis
**Severity:** Low (for Go-focused use), High (for general use)
**File:** `internal/analyzer/registry.go`
**Impact:** Non-Go repos get metadata-only analysis

Only Go has a proper analyzer with AST parsing, call graphs, side effects. Everything else uses `generic_analyzer.go` which guesses purpose from filename. No support for TypeScript, Python, Java, Rust.

**Requirements (future):**
- TypeScript/JavaScript analyzer (tree-sitter or ts-morph based)
- Python analyzer (AST-based)
- Consider tree-sitter as a universal parser backend

### 12. get_package_structure Groups by File Extension
**Severity:** Low
**File:** Package structure generation code
**Impact:** Odd grouping (.editorconfig/, .go/, .md/) instead of logical structure

For a flat Go package, files are grouped under extension-based headers like `.go/`, `.md/`, `.mod/`. This doesn't match how developers think about package structure.

**Requirements:**
- Group by logical purpose (source code, tests, documentation, config)
- Or present flat file list for single-package repos

---

## Testing Methodology

**Repos used:** gorilla/mux, gorilla/handlers, gorilla/sessions
**Tools tested:** analyze_repo, get_context, search_context, get_function_context, get_callers, search_by_concept, search_by_side_effect, compare_repos, find_duplicates, find_conflicts, find_gaps, smart_query, get_package_structure, visualize_call_graph, get_context_budgeted, ask, generate_ai_summary, generate_ai_arch_analysis, index_repository, semantic_search, execute_pattern, register_org, analyze_org, list_patterns

**Cross-cutting observations:**
- The server is fundamentally Go-centric, which is fine for Go repos but limits general adoption
- The multi-repo comparison features (compare, gaps, conflicts, duplicates) need the most work — they don't understand Go's type system
- Single-repo features work well (function analysis, callers, concept search, AI queries)
- The AI-powered features (ask, summary, arch analysis) are the strongest cross-repo tools
