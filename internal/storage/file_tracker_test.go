package storage

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFileTracker_UpdateAndGetFileHash(t *testing.T) {
	store, cleanup := createTestSQLiteStore(t)
	defer cleanup()

	ctx := context.Background()
	repoID := "github.com/test/repo"

	// Store test repo first (file_hashes has FK to repos)
	repoCtx := createTestRepoContext()
	repoCtx.ID = repoID
	if err := store.StoreRepoContext(ctx, repoID, repoCtx); err != nil {
		t.Fatalf("Failed to store repo context: %v", err)
	}

	// Test UpdateFileHash
	err := store.UpdateFileHash(ctx, repoID, "pkg/main.go", "hash123", 1000)
	if err != nil {
		t.Fatalf("UpdateFileHash failed: %v", err)
	}

	// Test GetFileHash
	info, err := store.GetFileHash(ctx, repoID, "pkg/main.go")
	if err != nil {
		t.Fatalf("GetFileHash failed: %v", err)
	}
	if info == nil {
		t.Fatal("GetFileHash returned nil")
	}
	if info.Hash != "hash123" {
		t.Errorf("Hash mismatch: got %s, want hash123", info.Hash)
	}
	if info.FileSize != 1000 {
		t.Errorf("FileSize mismatch: got %d, want 1000", info.FileSize)
	}
	if info.FilePath != "pkg/main.go" {
		t.Errorf("FilePath mismatch: got %s, want pkg/main.go", info.FilePath)
	}
}

func TestFileTracker_GetFileHash_NotFound(t *testing.T) {
	store, cleanup := createTestSQLiteStore(t)
	defer cleanup()

	ctx := context.Background()

	// Should return nil, nil for non-existent file
	info, err := store.GetFileHash(ctx, "github.com/test/nonexistent", "nonexistent.go")
	if err != nil {
		t.Fatalf("GetFileHash should not return error for missing file: %v", err)
	}
	if info != nil {
		t.Error("GetFileHash should return nil for missing file")
	}
}

func TestFileTracker_UpdateFileHash_Upsert(t *testing.T) {
	store, cleanup := createTestSQLiteStore(t)
	defer cleanup()

	ctx := context.Background()
	repoID := "github.com/test/repo"

	// Store test repo first
	repoCtx := createTestRepoContext()
	repoCtx.ID = repoID
	if err := store.StoreRepoContext(ctx, repoID, repoCtx); err != nil {
		t.Fatalf("Failed to store repo context: %v", err)
	}

	// Insert
	err := store.UpdateFileHash(ctx, repoID, "pkg/main.go", "hash1", 100)
	if err != nil {
		t.Fatalf("First UpdateFileHash failed: %v", err)
	}

	// Update (same key, different hash)
	err = store.UpdateFileHash(ctx, repoID, "pkg/main.go", "hash2", 200)
	if err != nil {
		t.Fatalf("Second UpdateFileHash failed: %v", err)
	}

	// Verify update
	info, err := store.GetFileHash(ctx, repoID, "pkg/main.go")
	if err != nil {
		t.Fatalf("GetFileHash failed: %v", err)
	}
	if info.Hash != "hash2" {
		t.Errorf("Hash not updated: got %s, want hash2", info.Hash)
	}
	if info.FileSize != 200 {
		t.Errorf("FileSize not updated: got %d, want 200", info.FileSize)
	}
}

