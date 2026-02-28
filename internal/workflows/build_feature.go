package workflows

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"unicode"

	ctxpkg "github.com/yashpalc/mcp-repo-context/internal/context"
)

// BuildFeatureParams configures the build_feature workflow.
type BuildFeatureParams struct {
	OrgID              string
	FeatureDescription string
	TargetRepos        []string
	TokenBudget        int
	Searcher           RepoSearcher
	OrgResolver        OrgResolver
	Vectors            VectorSearcher // may be nil
}

// BuildFeature plans how to build a feature across repositories in an organization.
func BuildFeature(ctx context.Context, params BuildFeatureParams) (*FeaturePlan, error) {
	// Validate inputs
	if len(params.FeatureDescription) < 3 {
		return nil, fmt.Errorf("feature_description must be at least 3 characters")
	}

	params.TokenBudget = clampBudget(params.TokenBudget, 4000)

	// Load org
	repos, err := params.OrgResolver.GetOrgRepos(ctx, params.OrgID)
	if err != nil {
		return nil, fmt.Errorf("organization '%s' not found: %w", params.OrgID, err)
	}
	if len(repos) == 0 {
		return nil, fmt.Errorf("no repositories in org. Run analyze_org first")
	}

	// Validate repos are analyzed
	repoContexts, unanalyzed := loadRepoContexts(ctx, params.Searcher, repos)
	if len(repoContexts) == 0 {
		return nil, fmt.Errorf("repositories not analyzed: %s. Run analyze_org", strings.Join(unanalyzed, ", "))
	}

	// Filter to target repos if specified
	if len(params.TargetRepos) > 0 {
		repos = filterStringSlice(repos, params.TargetRepos)
		repoContexts = filterRepoContexts(repoContexts, params.TargetRepos)
	}

	plan := &FeaturePlan{
		Feature:   params.FeatureDescription,
		OrgID:     params.OrgID,
		RiskLevel: RiskLow,
	}

	// Step 1: Semantic search
	semanticFallback := false
	var relevantCode []CodeLocation
	if params.Vectors != nil {
		for _, repoID := range repos {
			vResults, vErr := params.Vectors.SearchAll(ctx, params.FeatureDescription, repoID, 20)
			if vErr != nil {
				continue
			}
			for _, vr := range vResults {
				relevantCode = append(relevantCode, CodeLocation{
					RepoID:   vr.RepoID,
					FilePath: vr.FilePath,
					FuncName: vr.Name,
					Line:     vr.Line,
					Score:    vr.Score,
					Summary:  vr.Summary,
				})
			}
		}
	}

	if len(relevantCode) == 0 {
		// Fallback to keyword search
		semanticFallback = true
		relevantCode = searchKeywordFromContexts(repoContexts, params.FeatureDescription)
	}

	// Filter by target repos
	if len(params.TargetRepos) > 0 {
		relevantCode = filterCodeLocations(relevantCode, params.TargetRepos)
	}

	SortByScore(relevantCode)
	if len(relevantCode) > 20 {
		relevantCode = relevantCode[:20]
	}
	plan.RelevantCode = relevantCode

	if semanticFallback {
		plan.Warnings = append(plan.Warnings, "semantic search unavailable, using keyword fallback")
	}

	// Step 2: Identify entry points
	plan.EntryPoints = identifyEntryPoints(ctx, params.Searcher, relevantCode)

	// Step 3: Detect dependencies
	plan.Dependencies = detectDependencies(repoContexts, relevantCode)

	// Filter dependencies by target repos
	if len(params.TargetRepos) > 0 {
		plan.Dependencies = filterDependencies(plan.Dependencies, params.TargetRepos)
	}

	// Step 4: Files to touch
	plan.FilesToTouch = buildFileActions(relevantCode, plan.EntryPoints)

	// Step 5: Implementation order
	plan.SuggestedOrder = suggestOrder(plan.Dependencies, repoContexts)

	// Step 6: Risk assessment
	plan.RiskLevel = assessFeatureRisk(relevantCode, plan.EntryPoints)

	return plan, nil
}

// searchKeywordFromContexts falls back to keyword-based search across repo contexts.
func searchKeywordFromContexts(repos map[string]*ctxpkg.RepoContext, query string) []CodeLocation {
	var locations []CodeLocation
	queryLower := strings.ToLower(query)
	words := strings.Fields(queryLower)

	for repoID, repoCtx := range repos {
		for filePath, fc := range repoCtx.Files {
			for _, fn := range fc.Functions {
				score := keywordScore(fn.Name, fn.Description, words)
				if fn.Behavior != nil && fn.Behavior.Summary != "" {
					score += keywordScore(fn.Behavior.Summary, "", words)
				}
				if score > 0 {
					summary := ""
					if fn.Behavior != nil {
						summary = fn.Behavior.Summary
					}
					locations = append(locations, CodeLocation{
						RepoID:   repoID,
						FilePath: filePath,
						FuncName: fn.Name,
						Line:     fn.LineStart,
						Score:    score,
						Summary:  summary,
					})
				}
			}
		}
	}
	return locations
}

