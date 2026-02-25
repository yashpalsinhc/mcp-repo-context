package storage

import (
	"context"
	"database/sql"
	"os"
	"testing"

	_ "github.com/mattn/go-sqlite3"
	ctxpkg "github.com/yashpalc/mcp-repo-context/internal/context"
)

func createTestStore(t *testing.T) (*SQLiteStore, func()) {
	t.Helper()
	tmpFile, err := os.CreateTemp("", "fts_test_*.db")
	if err != nil {
		t.Fatalf("create temp: %v", err)
	}
	tmpFile.Close()

	store, err := NewSQLiteStore(tmpFile.Name())
	if err != nil {
		os.Remove(tmpFile.Name())
		t.Fatalf("NewSQLiteStore: %v", err)
	}

	cleanup := func() {
		store.Close()
		os.Remove(tmpFile.Name())
	}
	return store, cleanup
}

func makeTestRepoCtx(repoID string, functions map[string][]ctxpkg.FunctionDef) *ctxpkg.RepoContext {
	rc := &ctxpkg.RepoContext{
		ID:    repoID,
		Files: make(map[string]*ctxpkg.FileContext),
		Statistics: ctxpkg.RepoStatistics{
			TotalFiles: len(functions),
		},
	}
	for path, fns := range functions {
		fc := &ctxpkg.FileContext{Path: path, Functions: fns}
		rc.Files[path] = fc
		rc.Statistics.FunctionCount += len(fns)
	}
	return rc
}

// saveTestOrg stores an org and its repo mappings directly via SQL for testing.
func saveTestOrg(t *testing.T, db *sql.DB, orgID string, repoIDs []string) {
	t.Helper()
	_, err := db.Exec(`INSERT OR REPLACE INTO orgs (id, config_json, created_at) VALUES (?, '{}', datetime('now'))`, orgID)
	if err != nil {
		t.Fatalf("insert org: %v", err)
	}
	for _, repoID := range repoIDs {
		_, err = db.Exec(`INSERT OR IGNORE INTO org_repos (org_id, repo_id) VALUES (?, ?)`, orgID, repoID)
		if err != nil {
			t.Fatalf("insert org_repo: %v", err)
		}
	}
}

// requireFTS5 skips the test if FTS5 is not available in this SQLite build.
func requireFTS5(t *testing.T, store *SQLiteStore) {
	t.Helper()
	if !store.hasFTS5() {
		t.Skip("FTS5 not available — build with -tags fts5")
	}
}

func TestFTS5_TableCreated(t *testing.T) {
	store, cleanup := createTestStore(t)
	defer cleanup()
	requireFTS5(t, store)

	var name string
	err := store.db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='functions_fts'").Scan(&name)
	if err != nil {
		t.Fatalf("FTS5 table not found: %v", err)
	}
	if name != "functions_fts" {
		t.Errorf("expected 'functions_fts', got %q", name)
	}
}

func TestFTS5_InsertTriggerPopulatesIndex(t *testing.T) {
	store, cleanup := createTestStore(t)
	defer cleanup()
	requireFTS5(t, store)
	ctx := context.Background()

	repo := makeTestRepoCtx("repo1", map[string][]ctxpkg.FunctionDef{
		"a.go": {
			{Name: "GetUser", Signature: "func GetUser()", IsPublic: true},
			{Name: "DeleteUser", Signature: "func DeleteUser()", IsPublic: true},
			{Name: "CreateOrder", Signature: "func CreateOrder()", IsPublic: true},
		},
	})

	if err := store.StoreRepoContext(ctx, "repo1", repo); err != nil {
		t.Fatalf("StoreRepoContext: %v", err)
	}

	var count int
	if err := store.db.QueryRow("SELECT count(*) FROM functions_fts").Scan(&count); err != nil {
		t.Fatalf("count FTS: %v", err)
	}
	if count != 3 {
		t.Errorf("expected 3 FTS rows, got %d", count)
	}
}

