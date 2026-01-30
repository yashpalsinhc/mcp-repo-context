package orchestrator

import (
	"context"
	"fmt"
	"strings"

	ctxpkg "github.com/yashpalc/mcp-repo-context/internal/context"
)

// PRContextResult contains rich context for PR changes.
type PRContextResult struct {
	RepoID         string                    `json:"repo_id"`
	HasContext     bool                      `json:"has_context"`
	ChangedFiles   []FileChangeContext       `json:"changed_files"`
	ImpactAnalysis ImpactAnalysis            `json:"impact_analysis"`
	Summary        PRContextSummary          `json:"summary"`
}

// FileChangeContext provides context for a single changed file.
type FileChangeContext struct {
	FilePath         string                   `json:"file_path"`
	ChangeType       string                   `json:"change_type"` // added, modified, deleted
	Package          string                   `json:"package,omitempty"`
	Purpose          string                   `json:"purpose,omitempty"`

	// Functions in this file
	AddedFunctions    []FunctionChangeContext `json:"added_functions,omitempty"`
	ModifiedFunctions []FunctionChangeContext `json:"modified_functions,omitempty"`
	DeletedFunctions  []string                `json:"deleted_functions,omitempty"`

	// Types in this file
	AddedTypes       []TypeChangeContext      `json:"added_types,omitempty"`
	ModifiedTypes    []TypeChangeContext      `json:"modified_types,omitempty"`
}

// FunctionChangeContext provides deep context for a changed function.
type FunctionChangeContext struct {
	Name        string   `json:"name"`
	Signature   string   `json:"signature"`
	IsPublic    bool     `json:"is_public"`
	LineStart   int      `json:"line_start"`
	LineEnd     int      `json:"line_end"`

	// Behavior (auto-extracted, no AI)
	Summary     string   `json:"summary,omitempty"`
	Steps       []string `json:"steps,omitempty"`
	Patterns    []string `json:"patterns,omitempty"`

	// Relationships
	Callers     []CallerInfo   `json:"callers,omitempty"`      // Who calls this function
	Callees     []CalleeInfo   `json:"callees,omitempty"`      // What this function calls

	// Side effects
	SideEffects []string       `json:"side_effects,omitempty"`
	DBQueries   []DBQueryInfo  `json:"db_queries,omitempty"`
	HTTPCalls   []HTTPCallInfo `json:"http_calls,omitempty"`

	// Error handling
	ReturnsError  bool     `json:"returns_error"`
	ErrorTypes    []string `json:"error_types,omitempty"`

	// Related types used by this function
	RelatedTypes []string `json:"related_types,omitempty"`
}

// TypeChangeContext provides context for a changed type.
type TypeChangeContext struct {
	Name      string   `json:"name"`
	Kind      string   `json:"kind"` // struct, interface, alias
	IsPublic  bool     `json:"is_public"`
	Fields    []string `json:"fields,omitempty"`
	Methods   []string `json:"methods,omitempty"`
	UsedBy    []string `json:"used_by,omitempty"` // Functions that use this type
}

// CallerInfo describes a function that calls the changed function.
type CallerInfo struct {
	Function  string `json:"function"`
	File      string `json:"file"`
	Line      int    `json:"line"`
	Summary   string `json:"summary,omitempty"`
}

// CalleeInfo describes a function called by the changed function.
type CalleeInfo struct {
	Function  string `json:"function"`
	Package   string `json:"package,omitempty"`
	Type      string `json:"type"` // internal, stdlib, external
	Line      int    `json:"line"`
}

// DBQueryInfo describes a database query.
type DBQueryInfo struct {
	Operation string `json:"operation"` // SELECT, INSERT, UPDATE, DELETE
	Table     string `json:"table"`
	Query     string `json:"query,omitempty"`
}

// HTTPCallInfo describes an HTTP call.
type HTTPCallInfo struct {
	Method string `json:"method"`
	URL    string `json:"url,omitempty"`
}

