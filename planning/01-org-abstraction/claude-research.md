# Research Findings: 01-org-abstraction

## Part 1: Codebase Analysis

### Project Structure

The project is organized under `internal/` with well-separated packages:

```
internal/
├── ai/           # AI provider integrations (Anthropic registry)
├── analyzer/     # Code analysis engine (Go/Generic analyzers)
├── analytics/    # Usage tracking and telemetry
├── comparison/   # Multi-repo comparison logic
├── compose/      # Pattern-based tool composition
├── config/       # Configuration management
├── context/      # Core context type definitions
├── database/     # Legacy database utilities
├── graph/        # Call graph visualization (Mermaid/DOT)
├── logging/      # Structured logging
├── mcp/          # MCP protocol server implementation
├── orchestrator/ # Repository analysis orchestration
├── org/          # Organization management (ALREADY EXISTS)
├── prreview/     # PR review functionality
├── repo/         # Repository cloning and scanning
├── skills/       # Skills registry for extensibility
├── storage/      # SQLite and Filesystem storage backends
├── tokens/       # Token counting utilities
└── vectors/      # Semantic search with embeddings
```

### KEY FINDING: Organization Package Already Partially Exists

The `internal/org/` package is already implemented:

**`types.go`** defines:
- `Org` struct: ID, Repos ([]string), Config, Created
- `OrgConfig`: ExcludePatterns, MaxFileSize
- `OrgWithCount`: For listing with repo counts

**`manager.go`** provides Manager interface:
- Register, List, Get, AddRepos, RemoveRepos, Delete

**`store.go`** provides:
- `FilesystemStore` using JSON (`_orgs.json`)
- Thread-safe with RWMutex
- In-memory map-based storage

### Existing Patterns and Conventions

#### Manager Pattern (Used Throughout)
- Public interface defining operations
- Private struct implementing the interface
- `NewManager()` constructor returning interface type
- Examples: `orchestrator.Manager`, `comparison.Comparer`, `org.Manager`

#### Storage Layer
Two-tier approach:
- **ContextStore interface** (abstract contract): StoreRepoContext, GetRepoContext, DeleteContext
- **SearchableStore interface**: Extends for SQLite-specific operations
- **Implementations**: FilesystemStore (JSON), SQLiteStore (relational)

#### Error Handling
- Sentinel errors: `ErrNotFound`, `ErrStoreFailed`
- Custom error types: `ConfigError` with Message
- Error wrapping: `fmt.Errorf("failed to X: %w", err)`

#### Naming Conventions
- Interfaces: PascalCase, often "-er"/"-r" (Manager, Store, Scanner)
- Private implementations: lowercase
- JSON tags: `json:"field_name,omitempty"`

### SQLite Schema (Existing)

**`001_initial_schema.sql`** tables:
- `repos`: id, url, branch, commit_hash, analyzed_at, stats_json, ai_summary_json
- `files`: repo_id (FK), path, hash, language, package, concepts_json
- `functions`: name, name_lower, signature, behavior_json, error_handling_json
- `types`: name, kind, fields_json, methods_json
- `constants`: constant definitions
- **Lookup tables**: side_effects, concepts, file_concepts, function_calls

**Migration system**: `schema_migrations` table, `002_file_hashes.sql` for incremental indexing

**SQLiteStore patterns**:
- Transaction boundaries with explicit BEGIN/COMMIT
- Cascading deletes via FK constraints
- JSON serialization for complex nested data
- Connection pooling: MaxOpenConns(10), MaxIdleConns(5)
- WAL mode: `_journal_mode=WAL`

### MCP Tool Registration Pattern

Tools dispatched via switch in `handleCallToolWithID()`:
```go
switch params.Name {
case "analyze_repo":
    result = s.toolAnalyzeRepo(ctx, args)
case "get_context":
    result = s.toolGetContext(ctx, args)
}
```

Tool definitions: `toolDefinition` struct with Name, Description, InputSchema
Response format: `callToolResult` with `[]contentItem`
40+ tools already registered including `list_orgs`, `register_org`

### Vectors Layer

`VectorRecord` already includes `org_id` field. The vectors table has indexes on repo_id, org_id, type, name — designed for org-level queries.

### Orchestrator Manager

Comprehensive interface with analysis, retrieval, search, AI features, incremental operations.
Uses per-repo locking via `LockManager` (not global mutex).

### Testing

- Table-driven tests with multiple cases per function
- Helper factories: `createTestSQLiteStore()`, `createTestRepoContext()`
- Deferred cleanup with temp directories
- Co-located `*_test.go` files with `testdata/` dirs

### Dependencies

- `github.com/go-git/go-git/v5 v5.16.5` (Git cloning)
- `github.com/mattn/go-sqlite3 v1.14.34` (SQLite)
- Minimal external dependencies, strong stdlib reliance

### Main Server Entry Point (`cmd/mcp-server/main.go`)

Initialization order:
1. FilesystemStore creation (org storage)
2. GitCloner, FileScanner
3. Orchestrator Manager
4. Comparer
5. **OrgManager creation**
6. VectorStore (dimension=384)
7. MCP Server with all dependencies
8. ServeStdio()

### Summary: 70% Foundation Already Exists

The org package, vector org_id support, and MCP tools (register_org, list_orgs) are already in place. The task is primarily about:
1. Extending the org package with SQLite-backed storage
2. Adding `analyze_org` tool
3. Ensuring org-level queries flow through storage
4. Comprehensive testing

---

## Part 2: Web Research

### 1. Go SQLite Schema Migration Patterns

**Recommended: golang-migrate** (industry standard)
- Paired SQL files: `NNN_name.up.sql` / `NNN_name.down.sql`
- SQLite driver auto-wraps migrations in implicit transactions — do NOT use explicit BEGIN/COMMIT
- Sequential numbering with descriptive names
- One logical change per migration
- Never modify deployed migrations; create new ones

**Key SQLite-specific considerations:**
- PRAGMA statements (like `foreign_keys`) cannot execute within transactions
- Use `_journal_mode=WAL` for concurrent reads
- Enable `_foreign_keys=ON` in connection string

### 2. MCP Tool Registration

**From official spec (modelcontextprotocol.io):**
- Tool name: 1-128 chars, case-sensitive, A-Z/a-z/0-9/_/-/.
- Required: name, description, inputSchema (JSON Schema)
- inputSchema must be valid JSON Schema object (not null)
- For no-param tools: `{ "type": "object", "additionalProperties": false }`
- Results include content array + optional structuredContent
- Error handling: protocol errors (JSON-RPC) vs tool execution errors (isError: true)

### 3. Multi-Tenant Data Models

**Recommended: Shared Database, Shared Schema** (for SQLite)
- Single database, org_id on relevant tables
- Simpler operations, easier cross-org queries
- Junction tables for many-to-many (org ↔ repos)
- CASCADE deletes on org removal
- TEXT PRIMARY KEY for IDs

**Alternative (Database-per-Tenant):**
- Better isolation but more complex
- SQLite ATTACH for cross-org queries
- Only if compliance requires strict separation

### 4. Testing Patterns for SQLite Services

**Best practices:**
- Use `:memory:` SQLite for fast unit tests
- Each test gets fresh in-memory database
- Enable `_foreign_keys=ON` in test connections
- Table-driven tests for multiple scenarios
- Do NOT mock the database layer — use real SQLite in-memory
- Always use parameterized queries (`?` placeholders)
- Defer tx.Rollback() for cleanup in transaction tests
- Repository pattern for testable data access
