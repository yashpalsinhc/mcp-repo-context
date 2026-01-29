package repo

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
)

// Ensure cloner implements Source interface.
var _ Source = (*cloner)(nil)

// cloner handles repository cloning operations (private struct).
type cloner struct {
	tempDir string
}

// NewCloner creates a new repository cloner.
func NewCloner(tempDir string) (Source, error) {
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create temp directory: %w", err)
	}
	return &cloner{tempDir: tempDir}, nil
}

// Clone fetches repository to local temporary directory.
func (c *cloner) Clone(ctx context.Context, repoURL string, opts CloneOptions) (string, func(), error) {
	// Create unique directory for this clone
	repoName := extractRepoName(repoURL)
	localPath := filepath.Join(c.tempDir, repoName)

	// Cleanup function
	cleanup := func() {
		os.RemoveAll(localPath)
	}

	// Remove if exists
	os.RemoveAll(localPath)

	// Prepare clone options
	cloneOpts := &git.CloneOptions{
		URL:      repoURL,
		Progress: nil, // Silent
	}

	if opts.Branch != "" {
		cloneOpts.ReferenceName = plumbing.NewBranchReferenceName(opts.Branch)
		cloneOpts.SingleBranch = true
	}

	if opts.Depth > 0 {
		cloneOpts.Depth = opts.Depth
	}

	if opts.GitHubToken != "" {
		cloneOpts.Auth = &http.BasicAuth{
			Username: "x-access-token",
			Password: opts.GitHubToken,
		}
	}

	// Clone the repository
	_, err := git.PlainCloneContext(ctx, localPath, false, cloneOpts)
	if err != nil {
		cleanup()
		return "", nil, fmt.Errorf("failed to clone repository: %w", err)
	}

	return localPath, cleanup, nil
}

// GetCurrentCommit returns the HEAD commit hash.
func (c *cloner) GetCurrentCommit(ctx context.Context, localPath string) (string, error) {
	repo, err := git.PlainOpen(localPath)
	if err != nil {
		return "", fmt.Errorf("failed to open repository: %w", err)
	}

	head, err := repo.Head()
	if err != nil {
		return "", fmt.Errorf("failed to get HEAD: %w", err)
	}

	return head.Hash().String(), nil
}

// GetBranch returns the current branch name.
func (c *cloner) GetBranch(ctx context.Context, localPath string) (string, error) {
	repo, err := git.PlainOpen(localPath)
	if err != nil {
		return "", fmt.Errorf("failed to open repository: %w", err)
	}

	head, err := repo.Head()
	if err != nil {
		return "", fmt.Errorf("failed to get HEAD: %w", err)
	}

	if head.Name().IsBranch() {
		return head.Name().Short(), nil
	}

	return "HEAD", nil
}

func extractRepoName(url string) string {
	// Handle both HTTPS and SSH URLs
	url = strings.TrimSuffix(url, ".git")

	// Extract last two parts (owner/repo)
	parts := strings.Split(url, "/")
	if len(parts) >= 2 {
		return parts[len(parts)-2] + "_" + parts[len(parts)-1]
	}

	return filepath.Base(url)
}
