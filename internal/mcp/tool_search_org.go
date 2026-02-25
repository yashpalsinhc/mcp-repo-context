package mcp

import (
	"context"
	"fmt"
	"strings"

	ctxpkg "github.com/yashpalc/mcp-repo-context/internal/context"
	"github.com/yashpalc/mcp-repo-context/internal/search"
	"github.com/yashpalc/mcp-repo-context/internal/vectors"
)

// toolSearchOrg handles the search_org tool call.
func (s *server) toolSearchOrg(ctx context.Context, args map[string]any) callToolResult {
	orgID, _ := args["org_id"].(string)
	if orgID == "" {
		return errorResult("org_id is required")
	}

	query, _ := args["query"].(string)
	if strings.TrimSpace(query) == "" {
		return errorResult("query parameter is required and cannot be empty")
	}

	searchType, _ := args["search_type"].(string)
	if searchType == "" {
		searchType = "hybrid"
	}
	if searchType != "keyword" && searchType != "semantic" && searchType != "hybrid" {
		return errorResult("search_type must be one of: keyword, semantic, hybrid")
	}

	maxResults := 20
	if v, ok := args["max_results"].(float64); ok && v > 0 {
		maxResults = int(v)
	}

	tokenBudget := 4000
	if v, ok := args["token_budget"].(float64); ok && v > 0 {
		tokenBudget = int(v)
	}

	// Validate org exists
	if s.config.OrgManager == nil {
		return errorResult("Organization management is not configured")
	}

	orgData, err := s.config.OrgManager.Get(ctx, orgID)
	if err != nil {
		return errorResult(fmt.Sprintf("Organization '%s' not found. Use register_org to create it.", orgID))
	}

	// Validate repo_ids filter if provided
	var repoFilter map[string]bool
	if repoIDsRaw, ok := args["repo_ids"]; ok {
		if repoIDsList, ok := repoIDsRaw.([]interface{}); ok && len(repoIDsList) > 0 {
			repoFilter = make(map[string]bool, len(repoIDsList))
			orgRepoSet := make(map[string]bool, len(orgData.Repos))
			for _, r := range orgData.Repos {
				orgRepoSet[r] = true
			}
			for _, r := range repoIDsList {
				repoID, ok := r.(string)
				if !ok {
					continue
				}
				if !orgRepoSet[repoID] {
					return errorResult(fmt.Sprintf("Repository '%s' is not in organization '%s'", repoID, orgID))
				}
				repoFilter[repoID] = true
			}
		}
	}

	// Get org searcher
	if s.config.OrgSearcher == nil {
		return errorResult("Org search is not configured")
	}

	var rankedResults []search.RankedResult
	var warnings []string

	switch searchType {
	case "keyword":
		kwResults, err := s.config.OrgSearcher.SearchFunctionsOrg(ctx, orgID, query, maxResults)
		if err != nil {
			return errorResult(fmt.Sprintf("Keyword search failed: %v", err))
		}
		if repoFilter != nil {
			kwResults = filterRefsByRepos(kwResults, repoFilter)
		}
		rankedResults = search.MergeRRF(kwResults, nil, search.DefaultRRFK)

	case "semantic":
		if s.semanticSearch == nil {
			return errorResult("Semantic search is not configured. Run index_repository for each repo first.")
		}
		semResults, err := s.semanticSearch.SearchByOrg(ctx, query, orgID, maxResults)
		if err != nil {
			return errorResult(fmt.Sprintf("Semantic search failed: %v", err))
		}
		if repoFilter != nil {
			semResults = filterSemanticByRepos(semResults, repoFilter)
		}
		rankedResults = search.MergeRRF(nil, semResults, search.DefaultRRFK)

	case "hybrid":
		kwResults, err := s.config.OrgSearcher.SearchFunctionsOrg(ctx, orgID, query, maxResults)
		if err != nil {
			kwResults = nil
		}
		if repoFilter != nil {
			kwResults = filterRefsByRepos(kwResults, repoFilter)
		}

		var semResults []vectors.SearchResult
		if s.semanticSearch != nil {
			semResults, err = s.semanticSearch.SearchByOrg(ctx, query, orgID, maxResults)
			if err != nil {
				semResults = nil
				warnings = append(warnings, "Semantic search unavailable for this org. Results are keyword-only. Run index_repository for each repo to enable semantic search.")
			}
		} else {
			warnings = append(warnings, "Semantic search not configured. Results are keyword-only. Run index_repository for each repo to enable semantic search.")
		}
		if repoFilter != nil {
			semResults = filterSemanticByRepos(semResults, repoFilter)
		}

		rankedResults = search.MergeRRF(kwResults, semResults, search.DefaultRRFK)
	}

	if len(rankedResults) > maxResults {
		rankedResults = rankedResults[:maxResults]
	}

	output := FormatOrgSearchResult(query, orgID, rankedResults, tokenBudget)

	if len(warnings) > 0 {
		output += "\n\n**Note:** " + strings.Join(warnings, " ")
	}

	return callToolResult{
		Content: []contentItem{{Type: "text", Text: output}},
	}
}

// filterRefsByRepos filters FunctionRef results to only include repos in the allowed set.
func filterRefsByRepos(refs []ctxpkg.FunctionRef, repoFilter map[string]bool) []ctxpkg.FunctionRef {
	if repoFilter == nil {
		return refs
	}
	filtered := make([]ctxpkg.FunctionRef, 0, len(refs))
	for _, r := range refs {
		if repoFilter[r.RepoID] {
			filtered = append(filtered, r)
		}
	}
	return filtered
}

// filterSemanticByRepos filters semantic results to only include repos in the allowed set.
func filterSemanticByRepos(results []vectors.SearchResult, repoFilter map[string]bool) []vectors.SearchResult {
	if repoFilter == nil {
		return results
	}
	filtered := make([]vectors.SearchResult, 0, len(results))
	for _, r := range results {
		if repoFilter[r.Record.RepoID] {
			filtered = append(filtered, r)
		}
	}
	return filtered
}
