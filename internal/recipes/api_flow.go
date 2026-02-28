package recipes

import (
	"context"
	"fmt"
	"time"
)

// FlowStep represents a step in the execution flow.
type FlowStep struct {
	Service     string   `json:"service"`
	Function    string   `json:"function"`
	File        string   `json:"file"`
	Line        int      `json:"line"`
	Action      string   `json:"action"`
	SideEffects []string `json:"side_effects,omitempty"`
}

// APIFlowRecipe implements the explain_api_flow recipe.
type APIFlowRecipe struct{}

var _ Recipe = (*APIFlowRecipe)(nil)

func (r *APIFlowRecipe) Name() string { return "explain_api_flow" }

func (r *APIFlowRecipe) Description() string {
	return "Trace and explain the execution flow of a function with Mermaid visualization"
}

func (r *APIFlowRecipe) InputSchema() map[string]FieldSpec {
	return map[string]FieldSpec{
		"function_name": {
			Name:        "function_name",
			Type:        "string",
			Required:    true,
			Description: "Entry function name",
		},
		"repo_id": {
			Name:        "repo_id",
			Type:        "string",
			Required:    true,
			Description: "Repository ID",
		},
		"endpoint": {
			Name:        "endpoint",
			Type:        "string",
			Required:    false,
			Description: "HTTP endpoint (future: resolved to handler)",
		},
		"org_id": {
			Name:        "org_id",
			Type:        "string",
			Required:    false,
			Description: "Org ID for cross-service tracing",
		},
		"token_budget": {
			Name:        "token_budget",
			Type:        "int",
			Required:    false,
			Default:     8000,
			Description: "Max context tokens",
		},
	}
}

func (r *APIFlowRecipe) Run(ctx context.Context, runner *RecipeRunner, input RecipeInput) (*RecipeResult, error) {
	start := time.Now()

	funcName := input.GetString("function_name")
	repoID := input.GetString("repo_id")
	endpoint := input.GetString("endpoint")
	orgID := input.GetString("org_id")

	result := &RecipeResult{
		Recipe:  "explain_api_flow",
		Data:    make(map[string]any),
		Sources: []RecipeSourceRef{},
		Gaps:    []GapNote{},
	}

	// Step 1: Find Entry Point
	if funcName == "" && endpoint != "" {
		// Endpoint-only without function name
		result.Gaps = append(result.Gaps, GapNote{
			Section:    "endpoint_lookup",
			Reason:     "Endpoint-to-handler lookup requires 03-api-flow-tracing",
			Suggestion: "Use function_name input instead",
		})
		return nil, fmt.Errorf("endpoint-to-handler lookup not available; provide function_name instead")
	}

	if funcName == "" {
		return nil, fmt.Errorf("function_name is required")
	}

	// Get the entry function context
	entryCtx, err := runner.Manager().GetFunctionContext(repoID, "", funcName)
	if err != nil || entryCtx == nil {
		return nil, fmt.Errorf("function not found: %s", funcName)
	}

	// Extract entry point info
	entryPoint := extractEntryPoint(funcName, entryCtx)
	result.Data["entry_point"] = entryPoint
	result.Sources = append(result.Sources, RecipeSourceRef{
		File:  entryPoint["file"].(string),
		Line:  entryPoint["line"].(int),
		Claim: funcName + " entry point",
	})

	// If endpoint provided alongside function_name, note the gap
	if endpoint != "" {
		result.Gaps = append(result.Gaps, GapNote{
			Section:    "endpoint_lookup",
			Reason:     "Endpoint-to-handler lookup requires 03-api-flow-tracing. Using function_name input.",
			Suggestion: "Implement 03-api-flow-tracing for endpoint resolution",
		})
	}

	// Step 2: Trace Internal Flow
	visited := make(map[string]bool)
	flowSteps := r.traceFlow(ctx, runner, repoID, funcName, entryCtx, visited, 0, 10)
	result.Data["flow_steps"] = flowSteps

	// Add sources for flow steps
	for _, step := range flowSteps {
		result.Sources = append(result.Sources, RecipeSourceRef{
			File:  step.File,
			Line:  step.Line,
			Claim: step.Function + " flow step",
		})
	}

	// Step 3: Cross-Service Hops
	if orgID != "" {
		result.Data["cross_service_hops"] = []interface{}{}
		result.Gaps = append(result.Gaps, GapNote{
			Section:    "cross_service_hops",
			Reason:     "Cross-service tracing requires 03-api-flow-tracing",
			Suggestion: "Implement 03-api-flow-tracing for cross-repo endpoint matching",
		})
	} else {
		result.Data["cross_service_hops"] = []interface{}{}
	}

	// Step 4: Build Mermaid Diagram
	mermaid := buildMermaidDiagram(funcName, flowSteps)
	result.Data["mermaid"] = mermaid

	// Step 5: Extract data transformations from side effects
	transformations := extractDataTransformations(flowSteps)
	result.Data["data_transformations"] = transformations

	// Step 6: AI Explanation
	if runner.AI() != nil {
		analysis := r.generateFlowAnalysis(ctx, runner, funcName, flowSteps, mermaid)
		result.Analysis = analysis
	}

	hasAI := runner.AI() != nil
	result.Confidence = CalculateConfidence(hasAI, len(result.Gaps))
	result.DurationMS = time.Since(start).Milliseconds()

	return result, nil
}

