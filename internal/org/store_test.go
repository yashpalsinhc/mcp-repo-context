package org

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"testing"

	_ "github.com/mattn/go-sqlite3"
	"github.com/yashpalc/mcp-repo-context/internal/storage"
)

// testDBCounter provides unique names for shared in-memory databases.
var testDBCounter uint64
var testDBMu sync.Mutex

func nextTestDBName() string {
	testDBMu.Lock()
	defer testDBMu.Unlock()
	testDBCounter++
	return fmt.Sprintf("file:test%d?mode=memory&cache=shared&_foreign_keys=ON&_busy_timeout=5000", testDBCounter)
}

// newTestStore creates a fresh in-memory SQLiteStore for testing.
// Uses shared cache so concurrent connections share the same in-memory database.
func newTestStore(t *testing.T) *SQLiteStore {
	t.Helper()
	dsn := nextTestDBName()
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		t.Fatalf("Failed to open in-memory DB: %v", err)
	}
	// Limit to 1 connection: in-memory SQLite doesn't support WAL mode,
	// so concurrent connections cause lock contention. Production uses
	// WAL on disk. This still tests Go-level race conditions.
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })

	// Run storage migrations to create all tables (including org tables)
	_, err = storage.NewSQLiteStoreWithDB(db)
	if err != nil {
		t.Fatalf("Failed to run storage migrations: %v", err)
	}

	store, err := NewSQLiteStore(db)
	if err != nil {
		t.Fatalf("Failed to create org SQLiteStore: %v", err)
	}
	return store
}

// --- SaveOrg tests ---

func TestSaveOrg_InsertNew(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	o := &Org{
		ID:    "test-org",
		Repos: []string{"repo-a", "repo-b"},
		Config: OrgConfig{
			ExcludePatterns: []string{"*.log"},
			MaxFileSize:     1024,
		},
	}
	if err := store.SaveOrg(ctx, o); err != nil {
		t.Fatalf("SaveOrg failed: %v", err)
	}

	got, err := store.GetOrg(ctx, "test-org")
	if err != nil {
		t.Fatalf("GetOrg failed: %v", err)
	}

	if got.ID != "test-org" {
		t.Errorf("ID = %q, want %q", got.ID, "test-org")
	}
	if len(got.Repos) != 2 {
		t.Errorf("Repos len = %d, want 2", len(got.Repos))
	}
	if got.Config.MaxFileSize != 1024 {
		t.Errorf("MaxFileSize = %d, want 1024", got.Config.MaxFileSize)
	}
	if len(got.Config.ExcludePatterns) != 1 || got.Config.ExcludePatterns[0] != "*.log" {
		t.Errorf("ExcludePatterns = %v, want [*.log]", got.Config.ExcludePatterns)
	}
	if got.Created.IsZero() {
		t.Error("Created should not be zero")
	}
}

func TestSaveOrg_UpsertExisting(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	o := &Org{ID: "test-org", Repos: []string{"repo-a"}, Config: OrgConfig{MaxFileSize: 100}}
	if err := store.SaveOrg(ctx, o); err != nil {
		t.Fatalf("SaveOrg (first) failed: %v", err)
	}

	first, _ := store.GetOrg(ctx, "test-org")
	createdAt := first.Created

	// Update with new config
	o.Config.MaxFileSize = 200
	o.Repos = []string{"repo-b", "repo-c"}
	if err := store.SaveOrg(ctx, o); err != nil {
		t.Fatalf("SaveOrg (upsert) failed: %v", err)
	}

	got, _ := store.GetOrg(ctx, "test-org")
	if got.Config.MaxFileSize != 200 {
		t.Errorf("MaxFileSize = %d, want 200 (updated)", got.Config.MaxFileSize)
	}
	if len(got.Repos) != 2 {
		t.Errorf("Repos len = %d, want 2", len(got.Repos))
	}
	if !got.Created.Equal(createdAt) {
		t.Errorf("Created changed from %v to %v (should be preserved)", createdAt, got.Created)
	}
}

