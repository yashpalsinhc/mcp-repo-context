# Section 3: Auto-Index on Analyze

## Overview

Add automatic vector indexing when analyze_repo/analyze_local completes. Controlled by MCP_AUTO_INDEX env var (default: true since LocalEmbedder is free). This makes semantic search "just work" without manual index_repository calls.

## Dependencies

- Section 1: Auto-init vector store, IsAvailable()
- Section 2: Error messages for unavailable store

## Tests First

### File: `internal/mcp/tool_auto_index_test.go` (new)

```
Test: Auto-index creates vectors after analyze_local
- Setup: MCP_AUTO_INDEX=true (or default), semantic search available, temp Go repo
- Call: toolAnalyzeLocal with repo path
- Assert: vector count > 0 for the analyzed repo
- Assert: tool output contains "Auto-indexed"
- Assert: tool output contains item count and duration

Test: Auto-index off by env var skips indexing
- Setup: MCP_AUTO_INDEX=false
- Call: toolAnalyzeLocal with repo path
- Assert: vector count == 0
- Assert: tool output does NOT contain "Auto-indexed"

Test: Auto-index failure doesn't fail analyze
- Setup: MCP_AUTO_INDEX=true, semantic search with store that returns error on StoreBatch
- Call: toolAnalyzeLocal
- Assert: analyze succeeds (repo context stored)
- Assert: warning logged about indexing failure
- Assert: tool output includes analyze results

Test: Auto-index default is true
- Setup: no MCP_AUTO_INDEX env var set
- Read config
- Assert: config.AutoIndex == true

Test: Auto-index with MCP_AUTO_INDEX=true explicit
- Setup: MCP_AUTO_INDEX=true
- Read config
- Assert: config.AutoIndex == true

Test: Auto-index with analyze_repo (remote)
- Setup: MCP_AUTO_INDEX=true, mock remote repo
- Call: toolAnalyzeRepo
- Assert: vectors created after analysis
- Assert: output contains "Auto-indexed"

Test: Auto-index skipped when semantic search unavailable
- Setup: MCP_AUTO_INDEX=true, semantic search IsAvailable() == false
- Call: toolAnalyzeLocal
- Assert: analyze succeeds
- Assert: no indexing attempted
- Assert: warning logged
```

## Implementation Details

### 1. Configuration

Add to `ServerConfig` in `internal/mcp/server.go`:
```go
AutoIndex bool // auto-index vectors after analyze (default: true)
```

In `cmd/mcp-server/main.go`, read env var:
```go
autoIndex := true // default
if v := os.Getenv("MCP_AUTO_INDEX"); v != "" {
    autoIndex = strings.ToLower(v) == "true"
}
serverConfig.AutoIndex = autoIndex
```

Store on server struct for use in tool handlers.

### 2. Auto-Index After Analyze

In `toolAnalyzeLocal` and `toolAnalyzeRepo`, after successful analysis and storage:

```go
// After storing repo context...
if s.autoIndex && s.semanticSearch.IsAvailable() {
    start := time.Now()
    indexed, err := s.autoIndexRepo(ctx, repoID, repoCtx)
    if err != nil {
        log.Printf("Warning: auto-indexing failed for %s: %v", repoID, err)
    } else {
        duration := time.Since(start)
        output += fmt.Sprintf("\n\nAuto-indexed %d items in %s", indexed, duration.Round(time.Millisecond))
    }
}
```

### 3. autoIndexRepo Helper

```go
func (s *server) autoIndexRepo(ctx context.Context, repoID string, repoCtx *storage.RepoContext) (int, error)
```

Steps:
1. Clear existing vectors for the repo (in case of re-analyze)
2. Call `s.semanticSearch.IndexRepository(ctx, repoCtx)`
3. Get count: `s.semanticSearch.Count(ctx, repoID)`
4. Return count and nil error

This is essentially what `toolIndexRepository` does but without the user-facing output formatting.

### 4. Interaction with force Flag

`toolIndexRepository` with `force=true` should still work independently of auto-index. Auto-index is a convenience that runs during analyze; manual indexing remains available for explicit control.

If a repo was auto-indexed and the user later calls `index_repository force=true`, it re-indexes from scratch (rebuilds vocabulary, re-generates all embeddings).

## Error Handling

- Auto-index failure: log warning, do NOT fail the analyze operation. The analyze result is the primary output.
- Semantic search unavailable: skip auto-index silently (or with debug log)
- RepoContext empty (no functions): skip auto-index, nothing to index

## File Summary

| File | Action |
|------|--------|
| `internal/mcp/server.go` | Modify: add AutoIndex to ServerConfig and server struct |
| `cmd/mcp-server/main.go` | Modify: read MCP_AUTO_INDEX env var |
| `internal/mcp/tools.go` | Modify: add auto-index call in toolAnalyzeLocal and toolAnalyzeRepo |
| `internal/mcp/auto_index.go` | New: autoIndexRepo helper |
| `internal/mcp/tool_auto_index_test.go` | New: auto-index tests |
