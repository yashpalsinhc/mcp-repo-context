#!/bin/bash
# Switch MCP server to Docker mode (with volume mounts for local directory access)

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"
MCP_CONFIG="$HOME/.claude/.mcp.json"

echo "Stopping existing container if running..."
docker stop mcp-repo-context 2>/dev/null || true
docker rm mcp-repo-context 2>/dev/null || true

echo "Building and starting Docker container..."
cd "$PROJECT_DIR"
docker-compose up -d --build

echo "Updating MCP config to use Docker..."
cat > "$MCP_CONFIG" << EOF
{
  "mcpServers": {
    "repo-context": {
      "command": "docker",
      "args": ["exec", "-i", "mcp-repo-context", "/usr/local/bin/mcp-server"]
    }
  }
}
EOF

echo "Done! Please restart Claude Code for changes to take effect."
echo "The MCP server is now running in Docker with your home directory mounted."