func TestFileTracker_GetFileHashes(t *testing.T) {
	store, cleanup := createTestSQLiteStore(t)
	defer cleanup()

	ctx := context.Background()
	repoID := "github.com/test/repo"

	// Store test repo first
	repoCtx := createTestRepoContext()
	repoCtx.ID = repoID
	if err := store.StoreRepoContext(ctx, repoID, repoCtx); err != nil {
		t.Fatalf("Failed to store repo context: %v", err)
	}

	// Insert multiple files
	files := []struct {
		path string
		hash string
		size int64
	}{
		{"pkg/main.go", "hash1", 100},
		{"pkg/handler.go", "hash2", 200},
		{"internal/util.go", "hash3", 300},
	}

	for _, f := range files {
		if err := store.UpdateFileHash(ctx, repoID, f.path, f.hash, f.size); err != nil {
			t.Fatalf("UpdateFileHash failed for %s: %v", f.path, err)
		}
	}

	// Get all hashes
	hashes, err := store.GetFileHashes(ctx, repoID)
	if err != nil {
		t.Fatalf("GetFileHashes failed: %v", err)
	}

	if len(hashes) != 3 {
		t.Errorf("Expected 3 hashes, got %d", len(hashes))
	}

	// Verify each file
	for _, f := range files {
		info, exists := hashes[f.path]
		if !exists {
			t.Errorf("File %s not found in hashes", f.path)
			continue
		}
		if info.Hash != f.hash {
			t.Errorf("Hash mismatch for %s: got %s, want %s", f.path, info.Hash, f.hash)
		}
	}
}

func TestFileTracker_GetChangedFiles(t *testing.T) {
	store, cleanup := createTestSQLiteStore(t)
	defer cleanup()

	ctx := context.Background()
	repoID := "github.com/test/repo"

	// Store test repo first
	repoCtx := createTestRepoContext()
	repoCtx.ID = repoID
	if err := store.StoreRepoContext(ctx, repoID, repoCtx); err != nil {
		t.Fatalf("Failed to store repo context: %v", err)
	}

	// Store some file hashes
	storedFiles := []struct {
		path string
		hash string
	}{
		{"file1.go", "hash1"},
		{"file2.go", "hash2"},
		{"file3.go", "hash3"},
	}
	for _, f := range storedFiles {
		if err := store.UpdateFileHash(ctx, repoID, f.path, f.hash, 100); err != nil {
			t.Fatalf("UpdateFileHash failed: %v", err)
		}
	}

	// Current files on "disk" - simulating changes
	currentFiles := map[string]string{
		"file1.go":  "hash1",    // unchanged
		"file2.go":  "newhash2", // modified
		"newfile.go": "hash4",   // new file
	}
	// Note: file3.go is deleted (not in current files)

	// Get changed files
	changed, err := store.GetChangedFiles(ctx, repoID, currentFiles)
	if err != nil {
		t.Fatalf("GetChangedFiles failed: %v", err)
	}

	// Should report file2.go (modified) and newfile.go (new)
	expectedChanged := map[string]bool{
		"file2.go":  true,
		"newfile.go": true,
	}

	if len(changed) != len(expectedChanged) {
		t.Errorf("Expected %d changed files, got %d: %v", len(expectedChanged), len(changed), changed)
	}

	for _, path := range changed {
		if !expectedChanged[path] {
			t.Errorf("Unexpected changed file: %s", path)
		}
	}
}

func TestFileTracker_UpdateFileHashes_Batch(t *testing.T) {
	store, cleanup := createTestSQLiteStore(t)
	defer cleanup()

	ctx := context.Background()
	repoID := "github.com/test/repo"

	// Store test repo first
	repoCtx := createTestRepoContext()
	repoCtx.ID = repoID
	if err := store.StoreRepoContext(ctx, repoID, repoCtx); err != nil {
		t.Fatalf("Failed to store repo context: %v", err)
	}

	// Batch update
	files := []FileHashInfo{
		{FilePath: "file1.go", Hash: "hash1", FileSize: 100},
		{FilePath: "file2.go", Hash: "hash2", FileSize: 200},
		{FilePath: "file3.go", Hash: "hash3", FileSize: 300},
	}

	err := store.UpdateFileHashes(ctx, repoID, files)
	if err != nil {
		t.Fatalf("UpdateFileHashes failed: %v", err)
	}

	// Verify
	hashes, err := store.GetFileHashes(ctx, repoID)
	if err != nil {
		t.Fatalf("GetFileHashes failed: %v", err)
	}

	if len(hashes) != 3 {
		t.Errorf("Expected 3 hashes, got %d", len(hashes))
	}

	for _, f := range files {
		info, exists := hashes[f.FilePath]
		if !exists {
			t.Errorf("File %s not found", f.FilePath)
			continue
		}
		if info.Hash != f.Hash {
			t.Errorf("Hash mismatch for %s: got %s, want %s", f.FilePath, info.Hash, f.Hash)
		}
	}
}