// keywordScore computes a simple relevance score based on keyword matching.
func keywordScore(name, description string, words []string) float64 {
	score := 0.0
	nameLower := strings.ToLower(name)
	descLower := strings.ToLower(description)
	for _, w := range words {
		if strings.Contains(nameLower, w) {
			score += 0.5
		}
		if strings.Contains(descLower, w) {
			score += 0.3
		}
	}
	return score
}

// identifyEntryPoints finds public functions with few callers that are likely entry points.
func identifyEntryPoints(ctx context.Context, searcher RepoSearcher, locations []CodeLocation) []EntryPoint {
	var entryPoints []EntryPoint
	seen := make(map[string]bool)

	for _, loc := range locations {
		if !isExported(loc.FuncName) {
			continue
		}
		key := loc.RepoID + ":" + loc.FuncName
		if seen[key] {
			continue
		}
		seen[key] = true

		// Check caller count
		callers, err := searcher.GetCallers(ctx, loc.RepoID, loc.FuncName)
		if err != nil {
			// If we can't check callers, use repo context fallback
			repoCtx, rcErr := searcher.GetContext(ctx, loc.RepoID)
			if rcErr != nil {
				continue
			}
			callerCount := countCallersInContext(repoCtx, loc.FuncName)
			if callerCount <= 2 {
				why := "public function with few callers"
				if isHandlerLike(loc.FuncName, loc.FilePath) {
					why = "public handler/endpoint"
				}
				entryPoints = append(entryPoints, EntryPoint{
					RepoID:   loc.RepoID,
					FilePath: loc.FilePath,
					FuncName: loc.FuncName,
					Why:      why,
				})
			}
			continue
		}

		if len(callers) <= 2 {
			why := "public function with few callers"
			if isHandlerLike(loc.FuncName, loc.FilePath) {
				why = "public handler/endpoint"
			}
			entryPoints = append(entryPoints, EntryPoint{
				RepoID:   loc.RepoID,
				FilePath: loc.FilePath,
				FuncName: loc.FuncName,
				Why:      why,
			})
		}
	}
	return entryPoints
}

// detectDependencies finds cross-repo name-based dependencies.
func detectDependencies(repos map[string]*ctxpkg.RepoContext, locations []CodeLocation) []Dependency {
	var deps []Dependency
	seen := make(map[string]bool)

	// Build function index: funcName -> []repoID
	funcIndex := make(map[string][]string)
	for repoID, repoCtx := range repos {
		for _, fc := range repoCtx.Files {
			for _, fn := range fc.Functions {
				funcIndex[fn.Name] = append(funcIndex[fn.Name], repoID)
			}
		}
	}

	// For top relevant functions, check callees
	limit := 10
	if len(locations) < limit {
		limit = len(locations)
	}

	for _, loc := range locations[:limit] {
		repoCtx, ok := repos[loc.RepoID]
		if !ok {
			continue
		}
		for _, fc := range repoCtx.Files {
			for _, fn := range fc.Functions {
				if fn.Name != loc.FuncName {
					continue
				}
				for _, call := range fn.Calls {
					calledRepos := funcIndex[call.Function]
					for _, targetRepoID := range calledRepos {
						if targetRepoID == loc.RepoID {
							continue
						}
						key := loc.RepoID + ":" + targetRepoID + ":" + call.Function
						if seen[key] {
							continue
						}
						seen[key] = true
						deps = append(deps, Dependency{
							SourceRepoID: loc.RepoID,
							SourceFunc:   loc.FuncName,
							TargetRepoID: targetRepoID,
							TargetFunc:   call.Function,
							Type:         "name_match",
							Confidence:   "medium",
						})
					}
				}
			}
		}
	}
	return deps
}

// buildFileActions creates FileAction entries from relevant code and entry points.
func buildFileActions(locations []CodeLocation, entryPoints []EntryPoint) []FileAction {
	seen := make(map[string]bool)
	var actions []FileAction

	for _, ep := range entryPoints {
		key := ep.RepoID + ":" + ep.FilePath
		if seen[key] {
			continue
		}
		seen[key] = true
		actions = append(actions, FileAction{
			RepoID:   ep.RepoID,
			FilePath: ep.FilePath,
			Action:   "modify",
			Reason:   "Contains entry point " + ep.FuncName,
		})
	}

	for _, loc := range locations {
		key := loc.RepoID + ":" + loc.FilePath
		if seen[key] {
			continue
		}
		seen[key] = true
		actions = append(actions, FileAction{
			RepoID:   loc.RepoID,
			FilePath: loc.FilePath,
			Action:   "modify",
			Reason:   "Contains relevant function " + loc.FuncName,
		})
	}
	return actions
}

