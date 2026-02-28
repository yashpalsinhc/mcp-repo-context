# Spec: Plugin Interface

## Problem
The MCP server has a single hardcoded Go analyzer and a single hardcoded LocalEmbedder. There is no way to add language-specific analyzers (e.g., TypeScript, Python) or swap embedding models without modifying core code. Need a plugin architecture that allows registration of custom analyzers and embedders.

## Requirements

1. **AnalyzerPlugin interface** — Extends existing `Analyzer` interface. Add `Name() string` method. Built-in Go analyzer and generic analyzer registered as plugins.
2. **EmbedderPlugin interface** — Extends existing `Embedder` interface. Add `Name() string` method. Built-in LocalEmbedder registered as plugin.
3. **Analyzer Registry** — Closed constructor pattern: `NewRegistry(analyzers ...Analyzer)`. Replaces hardcoded registry. Keeps fallback to generic analyzer.
4. **Embedder Registry** — New registry for embedders. Same closed constructor pattern. Single shared embedder instance per server.
5. **OrgConfig extension** — Add `AnalyzerName` and `EmbedderName` fields to OrgConfig for per-org plugin selection.
6. **Built-in defaults** — Go analyzer and LocalEmbedder registered as default plugins. Work without any configuration.
7. **Manager/Server wiring** — Update orchestrator.NewManager and mcp.NewServer to accept registries instead of creating them internally.

## Design Decisions
- Closed constructor (no runtime registration) — simpler, no concurrent mutation
- Single shared embedder per server — vocabulary isolation at vector store level
- Extend existing interfaces rather than creating new ones
- No filesystem plugin discovery (out of scope)
- No plugin versioning (out of scope)

## Dependencies
- 01-org-abstraction (OrgConfig)
- internal/analyzer (existing interface)
- internal/vectors (existing Embedder interface)
