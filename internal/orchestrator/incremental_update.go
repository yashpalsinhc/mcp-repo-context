package orchestrator

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	ctxpkg "github.com/yashpalc/mcp-repo-context/internal/context"
	"github.com/yashpalc/mcp-repo-context/internal/repo"
	"github.com/yashpalc/mcp-repo-context/internal/storage"
)

// RefreshFileOptions configures single file refresh.
type RefreshFileOptions struct {
	Force bool // Force refresh even if hash matches
}

// RefreshFileResult contains the result of refreshing a single file.
type RefreshFileResult struct {
	FilePath    string
	WasStale    bool
	Updated     bool
	NewHash     string
	OldHash     string
	FunctionCount int
	TypeCount   int
}

// RefreshFile re-analyzes a single file and updates the context.
// This is much faster than full re-analysis (~10ms vs ~seconds).
func (m *manager) RefreshFile(ctx context.Context, projectID, filePath string, opts RefreshFileOptions) (*RefreshFileResult, error) {
	// Get existing context
	repoCtx, err := m.store.GetRepoContext(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("project not found: %s", projectID)
	}

	result := &RefreshFileResult{
		FilePath: filePath,
	}

	// Determine absolute path
	var absPath string
	if strings.HasPrefix(projectID, "local:") {
		basePath := strings.TrimPrefix(projectID, "local:")
		absPath = filepath.Join(basePath, filePath)
	} else {
		// For GitHub repos, we'd need the clone path
		// For now, this works with local projects
		return nil, fmt.Errorf("refresh_file currently only supports local projects (local:path)")
	}

	// Read file and compute hash
	content, err := os.ReadFile(absPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	newHash := computeHash(content)
	result.NewHash = newHash

	// Check if file changed
	if existingFile, ok := repoCtx.Files[filePath]; ok {
		result.OldHash = existingFile.Hash
		result.WasStale = existingFile.Hash != newHash

		if !result.WasStale && !opts.Force {
			// File hasn't changed, no update needed
			result.Updated = false
			result.FunctionCount = len(existingFile.Functions)
			result.TypeCount = len(existingFile.Types)
			return result, nil
		}
	} else {
		result.WasStale = true // New file
	}

	// Get analyzer for this file
	analyzer := m.registry.Get(absPath)

	// Create FileInfo
	info, err := os.Stat(absPath)
	if err != nil {
		return nil, fmt.Errorf("failed to stat file: %w", err)
	}

	fileInfo := repo.FileInfo{
		Path:    filePath,
		AbsPath: absPath,
		Size:    info.Size(),
		Hash:    newHash,
	}

	// Analyze the file
	fileCtx, err := analyzer.AnalyzeFile(ctx, fileInfo, content)
	if err != nil {
		return nil, fmt.Errorf("failed to analyze file: %w", err)
	}

	// Update the context
	repoCtx.Files[filePath] = fileCtx
	repoCtx.AnalyzedAt = time.Now()

	// Rebuild call graph edges for this file
	m.updateCallGraphForFile(repoCtx, filePath, fileCtx)

	// Rebuild search index entries for this file
	m.updateSearchIndexForFile(repoCtx, filePath, fileCtx)

	// Repopulate CalledBy relationships
	populateCalledBy(repoCtx.Files)

	// Save updated context
	if err := m.store.StoreRepoContext(ctx, projectID, repoCtx); err != nil {
		return nil, fmt.Errorf("failed to save context: %w", err)
	}

	// Update file hash tracker if supported
	if fileTracker, ok := m.store.(storage.FileTracker); ok {
		if err := fileTracker.UpdateFileHash(ctx, projectID, filePath, newHash, info.Size()); err != nil {
			log.Printf("warning: failed to update file hash tracker: %v", err)
		}
	}

	result.Updated = true
	result.FunctionCount = len(fileCtx.Functions)
	result.TypeCount = len(fileCtx.Types)

	log.Printf("refreshed file %s: %d functions, %d types", filePath, result.FunctionCount, result.TypeCount)

	return result, nil
}

// RefreshChangedFiles checks all files and refreshes only those that changed.
// Returns list of files that were updated.
// Uses the file_hashes table for efficient O(1) change detection per file.
func (m *manager) RefreshChangedFiles(ctx context.Context, projectID string) ([]RefreshFileResult, error) {
	repoCtx, err := m.store.GetRepoContext(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("project not found: %s", projectID)
	}

	if !strings.HasPrefix(projectID, "local:") {
		return nil, fmt.Errorf("refresh_changed currently only supports local projects")
	}

	basePath := strings.TrimPrefix(projectID, "local:")
	var results []RefreshFileResult

	// Try to use file tracker if store supports it
	fileTracker, hasTracker := m.store.(storage.FileTracker)

	// Build map of current file hashes by walking the filesystem
	currentFiles := make(map[string]string)
	var existingPaths []string

	err = filepath.Walk(basePath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		if info.IsDir() {
			name := info.Name()
			if strings.HasPrefix(name, ".") || isSkippableDir(name) {
				return filepath.SkipDir
			}
			return nil
		}

		if !isCodeFile(path) {
			return nil
		}

		relPath, _ := filepath.Rel(basePath, path)
		existingPaths = append(existingPaths, relPath)

		// Read file and compute hash
		content, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		currentFiles[relPath] = computeHash(content)

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to walk directory: %w", err)
	}

	var filesToRefresh []string

	if hasTracker {
		// Use efficient file tracker to detect changes
		log.Printf("using file tracker for incremental indexing")

		changedFiles, err := fileTracker.GetChangedFiles(ctx, projectID, currentFiles)
		if err != nil {
			log.Printf("file tracker error, falling back to context comparison: %v", err)
			// Fall back to context comparison
			filesToRefresh = m.compareWithContext(repoCtx, currentFiles)
		} else {
			filesToRefresh = changedFiles

			// Also check for files in the context but not on disk anymore (deleted)
			// and clean them up from the tracker
			deleted, err := fileTracker.CleanupDeletedFiles(ctx, projectID, existingPaths)
			if err != nil {
				log.Printf("failed to cleanup deleted files: %v", err)
			} else if deleted > 0 {
				log.Printf("cleaned up %d deleted file hashes", deleted)
			}
		}
	} else {
		// Fall back to comparing with context
		log.Printf("file tracker not available, using context comparison")
		filesToRefresh = m.compareWithContext(repoCtx, currentFiles)
	}

	log.Printf("found %d files to refresh", len(filesToRefresh))

	// Refresh changed files
	var updatedHashes []storage.FileHashInfo
	for _, filePath := range filesToRefresh {
		result, err := m.RefreshFile(ctx, projectID, filePath, RefreshFileOptions{})
		if err != nil {
			log.Printf("failed to refresh %s: %v", filePath, err)
			continue
		}
		results = append(results, *result)

		// Track hash for batch update
		if result.Updated && hasTracker {
			absPath := filepath.Join(basePath, filePath)
			info, _ := os.Stat(absPath)
			var fileSize int64
			if info != nil {
				fileSize = info.Size()
			}
			updatedHashes = append(updatedHashes, storage.FileHashInfo{
				FilePath:     filePath,
				Hash:         result.NewHash,
				FileSize:     fileSize,
				LastAnalyzed: time.Now(),
			})
		}
	}

	// Batch update file hashes if tracker is available
	if hasTracker && len(updatedHashes) > 0 {
		if err := fileTracker.UpdateFileHashes(ctx, projectID, updatedHashes); err != nil {
			log.Printf("failed to batch update file hashes: %v", err)
		}
	}

	return results, nil
}

// compareWithContext compares current files with stored context (fallback method).
func (m *manager) compareWithContext(repoCtx *ctxpkg.RepoContext, currentFiles map[string]string) []string {
	var filesToRefresh []string

	// Check existing files for changes
	for filePath, currentHash := range currentFiles {
		if fileCtx, exists := repoCtx.Files[filePath]; exists {
			if fileCtx.Hash != currentHash {
				filesToRefresh = append(filesToRefresh, filePath)
			}
		} else {
			// New file
			filesToRefresh = append(filesToRefresh, filePath)
		}
	}

	return filesToRefresh
}

// CheckFileStale checks if a file's context is stale without refreshing.
func (m *manager) CheckFileStale(ctx context.Context, projectID, filePath string) (bool, error) {
	repoCtx, err := m.store.GetRepoContext(ctx, projectID)
	if err != nil {
		return false, fmt.Errorf("project not found: %s", projectID)
	}

	if !strings.HasPrefix(projectID, "local:") {
		return false, nil // Can't check for non-local projects
	}

	basePath := strings.TrimPrefix(projectID, "local:")
	absPath := filepath.Join(basePath, filePath)

	content, err := os.ReadFile(absPath)
	if err != nil {
		return true, nil // File doesn't exist or can't read = stale
	}

	newHash := computeHash(content)

	if existingFile, ok := repoCtx.Files[filePath]; ok {
		return existingFile.Hash != newHash, nil
	}

	return true, nil // File not in context = stale
}

// updateCallGraphForFile updates call graph entries for a single file.
func (m *manager) updateCallGraphForFile(repoCtx *ctxpkg.RepoContext, filePath string, fileCtx *ctxpkg.FileContext) {
	if repoCtx.CallGraph == nil {
		repoCtx.CallGraph = &ctxpkg.CallGraph{
			Nodes: make(map[string]*ctxpkg.CallGraphNode),
			Edges: []ctxpkg.CallGraphEdge{},
		}
	}

	// Remove old nodes for this file
	for nodeID := range repoCtx.CallGraph.Nodes {
		if strings.HasPrefix(nodeID, filePath+":") {
			delete(repoCtx.CallGraph.Nodes, nodeID)
		}
	}

	// Remove old edges from this file
	var newEdges []ctxpkg.CallGraphEdge
	for _, edge := range repoCtx.CallGraph.Edges {
		if !strings.HasPrefix(edge.From, filePath+":") {
			newEdges = append(newEdges, edge)
		}
	}
	repoCtx.CallGraph.Edges = newEdges

	// Add new nodes and edges
	for _, fn := range fileCtx.Functions {
		nodeID := filePath + ":" + fn.Name
		node := &ctxpkg.CallGraphNode{
			ID:        nodeID,
			File:      filePath,
			Function:  fn.Name,
			Signature: fn.Signature,
			IsPublic:  fn.IsPublic,
			Calls:     []string{},
			CalledBy:  []string{},
		}

		for _, call := range fn.Calls {
			if call.Type == "internal" {
				// Try to resolve to a node ID
				for otherPath, otherFileCtx := range repoCtx.Files {
					for _, otherFn := range otherFileCtx.Functions {
						if otherFn.Name == call.Function {
							targetID := otherPath + ":" + otherFn.Name
							node.Calls = append(node.Calls, targetID)
							repoCtx.CallGraph.Edges = append(repoCtx.CallGraph.Edges, ctxpkg.CallGraphEdge{
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

		repoCtx.CallGraph.Nodes[nodeID] = node
	}
}

// updateSearchIndexForFile updates search index entries for a single file.
func (m *manager) updateSearchIndexForFile(repoCtx *ctxpkg.RepoContext, filePath string, fileCtx *ctxpkg.FileContext) {
	if repoCtx.SearchIndex == nil {
		repoCtx.SearchIndex = &ctxpkg.SearchIndex{
			FunctionNames: make(map[string][]ctxpkg.FunctionRef),
			Concepts:      make(map[string][]ctxpkg.FunctionRef),
			SideEffects:   make(map[string][]ctxpkg.FunctionRef),
			TypeUsage:     make(map[string][]ctxpkg.FunctionRef),
			ErrorTypes:    make(map[string][]ctxpkg.FunctionRef),
			Routes:        make(map[string][]ctxpkg.RouteRef),
			Packages:      make(map[string][]string),
		}
	}

	// Remove old entries for this file from all indexes
	removeFileFromIndex := func(index map[string][]ctxpkg.FunctionRef) {
		for key, refs := range index {
			var newRefs []ctxpkg.FunctionRef
			for _, ref := range refs {
				if ref.File != filePath {
					newRefs = append(newRefs, ref)
				}
			}
			if len(newRefs) > 0 {
				index[key] = newRefs
			} else {
				delete(index, key)
			}
		}
	}

	removeFileFromIndex(repoCtx.SearchIndex.FunctionNames)
	removeFileFromIndex(repoCtx.SearchIndex.Concepts)
	removeFileFromIndex(repoCtx.SearchIndex.SideEffects)
	removeFileFromIndex(repoCtx.SearchIndex.TypeUsage)
	removeFileFromIndex(repoCtx.SearchIndex.ErrorTypes)

	// Add new entries
	for _, fn := range fileCtx.Functions {
		ref := ctxpkg.FunctionRef{
			File:      filePath,
			Function:  fn.Name,
			Line:      fn.LineStart,
			Signature: fn.Signature,
		}
		if fn.Behavior != nil {
			ref.Summary = fn.Behavior.Summary
		}

		// Index function name
		nameLower := strings.ToLower(fn.Name)
		repoCtx.SearchIndex.FunctionNames[nameLower] = append(repoCtx.SearchIndex.FunctionNames[nameLower], ref)

		// Index side effects
		for _, effect := range fn.SideEffects {
			repoCtx.SearchIndex.SideEffects[effect] = append(repoCtx.SearchIndex.SideEffects[effect], ref)
		}

		// Index concepts
		if fn.Behavior != nil {
			for _, pattern := range fn.Behavior.Patterns {
				repoCtx.SearchIndex.Concepts[pattern] = append(repoCtx.SearchIndex.Concepts[pattern], ref)
			}
		}

		// Index error types
		if fn.ErrorHandling != nil {
			for _, errType := range fn.ErrorHandling.ErrorTypes {
				repoCtx.SearchIndex.ErrorTypes[errType] = append(repoCtx.SearchIndex.ErrorTypes[errType], ref)
			}
		}
	}
}

// computeHash computes SHA256 hash of content.
func computeHash(content []byte) string {
	hash := sha256.Sum256(content)
	return hex.EncodeToString(hash[:])
}
