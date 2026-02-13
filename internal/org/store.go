package org

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
)

// Store manages org persistence with atomic operations.
type Store interface {
	// Org CRUD
	SaveOrg(ctx context.Context, o *Org) error
	GetOrg(ctx context.Context, orgID string) (*Org, error)
	ListOrgs(ctx context.Context) ([]OrgWithCount, error)
	DeleteOrg(ctx context.Context, orgID string) error

	// Repo junction — atomic at DB level
	AddRepos(ctx context.Context, orgID string, repoIDs []string) error
	RemoveRepos(ctx context.Context, orgID string, repoIDs []string) error

	// Config overrides
	GetRepoConfigOverride(ctx context.Context, orgID, repoID string) (*OrgConfig, error)
	SetRepoConfigOverride(ctx context.Context, orgID, repoID string, config *OrgConfig) error

	// Migration
	RunMigrations() error
}

// SQLiteStore implements Store using SQLite.
type SQLiteStore struct {
	db *sql.DB
}

// NewSQLiteStore creates a new SQLite org store.
// The db should already have org tables created (via storage.NewSQLiteStoreWithDB).
func NewSQLiteStore(db *sql.DB) (*SQLiteStore, error) {
	if db == nil {
		return nil, fmt.Errorf("db must not be nil")
	}
	// Verify org tables exist (fail fast if migrations haven't run)
	if _, err := db.Exec("SELECT 1 FROM orgs LIMIT 0"); err != nil {
		return nil, fmt.Errorf("orgs table not found — run storage migrations first: %w", err)
	}
	return &SQLiteStore{db: db}, nil
}

// RunMigrations is a no-op — migrations are handled by storage.SQLiteStore.
func (s *SQLiteStore) RunMigrations() error {
	return nil
}

// SaveOrg upserts an org with its repos.
func (s *SQLiteStore) SaveOrg(ctx context.Context, o *Org) error {
	configJSON, err := json.Marshal(o.Config)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Upsert org
	_, err = tx.ExecContext(ctx, `
		INSERT INTO orgs (id, config_json, created_at)
		VALUES (?, ?, COALESCE((SELECT created_at FROM orgs WHERE id = ?), CURRENT_TIMESTAMP))
		ON CONFLICT(id) DO UPDATE SET config_json = excluded.config_json`,
		o.ID, string(configJSON), o.ID,
	)
	if err != nil {
		return fmt.Errorf("failed to upsert org: %w", err)
	}

	// Sync repos: delete old, insert new
	_, err = tx.ExecContext(ctx, `DELETE FROM org_repos WHERE org_id = ?`, o.ID)
	if err != nil {
		return fmt.Errorf("failed to delete old repos: %w", err)
	}

	for _, repoID := range o.Repos {
		_, err = tx.ExecContext(ctx, `
			INSERT INTO org_repos (org_id, repo_id) VALUES (?, ?)`,
			o.ID, repoID,
		)
		if err != nil {
			return fmt.Errorf("failed to insert repo %s: %w", repoID, err)
		}
	}

	return tx.Commit()
}

// GetOrg retrieves an org by ID.
func (s *SQLiteStore) GetOrg(ctx context.Context, orgID string) (*Org, error) {
	var o Org
	var configJSON sql.NullString
	var createdAt sql.NullTime

	err := s.db.QueryRowContext(ctx, `
		SELECT id, config_json, created_at FROM orgs WHERE id = ?`, orgID,
	).Scan(&o.ID, &configJSON, &createdAt)

	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get org: %w", err)
	}

	if configJSON.Valid && configJSON.String != "" {
		if err := json.Unmarshal([]byte(configJSON.String), &o.Config); err != nil {
			return nil, fmt.Errorf("failed to unmarshal config: %w", err)
		}
	}
	if createdAt.Valid {
		o.Created = createdAt.Time
	}

	// Load repos
	rows, err := s.db.QueryContext(ctx, `
		SELECT repo_id FROM org_repos WHERE org_id = ? ORDER BY repo_id`, orgID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get repos: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var repoID string
		if err := rows.Scan(&repoID); err != nil {
			return nil, fmt.Errorf("failed to scan repo: %w", err)
		}
		o.Repos = append(o.Repos, repoID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate repos: %w", err)
	}

	return &o, nil
}

