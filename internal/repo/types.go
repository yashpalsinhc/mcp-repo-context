package repo

import (
	"strings"
	"time"
)

// CloneOptions configures repository cloning.
type CloneOptions struct {
	Branch      string
	Depth       int
	GitHubToken string
}

// FileInfo describes a file in the repository.
type FileInfo struct {
	Path        string
	AbsPath     string
	Size        int64
	Hash        string
	Language    string
	IsDirectory bool
	ModTime     time.Time
}

// Commit represents a git commit.
type Commit struct {
	Hash      string
	Author    string
	Message   string
	Timestamp time.Time
	Files     []string
}

// RepoIDFromURL creates a consistent repo ID from URL.
func RepoIDFromURL(url string) string {
	url = strings.TrimSuffix(url, ".git")
	url = strings.TrimPrefix(url, "https://")
	url = strings.TrimPrefix(url, "http://")
	url = strings.TrimPrefix(url, "git@")
	url = strings.ReplaceAll(url, ":", "/")
	return url
}
