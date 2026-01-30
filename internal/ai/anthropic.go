package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	anthropicAPIURL     = "https://api.anthropic.com/v1/messages"
	anthropicAPIVersion = "2023-06-01"
	defaultModel        = "claude-sonnet-4-20250514"
)

// AnthropicProvider implements the Provider interface for Anthropic's Claude.
type AnthropicProvider struct {
	apiKey      string
	model       string
	maxTokens   int
	temperature float64
	client      *http.Client
	retryer     *Retryer
}

// NewAnthropicProvider creates a new Anthropic provider.
func NewAnthropicProvider(config Config) *AnthropicProvider {
	model := config.Model
	if model == "" {
		model = defaultModel
	}

	maxTokens := config.MaxTokens
	if maxTokens == 0 {
		maxTokens = 2048
	}

	temperature := config.Temperature
	if temperature == 0 {
		temperature = 0.3
	}

	return &AnthropicProvider{
		apiKey:      config.APIKey,
		model:       model,
		maxTokens:   maxTokens,
		temperature: temperature,
		client: &http.Client{
			Timeout: 60 * time.Second,
		},
		retryer: NewRetryer(DefaultRetryConfig()),
	}
}

// Name returns the provider name.
func (p *AnthropicProvider) Name() string {
	return "anthropic"
}

// IsConfigured returns true if the provider has valid credentials.
func (p *AnthropicProvider) IsConfigured() bool {
	return p.apiKey != ""
}

// GenerateSummary generates a summary for the repository.
func (p *AnthropicProvider) GenerateSummary(ctx context.Context, req SummaryRequest) (*SummaryResponse, error) {
	if !p.IsConfigured() {
		return nil, ErrProviderNotConfigured
	}

	prompt := buildSummaryPrompt(req)

	response, tokensUsed, err := p.complete(ctx, prompt, p.maxTokens)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrGenerationFailed, err)
	}

	// Parse the response
	summary, err := parseSummaryResponse(response)
	if err != nil {
		// Fallback to simple parsing
		summary = &SummaryResponse{
			Overview: response,
		}
	}
	summary.TokensUsed = tokensUsed

	return summary, nil
}

// AnalyzeArchitecture generates architecture analysis.
func (p *AnthropicProvider) AnalyzeArchitecture(ctx context.Context, req ArchitectureRequest) (*ArchitectureResponse, error) {
	if !p.IsConfigured() {
		return nil, ErrProviderNotConfigured
	}

	prompt := buildArchitecturePrompt(req)

	response, tokensUsed, err := p.complete(ctx, prompt, p.maxTokens)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrGenerationFailed, err)
	}

	// Parse the response
	arch, err := parseArchitectureResponse(response)
	if err != nil {
		arch = &ArchitectureResponse{
			Pattern: "Unknown",
			DataFlow: response,
		}
	}
	arch.TokensUsed = tokensUsed

	return arch, nil
}

// GenerateDescription generates a description for code.
func (p *AnthropicProvider) GenerateDescription(ctx context.Context, req DescriptionRequest) (string, error) {
	if !p.IsConfigured() {
		return "", ErrProviderNotConfigured
	}

	prompt := buildDescriptionPrompt(req)

	response, _, err := p.complete(ctx, prompt, 256)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrGenerationFailed, err)
	}

	return strings.TrimSpace(response), nil
}

// CompleteRaw sends a raw completion request and returns the response.
// This is exposed for use cases like PR review that need direct prompting.
func (p *AnthropicProvider) CompleteRaw(ctx context.Context, prompt string, maxTokens int) (string, int, error) {
	return p.complete(ctx, prompt, maxTokens)
}

