The hook encountered a path resolution error, but my task was to generate the section content. The content I produced above is the complete section for `section-02-gomod-parser`. Let me output it again cleanly so the system can capture it.

# Section 02: go.mod Parser

## Overview

This section creates a dedicated go.mod analyzer at `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/analyzer/gomod_analyzer.go` that parses go.mod files using `golang.org/x/mod/modfile` and returns a `ModuleInfo` struct. It also adds the `golang.org/x/mod` Go module dependency (already present in go.mod as an indirect dependency -- it needs to become a direct dependency).

## Dependencies

- **section-01-types-and-storage**: Must be completed first. This section defines the `ModuleInfo`, `ModuleDependency`, and `ModuleReplace` types in `internal/context/types.go` and the SQLite storage methods. This section (02) only creates the parser -- it does not touch types or storage.

## Background

The MCP repo-context server analyzes Go repositories but currently has no understanding of inter-repo dependencies. The `ArchitectureContext.Dependencies` field is always empty. The go.mod file contains structured dependency data (module path, Go version, require/replace directives) that this parser will extract into a `ModuleInfo` struct.

The project module path is `github.com/yashpalc/mcp-repo-context`. The `golang.org/x/mod` package is already in go.mod as an indirect dependency (v0.33.0), so adding it as a direct import will simply promote it.

## Types Used (Defined in Section 01)

The parser returns a `ModuleInfo` struct and its constituent types. These are defined in `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/context/types.go` by section-01:

```go
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

## Files to Create

### `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/analyzer/gomod_analyzer_test.go`

Tests come first. All tests use synthetic go.mod content (no real filesystem needed).

```go
package analyzer

import "testing"

// Test: Parse valid go.mod with direct and indirect dependencies
// - Provide a go.mod string with both direct and indirect require entries
// - Assert ModuleInfo.Dependencies contains all entries
// - Assert IsDirect is true for direct deps, false for indirect
func TestParseGoMod_ValidWithDeps(t *testing.T) {}

// Test: Parse go.mod with replace directives (version-specific)
// - Include a replace directive like: replace github.com/old => github.com/new v1.0.0
// - Assert ModuleInfo.Replaces contains the mapping
// - Assert the corresponding dependency has IsReplaced=true
func TestParseGoMod_ReplaceVersionSpecific(t *testing.T) {}

// Test: Parse go.mod with wildcard replace directive (no version on old)
// - Include: replace github.com/old => github.com/new v1.2.0
// - Assert Replaces entry has Old, New, Version populated correctly
func TestParseGoMod_ReplaceWildcard(t *testing.T) {}

// Test: Parse go.mod with local path replace directive
// - Include: replace github.com/foo => ../local-foo
// - Assert Replaces entry has New set to the local path, Version empty
func TestParseGoMod_ReplaceLocalPath(t *testing.T) {}

// Test: Parse go.mod with no dependencies (module declaration only)
// - Provide: module github.com/example/minimal\n\ngo 1.21
// - Assert ModuleInfo has correct ModulePath, GoVersion, empty Dependencies
func TestParseGoMod_NoDependencies(t *testing.T) {}

// Test: Handle malformed go.mod gracefully (return error, no panic)
// - Provide garbled content
// - Assert error is returned, result is nil, no panic
func TestParseGoMod_Malformed(t *testing.T) {}

// Test: Extract correct module path from go.mod
// - Assert ModuleInfo.ModulePath matches the module directive
func TestParseGoMod_ModulePath(t *testing.T) {}

// Test: Extract Go version from go.mod
// - Assert ModuleInfo.GoVersion matches the go directive
func TestParseGoMod_GoVersion(t *testing.T) {}

// Test: Distinguish direct vs indirect dependencies correctly
// - Provide go.mod with explicit // indirect comments
// - Assert IsDirect reflects the indirect flag correctly
func TestParseGoMod_DirectVsIndirect(t *testing.T) {}
```

### `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/analyzer/gomod_analyzer.go`

This is a new file in the existing `analyzer` package. It should NOT be placed inside `generic_analyzer.go` because go.mod parsing is complex enough to warrant its own file and uses `golang.org/x/mod/modfile` which has its own data structures.

**Public API:**

```go
package analyzer

