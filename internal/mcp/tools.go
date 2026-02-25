package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/yashpalc/mcp-repo-context/internal/comparison"
	"github.com/yashpalc/mcp-repo-context/internal/compose"
	graphpkg "github.com/yashpalc/mcp-repo-context/internal/graph"
	ctxpkg "github.com/yashpalc/mcp-repo-context/internal/context"
	"github.com/yashpalc/mcp-repo-context/internal/orchestrator"
	"github.com/yashpalc/mcp-repo-context/internal/prreview"
	"github.com/yashpalc/mcp-repo-context/internal/tokens"
	"github.com/yashpalc/mcp-repo-context/internal/vectors"
)

// toolAnalyzeRepo handles the analyze_repo tool call.
func (s *server) toolAnalyzeRepo(ctx context.Context, args map[string]any) callToolResult {
	repoURL, ok := args["repo_url"].(string)
	if !ok || repoURL == "" {
		return errorResult("repo_url is required")
	}

	branch, _ := args["branch"].(string)
	force, _ := args["force"].(bool)

	opts := orchestrator.AnalyzeOptions{
		Branch:      branch,
		Force:       force,
		GitHubToken: s.config.GitHubToken,
		MaxAge:      24 * time.Hour, // Consider cache fresh for 24 hours
	}

	result, err := s.manager.AnalyzeRepo(ctx, repoURL, opts)
	if err != nil {
		return errorResult(fmt.Sprintf("Analysis failed: %v", err))
	}

	// Format response
	var sb strings.Builder
	fmt.Fprintf(&sb, "Repository analyzed successfully\n\n")
	fmt.Fprintf(&sb, "**Repository ID:** `%s`\n", result.RepoID)
	fmt.Fprintf(&sb, "**Files Analyzed:** %d\n", result.FileCount)
	fmt.Fprintf(&sb, "**Duration:** %s\n", result.Duration.Round(time.Millisecond))

	if len(result.Warnings) > 0 {
		fmt.Fprintf(&sb, "\n**Warnings:**\n")
		for _, w := range result.Warnings {
			fmt.Fprintf(&sb, "- %s\n", w)
		}
	}

	fmt.Fprintf(&sb, "\nUse `get_context` with repo_id `%s` to retrieve the analysis.", result.RepoID)

	return callToolResult{
		Content: []contentItem{{Type: "text", Text: sb.String()}},
	}
}

// toolGetContext handles the get_context tool call.
func (s *server) toolGetContext(ctx context.Context, args map[string]any) callToolResult {
	repoID, ok := args["repo_id"].(string)
	if !ok || repoID == "" {
		return errorResult("repo_id is required")
	}

	scope, _ := args["scope"].(string)
	if scope == "" {
		scope = "full"
	}

	filePath, _ := args["file_path"].(string)

	switch scope {
	case "file":
		if filePath == "" {
			return errorResult("file_path is required when scope is 'file'")
		}
		return s.getFileContext(ctx, repoID, filePath)

	case "architecture":
		return s.getArchitectureContext(ctx, repoID)

	case "full":
		return s.getFullContext(ctx, repoID)

	default:
		return errorResult("Invalid scope: must be 'full', 'architecture', or 'file'")
	}
}

func (s *server) getFullContext(ctx context.Context, repoID string) callToolResult {
	repoCtx, err := s.manager.GetContext(ctx, repoID)
	if err != nil {
		return errorResult(fmt.Sprintf("Failed to get context: %v", err))
	}

	// Format as readable markdown
	var sb strings.Builder

	fmt.Fprintf(&sb, "# Repository: %s\n\n", repoCtx.ID)
	fmt.Fprintf(&sb, "- **URL:** %s\n", repoCtx.URL)
	fmt.Fprintf(&sb, "- **Branch:** %s\n", repoCtx.Branch)
	fmt.Fprintf(&sb, "- **Commit:** %s\n", repoCtx.CommitHash)
	fmt.Fprintf(&sb, "- **Analyzed:** %s\n\n", repoCtx.AnalyzedAt.Format(time.RFC3339))

	// Statistics
	sb.WriteString("## Statistics\n\n")
	fmt.Fprintf(&sb, "- Total Files: %d\n", repoCtx.Statistics.TotalFiles)
	fmt.Fprintf(&sb, "- Total Lines: %d\n", repoCtx.Statistics.TotalLines)
	fmt.Fprintf(&sb, "- Functions: %d\n", repoCtx.Statistics.FunctionCount)
	fmt.Fprintf(&sb, "- Types: %d\n", repoCtx.Statistics.TypeCount)
	fmt.Fprintf(&sb, "- Exports: %d\n\n", repoCtx.Statistics.ExportCount)

	// Languages
	sb.WriteString("### Languages\n\n")
	for lang, count := range repoCtx.Statistics.LanguageBreakdown {
		fmt.Fprintf(&sb, "- %s: %d files\n", lang, count)
	}
	sb.WriteString("\n")

	// Architecture
	if repoCtx.Architecture != nil {
		sb.WriteString("## Architecture\n\n")
		sb.WriteString(repoCtx.Architecture.Overview + "\n\n")

		if repoCtx.Architecture.BuildSystem != "" {
			fmt.Fprintf(&sb, "**Build System:** %s\n\n", repoCtx.Architecture.BuildSystem)
		}

		if len(repoCtx.Architecture.EntryPoints) > 0 {
			sb.WriteString("### Entry Points\n\n")
			for _, ep := range repoCtx.Architecture.EntryPoints {
				fmt.Fprintf(&sb, "- `%s` (%s)\n", ep.Path, ep.Type)
			}
			sb.WriteString("\n")
		}

		if len(repoCtx.Architecture.Modules) > 0 {
			sb.WriteString("### Modules\n\n")
			for _, mod := range repoCtx.Architecture.Modules {
				internal := ""
				if mod.IsInternal {
					internal = " (internal)"
				}
				fmt.Fprintf(&sb, "- **%s**%s: %d files\n", mod.Path, internal, len(mod.Files))
			}
			sb.WriteString("\n")
		}
	}

	// File summaries (condensed)
	sb.WriteString("## Files\n\n")
	for path, fileCtx := range repoCtx.Files {
		fmt.Fprintf(&sb, "### `%s`\n\n", path)
		fmt.Fprintf(&sb, "- Language: %s\n", fileCtx.Language)
		fmt.Fprintf(&sb, "- Lines: %d\n", fileCtx.LineCount)

		if fileCtx.Purpose != "" {
			fmt.Fprintf(&sb, "- Purpose: %s\n", fileCtx.Purpose)
		}

		if len(fileCtx.Exports) > 0 {
			fmt.Fprintf(&sb, "- Exports: %d\n", len(fileCtx.Exports))
			for _, exp := range fileCtx.Exports {
				fmt.Fprintf(&sb, "  - `%s` (%s)\n", exp.Name, exp.Kind)
			}
		}

		sb.WriteString("\n")
	}

	return callToolResult{
		Content: []contentItem{{Type: "text", Text: sb.String()}},
	}
}

