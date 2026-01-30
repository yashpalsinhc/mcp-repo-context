package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// Config holds all configuration for the MCP repo-context server.
type Config struct {
	// Storage configuration
	Storage StorageConfig `json:"storage"`

	// AI provider configuration
	AI AIConfig `json:"ai"`

	// Analysis configuration
	Analysis AnalysisConfig `json:"analysis"`

	// Server configuration
	Server ServerConfig `json:"server"`

	// Logging configuration
	Logging LoggingConfig `json:"logging"`
}

// StorageConfig configures how context is stored.
type StorageConfig struct {
	// Type is the storage backend type: "filesystem" or "sqlite" (future)
	Type string `json:"type"`

	// Path is the directory or file path for storage
	Path string `json:"path"`

	// MaxContextAge is how long to cache context before re-analysis
	MaxContextAge time.Duration `json:"max_context_age"`

	// EnableMetadataIndex enables fast metadata lookups without reading full context
	EnableMetadataIndex bool `json:"enable_metadata_index"`

	// MetadataIndexPath is the path to the metadata index file
	MetadataIndexPath string `json:"metadata_index_path"`
}

// AIConfig configures AI provider settings.
type AIConfig struct {
	// Provider is the AI provider to use: "anthropic", "openai", etc.
	Provider string `json:"provider"`

	// Model is the specific model to use
	Model string `json:"model"`

	// MaxTokens is the maximum tokens for AI responses
	MaxTokens int `json:"max_tokens"`

	// Temperature controls response randomness (0.0-1.0)
	Temperature float64 `json:"temperature"`

	// APIKey is the API key (can be overridden by env var)
	APIKey string `json:"api_key,omitempty"`

	// Retry configuration for API calls
	Retry RetryConfig `json:"retry"`

	// Timeout for AI API calls
	Timeout time.Duration `json:"timeout"`
}

// RetryConfig configures retry behavior.
type RetryConfig struct {
	// MaxRetries is the maximum number of retry attempts
	MaxRetries int `json:"max_retries"`

	// InitialBackoff is the initial wait time before first retry
	InitialBackoff time.Duration `json:"initial_backoff"`

	// MaxBackoff is the maximum wait time between retries
	MaxBackoff time.Duration `json:"max_backoff"`

	// BackoffMultiplier is the factor by which backoff increases
	BackoffMultiplier float64 `json:"backoff_multiplier"`
}

// AnalysisConfig configures code analysis behavior.
type AnalysisConfig struct {
	// MaxFileSize is the maximum file size to analyze (bytes)
	MaxFileSize int64 `json:"max_file_size"`

	// MaxFiles is the maximum number of files to analyze per repo
	MaxFiles int `json:"max_files"`

	// IgnorePatterns are glob patterns for files to ignore
	IgnorePatterns []string `json:"ignore_patterns"`

	// EnableDeepAnalysis enables call graph and behavior extraction
	EnableDeepAnalysis bool `json:"enable_deep_analysis"`

	// ConcurrentFiles is the number of files to analyze in parallel
	ConcurrentFiles int `json:"concurrent_files"`
}

// ServerConfig configures the MCP server.
type ServerConfig struct {
	// Transport is the transport type: "stdio" or "http" (future)
	Transport string `json:"transport"`

	// Address for HTTP transport
	Address string `json:"address,omitempty"`

	// MaxConcurrentOps is the maximum concurrent operations
	MaxConcurrentOps int `json:"max_concurrent_ops"`

	// RequestTimeout is the timeout for individual requests
	RequestTimeout time.Duration `json:"request_timeout"`
}

// LoggingConfig configures logging behavior.
type LoggingConfig struct {
	// Level is the log level: "debug", "info", "warn", "error"
	Level string `json:"level"`

	// Format is the log format: "json" or "text"
	Format string `json:"format"`

	// Output is where to write logs: "stderr", "stdout", or a file path
	Output string `json:"output"`
}

