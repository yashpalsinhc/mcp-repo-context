# Implementation Plan: Agent Workflows

## Overview

Add three high-level workflow tools (build_feature, refactor_org, merge_repos) that orchestrate existing tools to reduce fragmented agent calls. Each tool performs algorithmic analysis with optional AI enhancement, produces token-budgeted advisory output.

**Execution model:** Sequential for v1. Each workflow runs steps serially within a single goroutine. Future optimization: parallelize per-repo operations with errgroup.

**Dependencies:** Requires 01-org-abstraction (org store). Benefits from 01-core-bug-fixes (receiver-qualified conflict matching) and 02-dependency-graph (module deps for merge order). Works without them but with reduced accuracy.

## Current Architecture

### What Exists
- **Comparer** (`internal/comparison/`): FindDuplicates, FindConflicts, FindGaps, AnalyzeConsistency — works with RepoContext
- **SemanticSearch** (`internal/vectors/`): SearchByOrg, SearchFunctions — cross-repo vector search
- **SmartQuery** (`internal/orchestrator/`): Pattern-based query routing, no AI needed
- **Call graph** (`internal/graph/`): Per-repo call graph with callers/callees, Mermaid visualization. **Per-repo only — no cross-repo edges.**
- **Compose** (`internal/compose/`): Chain execution, variable passing, conditions
- **Org Manager** (`internal/org/`): Org CRUD, repo listing, concurrent analysis
- **Budgeter** (`internal/tokens/`): ScoredItem, greedy fill, summarize fallback
- **35 MCP tools**: Standard handler pattern, progressive disclosure, markdown output

### What's Missing
1. build_feature workflow tool
2. refactor_org workflow tool
3. merge_repos workflow tool (extends compare_repos)
4. Shared workflow infrastructure (result types, formatting)

### Cross-Repo Limitation

