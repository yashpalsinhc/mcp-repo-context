package flow

import (
	"context"
	"database/sql"
	"testing"

	"github.com/yashpalc/mcp-repo-context/internal/storage"
	_ "github.com/mattn/go-sqlite3"
)

func newTestImpactStore(t *testing.T) *storage.SQLiteStore {
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
	return store
}

func TestAnalyzeCrossServiceImpactDownstream(t *testing.T) {
	store := newTestImpactStore(t)
	ctx := context.Background()

	// Service A has a function that calls service B
	store.StoreServiceCalls(ctx, "service-a", []storage.ServiceCall{
		{FilePath: "c.go", FunctionName: "MakeCall", CallType: "http", Method: "POST", Target: "/auth/validate", Line: 1},
	})
	// Service B handles the endpoint
	store.StoreEndpoints(ctx, "service-b", []storage.Endpoint{
		{FilePath: "h.go", HandlerName: "Validate", Method: "POST", Path: "/auth/validate"},
	})

	impact, err := AnalyzeCrossServiceImpact(ctx, "service-a", []string{"MakeCall"}, store)
	if err != nil {
		t.Fatal(err)
	}

	if len(impact.DownstreamDeps) != 1 {
		t.Fatalf("Expected 1 downstream dep, got %d", len(impact.DownstreamDeps))
	}
	if impact.DownstreamDeps[0].ServiceName != "service-b" {
		t.Errorf("Expected service-b, got %s", impact.DownstreamDeps[0].ServiceName)
	}
	if impact.RiskLevel != "low" {
		t.Errorf("Expected risk level 'low', got %s", impact.RiskLevel)
	}
}

func TestAnalyzeCrossServiceImpactKafkaConsumers(t *testing.T) {
	store := newTestImpactStore(t)
	ctx := context.Background()

	// Service A produces to user.events
	store.StoreServiceCalls(ctx, "service-a", []storage.ServiceCall{
		{FilePath: "p.go", FunctionName: "Publish", CallType: "kafka_produce", Target: "user.events", Line: 1},
	})
	// Service C consumes user.events
	store.StoreServiceCalls(ctx, "service-c", []storage.ServiceCall{
		{FilePath: "c.go", FunctionName: "Consume", CallType: "kafka_consume", Target: "user.events", Line: 1},
	})

	impact, err := AnalyzeCrossServiceImpact(ctx, "service-a", []string{"Publish"}, store)
	if err != nil {
		t.Fatal(err)
	}

	if len(impact.KafkaConsumers) != 1 {
		t.Fatalf("Expected 1 Kafka consumer, got %d", len(impact.KafkaConsumers))
	}
	if impact.KafkaConsumers[0].ServiceName != "service-c" {
		t.Errorf("Expected service-c, got %s", impact.KafkaConsumers[0].ServiceName)
	}
}

func TestAnalyzeCrossServiceImpactNoImpact(t *testing.T) {
	store := newTestImpactStore(t)
	ctx := context.Background()

	// Service A has internal function with no external calls
	store.StoreServiceCalls(ctx, "service-a", []storage.ServiceCall{})
	store.StoreEndpoints(ctx, "service-a", []storage.Endpoint{})

	impact, err := AnalyzeCrossServiceImpact(ctx, "service-a", []string{"InternalFunc"}, store)
	if err != nil {
		t.Fatal(err)
	}

	if impact.AffectedServices != 0 {
		t.Errorf("Expected 0 affected services, got %d", impact.AffectedServices)
	}
	if impact.RiskLevel != "none" {
		t.Errorf("Expected risk level 'none', got %s", impact.RiskLevel)
	}
}

