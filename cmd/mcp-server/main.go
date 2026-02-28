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

	"github.com/yashpalc/mcp-repo-context/internal/analyzer"
	"github.com/yashpalc/mcp-repo-context/internal/api"
	"github.com/yashpalc/mcp-repo-context/internal/comparison"
	"github.com/yashpalc/mcp-repo-context/internal/logging"
	"github.com/yashpalc/mcp-repo-context/internal/mcp"
	"github.com/yashpalc/mcp-repo-context/internal/org"
	"github.com/yashpalc/mcp-repo-context/internal/orchestrator"
	"github.com/yashpalc/mcp-repo-context/internal/queue"
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
	mode := flag.String("mode", getEnvOrDefault("MCP_MODE", "mcp"), "Server mode: mcp (stdio) or http")
	listenAddr := flag.String("listen", getEnvOrDefault("MCP_LISTEN", ":8080"), "HTTP listen address (only for http mode)")
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

	// Create analyzer and embedder registries
	analyzerReg := analyzer.DefaultRegistry()
	embedderReg := vectors.DefaultEmbedderRegistry()

	// Create orchestrator manager with registries
	manager := orchestrator.NewManager(store, cloner, scanner,
		orchestrator.WithAnalyzerRegistry(analyzerReg),
	)

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

	// Setup context with signal handling
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	switch *mode {
	case "http":
		logger := logging.InitFromEnv()
		logger.Info("Starting HTTP server mode")

		apiConfig := api.DefaultAPIConfig()
		apiConfig.ListenAddr = *listenAddr
		apiConfig.GithubWebhookSecret = os.Getenv("GITHUB_WEBHOOK_SECRET")
		apiConfig.GitlabWebhookSecret = os.Getenv("GITLAB_WEBHOOK_SECRET")

		apiServer := api.NewAPIServer(manager, orgManager, apiConfig, logger)

		// Create job queue
		jobsDBPath := filepath.Join(*storagePath, "jobs.db")
		jobQueue, err := queue.NewJobQueue(jobsDBPath, manager, orgManager, 3, logger)
		if err != nil {
			log.Fatalf("Failed to create job queue: %v", err)
		}
		apiServer.SetJobQueue(jobQueue)
		jobQueue.Start(ctx)

		if err := apiServer.Start(ctx); err != nil && err != context.Canceled {
			log.Fatalf("HTTP server error: %v", err)
		}

		// Graceful shutdown: stop job queue
		jobQueue.Stop()

	default: // "mcp" mode
		// Create MCP server
		serverConfig := &mcp.ServerConfig{
			Name:             "mcp-repo-context",
			Version:          version,
			GitHubToken:      *githubToken,
			OrgManager:       orgManager,
			OrgSearcher:      store,
			Embedder:         embedder,
			EmbedderRegistry: embedderReg,
			AutoIndex:        getAutoIndex(),
		}
		if vectorStore != nil {
			serverConfig.VectorStore = vectorStore
		}
		server := mcp.NewServer(manager, comparer, serverConfig)

		log.Println("Starting MCP server on stdio...")
		if err := server.ServeStdio(ctx); err != nil && err != context.Canceled {
			log.Fatalf("Server error: %v", err)
		}
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
