package flow

import (
	"context"
	"database/sql"
	"testing"

	"github.com/yashpalc/mcp-repo-context/internal/storage"
	_ "github.com/mattn/go-sqlite3"
)

func newTestMatcher(t *testing.T) (*EndpointMatcher, *storage.SQLiteStore) {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:?_journal_mode=WAL&_foreign_keys=ON")
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	store, err := storage.NewSQLiteStoreWithDB(db)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return NewEndpointMatcher(store), store
}

func TestExactPathMatch(t *testing.T) {
	matcher, store := newTestMatcher(t)
	ctx := context.Background()

	store.StoreEndpoints(ctx, "repo-a", []storage.Endpoint{
		{FilePath: "h.go", HandlerName: "GetUsers", Method: "GET", Path: "/api/v1/users"},
	})

	results, err := matcher.MatchHTTP(ctx, storage.ServiceCall{
		CallType: "http", Method: "GET", Target: "/api/v1/users",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("Expected 1 match, got %d", len(results))
	}
	if results[0].Confidence != "exact" {
		t.Errorf("Expected confidence 'exact', got %q", results[0].Confidence)
	}
}

func TestParameterizedPathMatch(t *testing.T) {
	matcher, store := newTestMatcher(t)
	ctx := context.Background()

	store.StoreEndpoints(ctx, "repo-a", []storage.Endpoint{
		{FilePath: "h.go", HandlerName: "GetUser", Method: "GET", Path: "/api/v1/users/{id}"},
	})

	// FindMatchingEndpoints does exact SQL match, so for parameterized we need
	// to use the trie-based approach. Let's test via MatchAll.
	store.StoreServiceCalls(ctx, "repo-b", []storage.ServiceCall{
		{FilePath: "c.go", FunctionName: "Call", CallType: "http", Method: "GET", Target: "/api/v1/users/123"},
	})

	results, err := matcher.MatchAll(ctx, []string{"repo-a", "repo-b"})
	if err != nil {
		t.Fatal(err)
	}

	found := false
	for _, cm := range results {
		if cm.Call.Target == "/api/v1/users/123" && len(cm.Matches) > 0 {
			found = true
			if cm.Matches[0].Confidence != "parameterized" {
				t.Errorf("Expected confidence 'parameterized', got %q", cm.Matches[0].Confidence)
			}
		}
	}
	if !found {
		t.Error("Expected parameterized match for /api/v1/users/123")
	}
}

func TestNoMatchDifferentPaths(t *testing.T) {
	matcher, store := newTestMatcher(t)
	ctx := context.Background()

	store.StoreEndpoints(ctx, "repo-a", []storage.Endpoint{
		{FilePath: "h.go", HandlerName: "GetUsers", Method: "GET", Path: "/api/v1/users"},
	})

	results, err := matcher.MatchHTTP(ctx, storage.ServiceCall{
		CallType: "http", Method: "GET", Target: "/api/v1/orders",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Errorf("Expected 0 matches, got %d", len(results))
	}
}

func TestMethodFiltering(t *testing.T) {
	matcher, store := newTestMatcher(t)
	ctx := context.Background()

	store.StoreEndpoints(ctx, "repo-a", []storage.Endpoint{
		{FilePath: "h.go", HandlerName: "GetUsers", Method: "GET", Path: "/users"},
	})

	// POST should not match GET endpoint
	store.StoreServiceCalls(ctx, "repo-b", []storage.ServiceCall{
		{FilePath: "c.go", FunctionName: "Call", CallType: "http", Method: "POST", Target: "/users"},
	})

	results, err := matcher.MatchAll(ctx, []string{"repo-a", "repo-b"})
	if err != nil {
		t.Fatal(err)
	}

	for _, cm := range results {
		if cm.Call.Method == "POST" && len(cm.Matches) > 0 {
			t.Errorf("POST should not match GET endpoint, got %d matches", len(cm.Matches))
		}
	}
}

func TestServiceHintNarrowsMatches(t *testing.T) {
	matcher, store := newTestMatcher(t)
	ctx := context.Background()

	store.StoreEndpoints(ctx, "auth-service", []storage.Endpoint{
		{FilePath: "h.go", HandlerName: "Health", Method: "GET", Path: "/health"},
	})
	store.StoreEndpoints(ctx, "user-service", []storage.Endpoint{
		{FilePath: "h.go", HandlerName: "Health", Method: "GET", Path: "/health"},
	})

	store.StoreServiceCalls(ctx, "caller-repo", []storage.ServiceCall{
		{FilePath: "c.go", FunctionName: "Call", CallType: "http", Method: "GET", Target: "/health", ServiceHint: "auth"},
	})

	results, err := matcher.MatchAll(ctx, []string{"auth-service", "user-service", "caller-repo"})
	if err != nil {
		t.Fatal(err)
	}

	for _, cm := range results {
		if cm.Call.ServiceHint == "auth" && len(cm.Matches) > 0 {
			// First match should be the hint-matched one
			if cm.Matches[0].Confidence != "hint-matched" {
				t.Errorf("Expected first match confidence 'hint-matched', got %q", cm.Matches[0].Confidence)
			}
			if cm.Matches[0].Endpoint.RepoID != "auth-service" {
				t.Errorf("Expected auth-service first, got %s", cm.Matches[0].Endpoint.RepoID)
			}
		}
	}
}

func TestKafkaTopicExactMatch(t *testing.T) {
	matcher, store := newTestMatcher(t)
	ctx := context.Background()

	store.StoreServiceCalls(ctx, "repo-b", []storage.ServiceCall{
		{FilePath: "c.go", FunctionName: "Consume", CallType: "kafka_consume", Target: "user.events", Line: 1},
	})

	consumers, err := matcher.MatchKafkaConsumers(ctx, "user.events")
	if err != nil {
		t.Fatal(err)
	}
	if len(consumers) != 1 {
		t.Errorf("Expected 1 consumer, got %d", len(consumers))
	}
}

func TestURLNormalizationStripsSchemeAndHost(t *testing.T) {
	matcher, store := newTestMatcher(t)
	ctx := context.Background()

	store.StoreEndpoints(ctx, "repo-a", []storage.Endpoint{
		{FilePath: "h.go", HandlerName: "Validate", Method: "GET", Path: "/api/validate"},
	})

	results, err := matcher.MatchHTTP(ctx, storage.ServiceCall{
		CallType: "http", Method: "GET", Target: "https://auth-service:8080/api/validate",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Errorf("Expected 1 match after URL normalization, got %d", len(results))
	}
}

func TestURLNormalizationTrailingSlash(t *testing.T) {
	matcher, store := newTestMatcher(t)
	ctx := context.Background()

	store.StoreEndpoints(ctx, "repo-a", []storage.Endpoint{
		{FilePath: "h.go", HandlerName: "GetUsers", Method: "GET", Path: "/api/users/"},
	})

	results, err := matcher.MatchHTTP(ctx, storage.ServiceCall{
		CallType: "http", Method: "GET", Target: "/api/users",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Errorf("Expected 1 match (trailing slash ignored), got %d", len(results))
	}
}

func TestPathTrieBatchMatching(t *testing.T) {
	matcher, store := newTestMatcher(t)
	ctx := context.Background()

	// Create 10 endpoints across 2 repos
	var epsA, epsB []storage.Endpoint
	for i := 0; i < 5; i++ {
		epsA = append(epsA, storage.Endpoint{
			FilePath: "h.go", HandlerName: "H", Method: "GET",
			Path: "/api/v1/resource" + string(rune('a'+i)),
		})
		epsB = append(epsB, storage.Endpoint{
			FilePath: "h.go", HandlerName: "H", Method: "GET",
			Path: "/api/v1/resource" + string(rune('a'+i+5)),
		})
	}
	store.StoreEndpoints(ctx, "repo-a", epsA)
	store.StoreEndpoints(ctx, "repo-b", epsB)

	// Create calls matching some of them
	var calls []storage.ServiceCall
	for i := 0; i < 3; i++ {
		calls = append(calls, storage.ServiceCall{
			FilePath: "c.go", FunctionName: "F", CallType: "http", Method: "GET",
			Target: "/api/v1/resource" + string(rune('a'+i)),
			Line: i + 1,
		})
	}
	store.StoreServiceCalls(ctx, "repo-c", calls)

	results, err := matcher.MatchAll(ctx, []string{"repo-a", "repo-b", "repo-c"})
	if err != nil {
		t.Fatal(err)
	}

	matched := 0
	for _, cm := range results {
		if len(cm.Matches) > 0 {
			matched++
		}
	}
	if matched < 3 {
		t.Errorf("Expected at least 3 matched calls, got %d", matched)
	}
}

func TestGRPCExactPathMatch(t *testing.T) {
	matcher, store := newTestMatcher(t)
	ctx := context.Background()

	store.StoreEndpoints(ctx, "repo-a", []storage.Endpoint{
		{FilePath: "h.go", HandlerName: "GetUser", Method: "gRPC", Path: "/UserService/GetUser"},
	})

	results, err := matcher.MatchGRPC(ctx, storage.ServiceCall{
		CallType: "grpc", Method: "gRPC", Target: "/UserService/GetUser",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("Expected 1 gRPC match, got %d", len(results))
	}
	if results[0].Confidence != "exact" {
		t.Errorf("Expected confidence 'exact', got %q", results[0].Confidence)
	}
}

func TestAmbiguousMatchWithoutHint(t *testing.T) {
	matcher, store := newTestMatcher(t)
	ctx := context.Background()

	store.StoreEndpoints(ctx, "repo-a", []storage.Endpoint{
		{FilePath: "h.go", HandlerName: "H1", Method: "GET", Path: "/users"},
	})
	store.StoreEndpoints(ctx, "repo-b", []storage.Endpoint{
		{FilePath: "h.go", HandlerName: "H2", Method: "GET", Path: "/users"},
	})

	store.StoreServiceCalls(ctx, "repo-c", []storage.ServiceCall{
		{FilePath: "c.go", FunctionName: "Call", CallType: "http", Method: "GET", Target: "/users", Line: 1},
	})

	results, err := matcher.MatchAll(ctx, []string{"repo-a", "repo-b", "repo-c"})
	if err != nil {
		t.Fatal(err)
	}

	for _, cm := range results {
		if cm.Call.Target == "/users" {
			if len(cm.Matches) != 2 {
				t.Errorf("Expected 2 ambiguous matches, got %d", len(cm.Matches))
			}
			for _, mr := range cm.Matches {
				if mr.Confidence != "ambiguous" {
					t.Errorf("Expected confidence 'ambiguous', got %q", mr.Confidence)
				}
			}
		}
	}
}

func TestNormalizePathDotSegments(t *testing.T) {
	result := NormalizePath("/api/../api/users")
	if result != "/api/users" {
		t.Errorf("Expected /api/users, got %s", result)
	}
}

func TestNormalizePathPercentEncoding(t *testing.T) {
	result := NormalizePath("/api/users%2F123")
	if result != "/api/users/123" {
		t.Errorf("Expected /api/users/123, got %s", result)
	}
}

func TestNormalizePathLowercase(t *testing.T) {
	result := NormalizePath("/API/Users")
	if result != "/api/users" {
		t.Errorf("Expected /api/users, got %s", result)
	}
}