func TestSaveOrg_EmptyRepoList(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	o := &Org{ID: "empty-org", Repos: []string{}}
	if err := store.SaveOrg(ctx, o); err != nil {
		t.Fatalf("SaveOrg with empty repos failed: %v", err)
	}

	got, _ := store.GetOrg(ctx, "empty-org")
	if len(got.Repos) != 0 {
		t.Errorf("Repos len = %d, want 0", len(got.Repos))
	}
}

func TestSaveOrg_NilRepoList(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	o := &Org{ID: "nil-repos-org"} // Repos is nil
	if err := store.SaveOrg(ctx, o); err != nil {
		t.Fatalf("SaveOrg with nil repos failed: %v", err)
	}

	got, _ := store.GetOrg(ctx, "nil-repos-org")
	if len(got.Repos) != 0 {
		t.Errorf("Repos len = %d, want 0", len(got.Repos))
	}
}

func TestSaveOrg_ManyRepos(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	repos := make([]string, 100)
	for i := range repos {
		repos[i] = fmt.Sprintf("repo-%03d", i)
	}

	o := &Org{ID: "big-org", Repos: repos}
	if err := store.SaveOrg(ctx, o); err != nil {
		t.Fatalf("SaveOrg with 100 repos failed: %v", err)
	}

	got, _ := store.GetOrg(ctx, "big-org")
	if len(got.Repos) != 100 {
		t.Errorf("Repos len = %d, want 100", len(got.Repos))
	}
}

func TestSaveOrg_ConfigRoundtrip(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	cfg := OrgConfig{
		ExcludePatterns: []string{"*.log", "vendor/", "node_modules/"},
		MaxFileSize:     1048576,
	}
	o := &Org{ID: "cfg-org", Config: cfg}
	if err := store.SaveOrg(ctx, o); err != nil {
		t.Fatalf("SaveOrg failed: %v", err)
	}

	got, _ := store.GetOrg(ctx, "cfg-org")
	if len(got.Config.ExcludePatterns) != 3 {
		t.Errorf("ExcludePatterns len = %d, want 3", len(got.Config.ExcludePatterns))
	}
	for i, want := range cfg.ExcludePatterns {
		if got.Config.ExcludePatterns[i] != want {
			t.Errorf("ExcludePatterns[%d] = %q, want %q", i, got.Config.ExcludePatterns[i], want)
		}
	}
	if got.Config.MaxFileSize != 1048576 {
		t.Errorf("MaxFileSize = %d, want 1048576", got.Config.MaxFileSize)
	}
}

// --- ListOrgs tests ---

