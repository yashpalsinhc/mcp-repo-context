<!-- SECTION_MANIFEST
section-01-shared-types
section-02-build-feature
section-03-refactor-org
section-04-merge-repos
section-05-ai-enhance
section-06-integration-tests
END_MANIFEST -->

# Section Index: Agent Workflows

## Batch 1 (no dependencies)
- section-01-shared-types: Workflow types (RiskLevel, FeaturePlan, RefactorPlan, MergeReport, etc.) and formatting utilities with token budgeting

## Batch 2 (depends on batch 1)
- section-02-build-feature: build_feature tool — semantic search, entry points, name-based dependencies, implementation order, risk assessment
- section-03-refactor-org: refactor_org tool — pattern usages, concept+semantic search merge, impact analysis, risk assessment
- section-04-merge-repos: merge_repos tool — comparison analyses, merge order with cycle detection, advisory report

## Batch 3 (depends on batch 2)
- section-05-ai-enhance: Shared AI enhancement layer — EnhanceWithAI, timeout, prompt truncation, availability check

## Batch 4 (depends on batch 3)
- section-06-integration-tests: End-to-end tests for all three workflow tools
