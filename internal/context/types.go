package context

import (
	"encoding/json"
	"fmt"
	"time"
)

// RepoContext holds complete repository analysis.
type RepoContext struct {
	ID           string                  `json:"id"`
	URL          string                  `json:"url"`
	Branch       string                  `json:"branch"`
	CommitHash   string                  `json:"commit_hash"`
	AnalyzedAt   time.Time               `json:"analyzed_at"`
	Files        map[string]*FileContext `json:"files"`
	Architecture *ArchitectureContext    `json:"architecture"`
	Statistics   RepoStatistics          `json:"statistics"`
	Version      int                     `json:"version"`
	AISummary    *AISummary              `json:"ai_summary,omitempty"`

	// Deep context fields
	CallGraph   *CallGraph   `json:"call_graph,omitempty"`
	SearchIndex *SearchIndex `json:"search_index,omitempty"`

	// Dependency graph fields
	ModuleInfo    *ModuleInfo    `json:"module_info,omitempty"`
	ImportSummary *ImportSummary `json:"import_summary,omitempty"`
	ConfigFiles   []ConfigFile   `json:"config_files,omitempty"`
}

// CallGraph represents function call relationships across the repository.
type CallGraph struct {
	Nodes map[string]*CallGraphNode `json:"nodes"` // func_id (file:func) -> node
	Edges []CallGraphEdge           `json:"edges"`
}

// CallGraphNode represents a function in the call graph.
type CallGraphNode struct {
	ID        string   `json:"id"`        // Unique ID: "file:function"
	File      string   `json:"file"`      // File path
	Function  string   `json:"function"`  // Function name
	Signature string   `json:"signature"` // Function signature
	Package   string   `json:"package"`   // Package name
	IsPublic  bool     `json:"is_public"`
	Calls     []string `json:"calls"`     // IDs of functions this calls
	CalledBy  []string `json:"called_by"` // IDs of functions calling this
}

// CallGraphEdge represents a call relationship.
type CallGraphEdge struct {
	From     string `json:"from"`      // Caller function ID
	To       string `json:"to"`        // Called function ID
	Line     int    `json:"line"`      // Line number of call
	CallType string `json:"call_type"` // direct, defer, go, callback
}

// SearchIndex provides fast lookup without AI.
type SearchIndex struct {
	// Function name index (name fragments -> function refs)
	FunctionNames map[string][]FunctionRef `json:"function_names"`

	// Concept index (concept -> refs)
	Concepts map[string][]FunctionRef `json:"concepts"`

	// Side effects index (effect type -> refs)
	SideEffects map[string][]FunctionRef `json:"side_effects"`

	// Type usage index (type name -> refs)
	TypeUsage map[string][]FunctionRef `json:"type_usage"`

	// Error types index
	ErrorTypes map[string][]FunctionRef `json:"error_types"`

	// HTTP routes index
	Routes map[string][]RouteRef `json:"routes"`

	// Package index
	Packages map[string][]string `json:"packages"` // package -> files
}

// FunctionRef is a lightweight reference to a function.
type FunctionRef struct {
	File      string `json:"file"`
	Function  string `json:"function"`
	Line      int    `json:"line"`
	Signature string `json:"signature,omitempty"`
	Summary   string `json:"summary,omitempty"` // Brief behavior summary
}

// RouteRef references an HTTP route.
type RouteRef struct {
	File    string `json:"file"`
	Handler string `json:"handler"`
	Method  string `json:"method"`
	Path    string `json:"path"`
	Line    int    `json:"line"`
}

// AISummary contains AI-generated analysis of the repository.
type AISummary struct {
	Overview          string   `json:"overview"`
	Purpose           string   `json:"purpose"`
	KeyFeatures       []string `json:"key_features"`
	TechnologyStack   []string `json:"technology_stack"`
	ArchitectureStyle string   `json:"architecture_style"`
	MainComponents    []string `json:"main_components"`
	Suggestions       []string `json:"suggestions,omitempty"`
	GeneratedAt       time.Time `json:"generated_at"`
	TokensUsed        int      `json:"tokens_used"`
	Provider          string   `json:"provider"`
}

