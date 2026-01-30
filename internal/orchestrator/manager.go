package orchestrator

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/yashpalc/mcp-repo-context/internal/ai"
	"github.com/yashpalc/mcp-repo-context/internal/analyzer"
	ctxpkg "github.com/yashpalc/mcp-repo-context/internal/context"
	"github.com/yashpalc/mcp-repo-context/internal/prreview"
	"github.com/yashpalc/mcp-repo-context/internal/repo"
	"github.com/yashpalc/mcp-repo-context/internal/storage"
)

// Ensure manager implements Manager interface.
var _ Manager = (*manager)(nil)

// manager orchestrates repository analysis and context management (private struct).
type manager struct {
	store      storage.ContextStore
	cloner     repo.Source
	scanner    repo.FileScanner
	registry   analyzer.Registry
	aiRegistry *ai.Registry

	// Per-repo locking instead of global mutex
	locks *LockManager
}

// NewManager creates a new context manager.
func NewManager(store storage.ContextStore, cloner repo.Source, scanner repo.FileScanner) Manager {
	return &manager{
		store:      store,
		cloner:     cloner,
		scanner:    scanner,
		registry:   analyzer.NewRegistry(),
		aiRegistry: ai.NewRegistryFromEnv(),
		locks:      NewLockManager(),
	}
}

// NewManagerWithAI creates a new context manager with custom AI registry.
func NewManagerWithAI(store storage.ContextStore, cloner repo.Source, scanner repo.FileScanner, aiReg *ai.Registry) Manager {
	return &manager{
		store:      store,
		cloner:     cloner,
		scanner:    scanner,
		registry:   analyzer.NewRegistry(),
		aiRegistry: aiReg,
		locks:      NewLockManager(),
	}
}

