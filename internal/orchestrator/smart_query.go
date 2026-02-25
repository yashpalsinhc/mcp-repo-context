package orchestrator

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"

	ctxpkg "github.com/yashpalc/mcp-repo-context/internal/context"
	"github.com/yashpalc/mcp-repo-context/internal/nlp"
)

// QueryType represents the type of query being asked.
type QueryType string

const (
	QueryTypeFunction     QueryType = "function"      // Find/describe a function
	QueryTypeType         QueryType = "type"          // Find/describe a type
	QueryTypeSideEffect   QueryType = "side_effect"   // Find by side effect
	QueryTypeConcept      QueryType = "concept"       // Find by concept
	QueryTypeCallers      QueryType = "callers"       // Who calls this
	QueryTypeCalls        QueryType = "calls"         // What does this call
	QueryTypeFlow         QueryType = "flow"          // Trace execution flow
	QueryTypeFile         QueryType = "file"          // File-level context
	QueryTypePackage      QueryType = "package"       // Package/directory structure
	QueryTypeArchitecture QueryType = "architecture"  // Project structure
	QueryTypeGeneral      QueryType = "general"       // General question (needs AI)
)

// SmartQueryResult contains the result of a smart query.
type SmartQueryResult struct {
	QueryType      QueryType            `json:"query_type"`
	Answer         string               `json:"answer"`
	Functions      []ctxpkg.FunctionRef `json:"functions,omitempty"`
	Types          []TypeRef            `json:"types,omitempty"`
	Files          []string             `json:"files,omitempty"`
	FlowSteps      []FlowTraceStep      `json:"flow_steps,omitempty"`
	Sources        []string             `json:"sources,omitempty"`
	Confidence     float64              `json:"confidence"`
	NeedsAI        bool                 `json:"needs_ai"`
	SuggestedQuery string               `json:"suggested_query,omitempty"`
}

// TypeRef references a type definition.
type TypeRef struct {
	Name string `json:"name"`
	Kind string `json:"kind"`
	File string `json:"file"`
	Line int    `json:"line"`
}

// FlowTraceStep represents a step in an execution flow trace.
type FlowTraceStep struct {
	StepNumber  int      `json:"step_number"`
	Function    string   `json:"function"`
	File        string   `json:"file"`
	Line        int      `json:"line"`
	Action      string   `json:"action"`
	Calls       []string `json:"calls,omitempty"`
	DBQueries   []string `json:"db_queries,omitempty"`
	HTTPCalls   []string `json:"http_calls,omitempty"`
}

// SmartQuery analyzes the query and routes to appropriate tools.
// This is the main entry point that handles complex queries WITHOUT AI.
func (m *manager) SmartQuery(ctx context.Context, query string, projectID string) (*SmartQueryResult, error) {
	// Parse the query to understand intent
	queryType, extracted, parsingConfidence := parseQuery(query)

	// Get project context
	repoCtx, err := m.store.GetRepoContext(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("project not found: %s - run analyze first", projectID)
	}

	result := &SmartQueryResult{
		QueryType:  queryType,
		Confidence: parsingConfidence,
		Sources:    []string{},
	}

	var res *SmartQueryResult

	switch queryType {
	case QueryTypeFunction:
		res, err = m.handleFunctionQuery(ctx, repoCtx, extracted, result)

	case QueryTypeType:
		res, err = m.handleTypeQuery(ctx, repoCtx, extracted, result)

	case QueryTypeSideEffect:
		res, err = m.handleSideEffectQuery(ctx, repoCtx, extracted, result)

	case QueryTypeConcept:
		res, err = m.handleConceptQuery(ctx, repoCtx, extracted, result)

	case QueryTypeCallers:
		res, err = m.handleCallersQuery(ctx, repoCtx, extracted, result)

	case QueryTypeCalls:
		res, err = m.handleCallsQuery(ctx, repoCtx, extracted, result)

	case QueryTypeFlow:
		res, err = m.handleFlowQuery(ctx, repoCtx, extracted, result)

	case QueryTypeFile:
		res, err = m.handleFileQuery(ctx, repoCtx, extracted, result)

	case QueryTypePackage:
		res, err = m.handlePackageQuery(ctx, repoCtx, extracted, result)

	case QueryTypeArchitecture:
		res, err = m.handleArchitectureQuery(ctx, repoCtx, result)

	default:
		// General query - try to answer from context, flag if AI needed
		res, err = m.handleGeneralQuery(ctx, repoCtx, query, result)
	}

	if err != nil {
		return nil, err
	}

	// Enforce confidence ceiling: handler cannot raise above parsing confidence
	if res.Confidence > parsingConfidence {
		res.Confidence = parsingConfidence
	}

	// AI fallback threshold
	if res.Confidence < 0.6 {
		res.NeedsAI = true
	}

	return res, nil
}

// architectureWords are words that should route to architecture queries, not type lookups.
var architectureWords = map[string]bool{
	"structure": true, "architecture": true, "layout": true,
	"overview": true, "project": true, "codebase": true,
}

