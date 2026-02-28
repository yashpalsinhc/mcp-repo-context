Those sections haven't been written yet. I have sufficient context now to write the section. Let me produce the output.

# Section 05: Architecture Updates

## Overview

This section updates the `generateArchitecture()` method in `internal/orchestrator/manager.go` to populate the Dependencies field, detect package type (library vs application), identify library entry points, and include the Go version. It also adds incremental refresh support so that `RefreshFile` and `RefreshChangedFiles` properly handle go.mod and config file changes.

## Dependencies

- **Section 01 (Types and Storage):** Provides the `ModuleInfo`, `ImportSummary`, and `ConfigFile` types in `internal/context/types.go`, plus the SQLite store methods (`StoreModuleInfo`, `StoreImportSummary`, `StoreConfigFileContent`, etc.) and migration 004.
- **Section 02 (go.mod Parser):** Provides `GoModAnalyzer` in `internal/analyzer/gomod_analyzer.go` that parses go.mod content into `ModuleInfo`.
- **Section 04 (Import Aggregation):** Provides `ImportAggregator` in `internal/analyzer/import_aggregator.go` that classifies imports into stdlib/internal/external.

## Background Context

### Current State of `generateArchitecture()`

The method lives at `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/orchestrator/manager.go` (line ~408). Currently it:

1. Groups files by directory to create `Module` entries
2. Detects `main.go` files and adds them to `MainPackages` and `EntryPoints`
3. Detects build system from filenames (go.mod, Makefile, package.json)
4. Generates a simple overview string
5. **Does NOT populate the `Dependencies` field** (always empty `[]string`)
6. **Does NOT detect package type** (library vs application)
7. **Does NOT include Go version**
8. **Does NOT add library entry points** (only main entry points)

### Current `ArchitectureContext` Type

Defined in `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/context/types.go` (line ~322):

```go
type ArchitectureContext struct {
    Overview        string           `json:"overview"`
    Modules         []Module         `json:"modules"`
    EntryPoints     []EntryPoint     `json:"entry_points"`
    Dependencies    []string         `json:"dependencies"`
    BuildSystem     string           `json:"build_system"`
    MainPackages    []string         `json:"main_packages"`
    AIAnalysis      *AIArchAnalysis  `json:"ai_analysis,omitempty"`
}
```

The `Dependencies` field is `[]string` -- this section populates it with module paths from `ModuleInfo.Dependencies`.

### Current Manager Interface

The `Manager` interface is in `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/orchestrator/interface.go`. It already declares:

- `RefreshFile(ctx, projectID, filePath string, opts RefreshFileOptions) (*RefreshFileResult, error)`
- `RefreshChangedFiles(ctx, projectID string) ([]RefreshFileResult, error)`

These methods need to be updated (or their implementations extended) to handle go.mod and config files.

### Current `AnalyzeRepo()` Flow

In `manager.go` (line ~59), the flow is:
1. Clone repository
2. Scan and analyze files (per-file analysis)
3. `generateArchitecture(repoCtx)` -- **this is where we hook in**
4. Build call graph
5. Build search index
6. Store context

### `RepoContext` Struct

Currently has no `ModuleInfo`, `ImportSummary`, or `ConfigFiles` fields. Section 01 adds these:

```go
type RepoContext struct {
    // ... existing fields ...
    ModuleInfo    *ModuleInfo    `json:"module_info,omitempty"`
    ImportSummary *ImportSummary `json:"import_summary,omitempty"`
    ConfigFiles   []ConfigFile   `json:"config_files,omitempty"`
}
```

## Tests

### File: `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/orchestrator/manager_test.go` (extend existing or create new test file)

The architecture updates are tested through the manager and through focused unit tests on the updated `generateArchitecture()` logic. The incremental refresh tests verify that go.mod and config file changes trigger re-parsing.