// AnalyzeRepo analyzes a repository and stores the context.
func (m *manager) AnalyzeRepo(ctx context.Context, repoURL string, opts AnalyzeOptions) (*AnalyzeResult, error) {
	startTime := time.Now()
	result := &AnalyzeResult{}

	// Generate repo ID
	repoID := repo.RepoIDFromURL(repoURL)
	result.RepoID = repoID

	// Acquire per-repo lock (allows concurrent analysis of different repos)
	if err := m.locks.Lock(ctx, repoID); err != nil {
		return nil, fmt.Errorf("failed to acquire lock for %s: %w", repoID, err)
	}
	defer m.locks.Unlock(repoID)

	// Check if context exists and is fresh
	if !opts.Force {
		exists, lastUpdated, err := m.store.ContextExists(ctx, repoID, opts.MaxAge)
		if err == nil && exists {
			result.Warnings = append(result.Warnings, fmt.Sprintf("Using cached context from %s", lastUpdated.Format(time.RFC3339)))
			return result, nil
		}
	}

	// Clone repository
	localPath, cleanup, err := m.cloner.Clone(ctx, repoURL, repo.CloneOptions{
		Branch:      opts.Branch,
		Depth:       1, // Shallow clone for MVP
		GitHubToken: opts.GitHubToken,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to clone repository: %w", err)
	}
	defer cleanup()

	// Get commit info
	commitHash, err := m.cloner.GetCurrentCommit(ctx, localPath)
	if err != nil {
		commitHash = "unknown"
		result.Warnings = append(result.Warnings, "Could not determine commit hash")
	}

	branch, err := m.cloner.GetBranch(ctx, localPath)
	if err != nil {
		branch = opts.Branch
		if branch == "" {
			branch = "main"
		}
	}

	// Initialize repo context
	repoCtx := &ctxpkg.RepoContext{
		ID:         repoID,
		URL:        repoURL,
		Branch:     branch,
		CommitHash: commitHash,
		AnalyzedAt: time.Now(),
		Files:      make(map[string]*ctxpkg.FileContext),
		Statistics: ctxpkg.RepoStatistics{
			LanguageBreakdown: make(map[string]int),
		},
		Version: 1,
	}

	// Scan and analyze files
	err = m.scanner.Scan(ctx, localPath, func(file repo.FileInfo) error {
		// Check for context cancellation before processing each file
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("analysis cancelled: %w", err)
		}

		// Read file content
		content, err := os.ReadFile(file.AbsPath)
		if err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("Could not read %s: %v", file.Path, err))
			return nil
		}

		// Get appropriate analyzer
		a := m.registry.Get(file.Language)

		// Analyze file
		fileCtx, err := a.AnalyzeFile(ctx, file, content)
		if err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("Analysis failed for %s: %v", file.Path, err))
			return nil
		}

		repoCtx.Files[file.Path] = fileCtx
		result.FileCount++

		// Update statistics
		repoCtx.Statistics.TotalFiles++
		repoCtx.Statistics.TotalLines += fileCtx.LineCount
		repoCtx.Statistics.LanguageBreakdown[file.Language]++
		repoCtx.Statistics.FunctionCount += len(fileCtx.Functions)
		repoCtx.Statistics.TypeCount += len(fileCtx.Types)
		repoCtx.Statistics.ExportCount += len(fileCtx.Exports)

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to scan repository: %w", err)
	}

	// Generate architecture context
	repoCtx.Architecture = m.generateArchitecture(repoCtx)

	// Build call graph and populate CalledBy relationships
	callGraphBuilder := analyzer.NewCallGraphBuilder()
	repoCtx.CallGraph = callGraphBuilder.BuildFromFiles(repoCtx.Files)
	analyzer.PopulateCalledBy(repoCtx.Files)

	// Build search index
	searchIndexBuilder := analyzer.NewSearchIndexBuilder()
	repoCtx.SearchIndex = searchIndexBuilder.BuildFromFiles(repoCtx.Files)

	// Update statistics with deep analysis info
	if repoCtx.CallGraph != nil {
		repoCtx.Statistics.CallGraphNodes = len(repoCtx.CallGraph.Nodes)
		repoCtx.Statistics.CallGraphEdges = len(repoCtx.CallGraph.Edges)
	}
	if repoCtx.SearchIndex != nil {
		repoCtx.Statistics.IndexedConcepts = len(repoCtx.SearchIndex.Concepts)
	}

	// Calculate average complexity
	totalComplexity := 0
	funcCount := 0
	for _, fileCtx := range repoCtx.Files {
		for _, fn := range fileCtx.Functions {
			totalComplexity += fn.Complexity
			funcCount++
		}
	}
	if funcCount > 0 {
		repoCtx.Statistics.AvgComplexity = float64(totalComplexity) / float64(funcCount)
	}

	// Store context
	if err := m.store.StoreRepoContext(ctx, repoID, repoCtx); err != nil {
		return nil, fmt.Errorf("failed to store context: %w", err)
	}

	result.Duration = time.Since(startTime)
	return result, nil
}

// GetContext retrieves repository context.
func (m *manager) GetContext(ctx context.Context, repoID string) (*ctxpkg.RepoContext, error) {
	return m.store.GetRepoContext(ctx, repoID)
}

// GetFileContext retrieves context for a specific file.
func (m *manager) GetFileContext(ctx context.Context, repoID, filePath string) (*ctxpkg.FileContext, error) {
	return m.store.GetFileContext(ctx, repoID, filePath)
}

// GetFunctionContext retrieves comprehensive context for a specific function.
func (m *manager) GetFunctionContext(ctx context.Context, repoID, filePath, funcName string) (*FunctionContextResult, error) {
	repoCtx, err := m.store.GetRepoContext(ctx, repoID)
	if err != nil {
		return nil, err
	}

	fileCtx, ok := repoCtx.Files[filePath]
	if !ok {
		return nil, fmt.Errorf("file not found: %s", filePath)
	}

	// Find the function
	var targetFunc *ctxpkg.FunctionDef
	for i := range fileCtx.Functions {
		if fileCtx.Functions[i].Name == funcName {
			targetFunc = &fileCtx.Functions[i]
			break
		}
	}
	if targetFunc == nil {
		return nil, fmt.Errorf("function not found: %s", funcName)
	}

	result := &FunctionContextResult{
		Function: *targetFunc,
		FilePath: filePath,
		RepoID:   repoID,
		File: FileInfo{
			Path:     filePath,
			Language: fileCtx.Language,
			Purpose:  fileCtx.Purpose,
		},
	}

	// Get callers (functions that call this one)
	result.Callers = m.findCallers(repoCtx, funcName)

	// Get related types (types used in params/returns)
	result.RelatedTypes = m.findRelatedTypes(fileCtx, targetFunc)

	return result, nil
}

