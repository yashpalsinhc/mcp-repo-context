# Integration Notes: 05-service-layer

## Integrated

1. **Repo ID encoding** (#1) - URL-encode repo IDs in paths. Clients must percent-encode slashes. Document this. Example: `github.com%2Forg%2Frepo`. Chi will decode automatically.

2. **Atomic job dequeue** (#2) - Use UPDATE-with-subquery pattern: `UPDATE jobs SET status='running' WHERE id=(SELECT id FROM jobs WHERE status='pending' ORDER BY created_at LIMIT 1)`. Use IMMEDIATE transactions. Separate SQLite DB for jobs to avoid contention with main storage.

3. **Pagination** (#4) - Add `limit` (default 50, max 200) and `offset` query params to all list/search endpoints. Include pagination metadata in response envelope.

4. **HMAC constant-time comparison** (#5) - Use `hmac.Equal()` for GitHub, `subtle.ConstantTimeCompare` for GitLab. Buffer request body for HMAC then JSON parsing.

5. **Job retry/cleanup/stale recovery** (#9) - Max 3 retries with exponential backoff. Cleanup jobs older than 7 days. Sweeper reclaims jobs stuck in "running" for >30 minutes.

6. **Webhook dedup via DB** (#15) - Before enqueuing, check jobs table for same repo+type within last 5 minutes. Prevents duplicate analysis after restart.

7. **Webhook event filtering** (#11) - Filter by branch (configurable, default: default branch only). Skip documentation-only changes.

8. **Timeout per route group** (#8) - Apply chi timeout middleware per route group. Analysis endpoints don't need timeout (they're async 202). Regular endpoints: 30s.

9. **File path in URL** (#12) - Use wildcard `{path:*}` for file paths, or pass as query parameter. Clarify in section.

10. **Unit tests** (#10) - Add unit test specs for handler functions, rate limiter, HMAC verification, job queue dequeue.

## Not Integrated

1. **Auth** (#7) - Explicitly deferred. Plan states API is designed for auth middleware to be added later. For now, intended for internal/network-isolated deployment.

2. **API versioning strategy** (#6) - v1 is the only version. Strategy defined when breaking change is needed. Stated explicitly.

3. **OpenAPI spec** (#13) - Added as future consideration, not blocking v1 implementation.

4. **Dual-mode simultaneous** (#14) - Modes are mutually exclusive. MCP (stdio) or HTTP. Not both. Separate processes for both if needed.
