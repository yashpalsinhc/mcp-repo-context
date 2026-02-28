package workflows

import (
	"fmt"
	"strings"
)

// FormatFeaturePlan produces markdown output for a FeaturePlan within a token budget.
func FormatFeaturePlan(plan *FeaturePlan, budget int) string {
	if budget < 500 {
		return formatFeaturePlanSummary(plan)
	}

	charBudget := budget * 4
	var sb strings.Builder

	// Header
	sb.WriteString(fmt.Sprintf("# Feature Plan: %s\n\n", plan.Feature))
	sb.WriteString(fmt.Sprintf("**Organization:** %s\n\n", plan.OrgID))

	if len(plan.Warnings) > 0 {
		for _, w := range plan.Warnings {
			sb.WriteString(fmt.Sprintf("> **Warning:** %s\n", w))
		}
		sb.WriteString("\n")
	}

	// Budget allocation: 40% relevant code, 15% entry points, 15% dependencies, 15% files/order, 15% risk/AI
	codeAlloc := charBudget * 40 / 100
	entryAlloc := charBudget * 15 / 100
	depAlloc := charBudget * 15 / 100
	filesAlloc := charBudget * 15 / 100
	// rest for risk/AI

	// Relevant Code section
	sb.WriteString("## Relevant Code\n\n")
	if len(plan.RelevantCode) == 0 {
		sb.WriteString("No relevant code found.\n\n")
	} else {
		written := 0
		for i, loc := range plan.RelevantCode {
			line := fmt.Sprintf("- **%s** `%s` in `%s` (score: %.2f)", loc.FuncName, loc.FilePath, loc.RepoID, loc.Score)
			if loc.Summary != "" {
				line += fmt.Sprintf(" - %s", loc.Summary)
			}
			line += "\n"
			if written+len(line) > codeAlloc && i > 0 {
				remaining := len(plan.RelevantCode) - i
				sb.WriteString(fmt.Sprintf("- ... and %d more\n", remaining))
				break
			}
			sb.WriteString(line)
			written += len(line)
		}
		sb.WriteString("\n")
	}

	// Entry Points section
	sb.WriteString("## Entry Points\n\n")
	if len(plan.EntryPoints) == 0 {
		sb.WriteString("No entry points identified.\n\n")
	} else {
		written := 0
		for i, ep := range plan.EntryPoints {
			line := fmt.Sprintf("- **%s** in `%s` (%s) - %s\n", ep.FuncName, ep.FilePath, ep.RepoID, ep.Why)
			if written+len(line) > entryAlloc && i > 0 {
				remaining := len(plan.EntryPoints) - i
				sb.WriteString(fmt.Sprintf("- ... and %d more\n", remaining))
				break
			}
			sb.WriteString(line)
			written += len(line)
		}
		sb.WriteString("\n")
	}

	// Dependencies section
	sb.WriteString("## Dependencies\n\n")
	if len(plan.Dependencies) == 0 {
		sb.WriteString("No cross-repo dependencies detected.\n\n")
	} else {
		written := 0
		for i, dep := range plan.Dependencies {
			line := fmt.Sprintf("- `%s:%s` -> `%s:%s` (%s, confidence: %s)\n",
				dep.SourceRepoID, dep.SourceFunc, dep.TargetRepoID, dep.TargetFunc, dep.Type, dep.Confidence)
			if written+len(line) > depAlloc && i > 0 {
				remaining := len(plan.Dependencies) - i
				sb.WriteString(fmt.Sprintf("- ... and %d more\n", remaining))
				break
			}
			sb.WriteString(line)
			written += len(line)
		}
		sb.WriteString("\n")
	}

	// Files to Touch section
	sb.WriteString("## Files to Touch\n\n")
	if len(plan.FilesToTouch) == 0 {
		sb.WriteString("No files identified.\n\n")
	} else {
		written := 0
		for i, fa := range plan.FilesToTouch {
			line := fmt.Sprintf("- [%s] `%s` in `%s` - %s\n", fa.Action, fa.FilePath, fa.RepoID, fa.Reason)
			if written+len(line) > filesAlloc && i > 0 {
				remaining := len(plan.FilesToTouch) - i
				sb.WriteString(fmt.Sprintf("- ... and %d more\n", remaining))
				break
			}
			sb.WriteString(line)
			written += len(line)
		}
		sb.WriteString("\n")
	}

	// Implementation Order section
	sb.WriteString("## Implementation Order\n\n")
	if len(plan.SuggestedOrder) == 0 {
		sb.WriteString("No specific order suggested.\n\n")
	} else {
		for i, repo := range plan.SuggestedOrder {
			sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, repo))
		}
		sb.WriteString("\n")
	}

	// Risk Assessment section
	sb.WriteString("## Risk Assessment\n\n")
	sb.WriteString(fmt.Sprintf("**Risk Level:** %s\n\n", plan.RiskLevel))
	sb.WriteString(riskExplanation(plan.RiskLevel, len(plan.RelevantCode), countDistinctRepos(plan.RelevantCode)))

	// AI Analysis section
	if plan.AIEnhancement != "" {
		sb.WriteString("\n## AI Analysis\n\n")
		sb.WriteString(plan.AIEnhancement)
		sb.WriteString("\n")
	}

	result := sb.String()
	if len(result) > charBudget {
		result = result[:charBudget-20] + "\n\n... (truncated)\n"
	}
	return result
}