// FunctionContextResult contains comprehensive function context.
type FunctionContextResult struct {
	Function     ctxpkg.FunctionDef `json:"function"`
	FilePath     string             `json:"file_path"`
	RepoID       string             `json:"repo_id"`
	File         FileInfo           `json:"file"`
	Callers      []ctxpkg.CallRef   `json:"callers"`
	RelatedTypes []TypeInfo         `json:"related_types"`
}

// FileInfo provides brief file information.
type FileInfo struct {
	Path     string `json:"path"`
	Language string `json:"language"`
	Purpose  string `json:"purpose"`
}

// TypeInfo provides brief type information.
type TypeInfo struct {
	Name   string `json:"name"`
	Kind   string `json:"kind"`
	File   string `json:"file"`
}

func (m *manager) findCallers(repoCtx *ctxpkg.RepoContext, funcName string) []ctxpkg.CallRef {
	var callers []ctxpkg.CallRef
	for path, fileCtx := range repoCtx.Files {
		for _, fn := range fileCtx.Functions {
			for _, call := range fn.Calls {
				if call.Function == funcName {
					callers = append(callers, ctxpkg.CallRef{
						Function: fn.Name,
						File:     path,
						Line:     call.Line,
						Type:     "internal",
					})
				}
			}
		}
	}
	return callers
}

func (m *manager) findRelatedTypes(fileCtx *ctxpkg.FileContext, fn *ctxpkg.FunctionDef) []TypeInfo {
	var types []TypeInfo
	typeNames := make(map[string]bool)

	// Collect type names from parameters
	for _, param := range fn.Parameters {
		typeName := cleanTypeName(param.Type)
		if typeName != "" && !isBuiltinType(typeName) {
			typeNames[typeName] = true
		}
	}

	// Collect type names from returns
	for _, ret := range fn.Returns {
		typeName := cleanTypeName(ret)
		if typeName != "" && !isBuiltinType(typeName) {
			typeNames[typeName] = true
		}
	}

	// Find type definitions
	for _, t := range fileCtx.Types {
		if typeNames[t.Name] {
			types = append(types, TypeInfo{
				Name: t.Name,
				Kind: t.Kind,
				File: fileCtx.Path,
			})
		}
	}

	return types
}

func cleanTypeName(typeName string) string {
	typeName = strings.TrimPrefix(typeName, "*")
	typeName = strings.TrimPrefix(typeName, "[]")
	typeName = strings.TrimPrefix(typeName, "map[")
	if idx := strings.Index(typeName, "]"); idx >= 0 {
		typeName = typeName[idx+1:]
	}
	if idx := strings.Index(typeName, "."); idx >= 0 {
		typeName = typeName[idx+1:]
	}
	return typeName
}

func isBuiltinType(typeName string) bool {
	builtins := map[string]bool{
		"string": true, "int": true, "int8": true, "int16": true, "int32": true, "int64": true,
		"uint": true, "uint8": true, "uint16": true, "uint32": true, "uint64": true,
		"float32": true, "float64": true, "complex64": true, "complex128": true,
		"bool": true, "byte": true, "rune": true, "error": true, "any": true,
		"interface{}": true, "interface": true,
	}
	return builtins[typeName]
}

// SearchFunctions searches for functions by query.
func (m *manager) SearchFunctions(ctx context.Context, repoID, query string) ([]ctxpkg.FunctionRef, error) {
	repoCtx, err := m.store.GetRepoContext(ctx, repoID)
	if err != nil {
		return nil, err
	}

	if repoCtx.SearchIndex == nil {
		return nil, fmt.Errorf("search index not available for repository")
	}

	return analyzer.SearchFunctions(repoCtx.SearchIndex, query), nil
}

// SearchByConcept searches for functions related to a concept.
func (m *manager) SearchByConcept(ctx context.Context, repoID, concept string) ([]ctxpkg.FunctionRef, error) {
	repoCtx, err := m.store.GetRepoContext(ctx, repoID)
	if err != nil {
		return nil, err
	}

	if repoCtx.SearchIndex == nil {
		return nil, fmt.Errorf("search index not available for repository")
	}

	return analyzer.SearchByConcept(repoCtx.SearchIndex, concept), nil
}