// ImpactAnalysis shows the broader impact of changes.
type ImpactAnalysis struct {
	// Functions that will be affected by changes (callers of modified functions)
	AffectedFunctions []AffectedFunction `json:"affected_functions"`

	// APIs/Routes that include changed code
	AffectedRoutes []AffectedRoute `json:"affected_routes,omitempty"`

	// Database tables that might be affected
	AffectedTables []string `json:"affected_tables,omitempty"`

	// External services that might be affected
	AffectedServices []string `json:"affected_services,omitempty"`

	// Risk assessment
	RiskLevel   string   `json:"risk_level"` // low, medium, high
	RiskReasons []string `json:"risk_reasons,omitempty"`
}

// AffectedFunction describes a function affected by the PR changes.
type AffectedFunction struct {
	Function     string `json:"function"`
	File         string `json:"file"`
	Reason       string `json:"reason"` // "calls modified function X"
	IsPublicAPI  bool   `json:"is_public_api"`
}

// AffectedRoute describes an API route affected by changes.
type AffectedRoute struct {
	Method   string `json:"method"`
	Path     string `json:"path"`
	Handler  string `json:"handler"`
	File     string `json:"file"`
}

// PRContextSummary provides a high-level summary.
type PRContextSummary struct {
	TotalFilesChanged     int    `json:"total_files_changed"`
	TotalFunctionsChanged int    `json:"total_functions_changed"`
	TotalTypesChanged     int    `json:"total_types_changed"`
	PublicAPIChanges      int    `json:"public_api_changes"`
	InternalChanges       int    `json:"internal_changes"`
	HasDBChanges          bool   `json:"has_db_changes"`
	HasHTTPChanges        bool   `json:"has_http_changes"`
	ImpactedFunctions     int    `json:"impacted_functions"`
	ImpactedRoutes        int    `json:"impacted_routes"`
}

// ChangedFile represents a file changed in a PR.
type ChangedFile struct {
	Path       string
	ChangeType string // added, modified, deleted
	Additions  int
	Deletions  int
}

