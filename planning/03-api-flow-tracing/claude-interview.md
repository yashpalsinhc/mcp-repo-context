# Interview: Cross-Service API Flow Tracing

## Q1: Storage approach for cross-service matching?
**Answer:** Up to you (plan decides)
**Decision:** Use new normalized SQLite tables for routes and external_calls. The current JSON blob approach requires loading full RepoContext objects for every query, which doesn't scale for orgs with 50+ repos. Normalized tables enable efficient SQL joins across repos.

## Q2: Async message tracing scope?
**Answer:** HTTP + Kafka only
**Decision:** Include HTTP, gRPC, and Kafka detection. Defer NATS, RabbitMQ, and SQS to a future plan. This covers the most common communication patterns while keeping scope manageable.

## Q3: Visualization format?
**Answer:** Mermaid only
**Decision:** Support only Mermaid output for service maps and flow traces. Simpler implementation, consistent with MCP client rendering.

## Q4: Multi-language service support?
**Answer:** Include basic multi-lang
**Decision:** Parse docker-compose.yml and package.json for topic/endpoint hints in non-Go services. This enables the service map to show non-Go services even without deep analysis, which is critical for realistic org-level topology views.
