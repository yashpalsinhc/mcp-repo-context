package flow

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"github.com/yashpalc/mcp-repo-context/internal/storage"
	_ "github.com/mattn/go-sqlite3"
)

// mockOrgStore implements OrgStore for testing.
type mockOrgStore struct {
	repos map[string][]string // orgID → repoIDs
}

func (m *mockOrgStore) GetOrgRepos(ctx context.Context, orgID string) ([]string, error) {
	repos, ok := m.repos[orgID]
	if !ok {
		return nil, nil
	}
	return repos, nil
}

func newTestTopologyBuilder(t *testing.T) (*TopologyBuilder, *storage.SQLiteStore) {
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

	orgStore := &mockOrgStore{repos: make(map[string][]string)}
	builder := NewTopologyBuilder(store, orgStore)
	return builder, store
}

func TestBuildTopologyCreatesNodes(t *testing.T) {
	builder, store := newTestTopologyBuilder(t)
	ctx := context.Background()

	// Set up org repos
	orgStore := builder.orgStore.(*mockOrgStore)
	orgStore.repos["test-org"] = []string{"repo-a", "repo-b", "repo-c"}

	store.StoreEndpoints(ctx, "repo-a", []storage.Endpoint{
		{FilePath: "h.go", HandlerName: "H1", Method: "GET", Path: "/a"},
	})
	store.StoreEndpoints(ctx, "repo-b", []storage.Endpoint{
		{FilePath: "h.go", HandlerName: "H2", Method: "GET", Path: "/b"},
		{FilePath: "h.go", HandlerName: "H3", Method: "POST", Path: "/b"},
	})
	store.StoreEndpoints(ctx, "repo-c", []storage.Endpoint{})

	topology, err := builder.BuildTopology(ctx, "test-org")
	if err != nil {
		t.Fatal(err)
	}
	if len(topology.Nodes) != 3 {
		t.Fatalf("Expected 3 nodes, got %d", len(topology.Nodes))
	}

	// Check endpoint counts
	counts := map[string]int{}
	for _, n := range topology.Nodes {
		counts[n.ServiceName] = n.EndpointCount
	}
	if counts["repo-a"] != 1 || counts["repo-b"] != 2 || counts["repo-c"] != 0 {
		t.Errorf("Unexpected endpoint counts: %v", counts)
	}
}

func TestDeriveServiceName(t *testing.T) {
	tests := []struct {
		repoID   string
		expected string
	}{
		{"github.com/org/auth-service", "auth-service"},
		{"local:/path/to/user-service", "user-service"},
		{"my-repo", "my-repo"},
	}
	for _, tt := range tests {
		got := deriveServiceName(tt.repoID)
		if got != tt.expected {
			t.Errorf("deriveServiceName(%q) = %q, want %q", tt.repoID, got, tt.expected)
		}
	}
}

func TestBuildTopologyHTTPEdges(t *testing.T) {
	builder, store := newTestTopologyBuilder(t)
	ctx := context.Background()

	orgStore := builder.orgStore.(*mockOrgStore)
	orgStore.repos["test-org"] = []string{"service-a", "service-b"}

	store.StoreEndpoints(ctx, "service-b", []storage.Endpoint{
		{FilePath: "h.go", HandlerName: "GetUsers", Method: "GET", Path: "/api/users"},
	})
	store.StoreServiceCalls(ctx, "service-a", []storage.ServiceCall{
		{FilePath: "c.go", FunctionName: "Call", CallType: "http", Method: "GET", Target: "/api/users", Line: 1},
	})

	topology, err := builder.BuildTopology(ctx, "test-org")
	if err != nil {
		t.Fatal(err)
	}

	found := false
	for _, edge := range topology.Edges {
		if edge.Source == "service-a" && edge.Target == "service-b" && edge.EdgeType == "http" {
			found = true
			if edge.Confidence != "exact" {
				t.Errorf("Expected 'exact' confidence, got %q", edge.Confidence)
			}
		}
	}
	if !found {
		t.Error("Expected HTTP edge from service-a to service-b")
	}
}