// FileContext holds detailed file analysis.
type FileContext struct {
	Path        string        `json:"path"`
	Hash        string        `json:"hash"`
	Language    string        `json:"language"`
	Package     string        `json:"package,omitempty"` // Package name for Go files
	Size        int64         `json:"size"`
	LineCount   int           `json:"line_count"`
	Purpose     string        `json:"purpose"`
	Imports     []Import      `json:"imports"`
	Exports     []Export      `json:"exports"`
	Types       []TypeDef     `json:"types"`
	Functions   []FunctionDef `json:"functions"`
	Constants   []ConstantDef `json:"constants"`
	Routes      []Route       `json:"routes,omitempty"`
	Concepts    []string      `json:"concepts"`
	AnalyzedAt  time.Time     `json:"analyzed_at"`
}

// Route represents an HTTP route/endpoint.
type Route struct {
	Method      string   `json:"method"`
	Path        string   `json:"path"`
	Handler     string   `json:"handler"`
	Line        int      `json:"line"`
	Description string   `json:"description,omitempty"`
	Middleware  []string `json:"middleware,omitempty"`
}

// Import represents an import statement.
type Import struct {
	Path  string `json:"path"`
	Alias string `json:"alias,omitempty"`
}

// Export represents a public API element.
type Export struct {
	Name        string  `json:"name"`
	Kind        string  `json:"kind"` // function, type, const, var
	Signature   string  `json:"signature,omitempty"`
	Description string  `json:"description,omitempty"`
	LineStart   int     `json:"line_start"`
	LineEnd     int     `json:"line_end"`
}

// TypeDef describes a type/struct/interface.
type TypeDef struct {
	Name        string   `json:"name"`
	Kind        string   `json:"kind"` // struct, interface, alias
	Description string   `json:"description,omitempty"`
	Fields      []Field  `json:"fields,omitempty"`
	Methods     []string `json:"methods,omitempty"`
	Implements  []string `json:"implements,omitempty"`
	IsPublic    bool     `json:"is_public"`
	LineStart   int      `json:"line_start"`
	LineEnd     int      `json:"line_end"`
}

// Field represents a struct field.
type Field struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Tag      string `json:"tag,omitempty"`
	IsPublic bool   `json:"is_public"`
}

// FunctionDef provides deep function analysis.
type FunctionDef struct {
	Name        string   `json:"name"`
	Signature   string   `json:"signature"`
	Description string   `json:"description,omitempty"`
	Parameters  []Param  `json:"parameters,omitempty"`
	Returns     []string `json:"returns,omitempty"`
	Receiver    string   `json:"receiver,omitempty"`
	IsPublic    bool     `json:"is_public"`
	LineStart   int      `json:"line_start"`
	LineEnd     int      `json:"line_end"`
	Complexity  int      `json:"complexity,omitempty"`

	// Deep context fields (extracted from AST analysis)
	Behavior      *FunctionBehavior `json:"behavior,omitempty"`
	Calls         []CallRef         `json:"calls,omitempty"`
	CalledBy      []CallRef         `json:"called_by,omitempty"`
	SideEffects   []string          `json:"side_effects,omitempty"`
	ErrorHandling *ErrorHandling    `json:"error_handling,omitempty"`
	UsesImports   []string          `json:"uses_imports,omitempty"`
	APIFlow       *APIFlow          `json:"api_flow,omitempty"` // Complete API flow analysis
}

// FunctionBehavior describes what a function does (derived from AST).
type FunctionBehavior struct {
	Summary      string            `json:"summary"`                 // Auto-generated summary
	Steps        []string          `json:"steps,omitempty"`         // Key operations in order
	InputUsage   map[string]string `json:"input_usage,omitempty"`   // How params are used
	OutputSource string            `json:"output_source,omitempty"` // Where return value comes from
	Patterns     []string          `json:"patterns,omitempty"`      // Detected patterns (CRUD, validation, etc.)
}

// CallRef represents a function call reference.
type CallRef struct {
	Function string `json:"function"`
	Package  string `json:"package,omitempty"`
	Receiver string `json:"receiver,omitempty"` // Receiver type for method calls
	File     string `json:"file,omitempty"`
	Line     int    `json:"line,omitempty"`
	Type     string `json:"type,omitempty"` // internal, stdlib, external, method
}

// ErrorHandling describes how a function handles errors.
type ErrorHandling struct {
	ReturnsError    bool     `json:"returns_error"`
	ErrorTypes      []string `json:"error_types,omitempty"`
	WrapsErrors     bool     `json:"wraps_errors"`
	LogsErrors      bool     `json:"logs_errors"`
	PanicsOnError   bool     `json:"panics_on_error"`
	PropagatesError bool     `json:"propagates_error"`
	ErrorChecks     int      `json:"error_checks"` // Number of "if err != nil" patterns
}

