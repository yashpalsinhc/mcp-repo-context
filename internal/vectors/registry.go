package vectors

import (
	"fmt"
	"sort"
)

// EmbedderRegistry manages named embedders with dimension validation.
type EmbedderRegistry interface {
	// Get returns the embedder with the given name, or nil if not found.
	Get(name string) Embedder
	// Default returns the default embedder.
	Default() Embedder
	// Names returns sorted list of all registered embedder names.
	Names() []string
}

// Ensure embedderRegistry implements EmbedderRegistry.
var _ EmbedderRegistry = (*embedderRegistry)(nil)

// embedderRegistry is a private implementation of EmbedderRegistry.
type embedderRegistry struct {
	embedders  map[string]Embedder
	defaultEmb Embedder
	dimension  int
}

// NewEmbedderRegistry creates a new embedder registry.
// The first argument is the default embedder (required, non-nil).
// Extra embedders are registered by their Name().
// All embedders must have the same Dimension(); a mismatch returns an error.
// Duplicate names: last wins.
func NewEmbedderRegistry(defaultEmbedder Embedder, extras ...Embedder) (EmbedderRegistry, error) {
	if defaultEmbedder == nil {
		return nil, fmt.Errorf("default embedder required")
	}

	reg := &embedderRegistry{
		embedders:  make(map[string]Embedder),
		defaultEmb: defaultEmbedder,
		dimension:  defaultEmbedder.Dimension(),
	}

	// Register default by name
	reg.embedders[defaultEmbedder.Name()] = defaultEmbedder

	// Register extras
	for _, emb := range extras {
		if emb == nil {
			continue
		}
		if emb.Dimension() != reg.dimension {
			return nil, fmt.Errorf("dimension mismatch: embedder '%s' has dimension %d, expected %d",
				emb.Name(), emb.Dimension(), reg.dimension)
		}
		reg.embedders[emb.Name()] = emb
	}

	return reg, nil
}

// DefaultEmbedderRegistry returns a registry with the default local embedder.
func DefaultEmbedderRegistry() EmbedderRegistry {
	reg, err := NewEmbedderRegistry(NewDefaultEmbedder())
	if err != nil {
		panic(fmt.Sprintf("failed to create default embedder registry: %v", err))
	}
	return reg
}

// Get returns the embedder with the given name, or nil if not found.
func (r *embedderRegistry) Get(name string) Embedder {
	return r.embedders[name]
}

// Default returns the default embedder.
func (r *embedderRegistry) Default() Embedder {
	return r.defaultEmb
}

// Names returns a sorted list of all registered embedder names.
func (r *embedderRegistry) Names() []string {
	names := make([]string, 0, len(r.embedders))
	for name := range r.embedders {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
