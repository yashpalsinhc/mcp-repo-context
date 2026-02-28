# Interview Transcript: 01-core-bug-fixes

## Q1: Gap Analysis Scope
**Q:** For the gap analysis fix (#3): should we implement a simple heuristic now and defer architecture YAML to a later split, or include the architecture config file in this bug-fix split?

**A:** Simple heuristic now, architecture file later. Use concept/package similarity for gap scoping. Architecture YAML goes in 02-dependency-graph or a new split.

## Q2: Smart Query Fallback Chain
**Q:** For smart_query NLP fix (#4): when confidence is low, what should the fallback chain be?

**A:** Below 60% confidence: automatically fallback to AI (ask tool). Don't return low-confidence wrong results.

## Q3: Data Compatibility / Migration
**Q:** The normalizeFunctionKey() fix changes how function keys are generated. Should we handle migration of existing stored data or require re-analysis?

**A:** Add migration logic. Detect old format and auto-migrate stored keys for a seamless experience.

## Q4: Call Graph - go/types Integration
**Q:** For call graph callee extraction: should we improve the existing heuristic or add go/types integration for full type-checked call graphs?

**A:** Add go/types integration. Full type-checked call graph with go/types.Checker for accurate method resolution.

## Q5: External Dependencies for NLP
**Q:** Should we add external Go libraries for stemming/fuzzy matching, or use custom implementations?

**A:** Custom implementation. Write our own Porter stemmer and Levenshtein distance. No new external dependencies.

## Q6: Implementation Priority Order
**Q:** What priority order should we implement the 7 fixes?

**A:** By severity: Critical -> High -> Medium -> Low.
1. normalizeFunctionKey (Critical)
2. find_conflicts interface detection (Critical)
3. find_gaps semantic model (High)
4. smart_query NLP (High)
5. execute_pattern step execution (Medium)
6. Call graph callee extraction (Medium)
7. get_package_structure grouping (Low)

## Q7: go/types Failure Handling
**Q:** How should we handle repos where go/types module resolution fails (private deps, no go.sum)?

**A:** Make it configurable. Add a flag like `--use-type-checker` that defaults to heuristic. Users opt-in to go/types when they have full module access.

## Q8: Migration Timing
**Q:** Should migration happen on server startup or on first access to comparison tools?

**A:** On first comparison tool use (lazy migration). Re-key functions when compare_repos/find_duplicates is first called. No startup cost.
