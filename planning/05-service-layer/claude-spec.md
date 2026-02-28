# Spec: Service Layer & REST API

## Problem
The MCP server only runs as a local stdio-based server for Claude Code. It cannot be deployed as a shared service, receive webhooks, or serve REST API requests. Need to wrap the existing Manager/Comparer functionality as a REST API with multi-tenant support and webhook integration.

## Requirements

1. **REST API server** using chi router, exposing all core MCP tools as HTTP endpoints with JSON request/response
2. **Multi-tenant storage** using single SQLite DB with org_id scoping (existing schema already supports this)
3. **SQLite-backed job queue** for async analysis — survives restarts, tracks job status
4. **GitHub webhook integration** — push events trigger analysis, PR events trigger context extraction, HMAC-SHA256 verification
5. **GitLab webhook integration** — push and merge_request events, token verification
6. **Health and operations** — /health, /status, rate limiting per org, structured JSON logging, graceful shutdown
7. **Dual mode** — server can run in MCP mode (stdio) or HTTP mode (REST API), selected by command-line flag
8. **OpenAPI documentation** — generated from handler annotations or separate spec file

## Design Decisions
- chi router: lightweight, idiomatic, net/http compatible
- Single SQLite DB: existing org_repos table provides multi-tenant scoping
- SQLite job queue: durable across restarts, no external dependencies
- Auth deferred: design API to support auth middleware later (org-scoped endpoints)
- Docker: add HTTP port exposure alongside existing MCP mode

## Dependencies
- 01-org-abstraction (org CRUD)
- 01-core-bug-fixes (reliable tools)
