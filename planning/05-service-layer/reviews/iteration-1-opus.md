# Opus Review: 05-service-layer

## Issues Found

1. **Repo IDs with slashes break chi routing** (Critical) - github.com/org/repo can't be captured by {id}
2. **SQLite job queue race condition** (Critical) - No SELECT FOR UPDATE in SQLite, workers can grab same job
3. **Rate limiter state loss on restart** (Significant) - Webhook storms can hit again after restart
4. **Missing pagination** (Significant) - List endpoints return unbounded results
5. **HMAC verification incomplete** (Significant) - Missing constant-time comparison, body buffering
6. **API versioning strategy incomplete** (Significant) - No v2 transition plan
7. **No auth** (Significant) - Delete endpoints open to anyone
8. **Timeout middleware misconfigured** (Medium) - Can't have 2 timeouts globally, analysis is async anyway
9. **Job queue missing retry/cleanup/stale recovery** (Medium)
10. **Test coverage gaps** (Medium) - No unit tests, no load tests
11. **Webhook event filtering too broad** (Minor) - Every push triggers analysis
12. **File path in URL also has slash problem** (Minor) - /refresh/{path} also broken
13. **No OpenAPI spec** (Minor)
14. **Dual-mode complexity** (Minor) - Can both run simultaneously?
15. **Webhook dedup via DB not memory** (Medium) - Use jobs table for dedup
