package analyzer

import "sort"

// Ensure registry implements Registry interface.
var _ Registry = (*registry)(nil)

// registry manages language-specific analyzers (private struct).
type registry struct {
	analyzers map[string]Analyzer
	fallback  Analyzer
}

// NewRegistry creates a new analyzer registry with provided analyzers.
// Analyzers are registered by their Languages(). Duplicate languages: last wins.
// Returns a registry with generic analyzer as fallback.
func NewRegistry(analyzers ...Analyzer) Registry {
	r := &registry{
		analyzers: make(map[string]Analyzer),
		fallback:  NewGenericAnalyzer(),
	}

	// Register provided analyzers
	for _, a := range analyzers {
		if a == nil {
			continue
		}
		for _, lang := range a.Languages() {
			r.analyzers[lang] = a
		}
	}

	return r
}

// DefaultRegistry returns a registry with built-in analyzers (Go analyzer).
func DefaultRegistry() Registry {
	return NewRegistry(NewGoAnalyzer())
}

// Get returns the appropriate analyzer for a language.
func (r *registry) Get(lang string) Analyzer {
	if a, ok := r.analyzers[lang]; ok {
		return a
	}
	return r.fallback
}

// GetByName returns the analyzer with the given name, or nil if not found.
func (r *registry) GetByName(name string) Analyzer {
	// Check the fallback
	if r.fallback != nil && r.fallback.Name() == name {
		return r.fallback
	}
	// Check registered analyzers (deduplicated by name)
	for _, a := range r.analyzers {
		if a.Name() == name {
			return a
		}
	}
	return nil
}

// Names returns a list of all registered analyzer names (deduplicated, sorted).
func (r *registry) Names() []string {
	seen := make(map[string]bool)
	for _, a := range r.analyzers {
		seen[a.Name()] = true
	}
	if r.fallback != nil {
		seen[r.fallback.Name()] = true
	}
	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
