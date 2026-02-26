package analyzer

import (
	"context"

	ctxpkg "github.com/yashpalc/mcp-repo-context/internal/context"
	"github.com/yashpalc/mcp-repo-context/internal/repo"
)

// Analyzer extracts context from source code files.
type Analyzer interface {
	// Name returns the name of this analyzer.
	Name() string

	// Languages returns file extensions this analyzer handles.
	Languages() []string

	// AnalyzeFile extracts context from a single file.
	AnalyzeFile(ctx context.Context, file repo.FileInfo, content []byte) (*ctxpkg.FileContext, error)
}

// Registry manages language-specific analyzers.
type Registry interface {
	// Get returns the appropriate analyzer for a language.
	Get(lang string) Analyzer
}