func TestListOrgs_Empty(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	result, err := store.ListOrgs(ctx)
	if err != nil {
		t.Fatalf("ListOrgs failed: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("ListOrgs = %d items, want 0", len(result))
	}
}

func TestListOrgs_SingleOrgWithRepoCount(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	store.SaveOrg(ctx, &Org{ID: "org-1", Repos: []string{"a", "b", "c"}})

	result, err := store.ListOrgs(ctx)
	if err != nil {
		t.Fatalf("ListOrgs failed: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("ListOrgs = %d items, want 1", len(result))
	}
	if result[0].RepoCount != 3 {
		t.Errorf("RepoCount = %d, want 3", result[0].RepoCount)
	}
}

func TestListOrgs_MultipleOrgs(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	store.SaveOrg(ctx, &Org{ID: "alpha", Repos: []string{"r1", "r2"}})
	store.SaveOrg(ctx, &Org{ID: "beta", Repos: []string{"r3"}})
	store.SaveOrg(ctx, &Org{ID: "gamma"})

	result, err := store.ListOrgs(ctx)
	if err != nil {
		t.Fatalf("ListOrgs failed: %v", err)
	}
	if len(result) != 3 {
		t.Fatalf("ListOrgs = %d items, want 3", len(result))
	}

	// Results are ordered by ID
	counts := map[string]int{}
	for _, r := range result {
		counts[r.ID] = r.RepoCount
	}
	if counts["alpha"] != 2 {
		t.Errorf("alpha RepoCount = %d, want 2", counts["alpha"])
	}
	if counts["beta"] != 1 {
		t.Errorf("beta RepoCount = %d, want 1", counts["beta"])
	}
	if counts["gamma"] != 0 {
		t.Errorf("gamma RepoCount = %d, want 0", counts["gamma"])
	}
}

// --- GetOrg tests ---

func TestGetOrg_NotFound(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	_, err := store.GetOrg(ctx, "nonexistent")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("GetOrg error = %v, want ErrNotFound", err)
	}
}

func TestGetOrg_ReturnsFullRepoList(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	store.SaveOrg(ctx, &Org{ID: "org-1", Repos: []string{"repo-z", "repo-a", "repo-m"}})

	got, err := store.GetOrg(ctx, "org-1")
	if err != nil {
		t.Fatalf("GetOrg failed: %v", err)
	}
	// Repos are ordered by repo_id
	if len(got.Repos) != 3 {
		t.Fatalf("Repos len = %d, want 3", len(got.Repos))
	}
	if got.Repos[0] != "repo-a" || got.Repos[1] != "repo-m" || got.Repos[2] != "repo-z" {
		t.Errorf("Repos = %v, want [repo-a repo-m repo-z] (sorted)", got.Repos)
	}
}

// --- AddRepos tests ---

func TestAddRepos_NewRepos(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	store.SaveOrg(ctx, &Org{ID: "org-1", Repos: []string{"repo-a"}})

	if err := store.AddRepos(ctx, "org-1", []string{"repo-b", "repo-c"}); err != nil {
		t.Fatalf("AddRepos failed: %v", err)
	}

	got, _ := store.GetOrg(ctx, "org-1")
	if len(got.Repos) != 3 {
		t.Errorf("Repos len = %d, want 3", len(got.Repos))
	}
}

func TestAddRepos_Idempotent(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	store.SaveOrg(ctx, &Org{ID: "org-1", Repos: []string{"repo-a"}})

	// Add repo-a again — should be ignored
	if err := store.AddRepos(ctx, "org-1", []string{"repo-a", "repo-b"}); err != nil {
		t.Fatalf("AddRepos failed: %v", err)
	}

	got, _ := store.GetOrg(ctx, "org-1")
	if len(got.Repos) != 2 {
		t.Errorf("Repos len = %d, want 2 (repo-a deduplicated)", len(got.Repos))
	}
}

func TestAddRepos_NonExistentOrg(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	err := store.AddRepos(ctx, "ghost-org", []string{"repo-a"})
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("AddRepos error = %v, want ErrNotFound", err)
	}
}

func TestAddRepos_EmptyList(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	store.SaveOrg(ctx, &Org{ID: "org-1", Repos: []string{"repo-a"}})

	if err := store.AddRepos(ctx, "org-1", []string{}); err != nil {
		t.Fatalf("AddRepos empty list failed: %v", err)
	}

	got, _ := store.GetOrg(ctx, "org-1")
	if len(got.Repos) != 1 {
		t.Errorf("Repos len = %d, want 1 (unchanged)", len(got.Repos))
	}
}

// --- RemoveRepos tests ---

func TestRemoveRepos_RemovesSpecified(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	store.SaveOrg(ctx, &Org{ID: "org-1", Repos: []string{"a", "b", "c"}})

	if err := store.RemoveRepos(ctx, "org-1", []string{"b"}); err != nil {
		t.Fatalf("RemoveRepos failed: %v", err)
	}

	got, _ := store.GetOrg(ctx, "org-1")
	if len(got.Repos) != 2 {
		t.Errorf("Repos len = %d, want 2", len(got.Repos))
	}
}

func TestRemoveRepos_NonExistentRepoIsNoop(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	store.SaveOrg(ctx, &Org{ID: "org-1", Repos: []string{"a"}})

	if err := store.RemoveRepos(ctx, "org-1", []string{"zzz"}); err != nil {
		t.Fatalf("RemoveRepos failed: %v", err)
	}

	got, _ := store.GetOrg(ctx, "org-1")
	if len(got.Repos) != 1 {
		t.Errorf("Repos len = %d, want 1 (unchanged)", len(got.Repos))
	}
}

func TestRemoveRepos_AllRepos(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	store.SaveOrg(ctx, &Org{ID: "org-1", Repos: []string{"a", "b"}})

	if err := store.RemoveRepos(ctx, "org-1", []string{"a", "b"}); err != nil {
		t.Fatalf("RemoveRepos failed: %v", err)
	}

	got, _ := store.GetOrg(ctx, "org-1")
	if len(got.Repos) != 0 {
		t.Errorf("Repos len = %d, want 0", len(got.Repos))
	}
}