func TestBuildTopologyKafkaEdges(t *testing.T) {
	builder, store := newTestTopologyBuilder(t)
	ctx := context.Background()

	orgStore := builder.orgStore.(*mockOrgStore)
	orgStore.repos["test-org"] = []string{"producer-svc", "consumer-svc"}

	store.StoreEndpoints(ctx, "producer-svc", []storage.Endpoint{})
	store.StoreEndpoints(ctx, "consumer-svc", []storage.Endpoint{})

	store.StoreServiceCalls(ctx, "producer-svc", []storage.ServiceCall{
		{FilePath: "p.go", FunctionName: "Publish", CallType: "kafka_produce", Method: "produce", Target: "user.events", Line: 1},
	})
	store.StoreServiceCalls(ctx, "consumer-svc", []storage.ServiceCall{
		{FilePath: "c.go", FunctionName: "Consume", CallType: "kafka_consume", Method: "consume", Target: "user.events", Line: 1},
	})

	topology, err := builder.BuildTopology(ctx, "test-org")
	if err != nil {
		t.Fatal(err)
	}

	foundProduce := false
	foundConsume := false
	for _, edge := range topology.Edges {
		if edge.EdgeType == "kafka_produce" && edge.Source == "producer-svc" && edge.Path == "user.events" {
			foundProduce = true
		}
		if edge.EdgeType == "kafka_consume" && edge.Target == "consumer-svc" && edge.Path == "user.events" {
			foundConsume = true
		}
	}
	if !foundProduce {
		t.Error("Expected Kafka produce edge")
	}
	if !foundConsume {
		t.Error("Expected Kafka consume edge")
	}
}

func TestBuildTopologyFanOut(t *testing.T) {
	builder, store := newTestTopologyBuilder(t)
	ctx := context.Background()

	orgStore := builder.orgStore.(*mockOrgStore)
	orgStore.repos["test-org"] = []string{"producer", "consumer-a", "consumer-b"}

	store.StoreEndpoints(ctx, "producer", []storage.Endpoint{})
	store.StoreEndpoints(ctx, "consumer-a", []storage.Endpoint{})
	store.StoreEndpoints(ctx, "consumer-b", []storage.Endpoint{})

	store.StoreServiceCalls(ctx, "producer", []storage.ServiceCall{
		{FilePath: "p.go", FunctionName: "Pub", CallType: "kafka_produce", Target: "user.events", Line: 1},
	})
	store.StoreServiceCalls(ctx, "consumer-a", []storage.ServiceCall{
		{FilePath: "a.go", FunctionName: "ConA", CallType: "kafka_consume", Target: "user.events", Line: 1},
	})
	store.StoreServiceCalls(ctx, "consumer-b", []storage.ServiceCall{
		{FilePath: "b.go", FunctionName: "ConB", CallType: "kafka_consume", Target: "user.events", Line: 1},
	})

	topology, err := builder.BuildTopology(ctx, "test-org")
	if err != nil {
		t.Fatal(err)
	}

	kafkaEdges := 0
	for _, edge := range topology.Edges {
		if edge.EdgeType == "kafka_produce" || edge.EdgeType == "kafka_consume" {
			kafkaEdges++
		}
	}
	if kafkaEdges != 3 { // 1 produce + 2 consume
		t.Errorf("Expected 3 Kafka edges, got %d", kafkaEdges)
	}
}

func TestBuildTopologyUnmatchedCalls(t *testing.T) {
	builder, store := newTestTopologyBuilder(t)
	ctx := context.Background()

	orgStore := builder.orgStore.(*mockOrgStore)
	orgStore.repos["test-org"] = []string{"service-a"}

	store.StoreEndpoints(ctx, "service-a", []storage.Endpoint{})
	store.StoreServiceCalls(ctx, "service-a", []storage.ServiceCall{
		{FilePath: "c.go", FunctionName: "Call", CallType: "http", Method: "GET", Target: "/unknown/endpoint", Line: 1},
	})

	topology, err := builder.BuildTopology(ctx, "test-org")
	if err != nil {
		t.Fatal(err)
	}

	found := false
	for _, edge := range topology.Edges {
		if edge.Target == "<unknown>" && edge.Confidence == "unmatched" {
			found = true
		}
	}
	if !found {
		t.Error("Expected unmatched edge with Target='<unknown>'")
	}
}

