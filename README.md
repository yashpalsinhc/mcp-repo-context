# MCP Repo Context Server

A stateless MCP (Model Context Protocol) server that generates, stores, and serves comprehensive repository context for AI-powered Q&A.

## Features

- **Analyze GitHub repositories**: Clone and extract code structure, functions, types, and relationships
- **Store context**: Persist analysis as JSON files for fast retrieval
- **Search context**: Find functions, types, and files across analyzed repositories
- **Go language support**: Deep analysis of Go source files using AST parsing
- **Multi-repo comparison**: Compare multiple repositories for duplicates, conflicts, and gaps
- **AI-powered summaries**: Generate intelligent repository summaries using Claude (requires `ANTHROPIC_API_KEY`)
- **AI architecture analysis**: Get AI-generated architecture insights, strengths, weaknesses, and recommendations

## Installation

### Option 1: Docker (Recommended)

```bash
# Clone the repository
git clone https://github.com/yashpalc/mcp-repo-context.git
cd mcp-repo-context

# Build Docker image
docker build -t mcp-repo-context:latest .

# Create data directory
mkdir -p ./data/contexts
```

### Option 2: Build from Source

```bash
# Clone the repository
git clone https://github.com/yashpalc/mcp-repo-context.git
cd mcp-repo-context

# Build and install
go install ./cmd/mcp-server

# Or build only
go build -o mcp-server ./cmd/mcp-server
```

## Usage

### Docker with Claude Code

Add to `~/.claude/settings.json`:

```json
{
  "mcpServers": {
    "repo-context": {
      "command": "/path/to/mcp-repo-context/scripts/run-docker.sh",
      "env": {
        "GITHUB_TOKEN": "your-github-token"
      }
    }
  }
}
```

Or run Docker directly:

```json
{
  "mcpServers": {
    "repo-context": {
      "command": "docker",
      "args": [
        "run", "--rm", "-i",
        "-v", "/path/to/data:/data",
        "-e", "GITHUB_TOKEN",
        "-e", "MCP_STORAGE_PATH=/data/contexts",
        "mcp-repo-context:latest"
      ],
      "env": {
        "GITHUB_TOKEN": "your-github-token"
      }
    }
  }
}
```

### Docker Compose

```bash
# Copy environment file
cp .env.example .env
# Edit .env with your GITHUB_TOKEN

# Run with docker-compose
docker-compose up -d

# With PostgreSQL (for future index storage)
docker-compose --profile with-postgres up -d
```

### Native Binary with Claude Desktop / Claude Code

Add to your Claude Desktop configuration (`~/Library/Application Support/Claude/claude_desktop_config.json`):

```json
{
  "mcpServers": {
    "repo-context": {
      "command": "/path/to/mcp-server",
      "args": ["-storage", "/path/to/data/contexts"],
      "env": {
        "GITHUB_TOKEN": "your-github-token"
      }
    }
  }
}
```

### Command Line Options

```
-storage       Path to store context files (default: ./data/contexts)
-temp          Temporary directory for cloning repos (default: /tmp/mcp-repos)
-github-token  GitHub personal access token (or use GITHUB_TOKEN env var)
-version       Show version
```

### Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `GITHUB_TOKEN` | GitHub personal access token | - |
| `MCP_STORAGE_PATH` | Path to store context files | `./data/contexts` |
| `MCP_TEMP_DIR` | Temporary directory for cloning | `/tmp/mcp-repos` |
| `ANTHROPIC_API_KEY` | Anthropic API key for AI features | - |
| `ANTHROPIC_MODEL` | Claude model to use | `claude-sonnet-4-20250514` |

## Available Tools

### `analyze_repo`

Analyze a GitHub repository and generate comprehensive context.

**Input:**
- `repo_url` (required): GitHub repository URL
- `branch` (optional): Branch to analyze
- `force` (optional): Force re-analysis even if cached

**Example:**
```json
{
  "repo_url": "https://github.com/owner/repo",
  "branch": "main",
  "force": false
}
```

### `get_context`

Retrieve stored context for a repository.

**Input:**
- `repo_id` (required): Repository identifier
- `scope` (optional): "full", "architecture", or "file"
- `file_path` (optional): File path (required if scope is "file")

