# Specification: Org-Wide Semantic Index

## Purpose

Extend the vector/embedding layer to support org-wide indexing. Store embeddings partitioned by org_id; enable incremental updates at function/type granularity; build org-wide vocabulary for consistent cross-repo embeddings.

## Current State

The codebase already has significant infrastructure:
- `VectorRecord` with `OrgID` field in SQLite vectors table
- `SearchByOrg()`, `IndexRepositoryWithOrg()`, `CountByOrg()`, `DeleteByOrg()` methods
- `LocalEmbedder` (offline TF-IDF, 256-dim, vocabulary-based)
- Organization abstraction (`internal/org/`) with Manager, Store, AnalyzeOrg
- File tracking with SHA-256 hashes for incremental updates
- SemanticSearch service layer with per-repo indexing

## Requirements

### 1. New `index_org` MCP Tool
- Indexes all repos in an org with a single call
- Builds org-wide vocabulary from all repos for consistent embeddings
- Supports partial failures (index what succeeds, report failures per repo)
- Uses bounded concurrency (reuse org analyzer pattern)
- Returns summary: repos indexed, functions/types embedded, failures

### 2. Extend `index_repository` Tool
- Add optional `org_id` parameter
- When provided, tag embeddings with org_id and use org-wide vocabulary
- Backward compatible: without org_id, behaves as before

### 3. Org-Wide Vocabulary
- Build vocabulary corpus from all repos in org before embedding
- Results in consistent embeddings across repos (same IDF weights)
- Store vocabulary per org for reuse during incremental updates
- Rebuild vocabulary when repos are added/removed from org

### 4. Incremental Updates at Function/Type Level
- Track per-function/type content hashes (not just file-level)
- On `refresh_file`: identify changed functions/types within the file
- Delete stale embeddings for removed/changed functions
- Re-embed only changed functions/types
- Update vector store atomically per file

### 5. Stale Embedding Cleanup
- When a file is deleted: remove all its function/type embeddings
- When a function is renamed/removed: remove its embedding
- When org repos change: clean up embeddings for removed repos

## Architecture Decisions

- **Partitioning**: Shared SQLite with metadata filtering (org_id column, already exists)
- **Vocabulary scope**: Org-wide (build from all repos in org)
- **Update granularity**: Function/type level (requires per-function hash tracking)
- **Error handling**: Partial success for org indexing (matches AnalyzeOrg pattern)
- **Tool design**: Both new index_org tool and org_id on index_repository

## Dependencies

- Split 01 (org abstraction) - completed
- `internal/vectors` - vector store and embedder
- `internal/storage` - SQLite store with file tracking
- `internal/analyzer` - file/function analysis output

## Verification Criteria

- `index_org` indexes all repos in org
- Vector search returns results scoped to org
- Incremental: changing one function updates only that function's embedding
- No re-embed on unchanged content; index persists
- Org-wide vocabulary produces consistent embeddings across repos