import (
    "fmt"

    "golang.org/x/mod/modfile"
    ctx "github.com/yashpalc/mcp-repo-context/internal/context"
)

// GoModAnalyzer parses go.mod files into ModuleInfo structs.
type GoModAnalyzer struct{}

// NewGoModAnalyzer creates a new GoModAnalyzer.
func NewGoModAnalyzer() *GoModAnalyzer { ... }

// Parse takes the raw bytes of a go.mod file and returns structured ModuleInfo.
// Returns an error if the go.mod content is malformed.
// The filename parameter is used only for error messages (typically "go.mod").
func (a *GoModAnalyzer) Parse(filename string, content []byte) (*ctx.ModuleInfo, error) { ... }
```

**Implementation approach for `Parse`:**

1. Call `modfile.Parse(filename, content, nil)` -- the third argument is a "fix" function; pass `nil` to get strict parsing.
2. If parse returns an error, return `nil` and the error (wrapped with context using `fmt.Errorf` and `%w` wrapping `ErrGoModMalformed`).
3. Extract `file.Module.Mod.Path` as `ModulePath`. If `file.Module` is nil, return an error.
4. Extract `file.Go.Version` as `GoVersion`. If `file.Go` is nil, set to empty string.
5. Build a set of replaced module paths by iterating `file.Replace` -- map `rep.Old.Path` to true. This is used to mark dependencies as replaced.
6. Iterate `file.Require`:
   - `req.Mod.Path` becomes `ModuleDependency.Path`
   - `req.Mod.Version` becomes `ModuleDependency.Version`
   - `!req.Indirect` becomes `ModuleDependency.IsDirect` (note: `modfile` uses `Indirect` meaning the opposite of direct)
   - Check the replaced-module set to populate `IsReplaced`
7. Iterate `file.Replace`:
   - `rep.Old.Path` becomes `ModuleReplace.Old`
   - `rep.New.Path` becomes `ModuleReplace.New`
   - `rep.New.Version` becomes `ModuleReplace.Version` (empty for local path replacements)
8. Return the assembled `ModuleInfo`.

**Error handling:** If go.mod is malformed, the function returns an error. The caller (manager integration in section-05) is responsible for logging a warning and continuing analysis without dependency data. This parser does not log or swallow errors itself.

**Sentinel errors to define:**

```go
var (
    ErrGoModNotFound  = fmt.Errorf("go.mod file not found")
    ErrGoModMalformed = fmt.Errorf("go.mod file is malformed")
)
```

`ErrGoModNotFound` is used by the caller (manager) when no go.mod exists. `ErrGoModMalformed` can wrap the error returned by `modfile.Parse`.

## Go Module Dependency

The `golang.org/x/mod` package (v0.33.0) is already in `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/go.mod` as an indirect dependency. Once the import is added to `gomod_analyzer.go`, running `go mod tidy` will promote it to a direct dependency. No manual `go get` is needed.

## Integration Point

This parser is called from `manager.go` during `AnalyzeRepository()` (implemented in section-05). The manager checks if a `go.mod` file was found among analyzed files, reads its content, and calls `GoModAnalyzer.Parse()`. The returned `ModuleInfo` is then stored via `SQLiteStore.StoreModuleInfo()` (from section-01) and attached to the `RepoContext`.

## Verification

After implementation, the following should pass:

```
cd /Users/yashpalc/yashpalc-mcp/mcp-repo-context && go test ./internal/analyzer/ -run TestParseGoMod -v
```

All nine test cases listed above should pass. The tests are self-contained using synthetic go.mod strings and do not require any other sections to be complete, though the `ModuleInfo` type from section-01 must exist in `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/context/types.go`.