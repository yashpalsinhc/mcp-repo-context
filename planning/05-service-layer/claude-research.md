# Research: Service Layer & REST API

## Key Findings

### Current MCP Server
- JSON-RPC 2.0 over stdio in `internal/mcp/server.go`
- Switch statement dispatches to ~30 tool handlers
- Each handler: extract args -> call manager -> format markdown -> return callToolResult
- Server struct holds: manager, comparer, skills, config, semanticSearch, patternRegistry

### Main Entry Point
- `cmd/mcp-server/main.go`: parses flags, creates storage/cloner/scanner/manager/comparer/org, runs ServeStdio
- Config from environment: MCP_STORAGE_PATH, MCP_TEMP_DIR, MCP_VECTOR_STORE_PATH
- Dependencies created in order: store -> cloner -> scanner -> manager -> comparer -> orgManager -> server

### Storage
- Two implementations: FilesystemStore (JSON files) and SQLiteStore (with FTS, search capabilities)
- SQLite has: repos, files, functions, function_calls, types, constants, side_effects, concepts, file_hashes tables
- Org tables: orgs, org_repos (junction with config_override_json)
- All operations scoped by repoID — natural multi-tenant boundary

### Manager Interface (~55 methods)
- AnalyzeRepo, AnalyzeLocal, GetContext, GetFileContext, GetFunctionContext
- SearchFunctions, SearchByConcept, SearchBySideEffect, GetCallers
- ListRepos, SmartQuery, RefreshFile, RefreshChangedFiles
- GenerateAISummary, GenerateAIArchAnalysis, Ask, ReviewPR
- GetPRContext, DeleteRepoContext

### No Existing HTTP Code
- No HTTP server or router library in go.mod
- Only dependencies: go-git, go-sqlite3
- Would need to add HTTP framework

### Docker Setup
- Multi-stage Dockerfile: golang:1.24.0-alpine -> alpine:3.19
- Non-root user (mcp), data volume at /home/mcp/data
- docker-compose with stdin_open for MCP protocol
- Would need HTTP port exposure for REST API

### Org Manager
- Full CRUD: Register, List, Get, AddRepos, RemoveRepos, Delete
- Config inheritance: GetEffectiveConfig, SetRepoConfigOverride
- Concurrent analysis: AnalyzeOrg with bounded concurrency

### Webhook/HTTP
- No existing webhook handling
- No rate limiting
- No auth middleware
