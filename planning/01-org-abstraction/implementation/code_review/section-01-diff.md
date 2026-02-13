diff --git a/internal/storage/migrations/003_org_tables.sql b/internal/storage/migrations/003_org_tables.sql
new file mode 100644
index 0000000..b96be7b
--- /dev/null
+++ b/internal/storage/migrations/003_org_tables.sql
@@ -0,0 +1,32 @@
+-- MCP Repo Context Server - Organization Tables
+-- Adds organization management tables for grouping repos under orgs.
+
+-- Organizations table
+CREATE TABLE IF NOT EXISTS orgs (
+    id TEXT PRIMARY KEY,
+    config_json TEXT,
+    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
+    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
+);
+
+-- Organization-repository junction table
+CREATE TABLE IF NOT EXISTS org_repos (
+    org_id TEXT NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
+    repo_id TEXT NOT NULL,
+    config_override_json TEXT,
+    added_at DATETIME DEFAULT CURRENT_TIMESTAMP,
+    PRIMARY KEY (org_id, repo_id)
+);
+
+-- Index for reverse lookups (find which org a repo belongs to)
+CREATE INDEX IF NOT EXISTS idx_org_repos_repo_id ON org_repos(repo_id);
+
+-- Trigger to auto-update updated_at on orgs
+CREATE TRIGGER IF NOT EXISTS update_orgs_timestamp
+AFTER UPDATE ON orgs
+BEGIN
+    UPDATE orgs SET updated_at = CURRENT_TIMESTAMP WHERE id = NEW.id;
+END;
+
+-- Record migration version
+INSERT OR IGNORE INTO schema_migrations (version) VALUES (3);
diff --git a/internal/storage/org_migration_test.go b/internal/storage/org_migration_test.go
new file mode 100644
index 0000000..d449161
--- /dev/null
+++ b/internal/storage/org_migration_test.go
@@ -0,0 +1,219 @@
+package storage
+
+import (
+	"database/sql"
+	"testing"
+
+	_ "github.com/mattn/go-sqlite3"
+)
+
+// createTestDBWithMigrations creates an in-memory SQLite DB with all migrations applied.
+func createTestDBWithMigrations(t *testing.T) *sql.DB {
+	t.Helper()
+	db, err := sql.Open("sqlite3", ":memory:?_foreign_keys=ON")
+	if err != nil {
+		t.Fatalf("Failed to open in-memory DB: %v", err)
+	}
+	t.Cleanup(func() { db.Close() })
+
+	store, err := NewSQLiteStoreWithDB(db)
+	if err != nil {
+		t.Fatalf("Failed to create store with DB: %v", err)
+	}
+	_ = store
+	return db
+}
+
+func TestOrgMigration_CreatesOrgsTable(t *testing.T) {
+	db := createTestDBWithMigrations(t)
+
+	// Verify orgs table exists with correct columns
+	rows, err := db.Query("PRAGMA table_info(orgs)")
+	if err != nil {
+		t.Fatalf("Failed to query orgs table info: %v", err)
+	}
+	defer rows.Close()
+
+	columns := make(map[string]string) // name -> type
+	for rows.Next() {
+		var cid int
+		var name, colType string
+		var notNull, pk int
+		var dflt sql.NullString
+		if err := rows.Scan(&cid, &name, &colType, &notNull, &dflt, &pk); err != nil {
+			t.Fatalf("Failed to scan column info: %v", err)
+		}
+		columns[name] = colType
+	}
+
+	expectedCols := []string{"id", "config_json", "created_at", "updated_at"}
+	for _, col := range expectedCols {
+		if _, ok := columns[col]; !ok {
+			t.Errorf("Expected column %q in orgs table, not found. Got columns: %v", col, columns)
+		}
+	}
+}
+
+func TestOrgMigration_CreatesOrgReposTable(t *testing.T) {
+	db := createTestDBWithMigrations(t)
+
+	rows, err := db.Query("PRAGMA table_info(org_repos)")
+	if err != nil {
+		t.Fatalf("Failed to query org_repos table info: %v", err)
+	}
+	defer rows.Close()
+
+	columns := make(map[string]string)
+	for rows.Next() {
+		var cid int
+		var name, colType string
+		var notNull, pk int
+		var dflt sql.NullString
+		if err := rows.Scan(&cid, &name, &colType, &notNull, &dflt, &pk); err != nil {
+			t.Fatalf("Failed to scan column info: %v", err)
+		}
+		columns[name] = colType
+	}
+
+	expectedCols := []string{"org_id", "repo_id", "config_override_json", "added_at"}
+	for _, col := range expectedCols {
+		if _, ok := columns[col]; !ok {
+			t.Errorf("Expected column %q in org_repos table, not found. Got columns: %v", col, columns)
+		}
+	}
+}
+
+func TestOrgMigration_CreatesRepoIdIndex(t *testing.T) {
+	db := createTestDBWithMigrations(t)
+
+	var count int
+	err := db.QueryRow(`
+		SELECT COUNT(*) FROM sqlite_master
+		WHERE type='index' AND name='idx_org_repos_repo_id'
+	`).Scan(&count)
+	if err != nil {
+		t.Fatalf("Failed to check index: %v", err)
+	}
+	if count != 1 {
+		t.Error("Expected idx_org_repos_repo_id index to exist")
+	}
+}
+
+func TestOrgMigration_CreatesUpdatedAtTrigger(t *testing.T) {
+	db := createTestDBWithMigrations(t)
+
+	var count int
+	err := db.QueryRow(`
+		SELECT COUNT(*) FROM sqlite_master
+		WHERE type='trigger' AND name='update_orgs_timestamp'
+	`).Scan(&count)
+	if err != nil {
+		t.Fatalf("Failed to check trigger: %v", err)
+	}
+	if count != 1 {
+		t.Error("Expected update_orgs_timestamp trigger to exist")
+	}
+}
+
+func TestOrgMigration_IsIdempotent(t *testing.T) {
+	db, err := sql.Open("sqlite3", ":memory:?_foreign_keys=ON")
+	if err != nil {
+		t.Fatalf("Failed to open DB: %v", err)
+	}
+	defer db.Close()
+
+	// Create store twice — second time should not error
+	store1, err := NewSQLiteStoreWithDB(db)
+	if err != nil {
+		t.Fatalf("First NewSQLiteStoreWithDB failed: %v", err)
+	}
+	_ = store1
+
+	// Run migrations again (simulating restart)
+	store2 := &SQLiteStore{db: db}
+	if err := store2.migrate(); err != nil {
+		t.Fatalf("Second migrate() call failed: %v", err)
+	}
+}
+
+func TestOrgMigration_RecordsVersion3(t *testing.T) {
+	db := createTestDBWithMigrations(t)
+
+	var version int
+	err := db.QueryRow("SELECT MAX(version) FROM schema_migrations WHERE version = 3").Scan(&version)
+	if err != nil {
+		t.Fatalf("Failed to check schema version: %v", err)
+	}
+	if version != 3 {
+		t.Errorf("Expected schema version 3, got %d", version)
+	}
+}
+
+func TestOrgMigration_CascadeDelete(t *testing.T) {
+	db := createTestDBWithMigrations(t)
+
+	// Insert an org
+	_, err := db.Exec(`INSERT INTO orgs (id) VALUES ('test-org')`)
+	if err != nil {
+		t.Fatalf("Failed to insert org: %v", err)
+	}
+
+	// Insert org_repos entries
+	_, err = db.Exec(`INSERT INTO org_repos (org_id, repo_id) VALUES ('test-org', 'repo1')`)
+	if err != nil {
+		t.Fatalf("Failed to insert org_repo: %v", err)
+	}
+	_, err = db.Exec(`INSERT INTO org_repos (org_id, repo_id) VALUES ('test-org', 'repo2')`)
+	if err != nil {
+		t.Fatalf("Failed to insert org_repo: %v", err)
+	}
+
+	// Delete org — should cascade to org_repos
+	_, err = db.Exec(`DELETE FROM orgs WHERE id = 'test-org'`)
+	if err != nil {
+		t.Fatalf("Failed to delete org: %v", err)
+	}
+
+	// Verify org_repos entries are gone
+	var count int
+	err = db.QueryRow(`SELECT COUNT(*) FROM org_repos WHERE org_id = 'test-org'`).Scan(&count)
+	if err != nil {
+		t.Fatalf("Failed to count org_repos: %v", err)
+	}
+	if count != 0 {
+		t.Errorf("Expected 0 org_repos after cascade delete, got %d", count)
+	}
+}
+
+func TestOrgMigration_ForeignKeyConstraint(t *testing.T) {
+	db := createTestDBWithMigrations(t)
+
+	// Attempt to insert org_repo with non-existent org — should fail
+	_, err := db.Exec(`INSERT INTO org_repos (org_id, repo_id) VALUES ('nonexistent-org', 'repo1')`)
+	if err == nil {
+		t.Error("Expected foreign key constraint error, got nil")
+	}
+}
+
+func TestNewSQLiteStoreWithDB(t *testing.T) {
+	db, err := sql.Open("sqlite3", ":memory:?_foreign_keys=ON")
+	if err != nil {
+		t.Fatalf("Failed to open DB: %v", err)
+	}
+	defer db.Close()
+
+	store, err := NewSQLiteStoreWithDB(db)
+	if err != nil {
+		t.Fatalf("NewSQLiteStoreWithDB failed: %v", err)
+	}
+
+	// Verify the store can be used (migrations ran)
+	var count int
+	err = store.db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='orgs'").Scan(&count)
+	if err != nil {
+		t.Fatalf("Failed to query: %v", err)
+	}
+	if count != 1 {
+		t.Error("Expected orgs table to exist after NewSQLiteStoreWithDB")
+	}
+}
diff --git a/internal/storage/sqlite.go b/internal/storage/sqlite.go
index bca4543..f2b9eab 100644
--- a/internal/storage/sqlite.go
+++ b/internal/storage/sqlite.go
@@ -18,6 +18,9 @@ import (
 //go:embed migrations/001_initial_schema.sql
 var initialSchema string
 
+//go:embed migrations/003_org_tables.sql
+var orgTablesMigration string
+
 // Ensure SQLiteStore implements ContextStore interface.
 var _ ContextStore = (*SQLiteStore)(nil)
 
@@ -59,6 +62,23 @@ func NewSQLiteStore(path string) (*SQLiteStore, error) {
 	return store, nil
 }
 
+// NewSQLiteStoreWithDB creates a store using a pre-opened *sql.DB.
+// This enables sharing the same database connection between stores.
+func NewSQLiteStoreWithDB(db *sql.DB) (*SQLiteStore, error) {
+	store := &SQLiteStore{db: db}
+
+	if err := store.migrate(); err != nil {
+		return nil, fmt.Errorf("failed to run migrations: %w", err)
+	}
+
+	return store, nil
+}
+
+// DB returns the underlying *sql.DB for sharing with other stores.
+func (s *SQLiteStore) DB() *sql.DB {
+	return s.db
+}
+
 // migrate applies database migrations.
 func (s *SQLiteStore) migrate() error {
 	// Run initial schema migration
@@ -72,6 +92,31 @@ func (s *SQLiteStore) migrate() error {
 		return fmt.Errorf("failed to apply file hashes migration: %w", err)
 	}
 
+	// Run org tables migration
+	if err := s.migrateOrgTables(); err != nil {
+		return fmt.Errorf("failed to apply org tables migration: %w", err)
+	}
+
+	return nil
+}
+
+// migrateOrgTables applies the org tables migration.
+func (s *SQLiteStore) migrateOrgTables() error {
+	var version int
+	err := s.db.QueryRow("SELECT MAX(version) FROM schema_migrations").Scan(&version)
+	if err != nil {
+		return fmt.Errorf("failed to check schema version: %w", err)
+	}
+
+	if version >= 3 {
+		return nil // Already migrated
+	}
+
+	_, err = s.db.Exec(orgTablesMigration)
+	if err != nil {
+		return fmt.Errorf("failed to run org tables migration: %w", err)
+	}
+
 	return nil
 }
 