// SearchBySideEffect searches for functions with specific side effects.
func (m *manager) SearchBySideEffect(ctx context.Context, repoID, effect string) ([]ctxpkg.FunctionRef, error) {
	repoCtx, err := m.store.GetRepoContext(ctx, repoID)
	if err != nil {
		return nil, err
	}

	if repoCtx.SearchIndex == nil {
		return nil, fmt.Errorf("search index not available for repository")
	}

	return analyzer.SearchBySideEffect(repoCtx.SearchIndex, effect), nil
}

// ListRepos returns all analyzed repositories.
func (m *manager) ListRepos(ctx context.Context) ([]ctxpkg.ContextMetadata, error) {
	return m.store.ListContexts(ctx)
}

// generateArchitecture creates architecture overview from analyzed files.
func (m *manager) generateArchitecture(repoCtx *ctxpkg.RepoContext) *ctxpkg.ArchitectureContext {
	arch := &ctxpkg.ArchitectureContext{
		Modules: []ctxpkg.Module{},
	}

	// Group files by directory (module)
	moduleFiles := make(map[string][]string)
	for path := range repoCtx.Files {
		dir := filepath.Dir(path)
		if dir == "." {
			dir = "root"
		}
		moduleFiles[dir] = append(moduleFiles[dir], path)
	}

	// Create modules
	for dir, files := range moduleFiles {
		mod := ctxpkg.Module{
			Path:       dir,
			Name:       filepath.Base(dir),
			Files:      files,
			IsInternal: strings.HasPrefix(dir, "internal"),
		}

		// Try to determine module purpose from file names
		for _, f := range files {
			if f == "main.go" || filepath.Base(f) == "main.go" {
				arch.MainPackages = append(arch.MainPackages, dir)
				arch.EntryPoints = append(arch.EntryPoints, ctxpkg.EntryPoint{
					Path:    f,
					Type:    "main",
					Purpose: "Application entry point",
				})
			}
		}

		arch.Modules = append(arch.Modules, mod)
	}

	// Detect build system
	for path := range repoCtx.Files {
		switch filepath.Base(path) {
		case "go.mod":
			arch.BuildSystem = "go modules"
		case "Makefile":
			if arch.BuildSystem == "" {
				arch.BuildSystem = "make"
			}
		case "package.json":
			arch.BuildSystem = "npm"
		}
	}

	// Generate overview
	arch.Overview = fmt.Sprintf(
		"Repository with %d files across %d modules. %d public exports, %d types, %d functions.",
		repoCtx.Statistics.TotalFiles,
		len(arch.Modules),
		repoCtx.Statistics.ExportCount,
		repoCtx.Statistics.TypeCount,
		repoCtx.Statistics.FunctionCount,
	)

	return arch
}

// IsAIEnabled returns true if AI features are available.
func (m *manager) IsAIEnabled() bool {
	return m.aiRegistry != nil && m.aiRegistry.HasConfiguredProvider()
}

// GenerateAISummary generates AI-powered summary for a repository.
func (m *manager) GenerateAISummary(ctx context.Context, repoID string) (*ctxpkg.AISummary, error) {
	if !m.IsAIEnabled() {
		return nil, ai.ErrProviderNotConfigured
	}

	// Get existing context
	repoCtx, err := m.store.GetRepoContext(ctx, repoID)
	if err != nil {
		return nil, fmt.Errorf("failed to get repository context: %w", err)
	}

	// Get AI provider
	provider, err := m.aiRegistry.GetDefault()
	if err != nil {
		return nil, err
	}

	// Build request
	req := m.buildSummaryRequest(repoCtx)

	// Generate summary
	resp, err := provider.GenerateSummary(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("AI summary generation failed: %w", err)
	}

	// Convert to context type
	summary := &ctxpkg.AISummary{
		Overview:          resp.Overview,
		Purpose:           resp.Purpose,
		KeyFeatures:       resp.KeyFeatures,
		TechnologyStack:   resp.TechnologyStack,
		ArchitectureStyle: resp.ArchitectureStyle,
		MainComponents:    resp.MainComponents,
		Suggestions:       resp.Suggestions,
		GeneratedAt:       time.Now(),
		TokensUsed:        resp.TokensUsed,
		Provider:          provider.Name(),
	}

	// Update stored context
	repoCtx.AISummary = summary
	if err := m.store.StoreRepoContext(ctx, repoID, repoCtx); err != nil {
		return nil, fmt.Errorf("failed to store updated context: %w", err)
	}

	return summary, nil
}

