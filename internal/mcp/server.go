package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/yashpalc/mcp-repo-context/internal/analytics"
	"github.com/yashpalc/mcp-repo-context/internal/comparison"
	"github.com/yashpalc/mcp-repo-context/internal/compose"
	"github.com/yashpalc/mcp-repo-context/internal/logging"
	"github.com/yashpalc/mcp-repo-context/internal/orchestrator"
	"github.com/yashpalc/mcp-repo-context/internal/skills"
	"github.com/yashpalc/mcp-repo-context/internal/vectors"
)

// Server implements the MCP protocol.
type server struct {
	manager  orchestrator.Manager
	comparer comparison.Comparer
	skills   *skills.Registry
	config   *ServerConfig
	logger   *logging.Logger

	// New integrations for vectors, tokens, compose
	semanticSearch  *vectors.SemanticSearch
	patternRegistry *compose.PatternRegistry

	// Usage analytics
	usageTracker      *analytics.UsageTracker
	trackingMiddleware *analytics.TrackingMiddleware

	mu        sync.Mutex
	nextID    int
	requestID int64 // Counter for request tracking
}

// ServerConfig configures the MCP server.
type ServerConfig struct {
	Name        string
	Version     string
	GitHubToken string

	// Optional: Enable semantic search with a vector store
	VectorStore *vectors.SQLiteVectorStore

	// Optional: Usage analytics tracker
	UsageTracker *analytics.UsageTracker
}

// Server is the MCP server interface.
type Server interface {
	// ServeStdio runs the MCP server over stdio.
	ServeStdio(ctx context.Context) error
}

// Ensure server implements Server interface.
var _ Server = (*server)(nil)

// NewServer creates a new MCP server.
func NewServer(manager orchestrator.Manager, comparer comparison.Comparer, config *ServerConfig) Server {
	s := &server{
		manager:         manager,
		comparer:        comparer,
		skills:          skills.NewRegistry(),
		config:          config,
		logger:          logging.InitFromEnv(),
		patternRegistry: compose.DefaultRegistry(),
	}

	// Initialize semantic search if vector store is provided
	if config.VectorStore != nil {
		embedder := vectors.NewDefaultEmbedder()
		s.semanticSearch = vectors.NewSemanticSearch(embedder, config.VectorStore)
	}

	// Initialize usage tracking if tracker is provided
	if config.UsageTracker != nil {
		s.usageTracker = config.UsageTracker
		s.trackingMiddleware = analytics.NewTrackingMiddleware(config.UsageTracker)
	}

	return s
}

// log logs a tool call with fields.
func (s *server) log(tool string, fields map[string]interface{}) {
	s.logger.WithField("tool", tool).WithFields(fields).Info("tool call")
}

// JSON-RPC types
type jsonRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type jsonRPCResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      any         `json:"id,omitempty"`
	Result  interface{} `json:"result,omitempty"`
	Error   *rpcError   `json:"error,omitempty"`
}

type rpcError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// MCP types
type initializeResult struct {
	ProtocolVersion string             `json:"protocolVersion"`
	Capabilities    serverCapabilities `json:"capabilities"`
	ServerInfo      serverInfo         `json:"serverInfo"`
}

type serverCapabilities struct {
	Tools     *toolsCapability     `json:"tools,omitempty"`
	Resources *resourcesCapability `json:"resources,omitempty"`
}

type toolsCapability struct {
	ListChanged bool `json:"listChanged,omitempty"`
}

type resourcesCapability struct {
	Subscribe   bool `json:"subscribe,omitempty"`
	ListChanged bool `json:"listChanged,omitempty"`
}

type serverInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type listToolsResult struct {
	Tools []toolDefinition `json:"tools"`
}

type toolDefinition struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"inputSchema"`
}

type callToolParams struct {
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments"`
}

type callToolResult struct {
	Content []contentItem `json:"content"`
	IsError bool          `json:"isError,omitempty"`
}