func TestBuildTopologyServiceHint(t *testing.T) {
	builder, store := newTestTopologyBuilder(t)
	ctx := context.Background()

	orgStore := builder.orgStore.(*mockOrgStore)
	orgStore.repos["test-org"] = []string{"service-a", "service-b", "service-c"}

	store.StoreEndpoints(ctx, "service-b", []storage.Endpoint{
		{FilePath: "h.go", HandlerName: "Health", Method: "GET", Path: "/health"},
	})
	store.StoreEndpoints(ctx, "service-c", []storage.Endpoint{
		{FilePath: "h.go", HandlerName: "Health", Method: "GET", Path: "/health"},
	})
	store.StoreEndpoints(ctx, "service-a", []storage.Endpoint{})

	store.StoreServiceCalls(ctx, "service-a", []storage.ServiceCall{
		{FilePath: "c.go", FunctionName: "Call", CallType: "http", Method: "GET", Target: "/health", ServiceHint: "service-b", Line: 1},
	})

	topology, err := builder.BuildTopology(ctx, "test-org")
	if err != nil {
		t.Fatal(err)
	}

	// First edge should target service-b (hint-matched)
	for _, edge := range topology.Edges {
		if edge.Source == "service-a" && edge.Confidence == "hint-matched" {
			if edge.Target != "service-b" {
				t.Errorf("Expected hint-matched edge to target service-b, got %s", edge.Target)
			}
			return
		}
	}
	t.Error("Expected a hint-matched edge from service-a")
}

func TestBuildTopologyEmptyOrg(t *testing.T) {
	builder, _ := newTestTopologyBuilder(t)
	ctx := context.Background()

	orgStore := builder.orgStore.(*mockOrgStore)
	orgStore.repos["empty-org"] = []string{}

	topology, err := builder.BuildTopology(ctx, "empty-org")
	if err != nil {
		t.Fatal(err)
	}
	if len(topology.Nodes) != 0 {
		t.Errorf("Expected 0 nodes, got %d", len(topology.Nodes))
	}
	if len(topology.Edges) != 0 {
		t.Errorf("Expected 0 edges, got %d", len(topology.Edges))
	}
}

func TestGenerateMermaid(t *testing.T) {
	topology := &ServiceTopology{
		Nodes: []ServiceNode{
			{ServiceName: "auth-service", IsDeepAnalyzed: true},
			{ServiceName: "user-service", IsDeepAnalyzed: true},
		},
		Edges: []ServiceEdge{
			{Source: "auth-service", Target: "user-service", EdgeType: "http", Method: "GET", Path: "/users"},
			{Source: "auth-service", Target: "user.events", EdgeType: "kafka_produce", Method: "produce", Path: "user.events"},
			{Source: "user.events", Target: "user-service", EdgeType: "kafka_consume", Method: "consume", Path: "user.events"},
		},
	}

	mermaid := topology.GenerateMermaid()
	if !strings.Contains(mermaid, "graph LR") {
		t.Error("Expected 'graph LR' in Mermaid output")
	}
	if !strings.Contains(mermaid, "-->") {
		t.Error("Expected solid arrows for HTTP edges")
	}
	if !strings.Contains(mermaid, "-.->") {
		t.Error("Expected dashed arrows for Kafka edges")
	}
}

func TestGenerateMermaidMetadataNode(t *testing.T) {
	topology := &ServiceTopology{
		Nodes: []ServiceNode{
			{ServiceName: "frontend", IsDeepAnalyzed: false},
		},
	}

	mermaid := topology.GenerateMermaid()
	if !strings.Contains(mermaid, "fill:#ddd") {
		t.Error("Expected grayed-out style for metadata-only node")
	}
}
