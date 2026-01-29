#!/bin/bash
# Usage: ./scripts/analyze.sh https://github.com/owner/repo [branch]

REPO_URL="$1"
BRANCH="${2:-}"

if [ -z "$REPO_URL" ]; then
  echo "Usage: $0 <repo_url> [branch]"
  exit 1
fi

BRANCH_ARG=""
if [ -n "$BRANCH" ]; then
  BRANCH_ARG=",\"branch\":\"$BRANCH\""
fi

cat << EOF | docker exec -i mcp-repo-context /usr/local/bin/mcp-server 2>/dev/null | tail -1 | jq -r '.result.content[0].text'
{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"cli","version":"1.0"}}}
{"jsonrpc":"2.0","id":2,"method":"initialized","params":{}}
{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"analyze_repo","arguments":{"repo_url":"${REPO_URL}"${BRANCH_ARG}}}}
EOF
