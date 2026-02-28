# Section 5: Per-Org Plugin Selection

## Overview

Wire the MCP server layer to resolve org config (AnalyzerName/EmbedderName) and pass them to the manager and embedder registry. Surface warnings for unknown plugin names in tool output.

## Dependencies

- Section 3 (OrgConfig fields)
- Section 4 (Manager options, ServerConfig wiring)

## Tests First

### File: `internal/mcp/org_plugin_test.go` (new)

```
Test: resolveOrgAnalyzerName returns name from org config
- Register org with config.AnalyzerName="python"
- Add repo to org
- Call resolveOrgConfig(repoID)
- Assert AnalyzerName == "python"

Test: resolveOrgAnalyzerName returns empty for repo not in org
- Call resolveOrgConfig(repoID) for unregistered repo
- Assert AnalyzerName == ""

Test: analyze tool passes AnalyzerName to options
- Register org with AnalyzerName="custom"
- Trigger analyze_local for repo in org
- Assert AnalyzeOptions.AnalyzerName == "custom"

Test: unknown AnalyzerName falls back with warning in output
- Register org with AnalyzerName="nonexistent"
- Register analyzer registry without "nonexistent"
- Trigger analysis
- Assert analysis succeeds (default used)
- Assert tool output contains "Warning" and "nonexistent"

Test: org with no AnalyzerName uses default (no warning)
- Register org with empty config
- Trigger analysis
- Assert default analyzer used
- Assert no warning in output

Test: unknown EmbedderName falls back with warning
- Register org with EmbedderName="nonexistent"
- Trigger indexing
- Assert indexing succeeds (default used)
- Assert output contains warning

Test: per-org embedder selection
- Register embedder "test" in registry
- Register org with EmbedderName="test"
- Trigger indexing
- Assert "test" embedder used

Test: repo override takes precedence over org for analyzer
- Org config: AnalyzerName="org-analyzer"
- Repo override: AnalyzerName="repo-analyzer"
- Call MergeConfigs and resolve
- Assert resolved name is "repo-analyzer"
```

## Implementation Details

### 1. Org Config Resolution Helper

**File: `internal/mcp/server.go` (or new `internal/mcp/org_resolve.go`)**

Add a helper method on the server:

```go
func (s *server) resolveOrgConfig(repoID string) *org.OrgConfig
```

Steps:
1. If s.config.OrgManager is nil, return nil
2. List all orgs, find which org contains repoID
3. If found, get org config (with repo override merged)
4. Return merged config (or nil if repo not in any org)

This is called by tool handlers before triggering analysis.

### 2. Analyzer Selection in Tool Handlers

In the analyze_local and analyze_repo tool handlers:

```go
orgConfig := s.resolveOrgConfig(repoID)
var analyzerName string
if orgConfig != nil {
    analyzerName = orgConfig.AnalyzerName
}

// Pass to manager via options
result, err := s.manager.AnalyzeLocal(ctx, path, orchestrator.AnalyzeLocalOptions{
    AnalyzerName: analyzerName,
})
```

If the manager can't find the named analyzer:
- Log warning
- Fall back to default
- Add warning to the analysis result or tool output

### 3. Warning Surfacing

Warnings must appear in the tool's markdown output, not just server logs. Two approaches:

**Option A:** Manager returns warnings in the result struct. Add `Warnings []string` to AnalysisResult (or equivalent).

**Option B:** The MCP tool handler checks the analyzer name against the registry before calling the manager. If not found, add a warning to the output prefix.

Prefer **Option B** — it keeps the manager simple and the MCP layer handles user-facing concerns.

```go
var warnings []string
if analyzerName != "" {
    // Check if analyzer exists in registry
    found := false
    for _, name := range registeredAnalyzerNames {
        if name == analyzerName {
            found = true
            break
        }
    }
    if !found {
        warnings = append(warnings, fmt.Sprintf(
            "Warning: Analyzer '%s' not found in registry, using default analyzer.", analyzerName))
        analyzerName = "" // reset to default
    }
}
```

Prepend warnings to the tool output markdown.

### 4. Embedder Selection for Indexing

In the index_repository and auto-index code paths:

```go
orgConfig := s.resolveOrgConfig(repoID)
embedder := s.embedderRegistry.Default()
if orgConfig != nil && orgConfig.EmbedderName != "" {
    named := s.embedderRegistry.Get(orgConfig.EmbedderName)
    if named != nil {
        embedder = named
    } else {
        warnings = append(warnings, fmt.Sprintf(
            "Warning: Embedder '%s' not found, using default.", orgConfig.EmbedderName))
    }
}
```

Since all embedders in the registry share the same dimension (validated at construction), there's no risk of dimension mismatch.

### 5. Scope of Changes

Tool handlers that need org plugin resolution:
- `analyze_local` — analyzer selection
- `analyze_repo` — analyzer selection
- `index_repository` — embedder selection
- `analyze_org` — for each repo, resolve per-repo config

Other tools (search, query, etc.) don't need changes — they work with already-analyzed data.

## Error Handling

- Org not found for repo: no error, use defaults (many repos aren't in orgs)
- Unknown analyzer name: warning in output, use default
- Unknown embedder name: warning in output, use default
- OrgManager nil: skip org resolution entirely (backward compat)

## File Summary

| File | Action |
|------|--------|
| `internal/mcp/org_resolve.go` | New: resolveOrgConfig helper |
| `internal/mcp/org_plugin_test.go` | New: per-org plugin selection tests |
| `internal/mcp/tool_analyze_*.go` | Modify: add org config resolution before analysis |
| `internal/mcp/tool_index.go` | Modify: add embedder selection from org config |
