package analyzer

import (
	"context"
	"strings"
	"time"

	ctxpkg "github.com/yashpalc/mcp-repo-context/internal/context"
	"github.com/yashpalc/mcp-repo-context/internal/repo"
)

// Ensure genericAnalyzer implements Analyzer interface.
var _ Analyzer = (*genericAnalyzer)(nil)

// genericAnalyzer provides basic analysis for unsupported languages (private struct).
type genericAnalyzer struct{}

// newGenericAnalyzer creates a new generic analyzer.
func newGenericAnalyzer() Analyzer {
	return &genericAnalyzer{}
}

// Languages returns file extensions this analyzer handles.
func (a *genericAnalyzer) Languages() []string {
	return []string{"unknown"}
}

// AnalyzeFile extracts basic context from any file.
func (a *genericAnalyzer) AnalyzeFile(ctx context.Context, file repo.FileInfo, content []byte) (*ctxpkg.FileContext, error) {
	fileCtx := &ctxpkg.FileContext{
		Path:       file.Path,
		Hash:       file.Hash,
		Language:   file.Language,
		Size:       file.Size,
		LineCount:  countLines(content),
		Purpose:    a.guessPurpose(file.Path, file.Language),
		AnalyzedAt: time.Now(),
	}

	// Extract basic concepts from filename
	fileCtx.Concepts = a.extractBasicConcepts(file.Path)

	return fileCtx, nil
}

func (a *genericAnalyzer) guessPurpose(path, language string) string {
	name := strings.ToLower(path)

	switch {
	case strings.Contains(name, "readme"):
		return "Project documentation"
	case strings.Contains(name, "license"):
		return "License file"
	case strings.Contains(name, "dockerfile"):
		return "Docker container definition"
	case strings.Contains(name, "makefile"):
		return "Build automation"
	case strings.Contains(name, "config"):
		return "Configuration file"
	case strings.Contains(name, "test"):
		return "Test file"
	case strings.HasSuffix(name, ".md"):
		return "Documentation"
	case strings.HasSuffix(name, ".yaml") || strings.HasSuffix(name, ".yml"):
		return "YAML configuration"
	case strings.HasSuffix(name, ".json"):
		return "JSON data/configuration"
	case strings.HasSuffix(name, ".sql"):
		return "SQL database script"
	case strings.HasSuffix(name, ".sh"):
		return "Shell script"
	case strings.Contains(name, ".github/workflows"):
		return "GitHub Actions workflow"
	default:
		return language + " source file"
	}
}

func (a *genericAnalyzer) extractBasicConcepts(path string) []string {
	concepts := []string{}

	// Extract from path components
	parts := strings.Split(path, "/")
	for _, part := range parts {
		// Remove extension
		part = strings.TrimSuffix(part, strings.ToLower(strings.TrimPrefix(part, ".")))
		if len(part) > 2 {
			concepts = append(concepts, strings.ToLower(part))
		}
	}

	return concepts
}
