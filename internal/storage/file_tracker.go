package storage

import (
	"context"
	"database/sql"
	_ "embed"
	"fmt"
	"strings"
	"time"
)

//go:embed migrations/002_file_hashes.sql
var fileHashesMigration string

// FileHashInfo contains file hash metadata.
type FileHashInfo struct {
	RepoID       string
	FilePath     string
	Hash         string
	LastAnalyzed time.Time
	FileSize     int64
}

// FileTracker provides efficient file change detection using SHA256 hashes.
// It enables incremental indexing by tracking which files have changed since last analysis.
type FileTracker interface {
	// GetChangedFiles compares current file hashes with stored hashes and returns
	// paths of files that have changed (hash mismatch) or are new (not in store).
	// currentFiles is a map of file_path -> hash for the files on disk.
	GetChangedFiles(ctx context.Context, repoID string, currentFiles map[string]string) ([]string, error)

	// GetFileHashes retrieves all stored file hashes for a repository.
	GetFileHashes(ctx context.Context, repoID string) (map[string]FileHashInfo, error)

	// UpdateFileHash updates or inserts a file hash entry.
	UpdateFileHash(ctx context.Context, repoID, filePath, hash string, fileSize int64) error

	// UpdateFileHashes bulk updates file hashes (more efficient for batch operations).
	UpdateFileHashes(ctx context.Context, repoID string, files []FileHashInfo) error

	// CleanupDeletedFiles removes hash entries for files that no longer exist.
	// existingPaths should contain all file paths that currently exist.
	CleanupDeletedFiles(ctx context.Context, repoID string, existingPaths []string) (int64, error)

	// GetStaleFiles returns files that haven't been analyzed since the given time.
	GetStaleFiles(ctx context.Context, repoID string, olderThan time.Time) ([]string, error)

	// DeleteRepoHashes removes all hash entries for a repository.
	DeleteRepoHashes(ctx context.Context, repoID string) error

	// GetFileHash retrieves the hash for a single file.
	GetFileHash(ctx context.Context, repoID, filePath string) (*FileHashInfo, error)
}

// Ensure SQLiteStore implements FileTracker interface.
var _ FileTracker = (*SQLiteStore)(nil)

// migrateFileHashes applies the file hashes migration.
func (s *SQLiteStore) migrateFileHashes() error {
	// Check if migration already applied
	var version int
	err := s.db.QueryRow("SELECT MAX(version) FROM schema_migrations").Scan(&version)
	if err != nil {
		return fmt.Errorf("failed to check schema version: %w", err)
	}

	if version >= 2 {
		return nil // Already migrated
	}

	_, err = s.db.Exec(fileHashesMigration)
	if err != nil {
		return fmt.Errorf("failed to run file hashes migration: %w", err)
	}

	return nil
}

// GetChangedFiles compares current file hashes with stored hashes and returns
// paths of files that have changed or are new.
func (s *SQLiteStore) GetChangedFiles(ctx context.Context, repoID string, currentFiles map[string]string) ([]string, error) {
	if len(currentFiles) == 0 {
		return nil, nil
	}

	// Get all stored hashes for this repo
	storedHashes, err := s.GetFileHashes(ctx, repoID)
	if err != nil {
		return nil, fmt.Errorf("failed to get stored hashes: %w", err)
	}

	var changedFiles []string

	// Check each current file
	for path, currentHash := range currentFiles {
		stored, exists := storedHashes[path]
		if !exists || stored.Hash != currentHash {
			changedFiles = append(changedFiles, path)
		}
	}

	return changedFiles, nil
}

// GetFileHashes retrieves all stored file hashes for a repository.
func (s *SQLiteStore) GetFileHashes(ctx context.Context, repoID string) (map[string]FileHashInfo, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT file_path, hash, last_analyzed, file_size
		FROM file_hashes
		WHERE repo_id = ?`,
		repoID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query file hashes: %w", err)
	}
	defer rows.Close()

	hashes := make(map[string]FileHashInfo)
	for rows.Next() {
		var info FileHashInfo
		var fileSize sql.NullInt64
		var lastAnalyzed sql.NullTime

		if err := rows.Scan(&info.FilePath, &info.Hash, &lastAnalyzed, &fileSize); err != nil {
			return nil, fmt.Errorf("failed to scan file hash row: %w", err)
		}

		info.RepoID = repoID
		if lastAnalyzed.Valid {
			info.LastAnalyzed = lastAnalyzed.Time
		}
		if fileSize.Valid {
			info.FileSize = fileSize.Int64
		}

		hashes[info.FilePath] = info
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating file hashes: %w", err)
	}

	return hashes, nil
}

// UpdateFileHash updates or inserts a file hash entry.
func (s *SQLiteStore) UpdateFileHash(ctx context.Context, repoID, filePath, hash string, fileSize int64) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO file_hashes (repo_id, file_path, hash, last_analyzed, file_size)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(repo_id, file_path) DO UPDATE SET
			hash = excluded.hash,
			last_analyzed = excluded.last_analyzed,
			file_size = excluded.file_size`,
		repoID, filePath, hash, time.Now(), fileSize,
	)
	if err != nil {
		return fmt.Errorf("failed to update file hash: %w", err)
	}
	return nil
}

