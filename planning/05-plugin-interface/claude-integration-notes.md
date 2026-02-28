# Integration Notes: 05-plugin-interface

## Integrated

1. **Dimension mismatch** (#1) - Add dimension validation: all embedders in a registry must share the same dimension. Validate at construction time. This is the simplest approach and prevents silent data corruption. If different dimensions needed in future, partition vector store per dimension.

2. **Org-to-manager plumbing** (#2) - Use option (c): add AnalyzerName/EmbedderName to AnalyzeOptions. MCP server layer resolves org config and passes names. Manager stays org-agnostic.

3. **NewManagerWithAI** (#3) - Fold into options pattern: `WithAIRegistry(reg)` as a ManagerOption. Remove separate NewManagerWithAI constructor.

4. **copyConfig** (#4) - Explicitly update copyConfig to copy AnalyzerName and EmbedderName.

5. **Silent fallback warning** (#5) - Surface fallback in AnalysisResult.Warnings (or tool output). Not silent — visible in response.

6. **DefaultRegistry single path** (#6) - NewRegistry() with no args creates empty registry (generic fallback only). DefaultRegistry() is the only way to get built-in Go analyzer. Clean separation.

7. **Test mocks audit** (#7) - Add step to grep for all Analyzer/Embedder implementations in test files. Update each mock.

8. **Duplicate handling** (#8) - Last registration wins for duplicate languages/names. Document this behavior.

9. **Compile-time only** (#10) - Explicitly state: plugins are registered by editing Go code and recompiling. No runtime discovery.

## Not Integrated

1. **Constructor pattern inconsistency** (#9) - Asymmetry is justified: analyzer uses language dispatch, embedder uses name dispatch. Different semantics warrant different constructors. Add code comment.

2. **OrgConfig SQLite round-trip test** (#11) - OrgConfig is already stored as JSON in SQLite via the org store. Adding new JSON fields is automatically handled. Existing serialization tests cover this.
