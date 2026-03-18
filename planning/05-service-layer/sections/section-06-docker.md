# Section 06: Docker & Deployment Updates

## Overview

Update the Dockerfile and add docker-compose configuration for HTTP mode. The existing Docker setup supports MCP mode (stdio). This section adds HTTP mode support with port exposure, health checks, and environment variable configuration.

## Dependencies

- Section 01 (dual-mode entry point with --mode flag)
- Section 05 (health endpoint for Docker health check)
- Existing: `Dockerfile`, `docker-compose.yml`

## Dockerfile Updates

Modify the existing multi-stage Dockerfile:

1. **Add EXPOSE directive** — `EXPOSE 8080` after the binary copy. This documents the HTTP port but doesn't publish it.

2. **Default mode remains MCP** — The CMD/ENTRYPOINT stays the same (MCP mode over stdio). HTTP mode is activated via command override.

3. **No other changes needed** — The existing build stages, non-root user, data volumes, and binary compilation all work for both modes.

## docker-compose.yml (single file with profiles)

One `docker-compose.yml` contains all modes. HTTP and Postgres are optional profiles.

- **Stdio (default):** `docker compose up` — MCP over stdio for Claude Code.
- **HTTP mode:** `docker compose --profile http up -d` — service `mcp-repo-context-http` with port 8080, health check, webhook env vars.
- **With Postgres:** `docker compose --profile with-postgres up -d` — adds optional PostgreSQL for index storage.

No separate override file; use `--profile http` or `--profile with-postgres` as needed.

## Environment Variables

Document the full list of environment variables for HTTP mode:

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `ANTHROPIC_API_KEY` | No | - | For AI-powered features (ask, summary) |
| `GITHUB_TOKEN` | No | - | For cloning private repos |
| `GITHUB_WEBHOOK_SECRET` | No | - | HMAC secret for GitHub webhooks |
| `GITLAB_WEBHOOK_SECRET` | No | - | Token for GitLab webhooks |

## Health Check

The Docker health check uses `wget` (available in Alpine) to hit `/health`. This verifies:
- The HTTP server is running and accepting connections
- The router is functioning
- The process hasn't crashed

If the health check fails 3 times, Docker marks the container as unhealthy and can restart it (with `restart: unless-stopped`).

## Tests

### Manual Verification

These tests are manual (not automated) since they require Docker:

**Test: Docker build succeeds**
```bash
docker build -t mcp-repo-context .
# Assert: build completes without error
```

**Test: HTTP mode starts and responds**
```bash
docker compose --profile http up -d
curl http://localhost:8080/health
# Assert: {"status": "ok"}
docker compose --profile http down
```

**Test: MCP mode still works**
```bash
docker compose up -d
# Send JSON-RPC request via docker exec
echo '{"jsonrpc":"2.0","method":"list_repos","id":1}' | docker exec -i mcp-repo-context ./mcp-repo-context
# Assert: valid JSON-RPC response
docker compose down
```

**Test: Health check reports healthy**
```bash
docker compose --profile http up -d
sleep 35  # Wait for health check interval
docker inspect --format='{{.State.Health.Status}}' mcp-repo-context-mcp-repo-context-http-1
# Assert: "healthy"
docker compose --profile http down
```

## File Inventory

| File | Purpose |
|------|---------|
| `Dockerfile` | Updated with EXPOSE 8080 |
| `docker-compose.yml` | Single compose: stdio default, `--profile http` for HTTP, `--profile with-postgres` for Postgres |

## Acceptance Criteria

1. `docker build .` succeeds
2. HTTP mode starts and `/health` returns 200
3. MCP mode (default) continues to work
4. Health check reports container as healthy after start_period
5. Environment variables are properly passed through
6. Data volume persists between container restarts
