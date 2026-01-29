package repo

import (
	"testing"
)

func TestRepoIDFromURL(t *testing.T) {
	tests := []struct {
		url      string
		expected string
	}{
		{
			url:      "https://github.com/owner/repo",
			expected: "github.com/owner/repo",
		},
		{
			url:      "https://github.com/owner/repo.git",
			expected: "github.com/owner/repo",
		},
		{
			url:      "http://github.com/owner/repo",
			expected: "github.com/owner/repo",
		},
		{
			url:      "git@github.com:owner/repo.git",
			expected: "github.com/owner/repo",
		},
		{
			url:      "https://gitlab.com/group/subgroup/repo",
			expected: "gitlab.com/group/subgroup/repo",
		},
		{
			url:      "https://bitbucket.org/user/repo.git",
			expected: "bitbucket.org/user/repo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			result := RepoIDFromURL(tt.url)
			if result != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, result)
			}
		})
	}
}
