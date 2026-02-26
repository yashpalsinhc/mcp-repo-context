package ai

import (
	"context"
	"errors"
)

// ErrProviderNotConfigured is returned when no API key is set.
var ErrProviderNotConfigured = errors.New("AI provider not configured: missing API key")

// ErrGenerationFailed is returned when AI generation fails.
var ErrGenerationFailed = errors.New("AI generation failed")

// Provider defines the interface for AI/LLM providers.
type Provider interface {
	// GenerateSummary generates a summary for the given content.
	GenerateSummary(ctx context.Context, req SummaryRequest) (*SummaryResponse, error)

	// AnalyzeArchitecture generates architecture analysis.
	AnalyzeArchitecture(ctx context.Context, req ArchitectureRequest) (*ArchitectureResponse, error)

	// GenerateDescription generates a description for code.
	GenerateDescription(ctx context.Context, req DescriptionRequest) (string, error)

	// CompleteRaw sends a raw completion request and returns the response with token count.
	CompleteRaw(ctx context.Context, prompt string, maxTokens int) (string, int, error)

	// IsConfigured returns true if the provider has valid credentials.
	IsConfigured() bool

	// Name returns the provider name.
	Name() string
}

// SummaryRequest contains data for summary generation.
type SummaryRequest struct {
	RepoName       string
	RepoURL        string
	FileCount      int
	TotalLines     int
	Languages      map[string]int
	MainPackages   []string
	Modules        []ModuleInfo
	KeyFunctions   []FunctionInfo
	KeyTypes       []TypeInfo
	EntryPoints    []string
	Dependencies   []string
	MaxTokens      int
}

// ModuleInfo provides module details for summary.
type ModuleInfo struct {
	Name     string
	Path     string
	Purpose  string
	Files    int
	Internal bool
}

// FunctionInfo provides function details for summary.
type FunctionInfo struct {
	Name      string
	Signature string
	File      string
	IsPublic  bool
}

// TypeInfo provides type details for summary.
type TypeInfo struct {
	Name     string
	Kind     string
	File     string
	IsPublic bool
}

// SummaryResponse contains the generated summary.
type SummaryResponse struct {
	Overview           string   `json:"overview"`
	Purpose            string   `json:"purpose"`
	KeyFeatures        []string `json:"key_features"`
	TechnologyStack    []string `json:"technology_stack"`
	ArchitectureStyle  string   `json:"architecture_style"`
	MainComponents     []string `json:"main_components"`
	Suggestions        []string `json:"suggestions,omitempty"`
	TokensUsed         int      `json:"tokens_used"`
}

// ArchitectureRequest contains data for architecture analysis.
type ArchitectureRequest struct {
	RepoName     string
	Modules      []ModuleInfo
	EntryPoints  []string
	Dependencies []string
	CallPatterns []string
	FileStructure map[string][]string
	MaxTokens    int
}

// ArchitectureResponse contains architecture analysis.
type ArchitectureResponse struct {
	Pattern           string              `json:"pattern"`
	Layers            []LayerInfo         `json:"layers"`
	DataFlow          string              `json:"data_flow"`
	Strengths         []string            `json:"strengths"`
	Weaknesses        []string            `json:"weaknesses"`
	Recommendations   []string            `json:"recommendations"`
	ComponentDiagram  string              `json:"component_diagram,omitempty"`
	TokensUsed        int                 `json:"tokens_used"`
}

// LayerInfo describes an architecture layer.
type LayerInfo struct {
	Name        string   `json:"name"`
	Purpose     string   `json:"purpose"`
	Components  []string `json:"components"`
}

// DescriptionRequest contains data for description generation.
type DescriptionRequest struct {
	CodeType    string // function, type, file, module
	Name        string
	Signature   string
	Content     string
	Context     string
	MaxTokens   int
}

// Config holds AI provider configuration.
type Config struct {
	Provider      string // anthropic, openai, ollama
	APIKey        string
	Model         string
	MaxTokens     int
	Temperature   float64
	BaseURL       string // For custom endpoints
}

// DefaultConfig returns default AI configuration.
func DefaultConfig() Config {
	return Config{
		Provider:    "anthropic",
		Model:       "claude-sonnet-4-20250514",
		MaxTokens:   2048,
		Temperature: 0.3,
	}
}
