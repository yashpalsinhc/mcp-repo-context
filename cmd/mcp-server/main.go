package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/yashpalc/mcp-repo-context/internal/comparison"
	"github.com/yashpalc/mcp-repo-context/internal/mcp"
	"github.com/yashpalc/mcp-repo-context/internal/orchestrator"
	"github.com/yashpalc/mcp-repo-context/internal/repo"
	"github.com/yashpalc/mcp-repo-context/internal/storage"
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

	// Setup storage
	store, err := storage.NewFilesystemStore(*storagePath)
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

	// Create manager
	manager := orchestrator.NewManager(store, cloner, scanner)

	// Create comparer for multi-repo analysis
	comparer := comparison.NewComparer()

	// Create MCP server
	server := mcp.NewServer(manager, comparer, &mcp.ServerConfig{
		Name:        "mcp-repo-context",
		Version:     version,
		GitHubToken: *githubToken,
	})

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