type contentItem struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// ServeStdio runs the MCP server over stdio.
func (s *server) ServeStdio(ctx context.Context) error {
	reader := bufio.NewReader(os.Stdin)
	writer := os.Stdout

	s.logger.Info("MCP server ready, waiting for requests...")

	for {
		select {
		case <-ctx.Done():
			s.logger.Info("context cancelled, shutting down")
			return ctx.Err()
		default:
		}

		// Read line
		line, err := reader.ReadBytes('\n')
		if err != nil {
			if err == io.EOF {
				s.logger.Info("EOF received, shutting down")
				return nil
			}
			s.logger.Errorf("read error: %v", err)
			return fmt.Errorf("read error: %w", err)
		}

		// Generate request ID for tracking
		s.mu.Lock()
		s.requestID++
		reqID := s.requestID
		s.mu.Unlock()

		startTime := time.Now()

		// Log raw request (truncate if too long)
		rawReq := string(line)
		if len(rawReq) > 500 {
			s.logger.WithFields(map[string]interface{}{
				"req_id":   reqID,
				"size":     len(rawReq),
				"truncated": true,
			}).Debug("incoming request: %s...", rawReq[:500])
		} else {
			s.logger.WithFields(map[string]interface{}{
				"req_id": reqID,
				"size":   len(rawReq),
			}).Debug("incoming request: %s", rawReq)
		}

		// Parse request
		var req jsonRPCRequest
		if err := json.Unmarshal(line, &req); err != nil {
			s.logger.WithField("req_id", reqID).Errorf("JSON parse error: %v", err)
			s.writeError(writer, nil, -32700, "Parse error", err)
			continue
		}

		s.logger.WithFields(map[string]interface{}{
			"req_id": reqID,
			"method": req.Method,
			"rpc_id": req.ID,
		}).Info("request received")

		// Handle request
		resp := s.handleRequestWithID(ctx, &req, reqID)

		// Write response
		respBytes, err := json.Marshal(resp)
		if err != nil {
			s.logger.WithField("req_id", reqID).Errorf("failed to marshal response: %v", err)
			s.writeError(writer, req.ID, -32603, "Internal error", err)
			continue
		}

		duration := time.Since(startTime)

		// Log response details
		respSize := len(respBytes)
		logFields := map[string]interface{}{
			"req_id":      reqID,
			"method":      req.Method,
			"resp_size":   respSize,
			"duration_ms": duration.Milliseconds(),
		}

		// Check if response has error
		if resp.Error != nil {
			logFields["error_code"] = resp.Error.Code
			logFields["error_msg"] = resp.Error.Message
			s.logger.WithFields(logFields).Warn("sending error response")
		} else {
			s.logger.WithFields(logFields).Info("sending response")
		}

		// Log response content for debugging (truncated)
		respStr := string(respBytes)
		if len(respStr) > 1000 {
			s.logger.WithField("req_id", reqID).Debug("response content (truncated): %s...", respStr[:1000])
		} else {
			s.logger.WithField("req_id", reqID).Debug("response content: %s", respStr)
		}

		// Write response and check for errors
		if _, err := writer.Write(respBytes); err != nil {
			s.logger.WithField("req_id", reqID).Errorf("failed to write response: %v", err)
		}
		if _, err := writer.Write([]byte("\n")); err != nil {
			s.logger.WithField("req_id", reqID).Errorf("failed to write newline: %v", err)
		}
	}
}

func (s *server) handleRequest(ctx context.Context, req *jsonRPCRequest) *jsonRPCResponse {
	return s.handleRequestWithID(ctx, req, 0)
}

func (s *server) handleRequestWithID(ctx context.Context, req *jsonRPCRequest, reqID int64) *jsonRPCResponse {
	logger := s.logger.WithField("req_id", reqID)

	switch req.Method {
	case "initialize":
		logger.Info("initializing MCP server")
		return s.handleInitialize(req)
	case "initialized":
		logger.Info("MCP server initialized")
		return &jsonRPCResponse{JSONRPC: "2.0", ID: req.ID, Result: struct{}{}}
	case "notifications/initialized":
		logger.Debug("received notifications/initialized")
		return &jsonRPCResponse{JSONRPC: "2.0", ID: req.ID, Result: struct{}{}}
	case "tools/list":
		logger.Debug("listing tools")
		return s.handleListTools(req)
	case "tools/call":
		return s.handleCallToolWithID(ctx, req, reqID)
	case "resources/list":
		logger.Debug("listing resources")
		return s.handleListResources(ctx, req)
	case "resources/read":
		logger.Debug("reading resource")
		return s.handleReadResource(ctx, req)
	case "ping":
		logger.Debug("ping received")
		return &jsonRPCResponse{JSONRPC: "2.0", ID: req.ID, Result: struct{}{}}
	default:
		logger.Warnf("unknown method: %s", req.Method)
		return &jsonRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &rpcError{Code: -32601, Message: "Method not found"},
		}
	}
}

