# Research: Agent Workflows

## Key Findings

### Comparison Tools (`internal/comparison/`)
- Comparer interface: Compare, FindDuplicates, FindConflicts, FindGaps, AnalyzeConsistency
- CompareResult contains: Repos, UnifiedStatistics, Duplicates, Conflicts, Gaps, Consistency, Recommendations
- DuplicateGroup: type, instances, similarity (1.0 for exact), recommendation
- Conflict: type (signature_mismatch, behavior_difference, naming_conflict), severity, resolution
- Gap: type, source repos, priority, description, file path
- Options pattern with thresholds and toggles
- Works with *RepoContext directly, no remote calls

### Orchestrator (`internal/orchestrator/`)
- Manager interface: 55 methods covering analysis, queries, search, PR context, AI ops
- SmartQuery routes queries to appropriate tools via regex patterns (no AI)
- QueryTypes: function, type, side_effect, concept, callers, calls, flow, file, package, architecture, general
- PR context: rich extraction without LLM (callers, callees, side effects, DB queries, HTTP calls)
- Per-repo locking for concurrent analysis
- Incremental updates via RefreshFile/RefreshChanged

### Call Graph (`internal/graph/`)
- Per-repo call graph built from AST
- CallGraphNode: ID, File, Function, Calls, CalledBy
- Mermaid/DOT visualization support
- Depth-limited traversal (1-5 levels)
- Cross-repo: NOT supported natively — within single repo only

### MCP Tool Patterns
- 35 tools total, standard handler pattern: extract args → load contexts → domain logic → format markdown
- Progressive disclosure: compact summaries with DetailRef for drill-down
- Error pattern: return markdown with actionable suggestion
- Token budgeting for large results

### Composable Patterns (`internal/compose/`)
- Chain execution with steps, conditions, success/error handlers
- Variable interpolation: `{{variable}}` between steps
- PatternRegistry with pre-built patterns (search_with_context, impact_analysis, find_and_expand)
- ChainContext with Vars and Results for state passing

### Org Infrastructure (`internal/org/`)
- Manager: Register, List, Get, AddRepos, AnalyzeOrg
- Concurrent analysis with bounded semaphore (default 3, max 10)
- Config inheritance with per-repo overrides

### Token Budgeting (`internal/tokens/`)
- ScoredItem[T] generic with Item, Score, TokenCost
- Budgeter: greedy fill by score, SummarizeFunction fallback
- TokenCounter: chars/4 approximation
