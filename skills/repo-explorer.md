# Repo Explorer Skill

Use this skill when exploring codebases. **Always prefer MCP repo-context tools over Explore agents** - they use 5-10x fewer tokens.

## Quick Reference

| Task | MCP Tool | Tokens | AI Required |
|------|----------|--------|-------------|
| Explore local codebase | `analyze_local` + `smart_query` | ~3-5k | No |
| "How does X work?" | `ask` | ~5-10k | Yes |
| "What does function X do?" | `get_function_context` | ~3-5k | No |
| "Find function X" | `search_context` | ~1-5k | No |
| "What calls this?" | `get_callers` | ~2k | No |
| "Find DB operations" | `search_by_side_effect` | ~2-3k | No |
| "Find auth/validation" | `search_by_concept` | ~2-3k | No |
| Understand PR changes | `get_pr_context` | ~5-10k | No |
| AI-powered PR review | `review_pr` | ~15-20k | Yes |
| After editing code | `refresh_file` | ~1-2k | No |
| Compare repos | `compare_repos` | ~5-10k | No |
| Explore agent (AVOID) | - | ~50k+ | - |

## Master Decision Tree

```
What do you need?
│
├── EXPLORE/UNDERSTAND CODE
│   │
│   ├── Local directory (not GitHub)?
│   │   └── analyze_local → smart_query
│   │
│   ├── Specific function details?
│   │   └── get_function_context (behavior, callers, DB queries, etc.)
│   │
│   ├── Find functions by name?
│   │   └── search_context query="funcName" search_type="function"
│   │
│   ├── Find by behavior (DB, HTTP, files)?
│   │   └── search_by_side_effect effect="db_query|http_call|file_io"
│   │
│   ├── Find by concept (auth, validation)?
│   │   └── search_by_concept concept="authentication|validation|handler"
│   │
│   ├── Who calls this function?
│   │   └── get_callers function_name="X"
│   │
│   ├── Complex question about code?
│   │   └── ask query="How does authentication work?"
│   │
│   └── ONLY if nothing works → Explore agent
│
├── REVIEW PR
│   │
│   ├── Understand PR changes (no AI)?
│   │   └── get_pr_context (callers, callees, DB, impact)
│   │
│   └── Full AI review?
│       └── review_pr (uses same context + AI analysis)
│
├── REFACTORING
│   │
│   ├── Before editing?
│   │   └── smart_query or get_function_context
│   │
│   ├── After editing single file?
│   │   └── refresh_file (~10ms)
│   │
│   └── End of refactoring session?
│       └── refresh_changed (all modified files)
│
└── COMPARE REPOS
    └── compare_repos repo_ids=["repo1", "repo2"]
```

## Detailed Tool Reference

### 1. Local Directory Analysis (No GitHub Required)

**When:** Any local codebase on disk

```yaml
# Step 1: Analyze (one-time)
repo-context analyze_local:
  path: "/absolute/path/to/project"
  force: false  # true to re-analyze

# Step 2: Query naturally
repo-context smart_query:
  query: "what does the createUser function do?"
  project_id: "local:/absolute/path/to/project"
```

**smart_query understands:**
- "What does X do?" → function details
- "Who calls Y?" → callers
- "Find DB functions" → side effect search
- "Show auth code" → concept search
- "Project structure?" → architecture

---

### 2. Deep Function Analysis (No AI)

**When:** Need to understand exactly what a function does

```yaml
repo-context get_function_context:
  repo_id: "github.com/org/repo"
  file_path: "pkg/handlers/user.go"
  function_name: "CreateUser"
```

**Returns:**
- Behavior summary (auto-generated)
- Execution steps
- Who calls this function (with their summaries)
- What this function calls
- Database queries with SQL
- External HTTP calls
- Side effects
- Error handling patterns
- Related types

---

### 3. PR Context Analysis (No AI)

**When:** Need to understand PR changes deeply before or instead of AI review

```yaml
repo-context get_pr_context:
  repo_id: "github.com/org/repo"
  changed_files:
    - path: "pkg/handlers/user.go"
      change_type: "modified"
    - path: "pkg/models/user.go"
      change_type: "added"
    - path: "pkg/old/legacy.go"
      change_type: "deleted"
```

**Returns for each changed function:**
- Behavior summary
- Callers (who uses this - impact!)
- Callees (dependencies)
- DB queries, HTTP calls
- Side effects

