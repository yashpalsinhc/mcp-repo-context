# Section 6: Integration Tests

## Overview

End-to-end integration tests for all three workflow tools (build_feature, refactor_org, merge_repos). Tests use synthetic Go repos analyzed by the real analyzer, exercising the full pipeline from MCP tool call through to formatted output.

## Dependencies

- All previous sections (1-5)

## Tests First

### File: `internal/integration/workflows_test.go`

```
Test: build_feature end-to-end with indexed repos
- Setup: create temp org with 2 Go repos
  - repo-A: user_handlers.go (GetUser, CreateUser, UpdateUser — public handlers)
  - repo-B: order_handlers.go (ListOrders, CreateOrder) + shared.go (ValidateInput)
- Analyze and index both repos
- Register org with both repos
- Call build_feature tool: org_id=org, feature_description="user management"
- Assert output contains "GetUser", "CreateUser", "UpdateUser"
- Assert output contains "Entry Points" section
- Assert output contains "Risk" section
- Assert output is valid markdown

Test: build_feature keyword fallback (no vectors)
- Setup: same 2 repos, analyzed but NOT indexed
- Call build_feature: feature_description="user"
- Assert output contains results (keyword-based)
- Assert output contains "semantic search unavailable" or similar warning

Test: build_feature target_repos filter
- Setup: org with repos A and B
- Call build_feature with target_repos=["A"]
- Assert output only mentions repo A functions

Test: build_feature empty org error
- Setup: register org with no repos
- Call build_feature
- Assert error output suggesting analyze_org

Test: refactor_org end-to-end
- Setup: create temp org with 2 repos
  - Both repos have a "validateInput" function (same pattern)
  - repo-A: validateInput called by 3 functions
  - repo-B: validateInput called by 2 functions
- Analyze and index
- Call refactor_org: pattern_description="validate input pattern"
- Assert output contains usages from both repos
- Assert output contains "Impact Analysis" with caller counts
- Assert output contains "Risk" section

Test: refactor_org concept search merge
- Setup: repo with functions tagged as "validation" concept
- Call refactor_org: pattern_description="validation"
- Assert concept results merged with semantic results

Test: merge_repos end-to-end
- Setup: create 2 source repos and 1 target repo
  - source-A: HasFunc1, HasFunc2
  - source-B: HasFunc2 (duplicate), HasFunc3
  - target: HasFunc1 (duplicate with source-A), HasFunc4
- Analyze all 3
- Call merge_repos: sources=[source-A, source-B], target=target
- Assert output contains "Duplicates" section (HasFunc1, HasFunc2)
- Assert output contains "Gaps" section (HasFunc3)
- Assert output contains "Merge Steps" section
- Assert output contains "Risk" section

Test: merge_repos target in sources handled
- Call merge_repos: sources=["A", "target"], target="target"
- Assert "target" removed from effective sources
- Assert output contains warning about target in sources

Test: merge_repos circular dependency warning
- Setup: source-A calls function from source-B, source-B calls function from source-A
- Call merge_repos
- Assert output contains "circular dependency" warning

Test: All tools respect token budget
- Call build_feature with token_budget=1000
- Assert output length <= 1000 * 5 characters (generous bound)
- Call refactor_org with token_budget=1000
- Assert similar bound
- Call merge_repos with token_budget=2000
- Assert similar bound

Test: All tools handle unknown org
- Call build_feature with org_id="nonexistent"
- Assert error mentions org_id
- Call refactor_org with org_id="nonexistent"
- Assert error
- (merge_repos doesn't use org, uses repo IDs directly)

Test: All tools validate short descriptions
- Call build_feature with feature_description="ab"
- Assert validation error
- Call refactor_org with pattern_description=""
- Assert validation error
```

## Implementation Details

### 1. Test Fixture Setup

Create `internal/integration/workflow_fixtures_test.go`.

**setupWorkflowOrg(t *testing.T) (server, orgID, []repoID, tempDir)**

Creates:
- Temp directory with 2-3 Go repo directories
- Each repo has go.mod and .go files with known functions
- Calls analyze_local for each repo
- Optionally indexes repos for vector search
- Registers an org with all repos
- Returns server instance and IDs

### 2. Go Source Fixtures

**Repo A — user_handlers.go:**
```go
package handlers

func GetUser(id int) (*User, error) { /* retrieves user by ID */ }
func CreateUser(name string) (*User, error) { /* creates new user */ }
func UpdateUser(id int, name string) error { /* updates user profile */ }
func validateUserInput(name string) error { /* validates user input */ }
```

**Repo B — order_handlers.go + shared.go:**
```go
// order_handlers.go
package handlers
func ListOrders(userID int) ([]Order, error) { /* lists orders */ }
func CreateOrder(items []Item) (*Order, error) { /* creates order */ }

// shared.go
package handlers
func ValidateInput(data interface{}) error { /* validates any input */ }
```

**Repo C (for merge tests) — target repo with partial overlap.**

### 3. Tool Invocation

Tests call tool handlers directly (not via MCP protocol). Pass extracted parameters to the handler function and check the returned markdown string.

Alternatively, if the server has a `callTool(name, params)` method, use that for more realistic testing.

### 4. Assertion Helpers

```go
func assertOutputContains(t *testing.T, output, substring string) {
    t.Helper()
    if !strings.Contains(output, substring) {
        t.Errorf("output missing %q\nGot: %s", substring, output[:min(500, len(output))])
    }
}

func assertOutputNotContains(t *testing.T, output, substring string)
func assertValidMarkdown(t *testing.T, output string) // check for unclosed headers, etc.
func assertOutputWithinBudget(t *testing.T, output string, budget int)
```

### 5. Test Isolation

- Each test uses its own temp directory and org
- Tests use `t.TempDir()` for automatic cleanup
- Tests can run in parallel (separate orgs and repos)
- Vector store uses separate SQLite files per test

## Error Handling

- Fixture setup failure: `t.Fatal` with descriptive message
- Analysis failure on synthetic files: indicates analyzer bug — fail test
- Index failure: skip vector-dependent assertions, verify keyword fallback

## File Summary

| File | Action |
|------|--------|
| `internal/integration/workflows_test.go` | New: end-to-end tests |
| `internal/integration/workflow_fixtures_test.go` | New: fixture setup and helpers |
