package analyzer

import (
	"sort"
	"strings"

	ctxpkg "github.com/yashpalc/mcp-repo-context/internal/context"
)

// SearchIndexBuilder builds a search index from analyzed files.
type SearchIndexBuilder struct {
	index *ctxpkg.SearchIndex
}

// NewSearchIndexBuilder creates a new search index builder.
func NewSearchIndexBuilder() *SearchIndexBuilder {
	return &SearchIndexBuilder{
		index: &ctxpkg.SearchIndex{
			FunctionNames: make(map[string][]ctxpkg.FunctionRef),
			Concepts:      make(map[string][]ctxpkg.FunctionRef),
			SideEffects:   make(map[string][]ctxpkg.FunctionRef),
			TypeUsage:     make(map[string][]ctxpkg.FunctionRef),
			ErrorTypes:    make(map[string][]ctxpkg.FunctionRef),
			Routes:        make(map[string][]ctxpkg.RouteRef),
			Packages:      make(map[string][]string),
		},
	}
}

// BuildFromFiles builds a search index from file contexts.
func (b *SearchIndexBuilder) BuildFromFiles(files map[string]*ctxpkg.FileContext) *ctxpkg.SearchIndex {
	for path, fileCtx := range files {
		b.indexFile(path, fileCtx)
	}
	return b.index
}

func (b *SearchIndexBuilder) indexFile(path string, fileCtx *ctxpkg.FileContext) {
	// Index package
	pkg := extractPackageFromPath(path)
	if pkg != "" {
		b.index.Packages[pkg] = append(b.index.Packages[pkg], path)
	}

	// Index functions
	for _, fn := range fileCtx.Functions {
		b.indexFunction(path, fn)
	}

	// Index types
	for _, t := range fileCtx.Types {
		b.indexType(path, t)
	}

	// Index routes
	for _, route := range fileCtx.Routes {
		b.indexRoute(path, route)
	}

	// Index concepts from file
	for _, concept := range fileCtx.Concepts {
		ref := ctxpkg.FunctionRef{
			File:    path,
			Summary: fileCtx.Purpose,
		}
		b.addToConcepts(concept, ref)
	}
}

func (b *SearchIndexBuilder) indexFunction(path string, fn ctxpkg.FunctionDef) {
	ref := ctxpkg.FunctionRef{
		File:      path,
		Function:  fn.Name,
		Line:      fn.LineStart,
		Signature: fn.Signature,
	}

	// Add behavior summary if available
	if fn.Behavior != nil && fn.Behavior.Summary != "" {
		ref.Summary = fn.Behavior.Summary
	}

	// Index by function name (full and fragments)
	b.indexFunctionName(fn.Name, ref)

	// Index by patterns detected
	if fn.Behavior != nil {
		for _, pattern := range fn.Behavior.Patterns {
			b.addToConcepts(pattern, ref)
		}
	}

	// Index by side effects
	for _, effect := range fn.SideEffects {
		b.addToSideEffects(effect, ref)
	}

	// Index by error types
	if fn.ErrorHandling != nil {
		for _, errType := range fn.ErrorHandling.ErrorTypes {
			b.addToErrorTypes(errType, ref)
		}
	}

	// Index by used imports (types/packages)
	for _, imp := range fn.UsesImports {
		b.addToTypeUsage(imp, ref)
	}

	// Index by parameter types
	for _, param := range fn.Parameters {
		b.addToTypeUsage(param.Type, ref)
	}

	// Index by return types
	for _, ret := range fn.Returns {
		b.addToTypeUsage(ret, ref)
	}

	// Extract and index concepts from function
	concepts := extractConceptsFromFunction(fn)
	for _, concept := range concepts {
		b.addToConcepts(concept, ref)
	}
}

