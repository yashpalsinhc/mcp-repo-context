package workflows

import (
	"context"
	"fmt"
	"strings"
)

// RefactorOrgParams configures a refactor_org workflow.
type RefactorOrgParams struct {
	OrgID              string
	PatternDescription string
	TargetRepos        []string
	TokenBudget        int
	Searcher           RepoSearcher
	OrgResolver        OrgResolver
	Vectors            VectorSearcher // may be nil
}

// knownConcepts is the list of known concept keywords for concept search.
var knownConcepts = []string{
	"authentication", "validation", "handler", "crud", "middleware",
	"error", "database", "http", "logging",
}

// RefactorOrg plans a refactoring across repos in an org.
func RefactorOrg(ctx context.Context, params RefactorOrgParams) (*RefactorPlan, error) {
	if len(params.PatternDescription) < 3 {
		return nil, fmt.Errorf("pattern_description must be at least 3 characters")
	}
	if params.TokenBudget < 500 {
		params.TokenBudget = 500
	}
	if params.TokenBudget > 32000 {
		params.TokenBudget = 32000
	}

	repos, err := params.OrgResolver.GetOrgRepos(ctx, params.OrgID)
	if err != nil {
		return nil, fmt.Errorf("organization %q not found: %w", params.OrgID, err)
	}
	if len(repos) == 0 {
		return nil, fmt.Errorf("no repositories in org %q; run analyze_org first", params.OrgID)
	}

	if len(params.TargetRepos) > 0 {
		repos = filterStringSlice(repos, params.TargetRepos)
		if len(repos) == 0 {
			return nil, fmt.Errorf("none of target_repos found in org %q", params.OrgID)
		}
	}

	plan := &RefactorPlan{
		Pattern: params.PatternDescription,
		OrgID:   params.OrgID,
	}

	// Find usages: semantic + concept search merged
	usages := findPatternUsages(ctx, params, repos)

	// Deduplicate and sort
	usages = deduplicateLocations(usages)
	SortByScore(usages)
	if len(usages) > 30 {
		usages = usages[:30]
	}
	plan.Usages = usages

	// Impact analysis
	plan.ImpactAnalysis = analyzeImpact(ctx, params.Searcher, usages, repos)

	// Affected files
	plan.AffectedFiles = collectAffectedFiles(ctx, params.Searcher, usages)

	// Risk assessment
	plan.RiskLevel = assessRefactorRisk(plan.ImpactAnalysis, len(usages))

	return plan, nil
}

func findPatternUsages(ctx context.Context, params RefactorOrgParams, repos []string) []CodeLocation {
	var results []CodeLocation

	// Semantic search
	if params.Vectors != nil {
		for _, repoID := range repos {
			vResults, err := params.Vectors.SearchAll(ctx, params.PatternDescription, repoID, 30)
			if err != nil {
				continue
			}
			for _, vr := range vResults {
				results = append(results, CodeLocation{
					RepoID:   vr.RepoID,
					FilePath: vr.FilePath,
					FuncName: vr.Name,
					Line:     vr.Line,
					Summary:  vr.Summary,
					Score:    vr.Score,
				})
			}
		}
	}

	// Concept search if pattern matches known concept
	concept := containsKeyword(params.PatternDescription, knownConcepts)
	if concept != "" {
		for _, repoID := range repos {
			refs, err := params.Searcher.SearchByConcept(ctx, repoID, concept)
			if err != nil {
				continue
			}
			for _, ref := range refs {
				results = append(results, CodeLocation{
					RepoID:   repoID,
					FilePath: ref.File,
					FuncName: ref.Function,
					Line:     ref.Line,
					Summary:  ref.Summary,
					Score:    0.6, // Default concept search score
				})
			}
		}
	}

	// Keyword fallback if nothing found
	if len(results) == 0 {
		for _, repoID := range repos {
			refs, err := params.Searcher.SearchFunctions(ctx, repoID, params.PatternDescription)
			if err != nil {
				continue
			}
			for _, ref := range refs {
				results = append(results, CodeLocation{
					RepoID:   repoID,
					FilePath: ref.File,
					FuncName: ref.Function,
					Line:     ref.Line,
					Summary:  ref.Summary,
					Score:    0.5,
				})
			}
		}
	}

	return results
}

