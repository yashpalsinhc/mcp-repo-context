# Section 02: Core REST Endpoints

## Overview

Map core MCP tool functionality to REST endpoints with JSON request/response. Implements all CRUD handlers for repos, orgs, and search. Introduces the standard response envelope, URL-encoded repo ID handling, and pagination.

## Dependencies

- Section 01 (HTTP foundation, APIServer, router)
- Internal: `internal/orchestrator` (Manager), `internal/org` (Manager), `internal/storage`

## Repo ID Encoding

Repo IDs contain slashes (e.g., `github.com/org/repo`). Clients must URL-encode (percent-encode) the repo ID in path parameters. Chi automatically decodes percent-encoded path segments via `chi.URLParam()`.

Example: `GET /api/v1/repos/github.com%2Forg%2Frepo`

Document this in API responses and any future OpenAPI spec.

## Response Envelope

All endpoints return a standard JSON envelope:

```json
{
  "data": { ... },
  "meta": {
    "request_id": "abc-123",
    "duration_ms": 42,
    "total": 100,
    "limit": 50,
    "offset": 0
  },
  "error": null
}
```

On error:
```json
{
  "data": null,
  "meta": { "request_id": "abc-123" },
  "error": { "code": "not_found", "message": "repo not found: github.com/org/repo" }
}
```

Define types: `APIResponse`, `APIMeta`, `APIError`. Helper functions: `writeJSON(w, status, data, meta)`, `writeError(w, status, code, message, requestID)`.

## Pagination

All list/search endpoints accept query parameters:
- `limit` — default 50, max 200. Values < 1 reset to 50, values > 200 capped at 200.
- `offset` — default 0. Values < 0 reset to 0.

Helper function: `parsePagination(r *http.Request) (limit, offset int)`.

Response meta includes `total`, `limit`, `offset` for client-side pagination.

## Route Structure

```
POST   /api/v1/repos/analyze              -> handleAnalyzeRepo (async, 202)
GET    /api/v1/repos                       -> handleListRepos
GET    /api/v1/repos/{id}                  -> handleGetRepoContext
GET    /api/v1/repos/{id}/architecture     -> handleGetArchitecture
DELETE /api/v1/repos/{id}                  -> handleDeleteRepo
GET    /api/v1/repos/{id}/files            -> handleGetFileContext (?path=...)
GET    /api/v1/repos/{id}/search           -> handleSearchFunctions (?q=..., limit, offset)
GET    /api/v1/repos/{id}/functions/{name} -> handleGetFunctionContext
GET    /api/v1/repos/{id}/callers/{name}   -> handleGetCallers
GET    /api/v1/repos/{id}/concepts/{name}  -> handleSearchByConcept
GET    /api/v1/repos/{id}/side-effects/{type} -> handleSearchBySideEffect
POST   /api/v1/repos/{id}/refresh          -> handleRefreshChanged
POST   /api/v1/repos/{id}/refresh-file     -> handleRefreshFile (?path=...)
POST   /api/v1/repos/compare              -> handleCompareRepos
POST   /api/v1/repos/{id}/pr-context      -> handleGetPRContext

POST   /api/v1/orgs                        -> handleCreateOrg
GET    /api/v1/orgs                        -> handleListOrgs
GET    /api/v1/orgs/{id}                   -> handleGetOrg
DELETE /api/v1/orgs/{id}                   -> handleDeleteOrg
POST   /api/v1/orgs/{id}/repos            -> handleAddRepos
DELETE /api/v1/orgs/{id}/repos/{repoId}   -> handleRemoveRepo
POST   /api/v1/orgs/{id}/analyze          -> handleAnalyzeOrg (async, 202)

POST   /api/v1/ask                         -> handleAsk
POST   /api/v1/search                      -> handleSmartQuery

GET    /api/v1/jobs/{id}                   -> handleGetJob
GET    /api/v1/jobs                        -> handleListJobs (?status=..., limit, offset)
```

## Handler Pattern

Each handler follows this pattern:
1. Parse request — URL params via `chi.URLParam()` (auto percent-decoded), query params via `r.URL.Query()`, JSON body via `json.NewDecoder(r.Body).Decode()`.
2. Validate input — check required fields, return 400 on error.
3. Call Manager/OrgManager method.
4. Map result to response struct.
5. Return with appropriate HTTP status.

## Status Code Mapping

