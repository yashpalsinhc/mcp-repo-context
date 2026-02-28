<!-- PROJECT_CONFIG
runtime: go
test_command: go test ./internal/... -v -count=1
END_PROJECT_CONFIG -->

<!-- SECTION_MANIFEST
section-01-http-foundation
section-02-rest-endpoints
section-03-job-queue
section-04-webhooks
section-05-health-ops
section-06-docker
section-07-integration-tests
END_MANIFEST -->

# Section Index: Service Layer & REST API

## Batch 1 (no dependencies)
- section-01-http-foundation: Chi router setup, middleware stack, APIServer struct, dual-mode entry point
- section-03-job-queue: SQLite-backed job queue, atomic dequeue, workers, retry, cleanup, dedup

## Batch 2 (depends on batch 1)
- section-02-rest-endpoints: All REST endpoint handlers, response envelope, URL encoding, pagination
- section-04-webhooks: GitHub/GitLab webhook handlers, HMAC verification, event filtering

## Batch 3 (depends on batch 2)
- section-05-health-ops: Health/status endpoints, rate limiting, structured logging, graceful shutdown

## Batch 4 (depends on batch 3)
- section-06-docker: Dockerfile updates, docker-compose.http.yml, health check
- section-07-integration-tests: End-to-end tests for full API flow, webhooks, job queue