// suggestOrder suggests repo implementation order based on dependency in-degree.
func suggestOrder(deps []Dependency, repos map[string]*ctxpkg.RepoContext) []string {
	if len(repos) == 0 {
		return nil
	}

	inDegree := make(map[string]int)
	for repoID := range repos {
		inDegree[repoID] = 0
	}
	for _, dep := range deps {
		inDegree[dep.TargetRepoID]++
	}

	repoIDs := make([]string, 0, len(repos))
	for repoID := range repos {
		repoIDs = append(repoIDs, repoID)
	}

	sort.Slice(repoIDs, func(i, j int) bool {
		if inDegree[repoIDs[i]] != inDegree[repoIDs[j]] {
			return inDegree[repoIDs[i]] > inDegree[repoIDs[j]]
		}
		return repoIDs[i] < repoIDs[j]
	})

	return repoIDs
}

// assessFeatureRisk computes risk level based on scope.
func assessFeatureRisk(locations []CodeLocation, entryPoints []EntryPoint) RiskLevel {
	repoCount := countDistinctRepos(locations)
	funcCount := len(locations)

	hasPublicAPIWithCallers := false
	for _, ep := range entryPoints {
		if isHandlerLike(ep.FuncName, ep.FilePath) {
			hasPublicAPIWithCallers = true
		}
	}

	if repoCount > 5 || funcCount > 20 || hasPublicAPIWithCallers {
		return RiskHigh
	}
	if repoCount >= 2 || funcCount >= 5 {
		return RiskMedium
	}
	return RiskLow
}

// -- Helpers --

func clampBudget(budget, defaultVal int) int {
	if budget <= 0 {
		return defaultVal
	}
	if budget < 500 {
		return 500
	}
	if budget > 32000 {
		return 32000
	}
	return budget
}

func loadRepoContexts(ctx context.Context, searcher RepoSearcher, repoIDs []string) (map[string]*ctxpkg.RepoContext, []string) {
	repos := make(map[string]*ctxpkg.RepoContext)
	var unanalyzed []string
	for _, id := range repoIDs {
		rc, err := searcher.GetContext(ctx, id)
		if err != nil {
			unanalyzed = append(unanalyzed, id)
			continue
		}
		repos[id] = rc
	}
	return repos, unanalyzed
}

func filterStringSlice(items []string, allowed []string) []string {
	target := make(map[string]bool)
	for _, r := range allowed {
		target[r] = true
	}
	var filtered []string
	for _, item := range items {
		if target[item] {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

func filterRepoContexts(repos map[string]*ctxpkg.RepoContext, targetRepos []string) map[string]*ctxpkg.RepoContext {
	target := make(map[string]bool)
	for _, r := range targetRepos {
		target[r] = true
	}
	filtered := make(map[string]*ctxpkg.RepoContext)
	for id, rc := range repos {
		if target[id] {
			filtered[id] = rc
		}
	}
	return filtered
}

func filterCodeLocations(locations []CodeLocation, targetRepos []string) []CodeLocation {
	target := make(map[string]bool)
	for _, r := range targetRepos {
		target[r] = true
	}
	var filtered []CodeLocation
	for _, loc := range locations {
		if target[loc.RepoID] {
			filtered = append(filtered, loc)
		}
	}
	return filtered
}

func filterDependencies(deps []Dependency, targetRepos []string) []Dependency {
	target := make(map[string]bool)
	for _, r := range targetRepos {
		target[r] = true
	}
	var filtered []Dependency
	for _, dep := range deps {
		if target[dep.SourceRepoID] || target[dep.TargetRepoID] {
			filtered = append(filtered, dep)
		}
	}
	return filtered
}

func isExported(name string) bool {
	if name == "" {
		return false
	}
	return unicode.IsUpper(rune(name[0]))
}

func isHandlerLike(funcName, filePath string) bool {
	nameLower := strings.ToLower(funcName)
	pathLower := strings.ToLower(filePath)
	return strings.Contains(nameLower, "handler") ||
		strings.Contains(nameLower, "endpoint") ||
		strings.Contains(pathLower, "handler") ||
		strings.HasPrefix(nameLower, "get") ||
		strings.HasPrefix(nameLower, "post") ||
		strings.HasPrefix(nameLower, "create") ||
		strings.HasPrefix(nameLower, "update") ||
		strings.HasPrefix(nameLower, "delete") ||
		strings.HasPrefix(nameLower, "list")
}

func countCallersInContext(repoCtx *ctxpkg.RepoContext, funcName string) int {
	count := 0
	for _, fc := range repoCtx.Files {
		for _, fn := range fc.Functions {
			for _, call := range fn.Calls {
				if call.Function == funcName {
					count++
				}
			}
		}
	}
	return count
}

// containsKeyword returns the first keyword found in text from the list, or "".
func containsKeyword(text string, keywords []string) string {
	lower := strings.ToLower(text)
	for _, kw := range keywords {
		if strings.Contains(lower, kw) {
			return kw
		}
	}
	return ""
}
