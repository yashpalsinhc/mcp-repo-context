# MCP Repo Context Server

A Model Context Protocol (MCP) server that generates deep, structured context from codebases using AST parsing, call graph analysis, and semantic search — enabling AI agents to understand and navigate repositories with minimal token usage.

Built in Go. Two direct dependencies. 35+ tools.

## Why

AI coding assistants waste tokens re-reading files every session. This server analyzes repositories once, stores structured context (functions, types, call graphs, behaviors, side effects), and serves it on demand via MCP — so the AI gets precise answers in ~2-4K tokens instead of ~50K+.

## Features

- **AST-Based Analysis** — Full Go AST parsing: functions, types, interfaces, imports, complexity metrics
- **Call Graph** — Maps caller/callee relationships across the entire codebase, with Mermaid/DOT visualization
- **Semantic Search** — Local vector embeddings (384-dim) stored in SQLite for meaning-based code search
- **Behavior Extraction** — Auto-detects what functions do: DB queries (with SQL), HTTP calls, file I/O, error handling
- **Multi-Repo Comparison** — Find duplicates, conflicts, and gaps across repositories
- **Incremental Updates** — Refresh a single file (~1-2K tokens) or git changes (~2-5K tokens) without full re-analysis
- **AI-Powered Q&A** — Natural language queries routed to the right tool automatically via `smart_query`
- **PR Review** — Context-aware pull request analysis using function-level impact analysis
- **Organization Support** — Group repos, query across entire orgs
- **Composable Patterns** — Pre-built tool chains for common workflows (search + expand, impact analysis)

## Architecture

```
MCP Client (Claude Code / AI Agent)
    |  JSON-RPC 2.0 over stdio
    v
+---------------------------+
|   MCP Server (35+ tools)  |
+-----------+---------------+
            |
    +-------+-------+-----------+-----------+
    v       v       v           v           v
Analyzer  Storage  Vector Store  AI Module  Comparer
 (AST)   (JSON/FS) (SQLite)    (Claude)   (Multi-Repo)
    |
    +-- Go AST parser
    +-- Call graph builder
    +-- Search index (concepts, side effects, routes)
    +-- Behavior summarizer
    +-- Error handling detector
```

**Storage:** Each repo is analyzed into a structured JSON file containing all functions, types, call graphs, search indexes, and behavior data. Vector embeddings are stored in SQLite for semantic search.

## Quick Start

### Prerequisites
- Go 1.24+
- GitHub token (for private repos)
- Anthropic API key (optional, for AI features)

### Install from release (recommended)

