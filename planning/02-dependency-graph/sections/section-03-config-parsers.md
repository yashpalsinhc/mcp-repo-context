The hook script is failing due to a configuration issue -- it is interpreting `run` as a file path rather than passing it as an argument to the Python script. This is an environment/hook configuration problem, not a content problem.

The section content I have generated is complete. Since the automated hook cannot write the file, let me verify the target path and confirm the content is ready for manual placement.

The content should be written to:
`/Users/yashpalc/yashpalc-mcp/mcp-repo-context/planning/02-dependency-graph/sections/section-03-config-parsers.md`

The full section content follows below (identical to what was produced in the previous two turns):

---

# Section 03: Config File Parsers

## Overview

This section creates `internal/analyzer/config_parsers.go`, a collection of lightweight parsers for common config files found in Go repositories: Dockerfile, docker-compose.yml, Makefile, and CI configs (GitHub Actions, GitLab CI). Each parser extracts structured data from the raw file content and returns it as typed Go structs that can be serialized to JSON and stored in the `structured_json` column of the `files` table.

## Dependencies

- **section-01-types-and-storage** must be completed first. It defines the `ConfigFile` type (with its `StructuredJSON` field and helper methods) in `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/context/types.go`, and the storage schema with `content`, `structured_json`, and `config_type` columns on the `files` table.

## Files to Create

- `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/analyzer/config_parsers.go`
- `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/analyzer/config_parsers_test.go`

## New Go Module Dependency

Add `gopkg.in/yaml.v3` to the project. Note that `golang.org/x/mod` is already present in `go.mod` (as an indirect dependency). The `yaml.v3` package is needed for parsing docker-compose.yml and CI config files. Run:

```
go get gopkg.in/yaml.v3
```

from `/Users/yashpalc/yashpalc-mcp/mcp-repo-context`.

## Structured Data Types

These types should be defined in `config_parsers.go` (they are the concrete types behind the `ConfigFile.StructuredJSON` field). The `ConfigFile` type from section-01 has helper methods like `AsDockerfile()`, `AsCompose()`, `AsMakefile()`, and `AsCIConfig()` that deserialize `StructuredJSON` into these types.

```go
// DockerfileInfo holds structured data extracted from a Dockerfile.
type DockerfileInfo struct {
    BaseImages []string `json:"base_images"` // FROM directives (e.g., "golang:1.21-alpine")
    Stages     []string `json:"stages"`      // Named build stages (e.g., "builder")
    Ports      []string `json:"ports"`       // EXPOSE ports
    Entrypoint string   `json:"entrypoint"`  // CMD or ENTRYPOINT value
}

// ComposeInfo holds structured data from docker-compose.yml.
type ComposeInfo struct {
    Services []ComposeService `json:"services"`
}

// ComposeService describes one service in a docker-compose file.
type ComposeService struct {
    Name      string   `json:"name"`
    Image     string   `json:"image,omitempty"`
    Build     string   `json:"build,omitempty"`
    Ports     []string `json:"ports,omitempty"`
    DependsOn []string `json:"depends_on,omitempty"`
}

// MakefileInfo holds structured data from a Makefile.
type MakefileInfo struct {
    Targets []MakeTarget `json:"targets"`
}

// MakeTarget describes a Makefile target.
type MakeTarget struct {
    Name        string `json:"name"`
    Description string `json:"description,omitempty"` // From preceding comment
}

// CIInfo holds structured data from CI config files.
type CIInfo struct {
    Platform string   `json:"platform"` // "github-actions" or "gitlab-ci"
    Jobs     []CIJob  `json:"jobs"`
    Triggers []string `json:"triggers,omitempty"` // e.g., "push", "pull_request"
}

// CIJob describes a CI job.
type CIJob struct {
    Name  string `json:"name"`
    Stage string `json:"stage,omitempty"` // GitLab CI only
}
```

## Parser Functions

Each parser is a standalone exported function that takes `[]byte` content and returns the structured type plus an error. The overall dispatch function takes a file path and content, determines the appropriate parser, and returns the result as `json.RawMessage` alongside a config type string.

### Dispatch function

```go
// ParseConfigFile determines the config type from the file path, calls the
// appropriate parser, and returns the structured JSON and config type string.
// Returns ("", nil, nil) if the file is not a recognized config file.
func ParseConfigFile(path string, content []byte) (configType string, structured json.RawMessage, err error)
```

The dispatch logic uses path-based matching:
- Path contains "Dockerfile" (case-insensitive) -- call `ParseDockerfile`
- Path ends with "docker-compose.yml" or "docker-compose.yaml" -- call `ParseDockerCompose`
- Path contains "Makefile" (case-insensitive) -- call `ParseMakefile`
- Path matches `.github/workflows/*.yml` or `.github/workflows/*.yaml` -- call `ParseCIConfig` with platform "github-actions"
- Path matches `.gitlab-ci.yml` -- call `ParseCIConfig` with platform "gitlab-ci"

