package recipes

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// ServiceInfo describes a service/repo in the architecture.
type ServiceInfo struct {
	Name          string `json:"name"`
	RepoID        string `json:"repo_id"`
	Language      string `json:"language"`
	Framework     string `json:"framework"`
	FileCount     int    `json:"file_count"`
	FunctionCount int    `json:"function_count"`
	TestFileCount int    `json:"test_file_count"`
	Purpose       string `json:"purpose"`
}

// SharedPattern describes a pattern shared across repos.
type SharedPattern struct {
	Pattern   string   `json:"pattern"`
	Value     string   `json:"value"`
	RepoCount int      `json:"repo_count"`
	Repos     []string `json:"repos"`
}

// RepoHealth describes health indicators for a repo.
type RepoHealth struct {
	RepoID        string   `json:"repo_id"`
	TestFiles     int      `json:"test_files"`
	TestRatio     float64  `json:"test_ratio"`
	FunctionCount int      `json:"function_count"`
	HasReadme     bool     `json:"has_readme"`
	HasCIConfig   bool     `json:"has_ci_config"`
	Issues        []string `json:"issues"`
}

// ArchitectureRecipe implements the review_architecture recipe.
type ArchitectureRecipe struct{}

var _ Recipe = (*ArchitectureRecipe)(nil)

func (r *ArchitectureRecipe) Name() string { return "review_architecture" }

func (r *ArchitectureRecipe) Description() string {
	return "Assess architecture across multiple repositories or an org with health indicators and recommendations"
}

func (r *ArchitectureRecipe) InputSchema() map[string]FieldSpec {
	return map[string]FieldSpec{
		"org_id": {
			Name:        "org_id",
			Type:        "string",
			Required:    false,
			Description: "Org ID (one of org_id or repo_ids required)",
		},
		"repo_ids": {
			Name:        "repo_ids",
			Type:        "[]string",
			Required:    false,
			Description: "Repo IDs (alternative to org_id)",
		},
		"token_budget": {
			Name:        "token_budget",
			Type:        "int",
			Required:    false,
			Default:     12000,
			Description: "Max context tokens",
		},
		"focus_areas": {
			Name:        "focus_areas",
			Type:        "[]string",
			Required:    false,
			Description: "Focus: dependencies, testing, patterns, health",
		},
	}
}

func (r *ArchitectureRecipe) Run(ctx context.Context, runner *RecipeRunner, input RecipeInput) (*RecipeResult, error) {
	start := time.Now()

	orgID := input.GetString("org_id")
	repoIDs := input.GetStringSlice("repo_ids")

	// Validate: at least one of org_id or repo_ids required
	if orgID == "" && len(repoIDs) == 0 {
		return nil, fmt.Errorf("at least one of org_id or repo_ids is required")
	}

	result := &RecipeResult{
		Recipe:  "review_architecture",
		Data:    make(map[string]any),
		Sources: []RecipeSourceRef{},
		Gaps:    []GapNote{},
	}

	// Step 1: Resolve Repo List
	if orgID != "" && len(repoIDs) == 0 {
		orgData, err := runner.OrgManager().GetOrg(orgID)
		if err != nil {
			return nil, fmt.Errorf("failed to get org: %w", err)
		}
		repoIDs = extractRepoIDsFromOrg(orgData)
	}

	if len(repoIDs) == 0 {
		return nil, fmt.Errorf("no repositories found")
	}

	// Step 2: Service Inventory
	var services []ServiceInfo
	healthMap := make(map[string]RepoHealth)

	for _, repoID := range repoIDs {
		repoCtx, err := runner.Manager().GetContext(repoID, "full")
		if err != nil {
			result.Gaps = append(result.Gaps, GapNote{
				Section:    "service_inventory",
				Reason:     fmt.Sprintf("Failed to get context for %s: %v", repoID, err),
				Suggestion: fmt.Sprintf("Analyze %s first with analyze_repo or analyze_local", repoID),
			})
			continue
		}

		svc := extractServiceInfo(repoID, repoCtx)
		services = append(services, svc)

		// Step 5: Health Indicators
		health := computeHealth(repoID, repoCtx)
		healthMap[repoID] = health
	}

	result.Data["services"] = services

	// Step 3: Dependency Analysis (V1: gap)
	result.Data["dependency_graph"] = nil
	result.Gaps = append(result.Gaps, GapNote{
		Section:    "dependency_graph",
		Reason:     "Dependency analysis requires 02-dependency-graph",
		Suggestion: "Implement 02-dependency-graph for cross-repo dependency tracking",
	})

	// Step 4: Pattern Analysis
	patterns := analyzePatterns(services, repoIDs)
	result.Data["shared_patterns"] = patterns

	// Health indicators
	result.Data["health_indicators"] = healthMap

	// Aggregate issues
	var allIssues []string
	for _, health := range healthMap {
		for _, issue := range health.Issues {
			allIssues = append(allIssues, fmt.Sprintf("%s: %s", health.RepoID, issue))
		}
	}
	result.Data["issues"] = allIssues

	// Step 6: AI Recommendations
	if runner.AI() != nil {
		analysis := r.generateRecommendations(ctx, runner, services, patterns, healthMap)
		result.Analysis = analysis
	}

	hasAI := runner.AI() != nil
	result.Confidence = CalculateConfidence(hasAI, len(result.Gaps))
	result.DurationMS = time.Since(start).Milliseconds()

	return result, nil
}