// parseQuery analyzes the query and extracts key information.
// Returns the query type, extracted parameters, and a parsing confidence score.
func parseQuery(query string) (QueryType, map[string]string, float64) {
	queryLower := strings.ToLower(query)
	extracted := make(map[string]string)

	// Function patterns
	funcPatterns := []string{
		`what does (?:the )?(?:function )?["\x60]?(\w+)["\x60]? do`,
		`how does (?:the )?(?:function )?["\x60]?(\w+)["\x60]? work`,
		`explain (?:the )?(?:function )?["\x60]?(\w+)["\x60]?`,
		`find (?:the )?function ["\x60]?(\w+)["\x60]?`,
		`show (?:me )?(?:the )?function ["\x60]?(\w+)["\x60]?`,
		`describe (?:the )?function ["\x60]?(\w+)["\x60]?`,
	}
	for _, pattern := range funcPatterns {
		re := regexp.MustCompile(pattern)
		if matches := re.FindStringSubmatch(queryLower); len(matches) > 1 {
			extracted["function"] = matches[1]
			return QueryTypeFunction, extracted, 0.9
		}
	}

	// Type/struct patterns with architecture guard
	typePatterns := []string{
		`what is (?:the )?(?:type |struct )?["\x60]?(\w+)["\x60]?`,
		`show (?:me )?(?:the )?(?:type |struct )?["\x60]?(\w+)["\x60]? fields`,
		`describe (?:the )?(?:type |struct )?["\x60]?(\w+)["\x60]?`,
	}
	for _, pattern := range typePatterns {
		re := regexp.MustCompile(pattern)
		if matches := re.FindStringSubmatch(queryLower); len(matches) > 1 {
			word := matches[1]
			// Architecture guard: prevent architecture words from matching as type names
			if architectureWords[word] {
				return QueryTypeArchitecture, extracted, 0.7
			}
			extracted["type"] = word
			return QueryTypeType, extracted, 0.9
		}
	}

	// Side effect patterns
	sideEffectKeywords := map[string]string{
		"database":    "db_query",
		"db":          "db_query",
		"sql":         "db_query",
		"query":       "db_query",
		"http":        "http_call",
		"api call":    "http_call",
		"external":    "http_call",
		"file":        "file_io",
		"read file":   "file_io",
		"write file":  "file_io",
		"log":         "logging",
		"logging":     "logging",
	}
	for keyword, effect := range sideEffectKeywords {
		if strings.Contains(queryLower, keyword) &&
			(strings.Contains(queryLower, "find") ||
				strings.Contains(queryLower, "show") ||
				strings.Contains(queryLower, "which") ||
				strings.Contains(queryLower, "what") ||
				strings.Contains(queryLower, "list")) {
			extracted["effect"] = effect
			return QueryTypeSideEffect, extracted, 0.8
		}
	}

	// Concept patterns
	conceptKeywords := []string{
		"authentication", "auth", "login",
		"validation", "validate",
		"handler", "endpoint", "route",
		"middleware",
		"error", "exception",
		"cache", "caching",
		"crud", "create", "read", "update", "delete",
	}
	for _, concept := range conceptKeywords {
		if strings.Contains(queryLower, concept) &&
			(strings.Contains(queryLower, "how") ||
				strings.Contains(queryLower, "where") ||
				strings.Contains(queryLower, "find") ||
				strings.Contains(queryLower, "show")) {
			extracted["concept"] = concept
			return QueryTypeConcept, extracted, 0.8
		}
	}

	// Callers patterns
	callerPatterns := []string{
		`who calls ["\x60]?(\w+)["\x60]?`,
		`what calls ["\x60]?(\w+)["\x60]?`,
		`callers of ["\x60]?(\w+)["\x60]?`,
		`where is ["\x60]?(\w+)["\x60]? called`,
		`where is ["\x60]?(\w+)["\x60]? used`,
	}
	for _, pattern := range callerPatterns {
		re := regexp.MustCompile(pattern)
		if matches := re.FindStringSubmatch(queryLower); len(matches) > 1 {
			extracted["function"] = matches[1]
			return QueryTypeCallers, extracted, 0.9
		}
	}

	// Calls patterns
	callsPatterns := []string{
		`what does ["\x60]?(\w+)["\x60]? call`,
		`what functions does ["\x60]?(\w+)["\x60]? use`,
		`dependencies of ["\x60]?(\w+)["\x60]?`,
	}
	for _, pattern := range callsPatterns {
		re := regexp.MustCompile(pattern)
		if matches := re.FindStringSubmatch(queryLower); len(matches) > 1 {
			extracted["function"] = matches[1]
			return QueryTypeCalls, extracted, 0.9
		}
	}

	// Flow patterns
	flowPatterns := []string{
		`trace (?:the )?(?:flow |execution )?(?:of |from )?["\x60]?(\w+)["\x60]?`,
		`flow of ["\x60]?(\w+)["\x60]?`,
		`execution flow`,
		`request flow`,
		`api flow`,
	}
	for _, pattern := range flowPatterns {
		re := regexp.MustCompile(pattern)
		if matches := re.FindStringSubmatch(queryLower); len(matches) > 1 {
			extracted["function"] = matches[1]
			return QueryTypeFlow, extracted, 0.9
		}
	}
	if strings.Contains(queryLower, "flow") || strings.Contains(queryLower, "trace") {
		return QueryTypeFlow, extracted, 0.7
	}

	// File patterns (specific file with extension)
	filePatterns := []string{
		`show (?:me )?(?:the )?file ["\x60]?([^\s"]+\.[a-z]+)["\x60]?`,
		`explain ["\x60]?([^\s"]+\.(?:go|js|ts|py))["\x60]?`,
	}
	for _, pattern := range filePatterns {
		re := regexp.MustCompile(pattern)
		if matches := re.FindStringSubmatch(queryLower); len(matches) > 1 {
			extracted["file"] = matches[1]
			return QueryTypeFile, extracted, 0.9
		}
	}

	// Package/directory structure patterns - BEFORE architecture patterns
	// Matches: "structure of X package", "files in X", "what's in X folder", etc.
	packagePatterns := []string{
		`structure of (?:the )?["\x60]?([^\s"]+)["\x60]? (?:package|folder|directory|dir)`,
		`(?:package|folder|directory|dir) structure (?:of |for )?["\x60]?([^\s"]+)["\x60]?`,
		`what(?:'s| is)(?: the)? (?:structure|layout) (?:of )?(?:the )?["\x60]?([^\s"]+)["\x60]?`,
		`show (?:me )?(?:all )?files in ["\x60]?([^\s"]+)["\x60]?`,
		`what(?:'s| is) in (?:the )?["\x60]?([^\s"/]+(?:/[^\s"/]+)+)["\x60]?`, // Match paths like service/test/create
		`files (?:and|&) (?:their )?purposes? in ["\x60]?([^\s"]+)["\x60]?`,
		`list files in ["\x60]?([^\s"]+)["\x60]?`,
		`(?:describe|explain) (?:the )?["\x60]?([^\s"/]+(?:/[^\s"/]+)+)["\x60]? (?:package|folder|directory)`,
	}
	for _, pattern := range packagePatterns {
		re := regexp.MustCompile(pattern)
		if matches := re.FindStringSubmatch(queryLower); len(matches) > 1 {
			extracted["package"] = matches[1]
			return QueryTypePackage, extracted, 0.9
		}
	}

	// Check for path-like queries with structure keywords
	if (strings.Contains(queryLower, "structure") || strings.Contains(queryLower, "files")) &&
		strings.Contains(queryLower, "/") {
		// Extract the path from the query
		words := strings.Fields(queryLower)
		for _, word := range words {
			if strings.Contains(word, "/") {
				extracted["package"] = strings.Trim(word, `"'\x60?.,`)
				return QueryTypePackage, extracted, 0.8
			}
		}
	}

	// Architecture patterns (for whole project, not specific packages)
	if strings.Contains(queryLower, "architecture") ||
		strings.Contains(queryLower, "project structure") ||
		strings.Contains(queryLower, "codebase structure") ||
		strings.Contains(queryLower, "overview") ||
		strings.Contains(queryLower, "project layout") {
		return QueryTypeArchitecture, extracted, 0.7
	}

	return QueryTypeGeneral, extracted, 0.5
}

