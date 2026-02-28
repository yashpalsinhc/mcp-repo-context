# Implementation Plan: Service Layer & REST API

## Overview

Wrap the existing MCP server functionality as a REST API using chi router. Add a SQLite-backed job queue for async analysis, GitHub/GitLab webhook integration, and operational endpoints. The server supports dual mode (mutually exclusive): MCP (stdio) for local Claude Code usage, HTTP for shared service deployment.

**Auth:** Deferred to future split. API designed with middleware hooks for auth. For now, intended for internal/network-isolated deployment.

**API versioning:** v1 is the only version. Versioning strategy defined when a breaking change is needed.

## Current Architecture

### What Exists
- **MCP Server** (`internal/mcp/server.go`): JSON-RPC 2.0 over stdio, ~30 tool handlers
- **Manager** (`internal/orchestrator/`): ~55 methods covering analysis, search, AI, comparison
- **Storage** (`internal/storage/`): SQLite with repos, files, functions, call graph, side effects, concepts
- **Org Manager** (`internal/org/`): Full CRUD, config inheritance, concurrent analysis
- **Docker**: Multi-stage build, data volumes, non-root user
- **No HTTP code**: No router, no HTTP handlers, no webhook handling

### What's Missing
1. HTTP server and router setup
2. REST endpoint handlers
3. Job queue for async operations
4. Webhook handlers (GitHub, GitLab)
5. Rate limiting, health checks, graceful shutdown
6. Dual-mode entry point (MCP vs HTTP)

## Section-by-Section Plan

### Section 1: HTTP Server Foundation & Router

**Goal:** Set up chi router, middleware stack, and the HTTP server entry point.

**New package: `internal/api/`**

**Dependencies:** `github.com/go-chi/chi/v5`, `github.com/go-chi/chi/v5/middleware`

**Server struct:**

```
type APIServer struct {
    router     chi.Router
    manager    orchestrator.Manager
    comparer   comparison.Comparer
    orgManager org.Manager
    jobQueue   *JobQueue
    config     *APIConfig
    logger     *logging.Logger
}
```

**APIConfig:**
- ListenAddr (string, default ":8080")
- ReadTimeout, WriteTimeout, IdleTimeout (time.Duration)
- RateLimitPerOrg (int, requests per minute, default 60)
- MaxRequestBodySize (int64, default 10MB)

**Middleware stack (in order):**
1. RequestID (chi middleware)
2. RealIP
3. Structured JSON logger
4. Recoverer (panic recovery)
5. MaxBodySize
6. CORS (allow-all for now, tighten with auth)
7. Rate limiter (per org, in-memory token bucket)

**Timeout per route group:** Apply chi timeout middleware at route group level, not globally. Regular endpoints: 30s. Analysis endpoints are async (202), so they don't need long timeouts.

**Start method:**

```
func (s *APIServer) Start(ctx context.Context) error
```

Creates `http.Server` with the router, starts listening. Handles graceful shutdown on context cancellation (30s drain period).

**Entry point update in `cmd/mcp-server/main.go`:**

Add `--mode` flag: `mcp` (default, current behavior) or `http` (REST API).
Add `--listen` flag for HTTP address (default `:8080`).

Modes are mutually exclusive. MCP uses stdio, HTTP uses TCP. Run separate processes for both if needed.

### Section 2: Core REST Endpoints

**Goal:** Map core MCP tools to REST endpoints with JSON request/response.

**Repo ID encoding:** Repo IDs contain slashes (e.g., `github.com/org/repo`). Clients must URL-encode (percent-encode) the repo ID in path parameters. Chi automatically decodes percent-encoded path segments. Example: `GET /api/v1/repos/github.com%2Forg%2Frepo/search`. Document this in API docs.

**File path encoding:** File paths also contain slashes. For `/refresh/{path}`, use query parameter instead: `POST /api/v1/repos/{id}/refresh?path=pkg/handlers/user.go`. Avoids nested slash ambiguity.

**Route structure:**

