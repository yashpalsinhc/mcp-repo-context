The stop hook is failing due to a script path resolution issue -- it is trying to run `run ${CLAUDE_PLUGIN_ROOT}/scripts/hooks/write-section-on-stop.py` as a file path instead of interpreting `run` as a subcommand. This is not something I can fix from within this conversation.

The section content I generated is complete and ready. Here it is one final time for manual capture to `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/planning/02-dependency-graph/sections/section-04-import-aggregation.md`:

# Section 04: Import Aggregation

## Overview

This section creates the import aggregation system that classifies per-file Go imports into three categories (stdlib, internal, external) and resolves external imports to their module paths using go.mod data. The result is a per-repo `ImportSummary` stored in SQLite.

**File to create:** `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/analyzer/import_aggregator.go`
**Test file to create:** `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/analyzer/import_aggregator_test.go`

## Dependencies

- **section-01-types-and-storage:** Provides the `ModuleInfo`, `ModuleDependency`, `ModuleReplace`, `ImportSummary`, and `ExternalImport` types in `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/context/types.go`. Also provides the `StoreImportSummary` / `GetImportSummary` storage methods.
- **section-02-gomod-parser:** Provides the `GoModAnalyzer` that parses go.mod into a `ModuleInfo` struct with module path, dependencies, and replace directives.

This section does NOT modify the manager or any MCP tools. It produces a standalone package-level function that later sections (section-05) will call during the analysis flow.

## Background: Existing Import Data

The existing `FileContext` struct (in `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/context/types.go`) already contains per-file import information:

```go
type FileContext struct {
    // ...
    Imports []Import `json:"imports"`
    // ...
}

type Import struct {
    Path  string `json:"path"`
    Alias string `json:"alias,omitempty"`
}
```

The GoAnalyzer already extracts these imports from every `.go` file during analysis. The import aggregator's job is to collect all unique import paths across all files in a repo and classify them.

## Types Used (from section-01)

The aggregator produces an `ImportSummary` and consumes `ModuleInfo`:

```go
type ImportSummary struct {
    Stdlib   []string         `json:"stdlib"`
    Internal []string         `json:"internal"`
    External []ExternalImport `json:"external"`
}

type ExternalImport struct {
    ImportPath string `json:"import_path"`
    ModulePath string `json:"module_path"`
    Version    string `json:"version,omitempty"`
}

type ModuleInfo struct {
    ModulePath   string             `json:"module_path"`
    GoVersion    string             `json:"go_version"`
    Dependencies []ModuleDependency `json:"dependencies"`
    Replaces     []ModuleReplace    `json:"replaces,omitempty"`
}

type ModuleDependency struct {
    Path       string `json:"path"`
    Version    string `json:"version"`
    IsDirect   bool   `json:"is_direct"`
    IsReplaced bool   `json:"is_replaced,omitempty"`
}

type ModuleReplace struct {
    Old     string `json:"old"`
    New     string `json:"new"`
    Version string `json:"version,omitempty"`
}
```

## Tests

**File:** `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/analyzer/import_aggregator_test.go`

Write tests covering the following scenarios. Each test should construct the necessary input data (maps of `FileContext` and `ModuleInfo`) in-memory rather than reading from disk.

```go
package analyzer

import "testing"

// Test: Classify known stdlib imports (fmt, net/http, context)
// Build a set of FileContexts with imports like "fmt", "net/http", "context", "os".
// Call AggregateImports with a ModuleInfo whose ModulePath is "github.com/example/myapp".
// Assert all imports appear in ImportSummary.Stdlib.
// Assert ImportSummary.Internal and ImportSummary.External are empty.

// Test: Classify internal imports matching module path
// Build FileContexts with imports like "github.com/example/myapp/pkg/handlers",
// "github.com/example/myapp/internal/db".
// Call AggregateImports with ModuleInfo.ModulePath = "github.com/example/myapp".
// Assert these appear in ImportSummary.Internal.

// Test: Classify external imports not matching stdlib or module path
// Build FileContexts with imports like "github.com/gorilla/mux",
// "github.com/lib/pq". Provide ModuleInfo with matching require entries.
// Assert these appear in ImportSummary.External with correct ModulePath/Version.

// Test: Resolve external import to module path via longest prefix match
// Import path: "github.com/gorilla/mux/middleware" (subpackage).
// go.mod requires "github.com/gorilla/mux". The import should resolve to
// module path "github.com/gorilla/mux", not remain as the full import path.

// Test: Apply replace directives before import resolution
// Import path: "github.com/original/pkg/foo".
// go.mod has replace "github.com/original/pkg" => "github.com/fork/pkg" v1.2.0.
// The resolved ExternalImport should show module_path "github.com/fork/pkg"
// and version "v1.2.0".

// Test: Handle aliased imports correctly
// FileContext has Import{Path: "github.com/gorilla/mux", Alias: "router"}.
// The alias should be ignored for classification purposes; the import should
// still be classified as external based on its Path.

// Test: Handle import with no matching go.mod require (unresolved)
// Import path: "github.com/unknown/lib" but go.mod has no require for it.
// The import should still appear in External but with ModulePath set to the
// import path itself as a best-effort, and Version empty.

// Test: Aggregate imports across multiple files (deduplication)
// Two FileContexts both import "fmt" and "github.com/gorilla/mux".
// The resulting ImportSummary should contain each import path exactly once.

// Test: Handle repo with no go.mod (skip classification, return empty)
// Pass nil for ModuleInfo. The aggregator should return an empty ImportSummary
// (not panic).
```

## Implementation Details

