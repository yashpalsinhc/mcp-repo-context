package storage

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	ctxpkg "github.com/yashpalc/mcp-repo-context/internal/context"
)

func createTestSQLiteStore(t *testing.T) (*SQLiteStore, func()) {
	t.Helper()

	tmpDir, err := os.MkdirTemp("", "sqlite-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	dbPath := filepath.Join(tmpDir, "test.db")
	store, err := NewSQLiteStore(dbPath)
	if err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("Failed to create SQLite store: %v", err)
	}

	cleanup := func() {
		store.Close()
		os.RemoveAll(tmpDir)
	}

	return store, cleanup
}

func createTestRepoContext() *ctxpkg.RepoContext {
	return &ctxpkg.RepoContext{
		ID:         "github.com/test/repo",
		URL:        "https://github.com/test/repo",
		Branch:     "main",
		CommitHash: "abc123",
		AnalyzedAt: time.Now(),
		Files: map[string]*ctxpkg.FileContext{
			"pkg/handler.go": {
				Path:      "pkg/handler.go",
				Hash:      "hash1",
				Language:  "go",
				Package:   "handler",
				Size:      1000,
				LineCount: 50,
				Purpose:   "HTTP handlers for user management",
				Concepts:  []string{"handler", "http", "user"},
				Imports: []ctxpkg.Import{
					{Path: "net/http"},
					{Path: "encoding/json"},
				},
				Functions: []ctxpkg.FunctionDef{
					{
						Name:        "CreateUser",
						Signature:   "CreateUser(w http.ResponseWriter, r *http.Request)",
						Description: "Creates a new user in the database",
						IsPublic:    true,
						LineStart:   10,
						LineEnd:     30,
						Complexity:  3,
						Parameters: []ctxpkg.Param{
							{Name: "w", Type: "http.ResponseWriter"},
							{Name: "r", Type: "*http.Request"},
						},
						Returns: []string{},
						Behavior: &ctxpkg.FunctionBehavior{
							Summary:  "Creates a new user from JSON request body",
							Steps:    []string{"Parse JSON body", "Validate input", "Insert to database"},
							Patterns: []string{"crud", "handler"},
						},
						Calls: []ctxpkg.CallRef{
							{Function: "json.Decode", Package: "encoding/json", Type: "stdlib"},
							{Function: "ValidateUser", Type: "internal", Line: 15},
							{Function: "db.Insert", Package: "database/sql", Type: "external"},
						},
						SideEffects: []string{"db_query", "http_response"},
					},
					{
						Name:        "GetUser",
						Signature:   "GetUser(w http.ResponseWriter, r *http.Request)",
						Description: "Retrieves user by ID",
						IsPublic:    true,
						LineStart:   35,
						LineEnd:     50,
						Complexity:  2,
						Behavior: &ctxpkg.FunctionBehavior{
							Summary:  "Fetches user from database by ID",
							Patterns: []string{"crud", "handler"},
						},
						SideEffects: []string{"db_query"},
					},
				},
				Types: []ctxpkg.TypeDef{
					{
						Name:        "User",
						Kind:        "struct",
						Description: "User represents a system user",
						IsPublic:    true,
						LineStart:   5,
						LineEnd:     8,
						Fields: []ctxpkg.Field{
							{Name: "ID", Type: "int", IsPublic: true},
							{Name: "Name", Type: "string", IsPublic: true},
						},
					},
				},
				Constants: []ctxpkg.ConstantDef{
					{Name: "MaxUsers", Type: "int", Value: "1000", IsPublic: true, LineStart: 3},
				},
			},
			"pkg/auth.go": {
				Path:      "pkg/auth.go",
				Hash:      "hash2",
				Language:  "go",
				Package:   "auth",
				Size:      500,
				LineCount: 25,
				Purpose:   "Authentication middleware",
				Concepts:  []string{"authentication", "middleware"},
				Functions: []ctxpkg.FunctionDef{
					{
						Name:        "Authenticate",
						Signature:   "Authenticate(next http.Handler) http.Handler",
						Description: "Middleware that validates JWT tokens",
						IsPublic:    true,
						LineStart:   5,
						LineEnd:     20,
						Behavior: &ctxpkg.FunctionBehavior{
							Summary:  "Validates JWT token and sets user context",
							Patterns: []string{"authentication", "middleware"},
						},
						SideEffects: []string{"http_call"},
					},
				},
			},
		},
		Statistics: ctxpkg.RepoStatistics{
			TotalFiles:        2,
			TotalLines:        75,
			FunctionCount:     3,
			TypeCount:         1,
			LanguageBreakdown: map[string]int{"go": 2},
		},
	}
}