func TestFTS5_DeleteTriggerRemovesFromIndex(t *testing.T) {
	store, cleanup := createTestStore(t)
	defer cleanup()
	requireFTS5(t, store)
	ctx := context.Background()

	repo := makeTestRepoCtx("repo1", map[string][]ctxpkg.FunctionDef{
		"a.go": {
			{Name: "FuncA", Signature: "func FuncA()", IsPublic: true},
			{Name: "FuncB", Signature: "func FuncB()", IsPublic: true},
			{Name: "FuncC", Signature: "func FuncC()", IsPublic: true},
		},
	})

	if err := store.StoreRepoContext(ctx, "repo1", repo); err != nil {
		t.Fatalf("StoreRepoContext: %v", err)
	}

	// Delete one function directly
	store.db.Exec("DELETE FROM functions WHERE name = 'FuncB'")

	var count int
	store.db.QueryRow("SELECT count(*) FROM functions_fts").Scan(&count)
	if count != 2 {
		t.Errorf("expected 2 FTS rows after delete, got %d", count)
	}
}

func TestFTS5_CascadeDeleteCleansFTSIndex(t *testing.T) {
	store, cleanup := createTestStore(t)
	defer cleanup()
	requireFTS5(t, store)
	ctx := context.Background()

	// Store repo with 5 functions
	repo1 := makeTestRepoCtx("repo1", map[string][]ctxpkg.FunctionDef{
		"a.go": {
			{Name: "F1", Signature: "func F1()", IsPublic: true},
			{Name: "F2", Signature: "func F2()", IsPublic: true},
			{Name: "F3", Signature: "func F3()", IsPublic: true},
			{Name: "F4", Signature: "func F4()", IsPublic: true},
			{Name: "F5", Signature: "func F5()", IsPublic: true},
		},
	})

	if err := store.StoreRepoContext(ctx, "repo1", repo1); err != nil {
		t.Fatalf("StoreRepoContext (initial): %v", err)
	}

	// Re-store with 3 functions (replaces old data)
	repo2 := makeTestRepoCtx("repo1", map[string][]ctxpkg.FunctionDef{
		"a.go": {
			{Name: "G1", Signature: "func G1()", IsPublic: true},
			{Name: "G2", Signature: "func G2()", IsPublic: true},
			{Name: "G3", Signature: "func G3()", IsPublic: true},
		},
	})

	if err := store.StoreRepoContext(ctx, "repo1", repo2); err != nil {
		t.Fatalf("StoreRepoContext (replace): %v", err)
	}

	var count int
	store.db.QueryRow("SELECT count(*) FROM functions_fts").Scan(&count)
	if count != 3 {
		t.Errorf("expected 3 FTS rows after replace, got %d (stale entries)", count)
	}
}

func TestFTS5_SearchReturnsRankedResults(t *testing.T) {
	store, cleanup := createTestStore(t)
	defer cleanup()
	requireFTS5(t, store)
	ctx := context.Background()

	repo := makeTestRepoCtx("repo1", map[string][]ctxpkg.FunctionDef{
		"user.go": {
			{Name: "GetUser", Signature: "func GetUser()", Description: "Gets a user", IsPublic: true},
			{Name: "DeleteUser", Signature: "func DeleteUser()", Description: "Deletes a user", IsPublic: true},
			{Name: "UpdateUserProfile", Signature: "func UpdateUserProfile()", Description: "Updates user profile", IsPublic: true},
		},
		"order.go": {
			{Name: "CreateOrder", Signature: "func CreateOrder()", Description: "Creates an order", IsPublic: true},
		},
	})
	if err := store.StoreRepoContext(ctx, "repo1", repo); err != nil {
		t.Fatalf("StoreRepoContext: %v", err)
	}

	saveTestOrg(t, store.db, "org1", []string{"repo1"})

	results, err := store.SearchFunctionsFTS(ctx, "org1", "User", 10)
	if err != nil {
		t.Fatalf("SearchFunctionsFTS: %v", err)
	}

	if len(results) < 3 {
		t.Errorf("expected at least 3 results, got %d", len(results))
	}

	for _, r := range results {
		if r.Function == "CreateOrder" {
			t.Error("CreateOrder should not match 'User' query")
		}
	}
}