### Dockerfile parser

```go
// ParseDockerfile extracts base images, stages, ports, and entrypoint from a Dockerfile.
func ParseDockerfile(content []byte) (*DockerfileInfo, error)
```

Implementation approach:
- Scan line by line (no YAML parsing needed)
- For `FROM` lines: extract the image name (second token). If the line contains `AS <name>`, record the stage name in `Stages` and record the full image (without the `AS` part) in `BaseImages`
- For `EXPOSE` lines: extract port numbers/ranges (all tokens after EXPOSE)
- For `CMD` and `ENTRYPOINT` lines: record the value. If both exist, prefer ENTRYPOINT. Handle both exec form `["cmd"]` and shell form `cmd arg1`
- Return empty `DockerfileInfo` (not an error) for an empty Dockerfile
- Lines starting with `#` are comments; skip them

### docker-compose.yml parser

```go
// ParseDockerCompose extracts services, images, ports, and depends_on from a docker-compose.yml.
func ParseDockerCompose(content []byte) (*ComposeInfo, error)
```

Implementation approach:
- Use `gopkg.in/yaml.v3` to unmarshal into a flexible structure. The top-level key is `services` (Compose v3) or the entire file may be service definitions (Compose v2, less common)
- Define an intermediate struct for unmarshaling:
  ```go
  type composeFile struct {
      Services map[string]composeServiceDef `yaml:"services"`
  }
  type composeServiceDef struct {
      Image     string      `yaml:"image"`
      Build     interface{} `yaml:"build"` // Can be string or object
      Ports     []string    `yaml:"ports"`
      DependsOn interface{} `yaml:"depends_on"` // Can be []string or map
  }
  ```
- Handle `depends_on` being either a list of strings or a map (keys are service names). Normalize both to `[]string`
- Handle `build` being either a string path or an object with `context` field. Store the string representation
- If YAML parsing fails, return an error (the caller in the generic analyzer should log and continue)

### Makefile parser

```go
// ParseMakefile extracts target names and their descriptions from a Makefile.
func ParseMakefile(content []byte) (*MakefileInfo, error)
```

Implementation approach:
- Scan line by line
- Target lines match the regex `^([a-zA-Z_][a-zA-Z0-9_.-]*)\s*:` -- the key rule is the line must NOT start with a tab (tab-prefixed lines are recipe commands)
- Also exclude lines that look like variable assignments (`=`, `:=`, `?=`, `+=`)
- For descriptions: check the line immediately preceding a target. If it is a comment (`# Description text`), use the comment text (without the `#` prefix) as the description
- `.PHONY` lines should be ignored as targets
- Return empty `MakefileInfo` for a file with no targets

### CI config parser

```go
// ParseCIConfig extracts jobs and triggers from GitHub Actions or GitLab CI configs.
func ParseCIConfig(content []byte, platform string) (*CIInfo, error)
```

Implementation approach for **GitHub Actions** (`platform == "github-actions"`):
- Unmarshal YAML into a flexible structure
- Extract triggers from the `on` key (can be a string, list, or map). When it is a map, the keys are trigger names (e.g., `push`, `pull_request`)
- Extract jobs from the `jobs` key (a map of job-name to job definition). Record each key as a `CIJob.Name`

Implementation approach for **GitLab CI** (`platform == "gitlab-ci"`):
- Unmarshal YAML into `map[string]interface{}`
- Extract `stages` key (list of strings)
- Each top-level key that is not a reserved keyword (`stages`, `variables`, `include`, `default`, `workflow`, `image`, `before_script`, `after_script`, `cache`, `services`) is treated as a job name
- If the job value is a map and has a `stage` field, record that as `CIJob.Stage`

For unknown platform values, return an empty `CIInfo` with no error.

## Content Cap

Before any parser is called, the dispatch function should check the content size. If the content exceeds 100KB (102400 bytes), truncate to 100KB and proceed. The `ConfigFile.Content` field (managed by the caller/storage layer from section-01) stores raw content with the same cap.

## Integration with Generic Analyzer

The existing `genericAnalyzer` in `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/analyzer/generic_analyzer.go` already identifies config files by path in `guessPurpose()`. The config parsers are NOT called from the generic analyzer directly. Instead, they are called from the manager during `AnalyzeRepository()` (section-05 handles that integration). This section only creates the parsers and their tests.

The manager integration (done in section-05) will:
1. After file analysis, iterate over analyzed files
2. For each file, call `ParseConfigFile(path, content)` to check if it is a recognized config type
3. If recognized, store the raw content and structured JSON via the store methods from section-01

