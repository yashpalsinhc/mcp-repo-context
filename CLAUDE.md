# Claude Code Instructions for This Project

## MCP Server Usage

This project has an MCP server (`repo-context`) with pre-analyzed repository context.

**IMPORTANT: When answering questions about repositories:**

1. **USE** the `repo-context` MCP tools:
   - `ask` - For natural language questions about code
   - `search_context` - For finding specific functions/types
   - `get_context` - For getting file/architecture details
   - `compare_repos` - For comparing repositories

2. **DO NOT** use Explore agents or read files directly when the MCP tools can answer the question.

3. The MCP server already has analyzed context for these repos:
   - mobile-management-service
   - mobile-manual-test-management
   - lambda-test-forge-service
   - lambda-test-management-service
   - lambda-app-upload

## Why This Matters

- MCP `ask` tool: ~5-10k tokens (uses pre-analyzed context)
- Explore agents: ~50k tokens (reads files from disk)

## Example Queries

Instead of exploring, use:
```
repo-context ask: How does authentication work?
repo-context search_context: query="validateApp" search_type="function"
repo-context compare_repos: repo_ids=["mobile-management-service", "lambda-test-forge-service"]
repo-context refresh_ai_context: repo_ids=["mobile-management-service"] force=true
```

## Updating/Refreshing Context

When you need to update or regenerate AI context for repositories:
- Use `repo-context refresh_ai_context` with `force=true` to regenerate AI summaries
- This uses the Anthropic API configured in the MCP server
- Do NOT use Explore agents for re-analysis
