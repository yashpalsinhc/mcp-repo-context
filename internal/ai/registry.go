package ai

import (
	"fmt"
	"os"
)

// Registry manages AI providers.
type Registry struct {
	providers map[string]Provider
	default_  string
}

// NewRegistry creates a new AI provider registry.
func NewRegistry() *Registry {
	return &Registry{
		providers: make(map[string]Provider),
	}
}

// Register adds a provider to the registry.
func (r *Registry) Register(name string, provider Provider) {
	r.providers[name] = provider
}

// Get returns a provider by name.
func (r *Registry) Get(name string) (Provider, bool) {
	p, ok := r.providers[name]
	return p, ok
}

// GetDefault returns the default configured provider.
func (r *Registry) GetDefault() (Provider, error) {
	if r.default_ != "" {
		if p, ok := r.providers[r.default_]; ok && p.IsConfigured() {
			return p, nil
		}
	}

	// Try to find any configured provider
	for _, p := range r.providers {
		if p.IsConfigured() {
			return p, nil
		}
	}

	return nil, ErrProviderNotConfigured
}

// SetDefault sets the default provider.
func (r *Registry) SetDefault(name string) error {
	if _, ok := r.providers[name]; !ok {
		return fmt.Errorf("provider %s not found", name)
	}
	r.default_ = name
	return nil
}

// HasConfiguredProvider returns true if at least one provider is configured.
func (r *Registry) HasConfiguredProvider() bool {
	for _, p := range r.providers {
		if p.IsConfigured() {
			return true
		}
	}
	return false
}

// ListProviders returns all registered provider names.
func (r *Registry) ListProviders() []string {
	names := make([]string, 0, len(r.providers))
	for name := range r.providers {
		names = append(names, name)
	}
	return names
}

// NewRegistryFromEnv creates a registry with providers configured from environment variables.
func NewRegistryFromEnv() *Registry {
	r := NewRegistry()

	// Anthropic
	if apiKey := os.Getenv("ANTHROPIC_API_KEY"); apiKey != "" {
		r.Register("anthropic", NewAnthropicProvider(Config{
			APIKey: apiKey,
			Model:  getEnvOrDefault("ANTHROPIC_MODEL", defaultModel),
		}))
		r.default_ = "anthropic"
	}

	// OpenAI (placeholder for future implementation)
	if apiKey := os.Getenv("OPENAI_API_KEY"); apiKey != "" {
		// OpenAI provider would be registered here
		// r.Register("openai", NewOpenAIProvider(Config{APIKey: apiKey}))
	}

	return r
}

func getEnvOrDefault(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}