// traceFlow performs depth-first traversal of the call graph.
func (r *APIFlowRecipe) traceFlow(ctx context.Context, runner *RecipeRunner, repoID, funcName string,
	funcCtx interface{}, visited map[string]bool, depth, maxDepth int) []FlowStep {

	if depth >= maxDepth || visited[funcName] {
		return nil
	}
	visited[funcName] = true

	// Create step for current function
	step := FlowStep{
		Service:  "same",
		Function: funcName,
	}

	// Extract info from function context
	if data, ok := funcCtx.(map[string]interface{}); ok {
		if fp, ok := data["file_path"].(string); ok {
			step.File = fp
		} else if fp, ok := data["file"].(string); ok {
			step.File = fp
		}
		if l, ok := data["line"].(int); ok {
			step.Line = l
		} else if l, ok := data["line_start"].(int); ok {
			step.Line = l
		}
		if s, ok := data["summary"].(string); ok {
			step.Action = s
		}
		if effects, ok := data["side_effects"].([]interface{}); ok {
			for _, e := range effects {
				if es, ok := e.(string); ok {
					step.SideEffects = append(step.SideEffects, es)
				}
			}
		}
		if effects, ok := data["side_effects"].([]string); ok {
			step.SideEffects = append(step.SideEffects, effects...)
		}
	}

	steps := []FlowStep{step}

	// Get callees and recurse
	callees := extractCallees(funcCtx)
	for _, callee := range callees {
		if visited[callee] {
			continue
		}
		calleeCtx, err := runner.Manager().GetFunctionContext(repoID, "", callee)
		if err != nil || calleeCtx == nil {
			continue
		}
		childSteps := r.traceFlow(ctx, runner, repoID, callee, calleeCtx, visited, depth+1, maxDepth)
		steps = append(steps, childSteps...)
	}

	return steps
}

// extractEntryPoint pulls entry point info from function context.
func extractEntryPoint(funcName string, funcCtx interface{}) map[string]interface{} {
	entry := map[string]interface{}{
		"function": funcName,
		"file":     "",
		"line":     0,
	}

	if data, ok := funcCtx.(map[string]interface{}); ok {
		if fp, ok := data["file_path"].(string); ok {
			entry["file"] = fp
		} else if fp, ok := data["file"].(string); ok {
			entry["file"] = fp
		}
		if l, ok := data["line"].(int); ok {
			entry["line"] = l
		} else if l, ok := data["line_start"].(int); ok {
			entry["line"] = l
		}
		if s, ok := data["summary"].(string); ok {
			entry["summary"] = s
		}
	}

	return entry
}

// extractCallees gets callee function names from function context.
func extractCallees(funcCtx interface{}) []string {
	data, ok := funcCtx.(map[string]interface{})
	if !ok {
		return nil
	}

	var callees []string

	if calls, ok := data["callees"].([]interface{}); ok {
		for _, c := range calls {
			if cm, ok := c.(map[string]interface{}); ok {
				if name, ok := cm["function"].(string); ok {
					callees = append(callees, name)
				}
			} else if name, ok := c.(string); ok {
				callees = append(callees, name)
			}
		}
	}

	if calls, ok := data["calls"].([]interface{}); ok {
		for _, c := range calls {
			if cm, ok := c.(map[string]interface{}); ok {
				if name, ok := cm["function"].(string); ok {
					callees = append(callees, name)
				}
			}
		}
	}

	return callees
}

// extractDataTransformations identifies data flow patterns from side effects.
func extractDataTransformations(steps []FlowStep) []string {
	var transformations []string
	for _, step := range steps {
		for _, effect := range step.SideEffects {
			transformations = append(transformations, fmt.Sprintf("%s: %s", step.Function, effect))
		}
	}
	return transformations
}

// generateFlowAnalysis creates an AI narrative for the flow.
func (r *APIFlowRecipe) generateFlowAnalysis(ctx context.Context, runner *RecipeRunner,
	funcName string, steps []FlowStep, mermaid string) string {

	prompt := fmt.Sprintf(
		"Explain this execution flow starting from %s in 2-3 sentences. "+
			"The flow has %d steps.\nMermaid diagram:\n%s\n"+
			"Start with: 'When a request hits %s...'",
		funcName, len(steps), mermaid, funcName,
	)

	if len(prompt) > 4000 {
		prompt = prompt[:4000]
	}

	response, _, err := runner.AI().CompleteRaw(ctx, prompt, 1000)
	if err != nil {
		return ""
	}
	return response
}
