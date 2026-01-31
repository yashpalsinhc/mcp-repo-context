# MCP Repo Context Server - Implementation Roadmap

> A comprehensive plan to transform this MVP into a production-ready, agent-friendly MCP server that actually reduces token usage.

---

## Implementation Progress

| Task | Status | Files Created/Modified |
|------|--------|------------------------|
| **Phase 1.1: Per-Repo Locking** | ✅ DONE | `internal/orchestrator/locks.go`, `manager.go` |
| **Phase 1.2: Context Cancellation** | ✅ DONE | `internal/orchestrator/manager.go` |
| **Phase 1.3: AI Retry Logic** | ✅ DONE | `internal/ai/retry.go`, `anthropic.go` |
| **Phase 1.4: Metadata Index** | ✅ DONE | `internal/storage/metadata_index.go` |
| **Phase 1.5: Configuration System** | ✅ DONE | `internal/config/config.go` |
| **Phase 2.1: SQLite Storage** | ✅ DONE | `internal/storage/sqlite.go`, `sqlite_search.go`, `migrations/` |
| **Phase 2.2: Hybrid Search** | ✅ DONE | Included in `sqlite_search.go` |
| **Phase 2.3: Incremental Indexing** | ✅ DONE | `internal/storage/file_tracker.go`, `migrations/002_file_hashes.sql` |
| **Phase 3.1: Tool Hints** | ✅ DONE | `internal/mcp/tool_hints.go` |
| **Phase 3.2: MCP Resources** | ✅ DONE | `internal/mcp/resources.go` |
| **Phase 3.3: Progressive Disclosure** | ✅ DONE | `internal/mcp/progressive.go` |
| **Phase 3.4: Output Budgeting** | ✅ DONE | Added `max_results` to search tools |
| **Phase 3.5: Composable Patterns** | ✅ DONE | `internal/compose/chain.go`, `middleware.go`, `patterns.go`, `builder.go` |
| **Phase 4.1: Token Counting** | ✅ DONE | `internal/tokens/counter.go` |
| **Phase 4.2: Token Budgeting** | ✅ DONE | `internal/tokens/budgeter.go` |
| **Phase 4.3: Context Compression** | ✅ DONE | `internal/tokens/compressor.go` |
| **Phase 4.4: Usage Analytics** | ✅ DONE | `internal/analytics/tracker.go`, `middleware.go` |
| **Phase 5.1: Vector Embeddings** | ✅ DONE | `internal/vectors/embedder.go`, `store.go`, `search.go`, `similarity.go` |
| **Phase 5.2: Call Graph Visualization** | ✅ DONE | `internal/graph/visualizer.go` |
| **Phase 5.3: MCP Tool Integration** | ✅ DONE | `semantic_search`, `get_context_budgeted`, `execute_pattern`, `visualize_call_graph`, `get_usage_stats` tools |

**Last Updated**: 2026-01-31

### New MCP Tools Added

| Tool | Description | Package |
|------|-------------|---------|
| `semantic_search` | Vector-based semantic code search | `internal/vectors` |
| `index_repository` | Index repository for semantic search | `internal/vectors` |
| `get_context_budgeted` | Token-aware context retrieval | `internal/tokens` |
| `execute_pattern` | Run composed tool chains | `internal/compose` |
| `list_patterns` | List available patterns | `internal/compose` |
| `get_usage_stats` | View tool usage analytics | `internal/analytics` |
| `visualize_call_graph` | Generate Mermaid/DOT call graphs | `internal/graph` |

---

## Table of Contents

