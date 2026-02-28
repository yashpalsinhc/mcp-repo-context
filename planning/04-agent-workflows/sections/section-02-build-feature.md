# Section 2: build_feature Tool

## Overview

Implement the `build_feature` MCP tool that plans how to build a feature across repositories in an organization. Orchestrates semantic search, entry point identification, dependency detection, and risk assessment.

## Dependencies

- Section 1 (shared types and formatting)

## Tests First

### File: `internal/workflows/build_feature_test.go`

```
Test: BuildFeature returns relevant code from semantic search
- Setup: org with 2 repos indexed with known functions (GetUser, CreateOrder)
- Call BuildFeature("user management")
- Assert RelevantCode contains GetUser
- Assert sorted by relevance score descending

Test: BuildFeature identifies entry points (public handlers)
- Setup: repo with GetUser (public, exported), validateUser (private)
- Call BuildFeature("user")
- Assert GetUser in EntryPoints with Why containing "public"
- Assert validateUser NOT in EntryPoints

Test: BuildFeature identifies name-based dependencies
- Setup: repo-A with funcX calling funcY, repo-B also has funcY
- Call BuildFeature
- Assert Dependencies contains {SourceRepo: A, TargetRepo: B, TargetFunc: funcY, Type: "name_match", Confidence: "medium"}

Test: BuildFeature suggests order shared-libs first
- Setup: org with "shared-lib" and "service" repos
- Relevant functions in both
- Assert SuggestedOrder has shared-lib before service

Test: BuildFeature risk high when >5 repos or >20 functions
- Setup: 6 repos with relevant functions
- Assert RiskLevel == RiskHigh

Test: BuildFeature risk low when 1 repo <5 functions
- Setup: 1 repo, 3 relevant functions
- Assert RiskLevel == RiskLow

Test: BuildFeature target_repos filters results
- Setup: org with repos A, B, C all having relevant code
- Call with target_repos=["A"]
- Assert all RelevantCode, EntryPoints, Dependencies only reference repo A

Test: BuildFeature validates feature_description min length
- Call with feature_description=""
- Assert error with "feature_description must be at least 3 characters"

Test: BuildFeature validates token_budget range
- Call with token_budget=50
- Assert error or capped to 500

Test: BuildFeature keyword fallback without vectors
- Setup: repos analyzed but NOT indexed in vector store
- Call BuildFeature
- Assert RelevantCode populated (keyword results)
- Assert formatted output contains "semantic search unavailable"

Test: BuildFeature invalid org returns error
- Call with org_id="nonexistent"
- Assert error containing org ID

Test: BuildFeature repos not analyzed returns error
- Register org with repo IDs but don't analyze them
- Call BuildFeature
- Assert error suggesting analyze_org
```

## Implementation Details

### 1. Workflow Function

Create `internal/workflows/build_feature.go`.

**Primary function:**

```
func BuildFeature(ctx context.Context, params BuildFeatureParams) (*FeaturePlan, error)
```

**BuildFeatureParams struct:**
- OrgID (string, required)
- FeatureDescription (string, required, min 3 chars)
- TargetRepos ([]string, optional)
- TokenBudget (int, default 4000, range 500-32000)
- Manager — reference to orchestrator.Manager for repo access, search, call graph
- SemanticSearch — reference to vectors.SemanticSearch for vector search

### 2. Input Validation

- FeatureDescription length >= 3, else return descriptive error
- TokenBudget: if < 500 set to 500, if > 32000 set to 32000
- OrgID: load org via Manager, error if not found
- Get repos from org, error if empty with "no repos; run analyze_org"
- If TargetRepos provided, validate each is in the org's repo list

### 3. Semantic Search Step

Call SemanticSearch.SearchByOrg(ctx, orgID, featureDescription, limit=20). Post-filter by TargetRepos if specified.

**Fallback:** If SearchByOrg returns error (no vectors), iterate repos and use Manager.SmartQuery per repo with the feature description as query. Collect results, convert to []CodeLocation, set a flag `semanticFallback=true` to include warning in output.

Cap results at 20 items, sorted by score.

### 4. Entry Point Identification

For each CodeLocation in top results:
- Check if function is exported (first letter uppercase in Go)
- Check if function has concept tag "handler" or "endpoint" (via SearchByConcept if available)
- Check caller count via Manager.GetCallers: if 0-2 callers, likely an entry point
- Entry point = exported + (handler concept OR few callers)

### 5. Dependency Detection (Name-Based)

For top 10 relevant functions:
- Get per-repo call graph via Manager (callers and callees)
- Collect all callee function names
- For each callee name, check if it exists in another repo's context (by searching function names across loaded contexts)
- If match found: create Dependency with Type="name_match", Confidence="medium"
- Deduplicate dependencies by (sourceRepo, targetRepo, targetFunc)

### 6. Files to Touch

- All files containing relevant functions -> FileAction{Action: "modify"}
- Files containing entry points -> mark as primary (listed first)
- Deduplicate by (repoID, filePath), keep highest-priority action

### 7. Implementation Order

- Count how many dependencies point TO each repo (in-degree)
- Repos with highest in-degree = most depended upon = implement first
- Sort: lowest in-degree first (they depend on others), then alphabetical for ties
- Result: []string of repo IDs in suggested order

### 8. Risk Assessment

- repoCount = number of distinct repos with relevant code
- funcCount = number of relevant functions
- hasPublicAPI = any entry point that looks like an HTTP handler
- If repoCount > 5 OR funcCount > 20 OR hasPublicAPI with many callers -> RiskHigh
- If repoCount 2-5 OR funcCount 5-20 -> RiskMedium
- Else -> RiskLow

### 9. MCP Tool Handler

Register tool in server's tool list:
- Name: "build_feature"
- Description: "Plan how to build a feature across repositories in an organization"
- Parameters: org_id (required), feature_description (required), target_repos (optional array), token_budget (optional int)

Handler extracts params, calls BuildFeature, then FormatFeaturePlan with the result and budget.

## Error Handling

- Org not found: return "Organization '{org_id}' not found"
- No repos: return "No repositories in org. Run analyze_org first."
- Repos not analyzed: return "Repositories not analyzed: {list}. Run analyze_org."
- Semantic search failure: fall back to keyword, include warning
- Call graph unavailable for a repo: skip that repo's dependency analysis, continue

## File Summary

| File | Action |
|------|--------|
| `internal/workflows/build_feature.go` | New: BuildFeature function and params |
| `internal/workflows/build_feature_test.go` | New: unit tests |
| `cmd/mcp-repo-context/tools.go` (or equivalent) | Modify: register build_feature tool |
