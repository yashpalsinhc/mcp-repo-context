#!/bin/bash
# Script to run the MCP server in Docker with stdio transport
# This script is meant to be used as the command in Claude's MCP config

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"

# Ensure data directory exists
mkdir -p "$PROJECT_DIR/data/contexts"

# Run the container interactively for stdio transport
exec docker run --rm -i \
    -v "$PROJECT_DIR/data:/data" \
    -e GITHUB_TOKEN="${GITHUB_TOKEN}" \
    -e MCP_STORAGE_PATH=/data/contexts \
    -e MCP_TEMP_DIR=/tmp/mcp-repos \
    mcp-repo-context:latest
