package orchestrator

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/yashpalc/mcp-repo-context/internal/analyzer"
	ctxpkg "github.com/yashpalc/mcp-repo-context/internal/context"
	"github.com/yashpalc/mcp-repo-context/internal/repo"
)

// AnalyzeLocalOptions configures local directory analysis.
type AnalyzeLocalOptions struct {
	Force      bool          // Force re-analysis even if cached
	MaxAge     time.Duration // Consider cache fresh if newer than this
	IncludeAll bool          // Include all files (not just code files)
}

// AnalyzeLocalResult contains the result of local analysis.
type AnalyzeLocalResult struct {
	ProjectID     string
	ProjectName   string
	ProjectPath   string
	FileCount     int
	Duration      time.Duration
	Warnings      []string
	IsNewAnalysis bool
}

// AnalyzeLocal analyzes a local directory and stores the context.
func (m *manager) AnalyzeLocal(ctx context.Context, dirPath string, opts AnalyzeLocalOptions) (*AnalyzeLocalResult, error) {
	startTime := time.Now()

	// Resolve to absolute path
	absPath, err := filepath.Abs(dirPath)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve path: %w", err)
	}

	// Check if directory exists
	info, err := os.Stat(absPath)
	if err != nil {
		return nil, fmt.Errorf("directory not found: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("path is not a directory: %s", absPath)
	}

	// Create project ID from path
	projectID := "local:" + absPath
	projectName := filepath.Base(absPath)

	// Check if we have existing context
	existingCtx, err := m.store.GetRepoContext(ctx, projectID)
	if err == nil && existingCtx != nil && !opts.Force {
		// Check if cache is still fresh
		if opts.MaxAge > 0 {
			cacheAge := time.Since(existingCtx.AnalyzedAt)
			if cacheAge < opts.MaxAge {
				log.Printf("using cached context for %s (age: %s)", projectID, cacheAge.Round(time.Second))
				return &AnalyzeLocalResult{
					ProjectID:     projectID,
					ProjectName:   projectName,
					ProjectPath:   absPath,
					FileCount:     len(existingCtx.Files),
					Duration:      time.Since(startTime),
					IsNewAnalysis: false,
				}, nil
			}
		}
	}

	log.Printf("analyzing local directory: %s", absPath)

	// Create new context
	repoCtx := &ctxpkg.RepoContext{
		ID:         projectID,
		URL:        "file://" + absPath,
		Branch:     "local",
		CommitHash: fmt.Sprintf("local-%d", time.Now().Unix()),
		AnalyzedAt: time.Now(),
		Files:      make(map[string]*ctxpkg.FileContext),
	}

	// Walk the directory and analyze files
	var warnings []string
	fileCount := 0

	err = filepath.Walk(absPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("error accessing %s: %v", path, err))
			return nil
		}

		// Skip hidden directories and common non-code directories
		if info.IsDir() {
			name := info.Name()
			if strings.HasPrefix(name, ".") || isSkippableDir(name) {
				return filepath.SkipDir
			}
			return nil
		}

		// Get relative path
		relPath, err := filepath.Rel(absPath, path)
		if err != nil {
			return nil
		}

		// Check if it's a code file we can analyze
		if !isCodeFile(path) && !opts.IncludeAll {
			return nil
		}

		// Read file content
		content, err := os.ReadFile(path)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("error reading %s: %v", relPath, err))
			return nil
		}

		// Get appropriate analyzer based on file extension
		ext := strings.TrimPrefix(filepath.Ext(path), ".")
		analyzer := m.registry.Get(ext)

		// Create FileInfo for analyzer
		fileInfo := repo.FileInfo{
			Path:    relPath,
			AbsPath: path,
			Size:    info.Size(),
		}

		// Analyze the file
		fileCtx, err := analyzer.AnalyzeFile(ctx, fileInfo, content)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("error analyzing %s: %v", relPath, err))
			return nil
		}

		repoCtx.Files[relPath] = fileCtx
		fileCount++

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to walk directory: %w", err)
	}

	// Parse go.mod if present
	goModPath := filepath.Join(absPath, "go.mod")
	if goModContent, err := os.ReadFile(goModPath); err == nil {
		modInfo, err := analyzer.ParseGoMod(goModContent)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("Failed to parse go.mod: %v", err))
		} else {
			repoCtx.ModuleInfo = modInfo
		}
	}

	// Parse config files
	for path := range repoCtx.Files {
		absConfigPath := filepath.Join(absPath, path)
		cfContent, err := os.ReadFile(absConfigPath)
		if err != nil {
			continue
		}
		configType, structured, err := analyzer.ParseConfigFile(path, cfContent)
		if err != nil || configType == "" {
			continue
		}
		repoCtx.ConfigFiles = append(repoCtx.ConfigFiles, ctxpkg.ConfigFile{
			Path:           path,
			Type:           configType,
			Content:        string(cfContent),
			StructuredJSON: structured,
		})
	}

	// Aggregate imports
	if repoCtx.ModuleInfo != nil {
		repoCtx.ImportSummary = analyzer.AggregateImports(repoCtx.Files, repoCtx.ModuleInfo)
	}

	// Generate architecture
	repoCtx.Architecture = m.generateArchitecture(repoCtx)

	// Build call graph using the analyzer function
	callGraphBuilder := &callGraphBuilderHelper{}
	repoCtx.CallGraph = callGraphBuilder.BuildFromFiles(repoCtx.Files)

	// Build search index
	searchIndexBuilder := &searchIndexBuilderHelper{}
	repoCtx.SearchIndex = searchIndexBuilder.BuildFromFiles(repoCtx.Files)

	// Populate CalledBy relationships
	populateCalledBy(repoCtx.Files)

	// Save context
	if err := m.store.StoreRepoContext(ctx, projectID, repoCtx); err != nil {
		return nil, fmt.Errorf("failed to save context: %w", err)
	}

	duration := time.Since(startTime)
	log.Printf("analyzed %d files in %s", fileCount, duration.Round(time.Millisecond))

	return &AnalyzeLocalResult{
		ProjectID:     projectID,
		ProjectName:   projectName,
		ProjectPath:   absPath,
		FileCount:     fileCount,
		Duration:      duration,
		Warnings:      warnings,
		IsNewAnalysis: true,
	}, nil
}