// --- DeleteOrg tests ---

func TestDeleteOrg_RemovesOrgAndRepos(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	store.SaveOrg(ctx, &Org{ID: "doomed", Repos: []string{"r1", "r2"}})

	if err := store.DeleteOrg(ctx, "doomed"); err != nil {
		t.Fatalf("DeleteOrg failed: %v", err)
	}

	_, err := store.GetOrg(ctx, "doomed")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("GetOrg after delete: err = %v, want ErrNotFound", err)
	}
}

func TestDeleteOrg_NonExistentIsNoop(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	if err := store.DeleteOrg(ctx, "nonexistent"); err != nil {
		t.Errorf("DeleteOrg non-existent: err = %v, want nil", err)
	}
}

// --- Config override tests ---

func TestSetRepoConfigOverride_StoresConfig(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	store.SaveOrg(ctx, &Org{ID: "org-1", Repos: []string{"repo-a"}})

	cfg := &OrgConfig{MaxFileSize: 999, ExcludePatterns: []string{"*.tmp"}}
	if err := store.SetRepoConfigOverride(ctx, "org-1", "repo-a", cfg); err != nil {
		t.Fatalf("SetRepoConfigOverride failed: %v", err)
	}

	got, err := store.GetRepoConfigOverride(ctx, "org-1", "repo-a")
	if err != nil {
		t.Fatalf("GetRepoConfigOverride failed: %v", err)
	}
	if got == nil {
		t.Fatal("GetRepoConfigOverride returned nil")
	}
	if got.MaxFileSize != 999 {
		t.Errorf("MaxFileSize = %d, want 999", got.MaxFileSize)
	}
	if len(got.ExcludePatterns) != 1 || got.ExcludePatterns[0] != "*.tmp" {
		t.Errorf("ExcludePatterns = %v, want [*.tmp]", got.ExcludePatterns)
	}
}

func TestGetRepoConfigOverride_NilWhenNotSet(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	store.SaveOrg(ctx, &Org{ID: "org-1", Repos: []string{"repo-a"}})

	got, err := store.GetRepoConfigOverride(ctx, "org-1", "repo-a")
	if err != nil {
		t.Fatalf("GetRepoConfigOverride failed: %v", err)
	}
	if got != nil {
		t.Errorf("GetRepoConfigOverride = %+v, want nil", got)
	}
}

func TestGetRepoConfigOverride_NilForNonExistentRepo(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	got, err := store.GetRepoConfigOverride(ctx, "org-1", "nonexistent")
	if err != nil {
		t.Fatalf("GetRepoConfigOverride failed: %v", err)
	}
	if got != nil {
		t.Errorf("GetRepoConfigOverride = %+v, want nil", got)
	}
}

func TestSetRepoConfigOverride_NonExistentRepoErrors(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	store.SaveOrg(ctx, &Org{ID: "org-1", Repos: []string{"repo-a"}})

	err := store.SetRepoConfigOverride(ctx, "org-1", "nonexistent", &OrgConfig{MaxFileSize: 1})
	if err == nil {
		t.Error("SetRepoConfigOverride should fail for non-existent repo")
	}
}

// --- Concurrent access tests ---

func TestConcurrent_SaveOrg(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	var wg sync.WaitGroup
	errs := make([]error, 10)
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			o := &Org{
				ID:    fmt.Sprintf("org-%d", idx),
				Repos: []string{fmt.Sprintf("repo-%d", idx)},
			}
			errs[idx] = store.SaveOrg(ctx, o)
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("SaveOrg goroutine %d failed: %v", i, err)
		}
	}

	result, _ := store.ListOrgs(ctx)
	if len(result) != 10 {
		t.Errorf("ListOrgs = %d, want 10", len(result))
	}
}