// GenerateAIArchAnalysis generates AI-powered architecture analysis.
func (m *manager) GenerateAIArchAnalysis(ctx context.Context, repoID string) (*ctxpkg.AIArchAnalysis, error) {
	if !m.IsAIEnabled() {
		return nil, ai.ErrProviderNotConfigured
	}

	// Get existing context
	repoCtx, err := m.store.GetRepoContext(ctx, repoID)
	if err != nil {
		return nil, fmt.Errorf("failed to get repository context: %w", err)
	}

	// Get AI provider
	provider, err := m.aiRegistry.GetDefault()
	if err != nil {
		return nil, err
	}

	// Build request
	req := m.buildArchitectureRequest(repoCtx)

	// Generate analysis
	resp, err := provider.AnalyzeArchitecture(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("AI architecture analysis failed: %w", err)
	}

	// Convert to context type
	layers := make([]ctxpkg.LayerInfo, len(resp.Layers))
	for i, l := range resp.Layers {
		layers[i] = ctxpkg.LayerInfo{
			Name:       l.Name,
			Purpose:    l.Purpose,
			Components: l.Components,
		}
	}

	analysis := &ctxpkg.AIArchAnalysis{
		Pattern:         resp.Pattern,
		Layers:          layers,
		DataFlow:        resp.DataFlow,
		Strengths:       resp.Strengths,
		Weaknesses:      resp.Weaknesses,
		Recommendations: resp.Recommendations,
		GeneratedAt:     time.Now(),
		TokensUsed:      resp.TokensUsed,
		Provider:        provider.Name(),
	}

	// Update stored context
	if repoCtx.Architecture == nil {
		repoCtx.Architecture = &ctxpkg.ArchitectureContext{}
	}
	repoCtx.Architecture.AIAnalysis = analysis
	if err := m.store.StoreRepoContext(ctx, repoID, repoCtx); err != nil {
		return nil, fmt.Errorf("failed to store updated context: %w", err)
	}

	return analysis, nil
}

// buildSummaryRequest builds an AI summary request from repo context.
func (m *manager) buildSummaryRequest(repoCtx *ctxpkg.RepoContext) ai.SummaryRequest {
	req := ai.SummaryRequest{
		RepoName:   repoCtx.ID,
		RepoURL:    repoCtx.URL,
		FileCount:  repoCtx.Statistics.TotalFiles,
		TotalLines: repoCtx.Statistics.TotalLines,
		Languages:  repoCtx.Statistics.LanguageBreakdown,
	}

	// Add modules
	if repoCtx.Architecture != nil {
		for _, mod := range repoCtx.Architecture.Modules {
			req.Modules = append(req.Modules, ai.ModuleInfo{
				Name:     mod.Name,
				Path:     mod.Path,
				Purpose:  mod.Purpose,
				Files:    len(mod.Files),
				Internal: mod.IsInternal,
			})
		}
		req.MainPackages = repoCtx.Architecture.MainPackages
		for _, ep := range repoCtx.Architecture.EntryPoints {
			req.EntryPoints = append(req.EntryPoints, ep.Path)
		}
		req.Dependencies = repoCtx.Architecture.Dependencies
	}

	// Add key functions and types
	for _, file := range repoCtx.Files {
		for _, fn := range file.Functions {
			if fn.IsPublic {
				req.KeyFunctions = append(req.KeyFunctions, ai.FunctionInfo{
					Name:      fn.Name,
					Signature: fn.Signature,
					File:      file.Path,
					IsPublic:  fn.IsPublic,
				})
			}
		}
		for _, t := range file.Types {
			if t.IsPublic {
				req.KeyTypes = append(req.KeyTypes, ai.TypeInfo{
					Name:     t.Name,
					Kind:     t.Kind,
					File:     file.Path,
					IsPublic: t.IsPublic,
				})
			}
		}
	}

	return req
}

