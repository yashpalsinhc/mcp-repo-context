# Integration Notes: Opus Review Feedback

## Suggestions INTEGRATED

### Section 1: Receiver-Aware Comparison Keys

1. **Explicit `FindConflicts`/`FindGaps` refactoring** - INTEGRATE. The reviewer correctly identified that these functions use `fn.Name` directly and don't call `normalizeFunctionKey()`. The plan implied this but needs to be explicit.

2. **Existing `Version` field reconciliation** - INTEGRATE. The plan should reuse the existing `Version` field on `RepoContext` instead of adding a new `SchemaVersion`. Clarify interaction with SQLite `schema_migrations` table (which is DB-schema level, not data-content level).

3. **Migration idempotency** - INTEGRATE. Make lazy migration idempotent with version guard (check before write). This addresses the concurrency concern without adding locking complexity.

4. **Fix `normalizeTypeKey` too** - INTEGRATE. Same bug, same fix pattern. Trivial to include and prevents false type duplicates.

### Section 2: Domain-Aware Gap Analysis

5. **Stop words for common Go prefixes** - INTEGRATE. Add a stop word list (Get, Set, New, Handle, Error, etc.) to avoid false similarity from ubiquitous function name prefixes.

6. **O(1) domain profile lookup** - INTEGRATE. Use `map[string]bool` for the stemmed domain profile, not a slice.

7. **Acknowledge target-only gap behavior** - INTEGRATE. Add note that gap detection only runs when a target repo is specified.

### Section 3: Smart Query NLP

8. **Logic reordering for "how does X work" queries** - INTEGRATE. The reviewer is correct: the fix should check if the extracted word matches an actual function first, then fall back to concept search. Stemming alone doesn't solve the regex greediness.

9. **Fix count: "Five improvements" not "Four"** - INTEGRATE. Trivial fix.

10. **Clarify confidence contract** - INTEGRATE. Define: parsing confidence determines routing, handler confidence determines result quality. Handler confidence can lower but not raise parsing confidence.

11. **Fix `handleFileQuery` substring matching** - INTEGRATE. Same bug as package path matching. Include in scope.

### Section 4: Pattern Execution

12. **Disambiguation for multi-result searches** - INTEGRATE. Use first result with highest confidence score. If multiple files match, include file path in the result message so the user knows which was chosen.

13. **Step status tracking with three states** - INTEGRATE. Track "executed", "skipped" (condition false), and "not_reached" (earlier step failed). This is cleaner than the plan's two-state model.

### Section 5: Call Graph

14. **Update `makeNodeID` to include receiver** - INTEGRATE. Critical fix for node map collision. Format: `"file:ReceiverType.function"`.

15. **Fix `funcFile` map collision** - INTEGRATE. Key by receiver-qualified name instead of plain function name.

16. **Scope heuristic mode precisely** - INTEGRATE. List exactly which variable declaration patterns are handled: function parameters, `var x Type`, `x := Type{}`. Explicitly note what's NOT handled: function return values, type assertions, shadowed variables.

### Section 6: Package Structure

17. **Add deeply nested package behavior** - INTEGRATE. Specify: show top 2 levels of nesting, collapse deeper levels.

### Section 7 / Cross-Cutting

18. **Levenshtein early termination** - INTEGRATE. Bail out of distance calculation once accumulated cost exceeds `maxDistance`.

19. **NLP edge cases** - INTEGRATE. Handle empty inputs (return 0.0 similarity), single-char words (skip stemming), Unicode (lowercase only ASCII, pass through Unicode unchanged).

20. **Integration test with gorilla repos** - INTEGRATE. Add as acceptance criterion for the overall plan.

---

## Suggestions NOT INTEGRATED

### Section 2: Defer Porter Stemmer

**Reviewer says:** Consider starting with simpler camelCase splitting + lowercasing without stemming.

**Not integrating because:** The stemmer is shared across Sections 2 and 3. Smart query NLP explicitly needs stemming for "routing" -> "rout" matching. Building it once in Section 7 and using it everywhere is more efficient than building a simpler tokenizer now and adding stemming later. The risk of a buggy stemmer is mitigated by thorough testing against known word lists (which I'll add to the plan).

### Section 2: Threshold Justification with Real Data

**Reviewer says:** Test the 0.3 threshold against gorilla repos to demonstrate gap reduction.

**Not integrating in the plan itself because:** This is a testing/calibration task for implementation time, not a plan change. The plan already specifies the threshold is configurable. The integration test (item 20 above) will serve as validation.

### Section 5: Drop go/types Mode 2

**Reviewer says:** Reconcile "no external dependencies" with `golang.org/x/tools`.

**Not integrating removal because:** The "no external dependencies" constraint applies to NLP (where we want zero deps). `golang.org/x/tools` is Go's quasi-standard toolchain and is a different category. However, I'll clarify this distinction in the plan and note that Mode 2 is opt-in/future work, not required for the bug fix.

### Section 1: Lazy Migration as Explicit Command

**Reviewer says:** Consider making migration an explicit command instead of a side effect.

**Not integrating because:** An explicit migration command adds UX complexity. Users would need to know to run it before comparison tools work correctly. Lazy migration with idempotency guard (which I AM integrating) is transparent and safe enough.

### Section 7: Rollback Strategy

**Reviewer says:** Add rollback from version 1 to version 0.

**Not integrating because:** Version 0 data is strictly less information (no receiver in keys). Re-analyzing the repo produces version 1 data. If migration has a bug, the fix is to re-analyze, not to roll back to broken data. Adding rollback complexity is not justified.

### Section 7: Porter Stemmer Corrections

**Reviewer says:** Fix stemmer examples (handlers -> handler, not handl).

**Not integrating because:** The reviewer is correct about standard Porter stemming behavior, but the examples in the plan are illustrative of intent, not exact algorithm output. The implementation will use correct Porter algorithm. This is an implementation detail, not a plan change.
