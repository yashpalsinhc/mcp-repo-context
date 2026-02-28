# Opus Review: 04-agent-workflows

## Issues Found

1. **Plan vs Spec Misalignment** - Plan tools differ from 06-agent-recipes spec
2. **Cross-Repo Call Graph Doesn't Exist** - Plan relies on cross-repo dependency detection that isn't implemented
3. **Semantic Search Performance** - SearchByOrg loads all vectors into memory, no repo-level SQL filter
4. **Token Budget Section Allocation** - No strategy for splitting budget across sections of output
5. **Circular Dependencies in Merge Order** - Topological sort with no cycle handling
6. **FindConflicts Name-Only Matching** - Matches bare function name, false positives possible
7. **AI Enhancement: No Timeout/Token Limit** - No deadline on ctx, no prompt size limit
8. **Missing Type Definitions** - Dependency, ImpactSummary, MergeStep, DuplicateSummary, ConflictSummary, GapSummary undefined
9. **No Concurrency Model** - Sequential only, slow for large orgs
10. **Test Coverage Gaps** - Missing tests for partial indexing, empty org, large results, target in sources
11. **SearchByConceptOrg Doesn't Exist** - Referenced but not in codebase
12. **No Cancellation/Progress Reporting** - Long workflows with no progress tokens
13. **Missing Input Validation** - Empty feature_description, no min source repos, no max budget