1. [Current State Analysis](#current-state-analysis)
2. [Architecture Vision](#architecture-vision)
3. [Phase 1: Critical Fixes](#phase-1-critical-fixes-week-1-2)
4. [Phase 2: Storage & Search Revolution](#phase-2-storage--search-revolution-week-3-4)
5. [Phase 3: Agent-Friendly Design](#phase-3-agent-friendly-design-week-5-6)
6. [Phase 4: Token Intelligence](#phase-4-token-intelligence-week-7-8)
7. [Phase 5: Advanced Features](#phase-5-advanced-features-week-9-12)
8. [Research References](#research-references)

---

## Current State Analysis

### What Works
- Go AST analysis extracts useful information
- MCP JSON-RPC 2.0 protocol correctly implemented
- Call graph extraction functional
- Modular analyzer registry allows language extensions
- Basic filesystem storage works for small repos

### Critical Problems

| Problem | Impact | Status | Current Location |
|---------|--------|--------|------------------|
| Global mutex serializes all operations | Users wait minutes, timeouts | **FIXED** | `locks.go` (per-repo locking) |
| ListContexts reads all JSON files fully | O(n) file I/O for list | **FIXED** | `metadata_index.go` |
| No context cancellation in loops | Wasted resources on cancel | **FIXED** | `manager.go` |
| No retry logic for AI calls | Single failure = total failure | **FIXED** | `retry.go` |
| No configuration system | Magic numbers | **FIXED** | `config/config.go` |
| Entire repo in memory as one blob | OOM on large repos | PARTIAL | SQLite stores by table now |
| No actual token counting | Claims are guesses | NOT FIXED | CLAUDE.md |
| No inverted index | Linear search O(n) | **FIXED** | `sqlite.go` - indexed lookups |

### Missing From Design Doc

| Feature | Status | Priority |
|---------|--------|----------|
| SQLite/PostgreSQL database | Not implemented | Critical |
| Inverted index for search | Not implemented | Critical |
| HTTP/SSE transport | Not implemented | High |
| Call graph edge storage | Partial | High |
| Error flow analysis | Partial | Medium |
| RCA engine | Not implemented | Medium |
| Impact analyzer | Not implemented | Medium |
| Agent delegation | Not implemented | Low |
| Multiple AI providers | Only Anthropic | Low |

---

## Architecture Vision

### Current Architecture (Problematic)
```
┌─────────────────────────────────────────────────────┐
│                    MCP Server                        │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐  │
│  │   Tools     │──│  Manager    │──│  Storage    │  │
│  │  (15+)      │  │ (monolith)  │  │ (JSON fs)   │  │
│  └─────────────┘  └─────────────┘  └─────────────┘  │
│         │                │                │          │
│         └────────────────┼────────────────┘          │
│                          │                           │
│                    [GLOBAL MUTEX]                    │
│                    [FULL FILE I/O]                   │
│                    [NO INDEXES]                      │
└─────────────────────────────────────────────────────┘
```

### Target Architecture
```
┌──────────────────────────────────────────────────────────────────┐
│                         MCP Server                                │
│                                                                   │
│  ┌──────────────────────────────────────────────────────────┐    │
│  │                    Tool Layer (Thin)                      │    │
│  │   analyze  search  ask  get_context  review_pr  ...      │    │
│  └───────────────────────────┬──────────────────────────────┘    │
│                              │                                    │
│  ┌───────────┬───────────────┼───────────────┬───────────────┐   │
│  │           │               │               │               │   │
│  ▼           ▼               ▼               ▼               ▼   │
│ ┌─────┐  ┌─────────┐  ┌───────────┐  ┌──────────┐  ┌────────┐   │
│ │Index│  │Analysis │  │ Context   │  │   AI     │  │ Query  │   │
│ │Svc  │  │  Svc    │  │   Svc     │  │   Svc    │  │  Svc   │   │
│ └──┬──┘  └────┬────┘  └─────┬─────┘  └────┬─────┘  └───┬────┘   │
│    │          │             │             │            │         │
│    └──────────┴─────────────┴─────────────┴────────────┘         │
│                              │                                    │
│  ┌───────────────────────────▼───────────────────────────────┐   │
│  │                     Storage Layer                          │   │
│  │  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────┐│   │
│  │  │   SQLite    │  │   Blobs     │  │   Vector Store      ││   │
│  │  │  (indexes)  │  │  (content)  │  │   (embeddings)      ││   │
│  │  └─────────────┘  └─────────────┘  └─────────────────────┘│   │
│  └───────────────────────────────────────────────────────────┘   │
└──────────────────────────────────────────────────────────────────┘
```

### Key Principles

1. **Separation of Concerns**: Split monolithic manager into focused services
2. **Lazy Loading**: Never load full repo context into memory
3. **Indexed Queries**: All searches hit indexes, not raw data
4. **Per-Repo Locks**: Concurrent operations on different repos
5. **Streaming**: Large responses streamed, not buffered
6. **Token Awareness**: Actual token counting, not guesses

---

## Phase 1: Critical Fixes (Week 1-2) - COMPLETED

### 1.1 Fix the Global Mutex - DONE

**Problem**: Single mutex blocks all operations during analysis.

**Solution**: Per-repo locks with a lock manager.

**Implementation**: See `internal/orchestrator/locks.go` - includes context-aware locking, timeouts, TryLock, and RLock support.

```go
// internal/orchestrator/locks.go
type LockManager struct {
    mu    sync.Mutex
    locks map[string]*sync.RWMutex
}

func (m *LockManager) Lock(repoID string) {
    m.mu.Lock()
    lock, ok := m.locks[repoID]
    if !ok {
        lock = &sync.RWMutex{}
        m.locks[repoID] = lock
    }
    m.mu.Unlock()
    lock.Lock()
}

func (m *LockManager) RLock(repoID string) {
    // Similar but RLock for read operations
}
```

**Files to modify**:
- `internal/orchestrator/manager.go` - Replace global mutex
- Create `internal/orchestrator/locks.go`

### 1.2 Add Context Cancellation - DONE

**Problem**: Operations continue after cancellation.

**Solution**: Check `ctx.Done()` in all loops.

**Implementation**: Added context cancellation check in file scanning loop in `manager.go:122-125`.

```go
// In scanner callback
err = m.scanner.Scan(ctx, localPath, func(file repo.FileInfo) error {
    select {
    case <-ctx.Done():
        return ctx.Err()
    default:
    }
    // ... rest of processing
})
```

**Files to modify**:
- `internal/orchestrator/manager.go`
- `internal/repo/scanner.go`
- `internal/analyzer/*.go`

### 1.3 Add Retry Logic for AI Calls - DONE

**Problem**: Single failure = total failure.

**Solution**: Exponential backoff with jitter.

**Implementation**: See `internal/ai/retry.go` - includes configurable retry with exponential backoff, jitter, status code handling, and context cancellation support. The Anthropic provider now uses this retry logic for all API calls.

```go
// internal/ai/retry.go
func WithRetry(ctx context.Context, fn func() error, maxRetries int) error {
    var lastErr error
    for i := 0; i < maxRetries; i++ {
        if err := fn(); err != nil {
            lastErr = err
            if !isRetryable(err) {
                return err
            }
            delay := time.Duration(1<<i) * time.Second
            jitter := time.Duration(rand.Intn(1000)) * time.Millisecond
            select {
            case <-ctx.Done():
                return ctx.Err()
            case <-time.After(delay + jitter):
            }
            continue
        }
        return nil
    }
    return fmt.Errorf("max retries exceeded: %w", lastErr)
}
```

**Files to modify**:
- Create `internal/ai/retry.go`
- `internal/ai/anthropic.go` - Wrap API calls

### 1.4 Add Metadata Separation - DONE

**Problem**: ListContexts reads entire JSON files.

**Solution**: Store metadata separately.

**Implementation**: See `internal/storage/metadata_index.go` - provides O(1) listing without reading multi-MB context files. Includes in-memory caching, persistence to disk, and automatic rebuild from full contexts.

```go
// internal/storage/filesystem.go

// Store metadata in a separate index file
type MetadataIndex struct {
    Repos map[string]ContextMetadata `json:"repos"`
}

func (s *filesystemStore) indexPath() string {
    return filepath.Join(s.basePath, "_index.json")
}

func (s *filesystemStore) ListContexts(ctx context.Context) ([]ContextMetadata, error) {
    // Read only the small index file, not all repo files
    data, err := os.ReadFile(s.indexPath())
    // ...
}
```

**Files to modify**:
- `internal/storage/filesystem.go`
- `internal/storage/store.go` - Add UpdateMetadata method

### 1.5 Add Configuration System - DONE

**Problem**: Magic numbers everywhere.

**Solution**: Environment-based configuration.

**Implementation**: See `internal/config/config.go` - comprehensive configuration for storage, AI, analysis, server, and logging. Supports JSON config files with environment variable overrides. Includes validation.

```go
// internal/config/config.go
type Config struct {
    Storage    StorageConfig
    Analysis   AnalysisConfig
    AI         AIConfig
    Limits     LimitsConfig
}

type LimitsConfig struct {
    MaxFileSize       int64         `env:"MAX_FILE_SIZE" default:"1048576"`
    MaxFilesPerRepo   int           `env:"MAX_FILES_PER_REPO" default:"10000"`
    CacheMaxAge       time.Duration `env:"CACHE_MAX_AGE" default:"24h"`
    SearchMaxResults  int           `env:"SEARCH_MAX_RESULTS" default:"50"`
    ContextMaxFiles   int           `env:"CONTEXT_MAX_FILES" default:"10"`
    ContextMaxTokens  int           `env:"CONTEXT_MAX_TOKENS" default:"6000"`
}

func LoadFromEnv() *Config {
    // Parse environment variables with defaults
}
```

**New files**:
- `internal/config/config.go`
- `internal/config/defaults.go`

---

## Phase 2: Storage & Search Revolution (Week 3-4) - COMPLETED

### 2.1 Implement SQLite Storage - DONE

**Implementation**: See `internal/storage/sqlite.go` and `internal/storage/sqlite_search.go`

- Full SQLite schema with indexed tables for repos, files, functions, types, constants
- Lookup tables for side effects and concepts (fast O(1) filtering)
- LIKE-based search (portable, no FTS5 dependency)
- Hybrid search combining keyword and concept matching
- Call graph edges stored in `function_calls` table
- All 13 SQLite tests passing

**Why SQLite?**
- [SQLite FTS5](https://sqlite.org/fts5.html) provides fast full-text search
- Handles up to ~100K documents efficiently
- No external dependencies
- ACID transactions
- External content tables reduce storage

**Schema Design**:

```sql
-- Core tables
CREATE TABLE repos (
    id TEXT PRIMARY KEY,
    url TEXT NOT NULL,
    branch TEXT NOT NULL,
    commit_hash TEXT NOT NULL,
    analyzed_at TIMESTAMP NOT NULL,
    file_count INTEGER,
    stats_json TEXT  -- RepoStatistics as JSON
);

CREATE TABLE files (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    repo_id TEXT NOT NULL REFERENCES repos(id) ON DELETE CASCADE,
    path TEXT NOT NULL,
    hash TEXT NOT NULL,
    language TEXT,
    package TEXT,
    line_count INTEGER,
    purpose TEXT,
    concepts_json TEXT,  -- []string as JSON
    UNIQUE(repo_id, path)
);

CREATE TABLE functions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    file_id INTEGER NOT NULL REFERENCES files(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    signature TEXT,
    description TEXT,
    receiver TEXT,
    is_public BOOLEAN,
    line_start INTEGER,
    line_end INTEGER,
    complexity INTEGER,
    behavior_json TEXT,  -- FunctionBehavior as JSON
    error_handling_json TEXT,
    api_flow_json TEXT
);

CREATE TABLE function_calls (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    caller_id INTEGER NOT NULL REFERENCES functions(id) ON DELETE CASCADE,
    callee_name TEXT NOT NULL,
    callee_package TEXT,
    line INTEGER,
    call_type TEXT  -- internal, external, stdlib
);

CREATE TABLE types (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    file_id INTEGER NOT NULL REFERENCES files(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    kind TEXT,  -- struct, interface, alias
    is_public BOOLEAN,
    fields_json TEXT,
    methods_json TEXT
);

-- Full-text search indexes
CREATE VIRTUAL TABLE functions_fts USING fts5(
    name, signature, description,
    content='functions',
    content_rowid='id'
);

CREATE VIRTUAL TABLE files_fts USING fts5(
    path, purpose,
    content='files',
    content_rowid='id'
);

-- Side effects index for fast lookup
CREATE TABLE side_effects (
    function_id INTEGER NOT NULL REFERENCES functions(id) ON DELETE CASCADE,
    effect TEXT NOT NULL,  -- http_call, db_query, file_io, etc.
    PRIMARY KEY (function_id, effect)
);
CREATE INDEX idx_side_effects_effect ON side_effects(effect);

-- Concepts index
CREATE TABLE concepts (
    function_id INTEGER NOT NULL REFERENCES functions(id) ON DELETE CASCADE,
    concept TEXT NOT NULL,
    PRIMARY KEY (function_id, concept)
);
CREATE INDEX idx_concepts_concept ON concepts(concept);

-- Triggers to keep FTS in sync
CREATE TRIGGER functions_fts_insert AFTER INSERT ON functions BEGIN
    INSERT INTO functions_fts(rowid, name, signature, description)
    VALUES (new.id, new.name, new.signature, new.description);
END;

CREATE TRIGGER functions_fts_delete AFTER DELETE ON functions BEGIN
    INSERT INTO functions_fts(functions_fts, rowid, name, signature, description)
    VALUES ('delete', old.id, old.name, old.signature, old.description);
END;
```

**Implementation**:

```go
// internal/storage/sqlite.go
type SQLiteStore struct {
    db *sql.DB
}

func NewSQLiteStore(path string) (*SQLiteStore, error) {
    db, err := sql.Open("sqlite3", path+"?_journal_mode=WAL&_synchronous=NORMAL")
    if err != nil {
        return nil, err
    }

    // Run migrations
    if err := runMigrations(db); err != nil {
        return nil, err
    }

    return &SQLiteStore{db: db}, nil
}

// Efficient search using FTS5
func (s *SQLiteStore) SearchFunctions(ctx context.Context, query string, repoID string) ([]FunctionRef, error) {
    sql := `
        SELECT f.id, fi.path, f.name, f.signature, f.line_start
        FROM functions_fts fts
        JOIN functions f ON f.id = fts.rowid
        JOIN files fi ON fi.id = f.file_id
        WHERE functions_fts MATCH ?
        AND fi.repo_id = ?
        ORDER BY rank
        LIMIT 50
    `
    // Execute and return results
}

// Fast side effect lookup
func (s *SQLiteStore) SearchBySideEffect(ctx context.Context, repoID, effect string) ([]FunctionRef, error) {
    sql := `
        SELECT fi.path, f.name, f.signature, f.line_start
        FROM side_effects se
        JOIN functions f ON f.id = se.function_id
        JOIN files fi ON fi.id = f.file_id
        WHERE se.effect = ?
        AND fi.repo_id = ?
    `
    // Execute and return results
}
```

**New files**:
- `internal/storage/sqlite.go`
- `internal/storage/migrations/` - SQL migration files
- `internal/storage/queries.go` - Query implementations

### 2.2 Implement Hybrid Search

**Approach**: Combine keyword search (FTS5) with concept/semantic matching.

```go
// internal/search/hybrid.go
type HybridSearcher struct {
    store *SQLiteStore
}

func (s *HybridSearcher) Search(ctx context.Context, query string, opts SearchOptions) ([]SearchResult, error) {
    // 1. FTS5 keyword search
    keywordResults, _ := s.store.SearchFTS(ctx, query)

    // 2. Concept matching (map common terms to concepts)
    concepts := extractConcepts(query)
    conceptResults, _ := s.store.SearchByConcepts(ctx, concepts)

    // 3. Merge with reciprocal rank fusion
    return fuseResults(keywordResults, conceptResults), nil
}

func extractConcepts(query string) []string {
    // Map natural language to code concepts
    conceptMap := map[string]string{
        "login":         "authentication",
        "auth":          "authentication",
        "validate":      "validation",
        "database":      "db_query",
        "http":          "http_call",
        "api":           "handler",
        "error":         "error_handling",
        "test":          "testing",
    }
    // Extract matching concepts from query
}
```

### 2.3 Add Incremental Indexing

**Why?** Based on [Meta's Glean](https://engineering.fb.com/2024/12/19/developer-tools/glean-open-source-code-indexing/), incremental indexing is critical for large codebases.

```go
// internal/indexer/incremental.go
type IncrementalIndexer struct {
    store    *SQLiteStore
    watcher  *fsnotify.Watcher
    analyzer analyzer.Analyzer
}

func (i *IncrementalIndexer) Watch(ctx context.Context, repoPath string) error {
    return i.watcher.Add(repoPath)
}

func (i *IncrementalIndexer) handleChange(event fsnotify.Event) error {
    switch {
    case event.Op&fsnotify.Write != 0:
        return i.reindexFile(event.Name)
    case event.Op&fsnotify.Create != 0:
        return i.indexNewFile(event.Name)
    case event.Op&fsnotify.Remove != 0:
        return i.removeFile(event.Name)
    }
    return nil
}

func (i *IncrementalIndexer) reindexFile(path string) error {
    // 1. Compute new hash
    // 2. Compare with stored hash
    // 3. If different, re-analyze and update indexes
    // 4. Update affected call graph edges
}
```

---

## Phase 3: Agent-Friendly Design (Week 5-6) - ✅ COMPLETED

### 3.1 Implement MCP Best Practices

Based on [MCP Best Practices Guide](https://modelcontextprotocol.info/docs/best-practices/), follow these principles:

**1. Single Purpose per Tool**

Split complex tools into focused ones:

```
Before:
  get_context(scope: "full"|"architecture"|"file"|"function")

After:
  get_repo_overview()      - High-level stats only
  get_architecture()       - Modules and layers
  get_file_details(path)   - Single file info
  get_function_details()   - Single function deep dive
```

**2. Predictable Output Sizes**

```go
// Add size hints to tool definitions
{
    Name: "get_repo_overview",
    Description: "Returns ~500 tokens of high-level repo info",
    OutputHint: "small",  // New field
}

// Implement output budgeting
type OutputBudget struct {
    MaxTokens int
    Format    string  // "summary" | "detailed" | "minimal"
}
```

**3. Progressive Disclosure**

```go
// Tools return references, not full content
type FunctionSummary struct {
    Name      string `json:"name"`
    Signature string `json:"signature"`
    Summary   string `json:"summary"`      // 1 line
    DetailRef string `json:"detail_ref"`   // "func:repo:file:name" for get_function_details
}
```

### 3.2 Add Tool Hints for Agents - DONE

**Implementation**: See `internal/mcp/tool_hints.go`

Based on [mcp-agent patterns](https://github.com/lastmile-ai/mcp-agent), add metadata to help agents choose tools:

```go
type ToolDefinition struct {
    Name        string
    Description string
    InputSchema map[string]interface{}

    // New: Agent hints
    Hints       ToolHints
}

type ToolHints struct {
    OutputTokens  string   // "tiny" (<100), "small" (<500), "medium" (<2000), "large" (>2000)
    UseCases      []string // ["find function", "understand architecture"]
    Prerequisites []string // ["analyze_repo must be run first"]
    Composable    []string // ["use with get_function_context for details"]
    CostLevel     string   // "free" (no AI), "cheap" (cached), "expensive" (AI call)
}
```

### 3.3 Implement Composable Patterns

From [Building Effective Agents](https://www.anthropic.com/research/building-effective-agents):

**1. Map-Reduce Pattern**

```go
// Tool: search_and_summarize
// Searches across repos, then summarizes findings

func (s *server) toolSearchAndSummarize(ctx context.Context, args map[string]any) callToolResult {
    query := args["query"].(string)

    // Map: Search all repos in parallel
    var wg sync.WaitGroup
    results := make(chan SearchResult, 100)

    for _, repoID := range repoIDs {
        wg.Add(1)
        go func(id string) {
            defer wg.Done()
            found, _ := s.searcher.Search(ctx, id, query)
            for _, r := range found {
                results <- r
            }
        }(repoID)
    }

    // Reduce: Aggregate and rank
    go func() {
        wg.Wait()
        close(results)
    }()

    return rankAndFormat(results)
}
```

**2. Router Pattern**

```go
// smart_query already implements this, but make it explicit
type QueryRouter struct {
    patterns []QueryPattern
}

type QueryPattern struct {
    Regex     *regexp.Regexp
    Handler   func(context.Context, string) (string, error)
    TokenCost string
}

func NewQueryRouter() *QueryRouter {
    return &QueryRouter{
        patterns: []QueryPattern{
            {regexp.MustCompile(`what does (\w+) do`), handleFunctionQuery, "small"},
            {regexp.MustCompile(`who calls (\w+)`), handleCallersQuery, "small"},
            {regexp.MustCompile(`find .*(db|database)`), handleSideEffectQuery, "medium"},
            {regexp.MustCompile(`how does .* work`), handleArchitectureQuery, "large"},
        },
    }
}
```

### 3.4 Add Resource Protocol Support - DONE

**Implementation**: See `internal/mcp/resources.go`

MCP Resources protocol now implemented for direct access to repository data:

```go
// internal/mcp/resources.go
func (s *server) handleListResources(req *jsonRPCRequest) *jsonRPCResponse {
    repos, _ := s.manager.ListRepos(context.Background())

    var resources []ResourceDefinition
    for _, repo := range repos {
        resources = append(resources, ResourceDefinition{
            URI:         fmt.Sprintf("repo://%s/overview", repo.RepoID),
            Name:        repo.RepoID + " Overview",
            Description: "High-level repository summary",
            MimeType:    "application/json",
        })
        resources = append(resources, ResourceDefinition{
            URI:         fmt.Sprintf("repo://%s/architecture", repo.RepoID),
            Name:        repo.RepoID + " Architecture",
            Description: "Architecture and module structure",
            MimeType:    "application/json",
        })
    }

    return &jsonRPCResponse{
        JSONRPC: "2.0",
        ID:      req.ID,
        Result:  listResourcesResult{Resources: resources},
    }
}
```

---

## Phase 4: Token Intelligence (Week 7-8) - ✅ COMPLETED

### 4.1 Implement Actual Token Counting

**Problem**: Claims like "~2k tokens" are guesses.

**Solution**: Use tiktoken-go or count approximation.

```go
// internal/tokens/counter.go
type TokenCounter struct {
    encoding *tiktoken.Encoding
}

func NewTokenCounter() (*TokenCounter, error) {
    enc, err := tiktoken.GetEncoding("cl100k_base") // Claude's encoding
    if err != nil {
        return nil, err
    }
    return &TokenCounter{encoding: enc}, nil
}

func (c *TokenCounter) Count(text string) int {
    return len(c.encoding.Encode(text, nil, nil))
}

func (c *TokenCounter) CountJSON(v interface{}) int {
    data, _ := json.Marshal(v)
    return c.Count(string(data))
}
```

### 4.2 Add Token Budgeting

```go
// internal/context/budgeter.go
type TokenBudgeter struct {
    counter *TokenCounter
    budget  int
}

func (b *TokenBudgeter) BuildContext(files []FileContext, budget int) []FileContext {
    var result []FileContext
    used := 0

    // Sort by relevance (implement scoring)
    sort.Slice(files, func(i, j int) bool {
        return files[i].RelevanceScore > files[j].RelevanceScore
    })

    for _, f := range files {
        tokens := b.counter.CountJSON(f)
        if used + tokens > budget {
            // Try to include summarized version
            summary := b.summarize(f)
            summaryTokens := b.counter.CountJSON(summary)
            if used + summaryTokens <= budget {
                result = append(result, summary)
                used += summaryTokens
            }
            continue
        }
        result = append(result, f)
        used += tokens
    }

    return result
}
```

### 4.3 Implement Context Compression

Based on [LongCodeZip](https://arxiv.org/html/2510.00446v1) research:

```go
// internal/context/compressor.go
type ContextCompressor struct {
    counter *TokenCounter
}

// Coarse-grained: Keep only relevant functions
func (c *ContextCompressor) FilterRelevant(query string, funcs []FunctionDef, budget int) []FunctionDef {
    // Score functions by relevance to query
    scored := make([]scoredFunc, len(funcs))
    for i, f := range funcs {
        scored[i] = scoredFunc{
            func_:     f,
            relevance: c.scoreRelevance(query, f),
        }
    }

    // Sort by relevance
    sort.Slice(scored, func(i, j int) bool {
        return scored[i].relevance > scored[j].relevance
    })

    // Take top functions within budget
    var result []FunctionDef
    tokens := 0
    for _, sf := range scored {
        funcTokens := c.counter.CountJSON(sf.func_)
        if tokens + funcTokens > budget {
            break
        }
        result = append(result, sf.func_)
        tokens += funcTokens
    }

    return result
}

// Fine-grained: Summarize function details
func (c *ContextCompressor) Summarize(f FunctionDef) FunctionSummary {
    return FunctionSummary{
        Name:      f.Name,
        Signature: f.Signature,
        Summary:   f.Behavior.Summary, // Use pre-extracted summary
        Calls:     extractTopCalls(f.Calls, 5),
        Effects:   f.SideEffects,
    }
}
```

### 4.4 Add Usage Analytics

Track actual token usage for optimization:

```go
// internal/analytics/usage.go
type UsageTracker struct {
    db *sql.DB
}

type ToolUsage struct {
    Tool           string
    InputTokens    int
    OutputTokens   int
    AITokens       int  // Tokens sent to Claude
    Duration       time.Duration
    Timestamp      time.Time
}

func (t *UsageTracker) Record(usage ToolUsage) error {
    _, err := t.db.Exec(`
        INSERT INTO tool_usage (tool, input_tokens, output_tokens, ai_tokens, duration_ms, timestamp)
        VALUES (?, ?, ?, ?, ?, ?)
    `, usage.Tool, usage.InputTokens, usage.OutputTokens, usage.AITokens, usage.Duration.Milliseconds(), usage.Timestamp)
    return err
}

func (t *UsageTracker) GetAverages() map[string]TokenStats {
    // Return average token usage per tool
}
```

---

## Phase 5: Advanced Features (Week 9-12) - ✅ COMPLETED

### 5.1 Vector Embeddings for Semantic Search

Based on [CodeGrok approach](https://hackernoon.com/codegrok-mcp-semantic-code-search-that-saves-ai-agents-10x-in-context-usage):

```go
// internal/embeddings/embedder.go
type Embedder struct {
    provider EmbeddingProvider  // OpenAI, VoyageAI, or local
}

func (e *Embedder) EmbedFunction(f FunctionDef) ([]float32, error) {
    // Create embedding text from function
    text := fmt.Sprintf("%s %s %s", f.Name, f.Signature, f.Behavior.Summary)
    return e.provider.Embed(text)
}

// internal/search/semantic.go
type SemanticSearcher struct {
    store    VectorStore  // Milvus, Qdrant, or LanceDB
    embedder *Embedder
}

func (s *SemanticSearcher) Search(ctx context.Context, query string, topK int) ([]FunctionRef, error) {
    // 1. Embed query
    queryVec, _ := s.embedder.Embed(query)

    // 2. Search vector store
    results, _ := s.store.Search(queryVec, topK)

    // 3. Return function references
    return results, nil
}
```

### 5.2 Call Graph Visualization

```go
// internal/graph/visualizer.go
type GraphVisualizer struct {
    store *SQLiteStore
}

func (v *GraphVisualizer) GetCallPath(from, to string) (*CallPath, error) {
    // BFS to find shortest path between functions
}

func (v *GraphVisualizer) GetCallTree(funcName string, depth int) (*CallTree, error) {
    // Get all functions called by funcName up to depth
}

func (v *GraphVisualizer) GetCallerTree(funcName string, depth int) (*CallTree, error) {
    // Get all functions that call funcName up to depth
}
```

### 5.3 Impact Analysis

```go
// internal/impact/analyzer.go
type ImpactAnalyzer struct {
    store *SQLiteStore
    graph *CallGraphStore
}

type ImpactResult struct {
    DirectlyAffected   []FunctionRef
    IndirectlyAffected []FunctionRef
    RiskLevel          string  // low, medium, high, critical
    TestsToRun         []string
}

func (a *ImpactAnalyzer) AnalyzeChange(ctx context.Context, targets []string) (*ImpactResult, error) {
    result := &ImpactResult{}

    for _, target := range targets {
        // Find all callers recursively
        callers := a.graph.GetAllCallers(target)
        result.DirectlyAffected = append(result.DirectlyAffected, callers.Direct...)
        result.IndirectlyAffected = append(result.IndirectlyAffected, callers.Indirect...)
    }

    // Calculate risk based on affected surface
    result.RiskLevel = a.calculateRisk(result)

    // Find related tests
    result.TestsToRun = a.findRelatedTests(result.DirectlyAffected)

    return result, nil
}
```

### 5.4 HTTP/SSE Transport

```go
// internal/mcp/transport/http.go
type HTTPServer struct {
    handler *MCPHandler
    port    int
}

func (s *HTTPServer) Start(ctx context.Context) error {
    mux := http.NewServeMux()

    // JSON-RPC endpoint
    mux.HandleFunc("/rpc", s.handleRPC)

    // SSE endpoint for streaming
    mux.HandleFunc("/events", s.handleSSE)

    // Health check
    mux.HandleFunc("/health", s.handleHealth)

    server := &http.Server{
        Addr:    fmt.Sprintf(":%d", s.port),
        Handler: mux,
    }

    go func() {
        <-ctx.Done()
        server.Shutdown(context.Background())
    }()

    return server.ListenAndServe()
}
```

---

## Research References

### MCP Protocol & Best Practices
- [MCP Specification (June 2025)](https://modelcontextprotocol.io/specification/2025-06-18) - Official protocol spec
- [MCP Best Practices Guide](https://modelcontextprotocol.info/docs/best-practices/) - Architecture patterns
- [mcp-agent](https://github.com/lastmile-ai/mcp-agent) - Composable agent patterns
- [Thoughtworks MCP Analysis](https://www.thoughtworks.com/en-us/insights/blog/generative-ai/model-context-protocol-mcp-impact-2025) - Industry impact

### Token Compression & Efficiency
- [LongCodeZip](https://arxiv.org/html/2510.00446v1) - 5.6x compression for code LLMs
- [JENGA](https://www.usenix.org/system/files/atc25-wang-tuowei.pdf) - 1.93x memory reduction for long context
- [Token Compression Survey](https://www.aussieai.com/research/token-compression) - Comprehensive overview
- [Nano Surge Approach](https://arxiv.org/html/2504.15989v2) - Code reasoning efficiency

### Code Search & Indexing
- [CodeGrok MCP](https://hackernoon.com/codegrok-mcp-semantic-code-search-that-saves-ai-agents-10x-in-context-usage) - 10x context savings
- [Meta's Glean](https://engineering.fb.com/2024/12/19/developer-tools/glean-open-source-code-indexing/) - Incremental code indexing
- [Semantic Code Search](https://medium.com/@wangxj03/semantic-code-search-010c22e7d267) - AST + embeddings
- [Searchcode Index](https://boyter.org/posts/how-i-built-my-own-index-for-searchcode/) - Custom trigram index

### Storage & Database
- [SQLite FTS5](https://sqlite.org/fts5.html) - Full-text search extension
- [SQLite FTS5 Guide](https://blog.sqlite.ai/fts5-sqlite-text-search-extension) - Practical implementation
- [go-incr](https://github.com/wcharczuk/go-incr) - Incremental computation in Go

### Security
- [MCP Auth Updates (June 2025)](https://auth0.com/blog/mcp-specs-update-all-about-auth/) - Authorization best practices
- [MCP Security Analysis](https://en.wikipedia.org/wiki/Model_Context_Protocol) - Known vulnerabilities

---

## Implementation Checklist

### Phase 1 (Week 1-2)
- [ ] Replace global mutex with per-repo locks
- [ ] Add context cancellation checks in all loops
- [ ] Implement retry logic for AI calls
- [ ] Separate metadata storage
- [ ] Create configuration system
- [ ] Add comprehensive error handling

### Phase 2 (Week 3-4)
- [ ] Design SQLite schema
- [ ] Implement SQLite storage adapter
- [ ] Add FTS5 full-text search
- [ ] Create migration system
- [ ] Implement hybrid search
- [ ] Add incremental indexing

### Phase 3 (Week 5-6)
- [ ] Split tools into focused single-purpose
- [x] Add tool hints for agents
- [x] Implement progressive disclosure
- [x] Add MCP Resources support
- [x] Add output budgeting (max_results)
- [ ] Create composable patterns
- [ ] Document agent usage patterns

### Phase 4 (Week 7-8)
- [ ] Implement token counting
- [ ] Add token budgeting
- [ ] Create context compression
- [ ] Add usage analytics
- [ ] Update documentation with real token costs

### Phase 5 (Week 9-12)
- [ ] Add vector embeddings (optional)
- [ ] Implement semantic search
- [ ] Create call graph visualization
- [ ] Build impact analyzer
- [ ] Add HTTP/SSE transport
- [ ] Performance testing and optimization

---

## Success Metrics

| Metric | Current | Target |
|--------|---------|--------|
| ListRepos latency | O(n) file reads | O(1) index read |
| Search latency | O(n) linear scan | O(log n) indexed |
| Concurrent analysis | 1 (global mutex) | Unlimited (per-repo) |
| Memory for large repo | Full repo in RAM | Streaming/lazy |
| Token accuracy | Guessed | Measured |
| Agent tool selection | Trial and error | Hinted guidance |

---

*Document version: 1.0*
*Last updated: 2026-01-30*
*Author: Code Review Analysis*