func TestFTS5_ScopesToOrgRepos(t *testing.T) {
	store, cleanup := createTestStore(t)
	defer cleanup()
	requireFTS5(t, store)
	ctx := context.Background()

	repoA := makeTestRepoCtx("repo-a", map[string][]ctxpkg.FunctionDef{
		"a.go": {{Name: "GetUser", Signature: "func GetUser()", IsPublic: true}},
	})
	repoB := makeTestRepoCtx("repo-b", map[string][]ctxpkg.FunctionDef{
		"b.go": {{Name: "GetUser", Signature: "func GetUser()", IsPublic: true}},
	})

	store.StoreRepoContext(ctx, "repo-a", repoA)
	store.StoreRepoContext(ctx, "repo-b", repoB)

	saveTestOrg(t, store.db, "org1", []string{"repo-a"})

	results, err := store.SearchFunctionsFTS(ctx, "org1", "GetUser", 10)
	if err != nil {
		t.Fatalf("SearchFunctionsFTS: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result (repo-a only), got %d", len(results))
	}
	if results[0].RepoID != "repo-a" {
		t.Errorf("expected repo-a, got %s", results[0].RepoID)
	}
}

func TestFTS5_QuerySanitization_SpecialChars(t *testing.T) {
	store, cleanup := createTestStore(t)
	defer cleanup()
	requireFTS5(t, store)
	ctx := context.Background()

	repo := makeTestRepoCtx("repo1", map[string][]ctxpkg.FunctionDef{
		"a.go": {{Name: "errorHandler", Signature: "func errorHandler()", IsPublic: true}},
	})
	store.StoreRepoContext(ctx, "repo1", repo)
	saveTestOrg(t, store.db, "org1", []string{"repo1"})

	// Boolean operators neutralized by quoting
	_, err := store.SearchFunctionsFTS(ctx, "org1", "error OR panic", 10)
	if err != nil {
		t.Errorf("query with OR should not error: %v", err)
	}

	// Unmatched quotes
	_, err = store.SearchFunctionsFTS(ctx, "org1", `"unmatched quote`, 10)
	if err != nil {
		t.Errorf("query with unmatched quote should not error: %v", err)
	}

	// Wildcard
	_, err = store.SearchFunctionsFTS(ctx, "org1", "func*", 10)
	if err != nil {
		t.Errorf("query with wildcard should not error: %v", err)
	}
}

func TestFTS5_QuerySanitization_EmptyQuery(t *testing.T) {
	store, cleanup := createTestStore(t)
	defer cleanup()
	requireFTS5(t, store)
	ctx := context.Background()

	saveTestOrg(t, store.db, "org1", nil)

	results, err := store.SearchFunctionsFTS(ctx, "org1", "", 10)
	if err != nil {
		t.Errorf("empty query should not error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected empty results for empty query, got %d", len(results))
	}
}