func TestFileTracker_CleanupDeletedFiles(t *testing.T) {
	store, cleanup := createTestSQLiteStore(t)
	defer cleanup()

	ctx := context.Background()
	repoID := "github.com/test/repo"

	// Store test repo first
	repoCtx := createTestRepoContext()
	repoCtx.ID = repoID
	if err := store.StoreRepoContext(ctx, repoID, repoCtx); err != nil {
		t.Fatalf("Failed to store repo context: %v", err)
	}

	// Store file hashes
	files := []FileHashInfo{
		{FilePath: "keep1.go", Hash: "hash1", FileSize: 100},
		{FilePath: "keep2.go", Hash: "hash2", FileSize: 200},
		{FilePath: "delete1.go", Hash: "hash3", FileSize: 300},
		{FilePath: "delete2.go", Hash: "hash4", FileSize: 400},
	}
	if err := store.UpdateFileHashes(ctx, repoID, files); err != nil {
		t.Fatalf("UpdateFileHashes failed: %v", err)
	}

	// Cleanup - only keep1.go and keep2.go still exist
	existingPaths := []string{"keep1.go", "keep2.go"}
	deleted, err := store.CleanupDeletedFiles(ctx, repoID, existingPaths)
	if err != nil {
		t.Fatalf("CleanupDeletedFiles failed: %v", err)
	}

	if deleted != 2 {
		t.Errorf("Expected 2 deleted entries, got %d", deleted)
	}

	// Verify remaining files
	hashes, err := store.GetFileHashes(ctx, repoID)
	if err != nil {
		t.Fatalf("GetFileHashes failed: %v", err)
	}

	if len(hashes) != 2 {
		t.Errorf("Expected 2 remaining hashes, got %d", len(hashes))
	}

	if _, exists := hashes["delete1.go"]; exists {
		t.Error("delete1.go should have been removed")
	}
	if _, exists := hashes["delete2.go"]; exists {
		t.Error("delete2.go should have been removed")
	}
}

func TestFileTracker_GetStaleFiles(t *testing.T) {
	store, cleanup := createTestSQLiteStore(t)
	defer cleanup()

	ctx := context.Background()
	repoID := "github.com/test/repo"

	// Store test repo first
	repoCtx := createTestRepoContext()
	repoCtx.ID = repoID
	if err := store.StoreRepoContext(ctx, repoID, repoCtx); err != nil {
		t.Fatalf("Failed to store repo context: %v", err)
	}

	// Store files with different timestamps
	now := time.Now()
	oldTime := now.Add(-24 * time.Hour) // 24 hours ago
	veryOldTime := now.Add(-48 * time.Hour) // 48 hours ago

	files := []FileHashInfo{
		{FilePath: "recent.go", Hash: "hash1", FileSize: 100, LastAnalyzed: now},
		{FilePath: "old.go", Hash: "hash2", FileSize: 200, LastAnalyzed: oldTime},
		{FilePath: "veryold.go", Hash: "hash3", FileSize: 300, LastAnalyzed: veryOldTime},
	}
	if err := store.UpdateFileHashes(ctx, repoID, files); err != nil {
		t.Fatalf("UpdateFileHashes failed: %v", err)
	}

	// Get files older than 12 hours
	threshold := now.Add(-12 * time.Hour)
	staleFiles, err := store.GetStaleFiles(ctx, repoID, threshold)
	if err != nil {
		t.Fatalf("GetStaleFiles failed: %v", err)
	}

	// Should find old.go and veryold.go
	if len(staleFiles) != 2 {
		t.Errorf("Expected 2 stale files, got %d: %v", len(staleFiles), staleFiles)
	}

	staleSet := make(map[string]bool)
	for _, f := range staleFiles {
		staleSet[f] = true
	}

	if !staleSet["old.go"] {
		t.Error("old.go should be stale")
	}
	if !staleSet["veryold.go"] {
		t.Error("veryold.go should be stale")
	}
	if staleSet["recent.go"] {
		t.Error("recent.go should not be stale")
	}
}

