package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// OrgVocabularyRecord represents a stored org vocabulary entry.
type OrgVocabularyRecord struct {
	OrgID          string
	VocabularyJSON string
	VersionHash    string
	DocCount       int
	BuiltAt        time.Time
	RepoCount      int
	IsStale        bool
}

// migrateOrgVocabulary applies migration 006 for org vocabulary table.
func (s *SQLiteStore) migrateOrgVocabulary() error {
	var version int
	err := s.db.QueryRow("SELECT COALESCE(MAX(version), 0) FROM schema_migrations").Scan(&version)
	if err != nil {
		return fmt.Errorf("failed to check schema version: %w", err)
	}

	if version >= 6 {
		return nil
	}

	_, err = s.db.Exec(orgVocabularyMigration)
	if err != nil {
		return fmt.Errorf("failed to run org vocabulary migration: %w", err)
	}

	return nil
}

// StoreOrgVocabulary stores or replaces an org's vocabulary data.
func (s *SQLiteStore) StoreOrgVocabulary(ctx context.Context, orgID string, vocabJSON []byte, versionHash string, docCount, repoCount int) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT OR REPLACE INTO org_vocabulary (org_id, vocabulary_json, version_hash, doc_count, built_at, repo_count, is_stale)
		 VALUES (?, ?, ?, ?, ?, ?, 0)`,
		orgID, string(vocabJSON), versionHash, docCount, time.Now(), repoCount)
	if err != nil {
		return fmt.Errorf("failed to store org vocabulary: %w", err)
	}
	return nil
}

// GetOrgVocabulary retrieves an org's vocabulary data. Returns nil if not found.
func (s *SQLiteStore) GetOrgVocabulary(ctx context.Context, orgID string) (*OrgVocabularyRecord, error) {
	var rec OrgVocabularyRecord
	var builtAtStr sql.NullString
	err := s.db.QueryRowContext(ctx,
		`SELECT org_id, vocabulary_json, version_hash, doc_count, built_at, repo_count, is_stale
		 FROM org_vocabulary WHERE org_id = ?`, orgID).
		Scan(&rec.OrgID, &rec.VocabularyJSON, &rec.VersionHash, &rec.DocCount, &builtAtStr, &rec.RepoCount, &rec.IsStale)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get org vocabulary: %w", err)
	}
	if builtAtStr.Valid {
		rec.BuiltAt, _ = time.Parse(time.RFC3339, builtAtStr.String)
	}
	return &rec, nil
}

// MarkOrgVocabularyStale sets the is_stale flag for an org's vocabulary.
func (s *SQLiteStore) MarkOrgVocabularyStale(ctx context.Context, orgID string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE org_vocabulary SET is_stale = 1 WHERE org_id = ?`, orgID)
	if err != nil {
		return fmt.Errorf("failed to mark org vocabulary stale: %w", err)
	}
	return nil
}

// IsOrgVocabularyStale checks if an org's vocabulary is stale.
func (s *SQLiteStore) IsOrgVocabularyStale(ctx context.Context, orgID string) (bool, error) {
	var isStale bool
	err := s.db.QueryRowContext(ctx,
		`SELECT is_stale FROM org_vocabulary WHERE org_id = ?`, orgID).Scan(&isStale)
	if err == sql.ErrNoRows {
		return true, nil // Not found = stale
	}
	if err != nil {
		return false, fmt.Errorf("failed to check org vocabulary staleness: %w", err)
	}
	return isStale, nil
}

// DeleteOrgVocabulary removes an org's vocabulary data.
func (s *SQLiteStore) DeleteOrgVocabulary(ctx context.Context, orgID string) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM org_vocabulary WHERE org_id = ?`, orgID)
	if err != nil {
		return fmt.Errorf("failed to delete org vocabulary: %w", err)
	}
	return nil
}

// ParseOrgVocabularyJSON deserializes vocabulary JSON into a map.
func ParseOrgVocabularyJSON(vocabJSON string) (map[string]float64, error) {
	var wordIDF map[string]float64
	if err := json.Unmarshal([]byte(vocabJSON), &wordIDF); err != nil {
		return nil, fmt.Errorf("failed to parse vocabulary JSON: %w", err)
	}
	return wordIDF, nil
}