// GetPRContext extracts rich context for PR changes.
// This works WITHOUT AI - uses pre-analyzed context only.
func (m *manager) GetPRContext(ctx context.Context, repoID string, changedFiles []ChangedFile) (*PRContextResult, error) {
	result := &PRContextResult{
		RepoID:       repoID,
		ChangedFiles: []FileChangeContext{},
		ImpactAnalysis: ImpactAnalysis{
			AffectedFunctions: []AffectedFunction{},
			AffectedRoutes:    []AffectedRoute{},
			AffectedTables:    []string{},
			AffectedServices:  []string{},
			RiskReasons:       []string{},
		},
	}

	// Try to get repo context
	repoCtx, err := m.store.GetRepoContext(ctx, repoID)
	if err != nil {
		result.HasContext = false
		return result, nil
	}
	result.HasContext = true

	// Track all modified functions for impact analysis
	modifiedFuncs := make(map[string]bool)
	affectedTables := make(map[string]bool)
	affectedServices := make(map[string]bool)

	// Process each changed file
	for _, cf := range changedFiles {
		fileChange := FileChangeContext{
			FilePath:   cf.Path,
			ChangeType: cf.ChangeType,
		}

		fileCtx, exists := repoCtx.Files[cf.Path]
		if !exists {
			// File not in our context (might be new or non-code file)
			if cf.ChangeType == "added" {
				fileChange.AddedFunctions = []FunctionChangeContext{{
					Name:    "(new file - not yet analyzed)",
					Summary: "Run refresh_file to analyze this new file",
				}}
			}
			result.ChangedFiles = append(result.ChangedFiles, fileChange)
			continue
		}

		fileChange.Package = fileCtx.Package
		fileChange.Purpose = fileCtx.Purpose

		// Process functions in this file
		for _, fn := range fileCtx.Functions {
			funcCtx := m.buildFunctionContext(repoCtx, cf.Path, &fn)

			// Track for impact analysis
			modifiedFuncs[fn.Name] = true

			// Track DB tables and services
			for _, q := range funcCtx.DBQueries {
				if q.Table != "" {
					affectedTables[q.Table] = true
				}
			}
			for _, h := range funcCtx.HTTPCalls {
				if h.URL != "" {
					affectedServices[h.URL] = true
				}
			}

			// Categorize as added or modified based on change type
			if cf.ChangeType == "added" {
				fileChange.AddedFunctions = append(fileChange.AddedFunctions, funcCtx)
			} else {
				fileChange.ModifiedFunctions = append(fileChange.ModifiedFunctions, funcCtx)
			}
		}

		// Process types in this file
		for _, t := range fileCtx.Types {
			typeCtx := TypeChangeContext{
				Name:     t.Name,
				Kind:     t.Kind,
				IsPublic: t.IsPublic,
				Methods:  t.Methods,
			}

			for _, f := range t.Fields {
				typeCtx.Fields = append(typeCtx.Fields, fmt.Sprintf("%s %s", f.Name, f.Type))
			}

			// Find functions that use this type
			typeCtx.UsedBy = m.findTypeUsage(repoCtx, t.Name)

			if cf.ChangeType == "added" {
				fileChange.AddedTypes = append(fileChange.AddedTypes, typeCtx)
			} else {
				fileChange.ModifiedTypes = append(fileChange.ModifiedTypes, typeCtx)
			}
		}

		result.ChangedFiles = append(result.ChangedFiles, fileChange)
	}

	// Build impact analysis
	result.ImpactAnalysis = m.buildImpactAnalysis(repoCtx, modifiedFuncs, changedFiles)

	// Add affected tables and services
	for table := range affectedTables {
		result.ImpactAnalysis.AffectedTables = append(result.ImpactAnalysis.AffectedTables, table)
	}
	for service := range affectedServices {
		result.ImpactAnalysis.AffectedServices = append(result.ImpactAnalysis.AffectedServices, service)
	}

	// Build summary
	result.Summary = m.buildPRSummary(result)

	return result, nil
}

// buildFunctionContext creates detailed context for a function.
func (m *manager) buildFunctionContext(repoCtx *ctxpkg.RepoContext, filePath string, fn *ctxpkg.FunctionDef) FunctionChangeContext {
	ctx := FunctionChangeContext{
		Name:       fn.Name,
		Signature:  fn.Signature,
		IsPublic:   fn.IsPublic,
		LineStart:  fn.LineStart,
		LineEnd:    fn.LineEnd,
		SideEffects: fn.SideEffects,
	}

	// Add behavior
	if fn.Behavior != nil {
		ctx.Summary = fn.Behavior.Summary
		ctx.Steps = fn.Behavior.Steps
		ctx.Patterns = fn.Behavior.Patterns
	}

	// Add callers
	for _, caller := range fn.CalledBy {
		callerInfo := CallerInfo{
			Function: caller.Function,
			File:     caller.File,
			Line:     caller.Line,
		}
		// Get caller's summary
		if callerFile, ok := repoCtx.Files[caller.File]; ok {
			for _, callerFn := range callerFile.Functions {
				if callerFn.Name == caller.Function && callerFn.Behavior != nil {
					callerInfo.Summary = callerFn.Behavior.Summary
					break
				}
			}
		}
		ctx.Callers = append(ctx.Callers, callerInfo)
	}

	// Add callees
	for _, call := range fn.Calls {
		ctx.Callees = append(ctx.Callees, CalleeInfo{
			Function: call.Function,
			Package:  call.Package,
			Type:     call.Type,
			Line:     call.Line,
		})
	}

	// Add API flow details
	if fn.APIFlow != nil {
		for _, q := range fn.APIFlow.DBQueries {
			ctx.DBQueries = append(ctx.DBQueries, DBQueryInfo{
				Operation: q.Operation,
				Table:     q.Table,
				Query:     q.RawQuery,
			})
		}
		for _, h := range fn.APIFlow.ExternalCalls {
			ctx.HTTPCalls = append(ctx.HTTPCalls, HTTPCallInfo{
				Method: h.Method,
				URL:    h.URL,
			})
		}
	}

	// Add error handling
	if fn.ErrorHandling != nil {
		ctx.ReturnsError = fn.ErrorHandling.ReturnsError
		ctx.ErrorTypes = fn.ErrorHandling.ErrorTypes
	}

	// Find related types (from parameters and returns)
	for _, p := range fn.Parameters {
		typeName := extractTypeName(p.Type)
		if typeName != "" && !isBuiltinType(typeName) {
			ctx.RelatedTypes = append(ctx.RelatedTypes, typeName)
		}
	}
	for _, r := range fn.Returns {
		typeName := extractTypeName(r)
		if typeName != "" && !isBuiltinType(typeName) {
			ctx.RelatedTypes = append(ctx.RelatedTypes, typeName)
		}
	}

	return ctx
}

