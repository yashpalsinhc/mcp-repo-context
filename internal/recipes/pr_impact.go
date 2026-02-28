package recipes

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

// PRImpactRecipe implements the analyze_pr_impact recipe.
type PRImpactRecipe struct{}

// Ensure PRImpactRecipe implements Recipe.
var _ Recipe = (*PRImpactRecipe)(nil)

func (r *PRImpactRecipe) Name() string { return "analyze_pr_impact" }

func (r *PRImpactRecipe) Description() string {
	return "Comprehensive PR impact analysis with cross-repo awareness and AI risk assessment"
}

func (r *PRImpactRecipe) InputSchema() map[string]FieldSpec {
	return map[string]FieldSpec{
		"repo_id": {
			Name:     "repo_id",
			Type:     "string",
			Required: true,
			Description: "Repository ID",
		},
		"changed_files": {
			Name:     "changed_files",
			Type:     "[]ChangedFile",
			Required: true,
			Description: "Files changed in the PR",
		},
		"org_id": {
			Name:        "org_id",
			Type:        "string",
			Required:    false,
			Description: "Org ID for cross-repo analysis",
		},
		"token_budget": {
			Name:        "token_budget",
			Type:        "int",
			Required:    false,
			Default:     8000,
			Description: "Max context tokens",
		},
		"include_ai": {
			Name:        "include_ai",
			Type:        "bool",
			Required:    false,
			Default:     true,
			Description: "Include AI risk assessment",
		},
	}
}

func (r *PRImpactRecipe) Run(ctx context.Context, runner *RecipeRunner, input RecipeInput) (*RecipeResult, error) {
	start := time.Now()

	repoID := input.GetString("repo_id")
	orgID := input.GetString("org_id")
	includeAI := input.GetBool("include_ai", true)

	// Parse changed_files from input
	changedFiles := parseChangedFiles(input)

	result := &RecipeResult{
		Recipe:  "analyze_pr_impact",
		Data:    make(map[string]any),
		Sources: []RecipeSourceRef{},
		Gaps:    []GapNote{},
	}

	// Step 1: Single-Repo Impact (always runs)
	prCtx, err := runner.Manager().GetPRContext(repoID, changedFiles)
	if err != nil {
		return nil, fmt.Errorf("failed to get PR context: %w", err)
	}

	// Extract data from PR context
	changedFunctions, affectedCallers, affectedRoutes, sources := extractPRData(prCtx)
	result.Data["changed_functions"] = changedFunctions
	result.Data["affected_callers"] = affectedCallers
	result.Data["affected_routes"] = affectedRoutes
	result.Sources = append(result.Sources, sources...)

	// Check context cancellation
	if ctx.Err() != nil {
		result.Gaps = append(result.Gaps,
			GapNote{Section: "cross_service_impact", Reason: "Context cancelled", Suggestion: "Re-run the recipe"},
			GapNote{Section: "dependency_impact", Reason: "Context cancelled", Suggestion: "Re-run the recipe"},
			GapNote{Section: "risk_assessment", Reason: "Context cancelled", Suggestion: "Re-run the recipe"},
		)
		result.Confidence = CalculateConfidence(false, len(result.Gaps))
		result.DurationMS = time.Since(start).Milliseconds()
		return result, nil
	}

	// Steps 2 & 3: Cross-service and dependency impact (concurrent)
	var crossServiceImpact []interface{}
	var dependencyImpact []interface{}
	var step2Gaps, step3Gaps []GapNote

	var wg sync.WaitGroup

	if orgID != "" {
		wg.Add(2)
		go func() {
			defer wg.Done()
			crossServiceImpact, step2Gaps = r.crossServiceImpact(ctx, orgID)
		}()
		go func() {
			defer wg.Done()
			dependencyImpact, step3Gaps = r.dependencyImpact(ctx, orgID)
		}()
		wg.Wait()
	} else {
		crossServiceImpact = []interface{}{}
		dependencyImpact = []interface{}{}
	}

	result.Data["cross_service_impact"] = crossServiceImpact
	result.Data["dependency_impact"] = dependencyImpact
	result.Gaps = append(result.Gaps, step2Gaps...)
	result.Gaps = append(result.Gaps, step3Gaps...)

	// Check context cancellation again
	if ctx.Err() != nil {
		result.Gaps = append(result.Gaps,
			GapNote{Section: "risk_assessment", Reason: "Context cancelled", Suggestion: "Re-run the recipe"},
		)
		result.Confidence = CalculateConfidence(false, len(result.Gaps))
		result.DurationMS = time.Since(start).Milliseconds()
		return result, nil
	}

	// Step 4: Risk Assessment
	risk := r.assessRisk(ctx, runner, includeAI, changedFunctions, affectedCallers, prCtx)
	result.Data["risk"] = risk

	// Step 5: Suggested Reviewers
	result.Data["suggested_reviewers"] = []interface{}{}
	result.Gaps = append(result.Gaps, GapNote{
		Section:    "suggested_reviewers",
		Reason:     "Reviewer suggestions require git blame data in function metadata",
		Suggestion: "Add git blame extraction during analysis",
	})

	// AI analysis narrative
	if includeAI && runner.AI() != nil {
		analysis := r.generateAnalysis(ctx, runner, changedFunctions, affectedCallers, affectedRoutes, risk)
		result.Analysis = analysis
	}

	hasAI := runner.AI() != nil && includeAI
	result.Confidence = CalculateConfidence(hasAI, len(result.Gaps))
	result.DurationMS = time.Since(start).Milliseconds()

	return result, nil
}

