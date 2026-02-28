# Section 01: HTTP Server Foundation & Router

## Overview

Set up the chi router, middleware stack, APIServer struct, and dual-mode entry point. This is the foundation for all REST endpoints, providing request routing, structured logging, panic recovery, request ID tracking, body size limits, and graceful shutdown.

## Dependencies

- External: `github.com/go-chi/chi/v5`, `github.com/go-chi/chi/v5/middleware`
- Internal: `internal/orchestrator` (Manager interface), `internal/org` (Manager interface), `internal/logging`

## New Package

`internal/api/` — contains all HTTP server code.

## APIConfig Struct

Define `APIConfig` with fields:
- `ListenAddr` (string, default `:8080`)
- `ReadTimeout` (time.Duration, default 15s)
- `WriteTimeout` (time.Duration, default 15s)
- `IdleTimeout` (time.Duration, default 60s)
- `RateLimitPerMinute` (int, default 60)
- `MaxRequestBodySize` (int64, default 10MB)
- `GithubWebhookSecret` (string, from env)
- `GitlabWebhookSecret` (string, from env)

## APIServer Struct

```
type APIServer struct {
    router     chi.Router
    manager    orchestrator.Manager
    orgManager org.Manager
    jobQueue   *queue.JobQueue   // from internal/queue, added in section 3
    config     *APIConfig
    logger     *logging.Logger
    startTime  time.Time
}
```

Constructor: `NewAPIServer(manager, orgManager, config, logger) *APIServer` — creates the router, registers all middleware, mounts route groups.

## Middleware Stack (applied in order)

1. **RequestID** — `middleware.RequestID` from chi. Adds `X-Request-ID` header.
2. **RealIP** — `middleware.RealIP` from chi. Extracts real client IP from `X-Forwarded-For` / `X-Real-IP`.
3. **Structured JSON Logger** — Custom middleware. Logs each request as a JSON object with fields: `timestamp`, `level`, `method`, `path`, `status`, `duration_ms`, `request_id`, `remote_addr`. Uses the existing `internal/logging` package, extended with a JSON output mode.
4. **Recoverer** — `middleware.Recoverer` from chi. Catches panics, returns 500, logs stack trace.
5. **MaxBodySize** — Custom middleware using `http.MaxBytesReader` wrapping `r.Body`. Set to `config.MaxRequestBodySize`. Returns 413 if exceeded.
6. **CORS** — Allow all origins for now (tighten when auth is added). Set `Access-Control-Allow-Origin: *`, allow common methods and headers.

Rate limiting middleware is applied per route group in section 5 (Health & Ops).

## Route Registration

The constructor calls a private method `setupRoutes()` that mounts all route groups. Initially just registers placeholder routes. Actual handlers are added in section 2.

Route groups with chi timeout middleware:
- `/api/v1/` — 30s timeout for synchronous endpoints
- `/health` — no timeout (instant response)
- `/api/v1/webhooks/` — 30s timeout

Analysis endpoints (`POST /api/v1/repos/analyze`, `POST /api/v1/orgs/{id}/analyze`) are async (return 202 immediately), so the 30s timeout on the group is sufficient.

## Start Method

```
func (s *APIServer) Start(ctx context.Context) error
```

1. Create `http.Server` with the chi router, configured timeouts.
2. Start listening on `config.ListenAddr` in a goroutine.
3. Block on `ctx.Done()`.
4. On context cancellation, create a 30s shutdown context.
5. Call `httpServer.Shutdown(shutdownCtx)` to drain in-flight requests.
6. Return any error from shutdown.

## Entry Point Update

Modify `cmd/mcp-server/main.go`:

1. Add `--mode` flag: `mcp` (default) or `http`.
2. Add `--listen` flag: address for HTTP mode (default `:8080`).
3. Modes are mutually exclusive. MCP uses stdio (current behavior). HTTP creates APIServer and calls Start.
4. Both modes share the same Manager and storage initialization.

Decision flow:
- If `--mode=mcp` (or default): run existing MCP server over stdio.
- If `--mode=http`: create APIConfig from flags/env, create APIServer, call Start(ctx). Handle SIGINT/SIGTERM by cancelling the context.

## Tests

### `internal/api/server_test.go`

**Test: NewAPIServer creates server with routes**
- Create APIServer with mock manager (nil is fine for this test)
- Assert router is non-nil
- Walk the router, assert route count > 0

**Test: Start and Stop graceful shutdown**
- Start server on `:0` (random port)
- Send HTTP request, assert 200 from health endpoint
- Cancel context
- Assert server stops within 5s (no goroutine leak)

**Test: Middleware stack applies RequestID**
- Create APIServer
- Use `httptest.NewRecorder` and `httptest.NewRequest`
- Send request to `/health`
- Assert response has `X-Request-ID` header with non-empty value

**Test: Middleware panic recovery**
- Register a test route on the router that panics
- Send request via httptest
- Assert 500 response (not a crash/goroutine death)

**Test: MaxBodySize rejects large requests**
- Create APIServer with MaxRequestBodySize = 1024
- Send POST with body > 1024 bytes
- Assert 413 response

## File Inventory

| File | Purpose |
|------|---------|
| `internal/api/server.go` | APIServer struct, constructor, Start, middleware setup |
| `internal/api/config.go` | APIConfig struct and defaults |
| `internal/api/middleware.go` | Custom middleware (JSON logger, MaxBodySize, CORS) |
| `internal/api/routes.go` | Route registration (setupRoutes) |
| `internal/api/server_test.go` | All server tests |
| `cmd/mcp-server/main.go` | Updated entry point with --mode flag |

## Acceptance Criteria

1. `go build ./cmd/mcp-server/` succeeds
2. `--mode http` starts HTTP server that responds to `/health`
3. `--mode mcp` (default) continues to work as before
4. All 5 tests pass
5. Request ID header present on every response
6. Panics return 500, not crash
7. Oversized bodies return 413