// buildImpactAnalysis determines what's affected by the changes.
func (m *manager) buildImpactAnalysis(repoCtx *ctxpkg.RepoContext, modifiedFuncs map[string]bool, changedFiles []ChangedFile) ImpactAnalysis {
	analysis := ImpactAnalysis{
		AffectedFunctions: []AffectedFunction{},
		AffectedRoutes:    []AffectedRoute{},
		RiskReasons:       []string{},
	}

	changedPaths := make(map[string]bool)
	for _, cf := range changedFiles {
		changedPaths[cf.Path] = true
	}

	// Find functions that call modified functions (but aren't themselves modified)
	for path, fileCtx := range repoCtx.Files {
		if changedPaths[path] {
			continue // Skip files that are already changed
		}

		for _, fn := range fileCtx.Functions {
			for _, call := range fn.Calls {
				if modifiedFuncs[call.Function] {
					analysis.AffectedFunctions = append(analysis.AffectedFunctions, AffectedFunction{
						Function:    fn.Name,
						File:        path,
						Reason:      fmt.Sprintf("calls modified function %s", call.Function),
						IsPublicAPI: fn.IsPublic,
					})
					break // Only count once per function
				}
			}
		}
	}

	// Find routes that might be affected
	for path, fileCtx := range repoCtx.Files {
		for _, route := range fileCtx.Routes {
			// Check if route handler is in modified files or calls modified functions
			if changedPaths[path] {
				analysis.AffectedRoutes = append(analysis.AffectedRoutes, AffectedRoute{
					Method:  route.Method,
					Path:    route.Path,
					Handler: route.Handler,
					File:    path,
				})
			} else {
				// Check if any function in route's file calls modified functions
				for _, fn := range fileCtx.Functions {
					if fn.Name == route.Handler {
						for _, call := range fn.Calls {
							if modifiedFuncs[call.Function] {
								analysis.AffectedRoutes = append(analysis.AffectedRoutes, AffectedRoute{
									Method:  route.Method,
									Path:    route.Path,
									Handler: route.Handler,
									File:    path,
								})
								break
							}
						}
					}
				}
			}
		}
	}

	// Assess risk
	analysis.RiskLevel = "low"

	if len(analysis.AffectedFunctions) > 10 {
		analysis.RiskLevel = "high"
		analysis.RiskReasons = append(analysis.RiskReasons,
			fmt.Sprintf("High impact: %d functions affected", len(analysis.AffectedFunctions)))
	} else if len(analysis.AffectedFunctions) > 3 {
		analysis.RiskLevel = "medium"
		analysis.RiskReasons = append(analysis.RiskReasons,
			fmt.Sprintf("Moderate impact: %d functions affected", len(analysis.AffectedFunctions)))
	}

	if len(analysis.AffectedRoutes) > 0 {
		if analysis.RiskLevel == "low" {
			analysis.RiskLevel = "medium"
		}
		analysis.RiskReasons = append(analysis.RiskReasons,
			fmt.Sprintf("API changes: %d routes affected", len(analysis.AffectedRoutes)))
	}

	// Check for public API changes
	publicAPIChanges := 0
	for _, af := range analysis.AffectedFunctions {
		if af.IsPublicAPI {
			publicAPIChanges++
		}
	}
	if publicAPIChanges > 0 {
		analysis.RiskLevel = "high"
		analysis.RiskReasons = append(analysis.RiskReasons,
			fmt.Sprintf("Public API impact: %d public functions affected", publicAPIChanges))
	}

	return analysis
}

