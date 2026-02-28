<!-- SECTION_MANIFEST
section-01-analyzer-interface
section-02-embedder-registry
section-03-org-config
section-04-manager-wiring
section-05-per-org-selection
section-06-integration-tests
END_MANIFEST -->

# Section Index: Plugin Interface

## Batch 1 (no dependencies)
- section-01-analyzer-interface: Add Name() to Analyzer, NewRegistry/DefaultRegistry constructors, public Go/Generic constructors
- section-02-embedder-registry: Add Name() to Embedder, EmbedderRegistry interface, dimension validation, DefaultEmbedderRegistry
- section-03-org-config: Add AnalyzerName/EmbedderName to OrgConfig, update MergeConfigs and copyConfig

## Batch 2 (depends on batch 1)
- section-04-manager-wiring: Functional options for Manager, remove NewManagerWithAI, ServerConfig with EmbedderRegistry, main.go updates

## Batch 3 (depends on batch 2)
- section-05-per-org-selection: MCP server resolves org config, passes to AnalyzeOptions, warning surfacing for unknown plugins

## Batch 4 (depends on batch 3)
- section-06-integration-tests: End-to-end tests for custom analyzers, embedder registry, org config round-trip, backward compatibility
