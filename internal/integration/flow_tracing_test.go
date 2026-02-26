package integration

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yashpalc/mcp-repo-context/internal/flow"
	"github.com/yashpalc/mcp-repo-context/internal/orchestrator"
	"github.com/yashpalc/mcp-repo-context/internal/repo"
	"github.com/yashpalc/mcp-repo-context/internal/storage"
)

// orgStoreAdapter wraps a map of org->repos to implement flow.OrgStore.
type orgStoreAdapter struct {
	orgs map[string][]string
}

func (a *orgStoreAdapter) GetOrgRepos(_ context.Context, orgID string) ([]string, error) {
	return a.orgs[orgID], nil
}

// createFlowTestFixtures creates synthetic Go source files for 3 services.
func createFlowTestFixtures(t *testing.T) (string, map[string]string) {
	t.Helper()
	tmpDir := t.TempDir()

	services := map[string]string{
		"auth-service":         filepath.Join(tmpDir, "auth-service"),
		"user-service":         filepath.Join(tmpDir, "user-service"),
		"notification-service": filepath.Join(tmpDir, "notification-service"),
	}

	authDir := services["auth-service"]
	os.MkdirAll(authDir, 0o755)
	writeFlowFile(t, filepath.Join(authDir, "go.mod"), "module github.com/test/auth-service\n\ngo 1.21\n\nrequire (\n\tgithub.com/gorilla/mux v1.8.0\n\tgithub.com/segmentio/kafka-go v0.4.47\n)\n")
	writeFlowFile(t, filepath.Join(authDir, "main.go"), `package main

import (
	"fmt"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/segmentio/kafka-go"
)

func main() {
	r := mux.NewRouter()
	r.HandleFunc("/login", LoginHandler).Methods("POST")
	http.ListenAndServe(":8080", r)
}

func LoginHandler(w http.ResponseWriter, r *http.Request) {
	userID := 42
	resp, err := http.Get(fmt.Sprintf("http://user-service:8080/api/users/%d", userID))
	if err != nil {
		http.Error(w, "user lookup failed", 500)
		return
	}
	defer resp.Body.Close()

	writer := &kafka.Writer{
		Addr:  kafka.TCP("kafka:9092"),
		Topic: "user.logged_in",
	}
	_ = writer

	w.WriteHeader(http.StatusOK)
}
`)

	userDir := services["user-service"]
	os.MkdirAll(userDir, 0o755)
	writeFlowFile(t, filepath.Join(userDir, "go.mod"), "module github.com/test/user-service\n\ngo 1.21\n\nrequire github.com/gorilla/mux v1.8.0\n")
	writeFlowFile(t, filepath.Join(userDir, "main.go"), `package main

import (
	"database/sql"
	"net/http"

	"github.com/gorilla/mux"
)

var db *sql.DB

func main() {
	r := mux.NewRouter()
	r.HandleFunc("/api/users/{id}", GetUser).Methods("GET")
	http.ListenAndServe(":8080", r)
}

func GetUser(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]
	row := db.QueryRow("SELECT name, email FROM users WHERE id = ?", id)
	_ = row
	w.WriteHeader(http.StatusOK)
}
`)

	notifDir := services["notification-service"]
	os.MkdirAll(notifDir, 0o755)
	writeFlowFile(t, filepath.Join(notifDir, "go.mod"), "module github.com/test/notification-service\n\ngo 1.21\n\nrequire github.com/segmentio/kafka-go v0.4.47\n")
	writeFlowFile(t, filepath.Join(notifDir, "main.go"), `package main

import (
	"fmt"
	"net/http"

	"github.com/segmentio/kafka-go"
)

func main() {
	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers: []string{"kafka:9092"},
		Topic:   "user.logged_in",
		GroupID: "notification-group",
	})
	_ = reader
}

func HandleLogin(msg kafka.Message) {
	resp, err := http.Post("http://audit-service:9090/audit/log", "application/json", nil)
	if err != nil {
		fmt.Println("audit failed:", err)
		return
	}
	defer resp.Body.Close()
}
`)

	return tmpDir, services
}

func writeFlowFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("Failed to write %s: %v", path, err)
	}
}

func setupFlowTestMgr(t *testing.T) (orchestrator.Manager, *storage.SQLiteStore, func()) {
	t.Helper()
	tmpDir := t.TempDir()

	store, err := storage.NewSQLiteStore(filepath.Join(tmpDir, "test.db"))
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}

	cloner, err := repo.NewCloner(filepath.Join(tmpDir, "repos"))
	if err != nil {
		t.Fatalf("Failed to create cloner: %v", err)
	}

	scanner := repo.NewScanner(nil, 1024*1024)
	mgr := orchestrator.NewManager(store, cloner, scanner)

	return mgr, store, func() { store.Close() }
}

// analyzeLocalServices uses the orchestrator to analyze service directories.
func analyzeLocalServices(t *testing.T, mgr orchestrator.Manager, services map[string]string) []string {
	t.Helper()
	ctx := context.Background()
	var repoIDs []string

	for name, dir := range services {
		result, err := mgr.AnalyzeLocal(ctx, dir, orchestrator.AnalyzeLocalOptions{Force: true})
		if err != nil {
			t.Fatalf("Failed to analyze %s: %v", name, err)
		}
		repoIDs = append(repoIDs, result.ProjectID)
		t.Logf("Analyzed %s -> %s (files=%d)", name, result.ProjectID, result.FileCount)
	}
	return repoIDs
}

func TestFlowTracing_FullPipeline(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	_, services := createFlowTestFixtures(t)
	mgr, store, cleanup := setupFlowTestMgr(t)
	defer cleanup()

	repoIDs := analyzeLocalServices(t, mgr, services)

	ctx := context.Background()
	orgID := "test-org"

	orgStore := &orgStoreAdapter{
		orgs: map[string][]string{orgID: repoIDs},
	}

	builder := flow.NewTopologyBuilder(store, orgStore)
	topology, err := builder.BuildTopology(ctx, orgID)
	if err != nil {
		t.Fatalf("BuildTopology failed: %v", err)
	}

	// Assert nodes
	if len(topology.Nodes) != 3 {
		t.Errorf("Expected 3 nodes, got %d", len(topology.Nodes))
	}
	for _, n := range topology.Nodes {
		t.Logf("  Node: %s (repo=%s, endpoints=%d)", n.ServiceName, n.RepoID, n.EndpointCount)
	}

	// Log all edges
	for _, e := range topology.Edges {
		t.Logf("  Edge: %s -> %s [%s] %s %s (conf=%s)", e.Source, e.Target, e.EdgeType, e.Method, e.Path, e.Confidence)
	}

	// Check Mermaid generation
	mermaid := topology.GenerateMermaid()
	if !strings.Contains(mermaid, "graph LR") {
		t.Error("Expected Mermaid diagram to start with 'graph LR'")
	}
	t.Logf("Mermaid:\n%s", mermaid)
}

func TestFlowTracing_EndpointStorage(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	_, services := createFlowTestFixtures(t)
	mgr, store, cleanup := setupFlowTestMgr(t)
	defer cleanup()

	repoIDs := analyzeLocalServices(t, mgr, services)

	ctx := context.Background()

	// Find auth-service repoID
	var authRepoID string
	for _, id := range repoIDs {
		if strings.Contains(id, "auth-service") {
			authRepoID = id
			break
		}
	}
	if authRepoID == "" {
		t.Fatal("Could not find auth-service repo ID")
	}

	endpoints, err := store.GetEndpoints(ctx, authRepoID)
	if err != nil {
		t.Fatalf("GetEndpoints failed: %v", err)
	}

	t.Logf("auth-service endpoints: %d", len(endpoints))
	for _, ep := range endpoints {
		t.Logf("  %s %s -> %s (%s:%d)", ep.Method, ep.Path, ep.HandlerName, ep.FilePath, ep.Line)
	}

	hasLoginEndpoint := false
	for _, ep := range endpoints {
		if ep.Path == "/login" && ep.Method == "POST" {
			hasLoginEndpoint = true
		}
	}
	if !hasLoginEndpoint {
		t.Error("Expected POST /login endpoint for auth-service")
	}

	calls, err := store.GetServiceCalls(ctx, authRepoID)
	if err != nil {
		t.Fatalf("GetServiceCalls failed: %v", err)
	}

	t.Logf("auth-service service calls: %d", len(calls))
	for _, c := range calls {
		t.Logf("  %s %s %s -> %s (hint=%s)", c.CallType, c.Method, c.Target, c.FunctionName, c.ServiceHint)
	}
}

