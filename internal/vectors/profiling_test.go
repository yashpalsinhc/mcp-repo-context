package vectors

import (
	"bytes"
	"context"
	"log"
	"os"
	"strings"
	"testing"

	_ "github.com/mattn/go-sqlite3"
	ctxpkg "github.com/yashpalc/mcp-repo-context/internal/context"
)

// captureLog captures log output during a function call.
func captureLog(t *testing.T, fn func()) string {
	t.Helper()
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr) // restore
	fn()
	return buf.String()
}

func TestIndexRepository_LogsTotalDuration(t *testing.T) {
	ss, _, cleanup := setupIncrementalTest(t)
	defer cleanup()

	ctx := context.Background()
	repo := buildTestRepo("test-repo", map[string]*ctxpkg.FileContext{
		"pkg/user.go": {
			Path: "pkg/user.go",
			Functions: []ctxpkg.FunctionDef{
				{Name: "CreateUser", Signature: "func CreateUser() error", IsPublic: true},
			},
		},
	})

	output := captureLog(t, func() {
		ss.IndexRepository(ctx, repo)
	})

	if !strings.Contains(output, "IndexRepository") {
		t.Errorf("log should contain 'IndexRepository', got: %s", output)
	}
	if !strings.Contains(output, "completed in") {
		t.Errorf("log should contain 'completed in', got: %s", output)
	}
}

func TestSearchFunctions_LogsQueryAndDuration(t *testing.T) {
	ss, _, cleanup := setupIncrementalTest(t)
	defer cleanup()

	ctx := context.Background()
	repo := buildTestRepo("test-repo", map[string]*ctxpkg.FileContext{
		"pkg/user.go": {
			Path: "pkg/user.go",
			Functions: []ctxpkg.FunctionDef{
				{Name: "CreateUser", Signature: "func CreateUser() error", IsPublic: true},
			},
		},
	})
	ss.IndexRepository(ctx, repo)

	output := captureLog(t, func() {
		ss.SearchFunctions(ctx, "create user", "test-repo", 10)
	})

	if !strings.Contains(output, "SearchFunctions") {
		t.Errorf("log should contain 'SearchFunctions', got: %s", output)
	}
	if !strings.Contains(output, "completed in") {
		t.Errorf("log should contain duration, got: %s", output)
	}
}

func TestRefreshFile_LogsDuration(t *testing.T) {
	ss, _, cleanup := setupIncrementalTest(t)
	defer cleanup()

	ctx := context.Background()
	repo := buildTestRepo("test-repo", map[string]*ctxpkg.FileContext{
		"pkg/user.go": {
			Path: "pkg/user.go",
			Functions: []ctxpkg.FunctionDef{
				{Name: "CreateUser", Signature: "func CreateUser() error", IsPublic: true},
			},
		},
	})
	ss.IndexRepository(ctx, repo)

	output := captureLog(t, func() {
		ss.RefreshFile(ctx, "test-repo", "pkg/user.go",
			[]ctxpkg.FunctionDef{{Name: "UpdateUser", Signature: "func UpdateUser() error", IsPublic: true}},
			nil)
	})

	if !strings.Contains(output, "RefreshFile") {
		t.Errorf("log should contain 'RefreshFile', got: %s", output)
	}
	if !strings.Contains(output, "completed in") {
		t.Errorf("log should contain duration, got: %s", output)
	}
}

func TestVectorCount_ReturnsCorrectCount(t *testing.T) {
	ss, store, cleanup := setupIncrementalTest(t)
	defer cleanup()

	ctx := context.Background()

	// Store 5 function vectors and 3 type vectors
	records := make([]VectorRecord, 0, 8)
	for i := 0; i < 5; i++ {
		records = append(records, VectorRecord{
			ID:     "r:func:f.go:F" + string(rune('0'+i)),
			RepoID: "repo-a",
			Type:   "function",
			Name:   "F" + string(rune('0'+i)),
			Vector: make([]float64, 256),
		})
	}
	for i := 0; i < 3; i++ {
		records = append(records, VectorRecord{
			ID:     "r:type:t.go:T" + string(rune('0'+i)),
			RepoID: "repo-a",
			Type:   "type",
			Name:   "T" + string(rune('0'+i)),
			Vector: make([]float64, 256),
		})
	}
	store.StoreBatch(ctx, records)

	count, err := ss.Count(ctx, "repo-a")
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if count != 8 {
		t.Errorf("expected 8, got %d", count)
	}
}

func TestVectorCount_ReturnsZeroForUnindexedRepo(t *testing.T) {
	ss, _, cleanup := setupIncrementalTest(t)
	defer cleanup()

	count, err := ss.Count(context.Background(), "unknown-repo")
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 for unindexed repo, got %d", count)
	}
}

func TestVectorCount_ReturnsZeroForEmptyStore(t *testing.T) {
	ss, _, cleanup := setupIncrementalTest(t)
	defer cleanup()

	count, err := ss.Count(context.Background(), "any")
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 for empty store, got %d", count)
	}
}

func TestTiming_DoesNotAffectResults(t *testing.T) {
	ss, store, cleanup := setupIncrementalTest(t)
	defer cleanup()

	ctx := context.Background()
	repo := buildTestRepo("test-repo", map[string]*ctxpkg.FileContext{
		"pkg/user.go": {
			Path: "pkg/user.go",
			Functions: []ctxpkg.FunctionDef{
				{Name: "CreateUser", Signature: "func CreateUser() error", Description: "creates user", IsPublic: true},
				{Name: "DeleteUser", Signature: "func DeleteUser() error", Description: "deletes user", IsPublic: true},
			},
		},
	})

	// IndexRepository with timing
	if err := ss.IndexRepository(ctx, repo); err != nil {
		t.Fatalf("IndexRepository: %v", err)
	}

	count, _ := store.Count(ctx, "test-repo")
	if count != 2 {
		t.Errorf("expected 2 vectors, got %d", count)
	}

	// Search with timing
	results, err := ss.SearchFunctions(ctx, "create user", "test-repo", 10)
	if err != nil {
		t.Fatalf("SearchFunctions: %v", err)
	}
	if len(results) == 0 {
		t.Error("expected search results")
	}
}

func TestTruncateQuery(t *testing.T) {
	tests := []struct {
		query    string
		maxLen   int
		expected string
	}{
		{"short", 10, "short"},
		{"exactly10!", 10, "exactly10!"},
		{"this is a longer query", 10, "this is a ..."},
		{"", 10, ""},
	}

	for _, tc := range tests {
		result := truncateQuery(tc.query, tc.maxLen)
		if result != tc.expected {
			t.Errorf("truncateQuery(%q, %d) = %q, want %q", tc.query, tc.maxLen, result, tc.expected)
		}
	}
}
