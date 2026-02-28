# TDD Plan: Service Layer & REST API

## Section 1: HTTP Server Foundation

### Tests: `internal/api/server_test.go`

```
Test: NewAPIServer creates server with routes
- Create APIServer with mock manager
- Assert router is non-nil
- Assert route count > 0

Test: Start and Stop graceful shutdown
- Start server on random port
- Send request, assert 200
- Cancel context
- Assert server stops within 5s

Test: Middleware stack applies RequestID
- Send request to any endpoint
- Assert response has X-Request-ID header

Test: Middleware panic recovery
- Register route that panics
- Send request
- Assert 500 response (not crash)

Test: MaxBodySize rejects large requests
- Send POST with body > MaxRequestBodySize
- Assert 413 response
```

## Section 2: Core REST Endpoints

### Tests: `internal/api/handlers_test.go`

```
Test: GET /api/v1/repos returns paginated list
- Setup: analyze 3 repos
- GET /repos?limit=2&offset=0
- Assert 2 repos in data, meta.total=3

Test: GET /api/v1/repos/{encoded_id} returns context
- Encode repo ID: github.com%2Forg%2Frepo
- GET /repos/{encoded_id}
- Assert repo context returned

Test: GET /api/v1/repos/{id}/search returns results
- GET /repos/{id}/search?q=user&limit=10
- Assert search results returned

Test: GET /api/v1/repos/{id}/functions/{name} returns context
- GET /repos/{id}/functions/GetUser
- Assert function context with behavior

Test: POST /api/v1/repos/analyze returns 202 with job ID
- POST /repos/analyze with repo URL
- Assert 202 status
- Assert response has job_id

Test: POST /api/v1/orgs creates org
- POST /orgs with body {id, repos}
- Assert 201 status

Test: GET /api/v1/orgs returns paginated list
- GET /orgs?limit=10
- Assert orgs listed

Test: Response envelope format
- GET any endpoint
- Assert response has "data", "meta", "error" keys
- Assert meta has request_id

Test: 404 for unknown repo
- GET /repos/nonexistent%2Frepo
- Assert 404 with error message

Test: 400 for invalid JSON body
- POST /repos/analyze with malformed JSON
- Assert 400

Test: URL-encoded repo ID decoded correctly
- Encode github.com/org/repo as github.com%2Forg%2Frepo
- GET /repos/{encoded}
- Assert manager called with decoded "github.com/org/repo"

Test: Pagination defaults and bounds
- GET /repos (no params) -> limit=50, offset=0
- GET /repos?limit=999 -> capped at 200
- GET /repos?limit=-1 -> set to 50
```

## Section 3: Job Queue

### Tests: `internal/queue/queue_test.go`

```
Test: Enqueue creates pending job
- Enqueue("analyze_repo", "org1", "repo1", payload)
- Assert returned job ID non-empty
- GetJob: assert status="pending"

Test: Worker picks up and completes job
- Enqueue job
- Start queue with 1 worker
- Wait for job to complete
- Assert status="completed", result non-nil

Test: Failed job has error message
- Enqueue job that will fail (invalid repo)
- Wait for completion
- Assert status="failed", error non-empty

Test: Retry on failure
- Enqueue job with max_retries=3
- First attempt fails
- Assert retries incremented
- Assert job back to pending

Test: Max retries exhausted
- Enqueue job with max_retries=1
- Fail twice
- Assert status="failed" (not retried again)

Test: Concurrent workers claim different jobs
- Enqueue 10 jobs
- Start queue with 5 workers
- Wait for all to complete
- Assert each job processed exactly once

Test: Stale job recovery
- Enqueue job, manually set status="running", started_at=30min ago
- Run sweeper
- Assert job back to pending

Test: Job cleanup removes old jobs
- Enqueue job, manually set completed_at=8 days ago
- Run cleanup
- Assert job deleted

Test: Dedup prevents duplicate within 5 min
- Enqueue job for repo1
- Enqueue same repo1 job again
- Assert second returns existing job ID

Test: Dedup allows after 5 min
- Enqueue job for repo1 with created_at=10min ago
- Enqueue same repo1 again
- Assert new job created

Test: ListJobs paginated
- Enqueue 5 jobs
- ListJobs(limit=2, offset=0)
- Assert 2 returned, total=5

Test: ListJobs filtered by status
- Enqueue 3 jobs, complete 1
- ListJobs(status="completed")
- Assert 1 returned
```