// extractRepoIDsFromOrg extracts repo IDs from org data.
func extractRepoIDsFromOrg(orgData interface{}) []string {
	if data, ok := orgData.(map[string]interface{}); ok {
		if repos, ok := data["repos"].([]interface{}); ok {
			result := make([]string, 0, len(repos))
			for _, r := range repos {
				if s, ok := r.(string); ok {
					result = append(result, s)
				}
			}
			return result
		}
		if repos, ok := data["repos"].([]string); ok {
			return repos
		}
	}
	return nil
}

// extractServiceInfo extracts service metadata from repo context.
func extractServiceInfo(repoID string, repoCtx interface{}) ServiceInfo {
	svc := ServiceInfo{
		RepoID: repoID,
		Name:   repoID,
	}

	data, ok := repoCtx.(map[string]interface{})
	if !ok {
		return svc
	}

	// Extract name from ID
	parts := strings.Split(repoID, "/")
	if len(parts) > 0 {
		svc.Name = parts[len(parts)-1]
	}

	// Statistics
	if stats, ok := data["statistics"].(map[string]interface{}); ok {
		if fc, ok := stats["total_files"].(int); ok {
			svc.FileCount = fc
		} else if fc, ok := stats["total_files"].(float64); ok {
			svc.FileCount = int(fc)
		}
		if fnc, ok := stats["function_count"].(int); ok {
			svc.FunctionCount = fnc
		} else if fnc, ok := stats["function_count"].(float64); ok {
			svc.FunctionCount = int(fnc)
		}
	}

	// Architecture
	if arch, ok := data["architecture"].(map[string]interface{}); ok {
		if deps, ok := arch["dependencies"].([]interface{}); ok {
			svc.Framework = detectFramework(deps)
		}
		if deps, ok := arch["dependencies"].([]string); ok {
			ifaces := make([]interface{}, len(deps))
			for i, d := range deps {
				ifaces[i] = d
			}
			svc.Framework = detectFramework(ifaces)
		}
	}

	// Language
	if stats, ok := data["statistics"].(map[string]interface{}); ok {
		if langs, ok := stats["languages"].(map[string]interface{}); ok {
			maxCount := 0
			for lang, count := range langs {
				if c, ok := count.(int); ok && c > maxCount {
					maxCount = c
					svc.Language = lang
				} else if c, ok := count.(float64); ok && int(c) > maxCount {
					maxCount = int(c)
					svc.Language = lang
				}
			}
		}
	}

	// AI Summary purpose
	if ai, ok := data["ai_summary"].(map[string]interface{}); ok {
		if purpose, ok := ai["summary"].(string); ok {
			svc.Purpose = purpose
		}
	}

	return svc
}

// detectFramework identifies the primary framework from dependencies.
func detectFramework(deps []interface{}) string {
	frameworks := map[string]string{
		"gorilla/mux":      "gorilla/mux",
		"go-chi/chi":       "chi",
		"gin-gonic/gin":    "gin",
		"labstack/echo":    "echo",
		"gofiber/fiber":    "fiber",
		"grpc":             "gRPC",
		"net/http":         "net/http",
	}

	for _, dep := range deps {
		depStr, ok := dep.(string)
		if !ok {
			continue
		}
		for pattern, name := range frameworks {
			if strings.Contains(depStr, pattern) {
				return name
			}
		}
	}
	return ""
}

