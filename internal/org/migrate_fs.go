package org

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// MigrateFromFilesystem migrates org data from the legacy _orgs.json file
// to the SQLite store. It is idempotent: if _orgs.json doesn't exist
// (already migrated or never created), it returns nil.
// On success, _orgs.json is renamed to _orgs.json.migrated.
func MigrateFromFilesystem(fsPath string, sqlStore *SQLiteStore) error {
	jsonPath := filepath.Join(fsPath, "_orgs.json")

	data, err := os.ReadFile(jsonPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // Nothing to migrate
		}
		return fmt.Errorf("read _orgs.json: %w", err)
	}

	var d orgData
	if err := json.Unmarshal(data, &d); err != nil {
		return fmt.Errorf("parse _orgs.json: %w", err)
	}

	ctx := context.Background()
	tx, err := sqlStore.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	for _, o := range d.Orgs {
		configJSON, err := json.Marshal(o.Config)
		if err != nil {
			return fmt.Errorf("marshal config for org %s: %w", o.ID, err)
		}

		_, err = tx.ExecContext(ctx, `
			INSERT INTO orgs (id, config_json, created_at)
			VALUES (?, ?, COALESCE((SELECT created_at FROM orgs WHERE id = ?), CURRENT_TIMESTAMP))
			ON CONFLICT(id) DO UPDATE SET config_json = excluded.config_json`,
			o.ID, string(configJSON), o.ID,
		)
		if err != nil {
			return fmt.Errorf("upsert org %s: %w", o.ID, err)
		}

		// Sync repos
		_, err = tx.ExecContext(ctx, `DELETE FROM org_repos WHERE org_id = ?`, o.ID)
		if err != nil {
			return fmt.Errorf("delete old repos for org %s: %w", o.ID, err)
		}

		for _, repoID := range o.Repos {
			_, err = tx.ExecContext(ctx, `
				INSERT INTO org_repos (org_id, repo_id) VALUES (?, ?)`,
				o.ID, repoID,
			)
			if err != nil {
				return fmt.Errorf("insert repo %s for org %s: %w", repoID, o.ID, err)
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit migration: %w", err)
	}

	// Rename to .migrated on success
	migratedPath := jsonPath + ".migrated"
	if err := os.Rename(jsonPath, migratedPath); err != nil {
		return fmt.Errorf("rename _orgs.json: %w", err)
	}

	return nil
}