Cross-repo dependency detection is **best-effort, name-based matching only**. The call graph is per-repo; there is no interface-to-implementation resolution across repos. Dependencies are inferred by matching function names across repos (same approach as the comparison module's `normalizeFunctionKey`). This is acknowledged and acceptable for v1.

## Section-by-Section Plan

### Section 1: Workflow Types & Shared Infrastructure

**Goal:** Define shared types and formatting utilities used by all three workflow tools.

**New package: `internal/workflows/`**

**Types:**

```go
type RiskLevel string

const (
    RiskLow    RiskLevel = "low"
    RiskMedium RiskLevel = "medium"
    RiskHigh   RiskLevel = "high"
)
```

```go
type FeaturePlan struct {
    Feature     string              // Feature description
    OrgID       string
    RelevantCode []CodeLocation      // Semantic search results
    EntryPoints  []EntryPoint       // Where to start implementing
    Dependencies []Dependency       // Cross-repo dependencies (best-effort)
    FilesToTouch []FileAction       // Files that need changes
    SuggestedOrder []string         // Order to implement
    RiskLevel    RiskLevel
    AIEnhancement string           // Optional AI-generated summary
}

type RefactorPlan struct {
    Pattern      string             // Pattern being refactored
    OrgID        string
    Usages       []CodeLocation     // Where pattern is used
    AffectedFiles []FileAction
    ImpactAnalysis ImpactSummary   // Callers, callees affected
    RiskLevel    RiskLevel
    AIEnhancement string
}

type MergeReport struct {
    SourceRepos  []string
    TargetRepo   string
    Duplicates   []DuplicateSummary
    Conflicts    []ConflictSummary
    Gaps         []GapSummary
    MergeOrder   []MergeStep
    RiskLevel    RiskLevel
}

type CodeLocation struct {
    RepoID    string
    FilePath  string
    FuncName  string
    Line      int
    Summary   string
    Score     float64  // Relevance score
}

type FileAction struct {
    RepoID   string
    FilePath string
    Action   string  // "modify", "create", "review"
    Reason   string
}

type EntryPoint struct {
    RepoID   string
    FilePath string
    FuncName string
    Why      string  // Why this is an entry point
}

type Dependency struct {
    SourceRepoID string
    SourceFunc   string
    TargetRepoID string
    TargetFunc   string
    Type         string  // "name_match", "module_import"
    Confidence   string  // "high" (exact match), "medium" (partial)
}

type ImpactSummary struct {
    DirectCallers   int
    IndirectCallers int
    AffectedRepos   []string
    HotPaths        []string  // Functions with >5 callers
}

type DuplicateSummary struct {
    FunctionName string
    Repos        []string  // Which repos have this function
    Similarity   float64
    Recommendation string  // "keep target", "keep source X", "merge"
}

type ConflictSummary struct {
    FunctionName string
    Type         string  // from comparison.Conflict.Type
    Severity     string
    SourceRepo   string
    Resolution   string  // Suggested resolution
}

type GapSummary struct {
    FunctionName string
    SourceRepo   string
    Priority     string
    Description  string
}

type MergeStep struct {
    Order       int
    Action      string  // "migrate", "resolve_conflict", "fill_gap", "consolidate_duplicate"
    SourceRepo  string
    TargetItem  string  // Function/type name
    Description string
    Risk        RiskLevel
}
```

**Formatting utilities:**

`FormatFeaturePlan(plan *FeaturePlan, budget int) string` — markdown with sections
`FormatRefactorPlan(plan *RefactorPlan, budget int) string`
`FormatMergeReport(report *MergeReport, budget int) string`

All formatters use token budgeting to stay within response limits. **Budget allocation strategy:**
- 40% for relevant code / usages / duplicates (the core data)
- 30% for dependencies / impact / conflicts (the relationships)
- 20% for files to touch / merge steps / gaps (the action items)
- 10% for metadata, risk assessment, AI enhancement summary

When a section exceeds its allocation, items are sorted by score/relevance and truncated. The existing `ScoredItem[T]` budgeter is used by wrapping each list item as a `ScoredItem` with appropriate score (relevance score for CodeLocation, severity for conflicts, priority for gaps).

### Section 2: build_feature Tool

**Goal:** "Build feature X across repos" — find relevant code, identify entry points and dependencies, return implementation plan.

**Tool definition:**
```
Name: "build_feature"
Description: "Plan how to build a feature across repositories in an organization"
Parameters:
  - org_id (string, required)
  - feature_description (string, required, min length 3)
  - target_repos (array of string, optional): limit to specific repos
  - token_budget (integer, optional, default 4000, max 32000)
```

**Input validation:**
- feature_description must be at least 3 characters
- token_budget must be between 500 and 32000
- If target_repos provided, validate each is a member of the org
- All repos in org must be analyzed (have context loaded)

**Handler flow:**

1. **Validate org and get repos** — same pattern as search_org
2. **Semantic search** for relevant code:
   - SearchByOrg with feature_description as query
   - If target_repos specified, post-filter results by repo ID
   - Get top 20 relevant functions
   - **Fallback:** If semantic search unavailable (no vectors indexed), fall back to keyword search via SmartQuery per repo, warn in output
3. **Identify entry points**:
   - For each relevant function, check if it's a handler/endpoint (IsPublic, has HTTP/handler concept)
   - Entry points = public functions with no callers (or few callers) that relate to the feature
4. **Identify dependencies** (best-effort, name-based):
   - For top relevant functions, get per-repo call graph (callers and callees)
   - Cross-reference function names across repos: if function A in repo-1 calls function B, and repo-2 also has function B, that's a potential dependency with confidence "medium"
   - If 02-dependency-graph is available, also check module imports for "high" confidence dependencies
5. **Determine files to touch**:
   - All files containing relevant functions -> "modify"
   - Files containing entry points -> "modify" (primary)
   - If gap analysis needed -> "create"
6. **Suggest implementation order**:
   - Shared/base repos first (most depended upon)
   - Then service repos (depend on shared)
   - Within a repo: dependencies before dependents
7. **Risk assessment**:
   - Count affected repos -> more repos = higher risk
   - Count affected functions -> more = higher risk
   - Public API changes -> high risk
   - Thresholds: >5 repos or >20 functions = high, 2-5 repos or 5-20 = medium, else low
8. **Optional AI enhancement**:
   - If AI is available (Ask method works): Ask with truncated context (top 10 items per list)
   - If AI unavailable: skip, return algorithmic results only

### Section 3: refactor_org Tool

**Goal:** "Refactor pattern Y across org" — find usages, analyze impact, return refactoring plan.

**Tool definition:**
```
Name: "refactor_org"
Description: "Plan a refactoring across repositories in an organization"
Parameters:
  - org_id (string, required)
  - pattern_description (string, required, min length 3)
  - target_repos (array of string, optional)
  - token_budget (integer, optional, default 4000, max 32000)
```

**Handler flow:**

1. **Validate org and get repos**
2. **Find pattern usages**:
   - Semantic search: SearchByOrg with pattern_description
   - Concept search: iterate SearchByConcept per repo if pattern maps to a known concept (e.g., "authentication", "validation", "handler"), then merge results
   - Combine results, deduplicate by (repoID, filePath, funcName)
3. **Group usages by repo and file**:
   - Map: repoID -> []CodeLocation
   - Identify which repos have the most usages (refactoring hotspots)
4. **Impact analysis per repo**:
   - For each usage, get callers via per-repo call graph
   - Count total affected functions (direct + indirect callers)
   - Cross-repo impact: best-effort name matching of callers across repos
5. **Build affected files list**:
   - Files containing pattern usages -> "modify"
   - Files containing callers of usages -> "review" (may need updates)
6. **Risk assessment**:
   - Number of repos affected
   - Number of public functions affected
   - Whether the pattern is in hot paths (many callers)
   - High: >5 repos or >20 functions affected
   - Medium: 2-5 repos or 5-20 functions
   - Low: 1 repo or <5 functions
7. **Optional AI enhancement**:
   - If available: Ask with truncated context (top 10 usages, top 5 impact items)

### Section 4: merge_repos Tool

**Goal:** Extend compare_repos with merge strategy — advisory report with duplicates, conflicts, gaps, and suggested merge order.

**Tool definition:**
```
Name: "merge_repos"
Description: "Generate a merge strategy report for consolidating repositories"
Parameters:
  - source_repo_ids (array of string, required, min 1)
  - target_repo_id (string, required)
  - token_budget (integer, optional, default 8000, max 32000)
```

**Input validation:**
- source_repo_ids must have at least 1 entry
- target_repo_id must not be in source_repo_ids (if it is, remove it and warn)
- All repos must be analyzed

**Handler flow:**

1. **Load all repo contexts** — same as compare_repos
2. **Run all comparison analyses**:
   - `comparer.FindDuplicates(ctx, allContexts)` — what's duplicated
   - `comparer.FindConflicts(ctx, sourceContexts, targetContext)` — what conflicts
   - `comparer.FindGaps(ctx, sourceContexts, targetContext)` — what's missing from target
   - `comparer.AnalyzeConsistency(ctx, allContexts)` — naming/pattern consistency
   - **Note:** FindConflicts matches by bare function name. If 01-core-bug-fixes adds receiver-qualified matching, conflict accuracy improves. Until then, false positives are possible for common method names.
3. **Generate merge order**:
   - Sort source repos by dependency: repos with fewer external deps first
   - **Circular dependency handling:** If topological sort detects a cycle, break it by alphabetical order of repo ID and emit a warning in the report ("Circular dependency detected between repos X and Y; ordering alphabetically")
   - Within each repo, sort items by:
     a. Types/interfaces first (they're depended on)
     b. Utility functions next
     c. Business logic last
4. **Build advisory report**:
   - **Duplicates**: group by function name, show where each appears, recommend which to keep (prefer target's version, or most complete)
   - **Conflicts**: show each conflict with severity, suggest resolution (prefer target's signature, or show both options)
   - **Gaps**: prioritize by usage count, show what's missing from target
   - **Merge steps**: ordered list of actions (migrate X from repo-A, resolve conflict Y, etc.)
5. **Risk assessment**:
   - Count conflicts by severity
   - Count public API changes
   - High: any high-severity conflicts or >10 conflicts total
   - Medium: only medium conflicts, or >20 gaps
   - Low: no conflicts, few gaps

### Section 5: AI Enhancement Layer

**Goal:** Shared AI enhancement for all workflow tools.

**New: `internal/workflows/ai_enhance.go`**

```go
func EnhanceWithAI(ctx context.Context, askFunc AskFunc, prompt string) (string, error)
```

Where `AskFunc` is `func(ctx context.Context, query string, repoIDs []string) (*QueryResult, error)` — matches the manager's Ask method.

**Timeout:** EnhanceWithAI wraps the provided ctx with a 30-second timeout via `context.WithTimeout`. If AI takes longer, return empty string with no error (degrade gracefully).

**Prompt size limit:** Before calling Ask, truncate the prompt to a maximum of 4000 characters. Lists embedded in prompts (entry points, dependencies, usages) are capped at 10 items each with "... and N more" suffix.

**QueryResult to string mapping:** Extract the `Answer` field from QueryResult. If QueryResult has a `Summary` or `Answer` string field, use that directly.

**Usage pattern:**
```go
if s.aiAvailable() {
    enhancement, err := workflows.EnhanceWithAI(ctx, s.manager.Ask, prompt)
    if err != nil {
        log.Printf("AI enhancement failed: %v", err)
        // Continue without AI — never fail the workflow
    } else {
        plan.AIEnhancement = enhancement
    }
}
```

**AI prompts:**

For build_feature:
```
Given the following analysis for building feature "{description}":
- {N} relevant functions found across {repos}
- Entry points: {top 10 list}
- Key dependencies: {top 10 list}

Provide a brief implementation strategy (2-3 paragraphs).
```

For refactor_org:
```
The pattern "{pattern}" appears {N} times across {repos}.
Impact: {affected_count} functions directly affected, {caller_count} indirect callers.

Suggest a safe refactoring approach (2-3 paragraphs).
```

**AI availability check:**
- `aiAvailable()` returns true if the manager has an AI registry configured (non-nil)
- If not configured, return false — no error, no attempt
- All tools work without AI — it's purely additive

### Section 6: Integration Tests

**Goal:** End-to-end tests for all three workflow tools.

**Test scenarios:**

build_feature:
1. Build feature returns relevant code from multiple repos
2. Entry points are identified (public functions, handlers)
3. Dependencies between repos identified (name-based matching)
4. Files to touch includes all relevant files
5. Implementation order respects dependencies
6. Token budget limits output
7. AI enhancement added when available
8. Works without AI (returns algorithmic results)
9. Empty feature_description rejected (min length 3)
10. target_repos with invalid repo returns error
11. Semantic search fallback to keyword when vectors not indexed

refactor_org:
1. Find pattern usages across repos
2. Impact analysis counts callers correctly
3. Risk level reflects scope of change
4. Affected files include both usages and callers
5. Token budget limits output
6. Concept search merged with semantic results, deduplicated

merge_repos:
1. Combines FindDuplicates + FindConflicts + FindGaps
2. Merge order puts dependencies first
3. Advisory report includes all three analyses
4. Risk level reflects conflict severity
5. Token budget limits output
6. Works with 2 or more source repos
7. Target in sources: removed and warned
8. Circular dependency in merge order: alphabetical fallback with warning
9. Empty org (no repos) returns descriptive error
10. Large result set truncated by token budget

## Error Handling

- Unknown org: return error with org_id
- No repos in org: return error suggesting analyze_org
- Repos not analyzed: return error listing which repos need analysis
- Semantic search unavailable: fall back to keyword search, warn in output
- AI unavailable: skip enhancement, return algorithmic results
- AI timeout (>30s): skip enhancement, return algorithmic results
- Token budget too small (<500): return summary only with note
- Token budget too large (>32000): cap at 32000
- Empty/short description: return validation error

## Performance Considerations

- Semantic search is the most expensive operation (vector comparison)
- Comparison analysis works in-memory with loaded contexts
- Call graph traversal is per-repo (fast)
- AI enhancement is optional, has 30s timeout
- Token budgeting prevents oversized responses
- **V1 is sequential.** Future: parallelize per-repo operations with errgroup