// handleFunctionQuery handles function-related queries.
func (m *manager) handleFunctionQuery(ctx context.Context, repoCtx *ctxpkg.RepoContext, extracted map[string]string, result *SmartQueryResult) (*SmartQueryResult, error) {
	funcName := extracted["function"]
	if funcName == "" {
		result.NeedsAI = true
		result.Answer = "Could not identify function name in query"
		return result, nil
	}

	// Search for the function - exact match first
	var foundFunc *ctxpkg.FunctionDef
	var foundFile string
	stemmedMatch := false

	for path, fileCtx := range repoCtx.Files {
		for i := range fileCtx.Functions {
			fn := &fileCtx.Functions[i]
			if strings.EqualFold(fn.Name, funcName) {
				foundFunc = fn
				foundFile = path
				break
			}
		}
		if foundFunc != nil {
			break
		}
	}

	// Second pass: stemmed matching if exact match not found
	if foundFunc == nil {
		queryStem := nlp.Stem(funcName)
		for path, fileCtx := range repoCtx.Files {
			for i := range fileCtx.Functions {
				fn := &fileCtx.Functions[i]
				if nlp.Stem(strings.ToLower(fn.Name)) == queryStem {
					foundFunc = fn
					foundFile = path
					stemmedMatch = true
					break
				}
			}
			if foundFunc != nil {
				break
			}
		}
	}

	// Fall through to concept search if function not found
	if foundFunc == nil {
		extracted["concept"] = funcName
		conceptResult, err := m.handleConceptQuery(ctx, repoCtx, extracted, result)
		if err != nil {
			return nil, err
		}
		// If concept search found results, return those
		if len(conceptResult.Functions) > 0 {
			conceptResult.QueryType = QueryTypeConcept
			return conceptResult, nil
		}
		// Neither function nor concept found - suggest AI
		result.Answer = fmt.Sprintf("No function or concept matching `%s` found. Try the `ask` tool for complex questions.", funcName)
		result.Confidence = 0.3
		result.NeedsAI = true
		result.SuggestedQuery = fmt.Sprintf("ask: How does %s work?", funcName)
		return result, nil
	}

	// Adjust confidence for stemmed matches
	if stemmedMatch {
		result.Confidence = 0.75
	}

	// Build comprehensive answer
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("## Function: `%s`\n\n", foundFunc.Name))
	sb.WriteString(fmt.Sprintf("**File:** `%s` (lines %d-%d)\n", foundFile, foundFunc.LineStart, foundFunc.LineEnd))
	sb.WriteString(fmt.Sprintf("**Signature:** `%s`\n\n", foundFunc.Signature))

	if foundFunc.Description != "" {
		sb.WriteString(fmt.Sprintf("**Description:** %s\n\n", foundFunc.Description))
	}

	// Behavior
	if foundFunc.Behavior != nil {
		sb.WriteString("### What It Does\n\n")
		if foundFunc.Behavior.Summary != "" {
			sb.WriteString(foundFunc.Behavior.Summary + "\n\n")
		}
		if len(foundFunc.Behavior.Steps) > 0 {
			sb.WriteString("**Steps:**\n")
			for _, step := range foundFunc.Behavior.Steps {
				sb.WriteString(fmt.Sprintf("- %s\n", step))
			}
			sb.WriteString("\n")
		}
		if len(foundFunc.Behavior.Patterns) > 0 {
			sb.WriteString(fmt.Sprintf("**Patterns:** %s\n\n", strings.Join(foundFunc.Behavior.Patterns, ", ")))
		}
	}

	// API Flow
	if foundFunc.APIFlow != nil {
		flow := foundFunc.APIFlow
		if flow.IsHTTPHandler {
			sb.WriteString("### API Handler\n\n")
			if flow.RequestPayload != nil {
				sb.WriteString(fmt.Sprintf("**Request:** `%s`\n", flow.RequestPayload.TypeName))
			}
			if flow.ResponsePayload != nil {
				sb.WriteString(fmt.Sprintf("**Response:** `%s`\n", flow.ResponsePayload.TypeName))
			}
		}

		if len(flow.DBQueries) > 0 {
			sb.WriteString("\n### Database Queries\n\n")
			for _, q := range flow.DBQueries {
				sb.WriteString(fmt.Sprintf("- **%s** on `%s`", q.Operation, q.Table))
				if q.RawQuery != "" {
					sb.WriteString(fmt.Sprintf(": `%s`", q.RawQuery))
				}
				sb.WriteString("\n")
			}
		}

		if len(flow.ExternalCalls) > 0 {
			sb.WriteString("\n### External HTTP Calls\n\n")
			for _, c := range flow.ExternalCalls {
				sb.WriteString(fmt.Sprintf("- **%s** %s\n", c.Method, c.URL))
			}
		}

		if len(flow.ValidationSteps) > 0 {
			sb.WriteString("\n### Validations\n\n")
			for _, v := range flow.ValidationSteps {
				sb.WriteString(fmt.Sprintf("- %s\n", v))
			}
		}
	}

	// Calls
	if len(foundFunc.Calls) > 0 {
		sb.WriteString("\n### Calls\n\n")
		for _, call := range foundFunc.Calls {
			if call.Package != "" {
				sb.WriteString(fmt.Sprintf("- `%s.%s` (%s)\n", call.Package, call.Function, call.Type))
			} else {
				sb.WriteString(fmt.Sprintf("- `%s` (%s)\n", call.Function, call.Type))
			}
		}
	}

	// Called By
	if len(foundFunc.CalledBy) > 0 {
		sb.WriteString("\n### Called By\n\n")
		for _, caller := range foundFunc.CalledBy {
			sb.WriteString(fmt.Sprintf("- `%s` in `%s`\n", caller.Function, caller.File))
		}
	}

	// Side Effects
	if len(foundFunc.SideEffects) > 0 {
		sb.WriteString(fmt.Sprintf("\n**Side Effects:** %s\n", strings.Join(foundFunc.SideEffects, ", ")))
	}

	// Error Handling
	if foundFunc.ErrorHandling != nil {
		eh := foundFunc.ErrorHandling
		sb.WriteString("\n### Error Handling\n\n")
		sb.WriteString(fmt.Sprintf("- Returns error: %v\n", eh.ReturnsError))
		sb.WriteString(fmt.Sprintf("- Error checks: %d\n", eh.ErrorChecks))
		if eh.WrapsErrors {
			sb.WriteString("- Wraps errors with context\n")
		}
		if eh.LogsErrors {
			sb.WriteString("- Logs errors\n")
		}
		if eh.PanicsOnError {
			sb.WriteString("- **Warning:** Can panic on error\n")
		}
	}

	result.Answer = sb.String()
	result.Sources = []string{foundFile}
	result.Confidence = 0.95
	result.NeedsAI = false

	return result, nil
}

