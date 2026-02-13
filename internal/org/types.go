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
}

// OrgWithCount extends Org with repo count for listing.
type OrgWithCount struct {
	Org
	RepoCount int `json:"repo_count"`
}