// buildPRSummary creates a summary of the PR context.
func (m *manager) buildPRSummary(result *PRContextResult) PRContextSummary {
	summary := PRContextSummary{
		TotalFilesChanged: len(result.ChangedFiles),
	}

	for _, fc := range result.ChangedFiles {
		summary.TotalFunctionsChanged += len(fc.AddedFunctions) + len(fc.ModifiedFunctions)
		summary.TotalTypesChanged += len(fc.AddedTypes) + len(fc.ModifiedTypes)

		for _, fn := range fc.AddedFunctions {
			if fn.IsPublic {
				summary.PublicAPIChanges++
			} else {
				summary.InternalChanges++
			}
			if len(fn.DBQueries) > 0 {
				summary.HasDBChanges = true
			}
			if len(fn.HTTPCalls) > 0 {
				summary.HasHTTPChanges = true
			}
		}
		for _, fn := range fc.ModifiedFunctions {
			if fn.IsPublic {
				summary.PublicAPIChanges++
			} else {
				summary.InternalChanges++
			}
			if len(fn.DBQueries) > 0 {
				summary.HasDBChanges = true
			}
			if len(fn.HTTPCalls) > 0 {
				summary.HasHTTPChanges = true
			}
		}
	}

	summary.ImpactedFunctions = len(result.ImpactAnalysis.AffectedFunctions)
	summary.ImpactedRoutes = len(result.ImpactAnalysis.AffectedRoutes)

	return summary
}

// findTypeUsage finds functions that use a given type.
func (m *manager) findTypeUsage(repoCtx *ctxpkg.RepoContext, typeName string) []string {
	var usages []string

	for _, fileCtx := range repoCtx.Files {
		for _, fn := range fileCtx.Functions {
			// Check parameters
			for _, p := range fn.Parameters {
				if strings.Contains(p.Type, typeName) {
					usages = append(usages, fn.Name)
					break
				}
			}
			// Check returns
			for _, r := range fn.Returns {
				if strings.Contains(r, typeName) {
					usages = append(usages, fn.Name)
					break
				}
			}
		}
	}

	return usages
}

// extractTypeName extracts the base type name from a type string.
func extractTypeName(typeStr string) string {
	// Remove pointers, slices, maps
	typeStr = strings.TrimPrefix(typeStr, "*")
	typeStr = strings.TrimPrefix(typeStr, "[]")
	typeStr = strings.TrimPrefix(typeStr, "map[")

	// Get the base name
	if idx := strings.Index(typeStr, "."); idx > 0 {
		return typeStr[idx+1:]
	}
	if idx := strings.Index(typeStr, "]"); idx > 0 {
		return typeStr[idx+1:]
	}

	return typeStr
}

