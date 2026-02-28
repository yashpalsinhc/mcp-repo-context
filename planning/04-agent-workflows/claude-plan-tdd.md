# TDD Plan: Agent Workflows

## Section 1: Workflow Types & Shared Infrastructure

### Tests: `internal/workflows/types_test.go`

```
Test: RiskLevel constants are valid strings
- Assert RiskLow == "low", RiskMedium == "medium", RiskHigh == "high"

Test: FormatFeaturePlan respects token budget
- Create FeaturePlan with 50 CodeLocations, 20 dependencies
- Format with budget=2000
- Assert output length <= 2000 * 4 characters (approx)
- Assert output contains section headers

Test: FormatFeaturePlan budget allocation
- Create FeaturePlan with data in all sections
- Format with budget=4000
- Assert relevant code section gets ~40% of output
- Assert dependencies section gets ~30%
- Assert files section gets ~20%

Test: FormatRefactorPlan respects token budget
- Create RefactorPlan with 30 usages
- Format with budget=1000
- Assert truncated with "... and N more" indicator

Test: FormatMergeReport respects token budget
- Create MergeReport with many duplicates/conflicts/gaps
- Format with budget=8000
- Assert all sections present, truncated proportionally

Test: FormatFeaturePlan empty plan
- Create FeaturePlan with no results
- Format produces valid markdown with "no results found" messages

Test: CodeLocation sorting by score
- Create []CodeLocation with various scores
- Sort descending
- Assert highest score first
```

## Section 2: build_feature Tool

### Tests: `internal/workflows/build_feature_test.go`

```
Test: BuildFeature returns relevant code from semantic search
- Setup: org with 2 repos, both indexed with known functions
- Call BuildFeature("user authentication")
- Assert RelevantCode contains auth-related functions
- Assert RelevantCode sorted by relevance score

Test: BuildFeature identifies entry points
- Setup: org with repo containing GetUser (public, handler), validateUser (private)
- Call BuildFeature("user management")
- Assert GetUser is in EntryPoints
- Assert validateUser is NOT in EntryPoints

Test: BuildFeature identifies name-based dependencies
- Setup: org with repo-A having funcX that calls funcY, repo-B also has funcY
- Call BuildFeature
- Assert Dependencies contains entry linking repo-A to repo-B via funcY
- Assert Confidence is "medium"

Test: BuildFeature suggests implementation order
- Setup: org with shared-lib repo and service repo
- Assert shared-lib appears before service in SuggestedOrder

Test: BuildFeature risk assessment high
- Setup: org with 6 repos all having relevant functions, >20 functions total
- Assert RiskLevel == RiskHigh

Test: BuildFeature risk assessment low
- Setup: org with 1 repo, 3 functions
- Assert RiskLevel == RiskLow

Test: BuildFeature with target_repos filter
- Setup: org with repos A, B, C
- Call with target_repos=["A"]
- Assert all RelevantCode entries are from repo A only

Test: BuildFeature validates feature_description min length
- Call with feature_description=""
- Assert error returned with validation message

Test: BuildFeature validates token_budget range
- Call with token_budget=100
- Assert error or capped to minimum 500

Test: BuildFeature falls back to keyword search without vectors
- Setup: org with repos analyzed but NOT indexed
- Call BuildFeature
- Assert results returned (keyword-based)
- Assert output contains warning about semantic search unavailable

Test: BuildFeature with AI enhancement
- Setup: mock AskFunc that returns canned response
- Call BuildFeature with AI available
- Assert AIEnhancement is non-empty

Test: BuildFeature without AI
- Setup: no AI configured
- Call BuildFeature
- Assert AIEnhancement is empty
- Assert rest of plan is populated

Test: BuildFeature invalid org returns error
- Call with org_id="nonexistent"
- Assert descriptive error

Test: BuildFeature token budget limits output
- Create scenario with many results
- Call with token_budget=500
- Assert formatted output respects budget
```

## Section 3: refactor_org Tool

### Tests: `internal/workflows/refactor_org_test.go`

```
Test: RefactorOrg finds pattern usages via semantic search
- Setup: org with repos containing "authentication" related functions
- Call RefactorOrg("authentication pattern")
- Assert Usages contains auth functions from multiple repos

Test: RefactorOrg merges concept and semantic results
- Setup: repo with functions tagged as "authentication" concept AND semantically similar
- Call RefactorOrg("authentication")
- Assert no duplicate entries in Usages

Test: RefactorOrg groups usages by repo
- Setup: org with 3 repos
- Call RefactorOrg
- Assert usages grouped correctly per repo

Test: RefactorOrg impact analysis counts callers
- Setup: repo where funcA calls funcB, funcC calls funcB
- funcB matches pattern
- Assert ImpactAnalysis.DirectCallers >= 2

Test: RefactorOrg identifies hot paths
- Setup: function with >5 callers
- Assert ImpactAnalysis.HotPaths includes that function

Test: RefactorOrg risk high when many repos affected
- Setup: org with 6 repos, pattern in all
- Assert RiskLevel == RiskHigh

Test: RefactorOrg risk low when single repo
- Setup: org with 1 repo, 2 usages
- Assert RiskLevel == RiskLow

Test: RefactorOrg affected files includes callers
- Setup: funcA in file1 matches pattern, funcB in file2 calls funcA
- Assert file1 action="modify", file2 action="review"

Test: RefactorOrg validates pattern_description min length
- Call with pattern_description="ab"
- Assert validation error
```

