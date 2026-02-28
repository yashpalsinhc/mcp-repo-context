<!-- SECTION_MANIFEST
section-01-dimension-autoinit
section-02-error-messages
section-03-auto-index
section-04-incremental-indexing
section-05-vector-budgeted
section-06-profiling
section-07-integration-tests
END_MANIFEST -->

# Section Index: Semantic Search & Vector Store

## Batch 1 (parallel)

### section-01-dimension-autoinit
Fix dimension mismatch (256 vs 384), auto-init vector store, dimension migration, validation, bubble sort fix, IsAvailable method. From plan Section 1 and TDD Section 1.

### section-02-error-messages
Replace "not enabled" errors with actionable guidance. Sentinel errors, specific failure mode detection. From plan Section 2 and TDD Section 2.

## Batch 2 (parallel, depends on batch 1)

### section-03-auto-index
Auto-index on analyze with MCP_AUTO_INDEX env var (default true). Integration with toolAnalyzeRepo/toolAnalyzeLocal. From plan Section 3 and TDD Section 3.

### section-04-incremental-indexing
RefreshFile/RefreshFiles, DeleteByFile, vocabulary persistence/caching/staleness, cross-repo vocabulary isolation. From plan Section 4 and TDD Section 4.

## Batch 3 (parallel, depends on batch 2)

### section-05-vector-budgeted
Vector-ranked get_context_budgeted with keyword fallback, vocabulary loading for search, tiered budget docs. From plan Section 5 and TDD Section 5.

### section-06-profiling
Timing instrumentation, VectorCount, performance documentation. From plan Section 6 and TDD Section 6.

## Batch 4 (depends on batch 3)

### section-07-integration-tests
End-to-end tests: auto-init, dimension consistency, round-trip, incremental, restart, cross-repo vocab, error cascade. From plan Section 7 and TDD Section 7.