// complete sends a completion request to the Anthropic API with retry logic.
func (p *AnthropicProvider) complete(ctx context.Context, prompt string, maxTokens int) (string, int, error) {
	reqBody := anthropicRequest{
		Model:     p.model,
		MaxTokens: maxTokens,
		Messages: []anthropicMessage{
			{
				Role:    "user",
				Content: prompt,
			},
		},
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return "", 0, fmt.Errorf("failed to marshal request: %w", err)
	}

	var result string
	var tokensUsed int

	// Use retry logic for API calls
	err = p.retryer.Do(ctx, func() error {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, anthropicAPIURL, bytes.NewReader(jsonBody))
		if err != nil {
			return fmt.Errorf("failed to create request: %w", err)
		}

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("x-api-key", p.apiKey)
		req.Header.Set("anthropic-version", anthropicAPIVersion)

		resp, err := p.client.Do(req)
		if err != nil {
			return fmt.Errorf("request failed: %w", err)
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("failed to read response: %w", err)
		}

		if resp.StatusCode != http.StatusOK {
			// Return APIError for proper retry handling
			return NewAPIError("anthropic", resp.StatusCode, string(body))
		}

		var apiResp anthropicResponse
		if err := json.Unmarshal(body, &apiResp); err != nil {
			return fmt.Errorf("failed to parse response: %w", err)
		}

		if len(apiResp.Content) == 0 {
			return fmt.Errorf("empty response from API")
		}

		result = apiResp.Content[0].Text
		tokensUsed = apiResp.Usage.InputTokens + apiResp.Usage.OutputTokens
		return nil
	})

	if err != nil {
		return "", 0, err
	}

	return result, tokensUsed, nil
}

// Anthropic API types
type anthropicRequest struct {
	Model     string             `json:"model"`
	MaxTokens int                `json:"max_tokens"`
	Messages  []anthropicMessage `json:"messages"`
}

type anthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type anthropicResponse struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	Usage struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

// buildSummaryPrompt creates the prompt for summary generation.
func buildSummaryPrompt(req SummaryRequest) string {
	var sb strings.Builder

	sb.WriteString("Analyze this repository and provide a structured summary.\n\n")
	sb.WriteString(fmt.Sprintf("Repository: %s\n", req.RepoName))
	sb.WriteString(fmt.Sprintf("URL: %s\n", req.RepoURL))
	sb.WriteString(fmt.Sprintf("Files: %d, Lines: %d\n\n", req.FileCount, req.TotalLines))

	sb.WriteString("Languages:\n")
	for lang, count := range req.Languages {
		sb.WriteString(fmt.Sprintf("- %s: %d files\n", lang, count))
	}

	if len(req.Modules) > 0 {
		sb.WriteString("\nModules:\n")
		for _, mod := range req.Modules {
			internal := ""
			if mod.Internal {
				internal = " (internal)"
			}
			sb.WriteString(fmt.Sprintf("- %s%s: %d files\n", mod.Path, internal, mod.Files))
		}
	}

	if len(req.EntryPoints) > 0 {
		sb.WriteString("\nEntry Points:\n")
		for _, ep := range req.EntryPoints {
			sb.WriteString(fmt.Sprintf("- %s\n", ep))
		}
	}

	if len(req.KeyFunctions) > 0 {
		sb.WriteString("\nKey Functions:\n")
		for _, fn := range req.KeyFunctions[:min(10, len(req.KeyFunctions))] {
			sb.WriteString(fmt.Sprintf("- %s: %s\n", fn.Name, fn.Signature))
		}
	}

	if len(req.KeyTypes) > 0 {
		sb.WriteString("\nKey Types:\n")
		for _, t := range req.KeyTypes[:min(10, len(req.KeyTypes))] {
			sb.WriteString(fmt.Sprintf("- %s (%s)\n", t.Name, t.Kind))
		}
	}

	sb.WriteString(`
Please provide your analysis in the following JSON format:
{
  "overview": "A 2-3 sentence high-level description of what this repository does",
  "purpose": "The main purpose and problem this code solves",
  "key_features": ["feature1", "feature2", "feature3"],
  "technology_stack": ["tech1", "tech2"],
  "architecture_style": "The architectural pattern used (e.g., MVC, microservices, monolith, etc.)",
  "main_components": ["component1", "component2"],
  "suggestions": ["Optional improvement suggestions"]
}

Return ONLY the JSON, no additional text.`)

	return sb.String()
}

