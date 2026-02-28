# Interview Transcript: Dependency Graph & Import Analysis

## Q1: Analyzer Architecture
**Question:** For the dependency graph feature, should the go.mod parser be its own analyzer registered in the analyzer registry, or should it extend the existing generic analyzer?

**Answer:** Up to you — let the implementation plan decide the best approach.

## Q2: Config File Storage Scope
**Question:** What level of config file content storage do you need? The spec mentions go.mod, Dockerfile, docker-compose.yml, Makefile, CI configs. Should ALL of these be parsed with structured extraction, or just go.mod?

**Answer:** All structured where possible — parse Dockerfile, docker-compose.yml etc. into structured data too, not just go.mod.

## Q3: Tool Scope (get_dependency_graph)
**Question:** Should the new get_dependency_graph tool work at org level (all analyzed repos), or require explicit repo_ids?

**Answer:** Repo-level primary — require explicit repo_ids, add org support later.

## Q4: Storage Strategy
**Question:** Should the dependency graph persist across sessions or be computed on-the-fly?

**Answer:** Hybrid approach — store parsed go.mod data in SQLite, compute cross-repo graph on-the-fly from stored data.

## Q5: Stdlib Detection
**Question:** For classifying imports as stdlib vs external, what approach?

**Answer:** Use Go tooling — shell out to `go list std` at analysis time for most accurate results (requires Go installed on the system).

## Q6: Testing Strategy
**Question:** Should integration tests use real repos or synthetic fixtures?

**Answer:** Both — synthetic fixtures for unit tests with known dependency relationships, real repos (gorilla/*) for integration tests.
