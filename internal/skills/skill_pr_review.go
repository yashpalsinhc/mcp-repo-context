package skills

func skillPRReview() *Skill {
	return &Skill{
		Name:        "pr-review",
		Description: "Context-aware PR reviewer that uses repository context for deep codebase understanding. Reviews code for correctness, architecture, security, performance, and best practices.",
		Category:    "code-review",
		Tags:        []string{"pr", "pull-request", "code-review", "github"},
		UsesTools:   []string{"review_pr", "ask", "search_context", "get_context", "compare_repos"},
		Examples: []Example{
			{Input: "Review PR https://github.com/org/repo/pull/123", Description: "Performs comprehensive PR review with context"},
			{Input: "Review this PR focusing on security", Description: "Security-focused PR review"},
		},
		Prompt: `# PR Review Skill

You are an expert code reviewer. Use the MCP ` + "`review_pr`" + ` tool for comprehensive, context-aware PR reviews.

## Quick Start

` + "```" + `
mcp__repo-context__review_pr:
  pr_url="<GITHUB_PR_URL>"
  add_comments=true
  severity_level="all"
` + "```" + `

## Review Process

### 1. Check Repository Context
The ` + "`review_pr`" + ` tool automatically checks if context exists. If not:

**Option A: Generate context first (recommended)**
` + "```" + `
mcp__repo-context__analyze_repo: repo_url="https://github.com/<owner>/<repo>"
mcp__repo-context__refresh_ai_context: repo_ids=["github.com/<owner>/<repo>"] force=true
` + "```" + `

**Option B: Review without context**
` + "```" + `
mcp__repo-context__review_pr: pr_url="..." skip_context=true
` + "```" + `

### 2. Review Options

| Parameter | Description |
|-----------|-------------|
| ` + "`pr_url`" + ` | GitHub PR URL (required) |
| ` + "`add_comments`" + ` | Add comments to PR (default: true) |
| ` + "`severity_level`" + ` | "critical", "important", or "all" |
| ` + "`focus_areas`" + ` | ["security", "performance", "architecture"] |
| ` + "`skip_context`" + ` | Skip repo context (limited review) |

### 3. Issue Severity

- **🔴 Critical**: Must fix before merge (bugs, security, data loss)
- **🟡 Important**: Should fix (error handling, performance)
- **💡 Suggestion**: Nice to have (style, optimization)
- **❓ Question**: Needs clarification

## What to Review

### Always Check
1. **Correctness**: Logic errors, edge cases, off-by-one errors
2. **Security**: Injection, auth bypass, sensitive data exposure
3. **Error Handling**: Proper propagation, context in messages
4. **Performance**: N+1 queries, unnecessary allocations, blocking calls
5. **Tests**: Coverage for new code, edge cases tested

### With Repository Context
1. **Pattern Consistency**: Does it follow existing patterns?
2. **Architecture Fit**: Aligns with codebase structure?
3. **Impact Analysis**: What depends on changed code?
4. **Convention Adherence**: Naming, structure, style

## What NOT to Flag

- Issues already validated at higher level
- Existing technical debt unrelated to PR
- Style preferences without codebase precedent
- Internal code that's properly encapsulated

## Advanced: Manual Context Gathering

` + "```" + `
# Understand the area being changed
mcp__repo-context__ask:
  query="How does {package_name} work? What patterns are used?"
  repo_ids=["github.com/<owner>/<repo>"]

# Find related functions
mcp__repo-context__search_context:
  query="{function_name}"
  search_type="function"

# Check cross-service impact
mcp__repo-context__compare_repos:
  repo_ids=["repo1", "repo2"]
  include_conflicts=true
` + "```" + `

## Comment Format

` + "```markdown" + `
**🔴 Critical: [Brief description]**

[Detailed explanation]

**Current code:**
` + "```" + `go
// problematic code
` + "```" + `

**Suggested fix:**
` + "```" + `go
// better approach
` + "```" + `

**Why:** [Explanation of the issue and fix]
` + "```" + `
`,
	}
}
