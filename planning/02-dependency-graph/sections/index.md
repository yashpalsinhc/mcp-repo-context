<!-- PROJECT_CONFIG
runtime: go
test_command: go test ./...
END_PROJECT_CONFIG -->

<!-- SECTION_MANIFEST
section-01-types-and-storage
section-02-gomod-parser
section-03-config-parsers
section-04-import-aggregation
section-05-architecture-updates
section-06-dependency-graph-tool
section-07-tool-enhancements
section-08-integration-test
END_MANIFEST -->

# Implementation Sections Index

## Dependency Graph

| Section | Depends On | Blocks | Parallelizable |
|---------|------------|--------|----------------|
| section-01-types-and-storage | - | 02, 03, 04, 05, 06, 07 | Yes |
| section-02-gomod-parser | 01 | 04, 05, 06 | Yes (after 01) |
| section-03-config-parsers | 01 | 07 | Yes (after 01) |
| section-04-import-aggregation | 01, 02 | 05, 06 | No |
| section-05-architecture-updates | 01, 02, 04 | 06, 07 | No |
| section-06-dependency-graph-tool | 01, 02, 04, 05 | 08 | No |
| section-07-tool-enhancements | 01, 03, 05 | 08 | Yes (after 05) |
| section-08-integration-test | all | - | No |

## Execution Order

1. **Batch 1:** section-01-types-and-storage (no dependencies)
2. **Batch 2:** section-02-gomod-parser, section-03-config-parsers (parallel, both depend only on 01)
3. **Batch 3:** section-04-import-aggregation (depends on 01, 02)
4. **Batch 4:** section-05-architecture-updates (depends on 01, 02, 04)
5. **Batch 5:** section-06-dependency-graph-tool, section-07-tool-enhancements (parallel, both depend on 05)
6. **Batch 6:** section-08-integration-test (depends on all)

## Section Summaries

### section-01-types-and-storage
Define new types (ModuleInfo, ConfigFile, ImportSummary, DependencyGraph, etc.) in context/types.go. Create SQLite migration 004. Add store methods for ModuleInfo, ConfigFiles, ImportSummary. Security blocklist for sensitive files.

**Plan reference:** Sections 3, 8
**TDD reference:** Section 8 tests (storage)

### section-02-gomod-parser
Create `internal/analyzer/gomod_analyzer.go` using `golang.org/x/mod/modfile`. Parse module path, Go version, require/replace directives. Add `golang.org/x/mod` dependency.

**Plan reference:** Section 4
**TDD reference:** Section 4 tests

### section-03-config-parsers
Create `internal/analyzer/config_parsers.go` with Dockerfile, docker-compose.yml, Makefile, CI config parsers. Add `gopkg.in/yaml.v3` dependency. Integrate with generic analyzer.

**Plan reference:** Section 5
**TDD reference:** Section 5 tests

### section-04-import-aggregation
Create `internal/analyzer/import_aggregator.go`. Classify imports as stdlib/internal/external. Resolve external imports to module paths with replace directive support. Build static stdlib set.

**Plan reference:** Section 6
**TDD reference:** Section 6 tests

### section-05-architecture-updates
Update `generateArchitecture()` to populate Dependencies field, detect package type, identify library entry points, include Go version. Add incremental refresh support for go.mod and config files.

**Plan reference:** Sections 7, 12, 12b
**TDD reference:** Section 7 tests, 12b tests

### section-06-dependency-graph-tool
Create `internal/graph/dependency_graph.go` for cross-repo graph building. Add Mermaid/DOT visualization. Register `get_dependency_graph` MCP tool. Update Manager interface.

**Plan reference:** Sections 9, 10
**TDD reference:** Sections 9, 10 tests

### section-07-tool-enhancements
Update compare_repos to show dependency relationships. Update get_context(scope=architecture) to include dependency data. Update ask prompt template.

**Plan reference:** Section 11
**TDD reference:** Section 11 tests

### section-08-integration-test
End-to-end test: analyze gorilla/mux and gorilla/handlers, verify go.mod parsed, imports classified, dependency graph correct, compare_repos shows relationships.

**Plan reference:** Section 16 (Integration Tests)
**TDD reference:** Integration test stubs
