package recipes

import (
	"context"
)

// Recipe defines the interface for a recipe.
type Recipe interface {
	// Name returns the recipe name (lowercase, underscore-separated).
	Name() string

	// Description returns a short description of what the recipe does.
	Description() string

	// InputSchema returns the input field specifications.
	InputSchema() map[string]FieldSpec

	// Run executes the recipe with the given input.
	Run(ctx context.Context, runner *RecipeRunner, input RecipeInput) (*RecipeResult, error)
}

// FieldSpec defines the specification for an input field.
type FieldSpec struct {
	Name        string // field name
	Type        string // "string", "int", "bool", "[]string", "[]ChangedFile"
	Required    bool   // whether the field is required
	Description string // human-readable description
	Default     any    // default value if not provided
}

// RecipeInfo contains metadata about a registered recipe.
type RecipeInfo struct {
	Name        string
	Description string
	InputSchema map[string]FieldSpec
}