```go
package orchestrator

// === Architecture Generation Tests ===

// Test: generateArchitecture populates Dependencies from ModuleInfo
// Setup: Create a RepoContext with ModuleInfo containing 3 direct dependencies.
// Call generateArchitecture.
// Assert: arch.Dependencies contains the 3 module paths.
// Assert: Dependencies list only includes direct dependencies (IsDirect == true).

// Test: generateArchitecture detects "application" package type when main package exists
// Setup: Create a RepoContext with a file whose Package field is "main".
// Assert: The architecture identifies the repo as package type "application".

// Test: generateArchitecture detects "library" package type when no main package
// Setup: Create a RepoContext with files that have packages like "mux", "handlers" but no "main".
// Assert: The architecture identifies the repo as package type "library".

// Test: generateArchitecture includes library entry points for libraries
// Setup: Create a RepoContext for a library (no main package) with exported functions
//   in the root package.
// Assert: arch.EntryPoints contains entries with Type "export" for each public function/type.

// Test: generateArchitecture includes Go version from ModuleInfo
// Setup: Create a RepoContext with ModuleInfo.GoVersion = "1.21".
// Assert: The architecture overview includes "Go 1.21".

// Test: generateArchitecture handles nil ModuleInfo gracefully
// Setup: Create a RepoContext with ModuleInfo = nil.
// Assert: arch.Dependencies is empty (not nil). No panic.

// === Incremental Refresh Tests ===

// Test: RefreshFile on go.mod re-parses ModuleInfo
// Setup: Store initial repo context with a go.mod file.
//   Update go.mod content to add a new dependency.
//   Call RefreshFile with filePath = "go.mod".
// Assert: The stored ModuleInfo is updated with the new dependency.
// Assert: ImportSummary is re-aggregated.

// Test: RefreshFile on Dockerfile updates config file content
// Setup: Store initial repo context with a Dockerfile.
//   Update Dockerfile to change base image.
//   Call RefreshFile with filePath = "Dockerfile".
// Assert: The stored ConfigFile has updated content and structured_json.

// Test: RefreshChangedFiles detects go.mod changes and re-parses
// Setup: Store initial repo context. Modify go.mod on disk (change hash).
//   Call RefreshChangedFiles.
// Assert: ModuleInfo is updated.
// Assert: The go.mod file appears in the returned results as refreshed.
```

## Implementation Details

### Part 1: Update `generateArchitecture()` Signature and Logic

**File to modify:** `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/orchestrator/manager.go`

The `generateArchitecture` method currently takes only `repoCtx *ctxpkg.RepoContext`. Since Section 01 adds `ModuleInfo`, `ImportSummary`, and `ConfigFiles` to `RepoContext`, the method can access them directly through `repoCtx.ModuleInfo` without changing its signature.

Update the method body to add four new behaviors:

#### 1a. Populate Dependencies Field

After creating the `arch` struct, check if `repoCtx.ModuleInfo` is non-nil. If so, iterate `repoCtx.ModuleInfo.Dependencies` and collect all direct dependencies (where `IsDirect == true`) into `arch.Dependencies` as a `[]string` of module paths.

```go
// Populate dependencies from ModuleInfo (only direct deps)
if repoCtx.ModuleInfo != nil {
    for _, dep := range repoCtx.ModuleInfo.Dependencies {
        if dep.IsDirect {
            arch.Dependencies = append(arch.Dependencies, dep.Path)
        }
    }
}
```

#### 1b. Detect Package Type

Scan all files in `repoCtx.Files` for any file whose `Package` field equals `"main"`. If found, the repo is an "application". Otherwise, it is a "library".

This requires a new field on `ArchitectureContext`. However, the plan says to use the existing `DependencyNode.PackageType` for graph nodes (Section 06). For architecture context, store the package type as a simple string. Either:
- Add a `PackageType string` field to `ArchitectureContext` (preferred, Section 01 should include this), or
- Embed the info in the Overview string.

The recommended approach is to add `PackageType string` to `ArchitectureContext` in types.go (coordinate with Section 01):

```go
type ArchitectureContext struct {
    // ... existing fields ...
    PackageType string `json:"package_type,omitempty"` // "library" or "application"
    GoVersion   string `json:"go_version,omitempty"`
}
```

Detection logic:

```go
// Detect package type
hasMainPackage := false
for _, fileCtx := range repoCtx.Files {
    if fileCtx.Package == "main" {
        hasMainPackage = true
        break
    }
}
if hasMainPackage {
    arch.PackageType = "application"
} else {
    arch.PackageType = "library"
}
```

#### 1c. Library Entry Points

For libraries (no main package), scan the root package for exported functions and types, and add them as entry points. The "root package" is identified by files in the repository root directory (path has no directory separator, or directory is ".").

