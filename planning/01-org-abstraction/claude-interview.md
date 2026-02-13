# Interview Transcript: 01-org-abstraction

## Q1: Org Storage Backend
**Q:** The org package already exists with FilesystemStore (JSON). Should we migrate org storage to SQLite to match the main storage layer, keep the filesystem approach, or support both?

**A:** Migrate to SQLite. Single storage backend, consistent with repos/functions/vectors. Better for queries and joins.

## Q2: Error Handling for analyze_org
**Q:** For analyze_org, what should happen when one repo in the org fails analysis?

**A:** Retry failed, then continue. Retry each failure once, then continue with what succeeded.

## Q3: Concurrency Model for analyze_org
**Q:** Should analyze_org run repo analyses in parallel or sequentially?

**A:** Configurable. Let user choose via org config or tool parameter.

## Q4: Expected Scale
**Q:** What's the expected scale for organizations?

**A:** Medium (10-50 orgs, 20-100 repos each). Multi-team enterprise. Needs good indexing.

## Q5: Parallelism Strategy Across Splits
**Q:** For the parallel execution strategy across all 5 splits, do you want to maximize parallelism within each split or restructure the dependency graph?

**A:** Maximize within each split. Keep the dependency graph, but parallelize implementation tasks within each split as much as possible.

## Q6: Test Coverage Level
**Q:** What level of test coverage are you targeting?

**A:** Full coverage with benchmarks. Unit tests, integration tests, table-driven tests for all operations, plus Go benchmarks for storage/query performance.

## Q7: Config Inheritance Model
**Q:** Should the org abstraction support org-level config inheritance?

**A:** Inheritance with override. Repos inherit org config by default but can override specific settings.

## Q8: Org Deletion Behavior
**Q:** When deleting an org, what should happen to the repos?

**A:** Ask user at deletion time. Prompt via tool parameter: detach or cascade. Most flexible.

## Q9: Benchmark Scope
**Q:** What operations should benchmarks measure?

**A:** Both storage and pipeline. Comprehensive benchmarks covering both data layer and analysis operations.