// callGraphBuilderHelper wraps the analyzer's call graph builder
type callGraphBuilderHelper struct{}

func (h *callGraphBuilderHelper) BuildFromFiles(files map[string]*ctxpkg.FileContext) *ctxpkg.CallGraph {
	graph := &ctxpkg.CallGraph{
		Nodes: make(map[string]*ctxpkg.CallGraphNode),
		Edges: []ctxpkg.CallGraphEdge{},
	}

	// Build nodes for all functions
	for path, fileCtx := range files {
		for _, fn := range fileCtx.Functions {
			nodeID := path + ":" + fn.Name
			node := &ctxpkg.CallGraphNode{
				ID:        nodeID,
				File:      path,
				Function:  fn.Name,
				Signature: fn.Signature,
				IsPublic:  fn.IsPublic,
				Calls:     []string{},
				CalledBy:  []string{},
			}

			// Add calls from this function
			for _, call := range fn.Calls {
				if call.Type == "internal" {
					// Try to resolve to a node ID
					for otherPath, otherFileCtx := range files {
						for _, otherFn := range otherFileCtx.Functions {
							if otherFn.Name == call.Function {
								targetID := otherPath + ":" + otherFn.Name
								node.Calls = append(node.Calls, targetID)
								graph.Edges = append(graph.Edges, ctxpkg.CallGraphEdge{
									From: nodeID,
									To:   targetID,
									Line: call.Line,
								})
								break
							}
						}
					}
				}
			}

			graph.Nodes[nodeID] = node
		}
	}

	// Build CalledBy from edges
	for _, edge := range graph.Edges {
		if node, ok := graph.Nodes[edge.To]; ok {
			node.CalledBy = append(node.CalledBy, edge.From)
		}
	}

	return graph
}

// searchIndexBuilderHelper wraps the analyzer's search index builder
type searchIndexBuilderHelper struct{}

func (h *searchIndexBuilderHelper) BuildFromFiles(files map[string]*ctxpkg.FileContext) *ctxpkg.SearchIndex {
	index := &ctxpkg.SearchIndex{
		FunctionNames: make(map[string][]ctxpkg.FunctionRef),
		Concepts:      make(map[string][]ctxpkg.FunctionRef),
		SideEffects:   make(map[string][]ctxpkg.FunctionRef),
		TypeUsage:     make(map[string][]ctxpkg.FunctionRef),
		ErrorTypes:    make(map[string][]ctxpkg.FunctionRef),
		Routes:        make(map[string][]ctxpkg.RouteRef),
		Packages:      make(map[string][]string),
	}

	for path, fileCtx := range files {
		for _, fn := range fileCtx.Functions {
			ref := ctxpkg.FunctionRef{
				File:      path,
				Function:  fn.Name,
				Line:      fn.LineStart,
				Signature: fn.Signature,
			}
			if fn.Behavior != nil {
				ref.Summary = fn.Behavior.Summary
			}

			// Index function name
			nameLower := strings.ToLower(fn.Name)
			index.FunctionNames[nameLower] = append(index.FunctionNames[nameLower], ref)

			// Index side effects
			for _, effect := range fn.SideEffects {
				index.SideEffects[effect] = append(index.SideEffects[effect], ref)
			}

			// Index concepts from behavior patterns
			if fn.Behavior != nil {
				for _, pattern := range fn.Behavior.Patterns {
					index.Concepts[pattern] = append(index.Concepts[pattern], ref)
				}
			}

			// Index error types
			if fn.ErrorHandling != nil {
				for _, errType := range fn.ErrorHandling.ErrorTypes {
					index.ErrorTypes[errType] = append(index.ErrorTypes[errType], ref)
				}
			}
		}
	}

	return index
}