func TestFileTracker_DeleteRepoHashes(t *testing.T) {
	store, cleanup := createTestSQLiteStore(t)
	defer cleanup()

	ctx := context.Background()
	repoID := "github.com/test/repo"

	// Store test repo first
	repoCtx := createTestRepoContext()
	repoCtx.ID = repoID
	if err := store.StoreRepoContext(ctx, repoID, repoCtx); err != nil {
		t.Fatalf("Failed to store repo context: %v", err)
	}

	// Store file hashes
	files := []FileHashInfo{
		{FilePath: "file1.go", Hash: "hash1", FileSize: 100},
		{FilePath: "file2.go", Hash: "hash2", FileSize: 200},
	}
	if err := store.UpdateFileHashes(ctx, repoID, files); err != nil {
		t.Fatalf("UpdateFileHashes failed: %v", err)
	}

	// Verify files exist
	hashes, err := store.GetFileHashes(ctx, repoID)
	if err != nil {
		t.Fatalf("GetFileHashes failed: %v", err)
	}
	if len(hashes) != 2 {
		t.Fatalf("Expected 2 hashes before delete, got %d", len(hashes))
	}

	// Delete all hashes for repo
	if err := store.DeleteRepoHashes(ctx, repoID); err != nil {
		t.Fatalf("DeleteRepoHashes failed: %v", err)
	}

	// Verify deletion
	hashes, err = store.GetFileHashes(ctx, repoID)
	if err != nil {
		t.Fatalf("GetFileHashes after delete failed: %v", err)
	}
	if len(hashes) != 0 {
		t.Errorf("Expected 0 hashes after delete, got %d", len(hashes))
	}
}

func TestFileTracker_CompareAndGetStats(t *testing.T) {
	store, cleanup := createTestSQLiteStore(t)
	defer cleanup()

	ctx := context.Background()
	repoID := "github.com/test/repo"

	// Store test repo first
	repoCtx := createTestRepoContext()
	repoCtx.ID = repoID
	if err := store.StoreRepoContext(ctx, repoID, repoCtx); err != nil {
		t.Fatalf("Failed to store repo context: %v", err)
	}

	// Store file hashes
	storedFiles := []FileHashInfo{
		{FilePath: "unchanged.go", Hash: "hash1", FileSize: 100},
		{FilePath: "modified.go", Hash: "hash2", FileSize: 200},
		{FilePath: "deleted.go", Hash: "hash3", FileSize: 300},
	}
	if err := store.UpdateFileHashes(ctx, repoID, storedFiles); err != nil {
		t.Fatalf("UpdateFileHashes failed: %v", err)
	}

	// Current files
	currentFiles := map[string]string{
		"unchanged.go": "hash1",        // same hash
		"modified.go":  "newhash",      // different hash
		"newfile.go":   "hash4",        // new file
	}
	// deleted.go is not in current files

	stats, err := store.CompareAndGetStats(ctx, repoID, currentFiles)
	if err != nil {
		t.Fatalf("CompareAndGetStats failed: %v", err)
	}

	if stats.TotalTracked != 3 {
		t.Errorf("TotalTracked: got %d, want 3", stats.TotalTracked)
	}
	if stats.NewFiles != 1 {
		t.Errorf("NewFiles: got %d, want 1", stats.NewFiles)
	}
	if stats.ModifiedFiles != 1 {
		t.Errorf("ModifiedFiles: got %d, want 1", stats.ModifiedFiles)
	}
	if stats.DeletedFiles != 1 {
		t.Errorf("DeletedFiles: got %d, want 1", stats.DeletedFiles)
	}
	if stats.UnchangedFiles != 1 {
		t.Errorf("UnchangedFiles: got %d, want 1", stats.UnchangedFiles)
	}
}