// handleTypeQuery handles type-related queries.
func (m *manager) handleTypeQuery(ctx context.Context, repoCtx *ctxpkg.RepoContext, extracted map[string]string, result *SmartQueryResult) (*SmartQueryResult, error) {
	typeName := extracted["type"]
	if typeName == "" {
		result.NeedsAI = true
		result.Answer = "Could not identify type name in query"
		return result, nil
	}

	// Search for the type
	var foundType *ctxpkg.TypeDef
	var foundFile string

	for path, fileCtx := range repoCtx.Files {
		for i := range fileCtx.Types {
			t := &fileCtx.Types[i]
			if strings.EqualFold(t.Name, typeName) {
				foundType = t
				foundFile = path
				break
			}
		}
		if foundType != nil {
			break
		}
	}

	if foundType == nil {
		result.Answer = fmt.Sprintf("Type `%s` not found in this project", typeName)
		result.Confidence = 0.5
		return result, nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("## Type: `%s`\n\n", foundType.Name))
	sb.WriteString(fmt.Sprintf("**Kind:** %s\n", foundType.Kind))
	sb.WriteString(fmt.Sprintf("**File:** `%s` (line %d)\n\n", foundFile, foundType.LineStart))

	if foundType.Description != "" {
		sb.WriteString(fmt.Sprintf("**Description:** %s\n\n", foundType.Description))
	}

	if len(foundType.Fields) > 0 {
		sb.WriteString("### Fields\n\n")
		sb.WriteString("| Field | Type | Tag |\n")
		sb.WriteString("|-------|------|-----|\n")
		for _, field := range foundType.Fields {
			sb.WriteString(fmt.Sprintf("| `%s` | `%s` | `%s` |\n",
				field.Name, field.Type, field.Tag))
		}
	}

	if len(foundType.Methods) > 0 {
		sb.WriteString("\n### Methods\n\n")
		for _, method := range foundType.Methods {
			sb.WriteString(fmt.Sprintf("- `%s`\n", method))
		}
	}

	result.Answer = sb.String()
	result.Sources = []string{foundFile}
	result.Confidence = 0.95
	result.NeedsAI = false
	result.Types = []TypeRef{{Name: foundType.Name, Kind: foundType.Kind, File: foundFile, Line: foundType.LineStart}}

	return result, nil
}

// handleSideEffectQuery handles side effect queries.
func (m *manager) handleSideEffectQuery(ctx context.Context, repoCtx *ctxpkg.RepoContext, extracted map[string]string, result *SmartQueryResult) (*SmartQueryResult, error) {
	effect := extracted["effect"]
	if effect == "" {
		effect = "db_query" // Default to database
	}

	refs, err := m.SearchBySideEffect(ctx, repoCtx.ID, effect)
	if err != nil {
		return nil, err
	}

	if len(refs) == 0 {
		result.Answer = fmt.Sprintf("No functions found with side effect: %s", effect)
		return result, nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("## Functions with `%s` Side Effect\n\n", effect))
	sb.WriteString(fmt.Sprintf("Found %d functions:\n\n", len(refs)))

	for _, ref := range refs {
		sb.WriteString(fmt.Sprintf("### `%s`\n", ref.Function))
		sb.WriteString(fmt.Sprintf("- **File:** `%s:%d`\n", ref.File, ref.Line))

		// Get full function context for more details
		if fileCtx, ok := repoCtx.Files[ref.File]; ok {
			for _, fn := range fileCtx.Functions {
				if fn.Name == ref.Function {
					sb.WriteString(fmt.Sprintf("- **Signature:** `%s`\n", fn.Signature))
					if fn.Behavior != nil && fn.Behavior.Summary != "" {
						sb.WriteString(fmt.Sprintf("- **Summary:** %s\n", fn.Behavior.Summary))
					}
					if fn.APIFlow != nil && len(fn.APIFlow.DBQueries) > 0 {
						for _, q := range fn.APIFlow.DBQueries {
							if q.RawQuery != "" {
								sb.WriteString(fmt.Sprintf("- **Query:** `%s`\n", q.RawQuery))
							}
						}
					}
					break
				}
			}
		}
		sb.WriteString("\n")
	}

	result.Answer = sb.String()
	result.Functions = refs
	result.Confidence = 0.9
	result.NeedsAI = false

	return result, nil
}