// crossServiceImpact checks for cross-service impact (V1: always gap).
func (r *PRImpactRecipe) crossServiceImpact(ctx context.Context, orgID string) ([]interface{}, []GapNote) {
	return []interface{}{}, []GapNote{
		{
			Section:    "cross_service_impact",
			Reason:     "Cross-service impact analysis requires 03-api-flow-tracing",
			Suggestion: "Implement 03-api-flow-tracing split for cross-repo endpoint matching",
		},
	}
}

// dependencyImpact checks for dependency impact (V1: always gap).
func (r *PRImpactRecipe) dependencyImpact(ctx context.Context, orgID string) ([]interface{}, []GapNote) {
	return []interface{}{}, []GapNote{
		{
			Section:    "dependency_impact",
			Reason:     "Dependency impact analysis requires 02-dependency-graph",
			Suggestion: "Implement 02-dependency-graph split for cross-repo dependency tracking",
		},
	}
}

// assessRisk computes risk level using AI or heuristics.
func (r *PRImpactRecipe) assessRisk(ctx context.Context, runner *RecipeRunner, includeAI bool,
	changedFunctions, affectedCallers []interface{}, prCtx interface{}) RecipeRiskAssessment {

	if includeAI && runner.AI() != nil {
		return r.aiRiskAssessment(ctx, runner, changedFunctions, affectedCallers, prCtx)
	}
	return r.heuristicRiskAssessment(changedFunctions, affectedCallers, prCtx)
}

// aiRiskAssessment uses AI to assess risk.
func (r *PRImpactRecipe) aiRiskAssessment(ctx context.Context, runner *RecipeRunner,
	changedFunctions, affectedCallers []interface{}, prCtx interface{}) RecipeRiskAssessment {

	prompt := fmt.Sprintf(
		"Assess the risk level of a PR change. Changed functions: %d. Affected callers: %d.\n"+
			"Respond with exactly one line: 'Risk: <low|medium|high>'\n"+
			"Then on a new line: 'Reasoning: <your reasoning>'\n",
		len(changedFunctions), len(affectedCallers),
	)

	// Truncate prompt to 4000 chars
	if len(prompt) > 4000 {
		prompt = prompt[:4000]
	}

	response, _, err := runner.AI().CompleteRaw(ctx, prompt, 500)
	if err != nil {
		// Fallback to heuristic
		return r.heuristicRiskAssessment(changedFunctions, affectedCallers, prCtx)
	}

	level, reasoning := parseRiskResponse(response)
	return RecipeRiskAssessment{
		Level:      level,
		Reasoning:  reasoning,
		Confidence: 0.8,
	}
}

// heuristicRiskAssessment uses a simple scoring system.
func (r *PRImpactRecipe) heuristicRiskAssessment(changedFunctions, affectedCallers []interface{}, prCtx interface{}) RecipeRiskAssessment {
	score := len(affectedCallers)*2 + countExternalCalls(prCtx)*5 + countDBChanges(prCtx)*3

	level := "low"
	if score > 15 {
		level = "high"
	} else if score >= 5 {
		level = "medium"
	}

	reasoning := fmt.Sprintf("Heuristic: %d affected callers, %d external API calls, %d DB changes",
		len(affectedCallers), countExternalCalls(prCtx), countDBChanges(prCtx))

	return RecipeRiskAssessment{
		Level:      level,
		Reasoning:  reasoning,
		Confidence: 0.5,
	}
}

// generateAnalysis creates an AI narrative for the PR.
func (r *PRImpactRecipe) generateAnalysis(ctx context.Context, runner *RecipeRunner,
	changedFunctions, affectedCallers, affectedRoutes []interface{}, risk RecipeRiskAssessment) string {

	prompt := fmt.Sprintf(
		"Summarize this PR impact in 2-3 sentences:\n"+
			"- %d functions changed\n"+
			"- %d callers affected\n"+
			"- %d routes affected\n"+
			"- Risk level: %s (%s)\n",
		len(changedFunctions), len(affectedCallers), len(affectedRoutes),
		risk.Level, risk.Reasoning,
	)

	response, _, err := runner.AI().CompleteRaw(ctx, prompt, 500)
	if err != nil {
		return ""
	}
	return response
}

