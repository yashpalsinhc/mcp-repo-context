# Integration Notes: Opus Review Feedback

## Integrated

1. **Dual Route struct** (#1): Added note to Section 2 to consolidate analyzer.Route and context.Route, or at minimum establish explicit field mapping.

2. **HTTP client wrapper detection** (#3b): Added to Section 1 — detect method calls on struct fields with names containing "client", "service", or "api" as potential HTTP/gRPC client calls. Mark with lower confidence.

3. **Service-hint-based filtering in matching** (#4a): Added to Section 5 — use URL hostname (when available) and variable/field name hints to narrow matching. When multiple endpoints match, prefer matches where service hint matches repo/service name.

4. **Entry_point resolution** (#5): Added to Section 7 — require org_id + repo_id for unambiguous starting point, or use service_hint to disambiguate when multiple services register same path.

5. **Explicit cycle detection** (#10): Added to Section 7 — trace_api_flow uses a visited set to detect cycles, reports them as "[cycle: ServiceA → ServiceB → ServiceA]" rather than silently hitting depth limit.

6. **Batch endpoint matching** (#6): Added to Section 5/6 — load all endpoints into memory trie for topology build rather than per-call SQL queries.

7. **Migration strategy** (#7): Added to Section 4 — follow existing migration pattern in SQLiteStore.ensureSchema().

8. **Dead code fix** (#11): Added to Section 2 — fix duplicate Gin/Echo detection blocks.

9. **Sample output** (#14): Added sample output sketches for both trace_api_flow and get_service_map.

10. **ExternalCall backward compat** (#12): Added note confirming JSON backward compatibility.

## Not Integrated

1. **Cross-file constant resolution** (#3a): File-scoped constant resolution is a reasonable first pass. Per-package constant index is a good follow-up but adds significant complexity. Documented as known limitation.

2. **gRPC stored client detection** (#3c): Detecting `s.userClient.GetUser()` requires type-aware analysis that's beyond current AST-only approach. The HTTP client wrapper heuristic (names containing "client") will partially cover this. Documented as limitation.

3. **Kubernetes manifest parsing** (#13): Good idea but out of scope for this plan. docker-compose.yml is more common in development contexts where this tool will be used. Kubernetes support can be a follow-up.

4. **Consumer group handler tracing** (#9): Tracing topic from caller to ConsumerGroupHandler is a complex flow analysis problem. Document as limitation. Many codebases have the topic name in the same file as the consumer setup.
