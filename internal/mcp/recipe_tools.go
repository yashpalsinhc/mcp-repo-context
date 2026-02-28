package mcp

import (
	"context"
	"fmt"
	"strings"

	"github.com/yashpalc/mcp-repo-context/internal/recipes"
)

// toolAnalyzePRImpact handles the analyze_pr_impact recipe tool.
func (s *server) toolAnalyzePRImpact(ctx context.Context, args map[string]any) callToolResult {
	input := recipes.NewRecipeInput(args)
	return s.executeRecipeAndFormat(ctx, "analyze_pr_impact", input, "markdown")
}

// toolExplainAPIFlow handles the explain_api_flow recipe tool.
func (s *server) toolExplainAPIFlow(ctx context.Context, args map[string]any) callToolResult {
	input := recipes.NewRecipeInput(args)
	return s.executeRecipeAndFormat(ctx, "explain_api_flow", input, "markdown")
}

// toolReviewArchitecture handles the review_architecture recipe tool.
func (s *server) toolReviewArchitecture(ctx context.Context, args map[string]any) callToolResult {
	input := recipes.NewRecipeInput(args)
	return s.executeRecipeAndFormat(ctx, "review_architecture", input, "markdown")
}

// toolExecuteRecipe handles the generic execute_recipe tool.
func (s *server) toolExecuteRecipe(ctx context.Context, args map[string]any) callToolResult {
	recipeName, ok := args["recipe_name"].(string)
	if !ok || recipeName == "" {
		return errorResult("recipe_name is required")
	}

	params, _ := args["params"].(map[string]interface{})
	if params == nil {
		params = map[string]interface{}{}
	}

	format, _ := args["output_format"].(string)
	if format == "" {
		format = "markdown"
	}

	input := recipes.NewRecipeInput(params)
	return s.executeRecipeAndFormat(ctx, recipeName, input, format)
}

// toolListRecipes lists all available recipes.
func (s *server) toolListRecipes(ctx context.Context, args map[string]any) callToolResult {
	if s.recipeRunner == nil {
		return errorResult("Recipe system not initialized")
	}

	recipeList := s.recipeRunner.Registry().List()

	var sb strings.Builder
	sb.WriteString("# Available Recipes\n\n")

	for _, info := range recipeList {
		fmt.Fprintf(&sb, "## %s\n\n", info.Name)
		fmt.Fprintf(&sb, "%s\n\n", info.Description)

		if len(info.InputSchema) > 0 {
			sb.WriteString("**Input Schema:**\n\n")
			sb.WriteString("| Field | Type | Required | Description |\n")
			sb.WriteString("|-------|------|----------|-------------|\n")
			for _, spec := range info.InputSchema {
				required := "no"
				if spec.Required {
					required = "yes"
				}
				fmt.Fprintf(&sb, "| %s | %s | %s | %s |\n",
					spec.Name, spec.Type, required, spec.Description)
			}
			sb.WriteString("\n")
		}
	}

	sb.WriteString("---\n\nUse `execute_recipe` with a recipe name and params to run any recipe.\n")

	return callToolResult{
		Content: []contentItem{{Type: "text", Text: sb.String()}},
	}
}

// executeRecipeAndFormat executes a recipe and formats the output.
func (s *server) executeRecipeAndFormat(ctx context.Context, recipeName string, input recipes.RecipeInput, format string) callToolResult {
	if s.recipeRunner == nil {
		return errorResult("Recipe system not initialized")
	}

	result, err := s.recipeRunner.Execute(ctx, recipeName, input)
	if err != nil {
		return errorResult(fmt.Sprintf("Recipe failed: %v", err))
	}

	var text string
	switch format {
	case "json":
		text = formatRecipeResultJSON(result)
	default:
		text = formatRecipeResultMarkdown(result)
	}

	return callToolResult{
		Content: []contentItem{{Type: "text", Text: text}},
	}
}