```
POST   /api/v1/repos/analyze              -> AnalyzeRepo (async, returns job ID)
GET    /api/v1/repos                       -> ListRepos (paginated)
GET    /api/v1/repos/{id}                  -> GetContext (full repo context)
GET    /api/v1/repos/{id}/architecture     -> GetContext (architecture scope)
DELETE /api/v1/repos/{id}                  -> DeleteRepoContext
GET    /api/v1/repos/{id}/files            -> GetFileContext (query: path=...)
GET    /api/v1/repos/{id}/search           -> SearchFunctions (query: q=..., limit, offset)
GET    /api/v1/repos/{id}/functions/{name} -> GetFunctionContext
GET    /api/v1/repos/{id}/callers/{name}   -> GetCallers
GET    /api/v1/repos/{id}/concepts/{name}  -> SearchByConcept
GET    /api/v1/repos/{id}/side-effects/{type} -> SearchBySideEffect
POST   /api/v1/repos/{id}/refresh          -> RefreshChangedFiles
POST   /api/v1/repos/{id}/refresh-file     -> RefreshFile (query: path=...)
POST   /api/v1/repos/compare              -> CompareRepos
POST   /api/v1/repos/{id}/pr-context      -> GetPRContext

POST   /api/v1/orgs                        -> RegisterOrg
GET    /api/v1/orgs                        -> ListOrgs (paginated)
GET    /api/v1/orgs/{id}                   -> GetOrg
DELETE /api/v1/orgs/{id}                   -> DeleteOrg
POST   /api/v1/orgs/{id}/repos            -> AddRepos
DELETE /api/v1/orgs/{id}/repos/{repoId}   -> RemoveRepo
POST   /api/v1/orgs/{id}/analyze          -> AnalyzeOrg (async, returns job ID)

POST   /api/v1/ask                         -> Ask (AI-powered query)
POST   /api/v1/search                      -> SmartQuery (across repos)

GET    /api/v1/jobs/{id}                   -> GetJob status/result
GET    /api/v1/jobs                        -> ListJobs (paginated, filter by org_id, status)
```

**Pagination:** All list/search endpoints accept `limit` (default 50, max 200) and `offset` query parameters. Response includes pagination metadata.

**Response format (standard envelope):**

```json
{
  "data": { ... },
  "meta": { "request_id": "...", "duration_ms": 123, "total": 100, "limit": 50, "offset": 0 },
  "error": null
}
```

**Handler pattern:**

Each handler:
1. Parse request (URL params via chi.URLParam — percent-decoded, query params, JSON body)
2. Validate input
3. Call Manager/Comparer/OrgManager method
4. Map result to JSON response struct
5. Return with appropriate HTTP status

Status code mapping:
- 200: successful GET/POST
- 201: successful creation
- 202: accepted for async processing
- 400: bad request
- 404: not found
- 429: rate limited (include Retry-After header)
- 500: internal error (include request_id, no stack trace)

### Section 3: Job Queue

**Goal:** SQLite-backed job queue for async analysis operations. Uses a separate SQLite database file to avoid contention with the main storage.

**New package: `internal/queue/`**

**Separate DB:** The job queue uses its own SQLite file (`jobs.db`) to avoid single-writer contention with the main storage database during concurrent analysis operations.

**Schema:**

```sql
CREATE TABLE jobs (
    id TEXT PRIMARY KEY,
    type TEXT NOT NULL,
    org_id TEXT,
    repo_id TEXT,
    payload TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending',
    result TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    started_at DATETIME,
    completed_at DATETIME,
    error TEXT,
    retries INTEGER DEFAULT 0,
    max_retries INTEGER DEFAULT 3
);

CREATE INDEX idx_jobs_status ON jobs(status, created_at);
CREATE INDEX idx_jobs_org_repo ON jobs(org_id, repo_id, created_at);
```

**Atomic job dequeue (critical):**

Workers claim jobs using an atomic UPDATE-with-subquery:
```sql
UPDATE jobs SET status = 'running', started_at = CURRENT_TIMESTAMP
WHERE id = (SELECT id FROM jobs WHERE status = 'pending' ORDER BY created_at LIMIT 1);
```
Execute in an IMMEDIATE transaction. After UPDATE, check `changes()` — if 0, no job available, sleep and retry. This prevents two workers from claiming the same job.

**JobQueue methods:**

- `Enqueue(ctx, jobType, orgID, repoID, payload) (jobID, error)` — insert pending job
- `GetJob(ctx, jobID) (*Job, error)` — get status/result
- `ListJobs(ctx, orgID, status, limit, offset) ([]Job, int, error)` — paginated list
- `Start(ctx)` — launch worker goroutines
- `Stop()` — graceful shutdown

**Workers (default 3):**
Each worker loops: claim job -> execute -> mark completed/failed -> repeat. On failure, increment retries. If retries < max_retries, mark back as "pending" with exponential backoff (set created_at to future time).

**Stale job recovery:** A sweeper goroutine runs every 5 minutes. Reclaims jobs stuck in "running" for >30 minutes by setting them back to "pending" (counts as a retry).

**Job cleanup:** Delete completed/failed jobs older than 7 days.

**Webhook deduplication:** Before enqueuing, check if a job for the same (repo_id, type) was created within the last 5 minutes. If so, skip (return existing job ID).

### Section 4: Webhook Handlers

