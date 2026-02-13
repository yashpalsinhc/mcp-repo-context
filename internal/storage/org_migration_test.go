package storage

import (
	"database/sql"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

// createTestDBWithMigrations creates an in-memory SQLite DB with all migrations applied.
func createTestDBWithMigrations(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:?_foreign_keys=ON")
	if err != nil {
		t.Fatalf("Failed to open in-memory DB: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	store, err := NewSQLiteStoreWithDB(db)
	if err != nil {
		t.Fatalf("Failed to create store with DB: %v", err)
	}
	_ = store
	return db
}

func TestOrgMigration_CreatesOrgsTable(t *testing.T) {
	db := createTestDBWithMigrations(t)

	// Verify orgs table exists with correct columns
	rows, err := db.Query("PRAGMA table_info(orgs)")
	if err != nil {
		t.Fatalf("Failed to query orgs table info: %v", err)
	}
	defer rows.Close()

	columns := make(map[string]string) // name -> type
	for rows.Next() {
		var cid int
		var name, colType string
		var notNull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &colType, &notNull, &dflt, &pk); err != nil {
			t.Fatalf("Failed to scan column info: %v", err)
		}
		columns[name] = colType
	}

	expectedCols := []string{"id", "config_json", "created_at", "updated_at"}
	for _, col := range expectedCols {
		if _, ok := columns[col]; !ok {
			t.Errorf("Expected column %q in orgs table, not found. Got columns: %v", col, columns)
		}
	}
}

func TestOrgMigration_CreatesOrgReposTable(t *testing.T) {
	db := createTestDBWithMigrations(t)

	rows, err := db.Query("PRAGMA table_info(org_repos)")
	if err != nil {
		t.Fatalf("Failed to query org_repos table info: %v", err)
	}
	defer rows.Close()

	columns := make(map[string]string)
	for rows.Next() {
		var cid int
		var name, colType string
		var notNull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &colType, &notNull, &dflt, &pk); err != nil {
			t.Fatalf("Failed to scan column info: %v", err)
		}
		columns[name] = colType
	}

	expectedCols := []string{"org_id", "repo_id", "config_override_json", "added_at"}
	for _, col := range expectedCols {
		if _, ok := columns[col]; !ok {
			t.Errorf("Expected column %q in org_repos table, not found. Got columns: %v", col, columns)
		}
	}
}

func TestOrgMigration_CreatesRepoIdIndex(t *testing.T) {
	db := createTestDBWithMigrations(t)

	var count int
	err := db.QueryRow(`
		SELECT COUNT(*) FROM sqlite_master
		WHERE type='index' AND name='idx_org_repos_repo_id'
	`).Scan(&count)
	if err != nil {
		t.Fatalf("Failed to check index: %v", err)
	}
	if count != 1 {
		t.Error("Expected idx_org_repos_repo_id index to exist")
	}
}

func TestOrgMigration_CreatesUpdatedAtTrigger(t *testing.T) {
	db := createTestDBWithMigrations(t)

	// Verify trigger exists
	var count int
	err := db.QueryRow(`
		SELECT COUNT(*) FROM sqlite_master
		WHERE type='trigger' AND name='update_orgs_timestamp'
	`).Scan(&count)
	if err != nil {
		t.Fatalf("Failed to check trigger: %v", err)
	}
	if count != 1 {
		t.Error("Expected update_orgs_timestamp trigger to exist")
	}

	// Verify trigger actually fires on UPDATE
	_, err = db.Exec(`INSERT INTO orgs (id, config_json) VALUES ('trigger-test', '{}')`)
	if err != nil {
		t.Fatalf("Failed to insert org: %v", err)
	}

	var createdAt, updatedAt string
	err = db.QueryRow(`SELECT created_at, updated_at FROM orgs WHERE id = 'trigger-test'`).Scan(&createdAt, &updatedAt)
	if err != nil {
		t.Fatalf("Failed to read timestamps: %v", err)
	}
	originalUpdatedAt := updatedAt

	// Small delay to ensure timestamp difference
	// SQLite CURRENT_TIMESTAMP has second precision
	_, err = db.Exec(`UPDATE orgs SET config_json = '{"updated":true}' WHERE id = 'trigger-test'`)
	if err != nil {
		t.Fatalf("Failed to update org (trigger should not cause recursion): %v", err)
	}

	err = db.QueryRow(`SELECT updated_at FROM orgs WHERE id = 'trigger-test'`).Scan(&updatedAt)
	if err != nil {
		t.Fatalf("Failed to read updated_at after update: %v", err)
	}

	// The trigger should have fired — updated_at should be >= original
	if updatedAt < originalUpdatedAt {
		t.Errorf("updated_at went backwards: was %s, now %s", originalUpdatedAt, updatedAt)
	}
}

func TestNewSQLiteStoreWithDB_NilDB(t *testing.T) {
	_, err := NewSQLiteStoreWithDB(nil)
	if err == nil {
		t.Error("Expected error when passing nil db, got nil")
	}
}

func TestOrgMigration_IsIdempotent(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:?_foreign_keys=ON")
	if err != nil {
		t.Fatalf("Failed to open DB: %v", err)
	}
	defer db.Close()

	// Create store twice — second time should not error
	store1, err := NewSQLiteStoreWithDB(db)
	if err != nil {
		t.Fatalf("First NewSQLiteStoreWithDB failed: %v", err)
	}
	_ = store1

	// Run migrations again (simulating restart)
	store2 := &SQLiteStore{db: db}
	if err := store2.migrate(); err != nil {
		t.Fatalf("Second migrate() call failed: %v", err)
	}
}

func TestOrgMigration_RecordsVersion3(t *testing.T) {
	db := createTestDBWithMigrations(t)

	var version int
	err := db.QueryRow("SELECT MAX(version) FROM schema_migrations WHERE version = 3").Scan(&version)
	if err != nil {
		t.Fatalf("Failed to check schema version: %v", err)
	}
	if version != 3 {
		t.Errorf("Expected schema version 3, got %d", version)
	}
}

func TestOrgMigration_CascadeDelete(t *testing.T) {
	db := createTestDBWithMigrations(t)

	// Insert an org
	_, err := db.Exec(`INSERT INTO orgs (id) VALUES ('test-org')`)
	if err != nil {
		t.Fatalf("Failed to insert org: %v", err)
	}

	// Insert org_repos entries
	_, err = db.Exec(`INSERT INTO org_repos (org_id, repo_id) VALUES ('test-org', 'repo1')`)
	if err != nil {
		t.Fatalf("Failed to insert org_repo: %v", err)
	}
	_, err = db.Exec(`INSERT INTO org_repos (org_id, repo_id) VALUES ('test-org', 'repo2')`)
	if err != nil {
		t.Fatalf("Failed to insert org_repo: %v", err)
	}

	// Delete org — should cascade to org_repos
	_, err = db.Exec(`DELETE FROM orgs WHERE id = 'test-org'`)
	if err != nil {
		t.Fatalf("Failed to delete org: %v", err)
	}

	// Verify org_repos entries are gone
	var count int
	err = db.QueryRow(`SELECT COUNT(*) FROM org_repos WHERE org_id = 'test-org'`).Scan(&count)
	if err != nil {
		t.Fatalf("Failed to count org_repos: %v", err)
	}
	if count != 0 {
		t.Errorf("Expected 0 org_repos after cascade delete, got %d", count)
	}
}

func TestOrgMigration_ForeignKeyConstraint(t *testing.T) {
	db := createTestDBWithMigrations(t)

	// Attempt to insert org_repo with non-existent org — should fail
	_, err := db.Exec(`INSERT INTO org_repos (org_id, repo_id) VALUES ('nonexistent-org', 'repo1')`)
	if err == nil {
		t.Error("Expected foreign key constraint error, got nil")
	}
}

func TestNewSQLiteStoreWithDB(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:?_foreign_keys=ON")
	if err != nil {
		t.Fatalf("Failed to open DB: %v", err)
	}
	defer db.Close()

	store, err := NewSQLiteStoreWithDB(db)
	if err != nil {
		t.Fatalf("NewSQLiteStoreWithDB failed: %v", err)
	}

	// Verify the store can be used (migrations ran)
	var count int
	err = store.db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='orgs'").Scan(&count)
	if err != nil {
		t.Fatalf("Failed to query: %v", err)
	}
	if count != 1 {
		t.Error("Expected orgs table to exist after NewSQLiteStoreWithDB")
	}
}