func TestFlowTracing_CircularDependency(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	tmpDir := t.TempDir()

	serviceA := filepath.Join(tmpDir, "svc-a")
	serviceB := filepath.Join(tmpDir, "svc-b")
	os.MkdirAll(serviceA, 0o755)
	os.MkdirAll(serviceB, 0o755)

	writeFlowFile(t, filepath.Join(serviceA, "go.mod"), "module github.com/test/svc-a\n\ngo 1.21\n\nrequire github.com/gorilla/mux v1.8.0\n")
	writeFlowFile(t, filepath.Join(serviceA, "main.go"), `package main

import (
	"net/http"
	"github.com/gorilla/mux"
)

func main() {
	r := mux.NewRouter()
	r.HandleFunc("/auth", AuthHandler).Methods("POST")
	http.ListenAndServe(":8080", r)
}

func AuthHandler(w http.ResponseWriter, r *http.Request) {
	resp, _ := http.Get("http://svc-b:8080/users")
	if resp != nil {
		resp.Body.Close()
	}
}
`)

	writeFlowFile(t, filepath.Join(serviceB, "go.mod"), "module github.com/test/svc-b\n\ngo 1.21\n\nrequire github.com/gorilla/mux v1.8.0\n")
	writeFlowFile(t, filepath.Join(serviceB, "main.go"), `package main

import (
	"net/http"
	"github.com/gorilla/mux"
)

func main() {
	r := mux.NewRouter()
	r.HandleFunc("/users", UsersHandler).Methods("GET")
	http.ListenAndServe(":8080", r)
}

func UsersHandler(w http.ResponseWriter, r *http.Request) {
	resp, _ := http.Post("http://svc-a:8080/auth", "application/json", nil)
	if resp != nil {
		resp.Body.Close()
	}
}
`)

	mgr, store, cleanup := setupFlowTestMgr(t)
	defer cleanup()

	services := map[string]string{"svc-a": serviceA, "svc-b": serviceB}
	repoIDs := analyzeLocalServices(t, mgr, services)

	ctx := context.Background()
	orgID := "circular-org"
	orgStore := &orgStoreAdapter{
		orgs: map[string][]string{orgID: repoIDs},
	}

	builder := flow.NewTopologyBuilder(store, orgStore)
	topology, err := builder.BuildTopology(ctx, orgID)
	if err != nil {
		t.Fatalf("BuildTopology failed: %v", err)
	}

	for _, e := range topology.Edges {
		t.Logf("  Edge: %s -> %s [%s] %s %s", e.Source, e.Target, e.EdgeType, e.Method, e.Path)
	}

	if len(topology.Nodes) != 2 {
		t.Errorf("Expected 2 nodes, got %d", len(topology.Nodes))
	}
}

func TestFlowTracing_DeriveServiceName(t *testing.T) {
	tests := []struct {
		repoID   string
		expected string
	}{
		{"local:/path/to/auth-service", "auth-service"},
		{"github.com/org/user-service", "user-service"},
		{"my-repo", "my-repo"},
	}
	for _, tt := range tests {
		got := flow.DeriveServiceName(tt.repoID)
		if got != tt.expected {
			t.Errorf("DeriveServiceName(%q) = %q, want %q", tt.repoID, got, tt.expected)
		}
	}
}
