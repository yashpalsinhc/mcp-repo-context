# Integration Notes: 04-agent-workflows

## Integrated

1. **Cross-repo call graph limitation** (#2) - Acknowledge this is best-effort name-based matching only. No cross-repo interface resolution. Add explicit note in plan.

2. **Missing type definitions** (#8) - Define all missing types: Dependency, ImpactSummary, MergeStep, DuplicateSummary, ConflictSummary, GapSummary. Use typed RiskLevel constant.

3. **AI timeout and prompt size limit** (#7) - Add 30s timeout via context.WithTimeout. Cap prompt size by truncating lists to top 10 items.

4. **Circular dependency handling** (#5) - Fall back to alphabetical order when cycle detected, emit warning in report.

5. **SearchByConceptOrg clarification** (#11) - Clarify this means iterating SearchByConcept per repo and merging. Not a new method.

6. **Input validation** (#13) - Add: feature_description min length 3, source_repo_ids min 1, token_budget max 32000, org repos must be analyzed.

7. **Token budget allocation** (#4) - Define section weights: 40% relevant code, 30% dependencies/impact, 20% files/order, 10% metadata/risk.

8. **Test coverage additions** (#10) - Add tests for: empty org, partial indexing, target in sources, large result truncation.

9. **Sequential v1 note** (#9) - Explicitly state sequential for v1, note future concurrency optimization.

10. **FindConflicts dependency** (#6) - Note dependency on 01-core-bug-fixes for receiver-qualified matching.

## Not Integrated

1. **Plan vs spec misalignment** (#1) - This plan IS the 04-agent-workflows spec. The 06-agent-recipes is a separate future spec with different tools. No conflict.

2. **Semantic search performance** (#3) - This is a known issue tracked in 04-semantic-search. Not in scope for this plan to fix SearchByOrg internals.

3. **Progress reporting** (#12) - MCP progress tokens are a nice-to-have but add complexity. Out of scope for v1.
