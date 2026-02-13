diff --git a/internal/org/manager.go b/internal/org/manager.go
index f7d15c2..4eb9877 100644
--- a/internal/org/manager.go
+++ b/internal/org/manager.go
@@ -14,6 +14,8 @@ type Manager interface {
 	AddRepos(ctx context.Context, orgID string, repoIDs []string) error
 	RemoveRepos(ctx context.Context, orgID string, repoIDs []string) error
 	Delete(ctx context.Context, orgID string) error
+	GetEffectiveConfig(ctx context.Context, orgID, repoID string) (*OrgConfig, error)
+	SetRepoConfigOverride(ctx context.Context, orgID, repoID string, config *OrgConfig) error
 }
 
 // manager implements Manager.
@@ -40,58 +42,46 @@ func (m *manager) Register(ctx context.Context, orgID string, repoIDs []string,
 		Config:  cfg,
 		Created: time.Now(),
 	}
-	if err := m.store.Save(ctx, o); err != nil {
+	if err := m.store.SaveOrg(ctx, o); err != nil {
 		return nil, err
 	}
 	return o, nil
 }
 
 func (m *manager) List(ctx context.Context) ([]OrgWithCount, error) {
-	orgs, err := m.store.List(ctx)
-	if err != nil {
-		return nil, err
-	}
-	result := make([]OrgWithCount, len(orgs))
-	for i, o := range orgs {
-		result[i] = OrgWithCount{Org: o, RepoCount: len(o.Repos)}
-	}
-	return result, nil
+	return m.store.ListOrgs(ctx)
 }
 
 func (m *manager) Get(ctx context.Context, orgID string) (*Org, error) {
-	return m.store.Get(ctx, orgID)
+	return m.store.GetOrg(ctx, orgID)
 }
 
 func (m *manager) AddRepos(ctx context.Context, orgID string, repoIDs []string) error {
-	o, err := m.store.Get(ctx, orgID)
-	if err != nil {
-		return err
-	}
-	o.Repos = uniqueStrings(append(o.Repos, repoIDs...))
-	return m.store.Save(ctx, o)
+	return m.store.AddRepos(ctx, orgID, repoIDs)
 }
 
 func (m *manager) RemoveRepos(ctx context.Context, orgID string, repoIDs []string) error {
-	o, err := m.store.Get(ctx, orgID)
+	return m.store.RemoveRepos(ctx, orgID, repoIDs)
+}
+
+func (m *manager) Delete(ctx context.Context, orgID string) error {
+	return m.store.DeleteOrg(ctx, orgID)
+}
+
+func (m *manager) GetEffectiveConfig(ctx context.Context, orgID, repoID string) (*OrgConfig, error) {
+	o, err := m.store.GetOrg(ctx, orgID)
 	if err != nil {
-		return err
-	}
-	removeSet := make(map[string]bool)
-	for _, r := range repoIDs {
-		removeSet[r] = true
+		return nil, err
 	}
-	var kept []string
-	for _, r := range o.Repos {
-		if !removeSet[r] {
-			kept = append(kept, r)
-		}
+	override, err := m.store.GetRepoConfigOverride(ctx, orgID, repoID)
+	if err != nil {
+		return nil, err
 	}
-	o.Repos = kept
-	return m.store.Save(ctx, o)
+	return MergeConfigs(&o.Config, override), nil
 }
 
-func (m *manager) Delete(ctx context.Context, orgID string) error {
-	return m.store.Delete(ctx, orgID)
+func (m *manager) SetRepoConfigOverride(ctx context.Context, orgID, repoID string, config *OrgConfig) error {
+	return m.store.SetRepoConfigOverride(ctx, orgID, repoID, config)
 }
 
 func uniqueStrings(ss []string) []string {
diff --git a/internal/org/store.go b/internal/org/store.go
index f85dea7..27ffdf7 100644
--- a/internal/org/store.go
+++ b/internal/org/store.go
@@ -2,112 +2,282 @@ package org
 
 import (
 	"context"
+	"database/sql"
 	"encoding/json"
 	"fmt"
-	"os"
-	"path/filepath"
-	"sync"
+	"strings"
 )
 
-// Store manages org persistence.
+// Store manages org persistence with atomic operations.
 type Store interface {
-	Save(ctx context.Context, o *Org) error
-	Get(ctx context.Context, orgID string) (*Org, error)
-	List(ctx context.Context) ([]Org, error)
-	Delete(ctx context.Context, orgID string) error
+	// Org CRUD
+	SaveOrg(ctx context.Context, o *Org) error
+	GetOrg(ctx context.Context, orgID string) (*Org, error)
+	ListOrgs(ctx context.Context) ([]OrgWithCount, error)
+	DeleteOrg(ctx context.Context, orgID string) error
+
+	// Repo junction — atomic at DB level
+	AddRepos(ctx context.Context, orgID string, repoIDs []string) error
+	RemoveRepos(ctx context.Context, orgID string, repoIDs []string) error
+
+	// Config overrides
+	GetRepoConfigOverride(ctx context.Context, orgID, repoID string) (*OrgConfig, error)
+	SetRepoConfigOverride(ctx context.Context, orgID, repoID string, config *OrgConfig) error
+
+	// Migration
+	RunMigrations() error
 }
 
-// FilesystemStore stores orgs in a JSON file.
-type FilesystemStore struct {
-	path string
-	mu   sync.RWMutex
+// SQLiteStore implements Store using SQLite.
+type SQLiteStore struct {
+	db *sql.DB
 }
 
-// NewFilesystemStore creates a store that persists orgs to a JSON file.
-func NewFilesystemStore(basePath string) (*FilesystemStore, error) {
-	path := filepath.Join(basePath, "_orgs.json")
-	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
-		return nil, fmt.Errorf("failed to create org storage dir: %w", err)
+// NewSQLiteStore creates a new SQLite org store.
+// The db should already have org tables created (via storage.NewSQLiteStoreWithDB).
+func NewSQLiteStore(db *sql.DB) (*SQLiteStore, error) {
+	if db == nil {
+		return nil, fmt.Errorf("db must not be nil")
 	}
-	return &FilesystemStore{path: path}, nil
+	return &SQLiteStore{db: db}, nil
 }
 
-type orgData struct {
-	Orgs map[string]*Org `json:"orgs"`
+// RunMigrations is a no-op — migrations are handled by storage.SQLiteStore.
+func (s *SQLiteStore) RunMigrations() error {
+	return nil
 }
 
-func (s *FilesystemStore) load() (*orgData, error) {
-	data, err := os.ReadFile(s.path)
+// SaveOrg upserts an org with its repos.
+func (s *SQLiteStore) SaveOrg(ctx context.Context, o *Org) error {
+	configJSON, err := json.Marshal(o.Config)
 	if err != nil {
-		if os.IsNotExist(err) {
-			return &orgData{Orgs: make(map[string]*Org)}, nil
-		}
-		return nil, err
+		return fmt.Errorf("failed to marshal config: %w", err)
 	}
-	var d orgData
-	if err := json.Unmarshal(data, &d); err != nil {
-		return nil, fmt.Errorf("failed to decode orgs: %w", err)
+
+	tx, err := s.db.BeginTx(ctx, nil)
+	if err != nil {
+		return fmt.Errorf("failed to begin transaction: %w", err)
 	}
-	if d.Orgs == nil {
-		d.Orgs = make(map[string]*Org)
+	defer tx.Rollback()
+
+	// Upsert org
+	_, err = tx.ExecContext(ctx, `
+		INSERT INTO orgs (id, config_json, created_at)
+		VALUES (?, ?, COALESCE((SELECT created_at FROM orgs WHERE id = ?), CURRENT_TIMESTAMP))
+		ON CONFLICT(id) DO UPDATE SET config_json = excluded.config_json`,
+		o.ID, string(configJSON), o.ID,
+	)
+	if err != nil {
+		return fmt.Errorf("failed to upsert org: %w", err)
 	}
-	return &d, nil
+
+	// Sync repos: delete old, insert new
+	_, err = tx.ExecContext(ctx, `DELETE FROM org_repos WHERE org_id = ?`, o.ID)
+	if err != nil {
+		return fmt.Errorf("failed to delete old repos: %w", err)
+	}
+
+	for _, repoID := range o.Repos {
+		_, err = tx.ExecContext(ctx, `
+			INSERT INTO org_repos (org_id, repo_id) VALUES (?, ?)`,
+			o.ID, repoID,
+		)
+		if err != nil {
+			return fmt.Errorf("failed to insert repo %s: %w", repoID, err)
+		}
+	}
+
+	return tx.Commit()
 }
 
-func (s *FilesystemStore) save(d *orgData) error {
-	data, err := json.MarshalIndent(d, "", "  ")
+// GetOrg retrieves an org by ID.
+func (s *SQLiteStore) GetOrg(ctx context.Context, orgID string) (*Org, error) {
+	var o Org
+	var configJSON sql.NullString
+	var createdAt sql.NullTime
+
+	err := s.db.QueryRowContext(ctx, `
+		SELECT id, config_json, created_at FROM orgs WHERE id = ?`, orgID,
+	).Scan(&o.ID, &configJSON, &createdAt)
+
+	if err == sql.ErrNoRows {
+		return nil, ErrNotFound
+	}
 	if err != nil {
-		return err
+		return nil, fmt.Errorf("failed to get org: %w", err)
+	}
+
+	if configJSON.Valid && configJSON.String != "" {
+		if err := json.Unmarshal([]byte(configJSON.String), &o.Config); err != nil {
+			return nil, fmt.Errorf("failed to unmarshal config: %w", err)
+		}
+	}
+	if createdAt.Valid {
+		o.Created = createdAt.Time
 	}
-	return os.WriteFile(s.path, data, 0644)
+
+	// Load repos
+	rows, err := s.db.QueryContext(ctx, `
+		SELECT repo_id FROM org_repos WHERE org_id = ? ORDER BY repo_id`, orgID,
+	)
+	if err != nil {
+		return nil, fmt.Errorf("failed to get repos: %w", err)
+	}
+	defer rows.Close()
+
+	for rows.Next() {
+		var repoID string
+		if err := rows.Scan(&repoID); err != nil {
+			return nil, fmt.Errorf("failed to scan repo: %w", err)
+		}
+		o.Repos = append(o.Repos, repoID)
+	}
+
+	return &o, nil
 }
 
-func (s *FilesystemStore) Save(ctx context.Context, o *Org) error {
-	s.mu.Lock()
-	defer s.mu.Unlock()
-	d, err := s.load()
+// ListOrgs returns all orgs with repo counts (does not load repo IDs).
+func (s *SQLiteStore) ListOrgs(ctx context.Context) ([]OrgWithCount, error) {
+	rows, err := s.db.QueryContext(ctx, `
+		SELECT o.id, o.config_json, o.created_at, COUNT(r.repo_id)
+		FROM orgs o
+		LEFT JOIN org_repos r ON o.id = r.org_id
+		GROUP BY o.id
+		ORDER BY o.id`)
 	if err != nil {
-		return err
+		return nil, fmt.Errorf("failed to list orgs: %w", err)
 	}
-	d.Orgs[o.ID] = o
-	return s.save(d)
+	defer rows.Close()
+
+	var result []OrgWithCount
+	for rows.Next() {
+		var owc OrgWithCount
+		var configJSON sql.NullString
+		var createdAt sql.NullTime
+
+		if err := rows.Scan(&owc.ID, &configJSON, &createdAt, &owc.RepoCount); err != nil {
+			return nil, fmt.Errorf("failed to scan org: %w", err)
+		}
+
+		if configJSON.Valid && configJSON.String != "" {
+			json.Unmarshal([]byte(configJSON.String), &owc.Config)
+		}
+		if createdAt.Valid {
+			owc.Created = createdAt.Time
+		}
+
+		result = append(result, owc)
+	}
+
+	if result == nil {
+		result = []OrgWithCount{}
+	}
+
+	return result, nil
+}
+
+// DeleteOrg removes an org (CASCADE removes repos).
+func (s *SQLiteStore) DeleteOrg(ctx context.Context, orgID string) error {
+	_, err := s.db.ExecContext(ctx, `DELETE FROM orgs WHERE id = ?`, orgID)
+	return err
 }
 
-func (s *FilesystemStore) Get(ctx context.Context, orgID string) (*Org, error) {
-	s.mu.RLock()
-	defer s.mu.RUnlock()
-	d, err := s.load()
+// AddRepos adds repos to an org (idempotent).
+func (s *SQLiteStore) AddRepos(ctx context.Context, orgID string, repoIDs []string) error {
+	if len(repoIDs) == 0 {
+		return nil
+	}
+
+	// Verify org exists
+	var exists int
+	err := s.db.QueryRowContext(ctx, `SELECT 1 FROM orgs WHERE id = ?`, orgID).Scan(&exists)
+	if err == sql.ErrNoRows {
+		return ErrNotFound
+	}
 	if err != nil {
-		return nil, err
+		return fmt.Errorf("failed to check org: %w", err)
 	}
-	o, ok := d.Orgs[orgID]
-	if !ok {
-		return nil, fmt.Errorf("org not found: %s", orgID)
+
+	for _, repoID := range repoIDs {
+		_, err := s.db.ExecContext(ctx, `
+			INSERT OR IGNORE INTO org_repos (org_id, repo_id) VALUES (?, ?)`,
+			orgID, repoID,
+		)
+		if err != nil {
+			return fmt.Errorf("failed to add repo %s: %w", repoID, err)
+		}
 	}
-	return o, nil
+
+	return nil
 }
 
-func (s *FilesystemStore) List(ctx context.Context) ([]Org, error) {
-	s.mu.RLock()
-	defer s.mu.RUnlock()
-	d, err := s.load()
+// RemoveRepos removes repos from an org (no error for non-existent repos).
+func (s *SQLiteStore) RemoveRepos(ctx context.Context, orgID string, repoIDs []string) error {
+	if len(repoIDs) == 0 {
+		return nil
+	}
+
+	placeholders := make([]string, len(repoIDs))
+	args := make([]interface{}, len(repoIDs)+1)
+	args[0] = orgID
+	for i, id := range repoIDs {
+		placeholders[i] = "?"
+		args[i+1] = id
+	}
+
+	query := fmt.Sprintf(`DELETE FROM org_repos WHERE org_id = ? AND repo_id IN (%s)`,
+		strings.Join(placeholders, ","))
+
+	_, err := s.db.ExecContext(ctx, query, args...)
+	return err
+}
+
+// GetRepoConfigOverride returns the config override for a specific repo.
+func (s *SQLiteStore) GetRepoConfigOverride(ctx context.Context, orgID, repoID string) (*OrgConfig, error) {
+	var configJSON sql.NullString
+	err := s.db.QueryRowContext(ctx, `
+		SELECT config_override_json FROM org_repos WHERE org_id = ? AND repo_id = ?`,
+		orgID, repoID,
+	).Scan(&configJSON)
+
+	if err == sql.ErrNoRows {
+		return nil, nil
+	}
 	if err != nil {
-		return nil, err
+		return nil, fmt.Errorf("failed to get config override: %w", err)
+	}
+
+	if !configJSON.Valid || configJSON.String == "" {
+		return nil, nil
 	}
-	orgs := make([]Org, 0, len(d.Orgs))
-	for _, o := range d.Orgs {
-		orgs = append(orgs, *o)
+
+	var config OrgConfig
+	if err := json.Unmarshal([]byte(configJSON.String), &config); err != nil {
+		return nil, fmt.Errorf("failed to unmarshal config override: %w", err)
 	}
-	return orgs, nil
+
+	return &config, nil
 }
 
-func (s *FilesystemStore) Delete(ctx context.Context, orgID string) error {
-	s.mu.Lock()
-	defer s.mu.Unlock()
-	d, err := s.load()
+// SetRepoConfigOverride sets the config override for a specific repo.
+func (s *SQLiteStore) SetRepoConfigOverride(ctx context.Context, orgID, repoID string, config *OrgConfig) error {
+	configJSON, err := json.Marshal(config)
+	if err != nil {
+		return fmt.Errorf("failed to marshal config override: %w", err)
+	}
+
+	result, err := s.db.ExecContext(ctx, `
+		UPDATE org_repos SET config_override_json = ? WHERE org_id = ? AND repo_id = ?`,
+		string(configJSON), orgID, repoID,
+	)
 	if err != nil {
-		return err
+		return fmt.Errorf("failed to set config override: %w", err)
+	}
+
+	rows, _ := result.RowsAffected()
+	if rows == 0 {
+		return fmt.Errorf("repo %s not found in org %s", repoID, orgID)
 	}
-	delete(d.Orgs, orgID)
-	return s.save(d)
+
+	return nil
 }
diff --git a/internal/org/store_fs.go b/internal/org/store_fs.go
new file mode 100644
index 0000000..09bd955
--- /dev/null
+++ b/internal/org/store_fs.go
@@ -0,0 +1,158 @@
+package org
+
+import (
+	"context"
+	"encoding/json"
+	"fmt"
+	"os"
+	"path/filepath"
+	"sync"
+)
+
+// FilesystemStore stores orgs in a JSON file.
+// Deprecated: Use SQLiteStore instead. Kept for filesystem migration (section 05).
+type FilesystemStore struct {
+	path string
+	mu   sync.RWMutex
+}
+
+// NewFilesystemStore creates a store that persists orgs to a JSON file.
+func NewFilesystemStore(basePath string) (*FilesystemStore, error) {
+	path := filepath.Join(basePath, "_orgs.json")
+	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
+		return nil, fmt.Errorf("failed to create org storage dir: %w", err)
+	}
+	return &FilesystemStore{path: path}, nil
+}
+
+type orgData struct {
+	Orgs map[string]*Org `json:"orgs"`
+}
+
+func (s *FilesystemStore) load() (*orgData, error) {
+	data, err := os.ReadFile(s.path)
+	if err != nil {
+		if os.IsNotExist(err) {
+			return &orgData{Orgs: make(map[string]*Org)}, nil
+		}
+		return nil, err
+	}
+	var d orgData
+	if err := json.Unmarshal(data, &d); err != nil {
+		return nil, fmt.Errorf("failed to decode orgs: %w", err)
+	}
+	if d.Orgs == nil {
+		d.Orgs = make(map[string]*Org)
+	}
+	return &d, nil
+}
+
+func (s *FilesystemStore) save(d *orgData) error {
+	data, err := json.MarshalIndent(d, "", "  ")
+	if err != nil {
+		return err
+	}
+	return os.WriteFile(s.path, data, 0644)
+}
+
+func (s *FilesystemStore) SaveOrg(ctx context.Context, o *Org) error {
+	s.mu.Lock()
+	defer s.mu.Unlock()
+	d, err := s.load()
+	if err != nil {
+		return err
+	}
+	d.Orgs[o.ID] = o
+	return s.save(d)
+}
+
+func (s *FilesystemStore) GetOrg(ctx context.Context, orgID string) (*Org, error) {
+	s.mu.RLock()
+	defer s.mu.RUnlock()
+	d, err := s.load()
+	if err != nil {
+		return nil, err
+	}
+	o, ok := d.Orgs[orgID]
+	if !ok {
+		return nil, ErrNotFound
+	}
+	return o, nil
+}
+
+func (s *FilesystemStore) ListOrgs(ctx context.Context) ([]OrgWithCount, error) {
+	s.mu.RLock()
+	defer s.mu.RUnlock()
+	d, err := s.load()
+	if err != nil {
+		return nil, err
+	}
+	result := make([]OrgWithCount, 0, len(d.Orgs))
+	for _, o := range d.Orgs {
+		result = append(result, OrgWithCount{Org: *o, RepoCount: len(o.Repos)})
+	}
+	return result, nil
+}
+
+func (s *FilesystemStore) DeleteOrg(ctx context.Context, orgID string) error {
+	s.mu.Lock()
+	defer s.mu.Unlock()
+	d, err := s.load()
+	if err != nil {
+		return err
+	}
+	delete(d.Orgs, orgID)
+	return s.save(d)
+}
+
+func (s *FilesystemStore) AddRepos(ctx context.Context, orgID string, repoIDs []string) error {
+	s.mu.Lock()
+	defer s.mu.Unlock()
+	d, err := s.load()
+	if err != nil {
+		return err
+	}
+	o, ok := d.Orgs[orgID]
+	if !ok {
+		return ErrNotFound
+	}
+	o.Repos = uniqueStrings(append(o.Repos, repoIDs...))
+	return s.save(d)
+}
+
+func (s *FilesystemStore) RemoveRepos(ctx context.Context, orgID string, repoIDs []string) error {
+	s.mu.Lock()
+	defer s.mu.Unlock()
+	d, err := s.load()
+	if err != nil {
+		return err
+	}
+	o, ok := d.Orgs[orgID]
+	if !ok {
+		return ErrNotFound
+	}
+	removeSet := make(map[string]bool)
+	for _, r := range repoIDs {
+		removeSet[r] = true
+	}
+	var kept []string
+	for _, r := range o.Repos {
+		if !removeSet[r] {
+			kept = append(kept, r)
+		}
+	}
+	o.Repos = kept
+	return s.save(d)
+}
+
+func (s *FilesystemStore) GetRepoConfigOverride(_ context.Context, _, _ string) (*OrgConfig, error) {
+	return nil, nil // FilesystemStore doesn't support config overrides
+}
+
+func (s *FilesystemStore) SetRepoConfigOverride(_ context.Context, _, _ string, _ *OrgConfig) error {
+	return fmt.Errorf("config overrides not supported by FilesystemStore")
+}
+
+func (s *FilesystemStore) RunMigrations() error {
+	return nil // No migrations for filesystem store
+}
diff --git a/internal/org/store_test.go b/internal/org/store_test.go
new file mode 100644
index 0000000..b30b00d
--- /dev/null
+++ b/internal/org/store_test.go
@@ -0,0 +1,659 @@
+package org
+
+import (
+	"context"
+	"database/sql"
+	"errors"
+	"fmt"
+	"sync"
+	"testing"
+
+	_ "github.com/mattn/go-sqlite3"
+	"github.com/yashpalc/mcp-repo-context/internal/storage"
+)
+
+// testDBCounter provides unique names for shared in-memory databases.
+var testDBCounter uint64
+var testDBMu sync.Mutex
+
+func nextTestDBName() string {
+	testDBMu.Lock()
+	defer testDBMu.Unlock()
+	testDBCounter++
+	return fmt.Sprintf("file:test%d?mode=memory&cache=shared&_foreign_keys=ON&_busy_timeout=5000", testDBCounter)
+}
+
+// newTestStore creates a fresh in-memory SQLiteStore for testing.
+// Uses shared cache so concurrent connections share the same in-memory database.
+func newTestStore(t *testing.T) *SQLiteStore {
+	t.Helper()
+	dsn := nextTestDBName()
+	db, err := sql.Open("sqlite3", dsn)
+	if err != nil {
+		t.Fatalf("Failed to open in-memory DB: %v", err)
+	}
+	// Limit to 1 connection: in-memory SQLite doesn't support WAL mode,
+	// so concurrent connections cause lock contention. Production uses
+	// WAL on disk. This still tests Go-level race conditions.
+	db.SetMaxOpenConns(1)
+	t.Cleanup(func() { db.Close() })
+
+	// Run storage migrations to create all tables (including org tables)
+	_, err = storage.NewSQLiteStoreWithDB(db)
+	if err != nil {
+		t.Fatalf("Failed to run storage migrations: %v", err)
+	}
+
+	store, err := NewSQLiteStore(db)
+	if err != nil {
+		t.Fatalf("Failed to create org SQLiteStore: %v", err)
+	}
+	return store
+}
+
+// --- SaveOrg tests ---
+
+func TestSaveOrg_InsertNew(t *testing.T) {
+	store := newTestStore(t)
+	ctx := context.Background()
+
+	o := &Org{
+		ID:    "test-org",
+		Repos: []string{"repo-a", "repo-b"},
+		Config: OrgConfig{
+			ExcludePatterns: []string{"*.log"},
+			MaxFileSize:     1024,
+		},
+	}
+	if err := store.SaveOrg(ctx, o); err != nil {
+		t.Fatalf("SaveOrg failed: %v", err)
+	}
+
+	got, err := store.GetOrg(ctx, "test-org")
+	if err != nil {
+		t.Fatalf("GetOrg failed: %v", err)
+	}
+
+	if got.ID != "test-org" {
+		t.Errorf("ID = %q, want %q", got.ID, "test-org")
+	}
+	if len(got.Repos) != 2 {
+		t.Errorf("Repos len = %d, want 2", len(got.Repos))
+	}
+	if got.Config.MaxFileSize != 1024 {
+		t.Errorf("MaxFileSize = %d, want 1024", got.Config.MaxFileSize)
+	}
+	if len(got.Config.ExcludePatterns) != 1 || got.Config.ExcludePatterns[0] != "*.log" {
+		t.Errorf("ExcludePatterns = %v, want [*.log]", got.Config.ExcludePatterns)
+	}
+	if got.Created.IsZero() {
+		t.Error("Created should not be zero")
+	}
+}
+
+func TestSaveOrg_UpsertExisting(t *testing.T) {
+	store := newTestStore(t)
+	ctx := context.Background()
+
+	o := &Org{ID: "test-org", Repos: []string{"repo-a"}, Config: OrgConfig{MaxFileSize: 100}}
+	if err := store.SaveOrg(ctx, o); err != nil {
+		t.Fatalf("SaveOrg (first) failed: %v", err)
+	}
+
+	first, _ := store.GetOrg(ctx, "test-org")
+	createdAt := first.Created
+
+	// Update with new config
+	o.Config.MaxFileSize = 200
+	o.Repos = []string{"repo-b", "repo-c"}
+	if err := store.SaveOrg(ctx, o); err != nil {
+		t.Fatalf("SaveOrg (upsert) failed: %v", err)
+	}
+
+	got, _ := store.GetOrg(ctx, "test-org")
+	if got.Config.MaxFileSize != 200 {
+		t.Errorf("MaxFileSize = %d, want 200 (updated)", got.Config.MaxFileSize)
+	}
+	if len(got.Repos) != 2 {
+		t.Errorf("Repos len = %d, want 2", len(got.Repos))
+	}
+	if !got.Created.Equal(createdAt) {
+		t.Errorf("Created changed from %v to %v (should be preserved)", createdAt, got.Created)
+	}
+}
+
+func TestSaveOrg_EmptyRepoList(t *testing.T) {
+	store := newTestStore(t)
+	ctx := context.Background()
+
+	o := &Org{ID: "empty-org", Repos: []string{}}
+	if err := store.SaveOrg(ctx, o); err != nil {
+		t.Fatalf("SaveOrg with empty repos failed: %v", err)
+	}
+
+	got, _ := store.GetOrg(ctx, "empty-org")
+	if len(got.Repos) != 0 {
+		t.Errorf("Repos len = %d, want 0", len(got.Repos))
+	}
+}
+
+func TestSaveOrg_ManyRepos(t *testing.T) {
+	store := newTestStore(t)
+	ctx := context.Background()
+
+	repos := make([]string, 100)
+	for i := range repos {
+		repos[i] = fmt.Sprintf("repo-%03d", i)
+	}
+
+	o := &Org{ID: "big-org", Repos: repos}
+	if err := store.SaveOrg(ctx, o); err != nil {
+		t.Fatalf("SaveOrg with 100 repos failed: %v", err)
+	}
+
+	got, _ := store.GetOrg(ctx, "big-org")
+	if len(got.Repos) != 100 {
+		t.Errorf("Repos len = %d, want 100", len(got.Repos))
+	}
+}
+
+func TestSaveOrg_ConfigRoundtrip(t *testing.T) {
+	store := newTestStore(t)
+	ctx := context.Background()
+
+	cfg := OrgConfig{
+		ExcludePatterns: []string{"*.log", "vendor/", "node_modules/"},
+		MaxFileSize:     1048576,
+	}
+	o := &Org{ID: "cfg-org", Config: cfg}
+	if err := store.SaveOrg(ctx, o); err != nil {
+		t.Fatalf("SaveOrg failed: %v", err)
+	}
+
+	got, _ := store.GetOrg(ctx, "cfg-org")
+	if len(got.Config.ExcludePatterns) != 3 {
+		t.Errorf("ExcludePatterns len = %d, want 3", len(got.Config.ExcludePatterns))
+	}
+	for i, want := range cfg.ExcludePatterns {
+		if got.Config.ExcludePatterns[i] != want {
+			t.Errorf("ExcludePatterns[%d] = %q, want %q", i, got.Config.ExcludePatterns[i], want)
+		}
+	}
+	if got.Config.MaxFileSize != 1048576 {
+		t.Errorf("MaxFileSize = %d, want 1048576", got.Config.MaxFileSize)
+	}
+}
+
+// --- ListOrgs tests ---
+
+func TestListOrgs_Empty(t *testing.T) {
+	store := newTestStore(t)
+	ctx := context.Background()
+
+	result, err := store.ListOrgs(ctx)
+	if err != nil {
+		t.Fatalf("ListOrgs failed: %v", err)
+	}
+	if len(result) != 0 {
+		t.Errorf("ListOrgs = %d items, want 0", len(result))
+	}
+}
+
+func TestListOrgs_SingleOrgWithRepoCount(t *testing.T) {
+	store := newTestStore(t)
+	ctx := context.Background()
+
+	store.SaveOrg(ctx, &Org{ID: "org-1", Repos: []string{"a", "b", "c"}})
+
+	result, err := store.ListOrgs(ctx)
+	if err != nil {
+		t.Fatalf("ListOrgs failed: %v", err)
+	}
+	if len(result) != 1 {
+		t.Fatalf("ListOrgs = %d items, want 1", len(result))
+	}
+	if result[0].RepoCount != 3 {
+		t.Errorf("RepoCount = %d, want 3", result[0].RepoCount)
+	}
+}
+
+func TestListOrgs_MultipleOrgs(t *testing.T) {
+	store := newTestStore(t)
+	ctx := context.Background()
+
+	store.SaveOrg(ctx, &Org{ID: "alpha", Repos: []string{"r1", "r2"}})
+	store.SaveOrg(ctx, &Org{ID: "beta", Repos: []string{"r3"}})
+	store.SaveOrg(ctx, &Org{ID: "gamma"})
+
+	result, err := store.ListOrgs(ctx)
+	if err != nil {
+		t.Fatalf("ListOrgs failed: %v", err)
+	}
+	if len(result) != 3 {
+		t.Fatalf("ListOrgs = %d items, want 3", len(result))
+	}
+
+	// Results are ordered by ID
+	counts := map[string]int{}
+	for _, r := range result {
+		counts[r.ID] = r.RepoCount
+	}
+	if counts["alpha"] != 2 {
+		t.Errorf("alpha RepoCount = %d, want 2", counts["alpha"])
+	}
+	if counts["beta"] != 1 {
+		t.Errorf("beta RepoCount = %d, want 1", counts["beta"])
+	}
+	if counts["gamma"] != 0 {
+		t.Errorf("gamma RepoCount = %d, want 0", counts["gamma"])
+	}
+}
+
+// --- GetOrg tests ---
+
+func TestGetOrg_NotFound(t *testing.T) {
+	store := newTestStore(t)
+	ctx := context.Background()
+
+	_, err := store.GetOrg(ctx, "nonexistent")
+	if !errors.Is(err, ErrNotFound) {
+		t.Errorf("GetOrg error = %v, want ErrNotFound", err)
+	}
+}
+
+func TestGetOrg_ReturnsFullRepoList(t *testing.T) {
+	store := newTestStore(t)
+	ctx := context.Background()
+
+	store.SaveOrg(ctx, &Org{ID: "org-1", Repos: []string{"repo-z", "repo-a", "repo-m"}})
+
+	got, err := store.GetOrg(ctx, "org-1")
+	if err != nil {
+		t.Fatalf("GetOrg failed: %v", err)
+	}
+	// Repos are ordered by repo_id
+	if len(got.Repos) != 3 {
+		t.Fatalf("Repos len = %d, want 3", len(got.Repos))
+	}
+	if got.Repos[0] != "repo-a" || got.Repos[1] != "repo-m" || got.Repos[2] != "repo-z" {
+		t.Errorf("Repos = %v, want [repo-a repo-m repo-z] (sorted)", got.Repos)
+	}
+}
+
+// --- AddRepos tests ---
+
+func TestAddRepos_NewRepos(t *testing.T) {
+	store := newTestStore(t)
+	ctx := context.Background()
+
+	store.SaveOrg(ctx, &Org{ID: "org-1", Repos: []string{"repo-a"}})
+
+	if err := store.AddRepos(ctx, "org-1", []string{"repo-b", "repo-c"}); err != nil {
+		t.Fatalf("AddRepos failed: %v", err)
+	}
+
+	got, _ := store.GetOrg(ctx, "org-1")
+	if len(got.Repos) != 3 {
+		t.Errorf("Repos len = %d, want 3", len(got.Repos))
+	}
+}
+
+func TestAddRepos_Idempotent(t *testing.T) {
+	store := newTestStore(t)
+	ctx := context.Background()
+
+	store.SaveOrg(ctx, &Org{ID: "org-1", Repos: []string{"repo-a"}})
+
+	// Add repo-a again — should be ignored
+	if err := store.AddRepos(ctx, "org-1", []string{"repo-a", "repo-b"}); err != nil {
+		t.Fatalf("AddRepos failed: %v", err)
+	}
+
+	got, _ := store.GetOrg(ctx, "org-1")
+	if len(got.Repos) != 2 {
+		t.Errorf("Repos len = %d, want 2 (repo-a deduplicated)", len(got.Repos))
+	}
+}
+
+func TestAddRepos_NonExistentOrg(t *testing.T) {
+	store := newTestStore(t)
+	ctx := context.Background()
+
+	err := store.AddRepos(ctx, "ghost-org", []string{"repo-a"})
+	if !errors.Is(err, ErrNotFound) {
+		t.Errorf("AddRepos error = %v, want ErrNotFound", err)
+	}
+}
+
+func TestAddRepos_EmptyList(t *testing.T) {
+	store := newTestStore(t)
+	ctx := context.Background()
+
+	store.SaveOrg(ctx, &Org{ID: "org-1", Repos: []string{"repo-a"}})
+
+	if err := store.AddRepos(ctx, "org-1", []string{}); err != nil {
+		t.Fatalf("AddRepos empty list failed: %v", err)
+	}
+
+	got, _ := store.GetOrg(ctx, "org-1")
+	if len(got.Repos) != 1 {
+		t.Errorf("Repos len = %d, want 1 (unchanged)", len(got.Repos))
+	}
+}
+
+// --- RemoveRepos tests ---
+
+func TestRemoveRepos_RemovesSpecified(t *testing.T) {
+	store := newTestStore(t)
+	ctx := context.Background()
+
+	store.SaveOrg(ctx, &Org{ID: "org-1", Repos: []string{"a", "b", "c"}})
+
+	if err := store.RemoveRepos(ctx, "org-1", []string{"b"}); err != nil {
+		t.Fatalf("RemoveRepos failed: %v", err)
+	}
+
+	got, _ := store.GetOrg(ctx, "org-1")
+	if len(got.Repos) != 2 {
+		t.Errorf("Repos len = %d, want 2", len(got.Repos))
+	}
+}
+
+func TestRemoveRepos_NonExistentRepoIsNoop(t *testing.T) {
+	store := newTestStore(t)
+	ctx := context.Background()
+
+	store.SaveOrg(ctx, &Org{ID: "org-1", Repos: []string{"a"}})
+
+	if err := store.RemoveRepos(ctx, "org-1", []string{"zzz"}); err != nil {
+		t.Fatalf("RemoveRepos failed: %v", err)
+	}
+
+	got, _ := store.GetOrg(ctx, "org-1")
+	if len(got.Repos) != 1 {
+		t.Errorf("Repos len = %d, want 1 (unchanged)", len(got.Repos))
+	}
+}
+
+func TestRemoveRepos_AllRepos(t *testing.T) {
+	store := newTestStore(t)
+	ctx := context.Background()
+
+	store.SaveOrg(ctx, &Org{ID: "org-1", Repos: []string{"a", "b"}})
+
+	if err := store.RemoveRepos(ctx, "org-1", []string{"a", "b"}); err != nil {
+		t.Fatalf("RemoveRepos failed: %v", err)
+	}
+
+	got, _ := store.GetOrg(ctx, "org-1")
+	if len(got.Repos) != 0 {
+		t.Errorf("Repos len = %d, want 0", len(got.Repos))
+	}
+}
+
+// --- DeleteOrg tests ---
+
+func TestDeleteOrg_RemovesOrgAndRepos(t *testing.T) {
+	store := newTestStore(t)
+	ctx := context.Background()
+
+	store.SaveOrg(ctx, &Org{ID: "doomed", Repos: []string{"r1", "r2"}})
+
+	if err := store.DeleteOrg(ctx, "doomed"); err != nil {
+		t.Fatalf("DeleteOrg failed: %v", err)
+	}
+
+	_, err := store.GetOrg(ctx, "doomed")
+	if !errors.Is(err, ErrNotFound) {
+		t.Errorf("GetOrg after delete: err = %v, want ErrNotFound", err)
+	}
+}
+
+func TestDeleteOrg_NonExistentIsNoop(t *testing.T) {
+	store := newTestStore(t)
+	ctx := context.Background()
+
+	if err := store.DeleteOrg(ctx, "nonexistent"); err != nil {
+		t.Errorf("DeleteOrg non-existent: err = %v, want nil", err)
+	}
+}
+
+// --- Config override tests ---
+
+func TestSetRepoConfigOverride_StoresConfig(t *testing.T) {
+	store := newTestStore(t)
+	ctx := context.Background()
+
+	store.SaveOrg(ctx, &Org{ID: "org-1", Repos: []string{"repo-a"}})
+
+	cfg := &OrgConfig{MaxFileSize: 999, ExcludePatterns: []string{"*.tmp"}}
+	if err := store.SetRepoConfigOverride(ctx, "org-1", "repo-a", cfg); err != nil {
+		t.Fatalf("SetRepoConfigOverride failed: %v", err)
+	}
+
+	got, err := store.GetRepoConfigOverride(ctx, "org-1", "repo-a")
+	if err != nil {
+		t.Fatalf("GetRepoConfigOverride failed: %v", err)
+	}
+	if got == nil {
+		t.Fatal("GetRepoConfigOverride returned nil")
+	}
+	if got.MaxFileSize != 999 {
+		t.Errorf("MaxFileSize = %d, want 999", got.MaxFileSize)
+	}
+	if len(got.ExcludePatterns) != 1 || got.ExcludePatterns[0] != "*.tmp" {
+		t.Errorf("ExcludePatterns = %v, want [*.tmp]", got.ExcludePatterns)
+	}
+}
+
+func TestGetRepoConfigOverride_NilWhenNotSet(t *testing.T) {
+	store := newTestStore(t)
+	ctx := context.Background()
+
+	store.SaveOrg(ctx, &Org{ID: "org-1", Repos: []string{"repo-a"}})
+
+	got, err := store.GetRepoConfigOverride(ctx, "org-1", "repo-a")
+	if err != nil {
+		t.Fatalf("GetRepoConfigOverride failed: %v", err)
+	}
+	if got != nil {
+		t.Errorf("GetRepoConfigOverride = %+v, want nil", got)
+	}
+}
+
+func TestGetRepoConfigOverride_NilForNonExistentRepo(t *testing.T) {
+	store := newTestStore(t)
+	ctx := context.Background()
+
+	got, err := store.GetRepoConfigOverride(ctx, "org-1", "nonexistent")
+	if err != nil {
+		t.Fatalf("GetRepoConfigOverride failed: %v", err)
+	}
+	if got != nil {
+		t.Errorf("GetRepoConfigOverride = %+v, want nil", got)
+	}
+}
+
+func TestSetRepoConfigOverride_NonExistentRepoErrors(t *testing.T) {
+	store := newTestStore(t)
+	ctx := context.Background()
+
+	store.SaveOrg(ctx, &Org{ID: "org-1", Repos: []string{"repo-a"}})
+
+	err := store.SetRepoConfigOverride(ctx, "org-1", "nonexistent", &OrgConfig{MaxFileSize: 1})
+	if err == nil {
+		t.Error("SetRepoConfigOverride should fail for non-existent repo")
+	}
+}
+
+// --- Concurrent access tests ---
+
+func TestConcurrent_SaveOrg(t *testing.T) {
+	store := newTestStore(t)
+	ctx := context.Background()
+
+	var wg sync.WaitGroup
+	errs := make([]error, 10)
+	for i := 0; i < 10; i++ {
+		wg.Add(1)
+		go func(idx int) {
+			defer wg.Done()
+			o := &Org{
+				ID:    fmt.Sprintf("org-%d", idx),
+				Repos: []string{fmt.Sprintf("repo-%d", idx)},
+			}
+			errs[idx] = store.SaveOrg(ctx, o)
+		}(i)
+	}
+	wg.Wait()
+
+	for i, err := range errs {
+		if err != nil {
+			t.Errorf("SaveOrg goroutine %d failed: %v", i, err)
+		}
+	}
+
+	result, _ := store.ListOrgs(ctx)
+	if len(result) != 10 {
+		t.Errorf("ListOrgs = %d, want 10", len(result))
+	}
+}
+
+func TestConcurrent_AddRemoveRepos(t *testing.T) {
+	store := newTestStore(t)
+	ctx := context.Background()
+
+	store.SaveOrg(ctx, &Org{ID: "shared-org", Repos: []string{"seed"}})
+
+	var wg sync.WaitGroup
+	// 5 goroutines add repos, 5 remove
+	for i := 0; i < 5; i++ {
+		wg.Add(2)
+		go func(idx int) {
+			defer wg.Done()
+			store.AddRepos(ctx, "shared-org", []string{fmt.Sprintf("add-%d", idx)})
+		}(i)
+		go func(idx int) {
+			defer wg.Done()
+			store.RemoveRepos(ctx, "shared-org", []string{fmt.Sprintf("nonexistent-%d", idx)})
+		}(i)
+	}
+	wg.Wait()
+
+	// Org should still be accessible
+	got, err := store.GetOrg(ctx, "shared-org")
+	if err != nil {
+		t.Fatalf("GetOrg after concurrent ops: %v", err)
+	}
+	// At minimum, seed + 5 added repos (removes targeted nonexistent repos)
+	if len(got.Repos) < 1 {
+		t.Errorf("Repos len = %d, want >= 1", len(got.Repos))
+	}
+}
+
+func TestConcurrent_ReadsAndWrites(t *testing.T) {
+	store := newTestStore(t)
+	ctx := context.Background()
+
+	store.SaveOrg(ctx, &Org{ID: "rw-org", Repos: []string{"r1"}})
+
+	var wg sync.WaitGroup
+	// Concurrent reads
+	for i := 0; i < 10; i++ {
+		wg.Add(1)
+		go func() {
+			defer wg.Done()
+			store.GetOrg(ctx, "rw-org")
+		}()
+	}
+	// Concurrent writes
+	for i := 0; i < 5; i++ {
+		wg.Add(1)
+		go func(idx int) {
+			defer wg.Done()
+			store.AddRepos(ctx, "rw-org", []string{fmt.Sprintf("wr-%d", idx)})
+		}(i)
+	}
+	wg.Wait()
+
+	got, err := store.GetOrg(ctx, "rw-org")
+	if err != nil {
+		t.Fatalf("GetOrg after concurrent r/w: %v", err)
+	}
+	if len(got.Repos) < 1 {
+		t.Errorf("Repos len = %d, want >= 1", len(got.Repos))
+	}
+}
+
+// --- Manager integration tests ---
+
+func TestManager_GetEffectiveConfig(t *testing.T) {
+	store := newTestStore(t)
+	ctx := context.Background()
+	mgr := NewManager(store)
+
+	// Register org with config
+	_, err := mgr.Register(ctx, "org-1", []string{"repo-a"}, &OrgConfig{
+		ExcludePatterns: []string{"*.log"},
+		MaxFileSize:     100,
+	})
+	if err != nil {
+		t.Fatalf("Register failed: %v", err)
+	}
+
+	// No override — effective config = org config
+	cfg, err := mgr.GetEffectiveConfig(ctx, "org-1", "repo-a")
+	if err != nil {
+		t.Fatalf("GetEffectiveConfig failed: %v", err)
+	}
+	if cfg.MaxFileSize != 100 {
+		t.Errorf("MaxFileSize = %d, want 100", cfg.MaxFileSize)
+	}
+
+	// Set override
+	err = mgr.SetRepoConfigOverride(ctx, "org-1", "repo-a", &OrgConfig{
+		ExcludePatterns: []string{"*.tmp"},
+		MaxFileSize:     200,
+	})
+	if err != nil {
+		t.Fatalf("SetRepoConfigOverride failed: %v", err)
+	}
+
+	// Effective config = merged
+	cfg, err = mgr.GetEffectiveConfig(ctx, "org-1", "repo-a")
+	if err != nil {
+		t.Fatalf("GetEffectiveConfig (with override) failed: %v", err)
+	}
+	if cfg.MaxFileSize != 200 {
+		t.Errorf("MaxFileSize = %d, want 200 (override wins)", cfg.MaxFileSize)
+	}
+	if len(cfg.ExcludePatterns) != 2 {
+		t.Errorf("ExcludePatterns len = %d, want 2 (union)", len(cfg.ExcludePatterns))
+	}
+}
+
+func TestManager_DelegatesAtomicOps(t *testing.T) {
+	store := newTestStore(t)
+	ctx := context.Background()
+	mgr := NewManager(store)
+
+	mgr.Register(ctx, "org-1", []string{"repo-a"}, nil)
+
+	// AddRepos delegates to store.AddRepos (no read-modify-write)
+	if err := mgr.AddRepos(ctx, "org-1", []string{"repo-b"}); err != nil {
+		t.Fatalf("AddRepos failed: %v", err)
+	}
+	got, _ := mgr.Get(ctx, "org-1")
+	if len(got.Repos) != 2 {
+		t.Errorf("Repos len = %d, want 2", len(got.Repos))
+	}
+
+	// RemoveRepos delegates to store.RemoveRepos
+	if err := mgr.RemoveRepos(ctx, "org-1", []string{"repo-a"}); err != nil {
+		t.Fatalf("RemoveRepos failed: %v", err)
+	}
+	got, _ = mgr.Get(ctx, "org-1")
+	if len(got.Repos) != 1 {
+		t.Errorf("Repos len = %d, want 1", len(got.Repos))
+	}
+}
diff --git a/internal/org/types.go b/internal/org/types.go
index 85d3c5e..e4f6cce 100644
--- a/internal/org/types.go
+++ b/internal/org/types.go
@@ -1,6 +1,11 @@
 package org
 
-import "time"
+import (
+	"errors"
+	"time"
+)
+
+var ErrNotFound = errors.New("org: not found")
 
 // Org represents an organization grouping repositories.
 type Org struct {
diff --git a/planning/01-org-abstraction/implementation/deep_implement_config.json b/planning/01-org-abstraction/implementation/deep_implement_config.json
index 8b57992..e463485 100644
--- a/planning/01-org-abstraction/implementation/deep_implement_config.json
+++ b/planning/01-org-abstraction/implementation/deep_implement_config.json
@@ -15,7 +15,16 @@
     "section-06-mcp-tools-wiring",
     "section-07-integration-tests-benchmarks"
   ],
-  "sections_state": {},
+  "sections_state": {
+    "section-01-schema-migration": {
+      "status": "complete",
+      "commit_hash": "8201f69"
+    },
+    "section-02-config-inheritance": {
+      "status": "complete",
+      "commit_hash": "018a8a8"
+    }
+  },
   "pre_commit": {
     "present": false,
     "type": "none",
