<!-- PROJECT_CONFIG
runtime: go
test_command: go test ./...
END_PROJECT_CONFIG -->

<!-- SECTION_MANIFEST
section-01-shared-infra
section-02-comparison-keys
section-03-gap-analysis
section-04-smart-query
section-05-pattern-execution
section-06-call-graph
section-07-package-structure
section-08-integration-test
END_MANIFEST -->

# Implementation Sections Index

## Dependency Graph

| Section | Depends On | Blocks | Parallelizable |
|---------|------------|--------|----------------|
| section-01-shared-infra | - | 02, 03, 04 | Yes |
| section-02-comparison-keys | 01 | 03 | No |
| section-03-gap-analysis | 01, 02 | 08 | No |
| section-04-smart-query | 01 | 08 | Yes (after 01) |
| section-05-pattern-execution | - | 08 | Yes |
| section-06-call-graph | - | 08 | Yes |
| section-07-package-structure | - | 08 | Yes |
| section-08-integration-test | 03, 04, 05, 06, 07 | - | No |

## Execution Order

1. **Batch 1:** section-01-shared-infra (no dependencies)
2. **Batch 2:** section-02-comparison-keys (after 01), section-05-pattern-execution, section-06-call-graph, section-07-package-structure (parallel — these are independent)
3. **Batch 3:** section-03-gap-analysis (after 01, 02), section-04-smart-query (after 01) — parallel with each other
4. **Batch 4:** section-08-integration-test (after all others)

## Section Summaries

### section-01-shared-infra
Create `internal/nlp/` package: Porter stemmer, Levenshtein distance with early termination, concept similarity scoring with stop words and O(1) domain profile. Also add schema versioning using existing `Version` field on `RepoContext`. Includes all NLP unit tests.

**Plan reference:** Section 7 (Shared Infrastructure)
**TDD reference:** Section 7 tests

### section-02-comparison-keys
Fix `normalizeFunctionKey()` and `normalizeTypeKey()` to include receiver type. Refactor `FindConflicts` and `FindGaps` to use normalize functions instead of raw `fn.Name`. Implement idempotent lazy migration from Version 0→1.

**Plan reference:** Section 1 (Receiver-Aware Comparison Keys)
**TDD reference:** Section 1 tests

### section-03-gap-analysis
Add concept similarity scoring to gap detection using NLP package. Build domain profiles with `map[string]bool`, apply stop word filtering, configurable threshold (default 0.3), rank results by similarity score.

**Plan reference:** Section 2 (Domain-Aware Gap Analysis)
**TDD reference:** Section 2 tests

### section-04-smart-query
Fix smart query NLP: logic reordering for ambiguous queries, word stemming as secondary signal, common question patterns, two-level confidence contract, path/file substring boundary matching fix.

**Plan reference:** Section 3 (Smart Query NLP Improvements)
**TDD reference:** Section 3 tests

### section-05-pattern-execution
Add three-state step tracking (executed/skipped/not_reached) to chain execution. Fix impact_analysis with search step and disambiguation. Fix search_with_context result parsing. Add partial completion output.

**Plan reference:** Section 4 (Pattern Execution Fixes)
**TDD reference:** Section 4 tests

### section-06-call-graph
Fix callee extraction: include method calls in CalledBy population, add heuristic receiver type inference, fix makeNodeID and funcFile collisions, add Receiver field to CallRef. Implement opt-in go/types mode.

**Plan reference:** Section 5 (Call Graph Callee Extraction)
**TDD reference:** Section 5 tests

### section-07-package-structure
Replace path-based file grouping with purpose-based grouping (source/tests/docs/config). Add flat package optimization and 2-level nesting collapse for deep packages.

**Plan reference:** Section 6 (Package Structure Grouping)
**TDD reference:** Section 6 tests

### section-08-integration-test
End-to-end test: analyze gorilla/mux and gorilla/handlers, run compare_repos, find_duplicates, find_conflicts, verify gap count reduction, no false duplicates, no false conflicts. Acceptance criterion for the overall fix.

**Plan reference:** Integration Test section
**TDD reference:** Integration Test tests