```go
// For libraries, add exported functions/types as entry points
if arch.PackageType == "library" {
    // Determine root package name from module path
    for path, fileCtx := range repoCtx.Files {
        dir := filepath.Dir(path)
        if dir != "." {
            continue // only root package
        }
        for _, fn := range fileCtx.Functions {
            if fn.IsPublic {
                arch.EntryPoints = append(arch.EntryPoints, ctxpkg.EntryPoint{
                    Path:    path,
                    Type:    "export",
                    Purpose: fn.Signature,
                })
            }
        }
        for _, t := range fileCtx.Types {
            if t.IsPublic {
                arch.EntryPoints = append(arch.EntryPoints, ctxpkg.EntryPoint{
                    Path:    path,
                    Type:    "export",
                    Purpose: fmt.Sprintf("type %s (%s)", t.Name, t.Kind),
                })
            }
        }
    }
}
```

#### 1d. Include Go Version

If `repoCtx.ModuleInfo` is non-nil and has a `GoVersion`, store it:

```go
if repoCtx.ModuleInfo != nil && repoCtx.ModuleInfo.GoVersion != "" {
    arch.GoVersion = repoCtx.ModuleInfo.GoVersion
}
```

Also update the overview string to include Go version and dependency count:

```go
overview := fmt.Sprintf(
    "Repository with %d files across %d modules. %d public exports, %d types, %d functions.",
    repoCtx.Statistics.TotalFiles,
    len(arch.Modules),
    repoCtx.Statistics.ExportCount,
    repoCtx.Statistics.TypeCount,
    repoCtx.Statistics.FunctionCount,
)
if arch.GoVersion != "" {
    overview += fmt.Sprintf(" Go version: %s.", arch.GoVersion)
}
if len(arch.Dependencies) > 0 {
    overview += fmt.Sprintf(" %d direct dependencies.", len(arch.Dependencies))
}
arch.Overview = overview
```

### Part 2: Update `AnalyzeRepo()` Flow

**File to modify:** `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/orchestrator/manager.go`

After the file scan loop (line ~158) and before `generateArchitecture()` (line ~164), insert three new steps:

#### 2a. Parse go.mod

```go
// After file scanning, check for go.mod and parse it
if fileCtx, ok := repoCtx.Files["go.mod"]; ok {
    // Read go.mod content from disk
    goModPath := filepath.Join(localPath, "go.mod")
    goModContent, err := os.ReadFile(goModPath)
    if err == nil {
        modInfo, err := analyzer.ParseGoMod(goModContent)
        if err != nil {
            result.Warnings = append(result.Warnings,
                fmt.Sprintf("Failed to parse go.mod: %v", err))
        } else {
            repoCtx.ModuleInfo = modInfo
        }
    }
    _ = fileCtx // suppress unused warning
}
```

`analyzer.ParseGoMod` is provided by Section 02 in `internal/analyzer/gomod_analyzer.go`.

#### 2b. Parse Config Files

```go
// Parse config files
for path, fileCtx := range repoCtx.Files {
    configType := analyzer.DetectConfigType(path)
    if configType == "" {
        continue
    }
    absPath := filepath.Join(localPath, path)
    content, err := os.ReadFile(absPath)
    if err != nil {
        continue
    }
    cf, err := analyzer.ParseConfigFile(path, configType, content)
    if err != nil {
        result.Warnings = append(result.Warnings,
            fmt.Sprintf("Failed to parse config %s: %v", path, err))
        continue
    }
    repoCtx.ConfigFiles = append(repoCtx.ConfigFiles, *cf)
    _ = fileCtx
}
```

`analyzer.DetectConfigType` and `analyzer.ParseConfigFile` are provided by Section 03 in `internal/analyzer/config_parsers.go`.

#### 2c. Aggregate Imports

```go
// Aggregate imports (requires ModuleInfo for classification)
if repoCtx.ModuleInfo != nil {
    importSummary := analyzer.AggregateImports(repoCtx.Files, repoCtx.ModuleInfo)
    repoCtx.ImportSummary = importSummary
}
```

`analyzer.AggregateImports` is provided by Section 04 in `internal/analyzer/import_aggregator.go`.

#### 2d. Persist New Data

After `m.store.StoreRepoContext()` (or as part of the store operation if StoreRepoContext already serializes the full RepoContext), persist the new fields. If the store method serializes the entire RepoContext as JSON (which is the existing pattern), no additional persist calls are needed -- the new fields on RepoContext will be serialized automatically.