func (s *server) handleInitialize(req *jsonRPCRequest) *jsonRPCResponse {
	return &jsonRPCResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result: initializeResult{
			ProtocolVersion: "2024-11-05",
			Capabilities: serverCapabilities{
				Tools:     &toolsCapability{ListChanged: false},
				Resources: &resourcesCapability{Subscribe: false, ListChanged: false},
			},
			ServerInfo: serverInfo{
				Name:    s.config.Name,
				Version: s.config.Version,
			},
		},
	}
}

func (s *server) handleListTools(req *jsonRPCRequest) *jsonRPCResponse {
	tools := []toolDefinition{
		{
			Name:        "analyze_repo",
			Description: "Analyze a GitHub repository and generate comprehensive context for AI Q&A. Clones the repo, extracts code structure, functions, types, and relationships.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"repo_url": map[string]interface{}{
						"type":        "string",
						"description": "GitHub repository URL (e.g., https://github.com/owner/repo)",
					},
					"branch": map[string]interface{}{
						"type":        "string",
						"description": "Branch to analyze (default: default branch)",
					},
					"force": map[string]interface{}{
						"type":        "boolean",
						"description": "Force re-analysis even if cached context exists",
					},
				},
				"required": []string{"repo_url"},
			},
		},
		{
			Name:        "get_context",
			Description: "Retrieve stored context for a repository. Returns architecture overview, file details, functions, types, and relationships.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"repo_id": map[string]interface{}{
						"type":        "string",
						"description": "Repository identifier (from analyze_repo result or list_repos)",
					},
					"scope": map[string]interface{}{
						"type":        "string",
						"enum":        []string{"full", "architecture", "file"},
						"description": "Scope of context to retrieve (default: full)",
					},
					"file_path": map[string]interface{}{
						"type":        "string",
						"description": "File path (required if scope is 'file')",
					},
				},
				"required": []string{"repo_id"},
			},
		},
		{
			Name:        "list_repos",
			Description: "List all analyzed repositories with their metadata.",
			InputSchema: map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
			},
		},
		{
			Name:        "search_context",
			Description: "Search for code elements across analyzed repositories by name, type, or concept. Returns compact results with references for progressive disclosure.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"query": map[string]interface{}{
						"type":        "string",
						"description": "Search query (function name, type name, or concept)",
					},
					"repo_id": map[string]interface{}{
						"type":        "string",
						"description": "Limit search to specific repository",
					},
					"search_type": map[string]interface{}{
						"type":        "string",
						"enum":        []string{"function", "type", "file", "all"},
						"description": "Type of elements to search (default: all)",
					},
					"max_results": map[string]interface{}{
						"type":        "integer",
						"description": "Maximum number of results to return (default: 20, max: 100)",
					},
					"compact": map[string]interface{}{
						"type":        "boolean",
						"description": "Return compact results with detail_ref for drilling down (default: true)",
					},
				},
				"required": []string{"query"},
			},
		},
		{
			Name:        "compare_repos",
			Description: "Compare multiple repositories to identify duplicates, conflicts, gaps, and consistency issues. Useful for repo consolidation/merge planning.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"repo_ids": map[string]interface{}{
						"type":        "array",
						"items":       map[string]interface{}{"type": "string"},
						"description": "List of repository IDs to compare",
					},
					"target_repo_id": map[string]interface{}{
						"type":        "string",
						"description": "Target repository ID for merge analysis (optional)",
					},
					"include_duplicates": map[string]interface{}{
						"type":        "boolean",
						"description": "Include duplicate detection (default: true)",
					},
					"include_conflicts": map[string]interface{}{
						"type":        "boolean",
						"description": "Include conflict detection (default: true)",
					},
					"include_gaps": map[string]interface{}{
						"type":        "boolean",
						"description": "Include gap detection (default: true)",
					},
				},
				"required": []string{"repo_ids"},
			},
		},
		{
			Name:        "find_duplicates",
			Description: "Find duplicate functions, types, and patterns across multiple repositories.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"repo_ids": map[string]interface{}{
						"type":        "array",
						"items":       map[string]interface{}{"type": "string"},
						"description": "List of repository IDs to search for duplicates",
					},
				},
				"required": []string{"repo_ids"},
			},
		},
		{
			Name:        "find_conflicts",
			Description: "Find conflicting implementations between source repositories and a target repository.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"source_repo_ids": map[string]interface{}{
						"type":        "array",
						"items":       map[string]interface{}{"type": "string"},
						"description": "Source repository IDs",
					},
					"target_repo_id": map[string]interface{}{
						"type":        "string",
						"description": "Target repository ID",
					},
				},
				"required": []string{"source_repo_ids", "target_repo_id"},
			},
		},
		{
			Name:        "find_gaps",
			Description: "Find functionality in source repositories that is missing from the target repository.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"source_repo_ids": map[string]interface{}{
						"type":        "array",
						"items":       map[string]interface{}{"type": "string"},
						"description": "Source repository IDs",
					},
					"target_repo_id": map[string]interface{}{
						"type":        "string",
						"description": "Target repository ID",
					},
				},
				"required": []string{"source_repo_ids", "target_repo_id"},
			},
		},
		{
			Name:        "generate_ai_summary",
			Description: "Generate an AI-powered summary of a repository using Claude. Provides intelligent overview, purpose, key features, technology stack, and architecture analysis. Requires ANTHROPIC_API_KEY environment variable.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"repo_id": map[string]interface{}{
						"type":        "string",
						"description": "Repository ID to generate summary for (must be previously analyzed)",
					},
				},
				"required": []string{"repo_id"},
			},
		},
		{
			Name:        "generate_ai_arch_analysis",
			Description: "Generate an AI-powered architecture analysis of a repository using Claude. Identifies architecture patterns, layers, data flow, strengths, weaknesses, and recommendations. Requires ANTHROPIC_API_KEY environment variable.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"repo_id": map[string]interface{}{
						"type":        "string",
						"description": "Repository ID to analyze (must be previously analyzed)",
					},
				},
				"required": []string{"repo_id"},
			},
		},
		{
			Name:        "ask",
			Description: "Ask a natural language question about the analyzed repositories. The AI will automatically find relevant context and provide a focused answer. Requires ANTHROPIC_API_KEY. Examples: 'How does authentication work?', 'Where is the main entry point?', 'What does the Handler interface do?'",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"query": map[string]interface{}{
						"type":        "string",
						"description": "Your question about the code (natural language)",
					},
					"repo_ids": map[string]interface{}{
						"type":        "array",
						"items":       map[string]interface{}{"type": "string"},
						"description": "Limit search to specific repositories (optional, searches all if empty)",
					},
				},
				"required": []string{"query"},
			},
		},
		{
			Name:        "refresh_ai_context",
			Description: "Update existing repositories with AI-generated summaries and architecture analysis. Run this after adding ANTHROPIC_API_KEY to enrich all previously analyzed repos with AI insights. Use force=true to regenerate AI context even if it already exists.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"repo_ids": map[string]interface{}{
						"type":        "array",
						"items":       map[string]interface{}{"type": "string"},
						"description": "Specific repositories to update (optional, updates all if empty)",
					},
					"force": map[string]interface{}{
						"type":        "boolean",
						"description": "If true, regenerate AI context even for repos that already have it (default: false)",
					},
				},
			},
		},
		{
			Name:        "review_pr",
			Description: "Review a GitHub pull request using AI with codebase context. Analyzes code changes for bugs, security issues, performance problems, and adherence to patterns. Can add comments directly to the PR. Only reviews repositories that have pre-analyzed context (use analyze_repo first). Requires ANTHROPIC_API_KEY and GITHUB_TOKEN.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"pr_url": map[string]interface{}{
						"type":        "string",
						"description": "GitHub pull request URL (e.g., https://github.com/owner/repo/pull/123)",
					},
					"add_comments": map[string]interface{}{
						"type":        "boolean",
						"description": "Add review comments directly to the PR (default: true)",
					},
					"severity_level": map[string]interface{}{
						"type":        "string",
						"enum":        []string{"critical", "important", "all"},
						"description": "Filter comments by severity level (default: all)",
					},
					"focus_areas": map[string]interface{}{
						"type":        "array",
						"items":       map[string]interface{}{"type": "string"},
						"description": "Areas to focus on: security, performance, architecture, testing, etc.",
					},
					"skip_context": map[string]interface{}{
						"type":        "boolean",
						"description": "Skip using repository context (limited review without codebase understanding)",
					},
					"generate_context": map[string]interface{}{
						"type":        "boolean",
						"description": "Automatically generate context if not available (not recommended, use analyze_repo first)",
					},
				},
				"required": []string{"pr_url"},
			},
		},
		{
			Name:        "list_skills",
			Description: "List all available built-in skills. Skills provide specialized prompts and guidance for tasks like code review, Go development, security analysis, testing, and documentation.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"category": map[string]interface{}{
						"type":        "string",
						"description": "Filter by category: code-review, development, analysis, security, performance, documentation",
					},
				},
			},
		},
		{
			Name:        "get_skill",
			Description: "Get a specific skill's full prompt and instructions. Use this to retrieve detailed guidance for a particular task type like 'pr-review', 'go-expert', 'security-review', etc.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"name": map[string]interface{}{
						"type":        "string",
						"description": "Skill name (e.g., 'pr-review', 'go-expert', 'security-review', 'performance-review', 'test-generation', 'refactoring', 'documentation', 'code-analysis')",
					},
				},
				"required": []string{"name"},
			},
		},
		// NEW: Deep context tools (no AI required)
		{
			Name:        "get_function_context",
			Description: "Get comprehensive context for a specific function including behavior summary, what it calls, what calls it, side effects, and error handling. Does NOT require AI - uses pre-analyzed context.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"repo_id": map[string]interface{}{
						"type":        "string",
						"description": "Repository ID",
					},
					"file_path": map[string]interface{}{
						"type":        "string",
						"description": "Path to the file containing the function",
					},
					"function_name": map[string]interface{}{
						"type":        "string",
						"description": "Name of the function to get context for",
					},
				},
				"required": []string{"repo_id", "file_path", "function_name"},
			},
		},
		{
			Name:        "search_by_concept",
			Description: "Search for functions related to a concept (e.g., 'authentication', 'validation', 'database', 'http'). Does NOT require AI - uses pre-built search index.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"repo_id": map[string]interface{}{
						"type":        "string",
						"description": "Repository ID to search in",
					},
					"concept": map[string]interface{}{
						"type":        "string",
						"description": "Concept to search for (e.g., 'authentication', 'validation', 'http_call', 'database', 'crud', 'handler')",
					},
					"max_results": map[string]interface{}{
						"type":        "integer",
						"description": "Maximum number of results to return (default: 20)",
					},
				},
				"required": []string{"repo_id", "concept"},
			},
		},
		{
			Name:        "search_by_side_effect",
			Description: "Find functions that have specific side effects (e.g., 'http_call', 'db_query', 'file_io'). Does NOT require AI - uses pre-analyzed context.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"repo_id": map[string]interface{}{
						"type":        "string",
						"description": "Repository ID to search in",
					},
					"effect": map[string]interface{}{
						"type":        "string",
						"description": "Side effect to search for: http_call, db_query, db_transaction, file_io, redis_call, kafka_call, grpc_call, logging, panic",
					},
					"max_results": map[string]interface{}{
						"type":        "integer",
						"description": "Maximum number of results to return (default: 20)",
					},
				},
				"required": []string{"repo_id", "effect"},
			},
		},
		{
			Name:        "get_callers",
			Description: "Find all functions that call a specific function. Does NOT require AI - uses pre-built call graph.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"repo_id": map[string]interface{}{
						"type":        "string",
						"description": "Repository ID",
					},
					"function_name": map[string]interface{}{
						"type":        "string",
						"description": "Name of the function to find callers for",
					},
				},
				"required": []string{"repo_id", "function_name"},
			},
		},
		// NEW: Local directory analysis (no GitHub required)
		{
			Name:        "analyze_local",
			Description: "Analyze a local directory and store the context. Use this for any codebase on disk, not just GitHub repos. Does NOT require network access.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"path": map[string]interface{}{
						"type":        "string",
						"description": "Absolute path to the local directory to analyze",
					},
					"force": map[string]interface{}{
						"type":        "boolean",
						"description": "Force re-analysis even if cached context exists (default: false)",
					},
					"include_all": map[string]interface{}{
						"type":        "boolean",
						"description": "Include all files, not just code files (default: false)",
					},
				},
				"required": []string{"path"},
			},
		},
		{
			Name:        "smart_query",
			Description: "Ask a question about a project and get an intelligent answer without AI. Automatically parses your query and routes to appropriate tools. Use for: 'what does X do?', 'who calls Y?', 'find DB functions', 'show auth code', etc.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"query": map[string]interface{}{
						"type":        "string",
						"description": "Your question about the code in natural language",
					},
					"project_id": map[string]interface{}{
						"type":        "string",
						"description": "Project ID (repo ID or local:path for local projects)",
					},
				},
				"required": []string{"query", "project_id"},
			},
		},
		// Package structure exploration
		{
			Name:        "get_package_structure",
			Description: "Get detailed structure of a package/directory including all files, their purposes, types, and key functions. Use when you need to understand the layout and contents of a specific package or folder in the codebase.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"project_id": map[string]interface{}{
						"type":        "string",
						"description": "Project ID (repo ID or local:path for local projects)",
					},
					"package_path": map[string]interface{}{
						"type":        "string",
						"description": "Path to the package/directory (e.g., 'service/test/create', 'pkg/handlers')",
					},
				},
				"required": []string{"project_id", "package_path"},
			},
		},
		// Incremental update tools for refactoring workflows
		{
			Name:        "refresh_file",
			Description: "Re-analyze a single file after editing it. Much faster than full re-analysis (~10ms vs seconds). Use this during refactoring to keep context up-to-date.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"project_id": map[string]interface{}{
						"type":        "string",
						"description": "Project ID (local:path for local projects)",
					},
					"file_path": map[string]interface{}{
						"type":        "string",
						"description": "Relative path to the file within the project",
					},
					"force": map[string]interface{}{
						"type":        "boolean",
						"description": "Force refresh even if file hash hasn't changed (default: false)",
					},
				},
				"required": []string{"project_id", "file_path"},
			},
		},
		{
			Name:        "refresh_changed",
			Description: "Check all files in a project and refresh only those that have changed. Use after a refactoring session to update context for all modified files.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"project_id": map[string]interface{}{
						"type":        "string",
						"description": "Project ID (local:path for local projects)",
					},
				},
				"required": []string{"project_id"},
			},
		},
		{
			Name:        "get_pr_context",
			Description: "Get rich context for PR changes WITHOUT AI. Shows for each changed function: what it does, who calls it, what it calls, DB queries, HTTP calls, and impact analysis. Use this to understand PR changes deeply.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"repo_id": map[string]interface{}{
						"type":        "string",
						"description": "Repository ID (e.g., github.com/org/repo)",
					},
					"changed_files": map[string]interface{}{
						"type": "array",
						"items": map[string]interface{}{
							"type": "object",
							"properties": map[string]interface{}{
								"path": map[string]interface{}{
									"type":        "string",
									"description": "File path relative to repo root",
								},
								"change_type": map[string]interface{}{
									"type":        "string",
									"enum":        []string{"added", "modified", "deleted"},
									"description": "Type of change",
								},
							},
							"required": []string{"path", "change_type"},
						},
						"description": "List of changed files in the PR",
					},
				},
				"required": []string{"repo_id", "changed_files"},
			},
		},
		// Call graph visualization
		{
			Name:        "visualize_call_graph",
			Description: "Generate a visual representation of function call relationships. Returns either a Mermaid diagram or DOT format (for Graphviz). Shows both callers (what calls this function) and callees (what this function calls) up to the specified depth.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"repo_id": map[string]interface{}{
						"type":        "string",
						"description": "Repository ID to visualize",
					},
					"function_name": map[string]interface{}{
						"type":        "string",
						"description": "Name of the function to visualize call relationships for",
					},
					"format": map[string]interface{}{
						"type":        "string",
						"enum":        []string{"mermaid", "dot"},
						"description": "Output format: 'mermaid' for Mermaid flowchart, 'dot' for Graphviz DOT (default: mermaid)",
					},
					"depth": map[string]interface{}{
						"type":        "integer",
						"description": "How many levels of callers/callees to include (default: 2, max: 5)",
					},
				},
				"required": []string{"repo_id", "function_name"},
			},
		},
		// NEW: Semantic search tools (using internal/vectors)
		{
			Name:        "semantic_search",
			Description: "Search for code using semantic similarity. Finds functions and types that are conceptually similar to your query, even if they don't contain exact keyword matches. Requires repository to be indexed first with index_repository.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"query": map[string]interface{}{
						"type":        "string",
						"description": "Natural language query describing what you're looking for",
					},
					"repo_id": map[string]interface{}{
						"type":        "string",
						"description": "Repository ID to search in",
					},
					"limit": map[string]interface{}{
						"type":        "integer",
						"description": "Maximum number of results (default: 10, max: 50)",
					},
					"type": map[string]interface{}{
						"type":        "string",
						"enum":        []string{"function", "type", "all"},
						"description": "Type of items to search (default: all)",
					},
				},
				"required": []string{"query", "repo_id"},
			},
		},
		{
			Name:        "index_repository",
			Description: "Index a repository for semantic search. Creates vector embeddings for all functions and types. Required before using semantic_search.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"repo_id": map[string]interface{}{
						"type":        "string",
						"description": "Repository ID to index (must be previously analyzed)",
					},
					"force": map[string]interface{}{
						"type":        "boolean",
						"description": "Force re-indexing even if already indexed (default: false)",
					},
				},
				"required": []string{"repo_id"},
			},
		},
		// NEW: Token-budgeted context (using internal/tokens)
		{
			Name:        "get_context_budgeted",
			Description: "Get relevant context that fits within a token budget. Automatically selects and prioritizes the most relevant functions based on your query, fitting as much useful context as possible within the budget.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"repo_id": map[string]interface{}{
						"type":        "string",
						"description": "Repository ID to get context from",
					},
					"query": map[string]interface{}{
						"type":        "string",
						"description": "Query describing what context you need",
					},
					"token_budget": map[string]interface{}{
						"type":        "integer",
						"description": "Maximum tokens for the context (default: 4000, max: 32000)",
					},
				},
				"required": []string{"repo_id", "query"},
			},
		},
		// NEW: Compose pattern tools (using internal/compose)
		{
			Name:        "execute_pattern",
			Description: "Execute a pre-defined pattern of tool calls. Patterns automate common workflows like searching then expanding details, or analyzing impact of changes.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"pattern_name": map[string]interface{}{
						"type":        "string",
						"description": "Name of the pattern to execute (use list_patterns to see available)",
					},
					"params": map[string]interface{}{
						"type":        "object",
						"description": "Parameters for the pattern (varies by pattern)",
					},
				},
				"required": []string{"pattern_name"},
			},
		},
		{
			Name:        "list_patterns",
			Description: "List available patterns for execute_pattern. Patterns are pre-defined tool chains that automate common workflows.",
			InputSchema: map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
			},
		},
		// NEW: Usage analytics tool
		{
			Name:        "get_usage_stats",
			Description: "Get usage statistics for MCP tools including token counts, call counts, and performance metrics. Useful for monitoring and optimizing tool usage.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"tool": map[string]interface{}{
						"type":        "string",
						"description": "Filter stats for a specific tool (optional, shows all if not specified)",
					},
					"since_hours": map[string]interface{}{
						"type":        "integer",
						"description": "Show stats from the last N hours (optional, shows all time if not specified)",
					},
					"limit": map[string]interface{}{
						"type":        "integer",
						"description": "Limit recent usage records (default: 10, max: 50)",
					},
				},
			},
		},
	}

	return &jsonRPCResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  listToolsResult{Tools: tools},
	}
}