// handleConceptQuery handles concept-based queries.
func (m *manager) handleConceptQuery(ctx context.Context, repoCtx *ctxpkg.RepoContext, extracted map[string]string, result *SmartQueryResult) (*SmartQueryResult, error) {
	concept := extracted["concept"]
	if concept == "" {
		result.NeedsAI = true
		return result, nil
	}

	refs, err := m.SearchByConcept(ctx, repoCtx.ID, concept)
	if err != nil {
		return nil, err
	}

	if len(refs) == 0 {
		result.Answer = fmt.Sprintf("No functions found related to: %s", concept)
		return result, nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("## Functions Related to `%s`\n\n", concept))
	sb.WriteString(fmt.Sprintf("Found %d functions:\n\n", len(refs)))

	for _, ref := range refs {
		sb.WriteString(fmt.Sprintf("- **`%s`** in `%s:%d`\n", ref.Function, ref.File, ref.Line))
	}

	result.Answer = sb.String()
	result.Functions = refs
	result.Confidence = 0.85
	result.NeedsAI = false

	return result, nil
}

// handleCallersQuery handles "who calls X" queries.
func (m *manager) handleCallersQuery(ctx context.Context, repoCtx *ctxpkg.RepoContext, extracted map[string]string, result *SmartQueryResult) (*SmartQueryResult, error) {
	funcName := extracted["function"]
	if funcName == "" {
		result.NeedsAI = true
		return result, nil
	}

	// Find all callers across all files
	var callers []ctxpkg.FunctionRef
	for path, fileCtx := range repoCtx.Files {
		for _, fn := range fileCtx.Functions {
			if strings.EqualFold(fn.Name, funcName) {
				for _, caller := range fn.CalledBy {
					callers = append(callers, ctxpkg.FunctionRef{
						Function: caller.Function,
						File:     caller.File,
						Line:     caller.Line,
					})
				}
			}
		}
		_ = path
	}

	if len(callers) == 0 {
		result.Answer = fmt.Sprintf("No callers found for `%s` (or function not found)", funcName)
		return result, nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("## Callers of `%s`\n\n", funcName))
	sb.WriteString(fmt.Sprintf("Found %d callers:\n\n", len(callers)))

	for _, caller := range callers {
		sb.WriteString(fmt.Sprintf("- **`%s`** in `%s:%d`\n", caller.Function, caller.File, caller.Line))
	}

	result.Answer = sb.String()
	result.Functions = callers
	result.Confidence = 0.95
	result.NeedsAI = false

	return result, nil
}

// handleCallsQuery handles "what does X call" queries.
func (m *manager) handleCallsQuery(ctx context.Context, repoCtx *ctxpkg.RepoContext, extracted map[string]string, result *SmartQueryResult) (*SmartQueryResult, error) {
	funcName := extracted["function"]
	if funcName == "" {
		result.NeedsAI = true
		return result, nil
	}

	// Find the function and its calls
	var calls []ctxpkg.CallRef
	var foundFile string

	for path, fileCtx := range repoCtx.Files {
		for _, fn := range fileCtx.Functions {
			if strings.EqualFold(fn.Name, funcName) {
				calls = fn.Calls
				foundFile = path
				break
			}
		}
		if len(calls) > 0 {
			break
		}
	}

	if len(calls) == 0 {
		result.Answer = fmt.Sprintf("No calls found from `%s` (or function not found)", funcName)
		return result, nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("## Functions Called by `%s`\n\n", funcName))
	sb.WriteString(fmt.Sprintf("**File:** `%s`\n\n", foundFile))

	// Group by type
	internal := []ctxpkg.CallRef{}
	stdlib := []ctxpkg.CallRef{}
	external := []ctxpkg.CallRef{}

	for _, call := range calls {
		switch call.Type {
		case "internal":
			internal = append(internal, call)
		case "stdlib":
			stdlib = append(stdlib, call)
		default:
			external = append(external, call)
		}
	}

	if len(internal) > 0 {
		sb.WriteString("### Internal Calls\n")
		for _, c := range internal {
			sb.WriteString(fmt.Sprintf("- `%s` (line %d)\n", c.Function, c.Line))
		}
		sb.WriteString("\n")
	}

	if len(stdlib) > 0 {
		sb.WriteString("### Standard Library Calls\n")
		for _, c := range stdlib {
			sb.WriteString(fmt.Sprintf("- `%s.%s`\n", c.Package, c.Function))
		}
		sb.WriteString("\n")
	}

	if len(external) > 0 {
		sb.WriteString("### External Package Calls\n")
		for _, c := range external {
			sb.WriteString(fmt.Sprintf("- `%s.%s`\n", c.Package, c.Function))
		}
	}

	result.Answer = sb.String()
	result.Confidence = 0.95
	result.NeedsAI = false

	return result, nil
}

// handleFlowQuery handles execution flow queries.
func (m *manager) handleFlowQuery(ctx context.Context, repoCtx *ctxpkg.RepoContext, extracted map[string]string, result *SmartQueryResult) (*SmartQueryResult, error) {
	funcName := extracted["function"]

	// If no specific function, show handlers with their flows
	if funcName == "" {
		return m.handleArchitectureQuery(ctx, repoCtx, result)
	}

	// Find the function
	var foundFunc *ctxpkg.FunctionDef
	var foundFile string

	for path, fileCtx := range repoCtx.Files {
		for i := range fileCtx.Functions {
			fn := &fileCtx.Functions[i]
			if strings.EqualFold(fn.Name, funcName) {
				foundFunc = fn
				foundFile = path
				break
			}
		}
		if foundFunc != nil {
			break
		}
	}

	if foundFunc == nil {
		result.Answer = fmt.Sprintf("Function `%s` not found", funcName)
		return result, nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("## Execution Flow: `%s`\n\n", foundFunc.Name))
	sb.WriteString(fmt.Sprintf("**File:** `%s`\n\n", foundFile))

	// Build flow trace
	if foundFunc.APIFlow != nil && len(foundFunc.APIFlow.Steps) > 0 {
		sb.WriteString("### Step-by-Step Flow\n\n")
		for _, step := range foundFunc.APIFlow.Steps {
			sb.WriteString(fmt.Sprintf("%d. **%s** (line %d)\n", step.StepNumber, step.Action, step.Line))
			if step.Output != "" {
				sb.WriteString(fmt.Sprintf("   → Returns: `%s`\n", step.Output))
			}
		}
		sb.WriteString("\n")
	}

	// Show what gets called
	if len(foundFunc.Calls) > 0 {
		sb.WriteString("### Functions Called\n\n")
		for _, call := range foundFunc.Calls {
			if call.Type == "internal" {
				sb.WriteString(fmt.Sprintf("- `%s` → ", call.Function))
				// Try to get that function's summary
				for _, fileCtx := range repoCtx.Files {
					for _, fn := range fileCtx.Functions {
						if fn.Name == call.Function && fn.Behavior != nil {
							sb.WriteString(fn.Behavior.Summary)
							break
						}
					}
				}
				sb.WriteString("\n")
			}
		}
	}

	result.Answer = sb.String()
	result.Confidence = 0.9
	result.NeedsAI = false

	return result, nil
}

