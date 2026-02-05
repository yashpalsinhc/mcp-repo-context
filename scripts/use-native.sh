#!/bin/bash
# Switch MCP server to native mode (recommended for local directory analysis)

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"
MCP_CONFIG="$HOME/.claude/.mcp.json"
BINARY="$PROJECT_DIR/bin/mcp-server"

echo "Building MCP server binary..."
cd "$PROJECT_DIR"
go build -o bin/mcp-server ./cmd/mcp-server

echo "Updating MCP config to use native binary..."
cat > "$MCP_CONFIG" << EOF
{
  "mcpServers": {
    "repo-context": {
      "command": "$BINARY",
      "args": [],
      "env": {
        "MCP_STORAGE_PATH": "$PROJECT_DIR/data/contexts",
        "MCP_TEMP_DIR": "/tmp/mcp-repos"
      }
    }
  }
}
EOF

echo "Done! Please restart Claude Code for changes to take effect."
echo "The MCP server will now run natively with full filesystem access."
