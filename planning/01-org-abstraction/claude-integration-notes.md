# Integration Notes: Opus Review Feedback

## Integrating (with plan changes)

### 1. Store Interface Redesign (Critical #1, Medium #5)
**Integrating.** The plan was ambiguous. The correct approach: redesign the `Store` interface to have SQLite-native operations (`SaveOrg`, `GetOrg`, `ListOrgs`, `DeleteOrg`, `AddRepos`, `RemoveRepos`) instead of the current generic `Save`/`Get`/`List`/`Delete`. The `Manager` stays as the business logic layer on top, but the Store contract changes for SQLite atomicity.

### 2. main.go Uses FilesystemStore (Critical #2)
**Integrating.** The plan incorrectly assumed SQLiteStore was in use. Need to: add `DB()` accessor to existing `storage.SQLiteStore`, or open `*sql.DB` in main.go and pass to both stores. Will update plan to detail this.

### 3. Migration System (Critical #3)
**Integrating.** Will update plan to specify go:embed + hard-coded migrate method call, matching existing pattern exactly.

### 4. Existing toolAnalyzeOrg Replacement (Critical #4)
**Integrating.** Plan should explicitly state: replace sequential `toolAnalyzeOrg` in tools.go. Also document the `NewManager` constructor signature change (adds orchestrator dependency).

### 5. config_override_json in Migration (Medium #6)
**Integrating.** Add the column to the 003 migration schema.

### 6. DeleteContext on Orchestrator (Medium #7)
**Integrating.** Add `DeleteRepoContext(ctx, repoID)` to `orchestrator.Manager` interface — this is the cleanest approach that maintains layering.

### 7. org.ErrNotFound Sentinel (Minor #13)
**Integrating.** Define `var ErrNotFound = errors.New("org: not found")` in types.go.

### 8. Register: Upsert vs Error (Minor #14)
**Integrating.** Change to upsert behavior to maintain backward compatibility. Use INSERT OR REPLACE.

### 9. updated_at Trigger (Minor #15)
**Integrating.** Add SQLite trigger in migration for automatic `updated_at` updates.

### 10. Filesystem Migration Section (Medium #10)
**Integrating.** Promote from risk mitigation to dedicated section in the plan.

## NOT Integrating (with rationale)

### Retry Logic Refinement (Medium #8)
**Not integrating at this level.** The plan is a blueprint — the implementer will handle retry specifics. Simplifying to "retry once after 1 second delay, skip non-retryable errors (404, invalid repo)". No exponential backoff needed for single retry.

### Progress Reporting (Medium #9)
**Deferring.** MCP protocol is request-response. Adding streaming would require protocol changes beyond this split's scope. Will add a note about expected latency in tool description instead.

### EffectiveConfig Source Map (Minor #12)
**Not integrating.** Removing the Source map. EffectiveConfig will just be a merged OrgConfig — simpler. Debug info not needed at this stage.

### Benchmark Mock Latency (Minor #16)
**Not integrating.** Plan already notes benchmarks use mock orchestrator. This is appropriate for testing concurrency mechanics. Real latency testing is an integration-level concern.

### OrgWithCount List Optimization (Minor #17)
**Not integrating as separate concern.** The SQLite query will use COUNT(*) and not load repo IDs for List. This is implicit in the SQLite store design.

### Concurrency Validation (Minor #11)
**Partially integrating.** Will note clamping behavior in plan but not add JSON Schema min/max — the tool handler validates and clamps.
