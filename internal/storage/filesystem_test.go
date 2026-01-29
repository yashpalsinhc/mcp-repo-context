package storage

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	ctxpkg "github.com/yashpalc/mcp-repo-context/internal/context"
)

func TestNewFilesystemStore(t *testing.T) {
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "contexts")

	store, err := NewFilesystemStore(storePath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if store == nil {
		t.Fatal("expected non-nil store")
	}

	// Verify directory was created
	if _, err := os.Stat(storePath); os.IsNotExist(err) {
		t.Error("expected storage directory to be created")
	}
}

func TestFilesystemStore_StoreAndGet(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewFilesystemStore(tmpDir)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	ctx := context.Background()
	repoID := "github.com/example/repo"

	repoCtx := &ctxpkg.RepoContext{
		ID:         repoID,
		URL:        "https://github.com/example/repo",
		Branch:     "main",
		CommitHash: "abc123",
		AnalyzedAt: time.Now(),
		Files: map[string]*ctxpkg.FileContext{
			"main.go": {
				Path:      "main.go",
				Language:  "go",
				LineCount: 100,
			},
		},
		Statistics: ctxpkg.RepoStatistics{
			TotalFiles:        1,
			TotalLines:        100,
			LanguageBreakdown: map[string]int{"go": 1},
		},
		Version: 1,
	}

	// Store
	if err := store.StoreRepoContext(ctx, repoID, repoCtx); err != nil {
		t.Fatalf("failed to store: %v", err)
	}

	// Retrieve
	retrieved, err := store.GetRepoContext(ctx, repoID)
	if err != nil {
		t.Fatalf("failed to get: %v", err)
	}

	// Verify
	if retrieved.ID != repoID {
		t.Errorf("expected ID %s, got %s", repoID, retrieved.ID)
	}
	if retrieved.Branch != "main" {
		t.Errorf("expected branch main, got %s", retrieved.Branch)
	}
	if len(retrieved.Files) != 1 {
		t.Errorf("expected 1 file, got %d", len(retrieved.Files))
	}
}

