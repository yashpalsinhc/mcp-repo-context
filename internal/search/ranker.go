package search

import (
	"sort"

	ctxpkg "github.com/yashpalc/mcp-repo-context/internal/context"
	"github.com/yashpalc/mcp-repo-context/internal/vectors"
)

// DefaultRRFK is the default k parameter for RRF scoring (from the original RRF paper).
const DefaultRRFK = 60

// RankedResult holds a function reference with its RRF ranking metadata.
type RankedResult struct {
	FunctionRef  ctxpkg.FunctionRef
	KeywordRank  int     // 1-indexed rank from keyword search (0 = not present)
	SemanticRank int     // 1-indexed rank from semantic search (0 = not present)
	RRFScore     float64 // combined RRF score
}

// MergeRRF merges keyword and semantic search results using Reciprocal Rank Fusion.
// Items appearing in both lists get boosted scores. k controls rank smoothing.
func MergeRRF(keywordResults []ctxpkg.FunctionRef, semanticResults []vectors.SearchResult, k int) []RankedResult {
	if k <= 0 {
		k = DefaultRRFK
	}

	merged := make(map[string]*RankedResult)

	// Process keyword results
	for i, ref := range keywordResults {
		key := makeKey(ref.RepoID, ref.File, ref.Function)
		entry, ok := merged[key]
		if !ok {
			entry = &RankedResult{FunctionRef: ref}
			merged[key] = entry
		}
		entry.KeywordRank = i + 1
		entry.RRFScore += 1.0 / float64(k+i+1)
	}

	// Process semantic results
	for j, sr := range semanticResults {
		// Filter non-function results
		if sr.Record.Type != "function" {
			continue
		}
		// Skip entries with missing RepoID
		if sr.Record.RepoID == "" {
			continue
		}

		key := makeKey(sr.Record.RepoID, sr.Record.FilePath, sr.Record.Name)
		entry, ok := merged[key]
		if !ok {
			// Build FunctionRef from semantic record
			entry = &RankedResult{
				FunctionRef: ctxpkg.FunctionRef{
					Function: sr.Record.Name,
					File:     sr.Record.FilePath,
					RepoID:   sr.Record.RepoID,
				},
			}
			merged[key] = entry
		}
		entry.SemanticRank = j + 1
		entry.RRFScore += 1.0 / float64(k+j+1)
	}

	// Collect and sort by score descending
	results := make([]RankedResult, 0, len(merged))
	for _, entry := range merged {
		results = append(results, *entry)
	}

	sort.SliceStable(results, func(i, j int) bool {
		return results[i].RRFScore > results[j].RRFScore
	})

	return results
}

func makeKey(repoID, filePath, funcName string) string {
	return repoID + ":" + filePath + ":" + funcName
}