- 200: successful GET, successful synchronous POST
- 201: successful creation (POST /orgs)
- 202: accepted for async processing (POST /repos/analyze, POST /orgs/{id}/analyze)
- 400: bad request (invalid JSON, missing required fields)
- 404: not found (unknown repo, org, function, job)
- 413: request entity too large (body exceeds limit, handled by middleware)
- 429: rate limited (handled in section 5)
- 500: internal error (include request_id, no stack trace)

## Key Handler Details

### handleAnalyzeRepo (POST /api/v1/repos/analyze)
Request body: `{ "repo_url": "https://github.com/org/repo", "branch": "main" }`
- Validates repo_url is non-empty.
- Enqueues an `analyze_repo` job in the job queue (section 3). If job queue not yet available, calls manager directly (synchronous fallback).
- Returns 202 with `{ "job_id": "..." }`.

### handleListRepos (GET /api/v1/repos)
- Calls `manager.ListRepos()` (or equivalent).
- Applies pagination.
- Returns paginated list with total count.

### handleGetRepoContext (GET /api/v1/repos/{id})
- Decodes repo ID from URL: `chi.URLParam(r, "id")`.
- Calls `manager.GetContext(repoID, "full")`.
- Returns 404 if not found.

### handleSearchFunctions (GET /api/v1/repos/{id}/search)
- Query params: `q` (required), `limit`, `offset`.
- Returns 400 if `q` is empty.
- Calls manager search method.

### handleCreateOrg (POST /api/v1/orgs)
Request body: `{ "id": "my-org", "repos": ["repo1", "repo2"] }`
- Validates id is non-empty.
- Calls orgManager.RegisterOrg.
- Returns 201.

## File Path Handling

For endpoints that need file paths (which also contain slashes):
- `GET /api/v1/repos/{id}/files?path=pkg/handlers/user.go` — query parameter
- `POST /api/v1/repos/{id}/refresh-file?path=pkg/handlers/user.go` — query parameter

This avoids nested slash ambiguity in URL paths.

## Tests

### `internal/api/handlers_test.go`

**Test: GET /api/v1/repos returns paginated list**
- Setup: mock manager returns 3 repos
- GET /repos?limit=2&offset=0
- Assert 2 repos in data, meta.total=3

**Test: GET /api/v1/repos/{encoded_id} returns context**
- Encode repo ID: `github.com%2Forg%2Frepo`
- GET /repos/{encoded_id}
- Assert repo context returned with correct ID

**Test: GET /api/v1/repos/{id}/search returns results**
- GET /repos/{id}/search?q=user&limit=10
- Assert search results returned

**Test: GET /api/v1/repos/{id}/functions/{name} returns context**
- GET /repos/{id}/functions/GetUser
- Assert function context with behavior summary

**Test: POST /api/v1/repos/analyze returns 202 with job ID**
- POST /repos/analyze with `{"repo_url": "https://github.com/org/repo"}`
- Assert 202 status
- Assert response has job_id

**Test: POST /api/v1/orgs creates org**
- POST /orgs with `{"id": "test-org", "repos": []}`
- Assert 201 status

**Test: GET /api/v1/orgs returns paginated list**
- GET /orgs?limit=10
- Assert orgs listed with pagination meta

**Test: Response envelope format**
- GET any endpoint
- Assert response has "data", "meta", "error" keys
- Assert meta has request_id

**Test: 404 for unknown repo**
- GET /repos/nonexistent%2Frepo
- Assert 404 with error message

**Test: 400 for invalid JSON body**
- POST /repos/analyze with malformed JSON
- Assert 400

**Test: URL-encoded repo ID decoded correctly**
- Encode `github.com/org/repo` as `github.com%2Forg%2Frepo`
- GET /repos/{encoded}
- Assert manager called with decoded `github.com/org/repo`

**Test: Pagination defaults and bounds**
- GET /repos (no params) -> limit=50, offset=0
- GET /repos?limit=999 -> capped at 200
- GET /repos?limit=-1 -> set to 50

## File Inventory

| File | Purpose |
|------|---------|
| `internal/api/handlers.go` | All REST endpoint handlers |
| `internal/api/response.go` | APIResponse, APIMeta, APIError types, writeJSON, writeError |
| `internal/api/pagination.go` | parsePagination helper |
| `internal/api/routes.go` | Updated route registration with real handlers |
| `internal/api/handlers_test.go` | All handler tests |

## Acceptance Criteria

1. All routes registered and responding with correct status codes
2. Response envelope format consistent across all endpoints
3. URL-encoded repo IDs decoded correctly
4. Pagination works with defaults, bounds, and metadata
5. 404 for unknown resources, 400 for invalid input
6. All 12 tests pass
