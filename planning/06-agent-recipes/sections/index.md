<!-- SECTION_MANIFEST
section-01-recipe-framework
section-02-context-assembly
section-03-pr-impact
section-04-api-flow
section-05-architecture-review
section-06-mcp-integration
section-07-integration-tests
END_MANIFEST -->

# Section Index: Agent Recipes & Pre-Built Workflows

## Batch 1 (no dependencies)
- section-01-recipe-framework: Recipe interface, RecipeRunner, Registry, types, AI interface update, VectorSearcher interface
- section-02-context-assembly: Hybrid ContextAssembler combining structural, vector, keyword, metadata passes

## Batch 2 (depends on batch 1)
- section-03-pr-impact: analyze_pr_impact recipe with cross-repo and AI risk assessment
- section-04-api-flow: explain_api_flow recipe with Mermaid visualization
- section-05-architecture-review: review_architecture recipe with health indicators

## Batch 3 (depends on batch 2)
- section-06-mcp-integration: MCP tool handlers, execute_recipe, list_recipes, pattern deprecation

## Batch 4 (depends on batch 3)
- section-07-integration-tests: End-to-end tests, composability, token benchmarks