**Example:**
```json
{
  "repo_id": "github.com/owner/repo",
  "scope": "architecture"
}
```

### `list_repos`

List all analyzed repositories.

**Input:** None

### `search_context`

Search for code elements across repositories.

**Input:**
- `query` (required): Search query
- `repo_id` (optional): Limit to specific repository
- `search_type` (optional): "function", "type", "file", or "all"

**Example:**
```json
{
  "query": "Handler",
  "search_type": "function"
}
```

### `compare_repos`

Compare multiple repositories to identify duplicates, conflicts, gaps, and consistency issues.

**Input:**
- `repo_ids` (required): Array of repository IDs to compare
- `target_repo_id` (optional): Target repository for merge analysis
- `include_duplicates` (optional): Include duplicate detection (default: true)
- `include_conflicts` (optional): Include conflict detection (default: true)
- `include_gaps` (optional): Include gap detection (default: true)

**Example:**
```json
{
  "repo_ids": ["github.com/owner/repo1", "github.com/owner/repo2"],
  "target_repo_id": "github.com/owner/repo1"
}
```

### `find_duplicates`

Find duplicate functions, types, and patterns across multiple repositories.

**Input:**
- `repo_ids` (required): Array of repository IDs to search

### `find_conflicts`

Find conflicting implementations between source and target repositories.

**Input:**
- `source_repo_ids` (required): Array of source repository IDs
- `target_repo_id` (required): Target repository ID

### `find_gaps`

Find functionality in source repositories that is missing from the target.

**Input:**
- `source_repo_ids` (required): Array of source repository IDs
- `target_repo_id` (required): Target repository ID

### `generate_ai_summary`

Generate an AI-powered summary of a repository using Claude. Requires `ANTHROPIC_API_KEY`.

**Input:**
- `repo_id` (required): Repository ID (must be previously analyzed)

**Output includes:**
- Overview and purpose
- Key features
- Technology stack
- Architecture style
- Main components
- Improvement suggestions

**Example:**
```json
{
  "repo_id": "github.com/owner/repo"
}
```

### `generate_ai_arch_analysis`

Generate AI-powered architecture analysis using Claude. Requires `ANTHROPIC_API_KEY`.

**Input:**
- `repo_id` (required): Repository ID (must be previously analyzed)

**Output includes:**
- Architecture pattern identification
- Layer analysis
- Data flow description
- Strengths and weaknesses
- Recommendations

**Example:**
```json
{
  "repo_id": "github.com/owner/repo"
}
```

### `ask`

**The main tool for intelligent queries.** Ask natural language questions about your code - the AI automatically finds relevant context and provides focused answers. Requires `ANTHROPIC_API_KEY`.

**Input:**
- `query` (required): Your question in natural language
- `repo_ids` (optional): Limit to specific repositories

**Features:**
- Automatically extracts only relevant code context
- Classifies query type (search, explain, architecture, compare)
- Returns focused answers with source references
- Minimizes token usage by sending only necessary data

**Examples:**
```json
{"query": "How does authentication work?"}
{"query": "Where is the main entry point?"}
{"query": "What does the Handler interface do?"}
{"query": "Explain the data flow in the API layer"}
{"query": "What functions handle user input?", "repo_ids": ["github.com/owner/repo"]}
```

## What Gets Analyzed

For Go files:
- Package documentation
- Imports (with aliases)
- Exported and unexported functions (with signatures, parameters, returns)
- Types (structs, interfaces, aliases) with fields and methods
- Constants and variables
- Cyclomatic complexity

For other files:
- Basic file metadata
- Purpose detection based on filename

## Architecture Context

The server generates an architecture overview including:
- Module structure
- Entry points (main packages)
- Build system detection
- File organization

## Development

```bash
# Run tests
go test ./...

# Build
go build ./cmd/mcp-server

# Run locally
./mcp-server -storage ./data/contexts

# Build Docker image
docker build -t mcp-repo-context:latest .

# Test Docker image
echo '{"jsonrpc":"2.0","id":1,"method":"tools/list"}' | docker run --rm -i mcp-repo-context:latest
```

## License

MIT