// buildArchitectureRequest builds an AI architecture request from repo context.
func (m *manager) buildArchitectureRequest(repoCtx *ctxpkg.RepoContext) ai.ArchitectureRequest {
	req := ai.ArchitectureRequest{
		RepoName:      repoCtx.ID,
		FileStructure: make(map[string][]string),
	}

	// Add modules
	if repoCtx.Architecture != nil {
		for _, mod := range repoCtx.Architecture.Modules {
			req.Modules = append(req.Modules, ai.ModuleInfo{
				Name:     mod.Name,
				Path:     mod.Path,
				Purpose:  mod.Purpose,
				Files:    len(mod.Files),
				Internal: mod.IsInternal,
			})
			req.FileStructure[mod.Path] = mod.Files
		}
		for _, ep := range repoCtx.Architecture.EntryPoints {
			req.EntryPoints = append(req.EntryPoints, ep.Path)
		}
		req.Dependencies = repoCtx.Architecture.Dependencies
	}

	return req
}

// RefreshAIContext updates all repositories with AI-generated summaries.
// If force is true, re-generates AI context even if it already exists.
func (m *manager) RefreshAIContext(ctx context.Context, repoIDs []string, force bool) (*RefreshResult, error) {
	if !m.IsAIEnabled() {
		return nil, ai.ErrProviderNotConfigured
	}

	result := &RefreshResult{
		Updated: []string{},
		Skipped: []string{},
		Failed:  []string{},
		Errors:  []string{},
	}

	// Get list of repos to process
	var repos []string
	if len(repoIDs) > 0 {
		repos = repoIDs
	} else {
		metas, err := m.store.ListContexts(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to list repos: %w", err)
		}
		for _, meta := range metas {
			repos = append(repos, meta.RepoID)
		}
	}

	// Process each repo
	for _, repoID := range repos {
		repoCtx, err := m.store.GetRepoContext(ctx, repoID)
		if err != nil {
			result.Failed = append(result.Failed, repoID)
			result.Errors = append(result.Errors, fmt.Sprintf("%s: failed to get context: %v", repoID, err))
			continue
		}

		// Check if already has AI summary (unless force refresh)
		if !force && repoCtx.AISummary != nil && repoCtx.Architecture != nil && repoCtx.Architecture.AIAnalysis != nil {
			result.Skipped = append(result.Skipped, repoID)
			continue
		}

		// Generate AI summary
		summary, err := m.GenerateAISummary(ctx, repoID)
		if err != nil {
			result.Failed = append(result.Failed, repoID)
			result.Errors = append(result.Errors, fmt.Sprintf("%s: summary failed: %v", repoID, err))
			continue
		}
		result.TokensUsed += summary.TokensUsed

		// Generate AI architecture analysis
		archAnalysis, err := m.GenerateAIArchAnalysis(ctx, repoID)
		if err != nil {
			// Summary succeeded but arch failed - still count as partial success
			result.Updated = append(result.Updated, repoID+" (summary only)")
			result.Errors = append(result.Errors, fmt.Sprintf("%s: architecture analysis failed: %v", repoID, err))
			continue
		}
		result.TokensUsed += archAnalysis.TokensUsed

		result.Updated = append(result.Updated, repoID)
	}

	return result, nil
}