func deduplicateLocations(locs []CodeLocation) []CodeLocation {
	seen := make(map[string]int) // key -> index in result
	var result []CodeLocation

	for _, loc := range locs {
		key := loc.RepoID + ":" + loc.FilePath + ":" + loc.FuncName
		if idx, ok := seen[key]; ok {
			// Keep higher score
			if loc.Score > result[idx].Score {
				result[idx] = loc
			}
			continue
		}
		seen[key] = len(result)
		result = append(result, loc)
	}
	return result
}

func analyzeImpact(ctx context.Context, searcher RepoSearcher, usages []CodeLocation, repos []string) ImpactSummary {
	impact := ImpactSummary{}
	affectedRepos := make(map[string]bool)
	limit := 20
	if limit > len(usages) {
		limit = len(usages)
	}

	for _, usage := range usages[:limit] {
		affectedRepos[usage.RepoID] = true

		callers, err := searcher.GetCallers(ctx, usage.RepoID, usage.FuncName)
		if err != nil {
			continue
		}

		directCount := len(callers)
		impact.DirectCallers += directCount

		if directCount > 5 {
			impact.HotPaths = append(impact.HotPaths, usage.FuncName)
		}

		// One level of indirect callers
		for _, caller := range callers {
			indirectCallers, err := searcher.GetCallers(ctx, usage.RepoID, caller.Function)
			if err != nil {
				continue
			}
			impact.IndirectCallers += len(indirectCallers)
		}
	}

	for repo := range affectedRepos {
		impact.AffectedRepos = append(impact.AffectedRepos, repo)
	}

	return impact
}

func collectAffectedFiles(ctx context.Context, searcher RepoSearcher, usages []CodeLocation) []FileAction {
	seen := make(map[string]string) // key -> action
	var files []FileAction

	for _, u := range usages {
		key := u.RepoID + ":" + u.FilePath
		if _, ok := seen[key]; !ok {
			seen[key] = "modify"
			files = append(files, FileAction{
				RepoID:   u.RepoID,
				FilePath: u.FilePath,
				Action:   "modify",
				Reason:   "Contains pattern usage: " + u.FuncName,
			})
		}

		// Caller files
		callers, err := searcher.GetCallers(ctx, u.RepoID, u.FuncName)
		if err != nil {
			continue
		}
		for _, caller := range callers {
			cKey := u.RepoID + ":" + caller.File
			if existing, ok := seen[cKey]; ok && existing == "modify" {
				continue // modify takes priority
			}
			if _, ok := seen[cKey]; !ok {
				seen[cKey] = "review"
				files = append(files, FileAction{
					RepoID:   u.RepoID,
					FilePath: caller.File,
					Action:   "review",
					Reason:   "Calls affected function " + u.FuncName,
				})
			}
		}
	}

	return files
}

func assessRefactorRisk(impact ImpactSummary, usageCount int) RiskLevel {
	repoCount := len(impact.AffectedRepos)
	funcCount := impact.DirectCallers + usageCount

	if repoCount > 5 || funcCount > 20 {
		return RiskHigh
	}
	if repoCount >= 2 || funcCount >= 5 {
		return RiskMedium
	}
	return RiskLow
}

// countDistinctReposFromStrings counts unique repo IDs from a string slice.
func countDistinctReposFromStrings(repos []string) int {
	m := make(map[string]bool)
	for _, r := range repos {
		m[r] = true
	}
	return len(m)
}

// extractConcept checks if patternDescription maps to a known concept.
func extractConcept(patternDescription string) string {
	return containsKeyword(patternDescription, knownConcepts)
}

// truncateList formats a slice of strings to max items, with "... and N more" suffix.
func truncateList(items []string, max int) string {
	if len(items) == 0 {
		return "none"
	}
	if len(items) <= max {
		return strings.Join(items, ", ")
	}
	return strings.Join(items[:max], ", ") + fmt.Sprintf("... and %d more", len(items)-max)
}