func (b *SearchIndexBuilder) indexFunctionName(name string, ref ctxpkg.FunctionRef) {
	// Index full name (lowercase)
	nameLower := strings.ToLower(name)
	b.index.FunctionNames[nameLower] = append(b.index.FunctionNames[nameLower], ref)

	// Index name fragments (split CamelCase)
	fragments := splitCamelCaseToLower(name)
	for _, frag := range fragments {
		if len(frag) >= 3 { // Only index fragments with 3+ chars
			b.index.FunctionNames[frag] = append(b.index.FunctionNames[frag], ref)
		}
	}

	// Index common prefixes
	prefixes := []string{"get", "set", "create", "delete", "update", "find", "handle", "process", "validate", "parse", "build", "new", "is", "has", "can"}
	for _, prefix := range prefixes {
		if strings.HasPrefix(nameLower, prefix) {
			b.addToConcepts(prefix+"_operation", ref)
		}
	}
}

func (b *SearchIndexBuilder) indexType(path string, t ctxpkg.TypeDef) {
	ref := ctxpkg.FunctionRef{
		File:     path,
		Function: t.Name, // Use Function field for type name
		Line:     t.LineStart,
	}

	// Index by type name
	nameLower := strings.ToLower(t.Name)
	b.addToTypeUsage(nameLower, ref)

	// Index by kind
	b.addToConcepts(t.Kind, ref)

	// Index field types for structs
	for _, field := range t.Fields {
		b.addToTypeUsage(strings.ToLower(field.Type), ref)
	}

	// Extract concepts from type name
	fragments := splitCamelCaseToLower(t.Name)
	for _, frag := range fragments {
		if len(frag) >= 3 {
			b.addToConcepts(frag, ref)
		}
	}
}

func (b *SearchIndexBuilder) indexRoute(path string, route ctxpkg.Route) {
	ref := ctxpkg.RouteRef{
		File:    path,
		Handler: route.Handler,
		Method:  route.Method,
		Path:    route.Path,
		Line:    route.Line,
	}

	// Index by method
	method := strings.ToLower(route.Method)
	b.index.Routes[method] = append(b.index.Routes[method], ref)

	// Index by path segments
	segments := strings.Split(route.Path, "/")
	for _, seg := range segments {
		seg = strings.TrimPrefix(seg, ":")
		seg = strings.TrimPrefix(seg, "{")
		seg = strings.TrimSuffix(seg, "}")
		if len(seg) >= 2 {
			b.index.Routes[strings.ToLower(seg)] = append(b.index.Routes[strings.ToLower(seg)], ref)
		}
	}

	// Index as "route" concept
	funcRef := ctxpkg.FunctionRef{
		File:     path,
		Function: route.Handler,
		Line:     route.Line,
		Summary:  route.Method + " " + route.Path,
	}
	b.addToConcepts("route", funcRef)
	b.addToConcepts("http_endpoint", funcRef)
}

func (b *SearchIndexBuilder) addToConcepts(concept string, ref ctxpkg.FunctionRef) {
	concept = strings.ToLower(concept)
	b.index.Concepts[concept] = append(b.index.Concepts[concept], ref)
}

func (b *SearchIndexBuilder) addToSideEffects(effect string, ref ctxpkg.FunctionRef) {
	effect = strings.ToLower(effect)
	b.index.SideEffects[effect] = append(b.index.SideEffects[effect], ref)
}

func (b *SearchIndexBuilder) addToTypeUsage(typeName string, ref ctxpkg.FunctionRef) {
	typeName = strings.ToLower(typeName)
	// Clean up pointer and slice prefixes
	typeName = strings.TrimPrefix(typeName, "*")
	typeName = strings.TrimPrefix(typeName, "[]")
	typeName = strings.TrimPrefix(typeName, "map[")
	if typeName != "" {
		b.index.TypeUsage[typeName] = append(b.index.TypeUsage[typeName], ref)
	}
}

func (b *SearchIndexBuilder) addToErrorTypes(errType string, ref ctxpkg.FunctionRef) {
	errType = strings.ToLower(errType)
	b.index.ErrorTypes[errType] = append(b.index.ErrorTypes[errType], ref)
}