func TestSQLiteStore_StoreAndGetRepoContext(t *testing.T) {
	store, cleanup := createTestSQLiteStore(t)
	defer cleanup()

	ctx := context.Background()
	repoCtx := createTestRepoContext()

	// Store
	err := store.StoreRepoContext(ctx, repoCtx.ID, repoCtx)
	if err != nil {
		t.Fatalf("Failed to store repo context: %v", err)
	}

	// Get
	retrieved, err := store.GetRepoContext(ctx, repoCtx.ID)
	if err != nil {
		t.Fatalf("Failed to get repo context: %v", err)
	}

	// Verify
	if retrieved.ID != repoCtx.ID {
		t.Errorf("ID mismatch: got %s, want %s", retrieved.ID, repoCtx.ID)
	}
	if retrieved.URL != repoCtx.URL {
		t.Errorf("URL mismatch: got %s, want %s", retrieved.URL, repoCtx.URL)
	}
	if len(retrieved.Files) != len(repoCtx.Files) {
		t.Errorf("File count mismatch: got %d, want %d", len(retrieved.Files), len(repoCtx.Files))
	}

	// Verify file contents
	handlerFile := retrieved.Files["pkg/handler.go"]
	if handlerFile == nil {
		t.Fatal("Handler file not found")
	}
	if len(handlerFile.Functions) != 2 {
		t.Errorf("Function count mismatch: got %d, want 2", len(handlerFile.Functions))
	}
	if len(handlerFile.Types) != 1 {
		t.Errorf("Type count mismatch: got %d, want 1", len(handlerFile.Types))
	}
}

func TestSQLiteStore_GetFileContext(t *testing.T) {
	store, cleanup := createTestSQLiteStore(t)
	defer cleanup()

	ctx := context.Background()
	repoCtx := createTestRepoContext()

	if err := store.StoreRepoContext(ctx, repoCtx.ID, repoCtx); err != nil {
		t.Fatalf("Failed to store: %v", err)
	}

	fileCtx, err := store.GetFileContext(ctx, repoCtx.ID, "pkg/handler.go")
	if err != nil {
		t.Fatalf("Failed to get file context: %v", err)
	}

	if fileCtx.Path != "pkg/handler.go" {
		t.Errorf("Path mismatch: got %s", fileCtx.Path)
	}
	if fileCtx.Package != "handler" {
		t.Errorf("Package mismatch: got %s", fileCtx.Package)
	}
	if len(fileCtx.Functions) != 2 {
		t.Errorf("Function count mismatch: got %d", len(fileCtx.Functions))
	}
}

func TestSQLiteStore_GetFileContext_NotFound(t *testing.T) {
	store, cleanup := createTestSQLiteStore(t)
	defer cleanup()

	ctx := context.Background()
	_, err := store.GetFileContext(ctx, "nonexistent", "file.go")
	if err != ErrNotFound {
		t.Errorf("Expected ErrNotFound, got: %v", err)
	}
}

