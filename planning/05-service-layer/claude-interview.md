# Interview: Service Layer & REST API

## Q1: HTTP framework?
**Answer:** chi router
**Decision:** Use github.com/go-chi/chi/v5. Lightweight, idiomatic, net/http compatible. Standard middleware ecosystem.

## Q2: Storage isolation model?
**Answer:** Single DB with org_id
**Decision:** Keep single SQLite database. All tables already scoped by repo_id, and repos already associated with orgs via org_repos table. Simpler ops, single backup, easy cross-org queries.

## Q3: Queue system for async analysis?
**Answer:** SQLite-backed queue
**Decision:** Use SQLite table for job queue. Survives restarts. Jobs have status (pending/running/completed/failed), timestamps, and result data.
