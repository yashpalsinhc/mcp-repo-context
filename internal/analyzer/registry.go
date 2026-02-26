package analyzer

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