func TestSQLiteStore_ContextExists(t *testing.T) {
	store, cleanup := createTestSQLiteStore(t)
	defer cleanup()

	ctx := context.Background()
	repoCtx := createTestRepoContext()

	// Should not exist initially
	exists, _, err := store.ContextExists(ctx, repoCtx.ID, 0)
	if err != nil {
		t.Fatalf("ContextExists failed: %v", err)
	}
	if exists {
		t.Error("Context should not exist initially")
	}

	// Store and check again
	if err := store.StoreRepoContext(ctx, repoCtx.ID, repoCtx); err != nil {
		t.Fatalf("Failed to store: %v", err)
	}

	exists, analyzedAt, err := store.ContextExists(ctx, repoCtx.ID, 0)
	if err != nil {
		t.Fatalf("ContextExists failed: %v", err)
	}
	if !exists {
		t.Error("Context should exist")
	}
	if analyzedAt.IsZero() {
		t.Error("AnalyzedAt should not be zero")
	}
}

func TestSQLiteStore_DeleteContext(t *testing.T) {
	store, cleanup := createTestSQLiteStore(t)
	defer cleanup()

	ctx := context.Background()
	repoCtx := createTestRepoContext()

	if err := store.StoreRepoContext(ctx, repoCtx.ID, repoCtx); err != nil {
		t.Fatalf("Failed to store: %v", err)
	}

	if err := store.DeleteContext(ctx, repoCtx.ID); err != nil {
		t.Fatalf("Failed to delete: %v", err)
	}

	exists, _, _ := store.ContextExists(ctx, repoCtx.ID, 0)
	if exists {
		t.Error("Context should not exist after delete")
	}
}

func TestSQLiteStore_ListContexts(t *testing.T) {
	store, cleanup := createTestSQLiteStore(t)
	defer cleanup()

	ctx := context.Background()
	repoCtx := createTestRepoContext()

	if err := store.StoreRepoContext(ctx, repoCtx.ID, repoCtx); err != nil {
		t.Fatalf("Failed to store: %v", err)
	}

	list, err := store.ListContexts(ctx)
	if err != nil {
		t.Fatalf("Failed to list: %v", err)
	}

	if len(list) != 1 {
		t.Errorf("Expected 1 repo, got %d", len(list))
	}
	if list[0].RepoID != repoCtx.ID {
		t.Errorf("RepoID mismatch: got %s", list[0].RepoID)
	}
}

func TestSQLiteStore_SearchFunctions(t *testing.T) {
	store, cleanup := createTestSQLiteStore(t)
	defer cleanup()

	ctx := context.Background()
	repoCtx := createTestRepoContext()

	if err := store.StoreRepoContext(ctx, repoCtx.ID, repoCtx); err != nil {
		t.Fatalf("Failed to store: %v", err)
	}

	// Search for "Create"
	results, err := store.SearchFunctions(ctx, repoCtx.ID, "Create")
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	if len(results) == 0 {
		t.Error("Expected at least one result for 'Create'")
	}

	found := false
	for _, r := range results {
		if r.Function == "CreateUser" {
			found = true
			break
		}
	}
	if !found {
		t.Error("CreateUser should be in results")
	}
}

func TestSQLiteStore_SearchBySideEffect(t *testing.T) {
	store, cleanup := createTestSQLiteStore(t)
	defer cleanup()

	ctx := context.Background()
	repoCtx := createTestRepoContext()

	if err := store.StoreRepoContext(ctx, repoCtx.ID, repoCtx); err != nil {
		t.Fatalf("Failed to store: %v", err)
	}

	// Search for db_query
	results, err := store.SearchBySideEffect(ctx, repoCtx.ID, "db_query")
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	if len(results) < 2 {
		t.Errorf("Expected at least 2 functions with db_query, got %d", len(results))
	}
}

func TestSQLiteStore_SearchByConcept(t *testing.T) {
	store, cleanup := createTestSQLiteStore(t)
	defer cleanup()

	ctx := context.Background()
	repoCtx := createTestRepoContext()

	if err := store.StoreRepoContext(ctx, repoCtx.ID, repoCtx); err != nil {
		t.Fatalf("Failed to store: %v", err)
	}

	// Search for authentication concept
	results, err := store.SearchByConcept(ctx, repoCtx.ID, "authentication")
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	if len(results) == 0 {
		t.Error("Expected at least one result for 'authentication'")
	}

	found := false
	for _, r := range results {
		if r.Function == "Authenticate" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Authenticate should be in authentication concept results")
	}
}

