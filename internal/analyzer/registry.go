package analyzer

// Ensure registry implements Registry interface.
var _ Registry = (*registry)(nil)

// registry manages language-specific analyzers (private struct).
type registry struct {
	analyzers map[string]Analyzer
	fallback  Analyzer
}

// NewRegistry creates a new analyzer registry.
func NewRegistry() Registry {
	r := &registry{
		analyzers: make(map[string]Analyzer),
		fallback:  newGenericAnalyzer(),
	}

	// Register Go analyzer
	goAnalyzer := newGoAnalyzer()
	for _, lang := range goAnalyzer.Languages() {
		r.analyzers[lang] = goAnalyzer
	}

	return r
}

// Get returns the appropriate analyzer for a language.
func (r *registry) Get(lang string) Analyzer {
	if a, ok := r.analyzers[lang]; ok {
		return a
	}
	return r.fallback
}