// UpdateFileHashes bulk updates file hashes (more efficient for batch operations).
func (s *SQLiteStore) UpdateFileHashes(ctx context.Context, repoID string, files []FileHashInfo) error {
	if len(files) == 0 {
		return nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO file_hashes (repo_id, file_path, hash, last_analyzed, file_size)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(repo_id, file_path) DO UPDATE SET
			hash = excluded.hash,
			last_analyzed = excluded.last_analyzed,
			file_size = excluded.file_size`)
	if err != nil {
		return fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer stmt.Close()

	now := time.Now()
	for _, file := range files {
		lastAnalyzed := file.LastAnalyzed
		if lastAnalyzed.IsZero() {
			lastAnalyzed = now
		}

		_, err := stmt.ExecContext(ctx, repoID, file.FilePath, file.Hash, lastAnalyzed, file.FileSize)
		if err != nil {
			return fmt.Errorf("failed to insert file hash for %s: %w", file.FilePath, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// CleanupDeletedFiles removes hash entries for files that no longer exist.
// Returns the number of deleted entries.
func (s *SQLiteStore) CleanupDeletedFiles(ctx context.Context, repoID string, existingPaths []string) (int64, error) {
	if len(existingPaths) == 0 {
		// Delete all files for this repo if no existing paths
		result, err := s.db.ExecContext(ctx, `
			DELETE FROM file_hashes WHERE repo_id = ?`, repoID)
		if err != nil {
			return 0, fmt.Errorf("failed to delete all file hashes: %w", err)
		}
		return result.RowsAffected()
	}

	// Build placeholders for IN clause
	placeholders := make([]string, len(existingPaths))
	args := make([]interface{}, len(existingPaths)+1)
	args[0] = repoID
	for i, path := range existingPaths {
		placeholders[i] = "?"
		args[i+1] = path
	}

	query := fmt.Sprintf(`
		DELETE FROM file_hashes
		WHERE repo_id = ?
		AND file_path NOT IN (%s)`,
		strings.Join(placeholders, ","),
	)

	result, err := s.db.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, fmt.Errorf("failed to cleanup deleted files: %w", err)
	}

	return result.RowsAffected()
}

// GetStaleFiles returns files that haven't been analyzed since the given time.
func (s *SQLiteStore) GetStaleFiles(ctx context.Context, repoID string, olderThan time.Time) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT file_path FROM file_hashes
		WHERE repo_id = ? AND last_analyzed < ?`,
		repoID, olderThan,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query stale files: %w", err)
	}
	defer rows.Close()

	var stalePaths []string
	for rows.Next() {
		var path string
		if err := rows.Scan(&path); err != nil {
			return nil, fmt.Errorf("failed to scan stale file path: %w", err)
		}
		stalePaths = append(stalePaths, path)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating stale files: %w", err)
	}

	return stalePaths, nil
}

// DeleteRepoHashes removes all hash entries for a repository.
func (s *SQLiteStore) DeleteRepoHashes(ctx context.Context, repoID string) error {
	_, err := s.db.ExecContext(ctx, `
		DELETE FROM file_hashes WHERE repo_id = ?`, repoID)
	if err != nil {
		return fmt.Errorf("failed to delete repo hashes: %w", err)
	}
	return nil
}

// GetFileHash retrieves the hash for a single file.
func (s *SQLiteStore) GetFileHash(ctx context.Context, repoID, filePath string) (*FileHashInfo, error) {
	var info FileHashInfo
	var fileSize sql.NullInt64
	var lastAnalyzed sql.NullTime

	err := s.db.QueryRowContext(ctx, `
		SELECT file_path, hash, last_analyzed, file_size
		FROM file_hashes
		WHERE repo_id = ? AND file_path = ?`,
		repoID, filePath,
	).Scan(&info.FilePath, &info.Hash, &lastAnalyzed, &fileSize)

	if err == sql.ErrNoRows {
		return nil, nil // File not found, not an error
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get file hash: %w", err)
	}

	info.RepoID = repoID
	if lastAnalyzed.Valid {
		info.LastAnalyzed = lastAnalyzed.Time
	}
	if fileSize.Valid {
		info.FileSize = fileSize.Int64
	}

	return &info, nil
}

// SyncFileHashesFromFiles populates file_hashes table from existing files table.
// This is useful for migrating existing data to use incremental indexing.
func (s *SQLiteStore) SyncFileHashesFromFiles(ctx context.Context, repoID string) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO file_hashes (repo_id, file_path, hash, last_analyzed, file_size)
		SELECT ?, path, hash, analyzed_at, size
		FROM files
		WHERE repo_id = ?
		ON CONFLICT(repo_id, file_path) DO UPDATE SET
			hash = excluded.hash,
			last_analyzed = excluded.last_analyzed,
			file_size = excluded.file_size`,
		repoID, repoID,
	)
	if err != nil {
		return fmt.Errorf("failed to sync file hashes from files table: %w", err)
	}
	return nil
}

// FileChangeStats contains statistics about file changes.
type FileChangeStats struct {
	TotalTracked  int
	NewFiles      int
	ModifiedFiles int
	DeletedFiles  int
	UnchangedFiles int
}

// CompareAndGetStats compares current files with stored hashes and returns detailed stats.
// currentFiles is a map of file_path -> hash.
func (s *SQLiteStore) CompareAndGetStats(ctx context.Context, repoID string, currentFiles map[string]string) (*FileChangeStats, error) {
	storedHashes, err := s.GetFileHashes(ctx, repoID)
	if err != nil {
		return nil, fmt.Errorf("failed to get stored hashes: %w", err)
	}

	stats := &FileChangeStats{
		TotalTracked: len(storedHashes),
	}

	// Track which stored files we've seen
	seenPaths := make(map[string]bool)

	for path, currentHash := range currentFiles {
		seenPaths[path] = true
		stored, exists := storedHashes[path]
		if !exists {
			stats.NewFiles++
		} else if stored.Hash != currentHash {
			stats.ModifiedFiles++
		} else {
			stats.UnchangedFiles++
		}
	}

	// Count deleted files (in storage but not in current)
	for path := range storedHashes {
		if !seenPaths[path] {
			stats.DeletedFiles++
		}
	}

	return stats, nil
}
