# Section 07: Package Structure Grouping

## Overview

The `handlePackageQuery` function in `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/orchestrator/smart_query.go` (lines 891-1063) groups files by the first two path components of their relative path. For flat Go packages (no subdirectories), the file path IS the filename including extension, producing odd groupings like `.go/`, `.md/`, `.mod/` in the output headers.

This section replaces path-based grouping with purpose-based grouping and adds flat package optimization.

## Dependencies

- **None.** This section is independent and can be implemented in parallel with other sections.

## Files to Modify

- `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/orchestrator/smart_query.go` -- the `handlePackageQuery` function, specifically the file grouping logic at lines 933-950.

## Tests -- Write First

### File: `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/orchestrator/smart_query_test.go`

Add these tests (alongside any existing tests in that file):

```go
// Test: Flat package (single directory) shows flat file list, not grouped
// Given: A RepoContext with files all in the same directory (e.g., "mux/router.go", "mux/route.go", "mux/mux.go")
// When: handlePackageQuery is called for "mux"
// Then: Output does NOT contain directory group headers like "### `mux.go/`"
// And: Output lists files in a flat format

// Test: Flat package groups by purpose: source, tests, docs, config
// Given: A RepoContext with files "pkg/router.go", "pkg/router_test.go", "pkg/README.md", "pkg/config.yaml"
// When: handlePackageQuery is called for "pkg"
// Then: Output contains purpose-based group headers: "Source Code", "Tests", "Documentation", "Configuration"
// And: router.go appears under "Source Code"
// And: router_test.go appears under "Tests"
// And: README.md appears under "Documentation"
// And: config.yaml appears under "Configuration"

// Test: Multi-directory package groups by directory first, then purpose
// Given: A RepoContext with files spanning "pkg/handlers/auth.go", "pkg/handlers/auth_test.go", "pkg/models/user.go"
// When: handlePackageQuery is called for "pkg"
// Then: Output groups by directory ("handlers", "models") as top-level
// And: Within each directory, files are organized by purpose

// Test: Extension-based grouping (.go/, .md/) does NOT appear in output
// Given: A flat package with "router.go", "README.md", "go.mod"
// When: handlePackageQuery is called
// Then: Output does NOT contain ".go/" or ".md/" or ".mod/" as group headers

// Test: Deeply nested package (3+ levels) collapses to top 2 levels
// Given: Files at "pkg/a/b/c/deep.go", "pkg/a/b/shallow.go", "pkg/a/top.go"
// When: handlePackageQuery is called for "pkg"
// Then: Files deeper than 2 levels are collapsed into their 2nd-level parent
// And: "pkg/a/b/c/deep.go" appears under the "a/b" group

// Test: Single-file package still shows properly
// Given: A RepoContext with only one file "pkg/main.go"
// When: handlePackageQuery is called for "pkg"
// Then: Output shows the file without unnecessary grouping
```

These tests should construct `RepoContext` fixtures inline with mock `FileContext` entries, then call `handlePackageQuery` and assert on the formatted output string. Match the existing test style in the codebase (standard `testing` package, no external frameworks).

## Implementation Details

### Purpose-Based File Classification

Replace the current path-component-based grouping (lines 933-950) with a purpose classifier. Introduce a helper function `classifyFilePurpose` that takes a filename and returns one of five categories:

```go
// classifyFilePurpose returns the purpose category for a file based on its name/extension.
// Categories: "Source Code", "Tests", "Documentation", "Configuration", "Other"
func classifyFilePurpose(filename string) string
```

Classification rules (applied in order):

| Category       | Rule                                                                 |
|----------------|----------------------------------------------------------------------|
| Tests          | Ends with `_test.go`                                                 |
| Source Code    | Ends with `.go` (and not `_test.go`)                                 |
| Documentation  | Ends with `.md`                                                      |
| Configuration  | Ends with `.mod`, `.sum`, `.yml`, `.yaml`, `.json`, `.toml`; or filename is `Makefile`, `Dockerfile` |
| Other          | Everything else                                                      |

### Flat Package Optimization

After collecting all matching files, determine whether they span multiple directories or a single directory. Check this by examining the directory component of each file's path relative to the matched package path.

- **Single directory (flat package):** Group files by purpose only. Do not create directory-level headers. If all files fall into a single purpose category, show a flat list without even the purpose header.
- **Multiple directories:** Group first by directory (top 2 levels relative to the package path), then within each directory group by purpose.

### 2-Level Nesting Collapse

When files span multiple directories, collapse paths deeper than 2 levels into their 2nd-level parent. For example, given package path `pkg`:

- `pkg/a/file.go` -- directory is `a`
- `pkg/a/b/file.go` -- directory is `a/b`
- `pkg/a/b/c/file.go` -- directory is `a/b` (collapsed from `a/b/c`)

The collapse logic extracts the relative path within the package, splits on `/`, and takes at most the first 2 directory components.

### Refactored `handlePackageQuery` Flow

The overall flow of the refactored function:

1. Extract and normalize `packagePath` (unchanged from current code, lines 894-902).
2. Find all matching files (unchanged from current code, lines 910-916).
3. Handle empty results (unchanged, lines 918-922).
4. Sort files by path (unchanged, lines 924-927).
5. **NEW:** Determine if files are in a single directory or multiple directories.
6. **NEW:** If single directory, group by purpose using `classifyFilePurpose`.
7. **NEW:** If multiple directories, group by directory (2-level collapse), then by purpose within each directory.
8. Format output using the grouped structure.
9. Append navigation hints (unchanged, lines 1046-1051).

### Output Format

For a **flat package** (single directory):

```
## Package Structure: `mux`

**Files found:** 5

### Source Code

#### `router.go`
**Path:** `mux/router.go`
...

#### `route.go`
**Path:** `mux/route.go`
...

### Tests

#### `router_test.go`
...
```

For a **multi-directory package**:

```
## Package Structure: `pkg`

**Files found:** 8

### `handlers/`

**Source Code:**
#### `auth.go`
...

**Tests:**
#### `auth_test.go`
...

### `models/`

**Source Code:**
#### `user.go`
...
```

### Key Implementation Notes

- The `fileInfo` struct (lines 905-908) remains unchanged.
- The per-file detail rendering (lines 969-1043 -- types summary, functions summary, side effects) remains unchanged. Only the grouping and header logic changes.
- Purpose categories with zero files in them should be omitted from output.
- The "Other" category should only appear if there are files that do not match any of the first four categories.
- Keep the existing 10-function limit per file and truncation logic intact.

## Verification

After implementation, verify:

1. `go test ./internal/orchestrator/... -run TestPackageStructure` (or whatever test name prefix you use) passes.
2. `go vet ./internal/orchestrator/...` reports no issues.
3. Manually test with `get_package_structure` on a flat Go package (e.g., gorilla/mux root) and confirm no `.go/` groupings appear.
4. Manually test with a multi-level package and confirm 2-level collapse works.