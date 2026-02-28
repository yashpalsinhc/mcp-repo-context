# Interview: Org-Wide Semantic Index

## Q1: Tool Design - New index_org tool vs extending index_repository?

**Answer:** Both. Create a new `index_org` MCP tool that indexes all repos in an org with a single call, AND add an `org_id` parameter to the existing `index_repository` tool.

## Q2: Incremental update granularity - file level or function level?

**Answer:** Function level. Only re-embed specific changed functions/types. This is more efficient but requires per-function hash tracking.

## Q3: Vocabulary scope for org-wide indexing?

**Answer:** Org-wide vocabulary. Build vocabulary from all repos in the org for consistent embeddings across repos (better cross-repo search quality).

## Q4: Partial failure handling for index_org?

**Answer:** Up to the plan. (Decision: Partial success - index what we can, report failures per repo. This aligns with the existing AnalyzeOrg pattern which already handles partial failures.)