// extractConceptsFromFunction extracts searchable concepts from a function.
func extractConceptsFromFunction(fn ctxpkg.FunctionDef) []string {
	concepts := make(map[string]bool)

	// From behavior steps
	if fn.Behavior != nil {
		for _, step := range fn.Behavior.Steps {
			stepLower := strings.ToLower(step)

			// HTTP operations
			if strings.Contains(stepLower, "http") {
				concepts["http"] = true
			}
			if strings.Contains(stepLower, "request") {
				concepts["request"] = true
			}
			if strings.Contains(stepLower, "response") {
				concepts["response"] = true
			}

			// Database operations
			if strings.Contains(stepLower, "database") || strings.Contains(stepLower, "query") || strings.Contains(stepLower, "sql") {
				concepts["database"] = true
			}

			// File operations
			if strings.Contains(stepLower, "file") || strings.Contains(stepLower, "read") || strings.Contains(stepLower, "write") {
				concepts["file_io"] = true
			}

			// JSON operations
			if strings.Contains(stepLower, "json") || strings.Contains(stepLower, "encode") || strings.Contains(stepLower, "decode") {
				concepts["json"] = true
				concepts["serialization"] = true
			}

			// Validation
			if strings.Contains(stepLower, "validate") || strings.Contains(stepLower, "check") {
				concepts["validation"] = true
			}

			// Error handling
			if strings.Contains(stepLower, "error") {
				concepts["error_handling"] = true
			}

			// Authentication/Authorization
			if strings.Contains(stepLower, "auth") || strings.Contains(stepLower, "token") || strings.Contains(stepLower, "permission") {
				concepts["authentication"] = true
			}

			// Logging
			if strings.Contains(stepLower, "log") {
				concepts["logging"] = true
			}

			// Caching
			if strings.Contains(stepLower, "cache") || strings.Contains(stepLower, "redis") {
				concepts["caching"] = true
			}

			// Async operations
			if strings.Contains(stepLower, "goroutine") || strings.Contains(stepLower, "async") || strings.Contains(stepLower, "concurrent") {
				concepts["async"] = true
				concepts["concurrency"] = true
			}
		}
	}

	// From function name
	nameLower := strings.ToLower(fn.Name)

	// Common operation patterns
	operationPatterns := map[string][]string{
		"authentication": {"login", "logout", "auth", "authenticate", "token"},
		"authorization":  {"authorize", "permission", "role", "access"},
		"validation":     {"validate", "verify", "check", "is", "has", "can"},
		"crud":           {"create", "read", "update", "delete", "get", "set", "add", "remove"},
		"handler":        {"handle", "handler", "serve", "process"},
		"middleware":     {"middleware", "interceptor", "filter"},
		"transformation": {"convert", "transform", "parse", "format", "encode", "decode"},
		"initialization": {"init", "setup", "configure", "new", "start"},
		"cleanup":        {"close", "cleanup", "shutdown", "dispose", "teardown"},
		"notification":   {"notify", "send", "emit", "publish", "broadcast"},
		"subscription":   {"subscribe", "listen", "watch", "observe"},
	}

	for concept, keywords := range operationPatterns {
		for _, keyword := range keywords {
			if strings.Contains(nameLower, keyword) {
				concepts[concept] = true
				break
			}
		}
	}

	result := make([]string, 0, len(concepts))
	for concept := range concepts {
		result = append(result, concept)
	}
	return result
}

func extractPackageFromPath(path string) string {
	// Extract package name from file path
	parts := strings.Split(path, "/")
	if len(parts) > 1 {
		// Return the directory name (package)
		return parts[len(parts)-2]
	}
	return ""
}

func splitCamelCaseToLower(s string) []string {
	var words []string
	var word strings.Builder

	for i, r := range s {
		if i > 0 && r >= 'A' && r <= 'Z' {
			if word.Len() > 0 {
				words = append(words, strings.ToLower(word.String()))
				word.Reset()
			}
		}
		word.WriteRune(r)
	}
	if word.Len() > 0 {
		words = append(words, strings.ToLower(word.String()))
	}

	return words
}

// SearchIndex provides methods to search the index.

