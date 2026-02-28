# Section 05: Health & Operations

## Overview

Operational endpoints (health check, status), rate limiting, structured JSON logging, and graceful shutdown orchestration. These make the service production-ready for deployment.

## Dependencies

- Section 01 (HTTP foundation, APIServer, middleware)
- Section 03 (job queue, for queue depth in status)
- Internal: `internal/logging` (extend with JSON mode)

## Health Endpoint

```
GET /health
```

Returns 200 with `{"status": "ok"}`. No middleware timeout. No rate limiting. Used by load balancers and Docker health checks.

This endpoint is mounted outside the `/api/v1/` group so it's at the root level.

## Status Endpoint

```
GET /api/v1/status
```

Returns operational statistics:

```json
{
  "data": {
    "status": "ok",
    "repos_analyzed": 15,
    "orgs": 3,
    "queue_depth": 2,
    "queue_running": 1,
    "uptime": "2h15m30s",
    "version": "1.0.0"
  },
  "meta": { "request_id": "..." }
}
```

Implementation:
- `repos_analyzed`: count from manager's ListRepos
- `orgs`: count from orgManager's ListOrgs
- `queue_depth`: count of pending jobs from job queue
- `queue_running`: count of running jobs from job queue
- `uptime`: `time.Since(s.startTime)`
- `version`: build-time constant or hardcoded

## Rate Limiting

In-memory token bucket rate limiter, applied per-IP (or per-org when auth is added later).

### Implementation

Use a simple token bucket with `sync.Map` for concurrent access:
- Key: client IP (from `r.RemoteAddr` after RealIP middleware)
- Bucket: tracks token count and last refill time
- Rate: `config.RateLimitPerMinute` tokens per minute (default 60)
- Burst: allow up to rate tokens (1 minute worth)

### Middleware

```
func (s *APIServer) rateLimitMiddleware(next http.Handler) http.Handler
```

1. Extract client IP.
2. Get or create token bucket for this IP.
3. Try to consume 1 token.
4. If bucket empty: return 429 with `Retry-After` header (seconds until next refill).
5. If token available: call `next.ServeHTTP(w, r)`.

### Cleanup

Run a goroutine every 10 minutes that removes buckets not accessed in the last 10 minutes. Prevents unbounded memory growth.

### Application

Apply rate limiting middleware to the `/api/v1/` route group. Exclude `/health` (not rate limited).

## Structured JSON Logging

Extend the existing `internal/logging` package with a JSON output mode for HTTP request logging.

### Log Format

Each HTTP request produces one JSON log entry:

```json
{
  "timestamp": "2024-01-15T10:30:00Z",
  "level": "info",
  "message": "http_request",
  "method": "GET",
  "path": "/api/v1/repos",
  "status": 200,
  "duration_ms": 42,
  "request_id": "abc-123",
  "remote_addr": "10.0.0.1",
  "user_agent": "curl/8.0",
  "bytes_written": 1234
}
```

For errors (status >= 500):
- level: "error"
- Include `error` field with message

### Middleware Implementation

The JSON logger middleware (registered in section 01's middleware stack):
1. Wrap `http.ResponseWriter` to capture status code and bytes written.
2. Record start time.
3. Call next handler.
4. After response, compute duration and emit JSON log entry.

Use `chi/middleware.WrapResponseWriter` for response capturing.

## Graceful Shutdown

Orchestrate clean shutdown of all components on SIGINT/SIGTERM.

### Shutdown Sequence

1. **Signal received** — catch SIGINT/SIGTERM, cancel the root context.
2. **Stop accepting new connections** — `httpServer.Shutdown()` stops new connections but lets in-flight requests complete.
3. **Stop job queue** — `jobQueue.Stop()` stops workers from picking new jobs, waits for current jobs to finish.
4. **Drain timeout** — Wait up to 30s for in-flight HTTP requests to complete.
5. **Close databases** — Close both the main storage DB and the jobs DB.
6. **Exit** — Clean exit with code 0.

### Implementation in main.go

```
// Pseudocode
ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
defer cancel()

// Start server (blocks until context cancelled)
err := apiServer.Start(ctx)
```

The `Start` method handles the graceful shutdown internally when context is cancelled.

## Tests

### `internal/api/health_test.go`

**Test: GET /health returns 200**
- GET /health via httptest
- Assert 200 with `{"status": "ok"}`

**Test: GET /api/v1/status returns counts**
- Setup mock manager returning 2 repos, mock org manager returning 1 org
- GET /api/v1/status
- Assert repos_analyzed=2, orgs=1

**Test: Rate limiter allows within limit**
- Configure rate limit to 60/min
- Send 60 requests from same IP
- Assert all return 200

**Test: Rate limiter blocks over limit**
- Configure rate limit to 5/min (small for testing)
- Send 6 requests
- Assert 6th returns 429 with Retry-After header

**Test: Graceful shutdown drains requests**
- Start server on random port
- Begin a slow request (handler that sleeps 2s)
- Cancel context (trigger shutdown)
- Assert slow request completes (not cut off)
- Assert new requests after shutdown are rejected

**Test: Structured JSON log format**
- Send request through middleware chain
- Capture log output (buffer)
- Assert output is valid JSON with expected fields (timestamp, level, method, path, status, duration_ms, request_id)

## File Inventory

| File | Purpose |
|------|---------|
| `internal/api/health.go` | Health and status endpoint handlers |
| `internal/api/ratelimit.go` | Token bucket rate limiter middleware |
| `internal/api/middleware.go` | Updated with JSON log middleware (extend from section 01) |
| `internal/api/health_test.go` | All health/ops tests |

## Acceptance Criteria

1. `/health` returns 200 with `{"status": "ok"}`
2. `/api/v1/status` returns correct operational counts
3. Rate limiter allows requests within limit
4. Rate limiter returns 429 with Retry-After when exceeded
5. Rate limiter cleans up stale buckets
6. Graceful shutdown drains in-flight requests
7. Structured JSON logs emitted for each request
8. All 6 tests pass