// FormatPRContext formats the PR context as readable markdown.
func FormatPRContext(result *PRContextResult) string {
	var sb strings.Builder

	sb.WriteString("# PR Context Analysis\n\n")

	if !result.HasContext {
		sb.WriteString("⚠️ **Repository not analyzed.** Run `analyze_repo` first for full context.\n\n")
		return sb.String()
	}

	// Summary
	sb.WriteString("## Summary\n\n")
	s := result.Summary
	fmt.Fprintf(&sb, "| Metric | Count |\n")
	fmt.Fprintf(&sb, "|--------|-------|\n")
	fmt.Fprintf(&sb, "| Files Changed | %d |\n", s.TotalFilesChanged)
	fmt.Fprintf(&sb, "| Functions Changed | %d |\n", s.TotalFunctionsChanged)
	fmt.Fprintf(&sb, "| Types Changed | %d |\n", s.TotalTypesChanged)
	fmt.Fprintf(&sb, "| Public API Changes | %d |\n", s.PublicAPIChanges)
	fmt.Fprintf(&sb, "| Impacted Functions | %d |\n", s.ImpactedFunctions)
	fmt.Fprintf(&sb, "| Impacted Routes | %d |\n", s.ImpactedRoutes)
	sb.WriteString("\n")

	if s.HasDBChanges {
		sb.WriteString("🗄️ **Contains database operations**\n")
	}
	if s.HasHTTPChanges {
		sb.WriteString("🌐 **Contains external HTTP calls**\n")
	}
	sb.WriteString("\n")

	// Risk Assessment
	sb.WriteString("## Risk Assessment\n\n")
	riskEmoji := "🟢"
	if result.ImpactAnalysis.RiskLevel == "medium" {
		riskEmoji = "🟡"
	} else if result.ImpactAnalysis.RiskLevel == "high" {
		riskEmoji = "🔴"
	}
	fmt.Fprintf(&sb, "%s **Risk Level:** %s\n\n", riskEmoji, strings.ToUpper(result.ImpactAnalysis.RiskLevel))

	if len(result.ImpactAnalysis.RiskReasons) > 0 {
		for _, reason := range result.ImpactAnalysis.RiskReasons {
			fmt.Fprintf(&sb, "- %s\n", reason)
		}
		sb.WriteString("\n")
	}

	// Changed Files Detail
	sb.WriteString("## Changed Files\n\n")
	for _, fc := range result.ChangedFiles {
		fmt.Fprintf(&sb, "### `%s` (%s)\n\n", fc.FilePath, fc.ChangeType)

		if fc.Package != "" {
			fmt.Fprintf(&sb, "**Package:** `%s`\n", fc.Package)
		}
		if fc.Purpose != "" {
			fmt.Fprintf(&sb, "**Purpose:** %s\n", fc.Purpose)
		}
		sb.WriteString("\n")

		// Functions
		allFuncs := append(fc.AddedFunctions, fc.ModifiedFunctions...)
		if len(allFuncs) > 0 {
			sb.WriteString("#### Functions\n\n")
			for _, fn := range allFuncs {
				fmt.Fprintf(&sb, "##### `%s`\n\n", fn.Name)
				fmt.Fprintf(&sb, "**Signature:** `%s`\n", fn.Signature)
				if fn.Summary != "" {
					fmt.Fprintf(&sb, "**What it does:** %s\n", fn.Summary)
				}
				sb.WriteString("\n")

				// Callers (who uses this)
				if len(fn.Callers) > 0 {
					sb.WriteString("**Called by:**\n")
					for _, c := range fn.Callers {
						fmt.Fprintf(&sb, "- `%s` in `%s`", c.Function, c.File)
						if c.Summary != "" {
							fmt.Fprintf(&sb, " - %s", c.Summary)
						}
						sb.WriteString("\n")
					}
					sb.WriteString("\n")
				}

				// Callees (what this calls)
				if len(fn.Callees) > 0 {
					internalCalls := []CalleeInfo{}
					externalCalls := []CalleeInfo{}
					for _, c := range fn.Callees {
						if c.Type == "internal" {
							internalCalls = append(internalCalls, c)
						} else {
							externalCalls = append(externalCalls, c)
						}
					}

					if len(internalCalls) > 0 {
						sb.WriteString("**Internal calls:**\n")
						for _, c := range internalCalls {
							fmt.Fprintf(&sb, "- `%s` (line %d)\n", c.Function, c.Line)
						}
					}
					if len(externalCalls) > 0 {
						sb.WriteString("**External calls:**\n")
						for _, c := range externalCalls {
							if c.Package != "" {
								fmt.Fprintf(&sb, "- `%s.%s`\n", c.Package, c.Function)
							} else {
								fmt.Fprintf(&sb, "- `%s`\n", c.Function)
							}
						}
					}
					sb.WriteString("\n")
				}

				// DB Queries
				if len(fn.DBQueries) > 0 {
					sb.WriteString("**Database queries:**\n")
					for _, q := range fn.DBQueries {
						fmt.Fprintf(&sb, "- **%s** on `%s`", q.Operation, q.Table)
						if q.Query != "" {
							fmt.Fprintf(&sb, ": `%s`", q.Query)
						}
						sb.WriteString("\n")
					}
					sb.WriteString("\n")
				}

				// HTTP Calls
				if len(fn.HTTPCalls) > 0 {
					sb.WriteString("**HTTP calls:**\n")
					for _, h := range fn.HTTPCalls {
						fmt.Fprintf(&sb, "- **%s** %s\n", h.Method, h.URL)
					}
					sb.WriteString("\n")
				}

				// Side effects
				if len(fn.SideEffects) > 0 {
					fmt.Fprintf(&sb, "**Side effects:** %s\n\n", strings.Join(fn.SideEffects, ", "))
				}
			}
		}

		// Types
		allTypes := append(fc.AddedTypes, fc.ModifiedTypes...)
		if len(allTypes) > 0 {
			sb.WriteString("#### Types\n\n")
			for _, t := range allTypes {
				fmt.Fprintf(&sb, "##### `%s` (%s)\n\n", t.Name, t.Kind)
				if len(t.Fields) > 0 {
					sb.WriteString("**Fields:**\n")
					for _, f := range t.Fields {
						fmt.Fprintf(&sb, "- `%s`\n", f)
					}
					sb.WriteString("\n")
				}
				if len(t.UsedBy) > 0 {
					fmt.Fprintf(&sb, "**Used by:** %s\n\n", strings.Join(t.UsedBy, ", "))
				}
			}
		}
	}

	// Impact Analysis
	if len(result.ImpactAnalysis.AffectedFunctions) > 0 {
		sb.WriteString("## Impact Analysis\n\n")
		sb.WriteString("### Functions Affected (Not in PR)\n\n")
		sb.WriteString("These functions call modified code and may need testing:\n\n")
		for _, af := range result.ImpactAnalysis.AffectedFunctions {
			emoji := "🔧"
			if af.IsPublicAPI {
				emoji = "🌐"
			}
			fmt.Fprintf(&sb, "- %s `%s` in `%s` - %s\n", emoji, af.Function, af.File, af.Reason)
		}
		sb.WriteString("\n")
	}

	if len(result.ImpactAnalysis.AffectedRoutes) > 0 {
		sb.WriteString("### API Routes Affected\n\n")
		sb.WriteString("| Method | Path | Handler | File |\n")
		sb.WriteString("|--------|------|---------|------|\n")
		for _, r := range result.ImpactAnalysis.AffectedRoutes {
			fmt.Fprintf(&sb, "| %s | `%s` | `%s` | `%s` |\n", r.Method, r.Path, r.Handler, r.File)
		}
		sb.WriteString("\n")
	}

	if len(result.ImpactAnalysis.AffectedTables) > 0 {
		fmt.Fprintf(&sb, "### Database Tables Affected\n\n%s\n\n",
			strings.Join(result.ImpactAnalysis.AffectedTables, ", "))
	}

	return sb.String()
}