**Impact Analysis:**
- Functions affected (not in PR but call modified code)
- API routes affected
- Database tables affected
- Risk level (low/medium/high) with reasons

**Use Case:** Run this BEFORE `review_pr` to understand changes, or use standalone for quick PR understanding without AI.

---

### 4. Refactoring Workflow

**Problem:** Context gets stale when you edit code

**Solution:** Incremental updates (~10ms vs full re-analysis)

```yaml
# After editing a single file
repo-context refresh_file:
  project_id: "local:/path/to/project"
  file_path: "pkg/handlers/user.go"
  force: false  # true to refresh even if unchanged

# After editing multiple files
repo-context refresh_changed:
  project_id: "local:/path/to/project"
```

**Recommended Flow:**
1. `smart_query` → understand code
2. Edit code in your editor
3. `refresh_file` → update context (~10ms)
4. `smart_query` → verify changes
5. End of session → `refresh_changed`

---

### 5. Search Tools (No AI)

**Find by name:**
```yaml
repo-context search_context:
  query: "validateUser"
  search_type: "function"  # or "type", "file", "all"
  repo_id: "github.com/org/repo"
```

**Find by side effect:**
```yaml
repo-context search_by_side_effect:
  repo_id: "github.com/org/repo"
  effect: "db_query"  # db_query, http_call, file_io, logging, db_transaction
```

**Find by concept:**
```yaml
repo-context search_by_concept:
  repo_id: "github.com/org/repo"
  concept: "authentication"  # validation, handler, crud, middleware, error
```

**Find callers:**
```yaml
repo-context get_callers:
  repo_id: "github.com/org/repo"
  function_name: "ValidateToken"
```

---

### 6. AI-Powered Tools

**Natural language questions:**
```yaml
repo-context ask:
  query: "How does authentication work in this service?"
  repo_ids: ["github.com/org/repo"]
```

**Full PR review:**
```yaml
repo-context review_pr:
  pr_url: "https://github.com/org/repo/pull/123"
  add_comments: true
  severity_level: "all"  # critical, important, all
  focus_areas: ["security", "performance"]
```

---

## Token Cost Comparison

| Scenario | Old Method | MCP Method | Savings |
|----------|------------|------------|---------|
| Understand function | Explore (50k) | get_function_context (4k) | **92%** |
| Find DB operations | Grep + Read (30k) | search_by_side_effect (3k) | **90%** |
| Understand PR | Read files (40k) | get_pr_context (8k) | **80%** |
| Refactor + verify | Re-analyze (50k) | refresh_file (1k) | **98%** |
| Complex question | Explore (50k) | ask (10k) | **80%** |

---

## Pre-Analyzed Repositories

Check available repos:
```yaml
repo-context list_repos
```

Common ones:
- `github.com/LambdatestIncPrivate/mobile-management-service`
- `github.com/LambdatestIncPrivate/mobile-manual-test-management`
- `github.com/LambdatestIncPrivate/lambda-test-forge-service`
- `github.com/LambdatestIncPrivate/lambda-test-management-service`
- `github.com/LambdatestIncPrivate/lambda-app-upload`

---

## When to Use Explore Agent (RARE)

**ONLY use Explore agents when:**
1. Repository cannot be analyzed (no access, too large)
2. Need exact code implementation (not structure/behavior)
3. MCP tools explicitly don't have the answer
4. Need to search across non-code files

**Before using Explore, try:**
1. `smart_query` for natural language
2. `get_function_context` for function details
3. `search_by_concept` for related code
4. `ask` for AI-powered answers

---

## Quick Patterns

### "I need to understand this codebase"
```
analyze_local path="/path/to/code"
smart_query query="what is the project structure?"
smart_query query="how does authentication work?"
```

### "I need to review this PR"
```
get_pr_context repo_id="..." changed_files=[...]
# Read the impact analysis first
# Then optionally:
review_pr pr_url="..." add_comments=true
```

### "I need to refactor this function"
```
get_function_context repo_id="..." function_name="X"
# Understand callers, callees, side effects
# Make changes
refresh_file project_id="..." file_path="..."
# Verify with smart_query
```

### "I need to find all database operations"
```
search_by_side_effect repo_id="..." effect="db_query"
# For each result, optionally:
get_function_context for deeper details
```

### "What will break if I change this?"
```
get_function_context repo_id="..." function_name="X"
# Check "Called by" section
# Or for PR:
get_pr_context # Check Impact Analysis
```