Binaries are published to [GitHub Releases](https://github.com/yashpalsinhc/mcp-repo-context/releases) with SHA256 checksums (no binaries in the repo). Pre-built binaries are currently provided for **Linux** (amd64, arm64). Install the latest on Linux:

```bash
curl -sSL https://raw.githubusercontent.com/yashpalsinhc/mcp-repo-context/main/scripts/install.sh | bash
# Binary installed to ./bin/mcp-server
```

Or a specific version and destination:

```bash
./scripts/install.sh v1.0.0 /usr/local/bin
# Or: VERSION=v1.0.0 DEST=/usr/local/bin ./scripts/install.sh
```

### Build from source

```bash
git clone https://github.com/yashpalsinhc/mcp-repo-context.git
cd mcp-repo-context
go build -o mcp-server ./cmd/mcp-server
```

### Run

```bash
./mcp-server \
  -storage ./data/contexts \
  -temp /tmp/mcp-repos \
  -github-token $GITHUB_TOKEN
```

### Docker

The image is built from source (no committed binaries). Use `Dockerfile` or `Containerfile` (for Podman/buildah):

```bash
docker build -t mcp-repo-context .
# Or: podman build -t mcp-repo-context .   (Containerfile is used by default)
docker run -it \
  -v $(pwd)/data:/home/mcp/data \
  -e GITHUB_TOKEN \
  -e ANTHROPIC_API_KEY \
  mcp-repo-context
```

### Integrate with Claude Code

Add to your MCP config (`~/.claude/settings.json` or project `.mcp.json`):

```json
{
  "mcpServers": {
    "repo-context": {
      "command": "/path/to/mcp-server",
      "args": ["-storage", "/path/to/data/contexts"],
      "env": {
        "GITHUB_TOKEN": "your-token",
        "ANTHROPIC_API_KEY": "your-key"
      }
    }
  }
}
```

## Tools Reference

### Analysis
| Tool | Purpose | Tokens |
|------|---------|--------|
| `analyze_repo` | Clone and analyze a GitHub repository | ~8K |
| `analyze_local` | Analyze a local directory | ~8K |
| `refresh_file` | Re-analyze a single changed file | ~1-2K |
| `refresh_changed` | Re-analyze all git-changed files | ~2-5K |

### Search & Query
| Tool | Purpose | Tokens |
|------|---------|--------|
| `smart_query` | Natural language — auto-routes to the right tool | ~2-4K |
| `search_context` | Find functions/types/files by keyword | ~2K |
| `semantic_search` | Vector-based search by meaning | ~3K |
| `search_by_concept` | Find code by concept (auth, validation, CRUD) | ~3K |
| `search_by_side_effect` | Find code by effect (db_query, http_call, file_io) | ~3K |
| `get_function_context` | Deep function analysis: behavior, SQL, HTTP calls, callers | ~4K |
| `get_callers` | Who calls this function? | ~2K |
| `get_package_structure` | All files/types/functions in a package | ~3K |

### AI-Powered
| Tool | Purpose | Tokens |
|------|---------|--------|
| `ask` | Intelligent Q&A with auto-extracted context | ~8K |
| `generate_ai_summary` | AI-generated repo overview | ~6K |
| `review_pr` | Context-aware PR review | ~10K |

### Visualization & Comparison
| Tool | Purpose | Tokens |
|------|---------|--------|
| `visualize_call_graph` | Mermaid/DOT call graph diagrams | ~2K |
| `compare_repos` | Find duplicates, conflicts, gaps across repos | ~10K |
| `get_context_budgeted` | Get relevant context within a token budget | varies |

### System
| Tool | Purpose |
|------|---------|
| `list_repos` | List all analyzed repositories |
| `index_repository` | Build vector index for semantic search |
| `get_usage_stats` | Token usage analytics |
| `list_patterns` / `execute_pattern` | Composable tool chains |

## Configuration

| Flag | Env Var | Default | Purpose |
|------|---------|---------|---------|
| `-storage` | `MCP_STORAGE_PATH` | `./data/contexts` | Where analyzed context is stored |
| `-temp` | `MCP_TEMP_DIR` | `/tmp/mcp-repos` | Temp dir for cloning |
| `-github-token` | `GITHUB_TOKEN` | — | GitHub API access |
| — | `ANTHROPIC_API_KEY` | — | Claude API (for AI features) |
| — | `ANTHROPIC_MODEL` | `claude-sonnet-4-20250514` | Model for AI queries |
| — | `MCP_VECTOR_STORE_PATH` | `{storage}/vectors.db` | SQLite vector DB |

## What Gets Extracted (Go)

For every Go file, the analyzer produces:

- **Functions**: signature, parameters, returns, line range, visibility, complexity
- **Behavior**: one-line summary, data reads/writes, external API calls, DB queries with SQL
- **Types**: structs with fields (including JSON/DB tags), interfaces with methods
- **Call Graph**: full caller/callee map across the codebase
- **Search Index**: concepts (auth, validation), side effects (db_query, http_call), routes, error types
- **HTTP Handlers**: method, path, request/response types

## Project Structure

```
cmd/mcp-server/     Entry point, CLI flags, signal handling
internal/
  analyzer/         Go AST parser, call graph, behavior, search index
  ai/               Anthropic Claude integration, context extraction
  comparison/       Multi-repo duplicate/conflict/gap detection
  compose/          Tool composition patterns and chains
  context/          Core types (RepoContext, FileContext, CallGraph)
  graph/            Mermaid/DOT call graph visualization
  mcp/              MCP protocol handler, 35+ tool implementations
  orchestrator/     Analysis orchestration, smart query routing
  org/              Organization-level multi-repo operations
  prreview/         GitHub PR review automation
  repo/             Git clone, file scanning
  storage/          Filesystem-based JSON storage
  vectors/          Local embeddings, SQLite vector store, semantic search
  skills/           Built-in AI prompts (Go expert, PR review, security)
```

## Releasing

Releases are built by [GoReleaser](https://goreleaser.com) on tag push. No binaries are committed; they are published to GitHub Releases with SHA256 checksums.

```bash
git tag -a v1.0.0 -m "Release v1.0.0"
git push origin v1.0.0
```

CI builds Linux amd64/arm64 binaries and uploads them to the [Releases](https://github.com/yashpalsinhc/mcp-repo-context/releases) page.

## Dependencies

Minimal by design:

- `github.com/go-git/go-git/v5` — Git operations
- `github.com/mattn/go-sqlite3` — SQLite (vector store)

## License

MIT