// handleFileQuery handles file-level queries.
func (m *manager) handleFileQuery(ctx context.Context, repoCtx *ctxpkg.RepoContext, extracted map[string]string, result *SmartQueryResult) (*SmartQueryResult, error) {
	fileName := extracted["file"]
	if fileName == "" {
		result.NeedsAI = true
		return result, nil
	}

	// Find the file
	var fileCtx *ctxpkg.FileContext
	var filePath string

	for path, fc := range repoCtx.Files {
		if matchesFileName(path, fileName) {
			fileCtx = fc
			filePath = path
			break
		}
	}

	if fileCtx == nil {
		result.Answer = fmt.Sprintf("File `%s` not found", fileName)
		return result, nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("## File: `%s`\n\n", filePath))
	sb.WriteString(fmt.Sprintf("**Package:** %s\n", fileCtx.Package))
	sb.WriteString(fmt.Sprintf("**Language:** %s\n\n", fileCtx.Language))

	if len(fileCtx.Imports) > 0 {
		sb.WriteString("### Imports\n\n")
		for _, imp := range fileCtx.Imports {
			if imp.Alias != "" {
				sb.WriteString(fmt.Sprintf("- `%s` as `%s`\n", imp.Path, imp.Alias))
			} else {
				sb.WriteString(fmt.Sprintf("- `%s`\n", imp.Path))
			}
		}
		sb.WriteString("\n")
	}

	if len(fileCtx.Types) > 0 {
		sb.WriteString("### Types\n\n")
		for _, t := range fileCtx.Types {
			sb.WriteString(fmt.Sprintf("- **`%s`** (%s) - line %d\n", t.Name, t.Kind, t.LineStart))
		}
		sb.WriteString("\n")
	}

	if len(fileCtx.Functions) > 0 {
		sb.WriteString("### Functions\n\n")
		for _, fn := range fileCtx.Functions {
			sb.WriteString(fmt.Sprintf("#### `%s`\n", fn.Name))
			sb.WriteString(fmt.Sprintf("- **Signature:** `%s`\n", fn.Signature))
			if fn.Behavior != nil && fn.Behavior.Summary != "" {
				sb.WriteString(fmt.Sprintf("- **Summary:** %s\n", fn.Behavior.Summary))
			}
			if len(fn.SideEffects) > 0 {
				sb.WriteString(fmt.Sprintf("- **Side Effects:** %s\n", strings.Join(fn.SideEffects, ", ")))
			}
			sb.WriteString("\n")
		}
	}

	result.Answer = sb.String()
	result.Sources = []string{filePath}
	result.Confidence = 0.95
	result.NeedsAI = false

	return result, nil
}

// classifyFilePurpose returns the purpose category for a file based on its name/extension.
func classifyFilePurpose(filename string) string {
	base := filename
	if idx := strings.LastIndex(filename, "/"); idx >= 0 {
		base = filename[idx+1:]
	}

	switch {
	case strings.HasSuffix(base, "_test.go"):
		return "Tests"
	case strings.HasSuffix(base, ".go"):
		return "Source Code"
	case strings.HasSuffix(base, ".md"):
		return "Documentation"
	case strings.HasSuffix(base, ".mod") || strings.HasSuffix(base, ".sum") ||
		strings.HasSuffix(base, ".yml") || strings.HasSuffix(base, ".yaml") ||
		strings.HasSuffix(base, ".json") || strings.HasSuffix(base, ".toml") ||
		base == "Makefile" || base == "Dockerfile":
		return "Configuration"
	default:
		return "Other"
	}
}