// Helper functions

// parseChangedFiles extracts changed files from input.
func parseChangedFiles(input RecipeInput) []interface{} {
	raw, ok := input.Get("changed_files")
	if !ok {
		return []interface{}{}
	}

	switch v := raw.(type) {
	case []interface{}:
		return v
	case []map[string]interface{}:
		result := make([]interface{}, len(v))
		for i, m := range v {
			result[i] = m
		}
		return result
	default:
		return []interface{}{}
	}
}

// extractPRData pulls structured data from the PR context response.
func extractPRData(prCtx interface{}) (changedFunctions, affectedCallers, affectedRoutes []interface{}, sources []RecipeSourceRef) {
	changedFunctions = []interface{}{}
	affectedCallers = []interface{}{}
	affectedRoutes = []interface{}{}

	data, ok := prCtx.(map[string]interface{})
	if !ok {
		return
	}

	// Extract changed files with functions
	if files, ok := data["changed_files"].([]interface{}); ok {
		for _, f := range files {
			fileMap, ok := f.(map[string]interface{})
			if !ok {
				continue
			}
			filePath, _ := fileMap["file_path"].(string)

			// Extract functions from the file
			for _, key := range []string{"added_functions", "modified_functions"} {
				if funcs, ok := fileMap[key].([]interface{}); ok {
					for _, fn := range funcs {
						changedFunctions = append(changedFunctions, fn)
						// Add source ref
						if fnMap, ok := fn.(map[string]interface{}); ok {
							name, _ := fnMap["name"].(string)
							line := 0
							if l, ok := fnMap["line_start"].(int); ok {
								line = l
							} else if l, ok := fnMap["line_start"].(float64); ok {
								line = int(l)
							}
							sources = append(sources, RecipeSourceRef{
								File:  filePath,
								Line:  line,
								Claim: name + " behavior",
							})
						}
					}
				}
			}
		}
	}

	// Extract affected callers from impact analysis
	if impact, ok := data["impact_analysis"].(map[string]interface{}); ok {
		if funcs, ok := impact["affected_functions"].([]interface{}); ok {
			affectedCallers = funcs
		}
		if routes, ok := impact["affected_routes"].([]interface{}); ok {
			affectedRoutes = routes
		}
	}

	return
}

// parseRiskResponse parses AI response for risk level and reasoning.
func parseRiskResponse(response string) (string, string) {
	level := "medium" // default
	reasoning := response

	lines := strings.Split(response, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		lower := strings.ToLower(line)
		if strings.HasPrefix(lower, "risk:") {
			riskStr := strings.TrimSpace(strings.TrimPrefix(lower, "risk:"))
			switch {
			case strings.Contains(riskStr, "low"):
				level = "low"
			case strings.Contains(riskStr, "high"):
				level = "high"
			default:
				level = "medium"
			}
		}
		if strings.HasPrefix(lower, "reasoning:") {
			reasoning = strings.TrimSpace(line[len("reasoning:"):])
		}
	}

	return level, reasoning
}

// countExternalCalls counts HTTP calls in PR context.
func countExternalCalls(prCtx interface{}) int {
	data, ok := prCtx.(map[string]interface{})
	if !ok {
		return 0
	}
	count := 0
	if files, ok := data["changed_files"].([]interface{}); ok {
		for _, f := range files {
			if fileMap, ok := f.(map[string]interface{}); ok {
				for _, key := range []string{"added_functions", "modified_functions"} {
					if funcs, ok := fileMap[key].([]interface{}); ok {
						for _, fn := range funcs {
							if fnMap, ok := fn.(map[string]interface{}); ok {
								if calls, ok := fnMap["http_calls"].([]interface{}); ok {
									count += len(calls)
								}
							}
						}
					}
				}
			}
		}
	}
	return count
}

// countDBChanges counts DB queries in PR context.
func countDBChanges(prCtx interface{}) int {
	data, ok := prCtx.(map[string]interface{})
	if !ok {
		return 0
	}
	count := 0
	if files, ok := data["changed_files"].([]interface{}); ok {
		for _, f := range files {
			if fileMap, ok := f.(map[string]interface{}); ok {
				for _, key := range []string{"added_functions", "modified_functions"} {
					if funcs, ok := fileMap[key].([]interface{}); ok {
						for _, fn := range funcs {
							if fnMap, ok := fn.(map[string]interface{}); ok {
								if queries, ok := fnMap["db_queries"].([]interface{}); ok {
									count += len(queries)
								}
							}
						}
					}
				}
			}
		}
	}
	return count
}
