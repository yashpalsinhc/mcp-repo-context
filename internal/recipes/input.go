package recipes

import (
	"fmt"
	"strings"
)

// RecipeInput is a typed wrapper around map[string]any for recipe inputs.
type RecipeInput struct {
	data map[string]any
}

// NewRecipeInput creates a new RecipeInput from a map.
func NewRecipeInput(data map[string]any) RecipeInput {
	if data == nil {
		data = make(map[string]any)
	}
	return RecipeInput{data: data}
}

// Get retrieves a raw value by key.
func (r RecipeInput) Get(key string) (any, bool) {
	val, ok := r.data[key]
	return val, ok
}

// GetString retrieves a string value by key, returning "" if missing or wrong type.
func (r RecipeInput) GetString(key string) string {
	val, ok := r.data[key]
	if !ok {
		return ""
	}
	if str, ok := val.(string); ok {
		return str
	}
	return ""
}

// GetStringSlice retrieves a string slice, coercing []interface{} -> []string.
func (r RecipeInput) GetStringSlice(key string) []string {
	val, ok := r.data[key]
	if !ok {
		return []string{}
	}

	// Handle []string directly
	if strs, ok := val.([]string); ok {
		return strs
	}

	// Handle []interface{}
	if ifaces, ok := val.([]interface{}); ok {
		result := make([]string, 0, len(ifaces))
		for _, v := range ifaces {
			if str, ok := v.(string); ok {
				result = append(result, str)
			}
		}
		return result
	}

	// Handle single string
	if str, ok := val.(string); ok {
		return []string{str}
	}

	return []string{}
}

// GetInt retrieves an int value, returning the default if missing.
// Handles float64 (from JSON).
func (r RecipeInput) GetInt(key string, defaultVal int) int {
	val, ok := r.data[key]
	if !ok {
		return defaultVal
	}

	// Handle int directly
	if i, ok := val.(int); ok {
		return i
	}

	// Handle float64 (from JSON)
	if f, ok := val.(float64); ok {
		return int(f)
	}

	return defaultVal
}

// GetBool retrieves a bool value, returning the default if missing.
func (r RecipeInput) GetBool(key string, defaultVal bool) bool {
	val, ok := r.data[key]
	if !ok {
		return defaultVal
	}

	if b, ok := val.(bool); ok {
		return b
	}

	return defaultVal
}

// Validate checks that all required fields are present.
// Returns error listing missing required fields.
func (r RecipeInput) Validate(schema map[string]FieldSpec) error {
	var missing []string
	for _, spec := range schema {
		if spec.Required {
			if _, ok := r.data[spec.Name]; !ok {
				missing = append(missing, spec.Name)
			}
		}
	}

	if len(missing) > 0 {
		return fmt.Errorf("missing required fields: %s", strings.Join(missing, ", "))
	}

	return nil
}
