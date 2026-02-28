# Section 05: review_architecture Recipe

## Overview

Org-wide or multi-repo architecture assessment with service inventory, health indicators, shared pattern detection, and AI-generated recommendations. Degrades gracefully when dependency data is unavailable.

## Dependencies

- Section 01 (Recipe interface, RecipeRunner, types)
- Internal: `internal/orchestrator` (Manager.GetContext, Manager.ListRepos), `internal/org` (Manager.GetOrg), comparison module if available

## Recipe Registration

Register as "review_architecture" in `DefaultRegistry()`.

## Input Schema

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| org_id | string | conditional | - | Org ID (one of org_id or repo_ids required) |
| repo_ids | []string | conditional | - | Repo IDs (alternative to org_id) |
| token_budget | int | no | 12000 | Max context tokens |
| focus_areas | []string | no | [] | Focus: "dependencies", "testing", "patterns", "health" |

Validation: at least one of `org_id` or `repo_ids` must be provided.

## Execution Steps

### Step 1: Resolve Repo List

If `org_id` provided:
- Call `runner.OrgManager().GetOrg(orgID)` to get org with repo list
- Extract repo IDs

If `repo_ids` provided directly, use those.

### Step 2: Service Inventory

For each repo, get full `*RepoContext` from `runner.Manager().GetContext(repoID, "full")` (extract architecture fields in recipe code — no scope parameter on Manager interface).

Extract per repo:
```
type ServiceInfo struct {
    Name          string  // repo name
    RepoID        string
    Language      string  // from architecture
    Framework     string  // primary framework detected
    FileCount     int
    FunctionCount int
    TestFileCount int
    Purpose       string  // from AI summary if available, else ""
}
```

If GetContext returns error for a repo, add GapNote for that repo and continue with others.

### Step 3: Dependency Analysis

Check if dependency graph data is available (from 02-dependency-graph).

**V1:** Dependency data not available. Add GapNote:
```
GapNote{
    Section: "dependency_graph",
    Reason: "Dependency analysis requires 02-dependency-graph",
    Suggestion: "Implement 02-dependency-graph for cross-repo dependency tracking",
}
```

Set `Data["dependency_graph"] = nil`.

**Future:** Build graph showing which repos depend on which. Identify circular deps, tight coupling, orphans.

### Step 4: Pattern Analysis

Compare repos for shared patterns. If multiple repos:
- Count common external dependencies (from import analysis in architecture)
- Detect shared frameworks (gorilla/mux, chi, gin, etc.)
- Identify testing patterns (testify, go test, etc.)
- Note inconsistencies (one repo uses gorilla, another uses chi)

Use existing data from architecture context. Don't call compare_repos for now (it's a heavier operation) — extract patterns from the already-loaded contexts.

Output:
```
type SharedPattern struct {
    Pattern     string   // "router framework", "testing library", etc.
    Value       string   // "gorilla/mux"
    RepoCount   int      // how many repos use it
    Repos       []string // which repos
}
```

### Step 5: Health Indicators

Per repo:
```
type RepoHealth struct {
    RepoID        string
    TestFiles     int     // count of _test.go files
    TestRatio     float64 // test files / total files
    FunctionCount int
    HasReadme     bool
    HasCIConfig   bool    // presence of .github/workflows or .gitlab-ci.yml
    Issues        []string // detected issues
}
```

Issues detection:
- TestRatio < 0.1 → "Low test coverage"
- No README → "Missing documentation"
- No CI config → "No CI/CD detected"
- Very high function count (>500) → "Large codebase, consider splitting"

### Step 6: AI Recommendations

If `runner.AI()` available:
- Build prompt with service inventory, patterns, health indicators
- Ask for architectural recommendations
- Focus on: consistency, testing gaps, potential improvements
- Use `CompleteRaw(ctx, prompt, 1500)`
- Store in `RecipeResult.Analysis`

Without AI:
- Leave Analysis empty
- Structural data (inventory, patterns, health) is still valuable

## Output

Data keys:
- `services` — list of ServiceInfo
- `dependency_graph` — graph object (or nil with GapNote)
- `shared_patterns` — list of SharedPattern
- `health_indicators` — map of repoID -> RepoHealth
- `issues` — aggregated list of issues across all repos

## Tests

### `internal/recipes/architecture_test.go`

**Test: Single repo returns service inventory**
- Mock manager returns context for 1 repo
- Assert services[0] has name, function count

**Test: Multi-repo returns comparison**
- Mock 3 repos with different frameworks
- Assert services has 3 entries
- Assert shared_patterns detects common and divergent patterns

**Test: With org resolves repo list**
- Mock orgManager returns org with 2 repos
- Assert services has 2 entries

**Test: No dependency data adds GapNote**
- Assert GapNote for dependency_graph

**Test: Health indicators computed**
- Mock repo with 5 test files, 50 total files, 100 functions
- Assert health has test_files=5, test_ratio=0.1

**Test: AI recommendations generated**
- Mock CompleteRaw
- Assert Analysis non-empty

**Test: Focus areas filter output**
- focus_areas=["testing"]
- Assert health emphasized in output

**Test: Input validation requires org_id or repo_ids**
- Provide neither
- Assert error

**Test: Failed repo doesn't break entire recipe**
- Mock GetContext fails for 1 of 3 repos
- Assert 2 services returned + GapNote for failed repo

## File Inventory

| File | Purpose |
|------|---------|
| `internal/recipes/architecture.go` | review_architecture recipe implementation |
| `internal/recipes/architecture_test.go` | All architecture review tests |

## Acceptance Criteria

1. Service inventory extracted from repo contexts
2. Pattern analysis detects shared and divergent frameworks
3. Health indicators computed per repo
4. Dependency graph has proper GapNote for v1
5. AI recommendations generated when available
6. Failed repos produce GapNote, don't break recipe
7. All 9 tests pass