// APIFlow describes the complete flow of an API endpoint or function.
type APIFlow struct {
	IsHTTPHandler   bool            `json:"is_http_handler"`
	RequestPayload  *PayloadInfo    `json:"request_payload,omitempty"`
	ResponsePayload *PayloadInfo    `json:"response_payload,omitempty"`
	Steps           []FlowStep      `json:"steps,omitempty"`
	DBQueries       []DBQuery       `json:"db_queries,omitempty"`
	ExternalCalls   []ExternalCall  `json:"external_calls,omitempty"`
	ValidationSteps []string        `json:"validation_steps,omitempty"`
}

// FlowStep represents a single step in the function's execution flow.
type FlowStep struct {
	StepNumber int    `json:"step_number"`
	Action     string `json:"action"`      // Human-readable description
	Type       string `json:"type"`        // database, http_client, validation, internal, etc.
	Line       int    `json:"line"`
	Input      string `json:"input,omitempty"`
	Output     string `json:"output,omitempty"`
}

// DBQuery represents a database query found in the code.
type DBQuery struct {
	Operation string `json:"operation"` // SELECT, INSERT, UPDATE, DELETE
	Table     string `json:"table,omitempty"`
	RawQuery  string `json:"raw_query,omitempty"`
	Method    string `json:"method"` // The method called (Query, Exec, Find, etc.)
	Line      int    `json:"line"`
}

// ExternalCall represents an external HTTP call.
type ExternalCall struct {
	Method      string `json:"method"` // GET, POST, etc.
	URL         string `json:"url,omitempty"`
	Description string `json:"description,omitempty"`
	Line        int    `json:"line"`
}

// PayloadInfo describes a request or response payload.
type PayloadInfo struct {
	TypeName string      `json:"type_name"`
	Source   string      `json:"source,omitempty"` // json_body, query_params, path_params
	Fields   []FieldInfo `json:"fields,omitempty"`
}

// FieldInfo describes a field in a struct.
type FieldInfo struct {
	Name       string `json:"name"`
	Type       string `json:"type"`
	JSONTag    string `json:"json_tag,omitempty"`
	Validation string `json:"validation,omitempty"`
	Required   bool   `json:"required,omitempty"`
}

// RouteInfo describes HTTP route information.
type RouteInfo struct {
	Method      string   `json:"method"`      // GET, POST, PUT, DELETE, etc.
	Path        string   `json:"path"`        // URL path pattern
	Handler     string   `json:"handler"`     // Handler function name
	Middleware  []string `json:"middleware,omitempty"`
	Description string   `json:"description,omitempty"`
}

// StructDetail contains detailed information about a struct type.
type StructDetail struct {
	Name        string        `json:"name"`
	Category    string        `json:"category"` // request, response, model, config, dto, struct
	Fields      []StructField `json:"fields"`
	Description string        `json:"description,omitempty"`
}

// StructField describes a field in a struct with full details.
type StructField struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	JSONName    string `json:"json_name,omitempty"`
	DBColumn    string `json:"db_column,omitempty"`
	Validation  string `json:"validation,omitempty"`
	Required    bool   `json:"required,omitempty"`
	IsExported  bool   `json:"is_exported"`
	Description string `json:"description,omitempty"`
}

// Param represents a function parameter.
type Param struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// ConstantDef describes a constant.
type ConstantDef struct {
	Name      string `json:"name"`
	Type      string `json:"type,omitempty"`
	Value     string `json:"value,omitempty"`
	IsPublic  bool   `json:"is_public"`
	LineStart int    `json:"line_start"`
}

// ArchitectureContext describes system-level design.
type ArchitectureContext struct {
	Overview        string           `json:"overview"`
	Modules         []Module         `json:"modules"`
	EntryPoints     []EntryPoint     `json:"entry_points"`
	Dependencies    []string         `json:"dependencies"`
	BuildSystem     string           `json:"build_system"`
	MainPackages    []string         `json:"main_packages"`
	PackageType     string           `json:"package_type,omitempty"` // "library" or "application"
	GoVersion       string           `json:"go_version,omitempty"`
	AIAnalysis      *AIArchAnalysis  `json:"ai_analysis,omitempty"`
}

// AIArchAnalysis contains AI-generated architecture analysis.
type AIArchAnalysis struct {
	Pattern         string        `json:"pattern"`
	Layers          []LayerInfo   `json:"layers"`
	DataFlow        string        `json:"data_flow"`
	Strengths       []string      `json:"strengths"`
	Weaknesses      []string      `json:"weaknesses"`
	Recommendations []string      `json:"recommendations"`
	GeneratedAt     time.Time     `json:"generated_at"`
	TokensUsed      int           `json:"tokens_used"`
	Provider        string        `json:"provider"`
}