### File: `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/analyzer/import_aggregator.go`

**Package:** `analyzer`

### Core Function Signature

```go
// AggregateImports collects all imports from analyzed files and classifies them
// as stdlib, internal, or external relative to the given module info.
// If moduleInfo is nil (no go.mod found), returns an empty ImportSummary.
func AggregateImports(files map[string]*context.FileContext, moduleInfo *context.ModuleInfo) *context.ImportSummary
```

### Algorithm

1. **Collect unique imports.** Iterate all `FileContext` values in the `files` map. For each file, iterate its `Imports` slice. Add each `Import.Path` to a `map[string]struct{}` for deduplication. The `Import.Alias` field is irrelevant for classification.

2. **Handle nil ModuleInfo.** If `moduleInfo` is nil, return an empty `ImportSummary` with nil/empty slices. Do not panic.

3. **Classify each unique import path.** For each path in the deduplicated set, apply these checks in order:
   - **Stdlib check:** Call `isStdlib(path)` (see below). If true, append to `Stdlib`.
   - **Internal check:** Check if the import path starts with `moduleInfo.ModulePath + "/"` or equals `moduleInfo.ModulePath`. If true, append to `Internal`.
   - **External:** Everything else. Resolve to a module path (see step 4), then append to `External`.

4. **Resolve external imports to module paths.** For each external import path:
   - First, check replace directives. Iterate `moduleInfo.Replaces`. If the import path starts with `replace.Old + "/"` or equals `replace.Old`, substitute the prefix with `replace.New` and note the replacement version.
   - Then, find the matching require entry via **longest prefix match**. Iterate `moduleInfo.Dependencies`. For each dependency, check if the (possibly replaced) import path starts with `dep.Path + "/"` or equals `dep.Path`. Track the longest matching `dep.Path`. Use its version.
   - If no require entry matches, set `ModulePath` to the import path itself (best-effort) and leave `Version` empty.

5. **Sort the output slices.** Sort `Stdlib`, `Internal`, and `External` (by import path) for deterministic output and easier testing.

### Stdlib Detection

Build a static set of known Go stdlib package paths. The implementation should combine two approaches.

**Primary: Static compiled-in set.** Maintain a `var stdlibPackages = map[string]bool{...}` with all known stdlib packages. This avoids any runtime dependency on the `go` binary. Include all well-known packages and their subpackages: `fmt`, `os`, `io`, `net`, `net/http`, `net/http/httptest`, `net/http/httputil`, `net/url`, `context`, `strings`, `strconv`, `encoding/json`, `encoding/xml`, `encoding/base64`, `encoding/csv`, `encoding/hex`, `database/sql`, `sync`, `sync/atomic`, `time`, `path`, `path/filepath`, `log`, `errors`, `bytes`, `bufio`, `crypto`, `crypto/tls`, `crypto/sha256`, `crypto/rand`, `reflect`, `regexp`, `sort`, `math`, `math/rand`, `testing`, `flag`, `html`, `html/template`, `text/template`, `archive/zip`, `archive/tar`, `compress/gzip`, `compress/flate`, `unicode`, `unicode/utf8`, `runtime`, `debug`, `embed`, `go/ast`, `go/parser`, `go/token`, `go/format`, `go/build`, `os/exec`, `os/signal`, `io/fs`, `io/ioutil`, and others.

**Fallback: No-dots heuristic.** If a package is not in the static set, check whether the first path segment (before the first `/`) contains a dot. Stdlib packages never have dots in their first segment (e.g., `"fmt"`, `"net"`, `"encoding"`), while third-party packages always do (e.g., `"github.com"`, `"golang.org"`). This catches any stdlib packages missing from the static set.

A helper function:

```go
// isStdlib returns true if the import path is a Go standard library package.
func isStdlib(path string) bool
```

### Replace Directive Handling

Replace directives in go.mod can take several forms:
- **Version-specific:** `replace github.com/old v1.0.0 => github.com/new v1.1.0` (only applies to a specific version)
- **Wildcard:** `replace github.com/old => github.com/new v1.1.0` (applies to all versions)
- **Local path:** `replace github.com/old => ../local-fork` (local filesystem path)

For import resolution, treat all replace forms the same: if an import path matches the `Old` prefix, substitute with `New`. Local path replacements should still be recorded but the resolved module path will be the local path string (this is expected behavior for development setups).

A helper function:

```go
// applyReplaces checks if the import path matches any replace directive
// and returns the (possibly modified) path and version.
func applyReplaces(importPath string, replaces []context.ModuleReplace) (string, string)
```

### Edge Cases

- **Empty files map:** Return empty `ImportSummary`.
- **Files with no imports:** Skip them during iteration.
- **Duplicate imports across files:** Handled by the deduplication map in step 1.
- **Import path equals module path exactly:** Classified as internal (this is the root package).
- **Nested stdlib packages:** `"net/http/httptest"` is stdlib. The heuristic (no dots in first segment) handles this since `"net"` has no dots.
- **C pseudo-package:** The import path `"C"` (used for cgo) should be treated as stdlib or simply skipped.

## Integration Point

This function is called by the manager (implemented in section-05) after both file analysis and go.mod parsing are complete. The call site in the analysis flow will look like:

```go
// In manager.go AnalyzeRepository flow (section-05 adds this):
importSummary := analyzer.AggregateImports(repoContext.Files, moduleInfo)
// Then stored via: store.StoreImportSummary(repoID, importSummary)
```

The import aggregator itself has no side effects -- it is a pure function that takes input data and returns a classified summary. Storage is handled by the caller.