// buildArchitecturePrompt creates the prompt for architecture analysis.
func buildArchitecturePrompt(req ArchitectureRequest) string {
	var sb strings.Builder

	sb.WriteString("Analyze the architecture of this repository.\n\n")
	sb.WriteString(fmt.Sprintf("Repository: %s\n\n", req.RepoName))

	sb.WriteString("Modules:\n")
	for _, mod := range req.Modules {
		sb.WriteString(fmt.Sprintf("- %s: %s (%d files)\n", mod.Name, mod.Purpose, mod.Files))
	}

	if len(req.EntryPoints) > 0 {
		sb.WriteString("\nEntry Points:\n")
		for _, ep := range req.EntryPoints {
			sb.WriteString(fmt.Sprintf("- %s\n", ep))
		}
	}

	if len(req.Dependencies) > 0 {
		sb.WriteString("\nExternal Dependencies:\n")
		for _, dep := range req.Dependencies[:min(15, len(req.Dependencies))] {
			sb.WriteString(fmt.Sprintf("- %s\n", dep))
		}
	}

	sb.WriteString(`
Please provide your architecture analysis in the following JSON format:
{
  "pattern": "The main architectural pattern (e.g., layered, hexagonal, microservices)",
  "layers": [
    {"name": "layer_name", "purpose": "what this layer does", "components": ["comp1", "comp2"]}
  ],
  "data_flow": "Description of how data flows through the system",
  "strengths": ["strength1", "strength2"],
  "weaknesses": ["weakness1", "weakness2"],
  "recommendations": ["recommendation1", "recommendation2"]
}

Return ONLY the JSON, no additional text.`)

	return sb.String()
}

// buildDescriptionPrompt creates the prompt for description generation.
func buildDescriptionPrompt(req DescriptionRequest) string {
	return fmt.Sprintf(`Generate a brief, clear description for this %s.

Name: %s
Signature: %s

Context: %s

Code:
%s

Provide a 1-2 sentence description that explains what this %s does and why it exists. Be concise and technical.`,
		req.CodeType, req.Name, req.Signature, req.Context, req.Content, req.CodeType)
}

// parseSummaryResponse parses the AI response into a SummaryResponse.
func parseSummaryResponse(response string) (*SummaryResponse, error) {
	// Try to extract JSON from the response
	response = strings.TrimSpace(response)

	// Remove markdown code blocks if present
	if strings.HasPrefix(response, "```") {
		lines := strings.Split(response, "\n")
		var jsonLines []string
		inJSON := false
		for _, line := range lines {
			if strings.HasPrefix(line, "```") {
				inJSON = !inJSON
				continue
			}
			if inJSON || !strings.HasPrefix(line, "```") {
				jsonLines = append(jsonLines, line)
			}
		}
		response = strings.Join(jsonLines, "\n")
	}

	var summary SummaryResponse
	if err := json.Unmarshal([]byte(response), &summary); err != nil {
		return nil, err
	}
	return &summary, nil
}

// parseArchitectureResponse parses the AI response into an ArchitectureResponse.
func parseArchitectureResponse(response string) (*ArchitectureResponse, error) {
	response = strings.TrimSpace(response)

	// Remove markdown code blocks if present
	if strings.HasPrefix(response, "```") {
		lines := strings.Split(response, "\n")
		var jsonLines []string
		inJSON := false
		for _, line := range lines {
			if strings.HasPrefix(line, "```") {
				inJSON = !inJSON
				continue
			}
			if inJSON || !strings.HasPrefix(line, "```") {
				jsonLines = append(jsonLines, line)
			}
		}
		response = strings.Join(jsonLines, "\n")
	}

	var arch ArchitectureResponse
	if err := json.Unmarshal([]byte(response), &arch); err != nil {
		return nil, err
	}
	return &arch, nil
}
