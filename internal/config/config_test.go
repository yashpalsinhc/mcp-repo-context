package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	// Check storage defaults
	if config.Storage.Type != "filesystem" {
		t.Errorf("Expected storage type 'filesystem', got '%s'", config.Storage.Type)
	}
	if config.Storage.MaxContextAge != 24*time.Hour {
		t.Errorf("Expected max context age 24h, got %v", config.Storage.MaxContextAge)
	}
	if !config.Storage.EnableMetadataIndex {
		t.Error("Expected metadata index to be enabled by default")
	}

	// Check AI defaults
	if config.AI.Provider != "anthropic" {
		t.Errorf("Expected AI provider 'anthropic', got '%s'", config.AI.Provider)
	}
	if config.AI.MaxTokens != 2048 {
		t.Errorf("Expected max tokens 2048, got %d", config.AI.MaxTokens)
	}
	if config.AI.Retry.MaxRetries != 3 {
		t.Errorf("Expected 3 max retries, got %d", config.AI.Retry.MaxRetries)
	}

	// Check analysis defaults
	if config.Analysis.MaxFileSize != 1024*1024 {
		t.Errorf("Expected max file size 1MB, got %d", config.Analysis.MaxFileSize)
	}
	if !config.Analysis.EnableDeepAnalysis {
		t.Error("Expected deep analysis to be enabled by default")
	}

	// Check server defaults
	if config.Server.Transport != "stdio" {
		t.Errorf("Expected transport 'stdio', got '%s'", config.Server.Transport)
	}
}

func TestConfigValidation(t *testing.T) {
	config := DefaultConfig()

	// Valid config should pass
	if err := config.Validate(); err != nil {
		t.Errorf("Valid config should pass validation: %v", err)
	}

	// Empty storage path should fail
	config.Storage.Path = ""
	if err := config.Validate(); err == nil {
		t.Error("Empty storage path should fail validation")
	}
	config.Storage.Path = "/tmp/test" // Restore

	// Invalid max tokens should fail
	config.AI.MaxTokens = 0
	if err := config.Validate(); err == nil {
		t.Error("Zero max tokens should fail validation")
	}
	config.AI.MaxTokens = 2048 // Restore

	// Invalid temperature should fail
	config.AI.Temperature = 2.0
	if err := config.Validate(); err == nil {
		t.Error("Temperature > 1 should fail validation")
	}
	config.AI.Temperature = 0.3 // Restore

	// Invalid max file size should fail
	config.Analysis.MaxFileSize = 0
	if err := config.Validate(); err == nil {
		t.Error("Zero max file size should fail validation")
	}
}

func TestLoadConfig_FileNotExist(t *testing.T) {
	config, err := LoadConfig("/nonexistent/path/config.json")
	if err != nil {
		t.Errorf("Should return default config for missing file: %v", err)
	}
	if config == nil {
		t.Error("Config should not be nil")
	}
	if config.AI.Provider != "anthropic" {
		t.Error("Should have default values")
	}
}

func TestSaveAndLoadConfig(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "config-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	configPath := filepath.Join(tmpDir, "config.json")

	// Create custom config
	config := DefaultConfig()
	config.AI.MaxTokens = 4096
	config.AI.Model = "custom-model"
	config.Storage.MaxContextAge = 48 * time.Hour

	// Save config
	if err := SaveConfig(config, configPath); err != nil {
		t.Fatalf("Failed to save config: %v", err)
	}

	// Load config
	loaded, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Verify values
	if loaded.AI.MaxTokens != 4096 {
		t.Errorf("Expected max tokens 4096, got %d", loaded.AI.MaxTokens)
	}
	if loaded.AI.Model != "custom-model" {
		t.Errorf("Expected model 'custom-model', got '%s'", loaded.AI.Model)
	}
	if loaded.Storage.MaxContextAge != 48*time.Hour {
		t.Errorf("Expected max context age 48h, got %v", loaded.Storage.MaxContextAge)
	}
}

func TestLoadConfigFromEnv(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "config-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	configPath := filepath.Join(tmpDir, "config.json")

	// Save default config
	if err := SaveConfig(DefaultConfig(), configPath); err != nil {
		t.Fatalf("Failed to save config: %v", err)
	}

	// Set environment variables
	os.Setenv("ANTHROPIC_API_KEY", "test-api-key")
	os.Setenv("MCP_AI_MODEL", "env-model")
	os.Setenv("MCP_LOG_LEVEL", "debug")
	defer func() {
		os.Unsetenv("ANTHROPIC_API_KEY")
		os.Unsetenv("MCP_AI_MODEL")
		os.Unsetenv("MCP_LOG_LEVEL")
	}()

	// Load config with env overrides
	config, err := LoadConfigFromEnv(configPath)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Check env overrides
	if config.AI.APIKey != "test-api-key" {
		t.Errorf("Expected API key from env, got '%s'", config.AI.APIKey)
	}
	if config.AI.Model != "env-model" {
		t.Errorf("Expected model from env, got '%s'", config.AI.Model)
	}
	if config.Logging.Level != "debug" {
		t.Errorf("Expected log level from env, got '%s'", config.Logging.Level)
	}
}

func TestConfigError(t *testing.T) {
	err := ErrInvalidConfig("test error")
	if err.Error() != "config error: test error" {
		t.Errorf("Expected 'config error: test error', got '%s'", err.Error())
	}
}
