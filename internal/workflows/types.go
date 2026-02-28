// Package workflows provides high-level workflow tools for multi-repo operations.
package workflows

import "sort"

// RiskLevel represents the risk level of a workflow operation.
type RiskLevel string

const (
	RiskLow    RiskLevel = "low"
	RiskMedium RiskLevel = "medium"
	RiskHigh   RiskLevel = "high"
)

// FeaturePlan is the result of the build_feature workflow.
type FeaturePlan struct {
	Feature        string         `json:"feature"`
	OrgID          string         `json:"org_id"`
	RelevantCode   []CodeLocation `json:"relevant_code"`
	EntryPoints    []EntryPoint   `json:"entry_points"`
	Dependencies   []Dependency   `json:"dependencies"`
	FilesToTouch   []FileAction   `json:"files_to_touch"`
	SuggestedOrder []string       `json:"suggested_order"`
	RiskLevel      RiskLevel      `json:"risk_level"`
	AIEnhancement  string         `json:"ai_enhancement,omitempty"`
	Warnings       []string       `json:"warnings,omitempty"`
}

// RefactorPlan is the result of the refactor_org workflow.
type RefactorPlan struct {
	Pattern        string         `json:"pattern"`
	OrgID          string         `json:"org_id"`
	Usages         []CodeLocation `json:"usages"`
	AffectedFiles  []FileAction   `json:"affected_files"`
	ImpactAnalysis ImpactSummary  `json:"impact_analysis"`
	RiskLevel      RiskLevel      `json:"risk_level"`
	AIEnhancement  string         `json:"ai_enhancement,omitempty"`
	Warnings       []string       `json:"warnings,omitempty"`
}

// MergeReport is the result of the merge_repos workflow.
type MergeReport struct {
	SourceRepos []string           `json:"source_repos"`
	TargetRepo  string             `json:"target_repo"`
	Duplicates  []DuplicateSummary `json:"duplicates"`
	Conflicts   []ConflictSummary  `json:"conflicts"`
	Gaps        []GapSummary       `json:"gaps"`
	MergeOrder  []MergeStep        `json:"merge_order"`
	RiskLevel   RiskLevel          `json:"risk_level"`
	Warnings    []string           `json:"warnings,omitempty"`
}

// CodeLocation references a code element found by search.
type CodeLocation struct {
	RepoID   string  `json:"repo_id"`
	FilePath string  `json:"file_path"`
	FuncName string  `json:"func_name"`
	Line     int     `json:"line"`
	Summary  string  `json:"summary"`
	Score    float64 `json:"score"`
}

// FileAction describes an action to take on a file.
type FileAction struct {
	RepoID   string `json:"repo_id"`
	FilePath string `json:"file_path"`
	Action   string `json:"action"` // "modify", "create", "review"
	Reason   string `json:"reason"`
}

// EntryPoint identifies a public entry point function.
type EntryPoint struct {
	RepoID   string `json:"repo_id"`
	FilePath string `json:"file_path"`
	FuncName string `json:"func_name"`
	Why      string `json:"why"`
}

// Dependency represents a cross-repo dependency.
type Dependency struct {
	SourceRepoID string `json:"source_repo_id"`
	SourceFunc   string `json:"source_func"`
	TargetRepoID string `json:"target_repo_id"`
	TargetFunc   string `json:"target_func"`
	Type         string `json:"type"`       // "name_match", "module_import"
	Confidence   string `json:"confidence"` // "high", "medium"
}

// ImpactSummary summarizes the impact of a refactoring.
type ImpactSummary struct {
	DirectCallers   int      `json:"direct_callers"`
	IndirectCallers int      `json:"indirect_callers"`
	AffectedRepos   []string `json:"affected_repos"`
	HotPaths        []string `json:"hot_paths"`
}

// DuplicateSummary summarizes a duplicate finding.
type DuplicateSummary struct {
	FunctionName   string   `json:"function_name"`
	Repos          []string `json:"repos"`
	Similarity     float64  `json:"similarity"`
	Recommendation string   `json:"recommendation"`
}

// ConflictSummary summarizes a conflict finding.
type ConflictSummary struct {
	FunctionName string `json:"function_name"`
	Type         string `json:"type"`
	Severity     string `json:"severity"`
	SourceRepo   string `json:"source_repo"`
	Resolution   string `json:"resolution"`
}

// GapSummary summarizes a gap finding.
type GapSummary struct {
	FunctionName string `json:"function_name"`
	SourceRepo   string `json:"source_repo"`
	Priority     string `json:"priority"`
	Description  string `json:"description"`
}

// MergeStep describes a single step in a merge plan.
type MergeStep struct {
	Order       int       `json:"order"`
	Action      string    `json:"action"` // "migrate", "resolve_conflict", "fill_gap", "consolidate_duplicate"
	SourceRepo  string    `json:"source_repo"`
	TargetItem  string    `json:"target_item"`
	Description string    `json:"description"`
	Risk        RiskLevel `json:"risk"`
}

// SortByScore sorts a slice of CodeLocation by Score descending.
func SortByScore(locations []CodeLocation) {
	sort.Slice(locations, func(i, j int) bool {
		return locations[i].Score > locations[j].Score
	})
}