However, if config file content is stored separately (in the `files` table columns added by migration 004), add explicit storage calls:

```go
// Store config file data in files table
for _, cf := range repoCtx.ConfigFiles {
    if err := m.store.StoreConfigFileContent(repoID, cf.Path, cf.Content, cf.StructuredJSON, cf.Type); err != nil {
        result.Warnings = append(result.Warnings,
            fmt.Sprintf("Failed to store config %s: %v", cf.Path, err))
    }
}
// Store module info separately for batch queries
if repoCtx.ModuleInfo != nil {
    if err := m.store.StoreModuleInfo(repoID, repoCtx.ModuleInfo); err != nil {
        result.Warnings = append(result.Warnings,
            fmt.Sprintf("Failed to store module info: %v", err))
    }
}
if repoCtx.ImportSummary != nil {
    if err := m.store.StoreImportSummary(repoID, repoCtx.ImportSummary); err != nil {
        result.Warnings = append(result.Warnings,
            fmt.Sprintf("Failed to store import summary: %v", err))
    }
}
```

### Part 3: Incremental Refresh Support

**File to modify:** The file(s) implementing `RefreshFile` and `RefreshChangedFiles` on the `manager` struct. Based on the interface declaration in `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/orchestrator/interface.go`, these methods exist but their implementation file needs to be located or created.

The `RefreshFile` method must detect when the file being refreshed is a go.mod or a config file and trigger additional processing.

#### 3a. RefreshFile for go.mod

When `filePath == "go.mod"`:

1. Read the updated go.mod content from disk
2. Call `analyzer.ParseGoMod(content)` to get new `ModuleInfo`
3. Load the existing `RepoContext` from the store
4. Update `repoCtx.ModuleInfo` with the new value
5. Re-aggregate imports by calling `analyzer.AggregateImports(repoCtx.Files, repoCtx.ModuleInfo)`
6. Update `repoCtx.ImportSummary`
7. Re-generate architecture via `m.generateArchitecture(repoCtx)` to update Dependencies
8. Persist updates via `m.store.StoreModuleInfo()` and `m.store.StoreImportSummary()`

Pseudocode for the go.mod branch inside RefreshFile:

```go
// Inside RefreshFile, after detecting filePath == "go.mod":
content, err := os.ReadFile(absPath)
if err != nil {
    return nil, fmt.Errorf("failed to read go.mod: %w", err)
}
modInfo, err := analyzer.ParseGoMod(content)
if err != nil {
    return nil, fmt.Errorf("failed to parse go.mod: %w", err)
}
repoCtx.ModuleInfo = modInfo
repoCtx.ImportSummary = analyzer.AggregateImports(repoCtx.Files, modInfo)
repoCtx.Architecture = m.generateArchitecture(repoCtx)
// Persist
m.store.StoreModuleInfo(projectID, modInfo)
m.store.StoreImportSummary(projectID, repoCtx.ImportSummary)
m.store.StoreRepoContext(ctx, projectID, repoCtx)
```

#### 3b. RefreshFile for Config Files

When `analyzer.DetectConfigType(filePath)` returns a non-empty string:

1. Read the updated file content from disk
2. Call `analyzer.ParseConfigFile(filePath, configType, content)` to get new structured data
3. Update the corresponding entry in `repoCtx.ConfigFiles` (or append if new)
4. Persist via `m.store.StoreConfigFileContent()`

```go
// Inside RefreshFile, after detecting a config file:
configType := analyzer.DetectConfigType(filePath)
if configType != "" {
    content, _ := os.ReadFile(absPath)
    cf, err := analyzer.ParseConfigFile(filePath, configType, content)
    if err == nil {
        // Update or append in repoCtx.ConfigFiles
        updated := false
        for i, existing := range repoCtx.ConfigFiles {
            if existing.Path == filePath {
                repoCtx.ConfigFiles[i] = *cf
                updated = true
                break
            }
        }
        if !updated {
            repoCtx.ConfigFiles = append(repoCtx.ConfigFiles, *cf)
        }
        m.store.StoreConfigFileContent(projectID, cf.Path, cf.Content, cf.StructuredJSON, cf.Type)
    }
}
```

#### 3c. RefreshChangedFiles for go.mod and Config Files

The existing `RefreshChangedFiles` implementation iterates all files and checks if their hash has changed. Extend this to:

1. Track whether go.mod was among the changed files
2. Track which config files were among the changed files
3. After processing all individual file refreshes, if go.mod changed, trigger the go.mod-specific refresh logic (re-parse, re-aggregate imports, re-generate architecture)
4. For each changed config file, trigger the config-specific refresh logic

The key insight: `RefreshChangedFiles` likely calls `RefreshFile` for each changed file, so the logic in 3a and 3b should naturally fire. However, ensure that go.mod re-parsing happens before import re-aggregation in the correct order. If `RefreshChangedFiles` processes files in arbitrary order, add a post-processing step:

```go
// After processing all changed files, check if go.mod was refreshed
// and ensure import summary is consistent
goModChanged := false
for _, result := range results {
    if result.FilePath == "go.mod" {
        goModChanged = true
        break
    }
}
if goModChanged && repoCtx.ModuleInfo != nil {
    repoCtx.ImportSummary = analyzer.AggregateImports(repoCtx.Files, repoCtx.ModuleInfo)
    repoCtx.Architecture = m.generateArchitecture(repoCtx)
    m.store.StoreImportSummary(projectID, repoCtx.ImportSummary)
    m.store.StoreRepoContext(ctx, projectID, repoCtx)
}
```

### Part 4: Add `PackageType` and `GoVersion` to `ArchitectureContext`

**File to modify:** `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/context/types.go`

Add two fields to `ArchitectureContext` (coordinate with Section 01 if it defines types first):

```go
type ArchitectureContext struct {
    Overview        string           `json:"overview"`
    Modules         []Module         `json:"modules"`
    EntryPoints     []EntryPoint     `json:"entry_points"`
    Dependencies    []string         `json:"dependencies"`
    BuildSystem     string           `json:"build_system"`
    MainPackages    []string         `json:"main_packages"`
    PackageType     string           `json:"package_type,omitempty"` // NEW: "library" or "application"
    GoVersion       string           `json:"go_version,omitempty"`  // NEW: from go.mod
    AIAnalysis      *AIArchAnalysis  `json:"ai_analysis,omitempty"`
}
```

These are purely additive changes and do not break any existing serialization since both use `omitempty`.

## Files Modified

| File | Change |
|------|--------|
| `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/context/types.go` | Add `PackageType` and `GoVersion` fields to `ArchitectureContext` |
| `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/orchestrator/manager.go` | Update `generateArchitecture()` with dependency population, package type detection, library entry points, Go version; Update `AnalyzeRepo()` to call go.mod parser, config parsers, import aggregator, and persist results |
| `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/orchestrator/manager.go` (or the file implementing `RefreshFile`/`RefreshChangedFiles`) | Add go.mod and config file refresh logic |

## Error Handling

- If `ModuleInfo` is nil (no go.mod found), all new logic gracefully skips. Dependencies stays empty, package type defaults to "library" (since no `main` package found in non-Go repos either), GoVersion stays empty.
- If config file parsing fails for any file, log a warning and continue. Do not fail the analysis.
- If import aggregation fails, log a warning. The ImportSummary stays nil.
- During refresh, if go.mod re-parsing fails, return the error to the caller so the user knows the refresh failed for that file.

## TODO Checklist

1. Add `PackageType` and `GoVersion` fields to `ArchitectureContext` in types.go
2. Write tests for `generateArchitecture()` updates (dependencies, package type, library entry points, Go version, nil ModuleInfo)
3. Update `generateArchitecture()` to populate Dependencies from ModuleInfo
4. Update `generateArchitecture()` to detect and set PackageType
5. Update `generateArchitecture()` to add library entry points
6. Update `generateArchitecture()` to include GoVersion and update overview string
7. Update `AnalyzeRepo()` to call go.mod parser after file scan
8. Update `AnalyzeRepo()` to call config file parsers
9. Update `AnalyzeRepo()` to call import aggregator
10. Update `AnalyzeRepo()` to persist ModuleInfo, ImportSummary, and ConfigFiles
11. Write tests for RefreshFile on go.mod
12. Write tests for RefreshFile on config files
13. Write tests for RefreshChangedFiles detecting go.mod changes
14. Implement go.mod refresh logic in RefreshFile
15. Implement config file refresh logic in RefreshFile
16. Implement post-processing in RefreshChangedFiles for go.mod changes