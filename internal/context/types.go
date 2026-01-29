package context

import "time"

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

// FunctionDef provides function analysis.
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