func (s *server) getArchitectureContext(ctx context.Context, repoID string) callToolResult {
	repoCtx, err := s.manager.GetContext(ctx, repoID)
	if err != nil {
		return errorResult(fmt.Sprintf("Failed to get context: %v", err))
	}

	if repoCtx.Architecture == nil {
		return errorResult("No architecture context available")
	}

	arch := repoCtx.Architecture
	var sb strings.Builder

	sb.WriteString("# Architecture\n\n")

	// Overview
	if arch.Overview != "" {
		sb.WriteString("## Overview\n\n")
		sb.WriteString(arch.Overview)
		sb.WriteString("\n\n")
	}

	// Build System
	if arch.BuildSystem != "" {
		fmt.Fprintf(&sb, "**Build System:** %s\n\n", arch.BuildSystem)
	}

	// Module Info
	if repoCtx.ModuleInfo != nil {
		sb.WriteString("## Module Info\n\n")
		fmt.Fprintf(&sb, "- **Module Path:** %s\n", repoCtx.ModuleInfo.ModulePath)
		if repoCtx.ModuleInfo.GoVersion != "" {
			fmt.Fprintf(&sb, "- **Go Version:** %s\n", repoCtx.ModuleInfo.GoVersion)
		}
		sb.WriteString("\n")
	}

	// Package Type
	if arch.PackageType != "" {
		fmt.Fprintf(&sb, "**Package Type:** %s\n\n", arch.PackageType)
	}

	// Entry Points
	if len(arch.EntryPoints) > 0 {
		sb.WriteString("## Entry Points\n\n")
		for _, ep := range arch.EntryPoints {
			fmt.Fprintf(&sb, "- `%s` (%s)", ep.Path, ep.Type)
			if ep.Purpose != "" {
				fmt.Fprintf(&sb, " - %s", ep.Purpose)
			}
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	}

	// Modules
	if len(arch.Modules) > 0 {
		sb.WriteString("## Modules\n\n")
		for _, mod := range arch.Modules {
			fmt.Fprintf(&sb, "- **%s** (%d files)", mod.Name, len(mod.Files))
			if mod.Purpose != "" {
				fmt.Fprintf(&sb, " - %s", mod.Purpose)
			}
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	}

	// Dependencies (from ModuleInfo)
	if repoCtx.ModuleInfo != nil && len(repoCtx.ModuleInfo.Dependencies) > 0 {
		var directDeps []ctxpkg.ModuleDependency
		for _, dep := range repoCtx.ModuleInfo.Dependencies {
			if dep.IsDirect {
				directDeps = append(directDeps, dep)
			}
		}
		if len(directDeps) > 0 {
			fmt.Fprintf(&sb, "## Direct Dependencies (%d)\n\n", len(directDeps))
			for _, dep := range directDeps {
				fmt.Fprintf(&sb, "- `%s` %s\n", dep.Path, dep.Version)
			}
			sb.WriteString("\n")
		}
	}

	// Import Summary
	if repoCtx.ImportSummary != nil {
		sb.WriteString("## Import Summary\n\n")
		fmt.Fprintf(&sb, "- **Stdlib:** %d packages\n", len(repoCtx.ImportSummary.Stdlib))
		fmt.Fprintf(&sb, "- **Internal:** %d packages\n", len(repoCtx.ImportSummary.Internal))
		fmt.Fprintf(&sb, "- **External:** %d packages\n", len(repoCtx.ImportSummary.External))
		sb.WriteString("\n")
	}

	return callToolResult{
		Content: []contentItem{{Type: "text", Text: sb.String()}},
	}
}

func (s *server) getFileContext(ctx context.Context, repoID, filePath string) callToolResult {
	fileCtx, err := s.manager.GetFileContext(ctx, repoID, filePath)
	if err != nil {
		return errorResult(fmt.Sprintf("Failed to get file context: %v", err))
	}

	var sb strings.Builder

	fmt.Fprintf(&sb, "# File: %s\n\n", fileCtx.Path)
	fmt.Fprintf(&sb, "- **Language:** %s\n", fileCtx.Language)
	fmt.Fprintf(&sb, "- **Lines:** %d\n", fileCtx.LineCount)
	fmt.Fprintf(&sb, "- **Size:** %d bytes\n", fileCtx.Size)

	if fileCtx.Purpose != "" {
		fmt.Fprintf(&sb, "- **Purpose:** %s\n", fileCtx.Purpose)
	}
	sb.WriteString("\n")

	// Imports
	if len(fileCtx.Imports) > 0 {
		sb.WriteString("## Imports\n\n")
		for _, imp := range fileCtx.Imports {
			if imp.Alias != "" && imp.Alias != "_" {
				fmt.Fprintf(&sb, "- `%s` (as %s)\n", imp.Path, imp.Alias)
			} else {
				fmt.Fprintf(&sb, "- `%s`\n", imp.Path)
			}
		}
		sb.WriteString("\n")
	}

	// Types
	if len(fileCtx.Types) > 0 {
		sb.WriteString("## Types\n\n")
		for _, t := range fileCtx.Types {
			visibility := "private"
			if t.IsPublic {
				visibility = "public"
			}
			fmt.Fprintf(&sb, "### `%s` (%s %s)\n\n", t.Name, visibility, t.Kind)

			if t.Description != "" {
				sb.WriteString(t.Description + "\n\n")
			}

			if len(t.Fields) > 0 {
				sb.WriteString("**Fields:**\n")
				for _, f := range t.Fields {
					fmt.Fprintf(&sb, "- `%s %s`\n", f.Name, f.Type)
				}
				sb.WriteString("\n")
			}

			if len(t.Methods) > 0 {
				sb.WriteString("**Methods:** " + strings.Join(t.Methods, ", ") + "\n\n")
			}
		}
	}

	// Functions
	if len(fileCtx.Functions) > 0 {
		sb.WriteString("## Functions\n\n")
		for _, fn := range fileCtx.Functions {
			visibility := "private"
			if fn.IsPublic {
				visibility = "public"
			}
			fmt.Fprintf(&sb, "### `%s` (%s)\n\n", fn.Name, visibility)
			fmt.Fprintf(&sb, "```go\n%s\n```\n\n", fn.Signature)

			if fn.Description != "" {
				sb.WriteString(fn.Description + "\n\n")
			}

			fmt.Fprintf(&sb, "- Lines: %d-%d\n", fn.LineStart, fn.LineEnd)
			if fn.Complexity > 0 {
				fmt.Fprintf(&sb, "- Complexity: %d\n", fn.Complexity)
			}
			sb.WriteString("\n")
		}
	}

	// Constants
	if len(fileCtx.Constants) > 0 {
		sb.WriteString("## Constants\n\n")
		for _, c := range fileCtx.Constants {
			fmt.Fprintf(&sb, "- `%s`", c.Name)
			if c.Value != "" {
				fmt.Fprintf(&sb, " = %s", c.Value)
			}
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	}

	return callToolResult{
		Content: []contentItem{{Type: "text", Text: sb.String()}},
	}
}

// toolRegisterOrg handles the register_org tool call.
func (s *server) toolRegisterOrg(ctx context.Context, args map[string]any) callToolResult {
	if s.config.OrgManager == nil {
		return errorResult("org support not configured (OrgManager is nil)")
	}
	orgID, ok := args["org_id"].(string)
	if !ok || orgID == "" {
		return errorResult("org_id is required")
	}
	var repoIDs []string
	if rids, ok := args["repo_ids"].([]interface{}); ok {
		for _, r := range rids {
			if s, ok := r.(string); ok && s != "" {
				repoIDs = append(repoIDs, s)
			}
		}
	}
	o, err := s.config.OrgManager.Register(ctx, orgID, repoIDs, nil)
	if err != nil {
		return errorResult(fmt.Sprintf("Failed to register org: %v", err))
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "Registered org **%s** with %d repo(s)\n\n", o.ID, len(o.Repos))
	for _, r := range o.Repos {
		fmt.Fprintf(&sb, "- %s\n", r)
	}
	return callToolResult{
		Content: []contentItem{{Type: "text", Text: sb.String()}},
	}
}

// toolListOrgs handles the list_orgs tool call.
func (s *server) toolListOrgs(ctx context.Context, args map[string]any) callToolResult {
	if s.config.OrgManager == nil {
		return errorResult("org support not configured (OrgManager is nil)")
	}
	orgs, err := s.config.OrgManager.List(ctx)
	if err != nil {
		return errorResult(fmt.Sprintf("Failed to list orgs: %v", err))
	}
	if len(orgs) == 0 {
		return callToolResult{
			Content: []contentItem{{Type: "text", Text: "No organizations registered. Use `register_org` to create one."}},
		}
	}
	var sb strings.Builder
	sb.WriteString("# Organizations\n\n")
	for _, o := range orgs {
		fmt.Fprintf(&sb, "## %s\n", o.ID)
		fmt.Fprintf(&sb, "- **Repos:** %d\n\n", o.RepoCount)
	}
	return callToolResult{
		Content: []contentItem{{Type: "text", Text: sb.String()}},
	}
}

// toolAnalyzeOrg handles the analyze_org tool call using concurrent analysis.
func (s *server) toolAnalyzeOrg(ctx context.Context, args map[string]any) callToolResult {
	if s.config.OrgManager == nil {
		return errorResult("org support not configured (OrgManager is nil)")
	}
	orgID, ok := args["org_id"].(string)
	if !ok || orgID == "" {
		return errorResult("org_id is required")
	}
	force, _ := args["force"].(bool)

	concurrency := 3
	if c, ok := args["concurrency"].(float64); ok {
		concurrency = int(c)
		if concurrency < 1 {
			concurrency = 1
		}
		if concurrency > 10 {
			concurrency = 10
		}
	}

	result, err := s.config.OrgManager.AnalyzeOrg(ctx, orgID, force, concurrency)
	if err != nil {
		return errorResult(fmt.Sprintf("Failed to analyze org: %v", err))
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "# Org Analysis: %s\n\n", result.OrgID)
	fmt.Fprintf(&sb, "- **Total:** %d repos\n", result.Total)
	fmt.Fprintf(&sb, "- **Succeeded:** %d\n", result.Succeeded)
	fmt.Fprintf(&sb, "- **Failed:** %d\n", result.Failed)
	fmt.Fprintf(&sb, "- **Skipped:** %d\n", result.Skipped)
	fmt.Fprintf(&sb, "- **Duration:** %s\n", result.Duration.Round(time.Millisecond))

	if len(result.Errors) > 0 {
		sb.WriteString("\n## Errors\n\n")
		for _, e := range result.Errors {
			fmt.Fprintf(&sb, "- **%s**: %s\n", e.RepoID, e.Error)
		}
	}

	return callToolResult{
		Content: []contentItem{{Type: "text", Text: sb.String()}},
	}
}

// toolGetOrg handles the get_org tool call.
func (s *server) toolGetOrg(ctx context.Context, args map[string]any) callToolResult {
	if s.config.OrgManager == nil {
		return errorResult("org support not configured (OrgManager is nil)")
	}
	orgID, ok := args["org_id"].(string)
	if !ok || orgID == "" {
		return errorResult("org_id is required")
	}

	o, err := s.config.OrgManager.Get(ctx, orgID)
	if err != nil {
		return callToolResult{
			Content: []contentItem{{Type: "text", Text: fmt.Sprintf("Org not found: %v", err)}},
			IsError: true,
		}
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "# Org: %s\n\n", o.ID)
	fmt.Fprintf(&sb, "- **Repos:** %d\n", len(o.Repos))
	if !o.Created.IsZero() {
		fmt.Fprintf(&sb, "- **Created:** %s\n", o.Created.Format(time.RFC3339))
	}

	if len(o.Repos) > 0 {
		sb.WriteString("\n## Repositories\n\n")
		for _, r := range o.Repos {
			fmt.Fprintf(&sb, "- %s\n", r)
		}
	}

	if len(o.Config.ExcludePatterns) > 0 || o.Config.MaxFileSize > 0 {
		sb.WriteString("\n## Config\n\n")
		if len(o.Config.ExcludePatterns) > 0 {
			fmt.Fprintf(&sb, "- **Exclude patterns:** %s\n", strings.Join(o.Config.ExcludePatterns, ", "))
		}
		if o.Config.MaxFileSize > 0 {
			fmt.Fprintf(&sb, "- **Max file size:** %d bytes\n", o.Config.MaxFileSize)
		}
	}

	return callToolResult{
		Content: []contentItem{{Type: "text", Text: sb.String()}},
	}
}

// toolDeleteOrg handles the delete_org tool call.
func (s *server) toolDeleteOrg(ctx context.Context, args map[string]any) callToolResult {
	if s.config.OrgManager == nil {
		return errorResult("org support not configured (OrgManager is nil)")
	}
	orgID, ok := args["org_id"].(string)
	if !ok || orgID == "" {
		return errorResult("org_id is required")
	}

	mode := "detach"
	if m, ok := args["mode"].(string); ok && m != "" {
		mode = m
	}

	if mode == "cascade" {
		o, err := s.config.OrgManager.Get(ctx, orgID)
		if err != nil {
			return errorResult(fmt.Sprintf("Org not found: %v", err))
		}
		for _, repoID := range o.Repos {
			if err := s.manager.DeleteRepoContext(ctx, repoID); err != nil {
				s.logger.WithFields(map[string]interface{}{
					"org_id":  orgID,
					"repo_id": repoID,
				}).Warnf("failed to delete repo context during cascade: %v", err)
			}
		}
	}

	if err := s.config.OrgManager.Delete(ctx, orgID); err != nil {
		return errorResult(fmt.Sprintf("Failed to delete org: %v", err))
	}

	return callToolResult{
		Content: []contentItem{{Type: "text", Text: fmt.Sprintf("Deleted org **%s** (mode: %s)", orgID, mode)}},
	}
}

// toolUpdateOrgConfig handles the update_org_config tool call.
func (s *server) toolUpdateOrgConfig(ctx context.Context, args map[string]any) callToolResult {
	if s.config.OrgManager == nil {
		return errorResult("org support not configured (OrgManager is nil)")
	}
	orgID, ok := args["org_id"].(string)
	if !ok || orgID == "" {
		return errorResult("org_id is required")
	}

	o, err := s.config.OrgManager.Get(ctx, orgID)
	if err != nil {
		return errorResult(fmt.Sprintf("Org not found: %v", err))
	}

	if configMap, ok := args["config"].(map[string]interface{}); ok {
		if patterns, ok := configMap["exclude_patterns"].([]interface{}); ok {
			o.Config.ExcludePatterns = nil
			for _, p := range patterns {
				if ps, ok := p.(string); ok {
					o.Config.ExcludePatterns = append(o.Config.ExcludePatterns, ps)
				}
			}
		}
		if maxSize, ok := configMap["max_file_size"].(float64); ok {
			o.Config.MaxFileSize = int64(maxSize)
		}
	}

	// Re-register with updated config
	updated, err := s.config.OrgManager.Register(ctx, o.ID, o.Repos, &o.Config)
	if err != nil {
		return errorResult(fmt.Sprintf("Failed to update org config: %v", err))
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Updated config for org **%s**\n\n", updated.ID)
	if len(updated.Config.ExcludePatterns) > 0 {
		fmt.Fprintf(&sb, "- **Exclude patterns:** %s\n", strings.Join(updated.Config.ExcludePatterns, ", "))
	}
	if updated.Config.MaxFileSize > 0 {
		fmt.Fprintf(&sb, "- **Max file size:** %d bytes\n", updated.Config.MaxFileSize)
	}

	return callToolResult{
		Content: []contentItem{{Type: "text", Text: sb.String()}},
	}
}

// toolAddReposToOrg handles the add_repos_to_org tool call.
func (s *server) toolAddReposToOrg(ctx context.Context, args map[string]any) callToolResult {
	if s.config.OrgManager == nil {
		return errorResult("org support not configured (OrgManager is nil)")
	}
	orgID, ok := args["org_id"].(string)
	if !ok || orgID == "" {
		return errorResult("org_id is required")
	}

	var repoIDs []string
	if rids, ok := args["repo_ids"].([]interface{}); ok {
		for _, r := range rids {
			if s, ok := r.(string); ok && s != "" {
				repoIDs = append(repoIDs, s)
			}
		}
	}
	if len(repoIDs) == 0 {
		return errorResult("repo_ids is required and must not be empty")
	}

	if err := s.config.OrgManager.AddRepos(ctx, orgID, repoIDs); err != nil {
		return errorResult(fmt.Sprintf("Failed to add repos: %v", err))
	}

	o, err := s.config.OrgManager.Get(ctx, orgID)
	if err != nil {
		return errorResult(fmt.Sprintf("Repos added but failed to fetch updated org: %v", err))
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Added %d repo(s) to org **%s** (total: %d)\n\n", len(repoIDs), orgID, len(o.Repos))
	for _, r := range o.Repos {
		fmt.Fprintf(&sb, "- %s\n", r)
	}

	return callToolResult{
		Content: []contentItem{{Type: "text", Text: sb.String()}},
	}
}

// toolRemoveReposFromOrg handles the remove_repos_from_org tool call.
func (s *server) toolRemoveReposFromOrg(ctx context.Context, args map[string]any) callToolResult {
	if s.config.OrgManager == nil {
		return errorResult("org support not configured (OrgManager is nil)")
	}
	orgID, ok := args["org_id"].(string)
	if !ok || orgID == "" {
		return errorResult("org_id is required")
	}

	var repoIDs []string
	if rids, ok := args["repo_ids"].([]interface{}); ok {
		for _, r := range rids {
			if s, ok := r.(string); ok && s != "" {
				repoIDs = append(repoIDs, s)
			}
		}
	}
	if len(repoIDs) == 0 {
		return errorResult("repo_ids is required and must not be empty")
	}

	if err := s.config.OrgManager.RemoveRepos(ctx, orgID, repoIDs); err != nil {
		return errorResult(fmt.Sprintf("Failed to remove repos: %v", err))
	}

	o, err := s.config.OrgManager.Get(ctx, orgID)
	if err != nil {
		return errorResult(fmt.Sprintf("Repos removed but failed to fetch updated org: %v", err))
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Removed %d repo(s) from org **%s** (remaining: %d)\n\n", len(repoIDs), orgID, len(o.Repos))
	for _, r := range o.Repos {
		fmt.Fprintf(&sb, "- %s\n", r)
	}

	return callToolResult{
		Content: []contentItem{{Type: "text", Text: sb.String()}},
	}
}

// toolListRepos handles the list_repos tool call.
func (s *server) toolListRepos(ctx context.Context, args map[string]any) callToolResult {
	repos, err := s.manager.ListRepos(ctx)
	if err != nil {
		return errorResult(fmt.Sprintf("Failed to list repos: %v", err))
	}

	if len(repos) == 0 {
		return callToolResult{
			Content: []contentItem{{Type: "text", Text: "No repositories have been analyzed yet. Use `analyze_repo` to analyze a repository."}},
		}
	}

	var sb strings.Builder
	sb.WriteString("# Analyzed Repositories\n\n")

	for _, repo := range repos {
		fmt.Fprintf(&sb, "## %s\n\n", repo.RepoID)
		fmt.Fprintf(&sb, "- **URL:** %s\n", repo.URL)
		fmt.Fprintf(&sb, "- **Branch:** %s\n", repo.Branch)
		fmt.Fprintf(&sb, "- **Commit:** %s\n", repo.CommitHash)
		fmt.Fprintf(&sb, "- **Files:** %d\n", repo.FileCount)
		fmt.Fprintf(&sb, "- **Analyzed:** %s\n\n", repo.AnalyzedAt.Format(time.RFC3339))
	}

	return callToolResult{
		Content: []contentItem{{Type: "text", Text: sb.String()}},
	}
}

// toolSearchContext handles the search_context tool call.
func (s *server) toolSearchContext(ctx context.Context, args map[string]any) callToolResult {
	query, ok := args["query"].(string)
	if !ok || query == "" {
		return errorResult("query is required")
	}

	repoID, _ := args["repo_id"].(string)
	searchType, _ := args["search_type"].(string)
	if searchType == "" {
		searchType = "all"
	}

	// Parse max_results (default 20, max 100)
	maxResults := 20
	if mr, ok := args["max_results"].(float64); ok && mr > 0 {
		maxResults = int(mr)
		if maxResults > 100 {
			maxResults = 100
		}
	}

	// Parse compact mode (default true)
	compact := true
	if c, ok := args["compact"].(bool); ok {
		compact = c
	}

	// Split query into keywords for multi-word matching
	keywords := extractSearchKeywords(query)

	s.log("search_context", map[string]interface{}{
		"query":       query,
		"keywords":    keywords,
		"repo_id":     repoID,
		"search_type": searchType,
		"max_results": maxResults,
		"compact":     compact,
	})

	var results []searchResult

	// Get repos to search
	var repoIDs []string
	if repoID != "" {
		repoIDs = []string{repoID}
	} else {
		repos, err := s.manager.ListRepos(ctx)
		if err != nil {
			return errorResult(fmt.Sprintf("Failed to list repos: %v", err))
		}
		for _, r := range repos {
			repoIDs = append(repoIDs, r.RepoID)
		}
	}

	// Search each repo
	for _, rid := range repoIDs {
		repoCtx, err := s.manager.GetContext(ctx, rid)
		if err != nil {
			continue
		}

		for path, fileCtx := range repoCtx.Files {
			// Search functions
			if searchType == "all" || searchType == "function" {
				for _, fn := range fileCtx.Functions {
					if matchesAllKeywords(fn.Name, keywords) ||
						matchesAllKeywords(fn.Description, keywords) ||
						matchesAllKeywords(fn.Signature, keywords) {
						results = append(results, searchResult{
							RepoID:      rid,
							FilePath:    path,
							Name:        fn.Name,
							Type:        "function",
							Signature:   fn.Signature,
							Description: fn.Description,
							Line:        fn.LineStart,
						})
					}
				}
			}

			// Search types
			if searchType == "all" || searchType == "type" {
				for _, t := range fileCtx.Types {
					if matchesAllKeywords(t.Name, keywords) ||
						matchesAllKeywords(t.Description, keywords) {
						results = append(results, searchResult{
							RepoID:      rid,
							FilePath:    path,
							Name:        t.Name,
							Type:        "type",
							Signature:   t.Kind,
							Description: t.Description,
							Line:        t.LineStart,
						})
					}
				}
			}

			// Search files
			if searchType == "all" || searchType == "file" {
				if matchesAllKeywords(path, keywords) ||
					matchesAllKeywords(fileCtx.Purpose, keywords) {
					results = append(results, searchResult{
						RepoID:      rid,
						FilePath:    path,
						Name:        path,
						Type:        "file",
						Description: fileCtx.Purpose,
					})
				}
			}
		}
	}

	s.log("search_context_results", map[string]interface{}{
		"query":   query,
		"results": len(results),
	})

	if len(results) == 0 {
		return callToolResult{
			Content: []contentItem{{Type: "text", Text: fmt.Sprintf("No results found for query: %s\n\nTip: Try individual keywords like 'test' or 'create' instead of phrases.", query)}},
		}
	}

	// Limit results based on max_results parameter
	totalCount := len(results)
	truncated := false
	if len(results) > maxResults {
		results = results[:maxResults]
		truncated = true
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "# Search Results for \"%s\"\n\n", query)
	fmt.Fprintf(&sb, "Found %d results", len(results))
	if truncated {
		fmt.Fprintf(&sb, " (showing %d of %d - use `max_results` to see more)", len(results), totalCount)
	}
	sb.WriteString("\n\n")

	for _, r := range results {
		fmt.Fprintf(&sb, "## %s `%s`\n\n", r.Type, r.Name)
		fmt.Fprintf(&sb, "- **Repository:** %s\n", r.RepoID)
		fmt.Fprintf(&sb, "- **File:** %s\n", r.FilePath)
		if r.Line > 0 {
			fmt.Fprintf(&sb, "- **Line:** %d\n", r.Line)
		}
		if r.Signature != "" {
			fmt.Fprintf(&sb, "- **Signature:** `%s`\n", r.Signature)
		}
		if r.Description != "" {
			fmt.Fprintf(&sb, "- **Description:** %s\n", r.Description)
		}
		sb.WriteString("\n")
	}

	return callToolResult{
		Content: []contentItem{{Type: "text", Text: sb.String()}},
	}
}

// extractSearchKeywords splits a query into searchable keywords.
func extractSearchKeywords(query string) []string {
	query = strings.ToLower(query)
	words := strings.Fields(query)

	// Filter out very short words and common stop words
	stopWords := map[string]bool{
		"the": true, "a": true, "an": true, "and": true, "or": true,
		"in": true, "on": true, "at": true, "to": true, "for": true,
		"of": true, "with": true, "by": true, "from": true, "as": true,
		"is": true, "are": true, "was": true, "were": true,
	}

	keywords := make([]string, 0, len(words))
	for _, word := range words {
		word = strings.Trim(word, ".,;:!?\"'()[]{}")
		if len(word) >= 2 && !stopWords[word] {
			keywords = append(keywords, word)
		}
	}

	// If no keywords after filtering, use original words
	if len(keywords) == 0 && len(words) > 0 {
		for _, word := range words {
			word = strings.Trim(word, ".,;:!?\"'()[]{}")
			if len(word) >= 1 {
				keywords = append(keywords, word)
			}
		}
	}

	return keywords
}

// matchesAllKeywords returns true if text contains ALL keywords.
func matchesAllKeywords(text string, keywords []string) bool {
	if len(keywords) == 0 {
		return false
	}

	textLower := strings.ToLower(text)

	for _, kw := range keywords {
		if !strings.Contains(textLower, kw) {
			return false
		}
	}
	return true
}

type searchResult struct {
	RepoID      string
	FilePath    string
	Name        string
	Type        string
	Signature   string
	Description string
	Line        int
}

func errorResult(msg string) callToolResult {
	return callToolResult{
		Content: []contentItem{{Type: "text", Text: "Error: " + msg}},
		IsError: true,
	}
}

// toolCompareRepos handles the compare_repos tool call.
func (s *server) toolCompareRepos(ctx context.Context, args map[string]any) callToolResult {
	repoIDs, ok := args["repo_ids"].([]interface{})
	if !ok || len(repoIDs) < 2 {
		return errorResult("repo_ids must be an array with at least 2 repository IDs")
	}

	// Convert to string slice
	var ids []string
	for _, id := range repoIDs {
		if s, ok := id.(string); ok {
			ids = append(ids, s)
		}
	}

	// Get all repo contexts
	var repoContexts []*ctxpkg.RepoContext
	for _, id := range ids {
		rc, err := s.manager.GetContext(ctx, id)
		if err != nil {
			return errorResult(fmt.Sprintf("Failed to get context for %s: %v", id, err))
		}
		repoContexts = append(repoContexts, rc)
	}

	// Build compare options
	opts := comparison.DefaultCompareOptions()
	if targetID, ok := args["target_repo_id"].(string); ok {
		opts.TargetRepoID = targetID
	}
	if v, ok := args["include_duplicates"].(bool); ok {
		opts.IncludeDuplicates = v
	}
	if v, ok := args["include_conflicts"].(bool); ok {
		opts.IncludeConflicts = v
	}
	if v, ok := args["include_gaps"].(bool); ok {
		opts.IncludeGaps = v
	}

	// Run comparison
	result, err := s.comparer.Compare(ctx, repoContexts, opts)
	if err != nil {
		return errorResult(fmt.Sprintf("Comparison failed: %v", err))
	}

	// Format output
	var sb strings.Builder
	sb.WriteString("# Repository Comparison Report\n\n")

	// Repo summaries
	sb.WriteString("## Repositories\n\n")
	for _, repo := range result.Repos {
		target := ""
		if repo.IsTarget {
			target = " (TARGET)"
		}
		fmt.Fprintf(&sb, "### %s%s\n", repo.ID, target)
		fmt.Fprintf(&sb, "- Files: %d\n", repo.FileCount)
		fmt.Fprintf(&sb, "- Functions: %d\n", repo.FunctionCount)
		fmt.Fprintf(&sb, "- Types: %d\n\n", repo.TypeCount)
	}

	// Unified stats
	sb.WriteString("## Unified Statistics\n\n")
	fmt.Fprintf(&sb, "- Total Repos: %d\n", result.UnifiedStatistics.TotalRepos)
	fmt.Fprintf(&sb, "- Total Files: %d\n", result.UnifiedStatistics.TotalFiles)
	fmt.Fprintf(&sb, "- Unique Files: %d\n", result.UnifiedStatistics.UniqueFiles)
	fmt.Fprintf(&sb, "- Overlapping Files: %d\n", result.UnifiedStatistics.OverlappingFiles)
	fmt.Fprintf(&sb, "- Total Functions: %d\n", result.UnifiedStatistics.TotalFunctions)
	fmt.Fprintf(&sb, "- Total Types: %d\n\n", result.UnifiedStatistics.TotalTypes)

	// Duplicates
	if len(result.Duplicates) > 0 {
		sb.WriteString("## Duplicates\n\n")
		fmt.Fprintf(&sb, "Found %d duplicate groups:\n\n", len(result.Duplicates))
		for i, dup := range result.Duplicates {
			if i >= 10 {
				fmt.Fprintf(&sb, "... and %d more\n\n", len(result.Duplicates)-10)
				break
			}
			fmt.Fprintf(&sb, "### %s `%s`\n", dup.Type, dup.Name)
			fmt.Fprintf(&sb, "Found in %d locations:\n", len(dup.Instances))
			for _, inst := range dup.Instances {
				fmt.Fprintf(&sb, "- %s: %s:%d\n", inst.RepoID, inst.FilePath, inst.Line)
			}
			fmt.Fprintf(&sb, "**Recommendation:** %s\n\n", dup.Recommendation)
		}
	}

	// Conflicts
	if len(result.Conflicts) > 0 {
		sb.WriteString("## Conflicts\n\n")
		fmt.Fprintf(&sb, "Found %d conflicts:\n\n", len(result.Conflicts))
		for _, conflict := range result.Conflicts {
			fmt.Fprintf(&sb, "### %s: `%s` (%s severity)\n", conflict.Type, conflict.Name, conflict.Severity)
			fmt.Fprintf(&sb, "%s\n\n", conflict.Description)
			fmt.Fprintf(&sb, "**Resolution:** %s\n\n", conflict.Resolution)
		}
	}

	// Gaps
	if len(result.Gaps) > 0 {
		sb.WriteString("## Gaps (Missing in Target)\n\n")
		fmt.Fprintf(&sb, "Found %d gaps:\n\n", len(result.Gaps))
		for i, gap := range result.Gaps {
			if i >= 20 {
				fmt.Fprintf(&sb, "... and %d more\n\n", len(result.Gaps)-20)
				break
			}
			fmt.Fprintf(&sb, "- **%s** `%s` (%s priority) - from: %s\n",
				gap.Type, gap.Name, gap.Priority, strings.Join(gap.SourceRepos, ", "))
		}
		sb.WriteString("\n")
	}

	// Consistency
	if result.Consistency != nil {
		sb.WriteString("## Consistency Analysis\n\n")
		fmt.Fprintf(&sb, "**Overall Score:** %.0f%%\n\n", result.Consistency.OverallScore*100)
		fmt.Fprintf(&sb, "- Naming: %.0f%%\n", result.Consistency.NamingConsistency.Score*100)
		fmt.Fprintf(&sb, "- Patterns: %.0f%%\n", result.Consistency.PatternConsistency.Score*100)
		fmt.Fprintf(&sb, "- Structure: %.0f%%\n\n", result.Consistency.StructureConsistency.Score*100)

		if len(result.Consistency.Issues) > 0 {
			sb.WriteString("### Issues\n\n")
			for _, issue := range result.Consistency.Issues {
				fmt.Fprintf(&sb, "- **%s** (%s): %s\n", issue.Type, issue.Severity, issue.Description)
			}
			sb.WriteString("\n")
		}
	}

	// Dependency Relationships
	if result.DependencyRelationships != nil {
		sb.WriteString("## Dependency Relationships\n\n")
		if len(result.DependencyRelationships.InternalDeps) > 0 {
			sb.WriteString("### Inter-Repo Dependencies\n\n")
			for _, dep := range result.DependencyRelationships.InternalDeps {
				fmt.Fprintf(&sb, "- **%s** depends on **%s** (%s)\n", dep.FromModule, dep.ToModule, dep.Version)
			}
			sb.WriteString("\n")
		}
		if len(result.DependencyRelationships.SharedExternalDeps) > 0 {
			sb.WriteString("### Shared External Dependencies\n\n")
			for _, shared := range result.DependencyRelationships.SharedExternalDeps {
				parts := make([]string, 0, len(shared.RepoIDs))
				for _, id := range shared.RepoIDs {
					v := shared.Versions[id]
					parts = append(parts, fmt.Sprintf("%s (%s)", id, v))
				}
				fmt.Fprintf(&sb, "- `%s` used by: %s\n", shared.ModulePath, strings.Join(parts, ", "))
			}
			sb.WriteString("\n")
		}
	}

	// Recommendations
	if len(result.Recommendations) > 0 {
		sb.WriteString("## Recommendations\n\n")
		for _, rec := range result.Recommendations {
			fmt.Fprintf(&sb, "- %s\n", rec)
		}
	}

	return callToolResult{
		Content: []contentItem{{Type: "text", Text: sb.String()}},
	}
}

// toolFindDuplicates handles the find_duplicates tool call.
func (s *server) toolFindDuplicates(ctx context.Context, args map[string]any) callToolResult {
	repoIDs, ok := args["repo_ids"].([]interface{})
	if !ok || len(repoIDs) < 2 {
		return errorResult("repo_ids must be an array with at least 2 repository IDs")
	}

	// Get repo contexts
	var repoContexts []*ctxpkg.RepoContext
	for _, id := range repoIDs {
		if idStr, ok := id.(string); ok {
			rc, err := s.manager.GetContext(ctx, idStr)
			if err != nil {
				return errorResult(fmt.Sprintf("Failed to get context for %s: %v", idStr, err))
			}
			repoContexts = append(repoContexts, rc)
		}
	}

	duplicates, err := s.comparer.FindDuplicates(ctx, repoContexts)
	if err != nil {
		return errorResult(fmt.Sprintf("Find duplicates failed: %v", err))
	}

	if len(duplicates) == 0 {
		return callToolResult{
			Content: []contentItem{{Type: "text", Text: "No duplicates found across the specified repositories."}},
		}
	}

	var sb strings.Builder
	sb.WriteString("# Duplicate Analysis\n\n")
	fmt.Fprintf(&sb, "Found %d duplicate groups:\n\n", len(duplicates))

	for _, dup := range duplicates {
		fmt.Fprintf(&sb, "## %s `%s`\n\n", dup.Type, dup.Name)
		fmt.Fprintf(&sb, "**Similarity:** %.0f%%\n", dup.Similarity*100)
		fmt.Fprintf(&sb, "**Instances:**\n")
		for _, inst := range dup.Instances {
			if inst.Signature != "" {
				fmt.Fprintf(&sb, "- %s `%s:%d` - `%s`\n", inst.RepoID, inst.FilePath, inst.Line, inst.Signature)
			} else {
				fmt.Fprintf(&sb, "- %s `%s:%d`\n", inst.RepoID, inst.FilePath, inst.Line)
			}
		}
		fmt.Fprintf(&sb, "\n**Recommendation:** %s\n\n", dup.Recommendation)
	}

	return callToolResult{
		Content: []contentItem{{Type: "text", Text: sb.String()}},
	}
}

// toolFindConflicts handles the find_conflicts tool call.
func (s *server) toolFindConflicts(ctx context.Context, args map[string]any) callToolResult {
	sourceIDs, ok := args["source_repo_ids"].([]interface{})
	if !ok || len(sourceIDs) == 0 {
		return errorResult("source_repo_ids must be a non-empty array")
	}

	targetID, ok := args["target_repo_id"].(string)
	if !ok || targetID == "" {
		return errorResult("target_repo_id is required")
	}

	// Get source contexts
	var sourceContexts []*ctxpkg.RepoContext
	for _, id := range sourceIDs {
		if idStr, ok := id.(string); ok {
			rc, err := s.manager.GetContext(ctx, idStr)
			if err != nil {
				return errorResult(fmt.Sprintf("Failed to get context for %s: %v", idStr, err))
			}
			sourceContexts = append(sourceContexts, rc)
		}
	}

	// Get target context
	targetContext, err := s.manager.GetContext(ctx, targetID)
	if err != nil {
		return errorResult(fmt.Sprintf("Failed to get target context: %v", err))
	}

	conflicts, err := s.comparer.FindConflicts(ctx, sourceContexts, targetContext)
	if err != nil {
		return errorResult(fmt.Sprintf("Find conflicts failed: %v", err))
	}

	if len(conflicts) == 0 {
		return callToolResult{
			Content: []contentItem{{Type: "text", Text: "No conflicts found between source and target repositories."}},
		}
	}

	var sb strings.Builder
	sb.WriteString("# Conflict Analysis\n\n")
	fmt.Fprintf(&sb, "Found %d conflicts:\n\n", len(conflicts))

	for _, conflict := range conflicts {
		fmt.Fprintf(&sb, "## %s: `%s`\n\n", conflict.Type, conflict.Name)
		fmt.Fprintf(&sb, "**Severity:** %s\n", conflict.Severity)
		fmt.Fprintf(&sb, "**Description:** %s\n\n", conflict.Description)

		sb.WriteString("**Source instances:**\n")
		for _, inst := range conflict.SourceInstances {
			fmt.Fprintf(&sb, "- %s `%s:%d`", inst.RepoID, inst.FilePath, inst.Line)
			if inst.Signature != "" {
				fmt.Fprintf(&sb, " - `%s`", inst.Signature)
			}
			sb.WriteString("\n")
		}

		if conflict.TargetInstance != nil {
			sb.WriteString("\n**Target instance:**\n")
			fmt.Fprintf(&sb, "- %s `%s:%d`", conflict.TargetInstance.RepoID, conflict.TargetInstance.FilePath, conflict.TargetInstance.Line)
			if conflict.TargetInstance.Signature != "" {
				fmt.Fprintf(&sb, " - `%s`", conflict.TargetInstance.Signature)
			}
			sb.WriteString("\n")
		}

		fmt.Fprintf(&sb, "\n**Resolution:** %s\n\n", conflict.Resolution)
	}

	return callToolResult{
		Content: []contentItem{{Type: "text", Text: sb.String()}},
	}
}

// toolFindGaps handles the find_gaps tool call.
func (s *server) toolFindGaps(ctx context.Context, args map[string]any) callToolResult {
	sourceIDs, ok := args["source_repo_ids"].([]interface{})
	if !ok || len(sourceIDs) == 0 {
		return errorResult("source_repo_ids must be a non-empty array")
	}

	targetID, ok := args["target_repo_id"].(string)
	if !ok || targetID == "" {
		return errorResult("target_repo_id is required")
	}

	// Get source contexts
	var sourceContexts []*ctxpkg.RepoContext
	for _, id := range sourceIDs {
		if idStr, ok := id.(string); ok {
			rc, err := s.manager.GetContext(ctx, idStr)
			if err != nil {
				return errorResult(fmt.Sprintf("Failed to get context for %s: %v", idStr, err))
			}
			sourceContexts = append(sourceContexts, rc)
		}
	}

	// Get target context
	targetContext, err := s.manager.GetContext(ctx, targetID)
	if err != nil {
		return errorResult(fmt.Sprintf("Failed to get target context: %v", err))
	}

	gaps, err := s.comparer.FindGaps(ctx, sourceContexts, targetContext)
	if err != nil {
		return errorResult(fmt.Sprintf("Find gaps failed: %v", err))
	}

	if len(gaps) == 0 {
		return callToolResult{
			Content: []contentItem{{Type: "text", Text: "No gaps found. Target repository appears to have all functionality from source repositories."}},
		}
	}

	var sb strings.Builder
	sb.WriteString("# Gap Analysis\n\n")
	fmt.Fprintf(&sb, "Found %d items missing in target repository:\n\n", len(gaps))

	// Group by priority
	highPriority := []comparison.Gap{}
	mediumPriority := []comparison.Gap{}
	lowPriority := []comparison.Gap{}

	for _, gap := range gaps {
		switch gap.Priority {
		case "high":
			highPriority = append(highPriority, gap)
		case "medium":
			mediumPriority = append(mediumPriority, gap)
		default:
			lowPriority = append(lowPriority, gap)
		}
	}

	if len(highPriority) > 0 {
		sb.WriteString("## High Priority\n\n")
		for _, gap := range highPriority {
			fmt.Fprintf(&sb, "- **%s** `%s` - from: %s\n", gap.Type, gap.Name, strings.Join(gap.SourceRepos, ", "))
			if gap.FilePath != "" {
				fmt.Fprintf(&sb, "  - File: `%s`\n", gap.FilePath)
			}
		}
		sb.WriteString("\n")
	}

	if len(mediumPriority) > 0 {
		sb.WriteString("## Medium Priority\n\n")
		for _, gap := range mediumPriority {
			fmt.Fprintf(&sb, "- **%s** `%s` - from: %s\n", gap.Type, gap.Name, strings.Join(gap.SourceRepos, ", "))
		}
		sb.WriteString("\n")
	}

	if len(lowPriority) > 0 {
		sb.WriteString("## Low Priority\n\n")
		for _, gap := range lowPriority {
			fmt.Fprintf(&sb, "- **%s** `%s` - from: %s\n", gap.Type, gap.Name, strings.Join(gap.SourceRepos, ", "))
		}
	}

	return callToolResult{
		Content: []contentItem{{Type: "text", Text: sb.String()}},
	}
}

// toolGenerateAISummary handles the generate_ai_summary tool call.
func (s *server) toolGenerateAISummary(ctx context.Context, args map[string]any) callToolResult {
	repoID, ok := args["repo_id"].(string)
	if !ok || repoID == "" {
		return errorResult("repo_id is required")
	}

	// Check if AI is enabled
	if !s.manager.IsAIEnabled() {
		return errorResult("AI features not available. Please set ANTHROPIC_API_KEY environment variable.")
	}

	summary, err := s.manager.GenerateAISummary(ctx, repoID)
	if err != nil {
		return errorResult(fmt.Sprintf("AI summary generation failed: %v", err))
	}

	var sb strings.Builder
	sb.WriteString("# AI-Generated Repository Summary\n\n")

	fmt.Fprintf(&sb, "**Generated by:** %s\n", summary.Provider)
	fmt.Fprintf(&sb, "**Generated at:** %s\n", summary.GeneratedAt.Format(time.RFC3339))
	fmt.Fprintf(&sb, "**Tokens used:** %d\n\n", summary.TokensUsed)

	if summary.Overview != "" {
		sb.WriteString("## Overview\n\n")
		sb.WriteString(summary.Overview + "\n\n")
	}

	if summary.Purpose != "" {
		sb.WriteString("## Purpose\n\n")
		sb.WriteString(summary.Purpose + "\n\n")
	}

	if summary.ArchitectureStyle != "" {
		fmt.Fprintf(&sb, "## Architecture Style\n\n%s\n\n", summary.ArchitectureStyle)
	}

	if len(summary.KeyFeatures) > 0 {
		sb.WriteString("## Key Features\n\n")
		for _, f := range summary.KeyFeatures {
			fmt.Fprintf(&sb, "- %s\n", f)
		}
		sb.WriteString("\n")
	}

	if len(summary.TechnologyStack) > 0 {
		sb.WriteString("## Technology Stack\n\n")
		for _, t := range summary.TechnologyStack {
			fmt.Fprintf(&sb, "- %s\n", t)
		}
		sb.WriteString("\n")
	}

	if len(summary.MainComponents) > 0 {
		sb.WriteString("## Main Components\n\n")
		for _, c := range summary.MainComponents {
			fmt.Fprintf(&sb, "- %s\n", c)
		}
		sb.WriteString("\n")
	}

	if len(summary.Suggestions) > 0 {
		sb.WriteString("## Suggestions\n\n")
		for _, sug := range summary.Suggestions {
			fmt.Fprintf(&sb, "- %s\n", sug)
		}
	}

	return callToolResult{
		Content: []contentItem{{Type: "text", Text: sb.String()}},
	}
}

// toolGenerateAIArchAnalysis handles the generate_ai_arch_analysis tool call.
func (s *server) toolGenerateAIArchAnalysis(ctx context.Context, args map[string]any) callToolResult {
	repoID, ok := args["repo_id"].(string)
	if !ok || repoID == "" {
		return errorResult("repo_id is required")
	}

	// Check if AI is enabled
	if !s.manager.IsAIEnabled() {
		return errorResult("AI features not available. Please set ANTHROPIC_API_KEY environment variable.")
	}

	analysis, err := s.manager.GenerateAIArchAnalysis(ctx, repoID)
	if err != nil {
		return errorResult(fmt.Sprintf("AI architecture analysis failed: %v", err))
	}

	var sb strings.Builder
	sb.WriteString("# AI-Generated Architecture Analysis\n\n")

	fmt.Fprintf(&sb, "**Generated by:** %s\n", analysis.Provider)
	fmt.Fprintf(&sb, "**Generated at:** %s\n", analysis.GeneratedAt.Format(time.RFC3339))
	fmt.Fprintf(&sb, "**Tokens used:** %d\n\n", analysis.TokensUsed)

	if analysis.Pattern != "" {
		fmt.Fprintf(&sb, "## Architecture Pattern\n\n%s\n\n", analysis.Pattern)
	}

	if len(analysis.Layers) > 0 {
		sb.WriteString("## Layers\n\n")
		for _, layer := range analysis.Layers {
			fmt.Fprintf(&sb, "### %s\n\n", layer.Name)
			fmt.Fprintf(&sb, "**Purpose:** %s\n\n", layer.Purpose)
			if len(layer.Components) > 0 {
				sb.WriteString("**Components:**\n")
				for _, c := range layer.Components {
					fmt.Fprintf(&sb, "- %s\n", c)
				}
				sb.WriteString("\n")
			}
		}
	}

	if analysis.DataFlow != "" {
		sb.WriteString("## Data Flow\n\n")
		sb.WriteString(analysis.DataFlow + "\n\n")
	}

	if len(analysis.Strengths) > 0 {
		sb.WriteString("## Strengths\n\n")
		for _, str := range analysis.Strengths {
			fmt.Fprintf(&sb, "- %s\n", str)
		}
		sb.WriteString("\n")
	}

	if len(analysis.Weaknesses) > 0 {
		sb.WriteString("## Weaknesses\n\n")
		for _, w := range analysis.Weaknesses {
			fmt.Fprintf(&sb, "- %s\n", w)
		}
		sb.WriteString("\n")
	}

	if len(analysis.Recommendations) > 0 {
		sb.WriteString("## Recommendations\n\n")
		for _, r := range analysis.Recommendations {
			fmt.Fprintf(&sb, "- %s\n", r)
		}
	}

	return callToolResult{
		Content: []contentItem{{Type: "text", Text: sb.String()}},
	}
}

// toolRefreshAIContext handles the refresh_ai_context tool call.
func (s *server) toolRefreshAIContext(ctx context.Context, args map[string]any) callToolResult {
	// Check if AI is enabled
	if !s.manager.IsAIEnabled() {
		return errorResult("AI features not available. Please set ANTHROPIC_API_KEY environment variable.")
	}

	// Get repo IDs if specified
	var repoIDs []string
	if ids, ok := args["repo_ids"].([]interface{}); ok {
		for _, id := range ids {
			if idStr, ok := id.(string); ok {
				repoIDs = append(repoIDs, idStr)
			}
		}
	}

	// Check if force refresh is requested
	force := false
	if f, ok := args["force"].(bool); ok {
		force = f
	}

	result, err := s.manager.RefreshAIContext(ctx, repoIDs, force)
	if err != nil {
		return errorResult(fmt.Sprintf("Refresh failed: %v", err))
	}

	var sb strings.Builder
	sb.WriteString("# AI Context Refresh Results\n\n")

	if len(result.Updated) > 0 {
		sb.WriteString("## Updated\n\n")
		for _, repo := range result.Updated {
			fmt.Fprintf(&sb, "- %s\n", repo)
		}
		sb.WriteString("\n")
	}

	if len(result.Skipped) > 0 {
		sb.WriteString("## Skipped (already have AI context)\n\n")
		for _, repo := range result.Skipped {
			fmt.Fprintf(&sb, "- %s\n", repo)
		}
		sb.WriteString("\n")
	}

	if len(result.Failed) > 0 {
		sb.WriteString("## Failed\n\n")
		for _, repo := range result.Failed {
			fmt.Fprintf(&sb, "- %s\n", repo)
		}
		sb.WriteString("\n")
	}

	if len(result.Errors) > 0 {
		sb.WriteString("## Errors\n\n")
		for _, err := range result.Errors {
			fmt.Fprintf(&sb, "- %s\n", err)
		}
		sb.WriteString("\n")
	}

	fmt.Fprintf(&sb, "---\n*Total tokens used: %d*\n", result.TokensUsed)

	return callToolResult{
		Content: []contentItem{{Type: "text", Text: sb.String()}},
	}
}

// toolAsk handles the ask tool call - natural language queries about code.
func (s *server) toolAsk(ctx context.Context, args map[string]any) callToolResult {
	query, ok := args["query"].(string)
	if !ok || query == "" {
		return errorResult("query is required")
	}

	// Check if AI is enabled
	if !s.manager.IsAIEnabled() {
		return errorResult("AI features not available. Please set ANTHROPIC_API_KEY environment variable.")
	}

	// Get repo IDs if specified
	var repoIDs []string
	if ids, ok := args["repo_ids"].([]interface{}); ok {
		for _, id := range ids {
			if idStr, ok := id.(string); ok {
				repoIDs = append(repoIDs, idStr)
			}
		}
	}

	result, err := s.manager.Ask(ctx, query, repoIDs)
	if err != nil {
		return errorResult(fmt.Sprintf("Query failed: %v", err))
	}

	var sb strings.Builder

	// Main answer
	sb.WriteString("## Answer\n\n")
	sb.WriteString(result.Answer)
	sb.WriteString("\n\n")

	// Sources
	if len(result.Sources) > 0 {
		sb.WriteString("---\n\n")
		sb.WriteString("**Sources:**\n")
		for _, src := range result.Sources {
			if src.Function != "" {
				fmt.Fprintf(&sb, "- `%s:%s:%s`", src.RepoID, src.FilePath, src.Function)
				if src.Line > 0 {
					fmt.Fprintf(&sb, " (line %d)", src.Line)
				}
			} else {
				fmt.Fprintf(&sb, "- `%s:%s`", src.RepoID, src.FilePath)
			}
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	}

	// Metadata
	sb.WriteString("---\n")
	fmt.Fprintf(&sb, "*Query type: %s | Tokens: %d | Context files: %d*\n",
		result.QueryType, result.TokensUsed, len(result.ContextUsed))

	return callToolResult{
		Content: []contentItem{{Type: "text", Text: sb.String()}},
	}
}

// toolReviewPR handles the review_pr tool call - AI-powered PR review.
func (s *server) toolReviewPR(ctx context.Context, args map[string]any) callToolResult {
	prURL, ok := args["pr_url"].(string)
	if !ok || prURL == "" {
		return errorResult("pr_url is required")
	}

	// Check if AI is enabled
	if !s.manager.IsAIEnabled() {
		return errorResult("AI features not available. Please set ANTHROPIC_API_KEY environment variable.")
	}

	// Check if context exists for the repository
	contextStatus, err := s.manager.CheckPRContext(ctx, prURL)
	if err != nil {
		return errorResult(fmt.Sprintf("Failed to check context: %v", err))
	}

	// If no context and generate_context is not explicitly true, inform user
	generateContext, _ := args["generate_context"].(bool)
	if !contextStatus.HasContext && !generateContext {
		var sb strings.Builder
		sb.WriteString("## Repository Context Not Available\n\n")
		fmt.Fprintf(&sb, "**Repository:** `%s`\n\n", contextStatus.RepoID)
		sb.WriteString("The repository has not been analyzed yet. For a more comprehensive review that understands:\n")
		sb.WriteString("- Existing code patterns and conventions\n")
		sb.WriteString("- Architecture and design decisions\n")
		sb.WriteString("- Related functions and dependencies\n\n")
		sb.WriteString("**Options:**\n\n")
		sb.WriteString("1. **Generate context first** (recommended):\n")
		sb.WriteString("   ```\n")
		fmt.Fprintf(&sb, "   analyze_repo: repo_url=\"https://github.com/%s\"\n", strings.TrimPrefix(contextStatus.RepoID, "github.com/"))
		sb.WriteString("   ```\n")
		sb.WriteString("   Then run `review_pr` again.\n\n")
		sb.WriteString("2. **Review without context** (limited):\n")
		sb.WriteString("   Re-run with `generate_context=false` to proceed without codebase context.\n")
		sb.WriteString("   ```\n")
		fmt.Fprintf(&sb, "   review_pr: pr_url=\"%s\" skip_context=true\n", prURL)
		sb.WriteString("   ```\n")

		return callToolResult{
			Content: []contentItem{{Type: "text", Text: sb.String()}},
		}
	}

	// Build review options
	opts := prreview.DefaultReviewOptions()
	opts.GitHubToken = s.config.GitHubToken

	if addComments, ok := args["add_comments"].(bool); ok {
		opts.AddComments = addComments
	}
	if severity, ok := args["severity_level"].(string); ok {
		opts.SeverityLevel = severity
	}
	if focusAreas, ok := args["focus_areas"].([]interface{}); ok {
		for _, area := range focusAreas {
			if areaStr, ok := area.(string); ok {
				opts.FocusAreas = append(opts.FocusAreas, areaStr)
			}
		}
	}
	if skipContext, ok := args["skip_context"].(bool); ok && skipContext {
		opts.UseRepoContext = false
	}

	// Perform review
	result, err := s.manager.ReviewPR(ctx, prURL, opts)
	if err != nil {
		return errorResult(fmt.Sprintf("Review failed: %v", err))
	}

	// Format output
	var sb strings.Builder

	sb.WriteString("# AI-Powered PR Review\n\n")
	fmt.Fprintf(&sb, "**PR:** [%s](%s)\n", result.PRInfo.Title, result.PRInfo.URL)
	fmt.Fprintf(&sb, "**Author:** %s\n", result.PRInfo.Author)
	fmt.Fprintf(&sb, "**Branch:** `%s` → `%s`\n", result.PRInfo.HeadBranch, result.PRInfo.BaseBranch)
	fmt.Fprintf(&sb, "**Files Changed:** %d\n", len(result.PRInfo.Files))
	if result.HasContext {
		fmt.Fprintf(&sb, "**Repository Context:** ✅ Available (`%s`)\n", result.RepoID)
	} else {
		sb.WriteString("**Repository Context:** ⚠️ Not available (limited review)\n")
	}
	sb.WriteString("\n")

	// Summary
	sb.WriteString("## Summary\n\n")
	sb.WriteString(result.Summary.Overall + "\n\n")

	// Issue counts
	sb.WriteString("### Issues Found\n\n")
	sb.WriteString("| Type | Count |\n")
	sb.WriteString("|------|-------|\n")
	fmt.Fprintf(&sb, "| 🔴 Critical | %d |\n", result.Summary.CriticalIssues)
	fmt.Fprintf(&sb, "| 🟡 Important | %d |\n", result.Summary.ImportantIssues)
	fmt.Fprintf(&sb, "| 💡 Suggestions | %d |\n", result.Summary.Suggestions)
	fmt.Fprintf(&sb, "| ❓ Questions | %d |\n\n", result.Summary.Questions)

	// Strengths
	if len(result.Summary.Strengths) > 0 {
		sb.WriteString("### ✅ Strengths\n\n")
		for _, s := range result.Summary.Strengths {
			fmt.Fprintf(&sb, "- %s\n", s)
		}
		sb.WriteString("\n")
	}

	// Concerns
	if len(result.Summary.Concerns) > 0 {
		sb.WriteString("### ⚠️ Concerns\n\n")
		for _, c := range result.Summary.Concerns {
			fmt.Fprintf(&sb, "- %s\n", c)
		}
		sb.WriteString("\n")
	}

	// Comments
	if len(result.Comments) > 0 {
		sb.WriteString("### 💬 Review Comments\n\n")
		for _, c := range result.Comments {
			severity := "💬"
			switch c.Severity {
			case "critical":
				severity = "🔴"
			case "important":
				severity = "🟡"
			case "suggestion":
				severity = "💡"
			case "question":
				severity = "❓"
			}
			fmt.Fprintf(&sb, "#### %s `%s:%d`\n\n", severity, c.Path, c.Line)
			sb.WriteString(c.Body + "\n\n")
		}
	}

	// Impact Analysis
	if result.Summary.ImpactAnalysis != "" {
		sb.WriteString("### 📊 Impact Analysis\n\n")
		sb.WriteString(result.Summary.ImpactAnalysis + "\n\n")
	}

	// Pattern Notes (if context was available)
	if result.Summary.PatternNotes != "" && result.HasContext {
		sb.WriteString("### 📐 Pattern Adherence\n\n")
		sb.WriteString(result.Summary.PatternNotes + "\n\n")
	}

	// Recommendations
	if len(result.Summary.Recommendations) > 0 {
		sb.WriteString("### 📝 Recommendations\n\n")
		for i, r := range result.Summary.Recommendations {
			fmt.Fprintf(&sb, "%d. %s\n", i+1, r)
		}
		sb.WriteString("\n")
	}

	// GitHub comment status
	if opts.AddComments {
		sb.WriteString("---\n\n")
		sb.WriteString("### GitHub Integration\n\n")
		fmt.Fprintf(&sb, "- Comments added to PR: %d\n", result.CommentsAdded)
		if result.CommentsFailed > 0 {
			fmt.Fprintf(&sb, "- Comments failed: %d\n", result.CommentsFailed)
			for _, e := range result.CommentErrors {
				fmt.Fprintf(&sb, "  - %s\n", e)
			}
		}
		sb.WriteString("\n")
	}

	// Metadata
	sb.WriteString("---\n")
	fmt.Fprintf(&sb, "*Reviewed by %s | Tokens used: %d | Reviewed at: %s*\n",
		result.Provider, result.TokensUsed, result.ReviewedAt.Format(time.RFC3339))

	return callToolResult{
		Content: []contentItem{{Type: "text", Text: sb.String()}},
	}
}

// toolListSkills handles the list_skills tool call.
func (s *server) toolListSkills(ctx context.Context, args map[string]any) callToolResult {
	category, _ := args["category"].(string)

	var skillList []struct {
		Name        string
		Description string
		Category    string
		Tags        []string
	}

	if category != "" {
		for _, skill := range s.skills.ListByCategory(category) {
			skillList = append(skillList, struct {
				Name        string
				Description string
				Category    string
				Tags        []string
			}{
				Name:        skill.Name,
				Description: skill.Description,
				Category:    skill.Category,
				Tags:        skill.Tags,
			})
		}
	} else {
		for _, skill := range s.skills.List() {
			skillList = append(skillList, struct {
				Name        string
				Description string
				Category    string
				Tags        []string
			}{
				Name:        skill.Name,
				Description: skill.Description,
				Category:    skill.Category,
				Tags:        skill.Tags,
			})
		}
	}

	// Sort by name for consistent output
	sort.Slice(skillList, func(i, j int) bool {
		return skillList[i].Name < skillList[j].Name
	})

	if len(skillList) == 0 {
		return callToolResult{
			Content: []contentItem{{Type: "text", Text: "No skills found" + func() string {
				if category != "" {
					return " for category: " + category
				}
				return ""
			}()}},
		}
	}

	var sb strings.Builder
	sb.WriteString("# Available Skills\n\n")

	if category != "" {
		fmt.Fprintf(&sb, "**Category:** %s\n\n", category)
	}

	sb.WriteString("| Skill | Category | Description |\n")
	sb.WriteString("|-------|----------|-------------|\n")

	for _, skill := range skillList {
		fmt.Fprintf(&sb, "| `%s` | %s | %s |\n", skill.Name, skill.Category, skill.Description)
	}

	sb.WriteString("\n## Categories\n\n")
	sb.WriteString("- **code-review**: PR review, code analysis\n")
	sb.WriteString("- **development**: Go expert, testing, refactoring\n")
	sb.WriteString("- **analysis**: Code structure, architecture\n")
	sb.WriteString("- **security**: Security-focused review\n")
	sb.WriteString("- **performance**: Performance optimization\n")
	sb.WriteString("- **documentation**: Docs generation\n")

	sb.WriteString("\n## Usage\n\n")
	sb.WriteString("Use `get_skill` to retrieve a skill's full prompt:\n")
	sb.WriteString("```\n")
	sb.WriteString("mcp__repo-context__get_skill: name=\"pr-review\"\n")
	sb.WriteString("```\n")

	return callToolResult{
		Content: []contentItem{{Type: "text", Text: sb.String()}},
	}
}

// toolGetSkill handles the get_skill tool call.
func (s *server) toolGetSkill(ctx context.Context, args map[string]any) callToolResult {
	name, ok := args["name"].(string)
	if !ok || name == "" {
		return errorResult("name is required")
	}

	skill, found := s.skills.Get(name)
	if !found {
		// List available skills
		available := s.skills.List()
		var names []string
		for _, sk := range available {
			names = append(names, sk.Name)
		}
		sort.Strings(names)
		return errorResult(fmt.Sprintf("Skill '%s' not found. Available skills: %s", name, strings.Join(names, ", ")))
	}

	var sb strings.Builder

	fmt.Fprintf(&sb, "# Skill: %s\n\n", skill.Name)
	fmt.Fprintf(&sb, "**Description:** %s\n\n", skill.Description)
	fmt.Fprintf(&sb, "**Category:** %s\n\n", skill.Category)

	if len(skill.Tags) > 0 {
		fmt.Fprintf(&sb, "**Tags:** %s\n\n", strings.Join(skill.Tags, ", "))
	}

	if len(skill.UsesTools) > 0 {
		sb.WriteString("**MCP Tools Used:**\n")
		for _, tool := range skill.UsesTools {
			fmt.Fprintf(&sb, "- `%s`\n", tool)
		}
		sb.WriteString("\n")
	}

	if len(skill.Examples) > 0 {
		sb.WriteString("**Examples:**\n")
		for _, ex := range skill.Examples {
			fmt.Fprintf(&sb, "- \"%s\" - %s\n", ex.Input, ex.Description)
		}
		sb.WriteString("\n")
	}

	sb.WriteString("---\n\n")
	sb.WriteString(skill.Prompt)

	return callToolResult{
		Content: []contentItem{{Type: "text", Text: sb.String()}},
	}
}

// ============================================================================
// NEW: Deep Context Tools (No AI Required)
// ============================================================================

// toolGetFunctionContext retrieves comprehensive context for a specific function.
func (s *server) toolGetFunctionContext(ctx context.Context, args map[string]any) callToolResult {
	repoID, ok := args["repo_id"].(string)
	if !ok || repoID == "" {
		return errorResult("repo_id is required")
	}

	filePath, ok := args["file_path"].(string)
	if !ok || filePath == "" {
		return errorResult("file_path is required")
	}

	funcName, ok := args["function_name"].(string)
	if !ok || funcName == "" {
		return errorResult("function_name is required")
	}

	result, err := s.manager.GetFunctionContext(ctx, repoID, filePath, funcName)
	if err != nil {
		return errorResult(fmt.Sprintf("Failed to get function context: %v", err))
	}

	var sb strings.Builder

	// Header
	fmt.Fprintf(&sb, "# Function: `%s`\n\n", result.Function.Name)
	fmt.Fprintf(&sb, "**File:** `%s`\n", result.FilePath)
	fmt.Fprintf(&sb, "**Repository:** `%s`\n", result.RepoID)
	fmt.Fprintf(&sb, "**Lines:** %d-%d\n", result.Function.LineStart, result.Function.LineEnd)
	if result.Function.IsPublic {
		sb.WriteString("**Visibility:** Public\n")
	} else {
		sb.WriteString("**Visibility:** Private\n")
	}
	sb.WriteString("\n")

	// Signature
	sb.WriteString("## Signature\n\n")
	fmt.Fprintf(&sb, "```go\n%s\n```\n\n", result.Function.Signature)

	// Description (from godoc)
	if result.Function.Description != "" {
		sb.WriteString("## Description\n\n")
		sb.WriteString(result.Function.Description + "\n\n")
	}

	// Behavior (auto-extracted)
	if result.Function.Behavior != nil {
		sb.WriteString("## Behavior (Auto-Analyzed)\n\n")

		if result.Function.Behavior.Summary != "" {
			fmt.Fprintf(&sb, "**Summary:** %s\n\n", result.Function.Behavior.Summary)
		}

		if len(result.Function.Behavior.Steps) > 0 {
			sb.WriteString("**Steps:**\n")
			for _, step := range result.Function.Behavior.Steps {
				fmt.Fprintf(&sb, "- %s\n", step)
			}
			sb.WriteString("\n")
		}

		if len(result.Function.Behavior.Patterns) > 0 {
			fmt.Fprintf(&sb, "**Patterns:** %s\n\n", strings.Join(result.Function.Behavior.Patterns, ", "))
		}

		if result.Function.Behavior.OutputSource != "" {
			fmt.Fprintf(&sb, "**Output:** %s\n\n", result.Function.Behavior.OutputSource)
		}
	}

	// What this function calls
	if len(result.Function.Calls) > 0 {
		sb.WriteString("## Calls (What This Function Calls)\n\n")
		for _, call := range result.Function.Calls {
			if call.Package != "" {
				fmt.Fprintf(&sb, "- `%s.%s` (%s)\n", call.Package, call.Function, call.Type)
			} else {
				fmt.Fprintf(&sb, "- `%s` (%s)\n", call.Function, call.Type)
			}
		}
		sb.WriteString("\n")
	}

	// Who calls this function
	if len(result.Callers) > 0 {
		sb.WriteString("## Called By (What Calls This Function)\n\n")
		for _, caller := range result.Callers {
			fmt.Fprintf(&sb, "- `%s` in `%s`\n", caller.Function, caller.File)
		}
		sb.WriteString("\n")
	}

	// Side effects
	if len(result.Function.SideEffects) > 0 {
		sb.WriteString("## Side Effects\n\n")
		for _, effect := range result.Function.SideEffects {
			fmt.Fprintf(&sb, "- %s\n", effect)
		}
		sb.WriteString("\n")
	}

	// Error handling
	if result.Function.ErrorHandling != nil {
		eh := result.Function.ErrorHandling
		sb.WriteString("## Error Handling\n\n")
		fmt.Fprintf(&sb, "- Returns error: %v\n", eh.ReturnsError)
		fmt.Fprintf(&sb, "- Error checks: %d\n", eh.ErrorChecks)
		fmt.Fprintf(&sb, "- Wraps errors: %v\n", eh.WrapsErrors)
		fmt.Fprintf(&sb, "- Logs errors: %v\n", eh.LogsErrors)
		if eh.PanicsOnError {
			sb.WriteString("- ⚠️ Can panic on error\n")
		}
		if len(eh.ErrorTypes) > 0 {
			fmt.Fprintf(&sb, "- Error types: %s\n", strings.Join(eh.ErrorTypes, ", "))
		}
		sb.WriteString("\n")
	}

	// Related types
	if len(result.RelatedTypes) > 0 {
		sb.WriteString("## Related Types\n\n")
		for _, t := range result.RelatedTypes {
			fmt.Fprintf(&sb, "- `%s` (%s) in `%s`\n", t.Name, t.Kind, t.File)
		}
		sb.WriteString("\n")
	}

	// API Flow (comprehensive analysis)
	if result.Function.APIFlow != nil {
		flow := result.Function.APIFlow

		if flow.IsHTTPHandler {
			sb.WriteString("## API Flow (HTTP Handler)\n\n")

			// Request payload
			if flow.RequestPayload != nil {
				sb.WriteString("### Request Payload\n")
				fmt.Fprintf(&sb, "- Type: `%s`\n", flow.RequestPayload.TypeName)
				if flow.RequestPayload.Source != "" {
					fmt.Fprintf(&sb, "- Source: %s\n", flow.RequestPayload.Source)
				}
				sb.WriteString("\n")
			}

			// Response payload
			if flow.ResponsePayload != nil {
				sb.WriteString("### Response Payload\n")
				fmt.Fprintf(&sb, "- Type: `%s`\n", flow.ResponsePayload.TypeName)
				sb.WriteString("\n")
			}
		}

		// Execution steps
		if len(flow.Steps) > 0 {
			sb.WriteString("### Execution Flow\n\n")
			for _, step := range flow.Steps {
				fmt.Fprintf(&sb, "%d. **%s** (line %d)\n", step.StepNumber, step.Action, step.Line)
				if step.Output != "" {
					fmt.Fprintf(&sb, "   - Returns: `%s`\n", step.Output)
				}
			}
			sb.WriteString("\n")
		}

		// Database queries
		if len(flow.DBQueries) > 0 {
			sb.WriteString("### Database Queries\n\n")
			for _, q := range flow.DBQueries {
				fmt.Fprintf(&sb, "- **%s** on `%s` (line %d)\n", q.Operation, q.Table, q.Line)
				if q.RawQuery != "" {
					fmt.Fprintf(&sb, "  ```sql\n  %s\n  ```\n", q.RawQuery)
				}
			}
			sb.WriteString("\n")
		}

		// External HTTP calls
		if len(flow.ExternalCalls) > 0 {
			sb.WriteString("### External HTTP Calls\n\n")
			for _, call := range flow.ExternalCalls {
				fmt.Fprintf(&sb, "- **%s** ", call.Method)
				if call.URL != "" {
					fmt.Fprintf(&sb, "`%s`", call.URL)
				}
				fmt.Fprintf(&sb, " (line %d)\n", call.Line)
			}
			sb.WriteString("\n")
		}

		// Validation steps
		if len(flow.ValidationSteps) > 0 {
			sb.WriteString("### Validations\n\n")
			for _, v := range flow.ValidationSteps {
				fmt.Fprintf(&sb, "- %s\n", v)
			}
			sb.WriteString("\n")
		}
	}

	// Complexity
	if result.Function.Complexity > 0 {
		fmt.Fprintf(&sb, "**Cyclomatic Complexity:** %d\n", result.Function.Complexity)
	}

	return callToolResult{
		Content: []contentItem{{Type: "text", Text: sb.String()}},
	}
}

// toolSearchByConcept searches for functions related to a concept.
func (s *server) toolSearchByConcept(ctx context.Context, args map[string]any) callToolResult {
	repoID, ok := args["repo_id"].(string)
	if !ok || repoID == "" {
		return errorResult("repo_id is required")
	}

	concept, ok := args["concept"].(string)
	if !ok || concept == "" {
		return errorResult("concept is required")
	}

	// Parse max_results (default 20)
	maxResults := 20
	if mr, ok := args["max_results"].(float64); ok && mr > 0 {
		maxResults = int(mr)
		if maxResults > 100 {
			maxResults = 100
		}
	}

	results, err := s.manager.SearchByConcept(ctx, repoID, concept)
	if err != nil {
		return errorResult(fmt.Sprintf("Search failed: %v", err))
	}

	if len(results) == 0 {
		return callToolResult{
			Content: []contentItem{{Type: "text", Text: fmt.Sprintf("No functions found related to concept: %s\n\nAvailable concepts include: authentication, validation, crud, http, database, handler, error_handling, logging, async, initialization, cleanup", concept)}},
		}
	}

	totalCount := len(results)
	truncated := false
	if len(results) > maxResults {
		results = results[:maxResults]
		truncated = true
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "# Functions Related to Concept: `%s`\n\n", concept)
	fmt.Fprintf(&sb, "Found %d functions", len(results))
	if truncated {
		fmt.Fprintf(&sb, " (showing %d of %d - use `max_results` to see more)", len(results), totalCount)
	}
	sb.WriteString("\n\n")

	for _, ref := range results {
		fmt.Fprintf(&sb, "## `%s`\n\n", ref.Function)
		fmt.Fprintf(&sb, "- **File:** `%s:%d`\n", ref.File, ref.Line)
		fmt.Fprintf(&sb, "- **Detail:** `get_function_context repo_id=%s file_path=%s function_name=%s`\n", repoID, ref.File, ref.Function)
		if ref.Signature != "" {
			fmt.Fprintf(&sb, "- **Signature:** `%s`\n", ref.Signature)
		}
		if ref.Summary != "" {
			fmt.Fprintf(&sb, "- **Summary:** %s\n", ref.Summary)
		}
		sb.WriteString("\n")
	}

	return callToolResult{
		Content: []contentItem{{Type: "text", Text: sb.String()}},
	}
}

// toolSearchBySideEffect searches for functions with specific side effects.
func (s *server) toolSearchBySideEffect(ctx context.Context, args map[string]any) callToolResult {
	repoID, ok := args["repo_id"].(string)
	if !ok || repoID == "" {
		return errorResult("repo_id is required")
	}

	effect, ok := args["effect"].(string)
	if !ok || effect == "" {
		return errorResult("effect is required")
	}

	// Parse max_results (default 20)
	maxResults := 20
	if mr, ok := args["max_results"].(float64); ok && mr > 0 {
		maxResults = int(mr)
		if maxResults > 100 {
			maxResults = 100
		}
	}

	results, err := s.manager.SearchBySideEffect(ctx, repoID, effect)
	if err != nil {
		return errorResult(fmt.Sprintf("Search failed: %v", err))
	}

	if len(results) == 0 {
		return callToolResult{
			Content: []contentItem{{Type: "text", Text: fmt.Sprintf("No functions found with side effect: %s\n\nAvailable side effects: http_call, db_query, db_transaction, file_io, io_operation, redis_call, kafka_call, grpc_call, logging, context_management, time_delay, panic", effect)}},
		}
	}

	totalCount := len(results)
	truncated := false
	if len(results) > maxResults {
		results = results[:maxResults]
		truncated = true
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "# Functions with Side Effect: `%s`\n\n", effect)
	fmt.Fprintf(&sb, "Found %d functions", len(results))
	if truncated {
		fmt.Fprintf(&sb, " (showing %d of %d - use `max_results` to see more)", len(results), totalCount)
	}
	sb.WriteString("\n\n")

	for _, ref := range results {
		fmt.Fprintf(&sb, "## `%s`\n\n", ref.Function)
		fmt.Fprintf(&sb, "- **File:** `%s:%d`\n", ref.File, ref.Line)
		fmt.Fprintf(&sb, "- **Detail:** `get_function_context repo_id=%s file_path=%s function_name=%s`\n", repoID, ref.File, ref.Function)
		if ref.Signature != "" {
			fmt.Fprintf(&sb, "- **Signature:** `%s`\n", ref.Signature)
		}
		if ref.Summary != "" {
			fmt.Fprintf(&sb, "- **Summary:** %s\n", ref.Summary)
		}
		sb.WriteString("\n")
	}

	return callToolResult{
		Content: []contentItem{{Type: "text", Text: sb.String()}},
	}
}

// toolGetCallers finds all functions that call a specific function.
func (s *server) toolGetCallers(ctx context.Context, args map[string]any) callToolResult {
	repoID, ok := args["repo_id"].(string)
	if !ok || repoID == "" {
		return errorResult("repo_id is required")
	}

	funcName, ok := args["function_name"].(string)
	if !ok || funcName == "" {
		return errorResult("function_name is required")
	}

	// Get the full repo context to search for callers
	repoCtx, err := s.manager.GetContext(ctx, repoID)
	if err != nil {
		return errorResult(fmt.Sprintf("Failed to get repository context: %v", err))
	}

	// Find all functions that call this function
	var callers []struct {
		File     string
		Function string
		Line     int
		Summary  string
	}

	for path, fileCtx := range repoCtx.Files {
		for _, fn := range fileCtx.Functions {
			for _, call := range fn.Calls {
				if call.Function == funcName {
					summary := ""
					if fn.Behavior != nil {
						summary = fn.Behavior.Summary
					}
					callers = append(callers, struct {
						File     string
						Function string
						Line     int
						Summary  string
					}{
						File:     path,
						Function: fn.Name,
						Line:     call.Line,
						Summary:  summary,
					})
					break // Only count once per calling function
				}
			}
		}
	}

	if len(callers) == 0 {
		return callToolResult{
			Content: []contentItem{{Type: "text", Text: fmt.Sprintf("No functions found that call `%s`.\n\nThis could mean:\n- The function is not called within this repository\n- The function is called from external code\n- The function name is incorrect", funcName)}},
		}
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "# Functions That Call `%s`\n\n", funcName)
	fmt.Fprintf(&sb, "Found %d callers:\n\n", len(callers))

	for _, caller := range callers {
		fmt.Fprintf(&sb, "## `%s`\n\n", caller.Function)
		fmt.Fprintf(&sb, "- **File:** `%s`\n", caller.File)
		fmt.Fprintf(&sb, "- **Call at line:** %d\n", caller.Line)
		if caller.Summary != "" {
			fmt.Fprintf(&sb, "- **Caller behavior:** %s\n", caller.Summary)
		}
		sb.WriteString("\n")
	}

	return callToolResult{
		Content: []contentItem{{Type: "text", Text: sb.String()}},
	}
}

// ============================================================================
// NEW: Local Directory Analysis Tools
// ============================================================================

// toolAnalyzeLocal handles the analyze_local tool call.
func (s *server) toolAnalyzeLocal(ctx context.Context, args map[string]any) callToolResult {
	path, ok := args["path"].(string)
	if !ok || path == "" {
		return errorResult("path is required")
	}

	force, _ := args["force"].(bool)
	includeAll, _ := args["include_all"].(bool)

	opts := orchestrator.AnalyzeLocalOptions{
		Force:      force,
		IncludeAll: includeAll,
	}

	result, err := s.manager.AnalyzeLocal(ctx, path, opts)
	if err != nil {
		return errorResult(fmt.Sprintf("Analysis failed: %v", err))
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "# Local Directory Analyzed\n\n")
	fmt.Fprintf(&sb, "**Project ID:** `%s`\n", result.ProjectID)
	fmt.Fprintf(&sb, "**Project Name:** `%s`\n", result.ProjectName)
	fmt.Fprintf(&sb, "**Project Path:** `%s`\n", result.ProjectPath)
	fmt.Fprintf(&sb, "**Files Analyzed:** %d\n", result.FileCount)
	fmt.Fprintf(&sb, "**Duration:** %s\n", result.Duration.Round(time.Millisecond))

	if result.IsNewAnalysis {
		sb.WriteString("**Status:** New analysis completed\n")
	} else {
		sb.WriteString("**Status:** Using cached analysis\n")
	}

	if len(result.Warnings) > 0 {
		sb.WriteString("\n## Warnings\n\n")
		for _, w := range result.Warnings {
			fmt.Fprintf(&sb, "- %s\n", w)
		}
	}

	sb.WriteString("\n## Usage\n\n")
	sb.WriteString("You can now use the following tools with this project:\n\n")
	fmt.Fprintf(&sb, "- `get_context`: repo_id=\"%s\"\n", result.ProjectID)
	fmt.Fprintf(&sb, "- `search_context`: repo_id=\"%s\" query=\"...\"\n", result.ProjectID)
	fmt.Fprintf(&sb, "- `smart_query`: project_id=\"%s\" query=\"...\"\n", result.ProjectID)
	fmt.Fprintf(&sb, "- `search_by_concept`: repo_id=\"%s\" concept=\"...\"\n", result.ProjectID)
	fmt.Fprintf(&sb, "- `search_by_side_effect`: repo_id=\"%s\" effect=\"...\"\n", result.ProjectID)

	return callToolResult{
		Content: []contentItem{{Type: "text", Text: sb.String()}},
	}
}

// toolSmartQuery handles the smart_query tool call - intelligent query routing without AI.
func (s *server) toolSmartQuery(ctx context.Context, args map[string]any) callToolResult {
	query, ok := args["query"].(string)
	if !ok || query == "" {
		return errorResult("query is required")
	}

	projectID, ok := args["project_id"].(string)
	if !ok || projectID == "" {
		return errorResult("project_id is required")
	}

	result, err := s.manager.SmartQuery(ctx, query, projectID)
	if err != nil {
		return errorResult(fmt.Sprintf("Query failed: %v", err))
	}

	var sb strings.Builder

	// Show query type detected
	fmt.Fprintf(&sb, "*Query type detected: %s (confidence: %.0f%%)*\n\n", result.QueryType, result.Confidence*100)

	// Main answer
	sb.WriteString(result.Answer)

	// Show sources if available
	if len(result.Sources) > 0 {
		sb.WriteString("\n\n---\n**Sources:** ")
		sb.WriteString(strings.Join(result.Sources, ", "))
	}

	// If AI is needed for better answer
	if result.NeedsAI {
		sb.WriteString("\n\n---\n")
		sb.WriteString("💡 *For a more detailed answer, you can use the `ask` tool which uses AI.*")
		if result.SuggestedQuery != "" {
			fmt.Fprintf(&sb, "\n*Suggestion: %s*", result.SuggestedQuery)
		}
	}

	return callToolResult{
		Content: []contentItem{{Type: "text", Text: sb.String()}},
	}
}

// toolGetPackageStructure handles the get_package_structure tool call.
// Returns detailed structure of a package/directory with files, types, and functions.
func (s *server) toolGetPackageStructure(ctx context.Context, args map[string]any) callToolResult {
	projectID, ok := args["project_id"].(string)
	if !ok || projectID == "" {
		return errorResult("project_id is required")
	}

	packagePath, ok := args["package_path"].(string)
	if !ok || packagePath == "" {
		return errorResult("package_path is required")
	}

	// Use smart_query with a package structure query
	query := fmt.Sprintf("structure of %s package", packagePath)
	result, err := s.manager.SmartQuery(ctx, query, projectID)
	if err != nil {
		return errorResult(fmt.Sprintf("Failed to get package structure: %v", err))
	}

	var sb strings.Builder

	// Show query type detected
	fmt.Fprintf(&sb, "*Query type: %s (confidence: %.0f%%)*\n\n", result.QueryType, result.Confidence*100)

	// Main answer
	sb.WriteString(result.Answer)

	// Show sources if available
	if len(result.Sources) > 0 && len(result.Sources) <= 10 {
		sb.WriteString("\n\n---\n**Files included:** ")
		sb.WriteString(strings.Join(result.Sources, ", "))
	} else if len(result.Sources) > 10 {
		sb.WriteString("\n\n---\n")
		fmt.Fprintf(&sb, "**Files included:** %d files (see above for details)", len(result.Sources))
	}

	return callToolResult{
		Content: []contentItem{{Type: "text", Text: sb.String()}},
	}
}

// ============================================================================
// Incremental Update Tools (for refactoring workflows)
// ============================================================================

// toolRefreshFile handles the refresh_file tool call - fast single file update.
func (s *server) toolRefreshFile(ctx context.Context, args map[string]any) callToolResult {
	projectID, ok := args["project_id"].(string)
	if !ok || projectID == "" {
		return errorResult("project_id is required")
	}

	filePath, ok := args["file_path"].(string)
	if !ok || filePath == "" {
		return errorResult("file_path is required")
	}

	force, _ := args["force"].(bool)

	opts := orchestrator.RefreshFileOptions{
		Force: force,
	}

	result, err := s.manager.RefreshFile(ctx, projectID, filePath, opts)
	if err != nil {
		return errorResult(fmt.Sprintf("Refresh failed: %v", err))
	}

	var sb strings.Builder

	if result.Updated {
		fmt.Fprintf(&sb, "✅ **File refreshed:** `%s`\n\n", result.FilePath)
		fmt.Fprintf(&sb, "- **Functions:** %d\n", result.FunctionCount)
		fmt.Fprintf(&sb, "- **Types:** %d\n", result.TypeCount)
		fmt.Fprintf(&sb, "- **Hash:** `%s`\n", result.NewHash[:12])
		if result.WasStale {
			sb.WriteString("- **Status:** File had changed, context updated\n")
		} else {
			sb.WriteString("- **Status:** Forced refresh (file unchanged)\n")
		}
	} else {
		fmt.Fprintf(&sb, "ℹ️ **File unchanged:** `%s`\n\n", result.FilePath)
		fmt.Fprintf(&sb, "- **Functions:** %d\n", result.FunctionCount)
		fmt.Fprintf(&sb, "- **Types:** %d\n", result.TypeCount)
		sb.WriteString("- **Status:** Hash matches, no update needed\n")
		sb.WriteString("\nUse `force: true` to refresh anyway.\n")
	}

	return callToolResult{
		Content: []contentItem{{Type: "text", Text: sb.String()}},
	}
}

// toolRefreshChanged handles the refresh_changed tool call - refresh all modified files.
func (s *server) toolRefreshChanged(ctx context.Context, args map[string]any) callToolResult {
	projectID, ok := args["project_id"].(string)
	if !ok || projectID == "" {
		return errorResult("project_id is required")
	}

	results, err := s.manager.RefreshChangedFiles(ctx, projectID)
	if err != nil {
		return errorResult(fmt.Sprintf("Refresh failed: %v", err))
	}

	var sb strings.Builder

	if len(results) == 0 {
		sb.WriteString("✅ **All files up to date**\n\n")
		sb.WriteString("No files have changed since last analysis.\n")
	} else {
		fmt.Fprintf(&sb, "✅ **Refreshed %d changed files**\n\n", len(results))
		sb.WriteString("| File | Functions | Types | Status |\n")
		sb.WriteString("|------|-----------|-------|--------|\n")

		for _, r := range results {
			status := "updated"
			if !r.WasStale {
				status = "new"
			}
			fmt.Fprintf(&sb, "| `%s` | %d | %d | %s |\n",
				r.FilePath, r.FunctionCount, r.TypeCount, status)
		}
	}

	sb.WriteString("\n*Tip: Run this after refactoring to keep context in sync.*\n")

	return callToolResult{
		Content: []contentItem{{Type: "text", Text: sb.String()}},
	}
}

// toolGetPRContext handles the get_pr_context tool call - rich context for PR changes.
func (s *server) toolGetPRContext(ctx context.Context, args map[string]any) callToolResult {
	repoID, ok := args["repo_id"].(string)
	if !ok || repoID == "" {
		return errorResult("repo_id is required")
	}

	// Parse changed files
	changedFilesRaw, ok := args["changed_files"].([]interface{})
	if !ok || len(changedFilesRaw) == 0 {
		return errorResult("changed_files is required and must be a non-empty array")
	}

	var changedFiles []orchestrator.ChangedFile
	for _, cfRaw := range changedFilesRaw {
		cfMap, ok := cfRaw.(map[string]interface{})
		if !ok {
			continue
		}

		path, _ := cfMap["path"].(string)
		changeType, _ := cfMap["change_type"].(string)

		if path != "" && changeType != "" {
			changedFiles = append(changedFiles, orchestrator.ChangedFile{
				Path:       path,
				ChangeType: changeType,
			})
		}
	}

	if len(changedFiles) == 0 {
		return errorResult("No valid changed files provided")
	}

	result, err := s.manager.GetPRContext(ctx, repoID, changedFiles)
	if err != nil {
		return errorResult(fmt.Sprintf("Failed to get PR context: %v", err))
	}

	// Format the result as markdown
	output := orchestrator.FormatPRContext(result)

	return callToolResult{
		Content: []contentItem{{Type: "text", Text: output}},
	}
}

// ============================================================================
// Cross-repo dependency graph
// ============================================================================

// toolGetDependencyGraph handles the get_dependency_graph tool call.
func (s *server) toolGetDependencyGraph(ctx context.Context, args map[string]any) callToolResult {
	// Parse repo_ids (optional)
	var repoIDs []string
	if raw, ok := args["repo_ids"].([]interface{}); ok {
		for _, v := range raw {
			if id, ok := v.(string); ok && id != "" {
				repoIDs = append(repoIDs, id)
			}
		}
	}

	// Parse include_external (default true)
	includeExternal := true
	if v, ok := args["include_external"].(bool); ok {
		includeExternal = v
	}

	// Parse format (default "text")
	format := "text"
	if v, ok := args["format"].(string); ok && v != "" {
		format = v
	}

	graph, err := s.manager.GetDependencyGraph(ctx, repoIDs, includeExternal)
	if err != nil {
		return errorResult(fmt.Sprintf("Failed to build dependency graph: %v", err))
	}

	if len(graph.Nodes) == 0 {
		return callToolResult{
			Content: []contentItem{{Type: "text", Text: "No dependency information found. Ensure repositories have been analyzed and contain go.mod files."}},
		}
	}

	var output string
	switch format {
	case "json":
		data, err := json.MarshalIndent(graph, "", "  ")
		if err != nil {
			return errorResult(fmt.Sprintf("Failed to marshal graph: %v", err))
		}
		output = string(data)
	case "mermaid":
		output = graphpkg.GenerateDependencyMermaid(graph)
	case "dot":
		output = graphpkg.GenerateDependencyDOT(graph)
	default:
		output = graphpkg.GenerateTextSummary(graph)
	}

	return callToolResult{
		Content: []contentItem{{Type: "text", Text: output}},
	}
}

// ============================================================================
// NEW: Semantic Search Tools (using internal/vectors)
// ============================================================================

// toolSemanticSearch handles the semantic_search tool call - semantic code search.
func (s *server) toolSemanticSearch(ctx context.Context, args map[string]any) callToolResult {
	query, ok := args["query"].(string)
	if !ok || query == "" {
		return errorResult("query is required")
	}

	repoID, ok := args["repo_id"].(string)
	if !ok || repoID == "" {
		return errorResult("repo_id is required")
	}

	// Parse limit (default 10)
	limit := 10
	if l, ok := args["limit"].(float64); ok && l > 0 {
		limit = int(l)
		if limit > 50 {
			limit = 50
		}
	}

	// Parse type filter (default "all")
	searchType := "all"
	if t, ok := args["type"].(string); ok && t != "" {
		searchType = t
	}

	// Check if semantic search is available
	if s.semanticSearch == nil {
		return errorResult("Semantic search is not enabled. Initialize the server with a vector store.")
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "# Semantic Search Results for \"%s\"\n\n", query)
	fmt.Fprintf(&sb, "**Repository:** `%s`\n", repoID)
	fmt.Fprintf(&sb, "**Type filter:** %s\n\n", searchType)

	switch searchType {
	case "function":
		results, err := s.semanticSearch.SearchFunctions(ctx, query, repoID, limit)
		if err != nil {
			return errorResult(fmt.Sprintf("Search failed: %v", err))
		}

		if len(results) == 0 {
			sb.WriteString("No matching functions found.\n")
		} else {
			fmt.Fprintf(&sb, "Found %d matching functions:\n\n", len(results))
			for _, r := range results {
				fmt.Fprintf(&sb, "## `%s` (%.1f%% similar)\n\n", r.Name, r.Similarity*100)
				fmt.Fprintf(&sb, "- **File:** `%s`\n", r.FilePath)
				if r.Signature != "" {
					fmt.Fprintf(&sb, "- **Signature:** `%s`\n", r.Signature)
				}
				if r.Summary != "" {
					fmt.Fprintf(&sb, "- **Summary:** %s\n", r.Summary)
				}
				sb.WriteString("\n")
			}
		}

	case "type":
		results, err := s.semanticSearch.SearchTypes(ctx, query, repoID, limit)
		if err != nil {
			return errorResult(fmt.Sprintf("Search failed: %v", err))
		}

		if len(results) == 0 {
			sb.WriteString("No matching types found.\n")
		} else {
			fmt.Fprintf(&sb, "Found %d matching types:\n\n", len(results))
			for _, r := range results {
				fmt.Fprintf(&sb, "## `%s` (%.1f%% similar)\n\n", r.Name, r.Similarity*100)
				fmt.Fprintf(&sb, "- **File:** `%s`\n", r.FilePath)
				if r.Kind != "" {
					fmt.Fprintf(&sb, "- **Kind:** %s\n", r.Kind)
				}
				if r.Description != "" {
					fmt.Fprintf(&sb, "- **Description:** %s\n", r.Description)
				}
				sb.WriteString("\n")
			}
		}

	default: // "all"
		results, err := s.semanticSearch.SearchAll(ctx, query, repoID, limit)
		if err != nil {
			return errorResult(fmt.Sprintf("Search failed: %v", err))
		}

		if len(results) == 0 {
			sb.WriteString("No matching items found.\n")
		} else {
			fmt.Fprintf(&sb, "Found %d matching items:\n\n", len(results))
			for _, r := range results {
				fmt.Fprintf(&sb, "## %s `%s` (%.1f%% similar)\n\n", r.Record.Type, r.Record.Name, r.Similarity*100)
				fmt.Fprintf(&sb, "- **File:** `%s`\n", r.Record.FilePath)
				if sig, ok := r.Record.Metadata["signature"]; ok && sig != "" {
					fmt.Fprintf(&sb, "- **Signature:** `%s`\n", sig)
				}
				if summary, ok := r.Record.Metadata["summary"]; ok && summary != "" {
					fmt.Fprintf(&sb, "- **Summary:** %s\n", summary)
				}
				if desc, ok := r.Record.Metadata["description"]; ok && desc != "" {
					fmt.Fprintf(&sb, "- **Description:** %s\n", desc)
				}
				sb.WriteString("\n")
			}
		}
	}

	return callToolResult{
		Content: []contentItem{{Type: "text", Text: sb.String()}},
	}
}

// ============================================================================
// NEW: Token-Budgeted Context Tool (using internal/tokens)
// ============================================================================

// toolGetContextBudgeted handles the get_context_budgeted tool call.
func (s *server) toolGetContextBudgeted(ctx context.Context, args map[string]any) callToolResult {
	repoID, ok := args["repo_id"].(string)
	if !ok || repoID == "" {
		return errorResult("repo_id is required")
	}

	query, ok := args["query"].(string)
	if !ok || query == "" {
		return errorResult("query is required")
	}

	// Parse token budget (default 4000)
	tokenBudget := 4000
	if tb, ok := args["token_budget"].(float64); ok && tb > 0 {
		tokenBudget = int(tb)
		if tokenBudget > 32000 {
			tokenBudget = 32000 // Cap at 32k tokens
		}
	}

	// Get the repository context
	repoCtx, err := s.manager.GetContext(ctx, repoID)
	if err != nil {
		return errorResult(fmt.Sprintf("Failed to get repository context: %v", err))
	}

	// Create budgeter and counter
	budgeter := tokens.NewBudgeter()
	counter := tokens.NewTokenCounter()

	// Extract keywords from query for scoring
	queryKeywords := extractSearchKeywords(query)

	// Score and collect functions
	var scoredFunctions []tokens.ScoredItem[ctxpkg.FunctionDef]
	for _, fileCtx := range repoCtx.Files {
		for _, fn := range fileCtx.Functions {
			score := scoreFunctionRelevance(fn, queryKeywords)
			if score > 0 {
				cost := counter.CountJSON(fn)
				scoredFunctions = append(scoredFunctions, tokens.ScoredItem[ctxpkg.FunctionDef]{
					Item:      fn,
					Score:     score,
					TokenCost: cost,
				})
			}
		}
	}

	// Sort by score and select functions within budget
	sort.Slice(scoredFunctions, func(i, j int) bool {
		return scoredFunctions[i].Score > scoredFunctions[j].Score
	})

	// Reserve 500 tokens for metadata/formatting
	contextBudget := tokenBudget - 500
	selectedFunctions := budgeter.BuildFunctionContext(scoredFunctions, contextBudget)

	// Calculate tokens used
	tokensUsed := 0
	for _, fn := range selectedFunctions {
		tokensUsed += counter.CountJSON(fn)
	}

	// Format output
	var sb strings.Builder
	fmt.Fprintf(&sb, "# Context for Query: \"%s\"\n\n", query)
	fmt.Fprintf(&sb, "**Repository:** `%s`\n", repoID)
	fmt.Fprintf(&sb, "**Token Budget:** %d\n", tokenBudget)
	fmt.Fprintf(&sb, "**Tokens Used:** ~%d\n", tokensUsed)
	fmt.Fprintf(&sb, "**Functions Included:** %d of %d matched\n\n", len(selectedFunctions), len(scoredFunctions))

	if len(selectedFunctions) == 0 {
		sb.WriteString("No relevant functions found for the query.\n\n")
		sb.WriteString("Try a different query or use `search_context` for keyword-based search.\n")
	} else {
		sb.WriteString("## Relevant Functions\n\n")
		for _, fn := range selectedFunctions {
			fmt.Fprintf(&sb, "### `%s`\n\n", fn.Name)
			fmt.Fprintf(&sb, "```go\n%s\n```\n\n", fn.Signature)

			if fn.Description != "" {
				fmt.Fprintf(&sb, "**Description:** %s\n\n", fn.Description)
			}

			if fn.Behavior != nil && fn.Behavior.Summary != "" {
				fmt.Fprintf(&sb, "**Behavior:** %s\n\n", fn.Behavior.Summary)
			}

			if len(fn.SideEffects) > 0 {
				fmt.Fprintf(&sb, "**Side Effects:** %s\n\n", strings.Join(fn.SideEffects, ", "))
			}
		}
	}

	// Add summary stats
	sb.WriteString("---\n")
	fmt.Fprintf(&sb, "*Budget efficiency: %.1f%% used*\n", float64(tokensUsed)/float64(tokenBudget)*100)

	return callToolResult{
		Content: []contentItem{{Type: "text", Text: sb.String()}},
	}
}

// scoreFunctionRelevance scores how relevant a function is to the query keywords.
func scoreFunctionRelevance(fn ctxpkg.FunctionDef, keywords []string) float64 {
	if len(keywords) == 0 {
		return 0
	}

	score := 0.0
	text := strings.ToLower(fn.Name + " " + fn.Description + " " + fn.Signature)
	if fn.Behavior != nil {
		text += " " + strings.ToLower(fn.Behavior.Summary)
	}

	for _, kw := range keywords {
		if strings.Contains(text, kw) {
			score += 1.0
			// Boost for name matches
			if strings.Contains(strings.ToLower(fn.Name), kw) {
				score += 0.5
			}
		}
	}

	// Normalize by keyword count
	return score / float64(len(keywords))
}

// ============================================================================
// NEW: Compose Pattern Execution Tool (using internal/compose)
// ============================================================================

// toolExecutePattern handles the execute_pattern tool call.
func (s *server) toolExecutePattern(ctx context.Context, args map[string]any) callToolResult {
	patternName, ok := args["pattern_name"].(string)
	if !ok || patternName == "" {
		return errorResult("pattern_name is required")
	}

	// Get optional parameters
	params, _ := args["params"].(map[string]interface{})
	if params == nil {
		params = make(map[string]interface{})
	}

	// Get the pattern from registry
	if s.patternRegistry == nil {
		s.patternRegistry = compose.DefaultRegistry()
	}

	pattern, found := s.patternRegistry.Get(patternName)
	if !found {
		// List available patterns
		availablePatterns := s.patternRegistry.List()
		var names []string
		for _, p := range availablePatterns {
			names = append(names, p.Name)
		}
		return errorResult(fmt.Sprintf("Pattern '%s' not found. Available patterns: %s", patternName, strings.Join(names, ", ")))
	}

	// Create a tool executor that wraps our MCP tools
	executor := s.createToolExecutor()

	// Execute the pattern
	chainCtx, err := compose.ExecutePattern(ctx, executor, pattern, params)
	if err != nil {
		return errorResult(fmt.Sprintf("Pattern execution failed: %v", err))
	}

	// Format the results
	var sb strings.Builder
	fmt.Fprintf(&sb, "# Pattern Execution: `%s`\n\n", patternName)
	fmt.Fprintf(&sb, "**Description:** %s\n\n", pattern.Description())

	// Show execution summary
	summary := chainCtx.Summary()
	sb.WriteString("## Execution Summary\n\n")
	fmt.Fprintf(&sb, "- **Total Steps:** %d\n", summary.TotalSteps)
	fmt.Fprintf(&sb, "- **Successful:** %d\n", summary.SuccessSteps)
	fmt.Fprintf(&sb, "- **Failed:** %d\n", summary.FailedSteps)
	fmt.Fprintf(&sb, "- **Duration:** %s\n", summary.TotalDuration)
	fmt.Fprintf(&sb, "- **Tokens Used:** ~%d\n", summary.TotalTokens)
	if summary.StopReason != "" {
		fmt.Fprintf(&sb, "- **Stop Reason:** %s\n", summary.StopReason)
	}
	sb.WriteString("\n")

	// Show step results
	sb.WriteString("## Step Results\n\n")
	for i, result := range chainCtx.Results {
		status := "✅"
		if !result.Success {
			status = "❌"
		}
		fmt.Fprintf(&sb, "### Step %d: %s `%s`\n\n", i+1, status, result.ToolName)
		fmt.Fprintf(&sb, "- **Duration:** %s\n", result.Duration)
		if result.TokenCost > 0 {
			fmt.Fprintf(&sb, "- **Token Cost:** ~%d\n", result.TokenCost)
		}
		if result.Error != "" {
			fmt.Fprintf(&sb, "- **Error:** %s\n", result.Error)
		}
		sb.WriteString("\n")

		// Include data preview for successful results
		if result.Success && result.Data != nil {
			dataJSON, err := json.MarshalIndent(result.Data, "", "  ")
			if err == nil {
				preview := string(dataJSON)
				if len(preview) > 500 {
					preview = preview[:500] + "..."
				}
				sb.WriteString("**Result Preview:**\n")
				fmt.Fprintf(&sb, "```json\n%s\n```\n\n", preview)
			}
		}
	}

	// Show collected variables
	if len(chainCtx.Vars) > 0 {
		sb.WriteString("## Collected Data\n\n")
		for key, value := range chainCtx.Vars {
			valueJSON, _ := json.Marshal(value)
			preview := string(valueJSON)
			if len(preview) > 200 {
				preview = preview[:200] + "..."
			}
			fmt.Fprintf(&sb, "- **%s:** `%s`\n", key, preview)
		}
	}

	return callToolResult{
		Content: []contentItem{{Type: "text", Text: sb.String()}},
	}
}

// toolListPatterns handles listing available patterns.
func (s *server) toolListPatterns(ctx context.Context, args map[string]any) callToolResult {
	if s.patternRegistry == nil {
		s.patternRegistry = compose.DefaultRegistry()
	}

	patterns := s.patternRegistry.List()

	var sb strings.Builder
	sb.WriteString("# Available Patterns\n\n")
	sb.WriteString("Patterns are pre-defined tool chains that automate common workflows.\n\n")

	if len(patterns) == 0 {
		sb.WriteString("No patterns registered.\n")
	} else {
		sb.WriteString("| Pattern | Description |\n")
		sb.WriteString("|---------|-------------|\n")
		for _, p := range patterns {
			fmt.Fprintf(&sb, "| `%s` | %s |\n", p.Name, p.Description)
		}
	}

	sb.WriteString("\n## Usage\n\n")
	sb.WriteString("Use `execute_pattern` with a pattern name and parameters:\n")
	sb.WriteString("```\n")
	sb.WriteString("execute_pattern:\n")
	sb.WriteString("  pattern_name: \"search_with_context\"\n")
	sb.WriteString("  params:\n")
	sb.WriteString("    repo_id: \"github.com/org/repo\"\n")
	sb.WriteString("    query: \"authentication\"\n")
	sb.WriteString("```\n")

	return callToolResult{
		Content: []contentItem{{Type: "text", Text: sb.String()}},
	}
}

// createToolExecutor creates a compose.ToolExecutor that wraps MCP tool calls.
func (s *server) createToolExecutor() compose.ToolExecutor {
	return compose.NewFuncExecutor(func(chainCtx *compose.ChainContext, call compose.ToolCall) compose.ToolResult {
		// Convert params to map[string]any
		args := make(map[string]any)
		for k, v := range call.Params {
			args[k] = v
		}

		start := time.Now()

		// Route to appropriate tool
		var result callToolResult
		switch call.Name {
		case "search_context":
			result = s.toolSearchContext(chainCtx.Context, args)
		case "get_function_context":
			result = s.toolGetFunctionContext(chainCtx.Context, args)
		case "get_callers":
			result = s.toolGetCallers(chainCtx.Context, args)
		case "search_by_concept":
			result = s.toolSearchByConcept(chainCtx.Context, args)
		case "search_by_side_effect":
			result = s.toolSearchBySideEffect(chainCtx.Context, args)
		case "get_context":
			result = s.toolGetContext(chainCtx.Context, args)
		case "smart_query":
			result = s.toolSmartQuery(chainCtx.Context, args)
		default:
			return compose.ToolResult{
				ToolName: call.Name,
				Success:  false,
				Error:    fmt.Sprintf("tool '%s' not supported in patterns", call.Name),
			}
		}

		duration := time.Since(start)

		// Convert to compose.ToolResult
		composeResult := compose.ToolResult{
			ToolName: call.Name,
			Success:  !result.IsError,
			Duration: duration.String(),
		}

		if result.IsError {
			if len(result.Content) > 0 {
				composeResult.Error = result.Content[0].Text
			}
		} else {
			// Extract text content as data
			if len(result.Content) > 0 {
				composeResult.Data = result.Content[0].Text
				composeResult.TokenCost = len(result.Content[0].Text) / 4 // Approximate
			}
		}

		return composeResult
	})
}

// toolIndexRepository handles indexing a repository for semantic search.
func (s *server) toolIndexRepository(ctx context.Context, args map[string]any) callToolResult {
	repoID, ok := args["repo_id"].(string)
	if !ok || repoID == "" {
		return errorResult("repo_id is required")
	}

	if s.semanticSearch == nil {
		return errorResult("Semantic search is not enabled. Initialize the server with a vector store.")
	}

	// Get repository context
	repoCtx, err := s.manager.GetContext(ctx, repoID)
	if err != nil {
		return errorResult(fmt.Sprintf("Failed to get repository context: %v", err))
	}

	// Check if force re-index
	force, _ := args["force"].(bool)

	// Check existing index count
	existingCount, _ := s.semanticSearch.Count(ctx, repoID)
	if existingCount > 0 && !force {
		return callToolResult{
			Content: []contentItem{{Type: "text", Text: fmt.Sprintf(
				"Repository `%s` already indexed with %d items.\n\nUse `force: true` to re-index.",
				repoID, existingCount,
			)}},
		}
	}

	// Clear existing vectors if re-indexing
	if force && existingCount > 0 {
		if err := s.semanticSearch.ClearRepository(ctx, repoID); err != nil {
			return errorResult(fmt.Sprintf("Failed to clear existing index: %v", err))
		}
	}

	// Index the repository
	start := time.Now()
	if err := s.semanticSearch.IndexRepository(ctx, repoCtx); err != nil {
		return errorResult(fmt.Sprintf("Indexing failed: %v", err))
	}
	duration := time.Since(start)

	// Get new count
	newCount, _ := s.semanticSearch.Count(ctx, repoID)

	var sb strings.Builder
	sb.WriteString("# Repository Indexed for Semantic Search\n\n")
	fmt.Fprintf(&sb, "**Repository:** `%s`\n", repoID)
	fmt.Fprintf(&sb, "**Items Indexed:** %d\n", newCount)
	fmt.Fprintf(&sb, "**Duration:** %s\n\n", duration.Round(time.Millisecond))
	sb.WriteString("You can now use `semantic_search` to find similar code:\n")
	fmt.Fprintf(&sb, "```\nsemantic_search:\n  repo_id: \"%s\"\n  query: \"your search query\"\n```\n", repoID)

	return callToolResult{
		Content: []contentItem{{Type: "text", Text: sb.String()}},
	}
}

// Ensure vectors package types are used (to avoid unused import)
var _ vectors.Embedder = (*vectors.LocalEmbedder)(nil)

// ============================================================================
// Call Graph Visualization Tool
// ============================================================================

// toolVisualizeCallGraph generates a visual representation of function call relationships.
func (s *server) toolVisualizeCallGraph(ctx context.Context, args map[string]any) callToolResult {
	repoID, ok := args["repo_id"].(string)
	if !ok || repoID == "" {
		return errorResult("repo_id is required")
	}

	funcName, ok := args["function_name"].(string)
	if !ok || funcName == "" {
		return errorResult("function_name is required")
	}

	// Parse format (default "mermaid")
	format := "mermaid"
	if f, ok := args["format"].(string); ok && f != "" {
		format = f
	}

	// Parse depth (default 2, max 5)
	depth := 2
	if d, ok := args["depth"].(float64); ok && d > 0 {
		depth = int(d)
		if depth > 5 {
			depth = 5
		}
	}

	// Get repository context
	repoCtx, err := s.manager.GetContext(ctx, repoID)
	if err != nil {
		return errorResult(fmt.Sprintf("Failed to get repository context: %v", err))
	}

	if repoCtx == nil {
		return errorResult(fmt.Sprintf("Repository '%s' not found. Use analyze_repo or analyze_local first.", repoID))
	}

	// Build call graph from context
	// Find callers and callees for the function
	type funcNode struct {
		Name  string
		File  string
		Calls []string
	}

	// Build function map
	funcMap := make(map[string]*funcNode)
	for path, fileCtx := range repoCtx.Files {
		for _, fn := range fileCtx.Functions {
			node := &funcNode{
				Name:  fn.Name,
				File:  path,
				Calls: make([]string, 0),
			}
			for _, call := range fn.Calls {
				node.Calls = append(node.Calls, call.Function)
			}
			funcMap[fn.Name] = node
		}
	}

	// Check if function exists
	if _, found := funcMap[funcName]; !found {
		return errorResult(fmt.Sprintf("Function '%s' not found in repository", funcName))
	}

	// BFS to find callers and callees up to depth
	callers := make(map[string]int)   // function -> depth at which found
	callees := make(map[string]int)   // function -> depth at which found
	edges := make(map[string]bool)    // "from->to" -> exists

	// Find callees (what this function calls)
	queue := []struct {
		name  string
		depth int
	}{{funcName, 0}}

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		if current.depth >= depth {
			continue
		}

		if node, ok := funcMap[current.name]; ok {
			for _, callee := range node.Calls {
				edgeKey := current.name + "->" + callee
				if !edges[edgeKey] {
					edges[edgeKey] = true
					if _, found := callees[callee]; !found {
						callees[callee] = current.depth + 1
						queue = append(queue, struct {
							name  string
							depth int
						}{callee, current.depth + 1})
					}
				}
			}
		}
	}

	// Find callers (what calls this function)
	for name, node := range funcMap {
		for _, callee := range node.Calls {
			if callee == funcName {
				callers[name] = 1
				edgeKey := name + "->" + funcName
				edges[edgeKey] = true
			}
		}
	}

	// Generate output based on format
	var sb strings.Builder

	if format == "mermaid" {
		fmt.Fprintf(&sb, "# Call Graph for `%s`\n\n", funcName)
		fmt.Fprintf(&sb, "**Repository:** `%s`\n", repoID)
		fmt.Fprintf(&sb, "**Depth:** %d\n\n", depth)

		sb.WriteString("```mermaid\nflowchart TB\n")

		// Style the central node
		fmt.Fprintf(&sb, "    %s[[\"%s\"]]\n", sanitizeNodeID(funcName), funcName)
		fmt.Fprintf(&sb, "    style %s fill:#f9f,stroke:#333,stroke-width:4px\n", sanitizeNodeID(funcName))

		// Add caller nodes
		for caller := range callers {
			fmt.Fprintf(&sb, "    %s[\"%s\"]\n", sanitizeNodeID(caller), caller)
			fmt.Fprintf(&sb, "    %s --> %s\n", sanitizeNodeID(caller), sanitizeNodeID(funcName))
		}

		// Add callee nodes and edges
		for edge := range edges {
			parts := strings.Split(edge, "->")
			if len(parts) == 2 && parts[0] == funcName {
				fmt.Fprintf(&sb, "    %s[\"%s\"]\n", sanitizeNodeID(parts[1]), parts[1])
				fmt.Fprintf(&sb, "    %s --> %s\n", sanitizeNodeID(parts[0]), sanitizeNodeID(parts[1]))
			}
		}

		sb.WriteString("```\n\n")
	} else { // DOT format
		fmt.Fprintf(&sb, "# Call Graph for `%s` (DOT format)\n\n", funcName)
		fmt.Fprintf(&sb, "**Repository:** `%s`\n", repoID)
		fmt.Fprintf(&sb, "**Depth:** %d\n\n", depth)

		sb.WriteString("```dot\ndigraph callgraph {\n")
		sb.WriteString("    rankdir=TB;\n")
		sb.WriteString("    node [shape=box];\n")

		// Style the central node
		fmt.Fprintf(&sb, "    \"%s\" [style=filled, fillcolor=pink, penwidth=3];\n", funcName)

		// Add edges
		for edge := range edges {
			parts := strings.Split(edge, "->")
			if len(parts) == 2 {
				fmt.Fprintf(&sb, "    \"%s\" -> \"%s\";\n", parts[0], parts[1])
			}
		}

		sb.WriteString("}\n```\n\n")
	}

	// Add summary
	sb.WriteString("## Summary\n\n")
	fmt.Fprintf(&sb, "- **Direct callers:** %d\n", len(callers))
	fmt.Fprintf(&sb, "- **Direct/indirect callees:** %d\n", len(callees))
	fmt.Fprintf(&sb, "- **Total edges:** %d\n", len(edges))

	return callToolResult{
		Content: []contentItem{{Type: "text", Text: sb.String()}},
	}
}

// toolGetUsageStats returns usage statistics for MCP tools.
func (s *server) toolGetUsageStats(ctx context.Context, args map[string]any) callToolResult {
	if s.usageTracker == nil {
		return errorResult("Usage tracking is not enabled")
	}

	// Check if a specific tool is requested
	toolName, _ := args["tool"].(string)
	limit := 100
	if l, ok := args["limit"].(float64); ok && l > 0 {
		limit = int(l)
	}

	var sb strings.Builder

	if toolName != "" {
		// Get usage for specific tool
		usage, err := s.usageTracker.GetToolUsage(ctx, toolName, limit)
		if err != nil {
			return errorResult(fmt.Sprintf("Failed to get tool usage: %v", err))
		}

		sb.WriteString(fmt.Sprintf("# Usage Statistics for `%s`\n\n", toolName))
		sb.WriteString(fmt.Sprintf("Recent %d calls:\n\n", len(usage)))
		sb.WriteString("| Timestamp | Duration | Input Tokens | Output Tokens | Success |\n")
		sb.WriteString("|-----------|----------|--------------|---------------|--------|\n")

		for _, u := range usage {
			status := "✓"
			if !u.Success {
				status = "✗"
			}
			sb.WriteString(fmt.Sprintf("| %s | %dms | %d | %d | %s |\n",
				u.Timestamp.Format("2006-01-02 15:04"), u.DurationMs, u.InputTokens, u.OutputTokens, status))
		}
	} else {
		// Get overall stats
		stats, err := s.usageTracker.GetStats(ctx)
		if err != nil {
			return errorResult(fmt.Sprintf("Failed to get usage stats: %v", err))
		}

		sb.WriteString("# MCP Tool Usage Statistics\n\n")
		sb.WriteString("## Overview\n\n")
		sb.WriteString(fmt.Sprintf("- **Total Calls:** %d\n", stats.TotalCalls))
		sb.WriteString(fmt.Sprintf("- **Total Input Tokens:** %d\n", stats.TotalInputTokens))
		sb.WriteString(fmt.Sprintf("- **Total Output Tokens:** %d\n", stats.TotalOutputTokens))

		if len(stats.ToolStats) > 0 {
			sb.WriteString("\n## Per-Tool Statistics\n\n")
			sb.WriteString("| Tool | Calls | Avg Duration | Avg Input | Avg Output | Success Rate |\n")
			sb.WriteString("|------|-------|--------------|-----------|------------|-------------|\n")

			for _, ts := range stats.ToolStats {
				successRate := float64(0)
				if ts.TotalCalls > 0 {
					successRate = float64(ts.SuccessfulCalls) / float64(ts.TotalCalls) * 100
				}
				sb.WriteString(fmt.Sprintf("| `%s` | %d | %.0fms | %.0f | %.0f | %.1f%% |\n",
					ts.Tool, ts.TotalCalls, ts.AvgDurationMs, ts.AvgInputTokens, ts.AvgOutputTokens, successRate))
			}
		}
	}

	return callToolResult{
		Content: []contentItem{{Type: "text", Text: sb.String()}},
	}
}

// sanitizeNodeID sanitizes a function name for use as a Mermaid node ID.
func sanitizeNodeID(name string) string {
	// Replace special characters that could break Mermaid syntax
	result := strings.ReplaceAll(name, ".", "_")
	result = strings.ReplaceAll(result, "-", "_")
	result = strings.ReplaceAll(result, "(", "_")
	result = strings.ReplaceAll(result, ")", "_")
	result = strings.ReplaceAll(result, " ", "_")
	return result
}
