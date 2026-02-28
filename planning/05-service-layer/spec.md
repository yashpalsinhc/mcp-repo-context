# Spec: Service Layer & REST API

## Purpose

Make the MCP server deployable as an org-wide service. Currently it only runs as a local MCP server for Claude Code. This split wraps the core tools as a REST API with multi-tenant storage and webhook integration.

## Background

See `planning/deep_project_interview.md`. The user wants the server to be usable by "anyone in the world for their organization." The deployment model is both local (current) + hosted (new). Auth is deferred — focus on the core service plumbing first.

## Scope

### 1. REST API Wrapper
**Required:**
- HTTP server (Go net/http or chi router) exposing core MCP tools as REST endpoints
- Map MCP tool inputs/outputs to HTTP request/response JSON
- Key endpoints:
  - `POST /repos/analyze` — analyze a repo (by URL or local path)
  - `GET /repos/{id}/search` — search_context
  - `GET /repos/{id}/functions/{name}` — get_function_context
  - `GET /repos/{id}/callers/{name}` — get_callers
  - `GET /repos/{id}/architecture` — get_context(scope=architecture)
  - `POST /repos/compare` — compare_repos
  - `GET /orgs/{id}/dependency-graph` — get_dependency_graph (from split 02)
  - `POST /orgs/{id}/trace-flow` — trace_api_flow (from split 03)
  - `GET /orgs/{id}/service-map` — get_service_map (from split 03)
  - `POST /ask` — AI-powered queries
- OpenAPI/Swagger documentation

### 2. Multi-Tenant Storage
**Required:**
- Org isolation: each org's data is separate
- Options to evaluate during /deep-plan:
  - Separate SQLite DBs per org (simple isolation)
  - Single DB with org_id column (simpler ops)
- Org CRUD: create org, list orgs, add/remove repos from org
- Data retention/cleanup policies

### 3. GitHub Webhook Integration
**Required:**
- Webhook endpoint: `POST /webhooks/github`
- Handle push events: auto-trigger `analyze_repo` for changed repos
- Handle pull_request events: auto-trigger `get_pr_context`
- Webhook signature verification (HMAC-SHA256)
- Rate limiting to prevent webhook storms

### 4. GitLab Webhook Integration
**Required:**
- Webhook endpoint: `POST /webhooks/gitlab`
- Handle push events and merge_request events
- Webhook token verification

### 5. Manual API Trigger
**Required:**
- `POST /repos/analyze` with repo URL + optional branch
- Async analysis with status polling: `GET /repos/{id}/status`
- Analysis queue for concurrent requests

### 6. Health & Operations
**Required:**
- `GET /health` — health check
- `GET /status` — server status (analyzed repos, queue depth)
- Rate limiting per org (configurable)
- Structured logging (JSON)
- Graceful shutdown

## Dependencies

- **01-core-bug-fixes:** Core tools must be reliable before exposing as API
- **02-dependency-graph:** Dependency graph endpoints

## Provides to Other Splits

- **06-agent-recipes:** REST API enables external agents to use recipes

## Key Technical Decisions (Research During /deep-plan)

- HTTP framework: net/http + chi vs gin vs echo
- Storage isolation model: separate DBs vs shared DB
- Queue system for async analysis: in-memory vs external (Redis, etc.)
- Deployment: Docker image, binary, or both
- Auth deferred to future split (but design API to support it)

## Research from Interview

- **Multi-tenant patterns (2025):** Bridge model recommended — shared general resources + dedicated sensitive components
- **Scale target:** 50-200 repos per org, medium-sized repos
- **Deployment:** Docker image for self-hosted, potential cloud hosting later

## Testing Strategy

- Integration tests for all REST endpoints
- Load testing: concurrent analysis requests
- Webhook simulation: GitHub/GitLab payload replay
- Multi-tenant isolation verification