// FormatRefactorPlan produces markdown output for a RefactorPlan within a token budget.
func FormatRefactorPlan(plan *RefactorPlan, budget int) string {
	if budget < 500 {
		return formatRefactorPlanSummary(plan)
	}

	charBudget := budget * 4
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("# Refactor Plan: %s\n\n", plan.Pattern))
	sb.WriteString(fmt.Sprintf("**Organization:** %s\n\n", plan.OrgID))

	if len(plan.Warnings) > 0 {
		for _, w := range plan.Warnings {
			sb.WriteString(fmt.Sprintf("> **Warning:** %s\n", w))
		}
		sb.WriteString("\n")
	}

	// Usages section (~40%)
	usageAlloc := charBudget * 40 / 100
	sb.WriteString("## Usages\n\n")
	if len(plan.Usages) == 0 {
		sb.WriteString("No usages found.\n\n")
	} else {
		written := 0
		for i, loc := range plan.Usages {
			line := fmt.Sprintf("- **%s** `%s` in `%s` (score: %.2f)\n", loc.FuncName, loc.FilePath, loc.RepoID, loc.Score)
			if written+len(line) > usageAlloc && i > 0 {
				remaining := len(plan.Usages) - i
				sb.WriteString(fmt.Sprintf("- ... and %d more\n", remaining))
				break
			}
			sb.WriteString(line)
			written += len(line)
		}
		sb.WriteString("\n")
	}

	// Impact Analysis section (~25%)
	sb.WriteString("## Impact Analysis\n\n")
	sb.WriteString(fmt.Sprintf("- **Direct callers:** %d\n", plan.ImpactAnalysis.DirectCallers))
	sb.WriteString(fmt.Sprintf("- **Indirect callers:** %d\n", plan.ImpactAnalysis.IndirectCallers))
	if len(plan.ImpactAnalysis.AffectedRepos) > 0 {
		sb.WriteString(fmt.Sprintf("- **Affected repos:** %s\n", strings.Join(plan.ImpactAnalysis.AffectedRepos, ", ")))
	}
	if len(plan.ImpactAnalysis.HotPaths) > 0 {
		sb.WriteString(fmt.Sprintf("- **Hot paths (>5 callers):** %s\n", strings.Join(plan.ImpactAnalysis.HotPaths, ", ")))
	}
	sb.WriteString("\n")

	// Affected Files section (~20%)
	filesAlloc := charBudget * 20 / 100
	sb.WriteString("## Affected Files\n\n")
	if len(plan.AffectedFiles) == 0 {
		sb.WriteString("No affected files identified.\n\n")
	} else {
		written := 0
		for i, fa := range plan.AffectedFiles {
			line := fmt.Sprintf("- [%s] `%s` in `%s` - %s\n", fa.Action, fa.FilePath, fa.RepoID, fa.Reason)
			if written+len(line) > filesAlloc && i > 0 {
				remaining := len(plan.AffectedFiles) - i
				sb.WriteString(fmt.Sprintf("- ... and %d more\n", remaining))
				break
			}
			sb.WriteString(line)
			written += len(line)
		}
		sb.WriteString("\n")
	}

	// Risk Assessment
	sb.WriteString("## Risk Assessment\n\n")
	sb.WriteString(fmt.Sprintf("**Risk Level:** %s\n\n", plan.RiskLevel))
	sb.WriteString(riskExplanation(plan.RiskLevel, len(plan.Usages), len(plan.ImpactAnalysis.AffectedRepos)))

	// AI Analysis
	if plan.AIEnhancement != "" {
		sb.WriteString("\n## AI Analysis\n\n")
		sb.WriteString(plan.AIEnhancement)
		sb.WriteString("\n")
	}

	result := sb.String()
	if len(result) > charBudget {
		result = result[:charBudget-20] + "\n\n... (truncated)\n"
	}
	return result
}

