package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/yashpalc/mcp-repo-context/internal/comparison"
	"github.com/yashpalc/mcp-repo-context/internal/mcp"
	"github.com/yashpalc/mcp-repo-context/internal/org"
	"github.com/yashpalc/mcp-repo-context/internal/orchestrator"
	"github.com/yashpalc/mcp-repo-context/internal/repo"
	"github.com/yashpalc/mcp-repo-context/internal/storage"
	"github.com/yashpalc/mcp-repo-context/internal/vectors"

	_ "github.com/mattn/go-sqlite3"
)

var (
	version = "0.1.0"
)

func main() {
	// Parse flags
	storagePath := flag.String("storage", getEnvOrDefault("MCP_STORAGE_PATH", "./data/contexts"), "Path to store context files")
	tempDir := flag.String("temp", getEnvOrDefault("MCP_TEMP_DIR", "/tmp/mcp-repos"), "Temporary directory for cloning repos")
	githubToken := flag.String("github-token", os.Getenv("GITHUB_TOKEN"), "GitHub personal access token")
	showVersion := flag.Bool("version", false, "Show version")
	flag.Parse()

	if *showVersion {
		fmt.Printf("mcp-repo-context %s\n", version)
		os.Exit(0)
	}

	// Ensure storage directory exists
	if err := os.MkdirAll(*storagePath, 0755); err != nil {
		log.Fatalf("Failed to create storage directory: %v", err)
	}

	// Open shared SQLite database
	dbPath := filepath.Join(*storagePath, "contexts.db")
	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_foreign_keys=ON")
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)

	// Create main storage with shared DB (also creates org tables)
	store, err := storage.NewSQLiteStoreWithDB(db)
	if err != nil {
		log.Fatalf("Failed to create storage: %v", err)
	}

	// Setup cloner
	cloner, err := repo.NewCloner(*tempDir)
	if err != nil {
		log.Fatalf("Failed to create cloner: %v", err)
	}

	// Setup scanner with default exclude patterns
	excludePatterns := []string{
		"node_modules/**",
		"vendor/**",
		".git/**",
		"dist/**",
		"build/**",
		"*.min.js",
		"*.min.css",
		"package-lock.json",
		"yarn.lock",
		"go.sum",
		".idea/**",
		".vscode/**",
		"*.exe",
		"*.dll",
		"*.so",
		"*.dylib",
	}
	scanner := repo.NewScanner(excludePatterns, 1024*1024) // 1MB max file size

	// Create orchestrator manager
	manager := orchestrator.NewManager(store, cloner, scanner)

	// Create comparer for multi-repo analysis
	comparer := comparison.NewComparer()

	// Create org SQLite store with shared DB (org tables created by storage above)
	orgStore, err := org.NewSQLiteStore(db)
	if err != nil {
		log.Fatalf("Failed to create org store: %v", err)
	}

	// Run filesystem migration (one-time, idempotent)
	if err := org.MigrateFromFilesystem(*storagePath, orgStore); err != nil {
		log.Printf("WARNING: org filesystem migration failed: %v", err)
	}

	// Create org manager with orchestrator for concurrent analysis
	orgManager := org.NewManager(orgStore, manager)

	// Create embedder and vector store for semantic search
	embedder := vectors.NewDefaultEmbedder()
	vectorStorePath := getEnvOrDefault("MCP_VECTOR_STORE_PATH", *storagePath+"/vectors.db")
	vectorStore := initVectorStore(vectorStorePath, embedder.Dimension())
	if vectorStore != nil {
		defer vectorStore.Close()
	}

	// Create MCP server
	serverConfig := &mcp.ServerConfig{
		Name:        "mcp-repo-context",
		Version:     version,
		GitHubToken: *githubToken,
		OrgManager:  orgManager,
		OrgSearcher: store,
		Embedder:    embedder,
		AutoIndex:   getAutoIndex(),
	}
	if vectorStore != nil {
		serverConfig.VectorStore = vectorStore
	}
	server := mcp.NewServer(manager, comparer, serverConfig)

	// Setup context with cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle shutdown signals
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		cancel()
	}()

	// Run server
	log.Println("Starting MCP server on stdio...")
	if err := server.ServeStdio(ctx); err != nil && err != context.Canceled {
		log.Fatalf("Server error: %v", err)
	}
}

func getEnvOrDefault(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

// initVectorStore creates and initializes the vector store with dimension migration.
func initVectorStore(path string, dimension int) *vectors.SQLiteVectorStore {
	store, err := vectors.NewSQLiteVectorStore(path, dimension)
	if err != nil {
		log.Printf("Warning: Vector store not available at %s (semantic search disabled): %v", path, err)
		return nil
	}
	return store
}

// getAutoIndex reads MCP_AUTO_INDEX env var (default true).
func getAutoIndex() bool {
	v := os.Getenv("MCP_AUTO_INDEX")
	if v == "" {
		return true
	}
	return strings.ToLower(v) == "true"
}
