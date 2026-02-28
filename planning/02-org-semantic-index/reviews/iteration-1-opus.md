# Opus Review

**Model:** claude-opus-4
**Generated:** 2026-02-25

---

## Critical Issues

### 1. Vocabulary Invalidation Problem (Section 2 / Section 3)
TF-IDF embedder uses vocabulary-dependent indices. When vocabulary changes (new repo added), all existing embeddings become invalid. Plan needs vocabulary versioning strategy.

### 2. Hash Computation Relies on Analyzed Output, Not Source (Section 1)
Hashing name+signature+description+behaviorSummary means analyzer upgrades change hashes even if code hasn't changed. Recommend hashing raw source code instead.

### 3. Missing DeleteByFile in Vector Store
File summary lists it but section text doesn't define signature or behavior.

### 4. Two Separate SQLite Databases
function_hashes has vector_id FK but vectors may be in a different database. Need to clarify which DB each table belongs to.

## Performance Issues

### 5. Brute-Force Search Scalability at Org Level
SearchByOrg loads ALL vectors into memory. 50 repos × 500 functions = 25k vectors deserialized. Need scaling mitigation.

### 6. Vocabulary Building Memory Usage
BuildOrgVocabulary loads every RepoContext fully. Need lightweight query for just document strings.

### 7. StoreBatch Atomicity for Large Orgs
Single transaction for tens of thousands of inserts. Consider chunked batches.

## Design Issues

### 8. Concurrency Safety of Embedder State
Shared embedder instance with ImportVocabulary. Concurrent index_org calls could overwrite vocabulary mid-flight.

### 9. org_vocabulary Table Storing Raw JSON
200-500KB JSON blob. Consider binary format or mention parsing cost.

### 10. Embedder Interface Missing Vocabulary Methods
ExportVocabulary/ImportVocabulary only on concrete type, not interface. Consider VocabularyAwareEmbedder interface.

## Missing Considerations

### 11. No Migration Number Verification
Plan says "005 or next available" but doesn't verify.

### 12. No Handling for Functions with Same Name in Different Files
Receiver-qualified names like (*Foo).Bar and (*Baz).Bar both named "Bar".

### 13. No Rollback/Error Recovery for Partial Vector Updates
RefreshFileVectors involves multiple steps across potentially different DBs. No transaction wrapping.

### 14. RemoveRepos Does Not Rebuild Vocabulary
Vocabulary still includes removed repos' IDF weights.

### 15. No Progress Reporting for index_org
Large orgs could take minutes. No progress mechanism.

### 16. SearchByOrg Query Uses Which Vocabulary?
Must load org vocabulary before embedding the search query.