// Ask answers a natural language query about repositories using AI.
func (m *manager) Ask(ctx context.Context, query string, repoIDs []string) (*QueryResult, error) {
	if !m.IsAIEnabled() {
		return nil, ai.ErrProviderNotConfigured
	}

	// Get AI provider
	provider, err := m.aiRegistry.GetDefault()
	if err != nil {
		return nil, err
	}

	// Get repository contexts
	var repos []*ctxpkg.RepoContext
	if len(repoIDs) > 0 {
		for _, id := range repoIDs {
			repoCtx, err := m.store.GetRepoContext(ctx, id)
			if err != nil {
				continue // Skip repos that don't exist
			}
			repos = append(repos, repoCtx)
		}
	} else {
		// Get all repos
		metas, err := m.store.ListContexts(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to list repos: %w", err)
		}
		for _, meta := range metas {
			repoCtx, err := m.store.GetRepoContext(ctx, meta.RepoID)
			if err != nil {
				continue
			}
			repos = append(repos, repoCtx)
		}
	}

	if len(repos) == 0 {
		return nil, fmt.Errorf("no repositories found to query")
	}

	// Extract relevant context
	extractor := ai.NewContextExtractor().
		WithMaxFiles(10).
		WithMaxFunctions(15).
		WithMaxTokens(6000)

	relevantCtx := extractor.Extract(query, repos)

	// Create query handler and get answer
	handler := ai.NewQueryHandler(provider)
	resp, err := handler.Answer(ctx, query, relevantCtx)
	if err != nil {
		return nil, fmt.Errorf("failed to get AI answer: %w", err)
	}

	// Convert response
	sources := make([]SourceRef, len(resp.Sources))
	for i, s := range resp.Sources {
		sources[i] = SourceRef{
			RepoID:   s.RepoID,
			FilePath: s.FilePath,
			Function: s.Function,
			Line:     s.Line,
		}
	}

	return &QueryResult{
		Answer:      resp.Answer,
		Sources:     sources,
		Confidence:  resp.Confidence,
		TokensUsed:  resp.TokensUsed,
		ContextUsed: resp.ContextUsed,
		QueryType:   string(ai.ClassifyQuery(query)),
	}, nil
}

// CheckPRContext checks if context exists for the PR's repository.
func (m *manager) CheckPRContext(ctx context.Context, prURL string) (*prreview.ContextStatus, error) {
	// Parse PR URL
	owner, repoName, _, err := prreview.ParsePRURL(prURL)
	if err != nil {
		return nil, fmt.Errorf("invalid PR URL: %w", err)
	}

	// Generate repo ID
	repoID := prreview.RepoIDFromPR(owner, repoName)

	// Check if context exists
	exists, lastAnalyzed, err := m.store.ContextExists(ctx, repoID, 0)
	if err != nil {
		return &prreview.ContextStatus{
			RepoID:     repoID,
			HasContext: false,
			Message:    fmt.Sprintf("Error checking context: %v", err),
		}, nil
	}

	if !exists {
		return &prreview.ContextStatus{
			RepoID:     repoID,
			HasContext: false,
			Message:    fmt.Sprintf("No context found for repository %s. Would you like to generate it?", repoID),
		}, nil
	}

	return &prreview.ContextStatus{
		RepoID:       repoID,
		HasContext:   true,
		LastAnalyzed: lastAnalyzed,
		Message:      fmt.Sprintf("Context available for %s (analyzed: %s)", repoID, lastAnalyzed.Format("2006-01-02 15:04")),
	}, nil
}

// ReviewPR reviews a pull request using AI and repository context.
func (m *manager) ReviewPR(ctx context.Context, prURL string, opts prreview.ReviewOptions) (*prreview.ReviewResult, error) {
	if !m.IsAIEnabled() {
		return nil, ai.ErrProviderNotConfigured
	}

	// Parse PR URL
	owner, repoName, number, err := prreview.ParsePRURL(prURL)
	if err != nil {
		return nil, fmt.Errorf("invalid PR URL: %w", err)
	}

	// Get AI provider
	provider, err := m.aiRegistry.GetDefault()
	if err != nil {
		return nil, err
	}

	// Create GitHub client
	ghClient := prreview.NewGitHubClient(opts.GitHubToken)

	// Fetch PR info
	prInfo, err := ghClient.FetchPRInfo(ctx, owner, repoName, number)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch PR info: %w", err)
	}

	// Get repository context if available and requested
	var repoCtx *ctxpkg.RepoContext
	if opts.UseRepoContext {
		repoID := prreview.RepoIDFromPR(owner, repoName)
		repoCtx, err = m.store.GetRepoContext(ctx, repoID)
		if err != nil {
			// Context not available - that's okay, we'll review without it
			repoCtx = nil
		}
	}

	// Create reviewer and perform review
	reviewer := prreview.NewReviewer(provider, opts.GitHubToken)
	result, err := reviewer.Review(ctx, prInfo, repoCtx, opts)
	if err != nil {
		return nil, fmt.Errorf("review failed: %w", err)
	}

	return result, nil
}