## Section 4: merge_repos Tool

### Tests: `internal/workflows/merge_repos_test.go`

```
Test: MergeRepos combines all comparison analyses
- Setup: 2 source repos, 1 target with known overlaps
- Call MergeRepos
- Assert Duplicates, Conflicts, Gaps all populated

Test: MergeRepos merge order respects dependencies
- Setup: repo-A depends on repo-B (repo-A imports types from repo-B)
- Assert repo-B appears before repo-A in MergeOrder

Test: MergeRepos merge order types before functions
- Assert MergeStep items within a repo: types first, then utility, then business logic

Test: MergeRepos circular dependency fallback
- Setup: repo-A depends on repo-B, repo-B depends on repo-A
- Assert both appear in MergeOrder (alphabetical)
- Assert report contains circular dependency warning

Test: MergeRepos risk high with severe conflicts
- Setup: source with high-severity conflict against target
- Assert RiskLevel == RiskHigh

Test: MergeRepos risk low with no conflicts
- Setup: source with no overlapping functions
- Assert RiskLevel == RiskLow

Test: MergeRepos target in sources removed
- Call with source_repo_ids=["A", "B"], target_repo_id="A"
- Assert "A" removed from sources
- Assert warning in output

Test: MergeRepos validates min 1 source
- Call with source_repo_ids=[]
- Assert error

Test: MergeRepos advisory report structure
- Call MergeRepos
- Assert formatted output has sections: Duplicates, Conflicts, Gaps, Merge Steps, Risk

Test: MergeRepos token budget limits output
- Setup: many duplicates/conflicts/gaps
- Call with token_budget=2000
- Assert output truncated proportionally

Test: MergeRepos works with single source repo
- Call with 1 source
- Assert valid report generated
```

## Section 5: AI Enhancement Layer

### Tests: `internal/workflows/ai_enhance_test.go`

```
Test: EnhanceWithAI returns answer from AskFunc
- Mock AskFunc returning QueryResult with Answer
- Call EnhanceWithAI
- Assert returns the Answer string

Test: EnhanceWithAI respects 30s timeout
- Mock AskFunc that blocks for 60s
- Call EnhanceWithAI
- Assert returns empty string within ~30s
- Assert no error (graceful degradation)

Test: EnhanceWithAI truncates long prompts
- Create prompt with 10000 characters
- Call EnhanceWithAI
- Assert AskFunc received prompt <= 4000 chars

Test: EnhanceWithAI truncates lists in prompt
- Create prompt with 50 entry points
- Assert AskFunc received prompt with max 10 items + "... and 40 more"

Test: EnhanceWithAI handles AskFunc error gracefully
- Mock AskFunc returning error
- Call EnhanceWithAI
- Assert returns empty string and error

Test: aiAvailable returns false when no AI registry
- Setup server with no AI configured
- Assert aiAvailable() == false

Test: aiAvailable returns true when AI configured
- Setup server with AI registry
- Assert aiAvailable() == true
```

## Section 6: Integration Tests

### Tests: `internal/integration/workflows_test.go`

```
Test: build_feature end-to-end
- Create temp org with 2 Go repos (user_handlers, order_handlers)
- Analyze and index both repos
- Call build_feature tool via MCP
- Assert response contains relevant functions
- Assert markdown formatted output

Test: refactor_org end-to-end
- Create temp org with 2 repos sharing a pattern
- Analyze and index
- Call refactor_org tool
- Assert usages found across repos
- Assert impact analysis populated

Test: merge_repos end-to-end
- Create temp source and target repos with overlapping functions
- Analyze both
- Call merge_repos tool
- Assert duplicates, conflicts, gaps in output
- Assert merge order present

Test: build_feature without semantic index falls back
- Create org, analyze but do NOT index
- Call build_feature
- Assert results returned (keyword-based)
- Assert output contains fallback warning

Test: All workflows respect token budget
- Call each tool with budget=1000
- Assert output size reasonable

Test: All workflows handle unknown org
- Call each tool with nonexistent org
- Assert descriptive error message

Test: All workflows handle empty org
- Register org with no repos
- Call each tool
- Assert error suggesting analyze_org
```
