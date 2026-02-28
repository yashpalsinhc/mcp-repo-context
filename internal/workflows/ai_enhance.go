package workflows

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// EnhanceWithAI calls the AskFunc with a prompt and returns the AI-generated text.
func EnhanceWithAI(ctx context.Context, askFunc AskFunc, prompt string) (string, error) {
	if askFunc == nil {
		return "", nil
	}

	// Truncate prompt to 4000 chars
	if len(prompt) > 4000 {
		prompt = prompt[:4000]
	}

	// Create timeout context
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	result, err := askFunc(ctx, prompt, nil)
	if err != nil {
		return "", err
	}

	return result, nil
}

// BuildFeaturePrompt builds an AI prompt for a feature plan.
func BuildFeaturePrompt(plan *FeaturePlan) string {
	var sb strings.Builder

	fmt.Fprintf(&sb, "Given the following analysis for building feature %q:\n", plan.Feature)
	fmt.Fprintf(&sb, "- %d relevant functions found across %d repos\n",
		len(plan.RelevantCode), countDistinctRepos(plan.RelevantCode))

	if len(plan.EntryPoints) > 0 {
		names := make([]string, 0, len(plan.EntryPoints))
		for _, ep := range plan.EntryPoints {
			names = append(names, ep.FuncName)
		}
		fmt.Fprintf(&sb, "- Entry points: %s\n", truncateItems(names, 10))
	}

	if len(plan.Dependencies) > 0 {
		depNames := make([]string, 0, len(plan.Dependencies))
		for _, d := range plan.Dependencies {
			depNames = append(depNames, fmt.Sprintf("%s->%s", d.SourceRepoID, d.TargetRepoID))
		}
		fmt.Fprintf(&sb, "- Key dependencies: %s\n", truncateItems(depNames, 10))
	}

	sb.WriteString("\nProvide a brief implementation strategy (2-3 paragraphs).\n")

	return sb.String()
}

// BuildRefactorPrompt builds an AI prompt for a refactor plan.
func BuildRefactorPrompt(plan *RefactorPlan) string {
	var sb strings.Builder

	fmt.Fprintf(&sb, "The pattern %q appears %d times across %d repos.\n",
		plan.Pattern, len(plan.Usages), countDistinctRepos(plan.Usages))
	fmt.Fprintf(&sb, "Impact: %d functions directly affected, %d indirect callers.\n",
		plan.ImpactAnalysis.DirectCallers, plan.ImpactAnalysis.IndirectCallers)

	if len(plan.ImpactAnalysis.HotPaths) > 0 {
		fmt.Fprintf(&sb, "Hot paths: %s\n", truncateItems(plan.ImpactAnalysis.HotPaths, 5))
	}

	sb.WriteString("\nSuggest a safe refactoring approach (2-3 paragraphs).\n")

	return sb.String()
}

// truncateItems formats up to max items as comma-separated, with overflow indicator.
func truncateItems(items []string, max int) string {
	if len(items) == 0 {
		return "none"
	}
	if len(items) <= max {
		return strings.Join(items, ", ")
	}
	return strings.Join(items[:max], ", ") + fmt.Sprintf("... and %d more", len(items)-max)
}