## Section 4: Webhook Handlers

### Tests: `internal/api/webhooks_test.go`

```
Test: GitHub push webhook valid signature triggers job
- Create valid HMAC-SHA256 signature for payload
- POST /webhooks/github with signature header
- Assert 200
- Assert analysis job enqueued

Test: GitHub push webhook invalid signature returns 401
- POST /webhooks/github with wrong signature
- Assert 401
- Assert no job enqueued

Test: GitHub push webhook missing signature returns 401
- POST /webhooks/github without X-Hub-Signature-256
- Assert 401

Test: GitHub PR webhook triggers PR context job
- Create PR event payload
- POST with valid signature
- Assert PR context job enqueued

Test: GitHub push non-default branch skipped
- Create push payload for feature branch
- POST with valid signature
- Assert 200 but no job enqueued

Test: GitLab push webhook valid token
- POST /webhooks/gitlab with X-Gitlab-Token header
- Assert 200, job enqueued

Test: GitLab invalid token returns 401
- POST /webhooks/gitlab with wrong token
- Assert 401

Test: HMAC uses constant-time comparison
- (Unit test) Verify hmac.Equal is used, not ==

Test: Request body buffered for HMAC then parsing
- POST with valid signature
- Assert body parsed correctly (not EOF)

Test: Webhook dedup skips duplicate
- Send push webhook for same repo twice within 5 min
- Assert only 1 job enqueued
```

## Section 5: Health & Operations

### Tests: `internal/api/health_test.go`

```
Test: GET /health returns 200
- GET /health
- Assert {"status": "ok"}

Test: GET /api/v1/status returns counts
- Analyze 2 repos, register 1 org
- GET /status
- Assert repos_analyzed=2, orgs=1

Test: Rate limiter allows within limit
- Send 60 requests within 1 minute
- Assert all return 200

Test: Rate limiter blocks over limit
- Send 61 requests
- Assert 61st returns 429 with Retry-After header

Test: Graceful shutdown drains requests
- Start server
- Begin slow request (handler sleeps 2s)
- Send SIGTERM
- Assert slow request completes (not cut off)
- Assert new requests rejected

Test: Structured JSON log format
- Send request
- Assert log output is valid JSON with expected fields
```

## Section 6: Docker & Deployment

### Tests: manual verification

```
Test: Docker build succeeds
- docker build . passes

Test: HTTP mode starts and responds
- docker run --mode http
- curl http://localhost:8080/health returns 200

Test: MCP mode still works
- docker run (default mode)
- Send JSON-RPC request via stdin
```

## Section 7: Integration Tests

### Tests: `internal/integration/api_test.go`

```
Test: Full flow — analyze via API, search, get function
- POST /repos/analyze
- Poll GET /jobs/{id} until completed
- GET /repos/{id}/search?q=user
- Assert results

Test: Org flow — create, add repos, analyze, list
- POST /orgs
- POST /orgs/{id}/repos
- POST /orgs/{id}/analyze
- GET /orgs

Test: Webhook flow — push triggers analysis
- POST /webhooks/github with valid payload
- Poll job until complete
- GET /repos/{id} returns context

Test: Error handling end-to-end
- DELETE /repos/nonexistent -> 404
- POST /repos/analyze with empty URL -> 400
- GET /repos/{id}/functions/nonexistent -> 404
```