func TestSQLiteStore_GetFunctionCallers(t *testing.T) {
	store, cleanup := createTestSQLiteStore(t)
	defer cleanup()

	ctx := context.Background()
	repoCtx := createTestRepoContext()

	if err := store.StoreRepoContext(ctx, repoCtx.ID, repoCtx); err != nil {
		t.Fatalf("Failed to store: %v", err)
	}

	// Find who calls ValidateUser
	results, err := store.GetFunctionCallers(ctx, repoCtx.ID, "ValidateUser")
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	if len(results) != 1 {
		t.Errorf("Expected 1 caller of ValidateUser, got %d", len(results))
	}
	if len(results) > 0 && results[0].Function != "CreateUser" {
		t.Errorf("Expected CreateUser to call ValidateUser, got %s", results[0].Function)
	}
}

func TestSQLiteStore_GetRepoStats(t *testing.T) {
	store, cleanup := createTestSQLiteStore(t)
	defer cleanup()

	ctx := context.Background()
	repoCtx := createTestRepoContext()

	if err := store.StoreRepoContext(ctx, repoCtx.ID, repoCtx); err != nil {
		t.Fatalf("Failed to store: %v", err)
	}

	stats, err := store.GetRepoStats(ctx, repoCtx.ID)
	if err != nil {
		t.Fatalf("GetRepoStats failed: %v", err)
	}

	if stats.FileCount != 2 {
		t.Errorf("FileCount mismatch: got %d, want 2", stats.FileCount)
	}
	if stats.FunctionCount != 3 {
		t.Errorf("FunctionCount mismatch: got %d, want 3", stats.FunctionCount)
	}
	if stats.Languages["go"] != 2 {
		t.Errorf("Go file count mismatch: got %d, want 2", stats.Languages["go"])
	}
}

func TestSQLiteStore_HybridSearch(t *testing.T) {
	store, cleanup := createTestSQLiteStore(t)
	defer cleanup()

	ctx := context.Background()
	repoCtx := createTestRepoContext()

	if err := store.StoreRepoContext(ctx, repoCtx.ID, repoCtx); err != nil {
		t.Fatalf("Failed to store: %v", err)
	}

	// Search with a query that matches both keyword and concept
	results, err := store.HybridSearch(ctx, repoCtx.ID, "user authentication")
	if err != nil {
		t.Fatalf("HybridSearch failed: %v", err)
	}

	// Should find functions related to user and authentication
	if len(results) == 0 {
		t.Error("Expected some results from hybrid search")
	}
}

func TestSQLiteStore_UpdateExistingRepo(t *testing.T) {
	store, cleanup := createTestSQLiteStore(t)
	defer cleanup()

	ctx := context.Background()
	repoCtx := createTestRepoContext()

	// Store initial version
	if err := store.StoreRepoContext(ctx, repoCtx.ID, repoCtx); err != nil {
		t.Fatalf("Failed to store: %v", err)
	}

	// Modify and store again
	repoCtx.CommitHash = "newcommit456"
	repoCtx.Files["pkg/new.go"] = &ctxpkg.FileContext{
		Path:     "pkg/new.go",
		Hash:     "newhash",
		Language: "go",
		Package:  "new",
	}

	if err := store.StoreRepoContext(ctx, repoCtx.ID, repoCtx); err != nil {
		t.Fatalf("Failed to update: %v", err)
	}

	// Verify update
	retrieved, err := store.GetRepoContext(ctx, repoCtx.ID)
	if err != nil {
		t.Fatalf("Failed to get: %v", err)
	}

	if retrieved.CommitHash != "newcommit456" {
		t.Errorf("CommitHash not updated: got %s", retrieved.CommitHash)
	}
	if len(retrieved.Files) != 3 {
		t.Errorf("Expected 3 files, got %d", len(retrieved.Files))
	}
}
