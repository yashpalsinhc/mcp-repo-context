<!-- PROJECT_CONFIG
runtime: go
test_command: go test ./...
END_PROJECT_CONFIG -->

<!-- SECTION_MANIFEST
section-01-function-hash-tracking
section-02-org-vocabulary
section-03-incremental-vector-updates
section-04-stale-cleanup
section-05-index-org-tool
section-06-extend-index-repository
section-07-integration-tests
END_MANIFEST -->

# Implementation Sections Index

## Dependency Graph

| Section | Depends On | Blocks | Parallelizable |
|---------|------------|--------|----------------|
| section-01-function-hash-tracking | - | 03, 04 | Yes |
| section-02-org-vocabulary | - | 03, 05, 06 | Yes |
| section-03-incremental-vector-updates | 01, 02 | 04, 05 | No |
| section-04-stale-cleanup | 01, 03 | 05 | No |
| section-05-index-org-tool | 02, 03, 04 | 07 | No |
| section-06-extend-index-repository | 02 | 07 | Yes |
| section-07-integration-tests | all | - | No |

## Execution Order

1. **Batch 1:** section-01-function-hash-tracking, section-02-org-vocabulary (parallel, no dependencies)
2. **Batch 2:** section-03-incremental-vector-updates (requires 01 AND 02)
3. **Batch 3:** section-04-stale-cleanup, section-06-extend-index-repository (parallel after their deps)
4. **Batch 4:** section-05-index-org-tool (requires 02, 03, 04)
5. **Batch 5:** section-07-integration-tests (requires all)

## Section Summaries

### section-01-function-hash-tracking
New `function_hashes` SQLite table and CRUD methods for tracking per-function content hashes. Includes migration, FunctionHashInfo type, GetChangedFunctions diff logic. Foundation for incremental vector updates.

### section-02-org-vocabulary
Org-wide vocabulary building, storage (`org_vocabulary` table), and versioning. VocabularyAwareEmbedder interface. ExportVocabulary/ImportVocabulary on LocalEmbedder. Memory-efficient streaming vocabulary build.

### section-03-incremental-vector-updates
RefreshFileVectors method that diffs function hashes and updates only changed embeddings. DeleteByFile on vector store. Vocabulary loading for org-scoped updates. Cross-DB transaction documentation.

### section-04-stale-cleanup
CleanupStaleVectors method. Hooks into file deletion, repo removal, and org deletion flows. Marks vocabulary stale on repo add/remove.

### section-05-index-org-tool
New `index_org` MCP tool with bounded concurrency. Builds org vocabulary, then indexes all repos in parallel with per-goroutine embedder instances. Partial failure support. IndexOrgResult type.

### section-06-extend-index-repository
Add optional `org_id` parameter to `index_repository` tool. Load org vocabulary when org_id provided. Fix SearchByOrg to load org vocabulary before embedding query.

### section-07-integration-tests
End-to-end tests: full pipeline indexing, incremental updates, stale cleanup, vocabulary consistency, partial failures.
