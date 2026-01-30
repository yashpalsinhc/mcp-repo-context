package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// MCP Resource types
type ResourceDefinition struct {
	URI         string `json:"uri"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	MimeType    string `json:"mimeType,omitempty"`
}

type listResourcesResult struct {
	Resources []ResourceDefinition `json:"resources"`
}

type readResourceParams struct {
	URI string `json:"uri"`
}

type readResourceResult struct {
	Contents []ResourceContent `json:"contents"`
}

type ResourceContent struct {
	URI      string `json:"uri"`
	MimeType string `json:"mimeType,omitempty"`
	Text     string `json:"text,omitempty"`
}

// handleListResources returns available resources.
func (s *server) handleListResources(ctx context.Context, req *jsonRPCRequest) *jsonRPCResponse {
	repos, err := s.manager.ListRepos(ctx)
	if err != nil {
		return &jsonRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &rpcError{Code: -32603, Message: fmt.Sprintf("Failed to list repos: %v", err)},
		}
	}

	var resources []ResourceDefinition

	// Add resources for each analyzed repository
	for _, repo := range repos {
		repoID := repo.RepoID

		// Overview resource - high level stats
		resources = append(resources, ResourceDefinition{
			URI:         fmt.Sprintf("repo://%s/overview", repoID),
			Name:        fmt.Sprintf("%s - Overview", repoID),
			Description: "High-level repository summary with statistics (~500 tokens)",
			MimeType:    "application/json",
		})

		// Architecture resource - module structure
		resources = append(resources, ResourceDefinition{
			URI:         fmt.Sprintf("repo://%s/architecture", repoID),
			Name:        fmt.Sprintf("%s - Architecture", repoID),
			Description: "Architecture overview with modules and entry points (~1000 tokens)",
			MimeType:    "application/json",
		})

		// Functions index - list of all functions
		resources = append(resources, ResourceDefinition{
			URI:         fmt.Sprintf("repo://%s/functions", repoID),
			Name:        fmt.Sprintf("%s - Functions Index", repoID),
			Description: "Index of all public functions with signatures (~2000 tokens)",
			MimeType:    "application/json",
		})

		// Types index - list of all types
		resources = append(resources, ResourceDefinition{
			URI:         fmt.Sprintf("repo://%s/types", repoID),
			Name:        fmt.Sprintf("%s - Types Index", repoID),
			Description: "Index of all public types (~1000 tokens)",
			MimeType:    "application/json",
		})
	}

	// Add tool hints resource
	resources = append(resources, ResourceDefinition{
		URI:         "mcp://tool-hints",
		Name:        "Tool Selection Guide",
		Description: "Metadata about tools to help with selection (output sizes, costs, use cases)",
		MimeType:    "application/json",
	})

	return &jsonRPCResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  listResourcesResult{Resources: resources},
	}
}

// handleReadResource reads a specific resource.
func (s *server) handleReadResource(ctx context.Context, req *jsonRPCRequest) *jsonRPCResponse {
	var params readResourceParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return &jsonRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &rpcError{Code: -32602, Message: "Invalid params"},
		}
	}

	uri := params.URI

	// Handle tool hints resource
	if uri == "mcp://tool-hints" {
		return s.readToolHintsResource(req)
	}

	// Parse repo:// URIs
	if !strings.HasPrefix(uri, "repo://") {
		return &jsonRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &rpcError{Code: -32602, Message: "Invalid URI scheme"},
		}
	}

	// Extract repo ID and resource type
	path := strings.TrimPrefix(uri, "repo://")
	parts := strings.SplitN(path, "/", 2)
	if len(parts) != 2 {
		return &jsonRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &rpcError{Code: -32602, Message: "Invalid URI format"},
		}
	}

	repoID := parts[0]
	resourceType := parts[1]

	switch resourceType {
	case "overview":
		return s.readOverviewResource(ctx, req, repoID)
	case "architecture":
		return s.readArchitectureResource(ctx, req, repoID)
	case "functions":
		return s.readFunctionsResource(ctx, req, repoID)
	case "types":
		return s.readTypesResource(ctx, req, repoID)
	default:
		return &jsonRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &rpcError{Code: -32602, Message: fmt.Sprintf("Unknown resource type: %s", resourceType)},
		}
	}
}

func (s *server) readToolHintsResource(req *jsonRPCRequest) *jsonRPCResponse {
	hints := GetToolHints()

	// Format as helpful guide
	type ToolGuide struct {
		OutputSizes map[string][]string `json:"output_sizes"`
		FreeTools   []string            `json:"free_tools"`
		AITools     []string            `json:"ai_required_tools"`
		AllHints    map[string]*ToolHints `json:"all_hints"`
	}

	guide := ToolGuide{
		OutputSizes: GetToolsByOutputSize(),
		FreeTools:   GetFreeTools(),
		AITools:     []string{},
		AllHints:    hints,
	}

	for name, hint := range hints {
		if hint.RequiresAI {
			guide.AITools = append(guide.AITools, name)
		}
	}

	content, _ := json.MarshalIndent(guide, "", "  ")

	return &jsonRPCResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result: readResourceResult{
			Contents: []ResourceContent{{
				URI:      "mcp://tool-hints",
				MimeType: "application/json",
				Text:     string(content),
			}},
		},
	}
}

func (s *server) readOverviewResource(ctx context.Context, req *jsonRPCRequest, repoID string) *jsonRPCResponse {
	repoCtx, err := s.manager.GetContext(ctx, repoID)
	if err != nil {
		return &jsonRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &rpcError{Code: -32603, Message: fmt.Sprintf("Failed to get context: %v", err)},
		}
	}

	// Create compact overview (predictable size ~500 tokens)
	overview := map[string]any{
		"repo_id":     repoCtx.ID,
		"url":         repoCtx.URL,
		"branch":      repoCtx.Branch,
		"commit":      repoCtx.CommitHash,
		"analyzed_at": repoCtx.AnalyzedAt.Format(time.RFC3339),
		"statistics": map[string]any{
			"files":     repoCtx.Statistics.TotalFiles,
			"lines":     repoCtx.Statistics.TotalLines,
			"functions": repoCtx.Statistics.FunctionCount,
			"types":     repoCtx.Statistics.TypeCount,
			"languages": repoCtx.Statistics.LanguageBreakdown,
		},
	}

	// Add AI summary if available
	if repoCtx.AISummary != nil {
		overview["ai_summary"] = map[string]any{
			"overview": repoCtx.AISummary.Overview,
			"purpose":  repoCtx.AISummary.Purpose,
		}
	}

	content, _ := json.MarshalIndent(overview, "", "  ")

	return &jsonRPCResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result: readResourceResult{
			Contents: []ResourceContent{{
				URI:      fmt.Sprintf("repo://%s/overview", repoID),
				MimeType: "application/json",
				Text:     string(content),
			}},
		},
	}
}

func (s *server) readArchitectureResource(ctx context.Context, req *jsonRPCRequest, repoID string) *jsonRPCResponse {
	repoCtx, err := s.manager.GetContext(ctx, repoID)
	if err != nil {
		return &jsonRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &rpcError{Code: -32603, Message: fmt.Sprintf("Failed to get context: %v", err)},
		}
	}

	arch := map[string]any{
		"repo_id": repoCtx.ID,
	}

	if repoCtx.Architecture != nil {
		arch["overview"] = repoCtx.Architecture.Overview
		arch["build_system"] = repoCtx.Architecture.BuildSystem

		// Entry points
		entryPoints := make([]map[string]string, 0)
		for _, ep := range repoCtx.Architecture.EntryPoints {
			entryPoints = append(entryPoints, map[string]string{
				"path":    ep.Path,
				"type":    ep.Type,
				"purpose": ep.Purpose,
			})
		}
		arch["entry_points"] = entryPoints

		// Modules (compact)
		modules := make([]map[string]any, 0)
		for _, mod := range repoCtx.Architecture.Modules {
			modules = append(modules, map[string]any{
				"path":        mod.Path,
				"name":        mod.Name,
				"file_count":  len(mod.Files),
				"is_internal": mod.IsInternal,
			})
		}
		arch["modules"] = modules

		// AI analysis if available
		if repoCtx.Architecture.AIAnalysis != nil {
			arch["ai_analysis"] = map[string]any{
				"pattern":   repoCtx.Architecture.AIAnalysis.Pattern,
				"data_flow": repoCtx.Architecture.AIAnalysis.DataFlow,
				"strengths": repoCtx.Architecture.AIAnalysis.Strengths,
			}
		}
	}

	content, _ := json.MarshalIndent(arch, "", "  ")

	return &jsonRPCResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result: readResourceResult{
			Contents: []ResourceContent{{
				URI:      fmt.Sprintf("repo://%s/architecture", repoID),
				MimeType: "application/json",
				Text:     string(content),
			}},
		},
	}
}

func (s *server) readFunctionsResource(ctx context.Context, req *jsonRPCRequest, repoID string) *jsonRPCResponse {
	repoCtx, err := s.manager.GetContext(ctx, repoID)
	if err != nil {
		return &jsonRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &rpcError{Code: -32603, Message: fmt.Sprintf("Failed to get context: %v", err)},
		}
	}

	// Collect public functions (compact format)
	type FuncRef struct {
		Name      string `json:"name"`
		Signature string `json:"signature"`
		File      string `json:"file"`
		Line      int    `json:"line"`
		Summary   string `json:"summary,omitempty"`
	}

	functions := make([]FuncRef, 0)
	for path, fileCtx := range repoCtx.Files {
		for _, fn := range fileCtx.Functions {
			if fn.IsPublic {
				ref := FuncRef{
					Name:      fn.Name,
					Signature: fn.Signature,
					File:      path,
					Line:      fn.LineStart,
				}
				if fn.Behavior != nil && fn.Behavior.Summary != "" {
					ref.Summary = fn.Behavior.Summary
				}
				functions = append(functions, ref)
			}
		}
	}

	result := map[string]any{
		"repo_id":        repoID,
		"function_count": len(functions),
		"functions":      functions,
	}

	content, _ := json.MarshalIndent(result, "", "  ")

	return &jsonRPCResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result: readResourceResult{
			Contents: []ResourceContent{{
				URI:      fmt.Sprintf("repo://%s/functions", repoID),
				MimeType: "application/json",
				Text:     string(content),
			}},
		},
	}
}

func (s *server) readTypesResource(ctx context.Context, req *jsonRPCRequest, repoID string) *jsonRPCResponse {
	repoCtx, err := s.manager.GetContext(ctx, repoID)
	if err != nil {
		return &jsonRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &rpcError{Code: -32603, Message: fmt.Sprintf("Failed to get context: %v", err)},
		}
	}

	// Collect public types (compact format)
	type TypeRef struct {
		Name        string   `json:"name"`
		Kind        string   `json:"kind"`
		File        string   `json:"file"`
		Line        int      `json:"line"`
		Description string   `json:"description,omitempty"`
		Fields      []string `json:"fields,omitempty"`
	}

	types := make([]TypeRef, 0)
	for path, fileCtx := range repoCtx.Files {
		for _, t := range fileCtx.Types {
			if t.IsPublic {
				ref := TypeRef{
					Name:        t.Name,
					Kind:        t.Kind,
					File:        path,
					Line:        t.LineStart,
					Description: t.Description,
				}
				// Add field names for structs
				if t.Kind == "struct" && len(t.Fields) > 0 {
					for _, f := range t.Fields {
						if f.IsPublic {
							ref.Fields = append(ref.Fields, f.Name)
						}
					}
				}
				types = append(types, ref)
			}
		}
	}

	result := map[string]any{
		"repo_id":    repoID,
		"type_count": len(types),
		"types":      types,
	}

	content, _ := json.MarshalIndent(result, "", "  ")

	return &jsonRPCResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result: readResourceResult{
			Contents: []ResourceContent{{
				URI:      fmt.Sprintf("repo://%s/types", repoID),
				MimeType: "application/json",
				Text:     string(content),
			}},
		},
	}
}