// DefaultConfig returns a configuration with sensible defaults.
func DefaultConfig() *Config {
	homeDir, _ := os.UserHomeDir()
	dataDir := filepath.Join(homeDir, ".mcp-repo-context")

	return &Config{
		Storage: StorageConfig{
			Type:                "filesystem",
			Path:                filepath.Join(dataDir, "contexts"),
			MaxContextAge:       24 * time.Hour,
			EnableMetadataIndex: true,
			MetadataIndexPath:   filepath.Join(dataDir, "metadata-index.json"),
		},
		AI: AIConfig{
			Provider:    "anthropic",
			Model:       "claude-sonnet-4-20250514",
			MaxTokens:   2048,
			Temperature: 0.3,
			Retry: RetryConfig{
				MaxRetries:        3,
				InitialBackoff:    1 * time.Second,
				MaxBackoff:        30 * time.Second,
				BackoffMultiplier: 2.0,
			},
			Timeout: 60 * time.Second,
		},
		Analysis: AnalysisConfig{
			MaxFileSize: 1024 * 1024, // 1MB
			MaxFiles:    10000,
			IgnorePatterns: []string{
				"**/vendor/**",
				"**/node_modules/**",
				"**/.git/**",
				"**/dist/**",
				"**/build/**",
				"**/*.min.js",
				"**/*.min.css",
			},
			EnableDeepAnalysis: true,
			ConcurrentFiles:    4,
		},
		Server: ServerConfig{
			Transport:        "stdio",
			MaxConcurrentOps: 10,
			RequestTimeout:   5 * time.Minute,
		},
		Logging: LoggingConfig{
			Level:  "info",
			Format: "json",
			Output: "stderr",
		},
	}
}

// LoadConfig loads configuration from a file.
// Missing values are filled with defaults.
func LoadConfig(path string) (*Config, error) {
	config := DefaultConfig()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return config, nil // Use defaults if no config file
		}
		return nil, err
	}

	if err := json.Unmarshal(data, config); err != nil {
		return nil, err
	}

	return config, nil
}

// LoadConfigFromEnv loads configuration with environment variable overrides.
func LoadConfigFromEnv(configPath string) (*Config, error) {
	config, err := LoadConfig(configPath)
	if err != nil {
		return nil, err
	}

	// Override with environment variables
	if apiKey := os.Getenv("ANTHROPIC_API_KEY"); apiKey != "" {
		config.AI.APIKey = apiKey
	}

	if dataDir := os.Getenv("MCP_DATA_DIR"); dataDir != "" {
		config.Storage.Path = filepath.Join(dataDir, "contexts")
		config.Storage.MetadataIndexPath = filepath.Join(dataDir, "metadata-index.json")
	}

	if model := os.Getenv("MCP_AI_MODEL"); model != "" {
		config.AI.Model = model
	}

	if logLevel := os.Getenv("MCP_LOG_LEVEL"); logLevel != "" {
		config.Logging.Level = logLevel
	}

	return config, nil
}

// SaveConfig saves configuration to a file.
func SaveConfig(config *Config, path string) error {
	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}

// Validate checks that the configuration is valid.
func (c *Config) Validate() error {
	if c.Storage.Path == "" {
		return ErrInvalidConfig("storage.path is required")
	}

	if c.AI.MaxTokens <= 0 {
		return ErrInvalidConfig("ai.max_tokens must be positive")
	}

	if c.AI.Temperature < 0 || c.AI.Temperature > 1 {
		return ErrInvalidConfig("ai.temperature must be between 0 and 1")
	}

	if c.Analysis.MaxFileSize <= 0 {
		return ErrInvalidConfig("analysis.max_file_size must be positive")
	}

	if c.Analysis.ConcurrentFiles <= 0 {
		c.Analysis.ConcurrentFiles = 1 // Default to 1 if not set
	}

	return nil
}

// ConfigError represents a configuration error.
type ConfigError struct {
	Message string
}

func (e ConfigError) Error() string {
	return "config error: " + e.Message
}

// ErrInvalidConfig creates a new configuration error.
func ErrInvalidConfig(message string) error {
	return ConfigError{Message: message}
}