// computeHealth calculates health indicators for a repo.
func computeHealth(repoID string, repoCtx interface{}) RepoHealth {
	health := RepoHealth{
		RepoID: repoID,
	}

	data, ok := repoCtx.(map[string]interface{})
	if !ok {
		return health
	}

	totalFiles := 0
	testFiles := 0
	functionCount := 0

	if stats, ok := data["statistics"].(map[string]interface{}); ok {
		if fc, ok := stats["total_files"].(int); ok {
			totalFiles = fc
		} else if fc, ok := stats["total_files"].(float64); ok {
			totalFiles = int(fc)
		}
		if fnc, ok := stats["function_count"].(int); ok {
			functionCount = fnc
		} else if fnc, ok := stats["function_count"].(float64); ok {
			functionCount = int(fnc)
		}
	}

	// Count test files from file list
	if files, ok := data["files"].(map[string]interface{}); ok {
		for path := range files {
			if strings.HasSuffix(path, "_test.go") || strings.Contains(path, "test_") || strings.HasSuffix(path, ".test.js") || strings.HasSuffix(path, ".test.ts") {
				testFiles++
			}
		}
	}

	// Check for test_file_count in statistics
	if stats, ok := data["statistics"].(map[string]interface{}); ok {
		if tfc, ok := stats["test_file_count"].(int); ok {
			testFiles = tfc
		} else if tfc, ok := stats["test_file_count"].(float64); ok {
			testFiles = int(tfc)
		}
	}

	health.TestFiles = testFiles
	health.FunctionCount = functionCount
	if totalFiles > 0 {
		health.TestRatio = float64(testFiles) / float64(totalFiles)
	}

	// Check README
	if files, ok := data["files"].(map[string]interface{}); ok {
		for path := range files {
			lower := strings.ToLower(path)
			if lower == "readme.md" || lower == "readme" || lower == "readme.txt" {
				health.HasReadme = true
			}
			if strings.Contains(lower, ".github/workflows") || lower == ".gitlab-ci.yml" || lower == "jenkinsfile" {
				health.HasCIConfig = true
			}
		}
	}

	// Detect issues
	if health.TestRatio < 0.1 && totalFiles > 0 {
		health.Issues = append(health.Issues, "Low test coverage")
	}
	if !health.HasReadme {
		health.Issues = append(health.Issues, "Missing documentation")
	}
	if !health.HasCIConfig {
		health.Issues = append(health.Issues, "No CI/CD detected")
	}
	if functionCount > 500 {
		health.Issues = append(health.Issues, "Large codebase, consider splitting")
	}

	return health
}

// analyzePatterns detects shared and divergent patterns across repos.
func analyzePatterns(services []ServiceInfo, repoIDs []string) []SharedPattern {
	// Framework patterns
	frameworkCounts := make(map[string][]string) // framework -> repos
	languageCounts := make(map[string][]string)

	for _, svc := range services {
		if svc.Framework != "" {
			frameworkCounts[svc.Framework] = append(frameworkCounts[svc.Framework], svc.RepoID)
		}
		if svc.Language != "" {
			languageCounts[svc.Language] = append(languageCounts[svc.Language], svc.RepoID)
		}
	}

	var patterns []SharedPattern

	for framework, repos := range frameworkCounts {
		patterns = append(patterns, SharedPattern{
			Pattern:   "router framework",
			Value:     framework,
			RepoCount: len(repos),
			Repos:     repos,
		})
	}

	for lang, repos := range languageCounts {
		patterns = append(patterns, SharedPattern{
			Pattern:   "language",
			Value:     lang,
			RepoCount: len(repos),
			Repos:     repos,
		})
	}

	return patterns
}

// generateRecommendations creates AI recommendations.
func (r *ArchitectureRecipe) generateRecommendations(ctx context.Context, runner *RecipeRunner,
	services []ServiceInfo, patterns []SharedPattern, healthMap map[string]RepoHealth) string {

	var sb strings.Builder
	sb.WriteString("Architecture review:\n")
	fmt.Fprintf(&sb, "- %d services analyzed\n", len(services))
	for _, svc := range services {
		fmt.Fprintf(&sb, "  - %s (%s, %s, %d functions)\n", svc.Name, svc.Language, svc.Framework, svc.FunctionCount)
	}
	sb.WriteString("\nPatterns:\n")
	for _, p := range patterns {
		fmt.Fprintf(&sb, "- %s: %s (used by %d repos)\n", p.Pattern, p.Value, p.RepoCount)
	}
	sb.WriteString("\nHealth issues:\n")
	for _, health := range healthMap {
		for _, issue := range health.Issues {
			fmt.Fprintf(&sb, "- %s: %s\n", health.RepoID, issue)
		}
	}
	sb.WriteString("\nProvide 3-5 architectural recommendations focusing on consistency, testing gaps, and improvements.")

	prompt := sb.String()
	if len(prompt) > 4000 {
		prompt = prompt[:4000]
	}

	response, _, err := runner.AI().CompleteRaw(ctx, prompt, 1500)
	if err != nil {
		return ""
	}
	return response
}
