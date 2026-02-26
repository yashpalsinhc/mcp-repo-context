package recipes

// RecipeResult is the output of recipe execution.
type RecipeResult struct {
	Recipe        string              // recipe name
	Data          map[string]any      // structured output (facts from code)
	Analysis      string              // AI-generated summary (optional)
	Gaps          []GapNote           // sections that couldn't be computed
	Sources       []RecipeSourceRef   // file:line references
	ContextTokens int                 // context budget tokens used
	AITokens      int                 // AI API tokens consumed
	DurationMS    int64               // execution time in milliseconds
	Confidence    float64             // 0-1 confidence score
}

// GapNote describes a section that couldn't be computed.
type GapNote struct {
	Section    string // e.g., "cross_service_impact", "dependency_graph"
	Reason     string // why it couldn't be computed
	Suggestion string // suggestion for fixing it
}

// RecipeSourceRef is a file:line reference in the codebase.
// Named to avoid collision with orchestrator.SourceRef and ai.SourceRef.
type RecipeSourceRef struct {
	File  string // file path
	Line  int    // line number
	Claim string // what this reference supports
}

// RecipeRiskAssessment describes risk level with reasoning.
type RecipeRiskAssessment struct {
	Level      string  // "low", "medium", "high"
	Reasoning  string  // explanation of the risk level
	Confidence float64 // 0-1 confidence in the assessment
}

// CalculateConfidence computes the confidence score based on:
// - No AI available: multiply by 0.7
// - Each GapNote: multiply by 0.7
// - Floor at 0.1
func CalculateConfidence(hasAI bool, gapCount int) float64 {
	confidence := 1.0

	// Reduce if AI not available
	if !hasAI {
		confidence *= 0.7
	}

	// Reduce for each gap
	for i := 0; i < gapCount; i++ {
		confidence *= 0.7
	}

	// Floor at 0.1
	if confidence < 0.1 {
		confidence = 0.1
	}

	return confidence
}
