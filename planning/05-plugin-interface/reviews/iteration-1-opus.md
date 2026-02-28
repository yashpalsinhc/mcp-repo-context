# Opus Review: 05-plugin-interface

## Issues Found

1. **Dimension mismatch in vector store** (Critical) - Per-org embedder selection can mix dimensions in same store
2. **Missing org-to-manager plumbing** (High) - Manager has no concept of org; no way to resolve per-org config
3. **NewManagerWithAI not updated** (High) - Second constructor also hardcodes analyzer.NewRegistry()
4. **copyConfig not updated** (Medium) - Helper function doesn't copy new AnalyzerName/EmbedderName fields
5. **Silent fallback UX** (Medium) - Unknown plugin name silently degrades without user-visible warning
6. **DefaultRegistry dual-path drift** (Medium) - Both NewRegistry() zero-arg and DefaultRegistry() register Go analyzer
7. **Test mocks breaking** (Medium) - Adding Name() to interface breaks any mock implementations
8. **Duplicate name/language handling** (Low) - Unspecified behavior for duplicate registrations
9. **Constructor pattern inconsistency** (Low) - Analyzer vs embedder registry constructors differ
10. **Runtime plugin registration** (Low) - Not stated whether this is compile-time only
11. **OrgConfig serialization through SQLite** (Low) - Need round-trip test through store, not just JSON
