# Section 04: Webhook Handlers

## Overview

GitHub and GitLab webhook endpoints for automatic analysis on code push and pull/merge request events. Implements HMAC-SHA256 verification for GitHub, secret token verification for GitLab, event filtering by branch, and deduplication via the job queue.

## Dependencies

- Section 01 (HTTP foundation, router)
- Section 02 (response envelope, helpers)
- Section 03 (job queue for enqueuing analysis jobs)
- Standard library: `crypto/hmac`, `crypto/sha256`, `crypto/subtle`, `io`

## Configuration

Environment variables:
- `GITHUB_WEBHOOK_SECRET` — HMAC secret for GitHub webhook verification. If not set, the GitHub webhook endpoint returns 503 (Service Unavailable).
- `GITLAB_WEBHOOK_SECRET` — Secret token for GitLab webhook verification. If not set, the GitLab webhook endpoint returns 503.
- `WEBHOOK_BRANCHES` — Comma-separated list of branches to accept (default: only the repository's default branch).

These are read into `APIConfig` at startup.

## GitHub Webhook

### Endpoint

```
POST /api/v1/webhooks/github
```

### Verification Flow

1. Read the entire request body into a byte buffer using `io.ReadAll(r.Body)`.
2. Extract `X-Hub-Signature-256` header. If missing, return 401.
3. Compute HMAC-SHA256 of the buffered body using the configured secret.
4. Compare using `hmac.Equal()` (constant-time). If mismatch, return 401.
5. Parse the buffered body as JSON (not from `r.Body` which is now EOF).

**Critical:** The body must be buffered first, then HMAC verified, then parsed. Do not parse before verifying.

### Event Handling

Check `X-GitHub-Event` header:

**push event:**
1. Extract `repository.clone_url` and `ref` from payload.
2. Extract branch name from ref (`refs/heads/main` -> `main`).
3. Filter: only process if branch matches the default branch (from `repository.default_branch`) or is in `WEBHOOK_BRANCHES`.
4. Optional: skip if all changed files are documentation-only (all paths end in `.md`). Check `commits[].added`, `commits[].modified`, `commits[].removed`.
5. Enqueue `analyze_repo` job via job queue. The queue handles dedup (same repo within 5 minutes).
6. Return 200 with `{"status": "accepted"}`.

**pull_request event (opened, synchronize):**
1. Extract `pull_request.head.repo.clone_url`, `pull_request.number`, changed files info.
2. Enqueue `pr_context` job via job queue.
3. Return 200 with `{"status": "accepted"}`.

**Other events:** Return 200 with `{"status": "ignored"}`.

### Payload Types

Define Go structs for GitHub webhook payloads (only the fields we need):

```
type GitHubPushPayload struct {
    Ref        string
    Repository struct {
        CloneURL      string `json:"clone_url"`
        DefaultBranch string `json:"default_branch"`
        FullName      string `json:"full_name"`
    }
    Commits []struct {
        Added    []string
        Modified []string
        Removed  []string
    }
}

type GitHubPRPayload struct {
    Action      string
    PullRequest struct {
        Number int
        Head   struct {
            Repo struct {
                CloneURL string `json:"clone_url"`
                FullName string `json:"full_name"`
            }
        }
    } `json:"pull_request"`
}
```

## GitLab Webhook

### Endpoint

```
POST /api/v1/webhooks/gitlab
```

### Verification

1. Extract `X-Gitlab-Token` header. If missing, return 401.
2. Compare with configured secret using `subtle.ConstantTimeCompare()`. If mismatch, return 401.
3. Parse JSON body normally.

### Event Handling

Check `object_kind` field in payload:

**push event:**
1. Extract `project.git_http_url` and `ref`.
2. Filter by branch (same logic as GitHub).
3. Enqueue `analyze_repo` job.
4. Return 200.

**merge_request event:**
1. Extract merge request details.
2. Enqueue analysis job.
3. Return 200.

### Payload Types

```
type GitLabPushPayload struct {
    ObjectKind string `json:"object_kind"`
    Ref        string
    Project    struct {
        GitHTTPURL    string `json:"git_http_url"`
        DefaultBranch string `json:"default_branch"`
        PathWithNS    string `json:"path_with_namespace"`
    }
}
```

## HMAC Verification Helper

Extract into a reusable function:

```
func verifyGitHubSignature(body []byte, signature string, secret string) bool
```

1. Parse the `sha256=` prefix from the signature string.
2. Compute HMAC-SHA256 of body with secret.
3. Hex-encode the MAC.
4. Compare using `hmac.Equal()`.

## Tests

### `internal/api/webhooks_test.go`

**Test: GitHub push webhook valid signature triggers job**
- Create valid HMAC-SHA256 signature for a push payload
- POST /webhooks/github with `X-Hub-Signature-256` header
- Assert 200
- Assert analysis job enqueued in mock queue

**Test: GitHub push webhook invalid signature returns 401**
- POST /webhooks/github with wrong signature
- Assert 401
- Assert no job enqueued

**Test: GitHub push webhook missing signature returns 401**
- POST /webhooks/github without `X-Hub-Signature-256` header
- Assert 401

**Test: GitHub PR webhook triggers PR context job**
- Create pull_request event payload (action: "opened")
- POST with valid signature and `X-GitHub-Event: pull_request`
- Assert PR context job enqueued

**Test: GitHub push non-default branch skipped**
- Create push payload for `refs/heads/feature-x` where default_branch is "main"
- POST with valid signature
- Assert 200 but no job enqueued

**Test: GitLab push webhook valid token**
- POST /webhooks/gitlab with correct `X-Gitlab-Token` header
- Assert 200, job enqueued

**Test: GitLab invalid token returns 401**
- POST /webhooks/gitlab with wrong token
- Assert 401

**Test: HMAC uses constant-time comparison**
- Unit test for `verifyGitHubSignature`:
  - Valid signature returns true
  - Invalid signature returns false
  - Empty signature returns false

**Test: Request body buffered for HMAC then parsing**
- POST with valid signature
- Assert body parsed correctly (not EOF error)
- This is implicitly tested by all valid-signature tests

**Test: Webhook dedup skips duplicate**
- Send push webhook for same repo twice within 5 min
- Assert only 1 new job enqueued (second returns existing ID)

## File Inventory

| File | Purpose |
|------|---------|
| `internal/api/webhooks.go` | GitHub and GitLab webhook handlers |
| `internal/api/webhooks_github.go` | GitHub-specific payload types and HMAC verification |
| `internal/api/webhooks_gitlab.go` | GitLab-specific payload types and token verification |
| `internal/api/webhooks_test.go` | All webhook tests |

## Acceptance Criteria

1. GitHub webhook verifies HMAC-SHA256 using constant-time comparison
2. GitLab webhook verifies token using constant-time comparison
3. Invalid/missing signatures return 401 with no detail
4. Push events on default branch enqueue analysis jobs
5. Push events on non-default branch are skipped
6. PR/MR events enqueue PR context jobs
7. Dedup prevents duplicate jobs for same repo within 5 minutes
8. Missing webhook secret returns 503 (not configured)
9. All 10 tests pass
