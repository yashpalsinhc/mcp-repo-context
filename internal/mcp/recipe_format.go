package mcp

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/yashpalc/mcp-repo-context/internal/recipes"
)

// formatRecipeResultMarkdown formats a RecipeResult as markdown.
func formatRecipeResultMarkdown(result *recipes.RecipeResult) string {
	var sb strings.Builder

	fmt.Fprintf(&sb, "# %s\n\n", result.Recipe)

	// Data sections
	for key, value := range result.Data {
		sb.WriteString(formatDataSection(key, value))
	}

	// AI Analysis
	if result.Analysis != "" {
		sb.WriteString("## Analysis\n\n")
		sb.WriteString(result.Analysis)
		sb.WriteString("\n\n")
	}

	// Gaps
	if len(result.Gaps) > 0 {
		sb.WriteString("## Gaps\n\n")
		for _, gap := range result.Gaps {
			fmt.Fprintf(&sb, "- **%s:** %s\n", gap.Section, gap.Reason)
			if gap.Suggestion != "" {
				fmt.Fprintf(&sb, "  - Suggestion: %s\n", gap.Suggestion)
			}
		}
		sb.WriteString("\n")
	}

	// Sources
	if len(result.Sources) > 0 {
		sb.WriteString("## Sources\n\n")
		for _, src := range result.Sources {
			if src.Line > 0 {
				fmt.Fprintf(&sb, "- %s:%d (%s)\n", src.File, src.Line, src.Claim)
			} else if src.File != "" {
				fmt.Fprintf(&sb, "- %s (%s)\n", src.File, src.Claim)
			}
		}
		sb.WriteString("\n")
	}

	// Footer
	sb.WriteString("---\n")
	fmt.Fprintf(&sb, "Context tokens: %d | AI tokens: %d | Duration: %dms | Confidence: %.2f\n",
		result.ContextTokens, result.AITokens, result.DurationMS, result.Confidence)

	return sb.String()
}

// formatDataSection formats a single data key-value pair as markdown.
func formatDataSection(key string, value interface{}) string {
	var sb strings.Builder

	// Convert key to title case
	title := strings.ReplaceAll(key, "_", " ")
	title = strings.Title(title) //nolint:staticcheck
	fmt.Fprintf(&sb, "## %s\n\n", title)

	switch v := value.(type) {
	case nil:
		sb.WriteString("_Not available_\n\n")
	case string:
		sb.WriteString(v)
		sb.WriteString("\n\n")
	case []interface{}:
		if len(v) == 0 {
			sb.WriteString("_None_\n\n")
		} else {
			for _, item := range v {
				formatListItem(&sb, item)
			}
			sb.WriteString("\n")
		}
	case []recipes.FlowStep:
		if len(v) == 0 {
			sb.WriteString("_None_\n\n")
		} else {
			for _, step := range v {
				fmt.Fprintf(&sb, "- **%s** (%s:%d)", step.Function, step.File, step.Line)
				if step.Action != "" {
					fmt.Fprintf(&sb, " - %s", step.Action)
				}
				sb.WriteString("\n")
				if len(step.SideEffects) > 0 {
					fmt.Fprintf(&sb, "  - Side effects: %s\n", strings.Join(step.SideEffects, ", "))
				}
			}
			sb.WriteString("\n")
		}
	case []recipes.ServiceInfo:
		for _, svc := range v {
			fmt.Fprintf(&sb, "- **%s** (`%s`) - %s/%s, %d functions",
				svc.Name, svc.RepoID, svc.Language, svc.Framework, svc.FunctionCount)
			if svc.Purpose != "" {
				fmt.Fprintf(&sb, " - %s", svc.Purpose)
			}
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	case []recipes.SharedPattern:
		for _, p := range v {
			fmt.Fprintf(&sb, "- **%s:** %s (used by %d repos: %s)\n",
				p.Pattern, p.Value, p.RepoCount, strings.Join(p.Repos, ", "))
		}
		sb.WriteString("\n")
	case recipes.RecipeRiskAssessment:
		fmt.Fprintf(&sb, "**Level:** %s (confidence: %.1f)\n", v.Level, v.Confidence)
		if v.Reasoning != "" {
			fmt.Fprintf(&sb, "Reasoning: %s\n", v.Reasoning)
		}
		sb.WriteString("\n")
	case map[string]interface{}:
		for k, val := range v {
			fmt.Fprintf(&sb, "- **%s:** %v\n", k, val)
		}
		sb.WriteString("\n")
	case map[string]recipes.RepoHealth:
		for repoID, health := range v {
			fmt.Fprintf(&sb, "### %s\n\n", repoID)
			fmt.Fprintf(&sb, "- Test files: %d (ratio: %.2f)\n", health.TestFiles, health.TestRatio)
			fmt.Fprintf(&sb, "- Functions: %d\n", health.FunctionCount)
			if len(health.Issues) > 0 {
				sb.WriteString("- Issues:\n")
				for _, issue := range health.Issues {
					fmt.Fprintf(&sb, "  - %s\n", issue)
				}
			}
			sb.WriteString("\n")
		}
	case []string:
		for _, s := range v {
			fmt.Fprintf(&sb, "- %s\n", s)
		}
		sb.WriteString("\n")
	default:
		// Fall back to JSON
		data, err := json.MarshalIndent(v, "", "  ")
		if err == nil {
			fmt.Fprintf(&sb, "```json\n%s\n```\n\n", string(data))
		} else {
			fmt.Fprintf(&sb, "%v\n\n", v)
		}
	}

	return sb.String()
}

// formatListItem formats a single list item in markdown.
func formatListItem(sb *strings.Builder, item interface{}) {
	switch v := item.(type) {
	case map[string]interface{}:
		name, _ := v["name"].(string)
		function, _ := v["function"].(string)
		if name == "" {
			name = function
		}
		summary, _ := v["summary"].(string)

		if name != "" {
			fmt.Fprintf(sb, "- **%s**", name)
			if summary != "" {
				fmt.Fprintf(sb, " - %s", summary)
			}
			sb.WriteString("\n")
		} else {
			data, _ := json.Marshal(v)
			fmt.Fprintf(sb, "- %s\n", string(data))
		}
	case string:
		fmt.Fprintf(sb, "- %s\n", v)
	default:
		fmt.Fprintf(sb, "- %v\n", v)
	}
}

// formatRecipeResultJSON formats a RecipeResult as JSON.
func formatRecipeResultJSON(result *recipes.RecipeResult) string {
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fmt.Sprintf(`{"error": "failed to serialize result: %v"}`, err)
	}
	return string(data)
}
