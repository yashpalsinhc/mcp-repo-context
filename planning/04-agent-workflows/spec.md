# Split 04: Agent Workflows

## Purpose

Add high-impact tools that help agents build multi-repo features, refactor across org, and plan repo merges. Reduce fragmented tool calls.

## Context

- **Requirements:** `/mcp-repo-context/requirements.md`
- **Design:** `/mcp-repo-context/docs/DESIGN_ORG_SEMANTIC_SEARCH.md`
- **Current:** Agents must chain many tools (search, get_context, compare, ask)

## Scope

### In Scope

1. **build_feature** / **plan_feature** — "Build feature X across repos"
2. **refactor_org** / **plan_refactor** — "Refactor pattern Y across org"
3. **merge_repos** — Extend `compare_repos` with merge strategy

### Out of Scope

- Plugin system (Split 05)

## Technical Details

### build_feature

**Input:** org_id, feature_description, target_repos (optional)

**Flow:**
1. Semantic search for relevant code
2. Identify entry points, dependencies
3. Return: plan + files to touch + suggested order

**Output:** Structured plan (JSON/markdown)

### refactor_org

**Input:** org_id, pattern_to_refactor, repos (optional)

**Flow:**
1. Semantic search for pattern usages
2. Call graph / impact analysis (use existing)
3. Return: plan + affected files + risk level

### merge_repos

**Input:** source_repo_ids, target_repo_id

**Flow:**
1. Call existing `find_duplicates`, `find_conflicts`, `find_gaps`
2. Generate merge strategy (order, conflict resolution)
3. Return: plan + duplicates + conflicts + gaps

## Dependencies

- Split 01, 02, 03
- `internal/comparison`
- `internal/orchestrator`
- `internal/vectors`
- `internal/graph` (call graph)

## Verification

- [ ] `build_feature` returns actionable plan
- [ ] `refactor_org` identifies affected code
- [ ] `merge_repos` extends compare_repos with strategy
- [ ] All tools reduce agent tool calls for common workflows