// LayerInfo describes an architecture layer.
type LayerInfo struct {
	Name       string   `json:"name"`
	Purpose    string   `json:"purpose"`
	Components []string `json:"components"`
}

// Module represents a code module/package.
type Module struct {
	Path        string   `json:"path"`
	Name        string   `json:"name"`
	Purpose     string   `json:"purpose,omitempty"`
	Files       []string `json:"files"`
	IsInternal  bool     `json:"is_internal"`
}

// EntryPoint represents an application entry point.
type EntryPoint struct {
	Path     string `json:"path"`
	Type     string `json:"type"` // main, handler, command
	Purpose  string `json:"purpose,omitempty"`
}

// RepoStatistics summarizes repository metrics.
type RepoStatistics struct {
	TotalFiles        int            `json:"total_files"`
	TotalLines        int            `json:"total_lines"`
	LanguageBreakdown map[string]int `json:"languages"`
	FunctionCount     int            `json:"function_count"`
	TypeCount         int            `json:"type_count"`
	ExportCount       int            `json:"export_count"`

	// Deep analysis stats
	CallGraphNodes int `json:"call_graph_nodes,omitempty"`
	CallGraphEdges int `json:"call_graph_edges,omitempty"`
	IndexedConcepts int `json:"indexed_concepts,omitempty"`
	AvgComplexity   float64 `json:"avg_complexity,omitempty"`
}

// ContextMetadata for listing stored contexts.
type ContextMetadata struct {
	RepoID     string    `json:"repo_id"`
	URL        string    `json:"url"`
	Branch     string    `json:"branch"`
	CommitHash string    `json:"commit_hash"`
	FileCount  int       `json:"file_count"`
	AnalyzedAt time.Time `json:"analyzed_at"`
}

// --- Dependency Graph Types ---

// Sentinel errors for go.mod handling.
var (
	ErrGoModNotFound  = fmt.Errorf("go.mod file not found")
	ErrGoModMalformed = fmt.Errorf("go.mod file is malformed")
)

// ModuleInfo holds parsed go.mod data.
type ModuleInfo struct {
	ModulePath   string             `json:"module_path"`
	GoVersion    string             `json:"go_version"`
	Dependencies []ModuleDependency `json:"dependencies"`
	Replaces     []ModuleReplace    `json:"replaces,omitempty"`
}

// ModuleDependency represents a single go.mod require entry.
type ModuleDependency struct {
	Path       string `json:"path"`
	Version    string `json:"version"`
	IsDirect   bool   `json:"is_direct"`
	IsReplaced bool   `json:"is_replaced,omitempty"`
}

// ModuleReplace represents a go.mod replace directive.
type ModuleReplace struct {
	Old     string `json:"old"`
	New     string `json:"new"`
	Version string `json:"version,omitempty"`
}

// ImportSummary holds aggregated import classifications for a repo.
type ImportSummary struct {
	Stdlib   []string         `json:"stdlib"`
	Internal []string         `json:"internal"`
	External []ExternalImport `json:"external"`
}

// ExternalImport represents an external import resolved to its module.
type ExternalImport struct {
	ImportPath string `json:"import_path"`
	ModulePath string `json:"module_path"`
	Version    string `json:"version,omitempty"`
}

// ConfigFile represents a parsed config file in the repository.
type ConfigFile struct {
	Path           string          `json:"path"`
	Type           string          `json:"type"`
	Content        string          `json:"content,omitempty"`
	StructuredJSON json.RawMessage `json:"structured,omitempty"`
	SizeBytes      int             `json:"size_bytes"`
}

// DependencyGraph represents cross-repo dependency relationships.
type DependencyGraph struct {
	Nodes []DependencyNode `json:"nodes"`
	Edges []DependencyEdge `json:"edges"`
}

// DependencyNode represents a module in the dependency graph.
type DependencyNode struct {
	RepoID      string `json:"repo_id"`
	ModulePath  string `json:"module_path"`
	IsAnalyzed  bool   `json:"is_analyzed"`
	PackageType string `json:"package_type"`
}

// DependencyEdge represents a dependency between two modules.
type DependencyEdge struct {
	From    string `json:"from"`
	To      string `json:"to"`
	Version string `json:"version"`
	Direct  bool   `json:"direct"`
}
