package storage

import (
	"context"
	"database/sql"
	"testing"
	"time"

	ctxpkg "github.com/yashpalc/mcp-repo-context/internal/context"
	_ "github.com/mattn/go-sqlite3"
)

func newTestStore(t *testing.T) *SQLiteStore {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:?_journal_mode=WAL&_foreign_keys=ON")
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	store, err := NewSQLiteStoreWithDB(db)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return store
}

func TestStoreEndpointsRoundTrip(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	endpoints := []Endpoint{
		{RepoID: "test-repo", FilePath: "handlers/user.go", HandlerName: "GetUsers", Method: "GET", Path: "/api/users", RawPath: "/api/users", Framework: "chi", Line: 10},
		{RepoID: "test-repo", FilePath: "handlers/user.go", HandlerName: "CreateUser", Method: "POST", Path: "/api/users", RawPath: "/api/users", Framework: "chi", Line: 15},
		{RepoID: "test-repo", FilePath: "handlers/user.go", HandlerName: "DeleteUser", Method: "DELETE", Path: "/api/users/{id}", RawPath: "/api/users/{id}", Framework: "chi", Line: 20},
	}

	if err := store.StoreEndpoints(ctx, "test-repo", endpoints); err != nil {
		t.Fatalf("StoreEndpoints failed: %v", err)
	}

	got, err := store.GetEndpoints(ctx, "test-repo")
	if err != nil {
		t.Fatalf("GetEndpoints failed: %v", err)
	}

	if len(got) != 3 {
		t.Fatalf("Expected 3 endpoints, got %d", len(got))
	}

	// Results ordered by path, method
	if got[0].Method != "GET" || got[0].Path != "/api/users" {
		t.Errorf("First endpoint: expected GET /api/users, got %s %s", got[0].Method, got[0].Path)
	}
	if got[0].HandlerName != "GetUsers" {
		t.Errorf("Expected handler GetUsers, got %s", got[0].HandlerName)
	}
	if got[0].Framework != "chi" {
		t.Errorf("Expected framework chi, got %s", got[0].Framework)
	}
}

func TestStoreEndpointsReplacesExisting(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	// Store 3 endpoints first
	endpoints1 := []Endpoint{
		{RepoID: "test-repo", FilePath: "a.go", HandlerName: "H1", Method: "GET", Path: "/a"},
		{RepoID: "test-repo", FilePath: "b.go", HandlerName: "H2", Method: "POST", Path: "/b"},
		{RepoID: "test-repo", FilePath: "c.go", HandlerName: "H3", Method: "PUT", Path: "/c"},
	}
	if err := store.StoreEndpoints(ctx, "test-repo", endpoints1); err != nil {
		t.Fatal(err)
	}

	// Store 2 different endpoints for same repo
	endpoints2 := []Endpoint{
		{RepoID: "test-repo", FilePath: "x.go", HandlerName: "X1", Method: "GET", Path: "/x"},
		{RepoID: "test-repo", FilePath: "y.go", HandlerName: "Y1", Method: "POST", Path: "/y"},
	}
	if err := store.StoreEndpoints(ctx, "test-repo", endpoints2); err != nil {
		t.Fatal(err)
	}

	got, err := store.GetEndpoints(ctx, "test-repo")
	if err != nil {
		t.Fatal(err)
	}

	if len(got) != 2 {
		t.Errorf("Expected 2 endpoints (replaced), got %d", len(got))
	}
}

func TestStoreEndpointsMultiRepoIsolation(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	store.StoreEndpoints(ctx, "repo-a", []Endpoint{{FilePath: "a.go", HandlerName: "H1", Method: "GET", Path: "/a"}})
	store.StoreEndpoints(ctx, "repo-b", []Endpoint{{FilePath: "b.go", HandlerName: "H2", Method: "GET", Path: "/b"}})

	gotA, _ := store.GetEndpoints(ctx, "repo-a")
	if len(gotA) != 1 || gotA[0].Path != "/a" {
		t.Errorf("Expected 1 endpoint /a for repo-a, got %d", len(gotA))
	}
}