func TestAnalyzeCrossServiceImpactHighRisk(t *testing.T) {
	store := newTestImpactStore(t)
	ctx := context.Background()

	// Service A calls 3 different services
	store.StoreServiceCalls(ctx, "service-a", []storage.ServiceCall{
		{FilePath: "c.go", FunctionName: "CallAll", CallType: "http", Method: "GET", Target: "/b/api", Line: 1},
		{FilePath: "c.go", FunctionName: "CallAll", CallType: "http", Method: "GET", Target: "/c/api", Line: 2},
		{FilePath: "c.go", FunctionName: "CallAll", CallType: "http", Method: "GET", Target: "/d/api", Line: 3},
	})
	store.StoreEndpoints(ctx, "service-b", []storage.Endpoint{
		{FilePath: "h.go", HandlerName: "H", Method: "GET", Path: "/b/api"},
	})
	store.StoreEndpoints(ctx, "service-c", []storage.Endpoint{
		{FilePath: "h.go", HandlerName: "H", Method: "GET", Path: "/c/api"},
	})
	store.StoreEndpoints(ctx, "service-d", []storage.Endpoint{
		{FilePath: "h.go", HandlerName: "H", Method: "GET", Path: "/d/api"},
	})

	impact, err := AnalyzeCrossServiceImpact(ctx, "service-a", []string{"CallAll"}, store)
	if err != nil {
		t.Fatal(err)
	}

	if impact.AffectedServices != 3 {
		t.Errorf("Expected 3 affected services, got %d", impact.AffectedServices)
	}
	if impact.RiskLevel != "high" {
		t.Errorf("Expected risk level 'high', got %s", impact.RiskLevel)
	}
}

func TestAnalyzeCrossServiceImpactSkipsSelfReferences(t *testing.T) {
	store := newTestImpactStore(t)
	ctx := context.Background()

	// Service A calls its own endpoint
	store.StoreServiceCalls(ctx, "service-a", []storage.ServiceCall{
		{FilePath: "c.go", FunctionName: "CallSelf", CallType: "http", Method: "GET", Target: "/self/api", Line: 1},
	})
	store.StoreEndpoints(ctx, "service-a", []storage.Endpoint{
		{FilePath: "h.go", HandlerName: "Handler", Method: "GET", Path: "/self/api"},
	})

	impact, err := AnalyzeCrossServiceImpact(ctx, "service-a", []string{"CallSelf"}, store)
	if err != nil {
		t.Fatal(err)
	}

	if impact.AffectedServices != 0 {
		t.Errorf("Expected 0 affected services (self-ref), got %d", impact.AffectedServices)
	}
}

func TestAnalyzeCrossServiceImpactEmptyFunctions(t *testing.T) {
	store := newTestImpactStore(t)
	ctx := context.Background()

	impact, err := AnalyzeCrossServiceImpact(ctx, "service-a", []string{}, store)
	if err != nil {
		t.Fatal(err)
	}
	if impact.RiskLevel != "none" {
		t.Errorf("Expected 'none', got %s", impact.RiskLevel)
	}
}

func TestFormatCrossServiceImpact(t *testing.T) {
	impact := &CrossServiceImpact{
		DownstreamDeps: []ServiceCallRef{
			{ServiceName: "auth-service", RepoID: "repo-b", FunctionName: "Validate", CallType: "http", Path: "/auth/validate"},
		},
		KafkaConsumers: []ServiceCallRef{
			{ServiceName: "notify-service", RepoID: "repo-c", FunctionName: "HandleEvent", CallType: "kafka", Path: "user.updated"},
		},
		RiskLevel:        "medium",
		AffectedServices: 2,
	}

	output := FormatCrossServiceImpact(impact)
	if output == "" {
		t.Fatal("Expected non-empty output")
	}
	if !containsStr(output, "Cross-Service Impact") {
		t.Error("Expected 'Cross-Service Impact' in output")
	}
	if !containsStr(output, "Medium") {
		t.Error("Expected 'Medium' risk level in output")
	}
	if !containsStr(output, "auth-service") {
		t.Error("Expected 'auth-service' in output")
	}
}

func TestFormatCrossServiceImpactEmpty(t *testing.T) {
	impact := &CrossServiceImpact{RiskLevel: "none", AffectedServices: 0}
	output := FormatCrossServiceImpact(impact)
	if output != "" {
		t.Error("Expected empty output for no impact")
	}
}

func containsStr(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 && (s == substr || len(s) >= len(substr) && findSubstr(s, substr))
}

func findSubstr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
