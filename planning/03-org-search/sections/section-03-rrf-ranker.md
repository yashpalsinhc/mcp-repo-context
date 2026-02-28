# Section 3: RRF Hybrid Ranker

## Overview

Implement Reciprocal Rank Fusion (RRF) to merge keyword search results and semantic search results into a single ranked list. The RRF algorithm assigns scores based on rank position in each result list, boosting items that appear in both.

## Dependencies

- Section 1: FTS5 Virtual Tables (FunctionRef with RepoID)
- Section 2: Org-Scoped Search Methods (keyword results format)

## Tests First

### File: `internal/search/ranker_test.go` (new)

```
Test: MergeRRF with keyword-only results
- Input: 3 keyword results [A at rank 1, B at rank 2, C at rank 3], empty semantic
- Assert: 3 results returned
- Assert: A.RRFScore = 1/(60+1) ≈ 0.01639
- Assert: B.RRFScore = 1/(60+2) ≈ 0.01613
- Assert: C.RRFScore = 1/(60+3) ≈ 0.01587
- Assert: A.KeywordRank=1, A.SemanticRank=0

Test: MergeRRF with semantic-only results
- Input: empty keyword, 3 semantic results [X at rank 1, Y at rank 2, Z at rank 3]
- Assert: 3 results returned
- Assert: X.SemanticRank=1, X.KeywordRank=0
- Assert: X.RRFScore = 1/(60+1)

Test: MergeRRF boosts overlapping results
- Input: keyword=[GetUser at rank 3, DeleteUser at rank 1], semantic=[GetUser at rank 5]
- Assert: GetUser.RRFScore = 1/(60+3) + 1/(60+5) = 1/63 + 1/65 ≈ 0.03126
- Assert: DeleteUser.RRFScore = 1/(60+1) = 1/61 ≈ 0.01639
- Assert: GetUser ranked higher than DeleteUser (overlap boost)

Test: MergeRRF deduplicates by repoID:filePath:name
- Input: keyword has GetUser from repo-a/pkg/user.go, semantic has same function
- Assert: single entry in output, not duplicated
- Assert: both KeywordRank and SemanticRank populated

Test: MergeRRF sorts by score descending
- Input: mix of keyword and semantic with known ranks
- Assert: output[0].RRFScore >= output[1].RRFScore >= output[2].RRFScore

Test: MergeRRF with custom k value
- Input: same results with k=10 and k=60
- Assert: k=10 gives larger score differences between adjacent ranks
- Verify: rank 1 vs rank 2 gap is larger with k=10

Test: MergeRRF filters non-function semantic results
- Input: semantic results include a type entry (ItemType != "function")
- Assert: type entry not in merged output
- Assert: function entries present

Test: MergeRRF handles empty inputs
- Input: both keyword and semantic empty
- Assert: empty output, no error

Test: MergeRRF key generation handles special characters
- Input: function from repo "github.com/org/user-service" with path "pkg/handlers/user.go"
- Assert: key is "github.com/org/user-service:pkg/handlers/user.go:GetUser"
- Assert: correct dedup when same function in both lists

Test: RankedResult contains complete FunctionRef
- Input: keyword result with all FunctionRef fields populated
- Assert: RankedResult.FunctionRef preserves Name, Signature, File, Line, Summary, RepoID
```

## Implementation Details

### 1. New Package and Types

Create `internal/search/ranker.go`:

```go
package search

type RankedResult struct {
    FunctionRef  storage.FunctionRef
    KeywordRank  int     // 1-indexed rank from keyword search (0 = not present)
    SemanticRank int     // 1-indexed rank from semantic search (0 = not present)
    RRFScore     float64 // combined RRF score
}
```

### 2. MergeRRF Function

```go
func MergeRRF(keywordResults []storage.FunctionRef, semanticResults []vectors.SearchResult, k int) []RankedResult
```

Algorithm:
1. Create a map keyed by `makeKey(repoID, filePath, funcName)` → `*RankedResult`
2. Iterate keyword results (0-indexed position i):
   - Key: `result.RepoID + ":" + result.File + ":" + result.Function`
   - Create or update entry: KeywordRank = i+1, FunctionRef = result
   - Score += 1.0 / float64(k + i + 1)
3. Iterate semantic results (0-indexed position j):
   - Filter: skip if record type is not "function" (check `Record.ItemType`)
   - Key: `record.RepoID + ":" + record.FilePath + ":" + record.Name`
   - Create or update entry: SemanticRank = j+1
   - If entry was created from semantic (no keyword match), build FunctionRef from Record fields
   - Score += 1.0 / float64(k + j + 1)
4. Collect map values into slice
5. Sort by RRFScore descending (stable sort)
6. Return sorted slice

### 3. Key Generation

```go
func makeKey(repoID, filePath, funcName string) string
```

Simple string concatenation with colon separator. The key only needs to be unique within a single merge operation, so colons in repo IDs are fine (they're consistent between keyword and semantic results).

### 4. FunctionRef from Semantic Result

When a semantic result has no keyword match, build a FunctionRef:
- `Function` = Record.Name
- `File` = Record.FilePath
- `RepoID` = Record.RepoID
- `Summary` = Record.Description (or empty)
- `Signature`, `Line` = may not be available from vector record; leave empty/zero

This means semantic-only results may have incomplete FunctionRef data. The progressive disclosure layer handles this by showing available fields only.

### 5. Default k Constant

```go
const DefaultRRFK = 60
```

Exported so callers can pass it to MergeRRF. The value 60 is from the original RRF paper and works well for general-purpose ranking.

## Error Handling

- Empty inputs: return empty slice (not nil)
- Nil keyword or semantic slice: treat as empty
- Semantic results with missing RepoID: skip (log warning)

## File Summary

| File | Action |
|------|--------|
| `internal/search/ranker.go` | New: RankedResult type, MergeRRF function, makeKey helper, DefaultRRFK constant |
| `internal/search/ranker_test.go` | New: RRF merge tests |