## Tests

**File:** `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/analyzer/config_parsers_test.go`

All tests use synthetic inline fixtures (no external test files needed). The test file should be in `package analyzer` to test exported functions.

### Dockerfile Tests

```go
// Test: Extract base images from single-stage Dockerfile
// Input: "FROM golang:1.21-alpine\nRUN go build\nEXPOSE 8080\nCMD [\"./app\"]"
// Assert: BaseImages == ["golang:1.21-alpine"], Ports == ["8080"], Entrypoint == "[\"./app\"]"

// Test: Extract base images from multi-stage Dockerfile (multiple FROM)
// Input: "FROM golang:1.21 AS builder\nRUN go build\nFROM alpine:3.18\nCOPY --from=builder /app /app"
// Assert: BaseImages == ["golang:1.21", "alpine:3.18"], Stages == ["builder"]

// Test: Extract EXPOSE ports
// Input: Dockerfile with "EXPOSE 8080 9090"
// Assert: Ports contains "8080" and "9090"

// Test: Extract CMD/ENTRYPOINT
// Input: Dockerfile with both CMD and ENTRYPOINT
// Assert: Entrypoint is populated (ENTRYPOINT takes precedence)

// Test: Handle empty Dockerfile
// Input: empty byte slice
// Assert: No error, empty DockerfileInfo returned
```

### docker-compose.yml Tests

```go
// Test: Extract service names and images
// Input: YAML with two services, each with an image field
// Assert: Two ComposeService entries with correct names and images

// Test: Extract port mappings
// Input: YAML service with ports: ["8080:80", "443:443"]
// Assert: ComposeService.Ports matches

// Test: Extract depends_on relationships
// Input: YAML with depends_on as a list and as a map (two separate subtests)
// Assert: DependsOn normalized to string slices in both cases

// Test: Handle YAML parse error gracefully
// Input: "not: valid: yaml: [["
// Assert: Returns non-nil error
```

### Makefile Tests

```go
// Test: Extract target names
// Input: "build:\n\tgo build\ntest:\n\tgo test"
// Assert: Targets == [{Name: "build"}, {Name: "test"}]

// Test: Extract target descriptions from preceding comments
// Input: "# Build the binary\nbuild:\n\tgo build"
// Assert: Targets[0].Description == "Build the binary"

// Test: Ignore recipe lines (starting with tab)
// Input: Makefile with indented recipe lines
// Assert: Recipe lines not treated as targets

// Test: Handle Makefile with no targets
// Input: "# Just a comment\nVAR = value"
// Assert: Empty Targets slice, no error
```

### CI Config Tests

```go
// Test: Parse GitHub Actions workflow for job names and triggers
// Input: YAML with on: [push, pull_request] and jobs: {build: ..., test: ...}
// Assert: Triggers == ["push", "pull_request"], Jobs contain "build" and "test"

// Test: Parse GitLab CI for stages and job names
// Input: YAML with stages: [build, test] and job definitions
// Assert: Jobs have correct names and stages

// Test: Handle unknown CI format gracefully
// Input: Call with platform "unknown"
// Assert: Empty CIInfo, no error
```

### Dispatch Function Tests

```go
// Test: ParseConfigFile routes Dockerfile correctly
// Test: ParseConfigFile routes docker-compose.yml correctly
// Test: ParseConfigFile routes Makefile correctly
// Test: ParseConfigFile routes .github/workflows/ci.yml correctly
// Test: ParseConfigFile returns empty for unrecognized file paths (e.g., "main.go")
// Test: ParseConfigFile truncates content over 100KB without error
```

## Error Handling

- **Dockerfile malformed:** Since parsing is line-based regex, most malformed Dockerfiles will simply produce incomplete data rather than errors. This is acceptable.
- **YAML parse errors (docker-compose, CI configs):** Return an error from the parser. The caller (manager in section-05) should log a warning and continue without structured data for that file.
- **Makefile with no targets:** Return empty `MakefileInfo`, not an error.
- **Content over 100KB:** Truncate silently. The caller stores the truncated content.

## Summary of What This Section Delivers

After implementing this section, the project will have:
1. Five parser functions (`ParseDockerfile`, `ParseDockerCompose`, `ParseMakefile`, `ParseCIConfig`, `ParseConfigFile`) in `config_parsers.go`
2. Six structured data types (`DockerfileInfo`, `ComposeInfo`, `ComposeService`, `MakefileInfo`, `MakeTarget`, `CIInfo`, `CIJob`) in the same file
3. Comprehensive test coverage in `config_parsers_test.go`
4. The `gopkg.in/yaml.v3` dependency added to `go.mod`

These parsers are consumed by section-05 (architecture updates / manager integration) and section-07 (tool enhancements). They are standalone and testable in isolation.