**Goal:** GitHub and GitLab webhook endpoints for auto-analysis.

**GitHub webhook:**

```
POST /api/v1/webhooks/github
```

**Verification:**
1. Buffer request body fully (`io.ReadAll`)
2. Compute HMAC-SHA256 of body using configured secret
3. Compare with `X-Hub-Signature-256` header using `hmac.Equal()` (constant-time)
4. Parse JSON from buffered body

**Events handled:**
- `push`: Extract repo URL. Filter: only default branch (configurable). Skip if only `.md` files changed. Enqueue analyze_repo job.
- `pull_request` (opened/synchronize): Extract PR URL, enqueue PR context job.

**Configuration:**
- `GITHUB_WEBHOOK_SECRET` environment variable
- `WEBHOOK_BRANCHES` (comma-separated, default: default branch only)
- If secret not set, webhook endpoint returns 503

**GitLab webhook:**

```
POST /api/v1/webhooks/gitlab
```

**Verification:** Compare `X-Gitlab-Token` header with configured secret using `subtle.ConstantTimeCompare`.

**Events handled:**
- Push events: Extract repo URL, filter by branch, enqueue analyze_repo job
- Merge request events: Extract MR details, enqueue analysis

**Rate limiting:** Webhook requests use the per-repo dedup check in the job queue (same repo+type within 5 minutes = skip).

### Section 5: Health & Operations

**Goal:** Operational endpoints, structured logging, rate limiting, graceful shutdown.

**Endpoints:**

```
GET /health           -> { "status": "ok" }
GET /api/v1/status    -> { "repos_analyzed": N, "orgs": N, "queue_depth": N, "uptime": "..." }
```

**Structured JSON logging:**

Each log entry: timestamp, level, message, request_id, method, path, status, duration, org_id, repo_id. Use existing `internal/logging` package, extend with JSON output mode.

**Rate limiting:**

Per-org token bucket (in-memory):
- Default: 60 requests/minute per org
- Return 429 with `Retry-After` header
- State lost on restart (acceptable for API rate limiting; webhook dedup uses DB)

**Graceful shutdown:**

On SIGINT/SIGTERM:
1. Stop accepting new HTTP connections
2. Stop job queue (finish running jobs, don't start new)
3. Wait up to 30s for in-flight requests
4. Close databases
5. Exit

### Section 6: Docker & Deployment Updates

**Goal:** Update Docker for dual-mode support.

**Dockerfile:** Expose port 8080. Default to MCP mode.

**docker-compose.http.yml** (override):
```yaml
services:
  mcp-repo-context:
    command: ["--mode", "http", "--listen", ":8080"]
    ports: ["8080:8080"]
    environment:
      - GITHUB_WEBHOOK_SECRET=${GITHUB_WEBHOOK_SECRET}
    healthcheck:
      test: ["CMD", "wget", "-q", "--spider", "http://localhost:8080/health"]
      interval: 30s
```

### Section 7: Integration Tests

**Test scenarios:**

Unit tests:
1. HMAC verification with valid/invalid signatures
2. Rate limiter token bucket behavior
3. Job dequeue atomicity (concurrent goroutines)
4. Response envelope formatting
5. Repo ID URL encoding/decoding
6. Pagination parameter parsing and bounds

REST API integration:
1. POST /repos/analyze returns 202 with job ID
2. GET /jobs/{id} returns job status transitions
3. GET /repos lists analyzed repos with pagination
4. GET /repos/{id}/search returns results
5. Error responses have correct format and status codes
6. Rate limiting returns 429

Job queue:
1. Enqueue -> worker processes -> completed
2. Failed job retried up to max_retries
3. Stale job reclaimed by sweeper
4. Concurrent workers claim different jobs
5. Cleanup removes old jobs
6. Dedup prevents duplicate analysis within 5 min

Webhooks:
1. GitHub push triggers analysis job
2. Invalid HMAC returns 401
3. Non-default branch push skipped
4. GitLab push triggers analysis
5. Invalid GitLab token returns 401

Health:
1. /health returns 200
2. /status returns correct counts
3. Graceful shutdown drains requests

## Error Handling

- Invalid JSON body: 400 with parse error
- Unknown repo/org: 404 with resource type and ID
- Analysis failure: job marked failed with error
- Webhook verification failure: 401 (no detail — security)
- Rate limit exceeded: 429 with Retry-After header
- Internal error: 500 with request_id (no stack trace)

## Performance Considerations

- Chi router: zero-allocation routing
- Separate SQLite for jobs: no contention with main storage
- In-memory rate limiter: fast, acceptable state loss
- Async analysis: non-blocking API responses
- Pagination prevents unbounded result sets
- WAL mode on both SQLite databases
