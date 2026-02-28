# Research: Agent Recipes & Pre-Built Workflows

## Existing execute_pattern System

**Location:** `internal/compose/`

**Components:**
- `patterns.go` — Pattern definitions and registry
- `chain.go` — Chain execution with steps
- `middleware.go` — Middleware system
- `builder.go` — Fluent builder API

**Interface:**
```go
type Pattern interface {
    Name() string
    Description() string
    Build(executor ToolExecutor, params map[string]any) *Chain
}

type ToolExecutor interface {
    Execute(ctx *ChainContext, call ToolCall) ToolResult
}
```

**4 Existing Patterns:**
1. SearchWithContext — search functions + get context for first result
2. ImpactAnalysis — get function context + callers + related functions
3. FindAndExpand — generic search + expand (configurable)
4. MultiRepoSearch — search across multiple repos

**Known Issues:** MultiRepoSearch has closure over loop variable `i` — potential bug. The spec notes the system "silently skips steps."

**Chain Model:** Steps can be conditional, have transforms, use `{{varName}}` parameter resolution. Stops on error.

## PR Context System

**Location:** `internal/orchestrator/pr_context.go`

**Returns:**
- Changed functions with behavior summaries
- Direct callers (who calls this)
- Callees (what this calls)
- DB queries with SQL
- HTTP calls to external services
- Side effects
- Impact on downstream functions not in PR
- Affected API routes

**Key types:** PRContextResult, FileChangeContext, FunctionChangeContext, ImpactAnalysis

**Limitation:** Single-repo only. No cross-service or cross-dependency impact.

## AI System

**Location:** `internal/ai/`

**Provider interface:** GenerateSummary, AnalyzeArchitecture, GenerateDescription, CompleteRaw

**Ask tool flow:**
1. Get AI provider (ANTHROPIC_API_KEY required)
2. Extract context via ContextExtractor (keyword matching, query classification)
3. Build prompt with context
4. Call Claude API
5. Parse response (answer + sources)

**Retry:** Exponential backoff, handles rate limits and timeouts

## Token Budget System

**Location:** `internal/tokens/`

**Budgeter:** Scores functions by keyword relevance, selects greedily within budget.

**Scoring:** Count keyword matches in name/description/signature/behavior, boost name matches. Sort descending, fill budget.

**Defaults:** 4000 tokens, max 32000. ~4 chars/token ratio.

## Dependency Graph (Planned — 02-dependency-graph)

**Planned data:** ModuleDependency (path, version, direct/indirect), ImportGraph (per-package imports classified as stdlib/internal/external), ArchitectureDependencies.

**Planned tools:** get_dependency_graph, enhanced compare_repos with dep info.

## API Flow Tracing (Planned — 03-api-flow-tracing)

**Planned data:** HTTPClientCall, gRPCClientCall, AsyncProducer, AsyncConsumer, HTTPRoute, ServiceFlow, ServiceMap.

**Planned detections:** HTTP client calls, gRPC calls, Kafka/RabbitMQ/NATS producers/consumers, HTTP route handlers.

**Planned tools:** trace_api_flow (with Mermaid), get_service_map.

## Semantic Search

**Location:** `internal/vectors/`

**Embedder:** LocalEmbedder using TF-IDF (offline, no API). 256 dimensions default.

**VectorStore:** SQLite-backed with JSON vectors. Store, Search, SearchByType, DeleteByRepo.

**SemanticSearch service:** IndexRepository, SearchFunctions, SearchTypes, SearchAll, SearchByOrg.

## Test Patterns

- Chain tests use `MapExecutor` with registered mock handlers
- Vector store tests use temp SQLite DB
- Token counter tests verify ratio configurations
- Build tag `integration` separates slow tests
- Test helpers: `httptest.NewRecorder`, temp directories, cleanup functions

## Current Tool Count: 30 MCP tools implemented