// ListOrgs returns all orgs with repo counts (does not load repo IDs).
func (s *SQLiteStore) ListOrgs(ctx context.Context) ([]OrgWithCount, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT o.id, o.config_json, o.created_at, COUNT(r.repo_id)
		FROM orgs o
		LEFT JOIN org_repos r ON o.id = r.org_id
		GROUP BY o.id
		ORDER BY o.id`)
	if err != nil {
		return nil, fmt.Errorf("failed to list orgs: %w", err)
	}
	defer rows.Close()

	var result []OrgWithCount
	for rows.Next() {
		var owc OrgWithCount
		var configJSON sql.NullString
		var createdAt sql.NullTime

		if err := rows.Scan(&owc.ID, &configJSON, &createdAt, &owc.RepoCount); err != nil {
			return nil, fmt.Errorf("failed to scan org: %w", err)
		}

		if configJSON.Valid && configJSON.String != "" {
			if err := json.Unmarshal([]byte(configJSON.String), &owc.Config); err != nil {
				return nil, fmt.Errorf("failed to unmarshal config for org %s: %w", owc.ID, err)
			}
		}
		if createdAt.Valid {
			owc.Created = createdAt.Time
		}

		result = append(result, owc)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate orgs: %w", err)
	}

	if result == nil {
		result = []OrgWithCount{}
	}

	return result, nil
}

// DeleteOrg removes an org (CASCADE removes repos).
func (s *SQLiteStore) DeleteOrg(ctx context.Context, orgID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM orgs WHERE id = ?`, orgID)
	return err
}

// AddRepos adds repos to an org (idempotent, atomic).
func (s *SQLiteStore) AddRepos(ctx context.Context, orgID string, repoIDs []string) error {
	if len(repoIDs) == 0 {
		return nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Verify org exists
	var exists int
	err = tx.QueryRowContext(ctx, `SELECT 1 FROM orgs WHERE id = ?`, orgID).Scan(&exists)
	if err == sql.ErrNoRows {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("failed to check org: %w", err)
	}

	for _, repoID := range repoIDs {
		_, err := tx.ExecContext(ctx, `
			INSERT OR IGNORE INTO org_repos (org_id, repo_id) VALUES (?, ?)`,
			orgID, repoID,
		)
		if err != nil {
			return fmt.Errorf("failed to add repo %s: %w", repoID, err)
		}
	}

	return tx.Commit()
}

// RemoveRepos removes repos from an org (no error for non-existent repos).
func (s *SQLiteStore) RemoveRepos(ctx context.Context, orgID string, repoIDs []string) error {
	if len(repoIDs) == 0 {
		return nil
	}

	placeholders := make([]string, len(repoIDs))
	args := make([]interface{}, len(repoIDs)+1)
	args[0] = orgID
	for i, id := range repoIDs {
		placeholders[i] = "?"
		args[i+1] = id
	}

	query := fmt.Sprintf(`DELETE FROM org_repos WHERE org_id = ? AND repo_id IN (%s)`,
		strings.Join(placeholders, ","))

	_, err := s.db.ExecContext(ctx, query, args...)
	return err
}

// GetRepoConfigOverride returns the config override for a specific repo.
func (s *SQLiteStore) GetRepoConfigOverride(ctx context.Context, orgID, repoID string) (*OrgConfig, error) {
	var configJSON sql.NullString
	err := s.db.QueryRowContext(ctx, `
		SELECT config_override_json FROM org_repos WHERE org_id = ? AND repo_id = ?`,
		orgID, repoID,
	).Scan(&configJSON)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get config override: %w", err)
	}

	if !configJSON.Valid || configJSON.String == "" {
		return nil, nil
	}

	var config OrgConfig
	if err := json.Unmarshal([]byte(configJSON.String), &config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config override: %w", err)
	}

	return &config, nil
}

// SetRepoConfigOverride sets the config override for a specific repo.
func (s *SQLiteStore) SetRepoConfigOverride(ctx context.Context, orgID, repoID string, config *OrgConfig) error {
	configJSON, err := json.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal config override: %w", err)
	}

	result, err := s.db.ExecContext(ctx, `
		UPDATE org_repos SET config_override_json = ? WHERE org_id = ? AND repo_id = ?`,
		string(configJSON), orgID, repoID,
	)
	if err != nil {
		return fmt.Errorf("failed to set config override: %w", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to check rows affected: %w", err)
	}
	if affected == 0 {
		return fmt.Errorf("repo %s not found in org %s", repoID, orgID)
	}

	return nil
}
