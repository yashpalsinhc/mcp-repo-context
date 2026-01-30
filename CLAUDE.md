# Claude Code Instructions for This Project

## CRITICAL: Use MCP Server Instead of Explore Agents

This project has an MCP server (`repo-context`) with **deep pre-analyzed context** for repositories.

**ALWAYS use MCP tools first. They are 5-10x more efficient than Explore agents.**

## Quick Decision Guide

```
Need info about code?
│
├── ANY LOCAL DIRECTORY ─────────────► analyze_local + smart_query
├── "How does X work?" ──────────────► ask (8k tokens)
├── "Find function/type X" ──────────► search_context (2k tokens)
├── "What does this function do?" ───► get_function_context (4k tokens)
├── "Find all DB operations" ────────► search_by_side_effect (3k tokens)
├── "Find auth/validation code" ─────► search_by_concept (3k tokens)
├── "What calls this function?" ─────► get_callers (2k tokens)
├── "Compare these repos" ───────────► compare_repos (10k tokens)
│
└── ONLY if MCP can't answer ────────► Explore agent (50k+ tokens)
```

## NEW: Local Directory Support

### For ANY Local Codebase (No GitHub Required)
```
# First, analyze the local directory
repo-context analyze_local:
  path: "/path/to/your/project"
  force: false

# Then use smart_query for natural language questions
repo-context smart_query:
  query: "what does the login function do?"
  project_id: "local:/path/to/your/project"
```

### Smart Query (No AI Required)
`smart_query` parses your question and routes to the appropriate tool automatically:
- "What does X do?" → function details
- "Who calls Y?" → callers
- "Find DB functions" → side effect search
- "Show auth code" → concept search
- "What's the project structure?" → architecture

## MCP Tools Reference

### For Questions About Code (Most Common)
```
repo-context ask:
  query: "How does authentication work?"
  repo_ids: ["github.com/LambdatestIncPrivate/mobile-management-service"]
```

### For Finding Functions/Types
```
repo-context search_context:
  query: "validateUser"
  search_type: "function"
  repo_id: "github.com/LambdatestIncPrivate/mobile-management-service"
```

### For Deep Function Analysis (NEW - No AI Required)
```
repo-context get_function_context:
  repo_id: "github.com/LambdatestIncPrivate/mobile-management-service"
  file_path: "pkg/handlers/user.go"
  function_name: "CreateUser"
```
Returns: behavior, execution flow, DB queries, HTTP calls, callers, side effects

### For Finding Functions by Behavior
```
# Find all database operations
repo-context search_by_side_effect:
  repo_id: "..."
  effect: "db_query"   # Also: http_call, file_io, logging, db_write

# Find by pattern/concept
repo-context search_by_concept:
  repo_id: "..."
  concept: "authentication"  # Also: validation, handler, crud, middleware
```

### For Call Graph
```
repo-context get_callers:
  repo_id: "..."
  function_name: "ValidateToken"
```

## Pre-Analyzed Repositories

- `github.com/LambdatestIncPrivate/mobile-management-service`
- `github.com/LambdatestIncPrivate/mobile-manual-test-management`
- `github.com/LambdatestIncPrivate/lambda-test-forge-service`
- `github.com/LambdatestIncPrivate/lambda-test-management-service`
- `github.com/LambdatestIncPrivate/lambda-app-upload`
- `github.com/LambdatestIncPrivate/test-management-service-automation`
- `github.com/LambdatestIncPrivate/tms-migrator`

## Token Cost Comparison

| Method | Tokens | When to Use |
|--------|--------|-------------|
| `ask` | ~8k | Questions about how code works |
| `search_context` | ~2k | Find specific functions/types |
| `get_function_context` | ~4k | Understand what a function does |
| `search_by_side_effect` | ~3k | Find DB/HTTP/file operations |
| `search_by_concept` | ~3k | Find auth/validation/handler code |
| Explore agent | ~50k+ | **AVOID - Last resort only** |

## What MCP Provides Without AI

The deep context extraction provides (no AI needed):
- Function behavior summaries
- Execution flow (step-by-step)
- Database queries with SQL
- External HTTP calls
- Request/response payloads
- Call graph (who calls what)
- Side effects detection
- Error handling patterns
- Struct field details with JSON tags

## PR Review with Rich Context

### Get PR Context (No AI)
```
repo-context get_pr_context:
  repo_id: "github.com/org/repo"
  changed_files:
    - path: "pkg/handlers/user.go"
      change_type: "modified"
    - path: "pkg/models/user.go"
      change_type: "added"
```

**Returns for each changed function:**
- What it does (behavior summary)
- Who calls it (callers with their summaries)
- What it calls (internal + external dependencies)
- DB queries with SQL
- HTTP calls to external services
- Side effects
- Error handling patterns

**Impact Analysis:**
- Functions affected (not in PR, but call modified code)
- API routes affected
- Risk assessment (low/medium/high)

### Workflow for PR Review
1. **First:** Run `get_pr_context` to understand changes without AI
2. **Then:** Use `review_pr` for AI-powered review (uses the same context)
3. **Benefit:** AI review is more focused because context is pre-extracted

## When to Use Explore Agent (RARE)

Only use Explore agents when:
1. Repository is not analyzed and cannot be analyzed
2. Need exact code implementation (not just structure/behavior)
3. MCP tools explicitly don't have the answer

## Refreshing Context

To re-analyze repos with latest code:
```
repo-context analyze_repo:
  repo_url: "https://github.com/org/repo"
  branch: "main"
  force: true
```

## Refactoring Workflow (Incremental Updates)

During refactoring, use **incremental updates** to keep context fresh without full re-analysis:

### After Editing a Single File (~10ms)
```
repo-context refresh_file:
  project_id: "local:/path/to/project"
  file_path: "pkg/handlers/user.go"
```

### After Refactoring Session (Refresh All Changed)
```
repo-context refresh_changed:
  project_id: "local:/path/to/project"
```
This checks all files and only refreshes those with changed hashes.

### Recommended Refactoring Flow

1. **Before refactoring**: Use `smart_query` or `get_function_context` to understand code
2. **Make your changes**: Edit the code
3. **Quick refresh**: Run `refresh_file` on the file you edited
4. **Continue querying**: Context is now up-to-date
5. **End of session**: Run `refresh_changed` to catch any missed files

### Token Cost Comparison

| Action | Method | Tokens |
|--------|--------|--------|
| Full re-analysis | `analyze_local force=true` | ~50k+ |
| Single file refresh | `refresh_file` | ~1-2k |
| All changed files | `refresh_changed` | ~2-5k |