func TestConcurrent_AddRemoveRepos(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	store.SaveOrg(ctx, &Org{ID: "shared-org", Repos: []string{"seed"}})

	var wg sync.WaitGroup
	// 5 goroutines add repos, 5 remove
	for i := 0; i < 5; i++ {
		wg.Add(2)
		go func(idx int) {
			defer wg.Done()
			store.AddRepos(ctx, "shared-org", []string{fmt.Sprintf("add-%d", idx)})
		}(i)
		go func(idx int) {
			defer wg.Done()
			store.RemoveRepos(ctx, "shared-org", []string{fmt.Sprintf("nonexistent-%d", idx)})
		}(i)
	}
	wg.Wait()

	// Org should still be accessible
	got, err := store.GetOrg(ctx, "shared-org")
	if err != nil {
		t.Fatalf("GetOrg after concurrent ops: %v", err)
	}
	// seed + 5 added repos = 6 (removes targeted nonexistent repos)
	if len(got.Repos) != 6 {
		t.Errorf("Repos len = %d, want 6", len(got.Repos))
	}
}

func TestConcurrent_ReadsAndWrites(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	store.SaveOrg(ctx, &Org{ID: "rw-org", Repos: []string{"r1"}})

	var wg sync.WaitGroup
	// Concurrent reads
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			store.GetOrg(ctx, "rw-org")
		}()
	}
	// Concurrent writes
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			store.AddRepos(ctx, "rw-org", []string{fmt.Sprintf("wr-%d", idx)})
		}(i)
	}
	wg.Wait()

	got, err := store.GetOrg(ctx, "rw-org")
	if err != nil {
		t.Fatalf("GetOrg after concurrent r/w: %v", err)
	}
	// r1 + 5 concurrent adds = 6
	if len(got.Repos) != 6 {
		t.Errorf("Repos len = %d, want 6", len(got.Repos))
	}
}

// --- Manager integration tests ---

func TestManager_GetEffectiveConfig(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	mgr := NewManager(store)

	// Register org with config
	_, err := mgr.Register(ctx, "org-1", []string{"repo-a"}, &OrgConfig{
		ExcludePatterns: []string{"*.log"},
		MaxFileSize:     100,
	})
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	// No override — effective config = org config
	cfg, err := mgr.GetEffectiveConfig(ctx, "org-1", "repo-a")
	if err != nil {
		t.Fatalf("GetEffectiveConfig failed: %v", err)
	}
	if cfg.MaxFileSize != 100 {
		t.Errorf("MaxFileSize = %d, want 100", cfg.MaxFileSize)
	}

	// Set override
	err = mgr.SetRepoConfigOverride(ctx, "org-1", "repo-a", &OrgConfig{
		ExcludePatterns: []string{"*.tmp"},
		MaxFileSize:     200,
	})
	if err != nil {
		t.Fatalf("SetRepoConfigOverride failed: %v", err)
	}

	// Effective config = merged
	cfg, err = mgr.GetEffectiveConfig(ctx, "org-1", "repo-a")
	if err != nil {
		t.Fatalf("GetEffectiveConfig (with override) failed: %v", err)
	}
	if cfg.MaxFileSize != 200 {
		t.Errorf("MaxFileSize = %d, want 200 (override wins)", cfg.MaxFileSize)
	}
	if len(cfg.ExcludePatterns) != 2 {
		t.Errorf("ExcludePatterns len = %d, want 2 (union)", len(cfg.ExcludePatterns))
	}
}

func TestManager_DelegatesAtomicOps(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	mgr := NewManager(store)

	mgr.Register(ctx, "org-1", []string{"repo-a"}, nil)

	// AddRepos delegates to store.AddRepos (no read-modify-write)
	if err := mgr.AddRepos(ctx, "org-1", []string{"repo-b"}); err != nil {
		t.Fatalf("AddRepos failed: %v", err)
	}
	got, _ := mgr.Get(ctx, "org-1")
	if len(got.Repos) != 2 {
		t.Errorf("Repos len = %d, want 2", len(got.Repos))
	}

	// RemoveRepos delegates to store.RemoveRepos
	if err := mgr.RemoveRepos(ctx, "org-1", []string{"repo-a"}); err != nil {
		t.Fatalf("RemoveRepos failed: %v", err)
	}
	got, _ = mgr.Get(ctx, "org-1")
	if len(got.Repos) != 1 {
		t.Errorf("Repos len = %d, want 1", len(got.Repos))
	}
}
