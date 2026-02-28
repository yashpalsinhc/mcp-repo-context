# Integration Notes: Opus Review

## Integrating

### 1. Vocabulary Invalidation (#1) - CRITICAL
Integrating. Add vocabulary versioning: store version hash alongside each vector record. On search, if vocabulary version doesn't match current org vocabulary, re-embed the query with current vocabulary AND flag for background re-indexing. On index_org, always re-embed all vectors (vocabulary rebuild = full re-embed).

### 2. Hash Source Code Instead of Analysis Output (#2)
Integrating. Change hash computation to SHA256 of raw function source code (line range from file). This correctly detects code changes regardless of analyzer changes.

### 3. DeleteByFile Method (#3)
Integrating. Add explicit `DeleteByFile(ctx, repoID, filePath string) error` method signature to vector store interface.

### 4. Database Clarification (#4)
Integrating. Place `function_hashes` in the main storage DB. The `vector_id` column is an application-level reference, not a real FK. Document this.

### 8. Embedder Concurrency Safety (#8)
Integrating. Create new embedder instance per org-indexing operation. Don't mutate shared embedder. Pass vocabulary into embedding calls explicitly.

### 10. VocabularyAwareEmbedder Interface (#10)
Integrating. Add `VocabularyAwareEmbedder` interface with ExportVocabulary/ImportVocabulary methods.

### 12. Receiver-Qualified Function Names (#12)
Integrating. Use fully qualified names including receiver (e.g., `(*Foo).Bar`) as the name field in function_hashes.

### 13. Transaction Wrapping (#13)
Integrating. Wrap RefreshFileVectors in transaction where possible. Document cross-DB consistency trade-off.

### 14. Vocabulary Rebuild on RemoveRepos (#14)
Integrating. Mark org vocabulary as stale when repos are added/removed. Rebuild on next search or index_org.

### 16. SearchByOrg Vocabulary Loading (#16)
Integrating. Load org vocabulary before embedding search query in SearchByOrg.

## Not Integrating

### 5. Brute-Force Search Scalability (#5)
Not integrating as a plan change. The current brute-force approach is adequate for expected org sizes (< 100k vectors). Acknowledge as known limitation. ANN/vector DB is a future optimization.

### 6. Vocabulary Building Memory (#6)
Not integrating as separate section. Will note in Section 2 to stream documents repo-by-repo instead of loading all simultaneously.

### 7. StoreBatch Chunking (#7)
Not integrating as separate change. Will note in Section 5 to chunk StoreBatch into per-repo transactions (already the natural flow of index_org).

### 9. Binary Format for Vocabulary (#9)
Not integrating. JSON is fine for initial implementation. Can optimize later.

### 11. Migration Number (#11)
Acknowledged. Will verify actual next migration number during implementation.

### 15. Progress Reporting (#15)
Not integrating. MCP tools are synchronous. Can add logging. Progress reporting is a future enhancement.
