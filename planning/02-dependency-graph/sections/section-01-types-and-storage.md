# Section 01: Types and Storage

## Overview

This section defines the new Go types needed for dependency graph support and extends the SQLite schema to persist them. It is a foundational section that all other sections depend on.

## Dependencies

None. This is the first section to implement.

## Tests First

### File: `internal/storage/sqlite_test.go` (extend existing)

```go
// Test: Store and retrieve ModuleInfo
// Setup: Create a ModuleInfo with module_path, go_version, and dependencies
// Call: store.StoreModuleInfo(repoID, info)
// Call: got, err := store.GetModuleInfo(repoID)
// Assert: got matches original info (module_path, go_version, dependency count)

// Test: Store and retrieve ImportSummary
// Setup: Create an ImportSummary with stdlib, internal, and external imports
// Call: store.StoreImportSummary(repoID, summary)
// Call: got, err := store.GetImportSummary(repoID)
// Assert: got matches original (stdlib count, internal count, external count)

// Test: Store config file content and structured JSON
// Setup: Create a config file entry with content and structured JSON
// Call: store.StoreConfigFileContent(repoID, filePath, content, structuredJSON, configType)
// Call: got, err := store.GetConfigFiles(repoID)
// Assert: got has one entry with matching content and config_type

// Test: Batch load ModuleInfo for multiple repos
// Setup: Store ModuleInfo for 3 repos
// Call: got, err := store.GetModuleInfoBatch([]string{repo1, repo2, repo3})
// Assert: got map has 3 entries, each matching the stored info

// Test: Migration idempotency (run migration twice without error)
// Setup: Run migration once
// Call: Run migration again
// Assert: No error (PRAGMA table_info check prevents duplicate ALTER TABLE)

// Test: Config file blocklist (no content stored for .env files)
// Setup: Try to store content for a file named ".env"
// Call: store.StoreConfigFileContent(repoID, ".env", "SECRET=xxx", nil, "env")
// Assert: The stored entry has empty content (content was stripped by blocklist)
```

## New Types

### File: `internal/context/types.go`

Add these types to the existing types file:

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

type ConfigFile struct {
    Path           string          `json:"path"`
    Type           string          `json:"type"` // "go.mod", "dockerfile", "docker-compose", "makefile", "ci-config"
    Content        string          `json:"content,omitempty"`
    StructuredJSON json.RawMessage `json:"structured,omitempty"`
    SizeBytes      int             `json:"size_bytes"`
}

type DependencyGraph struct {
    Nodes []DependencyNode `json:"nodes"`
    Edges []DependencyEdge `json:"edges"`
}

type DependencyNode struct {
    RepoID      string `json:"repo_id"`
    ModulePath  string `json:"module_path"`
    IsAnalyzed  bool   `json:"is_analyzed"`
    PackageType string `json:"package_type"` // "library" or "application"
}

type DependencyEdge struct {
    From    string `json:"from"`    // module_path of the dependent repo
    To      string `json:"to"`      // module_path of the dependency
    Version string `json:"version"`
    Direct  bool   `json:"direct"`
}
```

Also add helper methods to `ConfigFile` for typed access to structured data:

```go
// func (c *ConfigFile) AsDockerfile() (*DockerfileInfo, error)
// func (c *ConfigFile) AsCompose() (*ComposeInfo, error)
// func (c *ConfigFile) AsMakefile() (*MakefileInfo, error)
// func (c *ConfigFile) AsCIConfig() (*CIInfo, error)
```

These helper methods deserialize `StructuredJSON` into the appropriate typed struct. The concrete types (`DockerfileInfo`, `ComposeInfo`, etc.) are defined in section-03 (config parsers).

### RepoContext Additions

Add to the existing `RepoContext` struct in `internal/context/types.go`:

- `ModuleInfo *ModuleInfo` -- parsed go.mod data
- `ImportSummary *ImportSummary` -- aggregated import classification
- `ConfigFiles []ConfigFile` -- parsed config file content

## SQLite Schema Extension

### New migration file: `internal/storage/migrations/004_dependency_graph.sql`

The SQL file itself is just documentation. The actual migration must be done in Go code because SQLite `ALTER TABLE ADD COLUMN` has no `IF NOT EXISTS` syntax.

The migration adds these columns:

```sql
-- Store parsed go.mod data per repo
ALTER TABLE repos ADD COLUMN module_info_json TEXT;

