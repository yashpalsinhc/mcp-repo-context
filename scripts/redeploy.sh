#!/bin/bash
# Redeploy MCP Repo Context Server
# Usage: ./scripts/redeploy.sh [--no-cache]

set -e

cd "$(dirname "$0")/.."

echo "=== MCP Repo Context Server Redeploy ==="
echo ""

# Check for --no-cache flag
NO_CACHE=""
if [ "$1" == "--no-cache" ]; then
    NO_CACHE="--no-cache"
    echo "Building with --no-cache"
fi

# Stop existing container
echo "[1/4] Stopping existing container..."
docker-compose down 2>/dev/null || true

# Build new image
echo "[2/4] Building new image..."
docker-compose build $NO_CACHE

# Start container
echo "[3/4] Starting container..."
docker-compose up -d

# Verify it's running
echo "[4/4] Verifying deployment..."
sleep 2

if docker ps | grep -q mcp-repo-context; then
    echo ""
    echo "=== Deployment Successful ==="
    echo ""
    echo "Container: mcp-repo-context"
    docker ps --filter name=mcp-repo-context --format "Status: {{.Status}}"
    echo ""
    echo "Available tools (12):"
    echo "  - analyze_repo          : Analyze GitHub repositories"
    echo "  - get_context           : Retrieve stored context"
    echo "  - list_repos            : List all analyzed repositories"
    echo "  - search_context        : Search functions, types, files"
    echo "  - compare_repos         : Compare multiple repositories"
    echo "  - find_duplicates       : Find duplicate code"
    echo "  - find_conflicts        : Find conflicting implementations"
    echo "  - find_gaps             : Find missing functionality"
    echo "  - generate_ai_summary   : AI-powered repository summary"
    echo "  - generate_ai_arch_analysis : AI-powered architecture analysis"
    echo "  - ask                   : Intelligent natural language queries"
    echo "  - refresh_ai_context    : Batch update repos with AI summaries"
    echo ""
    echo "AI Features: $([ -n "$ANTHROPIC_API_KEY" ] && echo "ENABLED" || echo "DISABLED (set ANTHROPIC_API_KEY)")"
else
    echo "ERROR: Container failed to start"
    docker-compose logs
    exit 1
fi
