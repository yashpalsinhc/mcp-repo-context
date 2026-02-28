package workflows

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/yashpalc/mcp-repo-context/internal/comparison"
	ctxpkg "github.com/yashpalc/mcp-repo-context/internal/context"
)

// MergeReposParams configures a merge_repos workflow.
type MergeReposParams struct {
	SourceRepoIDs []string
	TargetRepoID  string
	TokenBudget   int
	Searcher      RepoSearcher
	Comparer      comparison.Comparer
}

// MergeRepos generates a merge strategy report for consolidating repositories.
func MergeRepos(ctx context.Context, params MergeReposParams) (*MergeReport, error) {
	if len(params.SourceRepoIDs) == 0 {
		return nil, fmt.Errorf("at least one source repository required")
	}
	if params.TokenBudget < 500 {
		params.TokenBudget = 500
	}
	if params.TokenBudget > 32000 {
		params.TokenBudget = 32000
	}

	// Remove target from sources if present
	targetInSources := false
	var effectiveSources []string
	for _, s := range params.SourceRepoIDs {
		if s == params.TargetRepoID {
			targetInSources = true
			continue
		}
		effectiveSources = append(effectiveSources, s)
	}
	if len(effectiveSources) == 0 {
		return nil, fmt.Errorf("no source repositories after removing target")
	}

	// Load contexts
	var sourceContexts []*ctxpkg.RepoContext
	for _, id := range effectiveSources {
		rc, err := params.Searcher.GetContext(ctx, id)
		if err != nil {
			return nil, fmt.Errorf("failed to load context for %q: %w", id, err)
		}
		sourceContexts = append(sourceContexts, rc)
	}

	targetContext, err := params.Searcher.GetContext(ctx, params.TargetRepoID)
	if err != nil {
		return nil, fmt.Errorf("failed to load target context for %q: %w", params.TargetRepoID, err)
	}

	allContexts := append(sourceContexts, targetContext)

	report := &MergeReport{
		SourceRepos: effectiveSources,
		TargetRepo:  params.TargetRepoID,
	}

	// Run comparison analyses (continue on individual failures)
	duplicates, dupErr := params.Comparer.FindDuplicates(ctx, allContexts)
	if dupErr == nil {
		report.Duplicates = convertDuplicates(duplicates)
	}

	conflicts, confErr := params.Comparer.FindConflicts(ctx, sourceContexts, targetContext)
	if confErr == nil {
		report.Conflicts = convertConflicts(conflicts)
	}

	gaps, gapErr := params.Comparer.FindGaps(ctx, sourceContexts, targetContext)
	if gapErr == nil {
		report.Gaps = convertGaps(gaps)
	}

	if dupErr != nil && confErr != nil && gapErr != nil {
		return nil, fmt.Errorf("all comparison analyses failed")
	}

	// Generate merge order
	report.MergeOrder = generateMergeOrder(effectiveSources, report.Duplicates, report.Conflicts, report.Gaps)

	// Risk assessment
	report.RiskLevel = assessMergeRisk(report.Conflicts, report.Gaps)

	_ = targetInSources // used for warning in formatted output

	return report, nil
}

func convertDuplicates(groups []comparison.DuplicateGroup) []DuplicateSummary {
	var result []DuplicateSummary
	for _, g := range groups {
		var repos []string
		seen := make(map[string]bool)
		for _, inst := range g.Instances {
			if !seen[inst.RepoID] {
				repos = append(repos, inst.RepoID)
				seen[inst.RepoID] = true
			}
		}
		result = append(result, DuplicateSummary{
			FunctionName:   g.Name,
			Repos:          repos,
			Similarity:     g.Similarity,
			Recommendation: g.Recommendation,
		})
	}
	return result
}

func convertConflicts(conflicts []comparison.Conflict) []ConflictSummary {
	var result []ConflictSummary
	for _, c := range conflicts {
		sourceRepo := ""
		if len(c.SourceInstances) > 0 {
			sourceRepo = c.SourceInstances[0].RepoID
		}
		result = append(result, ConflictSummary{
			FunctionName: c.Name,
			Type:         c.Type,
			Severity:     c.Severity,
			SourceRepo:   sourceRepo,
			Resolution:   c.Resolution,
		})
	}
	return result
}

func convertGaps(gaps []comparison.Gap) []GapSummary {
	var result []GapSummary
	for _, g := range gaps {
		sourceRepo := ""
		if len(g.SourceRepos) > 0 {
			sourceRepo = g.SourceRepos[0]
		}
		result = append(result, GapSummary{
			FunctionName: g.Name,
			SourceRepo:   sourceRepo,
			Priority:     g.Priority,
			Description:  g.Description,
		})
	}
	return result
}

func generateMergeOrder(sources []string, duplicates []DuplicateSummary, conflicts []ConflictSummary, gaps []GapSummary) []MergeStep {
	var steps []MergeStep
	order := 1

	// Sort sources alphabetically for deterministic output
	sorted := make([]string, len(sources))
	copy(sorted, sources)
	sort.Strings(sorted)

	// Add duplicate resolution steps
	for _, d := range duplicates {
		steps = append(steps, MergeStep{
			Order:       order,
			Action:      "consolidate_duplicate",
			SourceRepo:  strings.Join(d.Repos, ", "),
			TargetItem:  d.FunctionName,
			Description: fmt.Sprintf("Consolidate %s (%s)", d.FunctionName, d.Recommendation),
			Risk:        RiskLow,
		})
		order++
	}

	// Add conflict resolution steps
	for _, c := range conflicts {
		risk := RiskMedium
		if c.Severity == "high" {
			risk = RiskHigh
		}
		steps = append(steps, MergeStep{
			Order:       order,
			Action:      "resolve_conflict",
			SourceRepo:  c.SourceRepo,
			TargetItem:  c.FunctionName,
			Description: fmt.Sprintf("Resolve %s conflict for %s: %s", c.Type, c.FunctionName, c.Resolution),
			Risk:        risk,
		})
		order++
	}

	// Add gap filling steps
	for _, g := range gaps {
		steps = append(steps, MergeStep{
			Order:       order,
			Action:      "fill_gap",
			SourceRepo:  g.SourceRepo,
			TargetItem:  g.FunctionName,
			Description: fmt.Sprintf("Migrate %s: %s", g.FunctionName, g.Description),
			Risk:        RiskLow,
		})
		order++
	}

	return steps
}

func assessMergeRisk(conflicts []ConflictSummary, gaps []GapSummary) RiskLevel {
	highConflicts := 0
	for _, c := range conflicts {
		if c.Severity == "high" {
			highConflicts++
		}
	}

	if highConflicts > 0 || len(conflicts) > 10 {
		return RiskHigh
	}
	if len(conflicts) > 0 || len(gaps) > 20 {
		return RiskMedium
	}
	return RiskLow
}
