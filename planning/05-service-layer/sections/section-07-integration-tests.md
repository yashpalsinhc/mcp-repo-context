# Section 07: Integration Tests

## Overview

End-to-end integration tests that exercise the full API stack: HTTP server, handlers, job queue, webhooks, and operational endpoints working together. These tests start a real HTTP server (on a random port) with real SQLite databases (in temp dirs) and verify complete workflows.

## Dependencies

- All previous sections (01-06)
- Test utilities: `net/http/httptest`, `testing`, `os`

## Test Infrastructure

### Test Helper

Create a test helper that sets up the full stack:

```
func setupTestServer(t *testing.T) (*httptest.Server, *APIServer, func())
```

This helper:
1. Creates temp directories for data and jobs DB.
2. Initializes real SQLite storage.
3. Creates real Manager and OrgManager.
4. Creates real JobQueue (1 worker for deterministic testing).
5. Creates APIServer with all components wired together.
6. Starts httptest.Server.
7. Returns the server, APIServer reference, and a cleanup function.

### Test Client

Create a thin test client wrapper:

```
type testClient struct {
    baseURL string
    client  *http.Client
}
```

Methods:
- `get(path string) (*http.Response, error)`
- `post(path string, body interface{}) (*http.Response, error)`
- `delete(path string) (*http.Response, error)`
- `parseResponse(resp) (*APIResponse, error)` — unmarshal envelope

## Integration Test Scenarios

### `internal/integration/api_test.go`

Use build tag `//go:build integration` to separate from unit tests. Run with `go test -tags integration ./internal/integration/...`.

**Test: Full analysis flow — analyze via API, search, get function**

1. POST /api/v1/repos/analyze with a small test repo (use a local fixture or a known small public repo).
2. Assert 202 with job_id.
3. Poll GET /api/v1/jobs/{job_id} until status="completed" (timeout after 60s).
4. GET /api/v1/repos — assert the analyzed repo appears in the list.
5. GET /api/v1/repos/{id}/search?q=main — assert search results returned.
6. If results include a function, GET /api/v1/repos/{id}/functions/{name} — assert function context returned.

**Test: Org flow — create, list, get**

1. POST /api/v1/orgs with `{"id": "test-org", "repos": []}`.
2. Assert 201.
3. GET /api/v1/orgs — assert "test-org" in list.
4. GET /api/v1/orgs/test-org — assert org details returned.
5. DELETE /api/v1/orgs/test-org — assert success.
6. GET /api/v1/orgs — assert "test-org" gone.

**Test: Webhook flow — GitHub push triggers analysis**

1. Configure test server with a known webhook secret.
2. Create a push payload for a test repo.
3. Compute valid HMAC-SHA256 signature.
4. POST /api/v1/webhooks/github with payload and signature.
5. Assert 200.
6. Poll GET /api/v1/jobs — assert a new analyze_repo job exists.

**Test: Error handling end-to-end**

1. GET /api/v1/repos/nonexistent%2Frepo — assert 404 with proper error envelope.
2. POST /api/v1/repos/analyze with `{}` (empty URL) — assert 400.
3. POST /api/v1/repos/analyze with invalid JSON — assert 400.
4. GET /api/v1/repos/test%2Frepo/functions/nonexistent — assert 404.

**Test: Pagination end-to-end**

1. Create 5 test entities (orgs or use pre-analyzed repos).
2. GET /api/v1/orgs?limit=2&offset=0 — assert 2 items, meta.total=5.
3. GET /api/v1/orgs?limit=2&offset=2 — assert 2 items (page 2).
4. GET /api/v1/orgs?limit=2&offset=4 — assert 1 item (last page).

**Test: Rate limiting end-to-end**

1. Configure test server with rate limit of 5/min.
2. Send 6 requests to any endpoint.
3. Assert first 5 return 200.
4. Assert 6th returns 429 with Retry-After header.

**Test: Health and status**

1. GET /health — assert 200 with `{"status": "ok"}`.
2. GET /api/v1/status — assert it returns valid stats (repos_analyzed >= 0, uptime present).

**Test: Job lifecycle**

1. POST /api/v1/repos/analyze to create a job.
2. GET /api/v1/jobs/{id} — assert status transitions: pending -> running -> completed/failed.
3. GET /api/v1/jobs — assert job appears in list.
4. GET /api/v1/jobs?status=pending — assert filtering works.

## Unit Test Additions

These are not integration tests but additional unit tests to cover gaps:

### `internal/api/response_test.go`

**Test: writeJSON produces correct envelope**
- Call writeJSON with test data
- Assert JSON output has data, meta, error fields

**Test: writeError produces correct error envelope**
- Call writeError with 404, "not_found", "repo not found"
- Assert JSON output has error field with code and message

### `internal/api/pagination_test.go`

**Test: parsePagination with various inputs**
- No params -> limit=50, offset=0
- limit=999 -> capped at 200
- limit=-1 -> set to 50
- offset=-5 -> set to 0
- limit=abc -> set to 50 (non-numeric)

## File Inventory

| File | Purpose |
|------|---------|
| `internal/integration/api_test.go` | All integration tests (build tag: integration) |
| `internal/integration/helpers_test.go` | Test helper: setupTestServer, testClient |
| `internal/api/response_test.go` | Unit tests for response envelope |
| `internal/api/pagination_test.go` | Unit tests for pagination parsing |

## Running Tests

```bash
# Unit tests only (fast)
go test ./internal/api/... ./internal/queue/...

# Integration tests (slower, requires filesystem)
go test -tags integration ./internal/integration/...

# All tests
go test -tags integration ./...
```

## Acceptance Criteria

1. Full analysis flow works end-to-end (analyze -> search -> get)
2. Org CRUD flow works end-to-end
3. Webhook triggers analysis job end-to-end
4. Error responses have correct status codes and envelope format
5. Pagination works across pages
6. Rate limiting enforced end-to-end
7. Health and status endpoints return correct data
8. Job lifecycle is observable via API
9. All integration tests pass with `go test -tags integration`
10. All unit tests continue to pass