func TestFileTracker_SyncFileHashesFromFiles(t *testing.T) {
	store, cleanup := createTestSQLiteStore(t)
	defer cleanup()

	ctx := context.Background()
	repoCtx := createTestRepoContext()

	// Store repo context (this populates the files table)
	if err := store.StoreRepoContext(ctx, repoCtx.ID, repoCtx); err != nil {
		t.Fatalf("Failed to store repo context: %v", err)
	}

	// Sync file hashes from files table
	if err := store.SyncFileHashesFromFiles(ctx, repoCtx.ID); err != nil {
		t.Fatalf("SyncFileHashesFromFiles failed: %v", err)
	}

	// Verify hashes were synced
	hashes, err := store.GetFileHashes(ctx, repoCtx.ID)
	if err != nil {
		t.Fatalf("GetFileHashes failed: %v", err)
	}

	// Should have hashes for all files in context
	expectedFiles := len(repoCtx.Files)
	if len(hashes) != expectedFiles {
		t.Errorf("Expected %d file hashes, got %d", expectedFiles, len(hashes))
	}

	// Verify hash values match
	for path, fileCtx := range repoCtx.Files {
		info, exists := hashes[path]
		if !exists {
			t.Errorf("Hash not found for %s", path)
			continue
		}
		if info.Hash != fileCtx.Hash {
			t.Errorf("Hash mismatch for %s: got %s, want %s", path, info.Hash, fileCtx.Hash)
		}
	}
}

func TestFileTracker_MigrationIdempotent(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "sqlite-migration-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "test.db")

	// Create store first time (runs migrations)
	store1, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("Failed to create first store: %v", err)
	}
	store1.Close()

	// Create store second time (should be idempotent)
	store2, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("Failed to create second store (migration should be idempotent): %v", err)
	}
	defer store2.Close()

	// Verify table exists and can be used
	ctx := context.Background()
	repoCtx := createTestRepoContext()
	if err := store2.StoreRepoContext(ctx, repoCtx.ID, repoCtx); err != nil {
		t.Fatalf("Failed to store repo context after re-open: %v", err)
	}

	err = store2.UpdateFileHash(ctx, repoCtx.ID, "test.go", "hash123", 100)
	if err != nil {
		t.Fatalf("UpdateFileHash failed after re-open: %v", err)
	}
}

func TestFileTracker_EmptyInputs(t *testing.T) {
	store, cleanup := createTestSQLiteStore(t)
	defer cleanup()

	ctx := context.Background()
	repoID := "github.com/test/repo"

	// Store test repo first
	repoCtx := createTestRepoContext()
	repoCtx.ID = repoID
	if err := store.StoreRepoContext(ctx, repoID, repoCtx); err != nil {
		t.Fatalf("Failed to store repo context: %v", err)
	}

	// GetChangedFiles with empty map
	changed, err := store.GetChangedFiles(ctx, repoID, map[string]string{})
	if err != nil {
		t.Fatalf("GetChangedFiles with empty map failed: %v", err)
	}
	if changed != nil && len(changed) != 0 {
		t.Errorf("Expected empty result, got %v", changed)
	}

	// UpdateFileHashes with empty slice
	err = store.UpdateFileHashes(ctx, repoID, []FileHashInfo{})
	if err != nil {
		t.Fatalf("UpdateFileHashes with empty slice failed: %v", err)
	}
}

func TestFileTracker_ConcurrentAccess(t *testing.T) {
	store, cleanup := createTestSQLiteStore(t)
	defer cleanup()

	ctx := context.Background()
	repoID := "github.com/test/repo"

	// Store test repo first
	repoCtx := createTestRepoContext()
	repoCtx.ID = repoID
	if err := store.StoreRepoContext(ctx, repoID, repoCtx); err != nil {
		t.Fatalf("Failed to store repo context: %v", err)
	}

	// Run concurrent updates
	errChan := make(chan error, 10)
	for i := 0; i < 10; i++ {
		go func(idx int) {
			path := filepath.Join("pkg", "file"+string(rune('0'+idx))+".go")
			hash := "hash" + string(rune('0'+idx))
			err := store.UpdateFileHash(ctx, repoID, path, hash, int64(idx*100))
			errChan <- err
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		if err := <-errChan; err != nil {
			t.Errorf("Concurrent update failed: %v", err)
		}
	}

	// Verify all files were stored
	hashes, err := store.GetFileHashes(ctx, repoID)
	if err != nil {
		t.Fatalf("GetFileHashes failed: %v", err)
	}

	if len(hashes) != 10 {
		t.Errorf("Expected 10 hashes after concurrent writes, got %d", len(hashes))
	}
}