func TestSearchFunctionsOrg_CrossRepo(t *testing.T) {
	store, cleanup := createTestStore(t)
	defer cleanup()
	ctx := context.Background()

	repoA := makeTestRepoCtx("repo-a", map[string][]ctxpkg.FunctionDef{
		"a.go": {
			{Name: "GetUser", Signature: "func GetUser()", IsPublic: true},
			{Name: "CreateUser", Signature: "func CreateUser()", IsPublic: true},
		},
	})
	repoB := makeTestRepoCtx("repo-b", map[string][]ctxpkg.FunctionDef{
		"b.go": {
			{Name: "GetOrder", Signature: "func GetOrder()", IsPublic: true},
		},
	})
	repoC := makeTestRepoCtx("repo-c", map[string][]ctxpkg.FunctionDef{
		"c.go": {
			{Name: "UpdateUser", Signature: "func UpdateUser()", IsPublic: true},
		},
	})

	store.StoreRepoContext(ctx, "repo-a", repoA)
	store.StoreRepoContext(ctx, "repo-b", repoB)
	store.StoreRepoContext(ctx, "repo-c", repoC)

	saveTestOrg(t, store.db, "org1", []string{"repo-a", "repo-b", "repo-c"})

	results, err := store.SearchFunctionsOrg(ctx, "org1", "User", 10)
	if err != nil {
		t.Fatalf("SearchFunctionsOrg: %v", err)
	}

	// Should find GetUser, CreateUser, UpdateUser (not GetOrder)
	if len(results) != 3 {
		t.Errorf("expected 3 results, got %d", len(results))
	}

	for _, r := range results {
		if r.RepoID == "" {
			t.Error("RepoID should be populated")
		}
	}
}

func TestSearchByConceptOrg_CrossRepo(t *testing.T) {
	store, cleanup := createTestStore(t)
	defer cleanup()
	ctx := context.Background()

	// Store repos with functions that auto-extract "authentication" concept
	repoA := makeTestRepoCtx("repo-a", map[string][]ctxpkg.FunctionDef{
		"a.go": {{Name: "LoginHandler", Signature: "func LoginHandler()", IsPublic: true}},
	})
	repoB := makeTestRepoCtx("repo-b", map[string][]ctxpkg.FunctionDef{
		"b.go": {{Name: "VerifyToken", Signature: "func VerifyToken()", IsPublic: true}},
	})

	store.StoreRepoContext(ctx, "repo-a", repoA)
	store.StoreRepoContext(ctx, "repo-b", repoB)
	saveTestOrg(t, store.db, "org1", []string{"repo-a", "repo-b"})

	results, err := store.SearchByConceptOrg(ctx, "org1", "authentication", 10)
	if err != nil {
		t.Fatalf("SearchByConceptOrg: %v", err)
	}

	if len(results) != 2 {
		t.Errorf("expected 2 results from both repos, got %d", len(results))
	}

	for _, r := range results {
		if r.RepoID == "" {
			t.Error("RepoID should be populated")
		}
	}
}

func TestHybridSearchOrg_Deduplicates(t *testing.T) {
	store, cleanup := createTestStore(t)
	defer cleanup()
	ctx := context.Background()

	// LoginHandler should match both FTS "Login" and concept "authentication" (auto-extracted)
	repo := makeTestRepoCtx("repo-a", map[string][]ctxpkg.FunctionDef{
		"a.go": {{Name: "LoginHandler", Signature: "func LoginHandler()", Description: "Login handler for authentication", IsPublic: true}},
	})

	store.StoreRepoContext(ctx, "repo-a", repo)
	saveTestOrg(t, store.db, "org1", []string{"repo-a"})

	results, err := store.HybridSearchOrg(ctx, "org1", "login", 10)
	if err != nil {
		t.Fatalf("HybridSearchOrg: %v", err)
	}

	// Should be deduplicated — LoginHandler appears once
	loginCount := 0
	for _, r := range results {
		if r.Function == "LoginHandler" {
			loginCount++
		}
	}
	if loginCount != 1 {
		t.Errorf("expected LoginHandler once, got %d times", loginCount)
	}
}

func TestSearchFunctionsOrg_EmptyOrg(t *testing.T) {
	store, cleanup := createTestStore(t)
	defer cleanup()
	ctx := context.Background()

	saveTestOrg(t, store.db, "empty-org", nil)

	results, err := store.SearchFunctionsOrg(ctx, "empty-org", "anything", 10)
	if err != nil {
		t.Fatalf("SearchFunctionsOrg empty org: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected empty results, got %d", len(results))
	}
}
