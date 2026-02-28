package recipes

import (
	"sync"
)

// Registry manages registered recipes.
type Registry struct {
	mu      sync.RWMutex
	recipes map[string]Recipe
}

// NewRegistry creates an empty recipe registry.
func NewRegistry() *Registry {
	return &Registry{
		recipes: make(map[string]Recipe),
	}
}

// Register adds a recipe to the registry.
// Last-write-wins on name collision.
func (r *Registry) Register(recipe Recipe) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.recipes[recipe.Name()] = recipe
}

// Get retrieves a recipe by name.
func (r *Registry) Get(name string) (Recipe, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	recipe, ok := r.recipes[name]
	return recipe, ok
}

// List returns metadata for all registered recipes.
func (r *Registry) List() []RecipeInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	info := make([]RecipeInfo, 0, len(r.recipes))
	for _, recipe := range r.recipes {
		info = append(info, RecipeInfo{
			Name:        recipe.Name(),
			Description: recipe.Description(),
			InputSchema: recipe.InputSchema(),
		})
	}
	return info
}

// DefaultRegistry returns a registry with all built-in recipes.
func DefaultRegistry() *Registry {
	registry := NewRegistry()
	registry.Register(&PRImpactRecipe{})
	registry.Register(&APIFlowRecipe{})
	registry.Register(&ArchitectureRecipe{})
	return registry
}
