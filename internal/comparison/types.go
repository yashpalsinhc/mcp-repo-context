package comparison

// CompareOptions configures the comparison behavior.
type CompareOptions struct {
	// TargetRepoID identifies which repo is the merge target (optional).
	TargetRepoID string

	// IncludeDuplicates enables duplicate detection.
	IncludeDuplicates bool

	// IncludeConflicts enables conflict detection.
	IncludeConflicts bool

	// IncludeGaps enables gap detection.
	IncludeGaps bool

	// IncludeConsistency enables consistency analysis.
	IncludeConsistency bool

	// SimilarityThreshold for duplicate detection (0.0-1.0).
	SimilarityThreshold float64
}

// DefaultCompareOptions returns sensible defaults for comparison.
func DefaultCompareOptions() CompareOptions {
	return CompareOptions{
		IncludeDuplicates:   true,
		IncludeConflicts:    true,
		IncludeGaps:         true,
		IncludeConsistency:  true,
		SimilarityThreshold: 0.8,
	}
}

// CompareResult contains the full comparison analysis.
type CompareResult struct {
	// Repos contains metadata about compared repos.
	Repos []RepoSummary `json:"repos"`

	// UnifiedStatistics aggregates statistics across all repos.
	UnifiedStatistics UnifiedStats `json:"unified_statistics"`

	// Duplicates lists groups of duplicate/similar implementations.
	Duplicates []DuplicateGroup `json:"duplicates,omitempty"`

	// Conflicts lists conflicting implementations.
	Conflicts []Conflict `json:"conflicts,omitempty"`

	// Gaps lists missing functionality in target repo.
	Gaps []Gap `json:"gaps,omitempty"`

	// Consistency contains the consistency report.
	Consistency *ConsistencyReport `json:"consistency,omitempty"`

	// FileOverlap shows files that exist in multiple repos.
	FileOverlap []FileOverlapInfo `json:"file_overlap,omitempty"`

	// Recommendations for the merge process.
	Recommendations []string `json:"recommendations,omitempty"`
}

// RepoSummary provides a quick overview of a repo in comparison.
type RepoSummary struct {
	ID            string         `json:"id"`
	URL           string         `json:"url"`
	Branch        string         `json:"branch"`
	FileCount     int            `json:"file_count"`
	Languages     map[string]int `json:"languages"`
	IsTarget      bool           `json:"is_target"`
	TotalLines    int            `json:"total_lines"`
	FunctionCount int            `json:"function_count"`
	TypeCount     int            `json:"type_count"`
}

// UnifiedStats aggregates statistics across all repos.
type UnifiedStats struct {
	TotalRepos          int            `json:"total_repos"`
	TotalFiles          int            `json:"total_files"`
	TotalLines          int            `json:"total_lines"`
	UniqueFiles         int            `json:"unique_files"`
	OverlappingFiles    int            `json:"overlapping_files"`
	TotalFunctions      int            `json:"total_functions"`
	TotalTypes          int            `json:"total_types"`
	LanguageBreakdown   map[string]int `json:"language_breakdown"`
}

// DuplicateGroup represents a group of similar/duplicate implementations.
type DuplicateGroup struct {
	// Type of duplicate: "function", "type", "file", "pattern".
	Type string `json:"type"`

	// Name of the duplicated element.
	Name string `json:"name"`

	// Instances lists where this duplicate appears.
	Instances []DuplicateInstance `json:"instances"`

	// Similarity score between instances (0.0-1.0).
	Similarity float64 `json:"similarity"`

	// Recommendation for handling this duplicate.
	Recommendation string `json:"recommendation"`
}

// DuplicateInstance represents one occurrence of a duplicate.
type DuplicateInstance struct {
	RepoID    string `json:"repo_id"`
	FilePath  string `json:"file_path"`
	Line      int    `json:"line,omitempty"`
	Signature string `json:"signature,omitempty"`
}

// Conflict represents a conflicting implementation between repos.
type Conflict struct {
	// Type of conflict: "signature_mismatch", "behavior_difference", "naming_conflict".
	Type string `json:"type"`

	// Name of the conflicting element.
	Name string `json:"name"`

	// Description of the conflict.
	Description string `json:"description"`

	// SourceInstances in source repos.
	SourceInstances []ConflictInstance `json:"source_instances"`

	// TargetInstance in target repo.
	TargetInstance *ConflictInstance `json:"target_instance,omitempty"`

	// Severity: "high", "medium", "low".
	Severity string `json:"severity"`

	// Resolution suggestion.
	Resolution string `json:"resolution"`
}

// ConflictInstance represents one side of a conflict.
type ConflictInstance struct {
	RepoID    string `json:"repo_id"`
	FilePath  string `json:"file_path"`
	Line      int    `json:"line,omitempty"`
	Signature string `json:"signature,omitempty"`
	Details   string `json:"details,omitempty"`
}

// Gap represents functionality missing in target repo.
type Gap struct {
	// Type of gap: "function", "type", "file", "package".
	Type string `json:"type"`

	// Name of the missing element.
	Name string `json:"name"`

	// SourceRepos where this functionality exists.
	SourceRepos []string `json:"source_repos"`

	// FilePath where this should be added.
	FilePath string `json:"file_path"`

	// Description of what's missing.
	Description string `json:"description"`

	// Priority: "high", "medium", "low".
	Priority string `json:"priority"`
}

// ConsistencyReport analyzes implementation consistency.
type ConsistencyReport struct {
	// OverallScore from 0.0 to 1.0.
	OverallScore float64 `json:"overall_score"`

	// NamingConsistency analyzes naming conventions.
	NamingConsistency ConsistencyCategory `json:"naming_consistency"`

	// PatternConsistency analyzes code patterns.
	PatternConsistency ConsistencyCategory `json:"pattern_consistency"`

	// StructureConsistency analyzes project structure.
	StructureConsistency ConsistencyCategory `json:"structure_consistency"`

	// Issues lists specific consistency problems.
	Issues []ConsistencyIssue `json:"issues,omitempty"`
}

// ConsistencyCategory represents a category of consistency analysis.
type ConsistencyCategory struct {
	Score       float64  `json:"score"`
	Description string   `json:"description"`
	Examples    []string `json:"examples,omitempty"`
}

// ConsistencyIssue represents a specific consistency problem.
type ConsistencyIssue struct {
	Type        string   `json:"type"`
	Description string   `json:"description"`
	Repos       []string `json:"repos"`
	Severity    string   `json:"severity"`
	Suggestion  string   `json:"suggestion"`
}

// FileOverlapInfo shows files that exist in multiple repos.
type FileOverlapInfo struct {
	// Path is the file path (relative).
	Path string `json:"path"`

	// Repos that contain this file.
	Repos []string `json:"repos"`

	// Identical indicates if files are identical across repos.
	Identical bool `json:"identical"`

	// Hashes maps repo ID to file hash.
	Hashes map[string]string `json:"hashes,omitempty"`
}
