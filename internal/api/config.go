package api

import "time"

// APIConfig holds configuration for the HTTP API server.
type APIConfig struct {
	ListenAddr         string
	ReadTimeout        time.Duration
	WriteTimeout       time.Duration
	IdleTimeout        time.Duration
	RateLimitPerMinute int
	MaxRequestBodySize int64
	GithubWebhookSecret string
	GitlabWebhookSecret string
}

// DefaultAPIConfig returns a config with sensible defaults.
func DefaultAPIConfig() *APIConfig {
	return &APIConfig{
		ListenAddr:         ":8080",
		ReadTimeout:        15 * time.Second,
		WriteTimeout:       15 * time.Second,
		IdleTimeout:        60 * time.Second,
		RateLimitPerMinute: 60,
		MaxRequestBodySize: 10 * 1024 * 1024, // 10MB
	}
}