-- Store import summary per repo
ALTER TABLE repos ADD COLUMN import_summary_json TEXT;

-- Extend files table for config file content and structured data
ALTER TABLE files ADD COLUMN content TEXT;
ALTER TABLE files ADD COLUMN structured_json TEXT;
ALTER TABLE files ADD COLUMN config_type TEXT;
```

### Go Migration Code

In the storage initialization code (where existing migrations are applied), add a Go function for migration 004 that:

1. Uses `PRAGMA table_info(repos)` to check if `module_info_json` column exists
2. If not, executes `ALTER TABLE repos ADD COLUMN module_info_json TEXT`
3. Repeats for each new column on both `repos` and `files` tables
4. Inserts version 4 into `schema_migrations`

This ensures the migration is idempotent -- running it twice is safe.

## SQLiteStore Method Additions

Add these methods to the `SQLiteStore` struct in `internal/storage/`:

- `StoreModuleInfo(repoID string, info *ModuleInfo) error` -- serialize ModuleInfo to JSON, UPDATE the repos row's `module_info_json` column
- `GetModuleInfo(repoID string) (*ModuleInfo, error)` -- SELECT `module_info_json` from repos, deserialize
- `GetModuleInfoBatch(repoIDs []string) (map[string]*ModuleInfo, error)` -- batch SELECT using `WHERE id IN (...)`, return map of repoID to ModuleInfo. Used by the dependency graph builder (section-06) for efficient loading.
- `StoreImportSummary(repoID string, summary *ImportSummary) error` -- serialize to JSON, UPDATE repos row
- `GetImportSummary(repoID string) (*ImportSummary, error)` -- SELECT and deserialize
- `StoreConfigFileContent(repoID, filePath string, content string, structuredJSON json.RawMessage, configType string) error` -- UPDATE the existing file row in the `files` table, setting the `content`, `structured_json`, and `config_type` columns. Check the security blocklist before storing content.
- `GetConfigFiles(repoID string) ([]ConfigFile, error)` -- SELECT from `files` WHERE `config_type IS NOT NULL` for the given repo

All methods follow the existing pattern of JSON serialization into TEXT columns.

## Security: Config File Blocklist

Implement a blocklist function that prevents storing raw content for files that may contain secrets:

- `.env`, `.env.*` files
- Files with "secret", "credential", or "token" in the name (case-insensitive)
- `*.pem`, `*.key` files

When `StoreConfigFileContent` is called for a blocklisted file, store only the structured/parsed output (if any) with raw content set to empty string. The function signature:

```go
func isSensitiveFile(filePath string) bool
```

## Error Handling

- Define sentinel errors: `ErrGoModNotFound` and `ErrGoModMalformed` in the context or storage package for programmatic handling by other sections.

## Summary of Deliverables

1. New types in `internal/context/types.go`: `ModuleInfo`, `ModuleDependency`, `ModuleReplace`, `ImportSummary`, `ExternalImport`, `ConfigFile`, `DependencyGraph`, `DependencyNode`, `DependencyEdge`
2. New fields on `RepoContext`: `ModuleInfo`, `ImportSummary`, `ConfigFiles`
3. SQLite migration 004 with idempotent column additions
4. Store methods: `StoreModuleInfo`, `GetModuleInfo`, `GetModuleInfoBatch`, `StoreImportSummary`, `GetImportSummary`, `StoreConfigFileContent`, `GetConfigFiles`
5. Security blocklist function `isSensitiveFile`
6. Sentinel errors `ErrGoModNotFound`, `ErrGoModMalformed`
7. Tests for all storage methods and migration idempotency
