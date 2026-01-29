package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	ctxpkg "github.com/yashpalc/mcp-repo-context/internal/context"
)

// Ensure filesystemStore implements ContextStore interface.
var _ ContextStore = (*filesystemStore)(nil)

// filesystemStore stores context as JSON files on local filesystem (private struct).
type filesystemStore struct {
	basePath string
}

// NewFilesystemStore creates a new filesystem-based store.
func NewFilesystemStore(basePath string) (ContextStore, error) {
	if err := os.MkdirAll(basePath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create storage directory: %w", err)
	}
	return &filesystemStore{basePath: basePath}, nil
}

func (s *filesystemStore) repoPath(repoID string) string {
	// Sanitize repo ID to be filesystem-safe
	safe := strings.ReplaceAll(repoID, "/", "_")
	safe = strings.ReplaceAll(safe, ":", "_")
	return filepath.Join(s.basePath, safe+".json")
}

// StoreRepoContext persists complete repository context.
func (s *filesystemStore) StoreRepoContext(ctx context.Context, repoID string, repoCtx *ctxpkg.RepoContext) error {
	data, err := json.MarshalIndent(repoCtx, "", "  ")
	if err != nil {
		return fmt.Errorf("%w: %v", ErrStoreFailed, err)
	}

	path := s.repoPath(repoID)
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("%w: %v", ErrStoreFailed, err)
	}

	return nil
}

// GetRepoContext retrieves full repository context.
func (s *filesystemStore) GetRepoContext(ctx context.Context, repoID string) (*ctxpkg.RepoContext, error) {
	path := s.repoPath(repoID)

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed to read context: %w", err)
	}

	var repoCtx ctxpkg.RepoContext
	if err := json.Unmarshal(data, &repoCtx); err != nil {
		return nil, fmt.Errorf("failed to parse context: %w", err)
	}

	return &repoCtx, nil
}

// GetFileContext retrieves context for a specific file.
func (s *filesystemStore) GetFileContext(ctx context.Context, repoID, filePath string) (*ctxpkg.FileContext, error) {
	repoCtx, err := s.GetRepoContext(ctx, repoID)
	if err != nil {
		return nil, err
	}

	fileCtx, ok := repoCtx.Files[filePath]
	if !ok {
		return nil, ErrNotFound
	}

	return fileCtx, nil
}

// ContextExists checks if context exists and is within maxAge.
func (s *filesystemStore) ContextExists(ctx context.Context, repoID string, maxAge time.Duration) (bool, time.Time, error) {
	path := s.repoPath(repoID)

	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, time.Time{}, nil
		}
		return false, time.Time{}, err
	}

	modTime := info.ModTime()
	if maxAge > 0 && time.Since(modTime) > maxAge {
		return false, modTime, nil
	}

	return true, modTime, nil
}

// DeleteContext removes all context for a repository.
func (s *filesystemStore) DeleteContext(ctx context.Context, repoID string) error {
	path := s.repoPath(repoID)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete context: %w", err)
	}
	return nil
}

// ListContexts returns metadata for all stored contexts.
func (s *filesystemStore) ListContexts(ctx context.Context) ([]ctxpkg.ContextMetadata, error) {
	entries, err := os.ReadDir(s.basePath)
	if err != nil {
		return nil, fmt.Errorf("failed to list contexts: %w", err)
	}

	var results []ctxpkg.ContextMetadata
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		// Extract repo ID from filename
		repoID := strings.TrimSuffix(entry.Name(), ".json")

		repoCtx, err := s.GetRepoContext(ctx, repoID)
		if err != nil {
			continue // Skip invalid files
		}

		results = append(results, ctxpkg.ContextMetadata{
			RepoID:     repoCtx.ID,
			URL:        repoCtx.URL,
			Branch:     repoCtx.Branch,
			CommitHash: repoCtx.CommitHash,
			FileCount:  len(repoCtx.Files),
			AnalyzedAt: repoCtx.AnalyzedAt,
		})
	}

	return results, nil
}