// handlePackageQuery handles package/directory structure queries.
// Shows all files in a package with their purposes, types, and key functions.
func (m *manager) handlePackageQuery(ctx context.Context, repoCtx *ctxpkg.RepoContext, extracted map[string]string, result *SmartQueryResult) (*SmartQueryResult, error) {
	packagePath := extracted["package"]
	if packagePath == "" {
		result.NeedsAI = true
		result.Answer = "Could not identify package/directory path in query"
		return result, nil
	}

	// Normalize the path (remove leading/trailing slashes)
	packagePath = strings.Trim(packagePath, "/")

	// Find all files that match this package path
	type fileInfo struct {
		path    string
		context *ctxpkg.FileContext
	}
	var matchingFiles []fileInfo

	for path, fileCtx := range repoCtx.Files {
		if matchesPackagePath(path, packagePath) {
			matchingFiles = append(matchingFiles, fileInfo{path: path, context: fileCtx})
		}
	}

	if len(matchingFiles) == 0 {
		result.Answer = fmt.Sprintf("No files found matching package/directory: `%s`\n\nTry using a more specific path or check available paths with `get_context` tool.", packagePath)
		result.Confidence = 0.5
		return result, nil
	}

	// Sort files by path for consistent output
	sort.Slice(matchingFiles, func(i, j int) bool {
		return matchingFiles[i].path < matchingFiles[j].path
	})

	var sb strings.Builder
	fmt.Fprintf(&sb, "## Package Structure: `%s`\n\n", packagePath)
	fmt.Fprintf(&sb, "**Files found:** %d\n\n", len(matchingFiles))

	// Determine if files span multiple directories
	dirSet := make(map[string]bool)
	for _, f := range matchingFiles {
		relPath := f.path
		if idx := strings.Index(relPath, packagePath); idx >= 0 {
			relPath = relPath[idx+len(packagePath):]
			relPath = strings.TrimPrefix(relPath, "/")
		}
		dir := ""
		if slashIdx := strings.Index(relPath, "/"); slashIdx >= 0 {
			dir = relPath[:slashIdx]
		}
		dirSet[dir] = true
	}

	isFlat := len(dirSet) <= 1

	if isFlat {
		// Flat package: group by purpose only
		purposeFiles := make(map[string][]fileInfo)
		for _, f := range matchingFiles {
			purpose := classifyFilePurpose(f.path)
			purposeFiles[purpose] = append(purposeFiles[purpose], f)
		}

		// Count non-empty purpose categories
		nonEmpty := 0
		for _, files := range purposeFiles {
			if len(files) > 0 {
				nonEmpty++
			}
		}

		purposeOrder := []string{"Source Code", "Tests", "Documentation", "Configuration", "Other"}
		for _, purpose := range purposeOrder {
			files := purposeFiles[purpose]
			if len(files) == 0 {
				continue
			}
			// Show purpose header if multiple categories
			if nonEmpty > 1 {
				fmt.Fprintf(&sb, "### %s\n\n", purpose)
			}
			for _, f := range files {
				m.writeFileDetail(&sb, f.path, f.context)
			}
		}
	} else {
		// Multi-directory: group by directory (2-level collapse), then purpose
		type dirGroup struct {
			dir   string
			files []fileInfo
		}
		dirFilesMap := make(map[string][]fileInfo)
		for _, f := range matchingFiles {
			relPath := f.path
			if idx := strings.Index(relPath, packagePath); idx >= 0 {
				relPath = relPath[idx+len(packagePath):]
				relPath = strings.TrimPrefix(relPath, "/")
			}
			// Collapse to at most 2 directory levels
			parts := strings.Split(relPath, "/")
			var dir string
			if len(parts) <= 1 {
				dir = "" // file directly in package root
			} else if len(parts) <= 3 {
				dir = strings.Join(parts[:len(parts)-1], "/")
			} else {
				dir = parts[0] + "/" + parts[1]
			}
			dirFilesMap[dir] = append(dirFilesMap[dir], f)
		}

		// Sort directory keys
		var dirs []string
		for dir := range dirFilesMap {
			dirs = append(dirs, dir)
		}
		sort.Strings(dirs)

		for _, dir := range dirs {
			files := dirFilesMap[dir]
			if dir != "" {
				fmt.Fprintf(&sb, "### `%s/`\n\n", dir)
			}

			// Group by purpose within directory
			purposeFiles := make(map[string][]fileInfo)
			for _, f := range files {
				purpose := classifyFilePurpose(f.path)
				purposeFiles[purpose] = append(purposeFiles[purpose], f)
			}

			purposeOrder := []string{"Source Code", "Tests", "Documentation", "Configuration", "Other"}
			nonEmpty := 0
			for _, pf := range purposeFiles {
				if len(pf) > 0 {
					nonEmpty++
				}
			}

			for _, purpose := range purposeOrder {
				pFiles := purposeFiles[purpose]
				if len(pFiles) == 0 {
					continue
				}
				if nonEmpty > 1 {
					fmt.Fprintf(&sb, "**%s:**\n\n", purpose)
				}
				for _, f := range pFiles {
					m.writeFileDetail(&sb, f.path, f.context)
				}
			}
		}
	}

	// Add navigation hints
	sb.WriteString("## Quick Reference\n\n")
	sb.WriteString("Use these commands for more details:\n")
	sb.WriteString("- `get_function_context` - Deep dive into a specific function\n")
	sb.WriteString("- `search_by_concept` - Find code by pattern (auth, validation, handler)\n")
	sb.WriteString("- `get_callers` - See who calls a function\n")

	result.Answer = sb.String()
	result.Confidence = 0.95
	result.NeedsAI = false

	// Collect source files
	for _, f := range matchingFiles {
		result.Sources = append(result.Sources, f.path)
	}

	return result, nil
}

// writeFileDetail writes the detail block for a single file.
func (m *manager) writeFileDetail(sb *strings.Builder, filePath string, fc *ctxpkg.FileContext) {
	fileName := filePath
	if lastSlash := strings.LastIndex(fileName, "/"); lastSlash >= 0 {
		fileName = fileName[lastSlash+1:]
	}
	fmt.Fprintf(sb, "#### `%s`\n\n", fileName)
	fmt.Fprintf(sb, "**Path:** `%s`\n", filePath)

	if fc.Purpose != "" {
		fmt.Fprintf(sb, "**Purpose:** %s\n", fc.Purpose)
	}

	fmt.Fprintf(sb, "**Lines:** %d\n", fc.LineCount)

	// Types summary
	if len(fc.Types) > 0 {
		sb.WriteString("\n**Types:**\n")
		for _, t := range fc.Types {
			if t.IsPublic {
				desc := ""
				if t.Description != "" {
					desc = " - " + truncateString(t.Description, 60)
				}
				fmt.Fprintf(sb, "- `%s` (%s)%s\n", t.Name, t.Kind, desc)
			}
		}
	}

	// Functions summary (show key functions)
	publicFuncs := []ctxpkg.FunctionDef{}
	for _, fn := range fc.Functions {
		if fn.IsPublic {
			publicFuncs = append(publicFuncs, fn)
		}
	}

	if len(publicFuncs) > 0 {
		sb.WriteString("\n**Functions:**\n")
		maxFuncs := 10
		if len(publicFuncs) < maxFuncs {
			maxFuncs = len(publicFuncs)
		}
		for i := 0; i < maxFuncs; i++ {
			fn := publicFuncs[i]
			summary := ""
			if fn.Behavior != nil && fn.Behavior.Summary != "" {
				summary = " - " + truncateString(fn.Behavior.Summary, 50)
			}
			fmt.Fprintf(sb, "- `%s`%s\n", fn.Name, summary)
		}
		if len(publicFuncs) > maxFuncs {
			fmt.Fprintf(sb, "- ... and %d more\n", len(publicFuncs)-maxFuncs)
		}
	}

	// Side effects summary
	sideEffects := make(map[string]bool)
	for _, fn := range fc.Functions {
		for _, se := range fn.SideEffects {
			sideEffects[se] = true
		}
	}
	if len(sideEffects) > 0 {
		effects := make([]string, 0, len(sideEffects))
		for se := range sideEffects {
			effects = append(effects, se)
		}
		sort.Strings(effects)
		fmt.Fprintf(sb, "\n**Side Effects:** %s\n", strings.Join(effects, ", "))
	}

	sb.WriteString("\n---\n\n")
}

