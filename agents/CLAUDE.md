# Claude Code Instructions for This Project

## Running Modes

The MCP server can run in two modes:

### Native Mode (Recommended for local directories)
Run the server directly on your machine for full filesystem access:
```bash
./scripts/use-native.sh
```
Then restart Claude Code.

### Docker Mode
Run in Docker container (requires volume mounts for local directories):
```bash
./scripts/use-docker.sh
```
Then restart Claude Code.

**Note:** If `analyze_local` fails with "directory not found", you're likely running in Docker mode without proper volume mounts. Switch to native mode or update docker-compose.yml to mount your project directories.

---

## CRITICAL: Use MCP Server Instead of Explore Agents

This project has an MCP server (`repo-context`) with **deep pre-analyzed context** for repositories.

**ALWAYS use MCP tools first. They are 5-10x more efficient than Explore agents.**

## Quick Decision Guide

```
Need info about code?
│
├── ANY LOCAL DIRECTORY ─────────────► analyze_local + smart_query
├── "Structure of X package?" ───────► get_package_structure (3k tokens) [NEW]
├── "How does X work?" ──────────────► ask (8k tokens)
├── "Find function/type X" ──────────► search_context (2k tokens)
├── "What does this function do?" ───► get_function_context (4k tokens)
├── "Find all DB operations" ────────► search_by_side_effect (3k tokens)
├── "Find auth/validation code" ─────► search_by_concept (3k tokens)
├── "What calls this function?" ─────► get_callers (2k tokens)
├── "Compare these repos" ───────────► compare_repos (10k tokens)
├── "Find similar code" ─────────────► semantic_search (3k tokens)
├── "Get context within budget" ─────► get_context_budgeted (varies)
├── "Visualize call graph" ──────────► visualize_call_graph (2k tokens)
│
└── ONLY if MCP can't answer ────────► Explore agent (50k+ tokens)
```

## NEW Tools (Phase 4 & 5)

### Package Structure (NEW)
Get detailed structure of a package/directory with all files, types, and functions:
```
repo-context get_package_structure:
  project_id: "local:/path/to/project"
  package_path: "service/test/create"
```
Returns:
- All files in the package with their purposes
- Types defined in each file (with descriptions)
- Key functions with behavior summaries
- Side effects (db_query, http_call, etc.)

Also works via `smart_query`:
```
repo-context smart_query:
  query: "What is the structure of the service/test/create package?"
  project_id: "local:/path/to/project"
```

### Semantic Search (Vector-Based)
Find code by meaning, not just keywords:
```
# First, index the repository (one-time)
repo-context index_repository:
  repo_id: "github.com/org/repo"

# Then search semantically
repo-context semantic_search:
  query: "handle user authentication and session management"
  repo_id: "github.com/org/repo"
  limit: 10
  type: "function"  # or "type" or "all"
```

### Token-Budgeted Context
Get the most relevant context that fits within a token budget:
```
repo-context get_context_budgeted:
  repo_id: "github.com/org/repo"
  query: "authentication"
  token_budget: 4000  # Default: 4000, Max: 32000
```
Returns the most relevant functions ranked by score, automatically summarized to fit budget.

### Call Graph Visualization
Generate visual diagrams of function relationships:
```
repo-context visualize_call_graph:
  repo_id: "github.com/org/repo"
  function_name: "CreateUser"
  format: "mermaid"  # or "dot" for Graphviz
  depth: 2  # How many levels to traverse
```

### Composable Patterns
Run pre-defined tool chains for common workflows:
```
# List available patterns
repo-context list_patterns

# Execute a pattern
repo-context execute_pattern:
  pattern_name: "search_with_context"
  params:
    repo_id: "github.com/org/repo"
    query: "authentication"
```

Available patterns:
- `search_with_context` - Search then get full context for top results
- `impact_analysis` - Analyze impact of changes to a function
- `find_and_expand` - Find items then expand details

### Usage Analytics
Track token usage across tools:
```
repo-context get_usage_stats
repo-context get_usage_stats:
  tool: "search_context"  # Filter by specific tool
  limit: 50
```

## Local Directory Support

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

### For Deep Function Analysis (No AI Required)
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

## Token Cost Comparison

| Method | Tokens | When to Use |
|--------|--------|-------------|
| `smart_query` | ~2-4k | Natural language questions about local code |
| `search_context` | ~2k | Find specific functions/types by keyword |
| `semantic_search` | ~3k | Find code by meaning (requires indexing) |
| `get_function_context` | ~4k | Understand what a function does |
| `get_context_budgeted` | varies | Get relevant context within token limit |
| `search_by_side_effect` | ~3k | Find DB/HTTP/file operations |
| `search_by_concept` | ~3k | Find auth/validation/handler code |
| `get_callers` | ~2k | Call graph analysis |
| `visualize_call_graph` | ~2k | Generate call graph diagrams |
| `execute_pattern` | ~5-10k | Run composed tool chains |
| `ask` | ~8k | AI-powered questions |
| **Explore agent** | **~50k+** | **AVOID - Last resort only** |

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
- Semantic similarity search (vector-based)
- Call graph visualizations (Mermaid/DOT)

## When to Use Explore Agent (RARE)

Only use Explore agents when:
1. Repository is not analyzed and cannot be analyzed
2. Need exact code implementation (not just structure/behavior)
3. MCP tools explicitly don't have the answer

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
