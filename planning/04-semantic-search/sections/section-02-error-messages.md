# Section 2: Fix Error Messages

## Overview

Replace unhelpful "Semantic search is not enabled" errors with actionable guidance. Add sentinel errors, specific failure mode detection, and markdown-formatted suggestions for each tool handler.

## Dependencies

- Section 1: IsAvailable() method on SemanticSearch

## Tests First

### File: `internal/mcp/tool_error_messages_test.go` (new)

```
Test: semantic_search on unindexed repo shows indexing instructions
- Setup: repo analyzed but not indexed (0 vectors)
- Call: toolSemanticSearch with repo_id and query
- Assert: output contains "index_repository"
- Assert: output contains repo_id in the suggestion

Test: semantic_search when store unavailable shows path guidance
- Setup: server with SemanticSearch where IsAvailable() == false
- Call: toolSemanticSearch
- Assert: output contains "not available"
- Assert: output contains "file permissions"

Test: index_repository when store unavailable shows config guidance
- Setup: server with nil vector store
- Call: toolIndexRepository
- Assert: output contains "MCP_VECTOR_STORE_PATH"
- Assert: output contains "file permissions"

Test: index_repository on unanalyzed repo suggests analyze first
- Setup: vector store available, repo NOT analyzed (no context)
- Call: toolIndexRepository(repo_id)
- Assert: output contains "analyze_repo"
- Assert: output contains "analyze_local"
- Assert: output contains the repo_id

Test: semantic_search with results works normally
- Setup: repo analyzed AND indexed
- Call: toolSemanticSearch with matching query
- Assert: results returned, no error message

Test: index_repository on analyzed repo works normally
- Setup: repo analyzed, vector store available
- Call: toolIndexRepository(repo_id)
- Assert: indexing succeeds, no error about missing analysis
```

## Implementation Details

### 1. Sentinel Errors

Add in `internal/vectors/errors.go` (new file):

```go
var (
    ErrVectorStoreUnavailable = errors.New("vector store is not available")
    ErrNotIndexed             = errors.New("repository is not indexed")
    ErrNotAnalyzed            = errors.New("repository has not been analyzed")
)
```

### 2. Update toolSemanticSearch

Replace the current nil check with multi-step validation:

```go
func (s *server) toolSemanticSearch(ctx context.Context, args map[string]any) callToolResult {
    // Step 1: Check if semantic search service is available
    if !s.semanticSearch.IsAvailable() {
        return errorResult(formatStoreUnavailableMessage())
    }

    // Step 2: Check if repo has vectors indexed
    count, _ := s.semanticSearch.Count(ctx, repoID)
    if count == 0 {
        return errorResult(formatNotIndexedMessage(repoID))
    }

    // Step 3: Proceed with search (existing logic)
}
```

### 3. Update toolIndexRepository

```go
func (s *server) toolIndexRepository(ctx context.Context, args map[string]any) callToolResult {
    // Step 1: Check vector store availability
    if !s.semanticSearch.IsAvailable() {
        return errorResult(formatStoreUnavailableForIndexMessage())
    }

    // Step 2: Check if repo has been analyzed
    repoCtx := s.manager.GetContext(ctx, repoID)
    if repoCtx == nil || len(repoCtx.Functions) == 0 {
        return errorResult(formatNotAnalyzedMessage(repoID))
    }

    // Step 3: Proceed with indexing (existing logic)
}
```

### 4. Error Message Formatters

In `internal/mcp/error_messages.go` (new file):

```go
func formatStoreUnavailableMessage() string
```
Returns:
```markdown
## Vector Store Unavailable

The server could not initialize the vector database.

**To fix:**
1. Check the `MCP_VECTOR_STORE_PATH` environment variable
2. Ensure the directory has write permissions
3. Check available disk space
4. Restart the server
```

```go
func formatNotIndexedMessage(repoID string) string
```
Returns:
```markdown
## Repository Not Indexed

Repository `{repoID}` has not been indexed for semantic search.

**To index, run:**
```
index_repository(repo_id: "{repoID}")
```

This generates vector embeddings for all functions and types,
enabling similarity-based search. Indexing is fast (~1-5 seconds for most repos).
```

```go
func formatNotAnalyzedMessage(repoID string) string
```
Returns:
```markdown
## Repository Not Analyzed

Repository `{repoID}` has not been analyzed yet.

**Analyze first, then index:**
```
analyze_repo(repo_url: "https://github.com/...")
```
or for local directories:
```
analyze_local(path: "/path/to/repo")
```

After analysis, run `index_repository` to enable semantic search.
```

```go
func formatStoreUnavailableForIndexMessage() string
```
Returns:
```markdown
## Vector Store Unavailable

Cannot index: the vector database is not available.

**To fix:**
1. Check the `MCP_VECTOR_STORE_PATH` environment variable (default: `{storagePath}/vectors.db`)
2. Ensure the path is writable
3. Restart the server
```

### 5. Count Method

If SemanticSearch doesn't already have a `Count(ctx, repoID) (int, error)` method, add one that delegates to `store.Count()`. This is used to distinguish "not indexed" from "indexed but no matches".

## Error Handling

- All error messages are user-facing markdown — no stack traces or internal details
- Each message includes specific next steps
- Error formatters are pure functions (no side effects) for easy testing

## File Summary

| File | Action |
|------|--------|
| `internal/vectors/errors.go` | New: sentinel errors |
| `internal/mcp/error_messages.go` | New: error message formatters |
| `internal/mcp/tools.go` | Modify: update toolSemanticSearch and toolIndexRepository error handling |
| `internal/mcp/tool_error_messages_test.go` | New: error message tests |