func TestStoreServiceCallsRoundTrip(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	calls := []ServiceCall{
		{RepoID: "test-repo", FilePath: "svc.go", FunctionName: "CallAPI", CallType: "http", Method: "GET", Target: "http://user-svc/api/users", ServiceHint: "user-svc", Line: 10},
		{RepoID: "test-repo", FilePath: "svc.go", FunctionName: "Publish", CallType: "kafka_produce", Method: "produce", Target: "user.events", Line: 20},
	}

	if err := store.StoreServiceCalls(ctx, "test-repo", calls); err != nil {
		t.Fatalf("StoreServiceCalls failed: %v", err)
	}

	got, err := store.GetServiceCalls(ctx, "test-repo")
	if err != nil {
		t.Fatalf("GetServiceCalls failed: %v", err)
	}

	if len(got) != 2 {
		t.Fatalf("Expected 2 service calls, got %d", len(got))
	}
}

func TestStoreServiceCallsReplacesExisting(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	calls1 := []ServiceCall{
		{FilePath: "a.go", FunctionName: "F1", CallType: "http", Target: "/a", Line: 1},
		{FilePath: "b.go", FunctionName: "F2", CallType: "http", Target: "/b", Line: 2},
		{FilePath: "c.go", FunctionName: "F3", CallType: "http", Target: "/c", Line: 3},
	}
	store.StoreServiceCalls(ctx, "test-repo", calls1)

	calls2 := []ServiceCall{
		{FilePath: "x.go", FunctionName: "X1", CallType: "grpc", Target: "svc/Method", Line: 1},
	}
	store.StoreServiceCalls(ctx, "test-repo", calls2)

	got, _ := store.GetServiceCalls(ctx, "test-repo")
	if len(got) != 1 {
		t.Errorf("Expected 1 service call (replaced), got %d", len(got))
	}
}

func TestFindMatchingEndpoints(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	store.StoreEndpoints(ctx, "repo-a", []Endpoint{{FilePath: "a.go", HandlerName: "H1", Method: "GET", Path: "/api/users"}})
	store.StoreEndpoints(ctx, "repo-b", []Endpoint{{FilePath: "b.go", HandlerName: "H2", Method: "GET", Path: "/api/users"}})

	got, err := store.FindMatchingEndpoints(ctx, "GET", "/api/users")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Errorf("Expected 2 matching endpoints, got %d", len(got))
	}
}

func TestFindMatchingEndpointsFiltersByMethod(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	store.StoreEndpoints(ctx, "repo-a", []Endpoint{{FilePath: "a.go", HandlerName: "H1", Method: "GET", Path: "/api/users"}})
	store.StoreEndpoints(ctx, "repo-b", []Endpoint{{FilePath: "b.go", HandlerName: "H2", Method: "POST", Path: "/api/users"}})

	got, _ := store.FindMatchingEndpoints(ctx, "GET", "/api/users")
	if len(got) != 1 {
		t.Errorf("Expected 1 matching endpoint (GET only), got %d", len(got))
	}
	if got[0].RepoID != "repo-a" {
		t.Errorf("Expected repo-a, got %s", got[0].RepoID)
	}
}

func TestFindMatchingEndpointsANYMatchesAll(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	store.StoreEndpoints(ctx, "repo-a", []Endpoint{{FilePath: "a.go", HandlerName: "H1", Method: "ANY", Path: "/api/users"}})

	got, _ := store.FindMatchingEndpoints(ctx, "POST", "/api/users")
	if len(got) != 1 {
		t.Errorf("Expected 1 matching endpoint (ANY matches POST), got %d", len(got))
	}
}

func TestFindMatchingConsumers(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	store.StoreServiceCalls(ctx, "repo-b", []ServiceCall{{FilePath: "b.go", FunctionName: "Consume", CallType: "kafka_consume", Target: "user.events", Line: 1}})
	store.StoreServiceCalls(ctx, "repo-c", []ServiceCall{{FilePath: "c.go", FunctionName: "Listen", CallType: "kafka_consume", Target: "user.events", Line: 1}})

	got, err := store.FindMatchingConsumers(ctx, "user.events")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Errorf("Expected 2 consumers, got %d", len(got))
	}
}

func TestFindMatchingProducers(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	store.StoreServiceCalls(ctx, "repo-a", []ServiceCall{{FilePath: "a.go", FunctionName: "Publish", CallType: "kafka_produce", Target: "user.events", Line: 1}})

	got, err := store.FindMatchingProducers(ctx, "user.events")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Errorf("Expected 1 producer, got %d", len(got))
	}
}