// FormatMergeReport produces markdown output for a MergeReport within a token budget.
func FormatMergeReport(report *MergeReport, budget int) string {
	if budget < 500 {
		return formatMergeReportSummary(report)
	}

	charBudget := budget * 4
	var sb strings.Builder

	sb.WriteString("# Merge Strategy Report\n\n")
	sb.WriteString(fmt.Sprintf("**Target:** %s\n", report.TargetRepo))
	sb.WriteString(fmt.Sprintf("**Sources:** %s\n\n", strings.Join(report.SourceRepos, ", ")))

	if len(report.Warnings) > 0 {
		for _, w := range report.Warnings {
			sb.WriteString(fmt.Sprintf("> **Warning:** %s\n", w))
		}
		sb.WriteString("\n")
	}

	sectionAlloc := charBudget / 5

	// Duplicates section
	sb.WriteString("## Duplicates\n\n")
	if len(report.Duplicates) == 0 {
		sb.WriteString("No duplicates found.\n\n")
	} else {
		written := 0
		for i, dup := range report.Duplicates {
			line := fmt.Sprintf("- **%s** in repos %s (similarity: %.0f%%) - %s\n",
				dup.FunctionName, strings.Join(dup.Repos, ", "), dup.Similarity*100, dup.Recommendation)
			if written+len(line) > sectionAlloc && i > 0 {
				remaining := len(report.Duplicates) - i
				sb.WriteString(fmt.Sprintf("- ... and %d more\n", remaining))
				break
			}
			sb.WriteString(line)
			written += len(line)
		}
		sb.WriteString("\n")
	}

	// Conflicts section
	sb.WriteString("## Conflicts\n\n")
	if len(report.Conflicts) == 0 {
		sb.WriteString("No conflicts found.\n\n")
	} else {
		written := 0
		for i, c := range report.Conflicts {
			line := fmt.Sprintf("- **%s** [%s] severity=%s from %s - %s\n",
				c.FunctionName, c.Type, c.Severity, c.SourceRepo, c.Resolution)
			if written+len(line) > sectionAlloc && i > 0 {
				remaining := len(report.Conflicts) - i
				sb.WriteString(fmt.Sprintf("- ... and %d more\n", remaining))
				break
			}
			sb.WriteString(line)
			written += len(line)
		}
		sb.WriteString("\n")
	}

	// Gaps section
	sb.WriteString("## Gaps\n\n")
	if len(report.Gaps) == 0 {
		sb.WriteString("No gaps found.\n\n")
	} else {
		written := 0
		for i, g := range report.Gaps {
			line := fmt.Sprintf("- **%s** from %s (priority: %s) - %s\n",
				g.FunctionName, g.SourceRepo, g.Priority, g.Description)
			if written+len(line) > sectionAlloc && i > 0 {
				remaining := len(report.Gaps) - i
				sb.WriteString(fmt.Sprintf("- ... and %d more\n", remaining))
				break
			}
			sb.WriteString(line)
			written += len(line)
		}
		sb.WriteString("\n")
	}

	// Merge Steps section
	sb.WriteString("## Merge Steps\n\n")
	if len(report.MergeOrder) == 0 {
		sb.WriteString("No merge steps generated.\n\n")
	} else {
		written := 0
		for i, step := range report.MergeOrder {
			line := fmt.Sprintf("%d. [%s] %s (from %s, risk: %s) - %s\n",
				step.Order, step.Action, step.TargetItem, step.SourceRepo, step.Risk, step.Description)
			if written+len(line) > sectionAlloc && i > 0 {
				remaining := len(report.MergeOrder) - i
				sb.WriteString(fmt.Sprintf("- ... and %d more\n", remaining))
				break
			}
			sb.WriteString(line)
			written += len(line)
		}
		sb.WriteString("\n")
	}

	// Risk section
	sb.WriteString("## Risk Assessment\n\n")
	sb.WriteString(fmt.Sprintf("**Risk Level:** %s\n\n", report.RiskLevel))

	result := sb.String()
	if len(result) > charBudget {
		result = result[:charBudget-20] + "\n\n... (truncated)\n"
	}
	return result
}

// Summary-only formatters for budget < 500.

func formatFeaturePlanSummary(plan *FeaturePlan) string {
	return fmt.Sprintf("# Feature Plan: %s\n\nRelevant code: %d | Entry points: %d | Dependencies: %d | Risk: %s\n",
		plan.Feature, len(plan.RelevantCode), len(plan.EntryPoints), len(plan.Dependencies), plan.RiskLevel)
}

func formatRefactorPlanSummary(plan *RefactorPlan) string {
	return fmt.Sprintf("# Refactor Plan: %s\n\nUsages: %d | Direct callers: %d | Affected repos: %d | Risk: %s\n",
		plan.Pattern, len(plan.Usages), plan.ImpactAnalysis.DirectCallers, len(plan.ImpactAnalysis.AffectedRepos), plan.RiskLevel)
}

func formatMergeReportSummary(report *MergeReport) string {
	return fmt.Sprintf("# Merge Report: %s\n\nDuplicates: %d | Conflicts: %d | Gaps: %d | Steps: %d | Risk: %s\n",
		report.TargetRepo, len(report.Duplicates), len(report.Conflicts), len(report.Gaps), len(report.MergeOrder), report.RiskLevel)
}

// Helper: count distinct repos in a set of code locations.
func countDistinctRepos(locations []CodeLocation) int {
	repos := make(map[string]bool)
	for _, loc := range locations {
		repos[loc.RepoID] = true
	}
	return len(repos)
}

// Helper: risk explanation text.
func riskExplanation(level RiskLevel, funcCount, repoCount int) string {
	switch level {
	case RiskHigh:
		return fmt.Sprintf("High risk: %d functions across %d repos. Careful coordination required.\n", funcCount, repoCount)
	case RiskMedium:
		return fmt.Sprintf("Medium risk: %d functions across %d repos. Standard review process recommended.\n", funcCount, repoCount)
	default:
		return fmt.Sprintf("Low risk: %d functions across %d repos. Straightforward change.\n", funcCount, repoCount)
	}
}