// SearchFunctions searches for functions by name or concept.
func SearchFunctions(index *ctxpkg.SearchIndex, query string) []ctxpkg.FunctionRef {
	query = strings.ToLower(query)
	results := make(map[string]ctxpkg.FunctionRef) // Deduplicate

	// Search function names
	if refs, ok := index.FunctionNames[query]; ok {
		for _, ref := range refs {
			key := ref.File + ":" + ref.Function
			results[key] = ref
		}
	}

	// Search concepts
	if refs, ok := index.Concepts[query]; ok {
		for _, ref := range refs {
			key := ref.File + ":" + ref.Function
			results[key] = ref
		}
	}

	// Search partial matches
	for name, refs := range index.FunctionNames {
		if strings.Contains(name, query) {
			for _, ref := range refs {
				key := ref.File + ":" + ref.Function
				results[key] = ref
			}
		}
	}

	// Convert to slice
	resultSlice := make([]ctxpkg.FunctionRef, 0, len(results))
	for _, ref := range results {
		resultSlice = append(resultSlice, ref)
	}

	// Sort by relevance (exact matches first)
	sort.Slice(resultSlice, func(i, j int) bool {
		iExact := strings.ToLower(resultSlice[i].Function) == query
		jExact := strings.ToLower(resultSlice[j].Function) == query
		if iExact != jExact {
			return iExact
		}
		return resultSlice[i].Function < resultSlice[j].Function
	})

	return resultSlice
}

// SearchBySideEffect finds functions with specific side effects.
func SearchBySideEffect(index *ctxpkg.SearchIndex, effect string) []ctxpkg.FunctionRef {
	effect = strings.ToLower(effect)
	if refs, ok := index.SideEffects[effect]; ok {
		return refs
	}
	return nil
}

// SearchByType finds functions that use a specific type.
func SearchByType(index *ctxpkg.SearchIndex, typeName string) []ctxpkg.FunctionRef {
	typeName = strings.ToLower(typeName)
	if refs, ok := index.TypeUsage[typeName]; ok {
		return refs
	}
	return nil
}

// SearchByConcept finds functions related to a concept.
func SearchByConcept(index *ctxpkg.SearchIndex, concept string) []ctxpkg.FunctionRef {
	concept = strings.ToLower(concept)
	if refs, ok := index.Concepts[concept]; ok {
		return refs
	}
	return nil
}

// SearchRoutes finds routes by method, path, or handler.
func SearchRoutes(index *ctxpkg.SearchIndex, query string) []ctxpkg.RouteRef {
	query = strings.ToLower(query)
	results := make(map[string]ctxpkg.RouteRef)

	for key, refs := range index.Routes {
		if strings.Contains(key, query) {
			for _, ref := range refs {
				refKey := ref.File + ":" + ref.Handler
				results[refKey] = ref
			}
		}
	}

	resultSlice := make([]ctxpkg.RouteRef, 0, len(results))
	for _, ref := range results {
		resultSlice = append(resultSlice, ref)
	}
	return resultSlice
}

// GetFunctionContext returns comprehensive context for a specific function.
func GetFunctionContext(files map[string]*ctxpkg.FileContext, filePath, funcName string) *FunctionContext {
	fileCtx, ok := files[filePath]
	if !ok {
		return nil
	}

	for _, fn := range fileCtx.Functions {
		if fn.Name == funcName {
			return &FunctionContext{
				Function:   fn,
				File:       fileCtx,
				FilePath:   filePath,
				Callers:    findCallers(files, funcName),
				Callees:    fn.Calls,
			}
		}
	}
	return nil
}

// FunctionContext provides comprehensive context about a function.
type FunctionContext struct {
	Function   ctxpkg.FunctionDef
	File       *ctxpkg.FileContext
	FilePath   string
	Callers    []ctxpkg.CallRef
	Callees    []ctxpkg.CallRef
}

func findCallers(files map[string]*ctxpkg.FileContext, funcName string) []ctxpkg.CallRef {
	var callers []ctxpkg.CallRef
	for path, fileCtx := range files {
		for _, fn := range fileCtx.Functions {
			for _, call := range fn.Calls {
				if call.Function == funcName {
					callers = append(callers, ctxpkg.CallRef{
						Function: fn.Name,
						File:     path,
						Line:     call.Line,
						Type:     "internal",
					})
				}
			}
		}
	}
	return callers
}