func (s *server) handleCallTool(ctx context.Context, req *jsonRPCRequest) *jsonRPCResponse {
	return s.handleCallToolWithID(ctx, req, 0)
}

func (s *server) handleCallToolWithID(ctx context.Context, req *jsonRPCRequest, reqID int64) *jsonRPCResponse {
	var params callToolParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		s.logger.WithField("req_id", reqID).Errorf("invalid tool params: %v", err)
		return &jsonRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &rpcError{Code: -32602, Message: "Invalid params"},
		}
	}

	// Log tool arguments
	argsJSON, _ := json.Marshal(params.Arguments)
	logger := s.logger.WithFields(map[string]interface{}{
		"req_id": reqID,
		"tool":   params.Name,
	})

	logger.WithField("args", string(argsJSON)).Info("tool call started")

	toolStart := time.Now()
	var result callToolResult

	switch params.Name {
	case "analyze_repo":
		result = s.toolAnalyzeRepo(ctx, params.Arguments)
	case "get_context":
		result = s.toolGetContext(ctx, params.Arguments)
	case "list_repos":
		result = s.toolListRepos(ctx, params.Arguments)
	case "search_context":
		result = s.toolSearchContext(ctx, params.Arguments)
	case "compare_repos":
		result = s.toolCompareRepos(ctx, params.Arguments)
	case "find_duplicates":
		result = s.toolFindDuplicates(ctx, params.Arguments)
	case "find_conflicts":
		result = s.toolFindConflicts(ctx, params.Arguments)
	case "find_gaps":
		result = s.toolFindGaps(ctx, params.Arguments)
	case "generate_ai_summary":
		result = s.toolGenerateAISummary(ctx, params.Arguments)
	case "generate_ai_arch_analysis":
		result = s.toolGenerateAIArchAnalysis(ctx, params.Arguments)
	case "ask":
		result = s.toolAsk(ctx, params.Arguments)
	case "refresh_ai_context":
		result = s.toolRefreshAIContext(ctx, params.Arguments)
	case "review_pr":
		result = s.toolReviewPR(ctx, params.Arguments)
	case "list_skills":
		result = s.toolListSkills(ctx, params.Arguments)
	case "get_skill":
		result = s.toolGetSkill(ctx, params.Arguments)
	// NEW: Deep context tools (no AI required)
	case "get_function_context":
		result = s.toolGetFunctionContext(ctx, params.Arguments)
	case "search_by_concept":
		result = s.toolSearchByConcept(ctx, params.Arguments)
	case "search_by_side_effect":
		result = s.toolSearchBySideEffect(ctx, params.Arguments)
	case "get_callers":
		result = s.toolGetCallers(ctx, params.Arguments)
	// NEW: Local directory analysis
	case "analyze_local":
		result = s.toolAnalyzeLocal(ctx, params.Arguments)
	case "smart_query":
		result = s.toolSmartQuery(ctx, params.Arguments)
	case "get_package_structure":
		result = s.toolGetPackageStructure(ctx, params.Arguments)
	case "refresh_file":
		result = s.toolRefreshFile(ctx, params.Arguments)
	case "refresh_changed":
		result = s.toolRefreshChanged(ctx, params.Arguments)
	case "get_pr_context":
		result = s.toolGetPRContext(ctx, params.Arguments)
	// Call graph visualization
	case "visualize_call_graph":
		result = s.toolVisualizeCallGraph(ctx, params.Arguments)
	// NEW: Semantic search tools (using internal/vectors)
	case "semantic_search":
		result = s.toolSemanticSearch(ctx, params.Arguments)
	case "index_repository":
		result = s.toolIndexRepository(ctx, params.Arguments)
	// NEW: Token-budgeted context (using internal/tokens)
	case "get_context_budgeted":
		result = s.toolGetContextBudgeted(ctx, params.Arguments)
	// NEW: Compose pattern tools (using internal/compose)
	case "execute_pattern":
		result = s.toolExecutePattern(ctx, params.Arguments)
	case "list_patterns":
		result = s.toolListPatterns(ctx, params.Arguments)
	// NEW: Usage analytics
	case "get_usage_stats":
		result = s.toolGetUsageStats(ctx, params.Arguments)
	default:
		logger.Warn("unknown tool requested")
		return &jsonRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &rpcError{Code: -32602, Message: "Unknown tool: " + params.Name},
		}
	}

	toolDuration := time.Since(toolStart)

	// Calculate response content size
	contentSize := 0
	for _, item := range result.Content {
		contentSize += len(item.Text)
	}

	logFields := map[string]interface{}{
		"req_id":           reqID,
		"tool":             params.Name,
		"tool_duration_ms": toolDuration.Milliseconds(),
		"content_size":     contentSize,
		"is_error":         result.IsError,
	}

	if result.IsError {
		// Log the error content
		if len(result.Content) > 0 {
			logFields["error_text"] = result.Content[0].Text
		}
		s.logger.WithFields(logFields).Warn("tool call failed")
	} else {
		s.logger.WithFields(logFields).Info("tool call completed")
		// Log first part of response content for debugging
		if len(result.Content) > 0 && len(result.Content[0].Text) > 0 {
			preview := result.Content[0].Text
			if len(preview) > 200 {
				preview = preview[:200] + "..."
			}
			s.logger.WithField("req_id", reqID).Debug("response preview: %s", preview)
		}
	}

	return &jsonRPCResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  result,
	}
}

func (s *server) writeError(w io.Writer, id interface{}, code int, message string, err error) {
	resp := jsonRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error: &rpcError{
			Code:    code,
			Message: message,
			Data:    err.Error(),
		},
	}
	data, _ := json.Marshal(resp)
	w.Write(data)
	w.Write([]byte("\n"))
}