// truncateString truncates a string to maxLen characters with ellipsis.
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// handleArchitectureQuery handles project structure queries.
func (m *manager) handleArchitectureQuery(ctx context.Context, repoCtx *ctxpkg.RepoContext, result *SmartQueryResult) (*SmartQueryResult, error) {
	var sb strings.Builder
	sb.WriteString("## Project Architecture\n\n")

	// Count by directory
	dirCounts := make(map[string]int)
	for path := range repoCtx.Files {
		dir := strings.Split(path, "/")[0]
		dirCounts[dir]++
	}

	sb.WriteString("### Directory Structure\n\n")
	for dir, count := range dirCounts {
		sb.WriteString(fmt.Sprintf("- `%s/` - %d files\n", dir, count))
	}
	sb.WriteString("\n")

	// Find handlers
	handlers := []string{}
	for _, fileCtx := range repoCtx.Files {
		for _, fn := range fileCtx.Functions {
			if fn.APIFlow != nil && fn.APIFlow.IsHTTPHandler {
				handlers = append(handlers, fn.Name)
			}
		}
	}

	if len(handlers) > 0 {
		sb.WriteString("### HTTP Handlers\n\n")
		for _, h := range handlers {
			sb.WriteString(fmt.Sprintf("- `%s`\n", h))
		}
		sb.WriteString("\n")
	}

	// Find models (structs with db tags or named *Model)
	models := []string{}
	for _, fileCtx := range repoCtx.Files {
		for _, t := range fileCtx.Types {
			nameLower := strings.ToLower(t.Name)
			if strings.HasSuffix(nameLower, "model") ||
				strings.HasSuffix(nameLower, "entity") ||
				t.Kind == "struct" {
				models = append(models, t.Name)
			}
		}
	}

	if len(models) > 0 && len(models) < 20 {
		sb.WriteString("### Data Models\n\n")
		for _, m := range models {
			sb.WriteString(fmt.Sprintf("- `%s`\n", m))
		}
	}

	result.Answer = sb.String()
	result.Confidence = 0.8
	result.NeedsAI = false

	return result, nil
}

// handleGeneralQuery handles queries that don't fit specific patterns.
func (m *manager) handleGeneralQuery(ctx context.Context, repoCtx *ctxpkg.RepoContext, query string, result *SmartQueryResult) (*SmartQueryResult, error) {
	// Try to find relevant information
	queryLower := strings.ToLower(query)

	// Search for relevant functions
	var relevantFuncs []ctxpkg.FunctionRef

	for path, fileCtx := range repoCtx.Files {
		for _, fn := range fileCtx.Functions {
			// Check if function name or behavior matches query words
			if containsAnyWord(strings.ToLower(fn.Name), queryLower) ||
				(fn.Behavior != nil && containsAnyWord(strings.ToLower(fn.Behavior.Summary), queryLower)) {
				relevantFuncs = append(relevantFuncs, ctxpkg.FunctionRef{
					Function: fn.Name,
					File:     path,
					Line:     fn.LineStart,
				})
			}
		}
	}

	if len(relevantFuncs) > 0 {
		var sb strings.Builder
		sb.WriteString("## Potentially Relevant Functions\n\n")
		for _, ref := range relevantFuncs {
			sb.WriteString(fmt.Sprintf("- `%s` in `%s:%d`\n", ref.Function, ref.File, ref.Line))
		}
		sb.WriteString("\n*Note: For a more detailed answer, try asking about a specific function.*")

		result.Answer = sb.String()
		result.Functions = relevantFuncs
		result.NeedsAI = true // Suggest AI for deeper analysis
		result.SuggestedQuery = "Use 'ask' tool for complex questions"
		result.Confidence = 0.5
	} else {
		result.Answer = "Could not find relevant information without AI. Try the 'ask' tool for complex questions."
		result.NeedsAI = true
		result.Confidence = 0.3
	}

	return result, nil
}

// containsAnyWord checks if text contains any word from the query.
func containsAnyWord(text, query string) bool {
	words := strings.Fields(query)
	for _, word := range words {
		if len(word) > 3 && strings.Contains(text, word) {
			return true
		}
	}
	return false
}

// matchesPackagePath checks if filePath contains packagePath at a "/" boundary.
// "http" matches "net/http/server.go" but NOT "net/https/client.go".
func matchesPackagePath(filePath, packagePath string) bool {
	idx := strings.Index(filePath, packagePath)
	if idx < 0 {
		return false
	}
	// Check left boundary: must be at start or preceded by "/"
	if idx > 0 && filePath[idx-1] != '/' {
		return false
	}
	// Check right boundary: must be at end or followed by "/"
	endIdx := idx + len(packagePath)
	if endIdx < len(filePath) && filePath[endIdx] != '/' {
		return false
	}
	return true
}

// matchesFileName checks if filePath ends with exactly the given fileName.
// "router.go" matches "gorilla/mux/router.go" but NOT "gorilla/mux/subrouter.go".
func matchesFileName(filePath, fileName string) bool {
	if strings.HasSuffix(filePath, "/"+fileName) || filePath == fileName {
		return true
	}
	return false
}
