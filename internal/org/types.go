package org

import (
	"errors"
	"time"
)

var ErrNotFound = errors.New("org: not found")

// Org represents an organization grouping repositories.
type Org struct {
	ID      string    `json:"id"`
	Repos   []string  `json:"repos"`
	Config  OrgConfig `json:"config"`
	Created time.Time `json:"created"`
}

// OrgConfig holds organization-level configuration.
type OrgConfig struct {
	ExcludePatterns []string `json:"exclude_patterns,omitempty"`
	MaxFileSize     int64    `json:"max_file_size,omitempty"`
	AnalyzerName    string   `json:"analyzer_name,omitempty"`
	EmbedderName    string   `json:"embedder_name,omitempty"`
}

// OrgWithCount extends Org with repo count for listing.
type OrgWithCount struct {
	Org
	RepoCount int `json:"repo_count"`
}

// AnalysisResult holds the outcome of analyzing all repos in an org.
type AnalysisResult struct {
	OrgID     string        `json:"org_id"`
	Total     int           `json:"total"`
	Succeeded int           `json:"succeeded"`
	Failed    int           `json:"failed"`
	Skipped   int           `json:"skipped"`
	Errors    []RepoError   `json:"errors,omitempty"`
	Duration  time.Duration `json:"duration"`
}

// RepoError records a per-repo analysis failure.
type RepoError struct {
	RepoID string `json:"repo_id"`
	Error  string `json:"error"`
}