func TestFilesystemStore_GetNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewFilesystemStore(tmpDir)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	ctx := context.Background()
	_, err = store.GetRepoContext(ctx, "nonexistent")
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestFilesystemStore_GetFileContext(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewFilesystemStore(tmpDir)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	ctx := context.Background()
	repoID := "github.com/example/repo"

	repoCtx := &ctxpkg.RepoContext{
		ID: repoID,
		Files: map[string]*ctxpkg.FileContext{
			"main.go": {
				Path:      "main.go",
				Language:  "go",
				LineCount: 50,
			},
			"util.go": {
				Path:      "util.go",
				Language:  "go",
				LineCount: 30,
			},
		},
	}

	if err := store.StoreRepoContext(ctx, repoID, repoCtx); err != nil {
		t.Fatalf("failed to store: %v", err)
	}

	// Get specific file
	fileCtx, err := store.GetFileContext(ctx, repoID, "main.go")
	if err != nil {
		t.Fatalf("failed to get file context: %v", err)
	}

	if fileCtx.Path != "main.go" {
		t.Errorf("expected path main.go, got %s", fileCtx.Path)
	}
	if fileCtx.LineCount != 50 {
		t.Errorf("expected 50 lines, got %d", fileCtx.LineCount)
	}

	// Get nonexistent file
	_, err = store.GetFileContext(ctx, repoID, "nonexistent.go")
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestFilesystemStore_ContextExists(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewFilesystemStore(tmpDir)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	ctx := context.Background()
	repoID := "github.com/example/repo"

	// Should not exist
	exists, _, err := store.ContextExists(ctx, repoID, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exists {
		t.Error("expected context to not exist")
	}

	// Store context
	repoCtx := &ctxpkg.RepoContext{ID: repoID}
	if err := store.StoreRepoContext(ctx, repoID, repoCtx); err != nil {
		t.Fatalf("failed to store: %v", err)
	}

	// Should exist
	exists, modTime, err := store.ContextExists(ctx, repoID, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !exists {
		t.Error("expected context to exist")
	}
	if modTime.IsZero() {
		t.Error("expected non-zero mod time")
	}

	// Test max age
	exists, _, err = store.ContextExists(ctx, repoID, time.Hour)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !exists {
		t.Error("expected context to exist within max age")
	}

	// Very short max age should make it "expired"
	exists, _, err = store.ContextExists(ctx, repoID, time.Nanosecond)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exists {
		t.Error("expected context to be expired")
	}
}

func TestFilesystemStore_DeleteContext(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewFilesystemStore(tmpDir)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	ctx := context.Background()
	repoID := "github.com/example/repo"

	// Store context
	repoCtx := &ctxpkg.RepoContext{ID: repoID}
	if err := store.StoreRepoContext(ctx, repoID, repoCtx); err != nil {
		t.Fatalf("failed to store: %v", err)
	}

	// Delete
	if err := store.DeleteContext(ctx, repoID); err != nil {
		t.Fatalf("failed to delete: %v", err)
	}

	// Should not exist
	_, err = store.GetRepoContext(ctx, repoID)
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound after delete, got %v", err)
	}

	// Delete nonexistent should not error
	if err := store.DeleteContext(ctx, "nonexistent"); err != nil {
		t.Errorf("delete nonexistent should not error, got %v", err)
	}
}

func TestFilesystemStore_ListContexts(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewFilesystemStore(tmpDir)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	ctx := context.Background()

	// Initially empty
	list, err := store.ListContexts(ctx)
	if err != nil {
		t.Fatalf("failed to list: %v", err)
	}
	if len(list) != 0 {
		t.Errorf("expected empty list, got %d items", len(list))
	}

	// Add some contexts
	repos := []string{
		"github.com/example/repo1",
		"github.com/example/repo2",
		"github.com/example/repo3",
	}

	for i, repoID := range repos {
		repoCtx := &ctxpkg.RepoContext{
			ID:         repoID,
			URL:        "https://" + repoID,
			Branch:     "main",
			CommitHash: "hash" + string(rune('a'+i)),
			AnalyzedAt: time.Now(),
			Files: map[string]*ctxpkg.FileContext{
				"main.go": {},
			},
		}
		if err := store.StoreRepoContext(ctx, repoID, repoCtx); err != nil {
			t.Fatalf("failed to store %s: %v", repoID, err)
		}
	}

	// List should have 3 items
	list, err = store.ListContexts(ctx)
	if err != nil {
		t.Fatalf("failed to list: %v", err)
	}
	if len(list) != 3 {
		t.Errorf("expected 3 items, got %d", len(list))
	}

	// Verify each item
	foundIDs := make(map[string]bool)
	for _, meta := range list {
		foundIDs[meta.RepoID] = true
		if meta.FileCount != 1 {
			t.Errorf("expected 1 file for %s, got %d", meta.RepoID, meta.FileCount)
		}
	}
	for _, repoID := range repos {
		if !foundIDs[repoID] {
			t.Errorf("missing repo %s in list", repoID)
		}
	}
}

func TestFilesystemStore_RepoIDSanitization(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewFilesystemStore(tmpDir)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	ctx := context.Background()

	// Test repo IDs with special characters
	repoIDs := []string{
		"github.com/user/repo",
		"gitlab.com/group/subgroup/repo",
		"bitbucket.org/user/repo",
	}

	for _, repoID := range repoIDs {
		repoCtx := &ctxpkg.RepoContext{ID: repoID}
		if err := store.StoreRepoContext(ctx, repoID, repoCtx); err != nil {
			t.Errorf("failed to store %s: %v", repoID, err)
			continue
		}

		retrieved, err := store.GetRepoContext(ctx, repoID)
		if err != nil {
			t.Errorf("failed to get %s: %v", repoID, err)
			continue
		}

		if retrieved.ID != repoID {
			t.Errorf("expected ID %s, got %s", repoID, retrieved.ID)
		}
	}
}
