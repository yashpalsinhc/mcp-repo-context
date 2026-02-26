package storage

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strings"
)

// FunctionHashInfo contains metadata about a single function or type's content hash.
type FunctionHashInfo struct {
	Name        string // Fully qualified with receiver, e.g. "(*Foo).Bar"
	Type        string // "function" or "type"
	ContentHash string // SHA256 of raw source code
	VectorID    string // Application-level ref to vectors table
}

// QualifyFunctionName builds a fully qualified function name including receiver.
// For methods: "(Foo).Bar" or "(*Foo).Bar"
// For plain functions: "MyFunction"
func QualifyFunctionName(name, receiver string) string {
	if receiver == "" {
		return name
	}
	return fmt.Sprintf("(%s).%s", receiver, name)
}

// ComputeFunctionHash computes SHA256 hash of a function's source code.
// It reads lines [lineStart, lineEnd] from fileContent.
// If lineStart/lineEnd are 0 or extraction fails, falls back to hashing
// the receiver+name+signature from the AST.
func ComputeFunctionHash(fileContent string, lineStart, lineEnd int, name, receiver, signature string) string {
	if lineStart > 0 && lineEnd > 0 && fileContent != "" {
		lines := strings.Split(fileContent, "\n")
		// Convert 1-indexed to 0-indexed
		start := lineStart - 1
		end := lineEnd
		if start >= 0 && end <= len(lines) && start < end {
			source := strings.Join(lines[start:end], "\n")
			return fmt.Sprintf("%x", sha256.Sum256([]byte(source)))
		}
	}
	// Fallback: hash receiver+name+signature
	fallback := receiver + name + signature
	return fmt.Sprintf("%x", sha256.Sum256([]byte(fallback)))
}

// GetFunctionHashes retrieves all function hashes for a file.
// Returns a map keyed by "name:type".
func (s *SQLiteStore) GetFunctionHashes(ctx context.Context, repoID, filePath string) (map[string]FunctionHashInfo, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT name, type, content_hash, COALESCE(vector_id, '') FROM function_hashes WHERE repo_id = ? AND file_path = ?`,
		repoID, filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to query function hashes: %w", err)
	}
	defer rows.Close()

	result := make(map[string]FunctionHashInfo)
	for rows.Next() {
		var info FunctionHashInfo
		if err := rows.Scan(&info.Name, &info.Type, &info.ContentHash, &info.VectorID); err != nil {
			return nil, fmt.Errorf("failed to scan function hash: %w", err)
		}
		key := info.Name + ":" + info.Type
		result[key] = info
	}
	return result, rows.Err()
}

// UpdateFunctionHashes bulk upserts function hashes for a file.
func (s *SQLiteStore) UpdateFunctionHashes(ctx context.Context, repoID, filePath string, hashes []FunctionHashInfo) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx,
		`INSERT OR REPLACE INTO function_hashes (repo_id, file_path, name, type, content_hash, vector_id) VALUES (?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer stmt.Close()

	for _, h := range hashes {
		var vectorID any = nil
		if h.VectorID != "" {
			vectorID = h.VectorID
		}
		if _, err := stmt.ExecContext(ctx, repoID, filePath, h.Name, h.Type, h.ContentHash, vectorID); err != nil {
			return fmt.Errorf("failed to upsert function hash %s: %w", h.Name, err)
		}
	}

	return tx.Commit()
}

// DeleteFunctionHashes removes all function hashes for a file.
func (s *SQLiteStore) DeleteFunctionHashes(ctx context.Context, repoID, filePath string) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM function_hashes WHERE repo_id = ? AND file_path = ?`,
		repoID, filePath)
	if err != nil {
		return fmt.Errorf("failed to delete function hashes: %w", err)
	}
	return nil
}

// GetChangedFunctions compares current function hashes with stored ones.
// current is a map of "name:type" to content_hash.
// Returns slices of "name:type" keys for added, modified, and removed functions.
func (s *SQLiteStore) GetChangedFunctions(ctx context.Context, repoID, filePath string, current map[string]string) (added, modified, removed []string, err error) {
	stored, err := s.GetFunctionHashes(ctx, repoID, filePath)
	if err != nil {
		return nil, nil, nil, err
	}

	// Find added and modified
	for key, hash := range current {
		info, exists := stored[key]
		if !exists {
			added = append(added, key)
		} else if info.ContentHash != hash {
			modified = append(modified, key)
		}
	}

	// Find removed
	for key := range stored {
		if _, exists := current[key]; !exists {
			removed = append(removed, key)
		}
	}

	return added, modified, removed, nil
}