// populateCalledBy populates CalledBy for all functions
func populateCalledBy(files map[string]*ctxpkg.FileContext) {
	// Build a map of function name to file+index for quick lookup
	funcLocations := make(map[string][]struct {
		file string
		idx  int
	})

	for path, fileCtx := range files {
		for i, fn := range fileCtx.Functions {
			funcLocations[fn.Name] = append(funcLocations[fn.Name], struct {
				file string
				idx  int
			}{path, i})
		}
	}

	// Now populate CalledBy
	for callerPath, callerFile := range files {
		for _, callerFn := range callerFile.Functions {
			for _, call := range callerFn.Calls {
				if call.Type == "internal" {
					// Find the called function and add caller to its CalledBy
					if locs, ok := funcLocations[call.Function]; ok {
						for _, loc := range locs {
							files[loc.file].Functions[loc.idx].CalledBy = append(
								files[loc.file].Functions[loc.idx].CalledBy,
								ctxpkg.CallRef{
									Function: callerFn.Name,
									File:     callerPath,
									Line:     call.Line,
									Type:     "internal",
								},
							)
						}
					}
				}
			}
		}
	}
}

// isSkippableDir returns true if the directory should be skipped.
func isSkippableDir(name string) bool {
	skipDirs := map[string]bool{
		"node_modules":  true,
		"vendor":        true,
		"dist":          true,
		// "build" removed - conflicts with legitimate Go packages like service/build
		"target":        true,
		"__pycache__":   true,
		".git":          true,
		".svn":          true,
		".hg":           true,
		"coverage":      true,
		".idea":         true,
		".vscode":       true,
		"bin":           true,
		"obj":           true,
		".next":         true,
		".nuxt":         true,
		"venv":          true,
		"env":           true,
		".env":          true,
		"tmp":           true,
		"temp":          true,
		"logs":          true,
	}
	return skipDirs[name]
}

// isCodeFile returns true if the file is a code file we can analyze.
func isCodeFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	codeExtensions := map[string]bool{
		".go":      true,
		".js":      true,
		".ts":      true,
		".jsx":     true,
		".tsx":     true,
		".py":      true,
		".java":    true,
		".rs":      true,
		".rb":      true,
		".php":     true,
		".c":       true,
		".cpp":     true,
		".h":       true,
		".hpp":     true,
		".cs":      true,
		".swift":   true,
		".kt":      true,
		".scala":   true,
		".ex":      true,
		".exs":     true,
		".erl":     true,
		".clj":     true,
		".sql":     true,
		".sh":      true,
		".bash":    true,
		".zsh":     true,
		".yaml":    true,
		".yml":     true,
		".json":    true,
		".xml":     true,
		".proto":   true,
		".graphql": true,
		".gql":     true,
	}
	return codeExtensions[ext]
}

// GetOrAnalyzeLocal gets existing context or analyzes if not present.
// This enables on-demand analysis without requiring pre-analysis.
func (m *manager) GetOrAnalyzeLocal(ctx context.Context, dirPath string) (*ctxpkg.RepoContext, error) {
	absPath, err := filepath.Abs(dirPath)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve path: %w", err)
	}

	projectID := "local:" + absPath

	// Try to load existing context
	existingCtx, err := m.store.GetRepoContext(ctx, projectID)
	if err == nil && existingCtx != nil {
		return existingCtx, nil
	}

	// Analyze on demand
	_, err = m.AnalyzeLocal(ctx, dirPath, AnalyzeLocalOptions{
		MaxAge: 24 * time.Hour,
	})
	if err != nil {
		return nil, err
	}

	// Load the newly created context
	return m.store.GetRepoContext(ctx, projectID)
}
