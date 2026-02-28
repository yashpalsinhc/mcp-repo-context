# Interview: Agent Workflows

## Q1: AI usage for plan generation?
**Answer:** Hybrid
**Decision:** Algorithmic analysis first (code search, duplicates, call graph, gaps). Optional AI enhancement via Ask() when available. If AI unavailable, return algorithmic results only.

## Q2: merge_repos output format?
**Answer:** Advisory report
**Decision:** Return structured analysis (duplicates to consolidate, conflicts to resolve, gaps to fill, suggested merge order). Agent/user decides what to act on. Not prescriptive file operations.