func TestEndpointTablesCreatedByMigration(t *testing.T) {
	store := newTestStore(t)

	// Check endpoints table exists
	var name string
	err := store.db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='endpoints'").Scan(&name)
	if err != nil {
		t.Errorf("endpoints table not found: %v", err)
	}

	// Check service_calls table exists
	err = store.db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='service_calls'").Scan(&name)
	if err != nil {
		t.Errorf("service_calls table not found: %v", err)
	}
}

func TestDeleteContextRemovesEndpoints(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	// Need to create a repo first so DeleteContext has something to delete
	repoCtx := &ctxpkg.RepoContext{
		ID:         "test-repo",
		AnalyzedAt: time.Now(),
		Statistics: ctxpkg.RepoStatistics{},
		Files:      make(map[string]*ctxpkg.FileContext),
	}
	store.StoreRepoContext(ctx, "test-repo", repoCtx)

	store.StoreEndpoints(ctx, "test-repo", []Endpoint{{FilePath: "a.go", HandlerName: "H1", Method: "GET", Path: "/a"}})
	store.StoreServiceCalls(ctx, "test-repo", []ServiceCall{{FilePath: "a.go", FunctionName: "F1", CallType: "http", Target: "/x", Line: 1}})

	if err := store.DeleteContext(ctx, "test-repo"); err != nil {
		t.Fatal(err)
	}

	eps, _ := store.GetEndpoints(ctx, "test-repo")
	if len(eps) != 0 {
		t.Errorf("Expected 0 endpoints after delete, got %d", len(eps))
	}

	scs, _ := store.GetServiceCalls(ctx, "test-repo")
	if len(scs) != 0 {
		t.Errorf("Expected 0 service calls after delete, got %d", len(scs))
	}
}

func TestStoreRepoContextPopulatesEndpoints(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	repoCtx := &ctxpkg.RepoContext{
		ID:         "test-repo",
		AnalyzedAt: time.Now(),
		Statistics: ctxpkg.RepoStatistics{},
		Files: map[string]*ctxpkg.FileContext{
			"handlers/user.go": {
				Path:     "handlers/user.go",
				Language: "go",
				Routes: []ctxpkg.Route{
					{Method: "GET", Path: "/api/users", RawPath: "/api/users", Handler: "GetUsers", Framework: "chi", File: "handlers/user.go", Line: 10},
					{Method: "POST", Path: "/api/users", RawPath: "/api/users", Handler: "CreateUser", Framework: "chi", File: "handlers/user.go", Line: 15},
				},
				Functions: []ctxpkg.FunctionDef{
					{
						Name: "CallExternal",
						APIFlow: &ctxpkg.APIFlow{
							ExternalCalls: []ctxpkg.ExternalCall{
								{Method: "GET", URL: "http://order-svc/api/orders", ServiceHint: "order-svc", Line: 30},
							},
						},
					},
					{
						Name: "PublishEvent",
						AsyncCalls: []ctxpkg.AsyncCall{
							{Protocol: "kafka", Direction: "produce", Topic: "user.events", Library: "segmentio", Line: 40},
						},
					},
				},
			},
		},
	}

	if err := store.StoreRepoContext(ctx, "test-repo", repoCtx); err != nil {
		t.Fatalf("StoreRepoContext failed: %v", err)
	}

	// Check endpoints populated
	eps, err := store.GetEndpoints(ctx, "test-repo")
	if err != nil {
		t.Fatal(err)
	}
	if len(eps) != 2 {
		t.Errorf("Expected 2 endpoints from routes, got %d", len(eps))
	}

	// Check service calls populated
	scs, err := store.GetServiceCalls(ctx, "test-repo")
	if err != nil {
		t.Fatal(err)
	}
	if len(scs) != 2 {
		t.Errorf("Expected 2 service calls (1 http + 1 kafka), got %d", len(scs))
	}

	// Verify HTTP call
	foundHTTP := false
	foundKafka := false
	for _, sc := range scs {
		if sc.CallType == "http" && sc.Target == "http://order-svc/api/orders" {
			foundHTTP = true
		}
		if sc.CallType == "kafka_produce" && sc.Target == "user.events" {
			foundKafka = true
		}
	}
	if !foundHTTP {
		t.Error("Expected HTTP service call")
	}
	if !foundKafka {
		t.Error("Expected Kafka service call")
	}
}
