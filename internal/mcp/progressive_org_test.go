package mcp

import (
	"strings"
	"testing"

	ctxpkg "github.com/yashpalc/mcp-repo-context/internal/context"
	"github.com/yashpalc/mcp-repo-context/internal/search"
)

func TestMakeOrgFunctionDetailRef(t *testing.T) {
	ref := MakeOrgFunctionDetailRef("github.com/org/user-service", "pkg/handlers/user.go", "GetUser")
	expected := "func|github.com/org/user-service|pkg/handlers/user.go|GetUser"
	if ref != expected {
		t.Errorf("expected %q, got %q", expected, ref)
	}
}

func TestMakeOrgFunctionDetailRef_SpecialChars(t *testing.T) {
	ref := MakeOrgFunctionDetailRef("github.com/org/my-service", "pkg/auth/jwt_validator.go", "Validate")
	expected := "func|github.com/org/my-service|pkg/auth/jwt_validator.go|Validate"
	if ref != expected {
		t.Errorf("expected %q, got %q", expected, ref)
	}
}

func TestExpandDetailRef(t *testing.T) {
	refType, repoID, filePath, funcName, err := ExpandDetailRef("func|github.com/org/user-service|pkg/handlers/user.go|GetUser")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if refType != "func" || repoID != "github.com/org/user-service" || filePath != "pkg/handlers/user.go" || funcName != "GetUser" {
		t.Errorf("unexpected values: type=%s repo=%s file=%s func=%s", refType, repoID, filePath, funcName)
	}
}

func TestExpandDetailRef_OldColonFormat(t *testing.T) {
	_, _, _, _, err := ExpandDetailRef("func:github.com/org/repo:path/file.go:Func")
	if err == nil {
		t.Error("expected error for old colon format")
	}
}

func TestExpandDetailRef_InvalidRef(t *testing.T) {
	_, _, _, _, err := ExpandDetailRef("invalid-ref-string")
	if err == nil {
		t.Error("expected error for invalid ref")
	}
}

func TestShortRepoName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"github.com/myorg/user-service", "user-service"},
		{"github.com/org/repo", "repo"},
		{"simple", "simple"},
		{"a/b", "b"},
	}
	for _, tt := range tests {
		got := shortRepoName(tt.input)
		if got != tt.want {
			t.Errorf("shortRepoName(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestFormatOrgSearchResult_Header(t *testing.T) {
	results := makeTestRankedResults(5, 2)
	output := FormatOrgSearchResult("User", "my-org", results, 4000)

	if !strings.Contains(output, `"User"`) {
		t.Error("expected query in header")
	}
	if !strings.Contains(output, "my-org") {
		t.Error("expected org in header")
	}
	if !strings.Contains(output, "5 results") {
		t.Error("expected result count")
	}
	if !strings.Contains(output, "2 repos") {
		t.Error("expected repo count")
	}
}

func TestFormatOrgSearchResult_IndividualResult(t *testing.T) {
	results := []search.RankedResult{
		{
			FunctionRef: ctxpkg.FunctionRef{
				Function: "GetUser",
				File:     "pkg/handlers/user.go",
				Line:     45,
				Summary:  "Retrieves a user by ID",
				RepoID:   "github.com/myorg/user-service",
			},
			RRFScore: 0.0312,
		},
	}

	output := FormatOrgSearchResult("User", "my-org", results, 4000)

	if !strings.Contains(output, "**GetUser**") {
		t.Error("expected function name in bold")
	}
	if !strings.Contains(output, "(user-service)") {
		t.Error("expected short repo name")
	}
	if !strings.Contains(output, "pkg/handlers/user.go:45") {
		t.Error("expected file:line")
	}
	if !strings.Contains(output, "Retrieves a user by ID") {
		t.Error("expected summary")
	}
	if !strings.Contains(output, "func|github.com/myorg/user-service|pkg/handlers/user.go|GetUser") {
		t.Error("expected detail_ref with pipe separator")
	}
}

func TestFormatOrgSearchResult_TokenBudget(t *testing.T) {
	results := makeTestRankedResults(50, 5)
	output := FormatOrgSearchResult("Get", "my-org", results, 500)

	// Should be truncated
	if !strings.Contains(output, "more results") {
		t.Error("expected truncation message")
	}
	if !strings.Contains(output, "Token budget:") {
		t.Error("expected token budget footer")
	}

	// Should fit roughly within budget
	outputTokens := len(output) / 4
	if outputTokens > 700 { // Allow some slack
		t.Errorf("output too large: ~%d tokens for budget 500", outputTokens)
	}
}

func TestFormatOrgSearchResult_NoBudgetTruncation(t *testing.T) {
	results := makeTestRankedResults(3, 1)
	output := FormatOrgSearchResult("User", "my-org", results, 4000)

	if strings.Contains(output, "more results") {
		t.Error("should not have truncation message")
	}
}

func TestFormatOrgSearchResult_EmptyResults(t *testing.T) {
	output := FormatOrgSearchResult("xyz", "my-org", nil, 4000)
	if !strings.Contains(output, "No results found") {
		t.Error("expected no results message")
	}
}

func TestFormatOrgSearchResult_NoLineNumber(t *testing.T) {
	results := []search.RankedResult{
		{
			FunctionRef: ctxpkg.FunctionRef{
				Function: "GetUser",
				File:     "pkg/user.go",
				Line:     0,
				RepoID:   "repo-a",
			},
		},
	}
	output := FormatOrgSearchResult("User", "org", results, 4000)
	if strings.Contains(output, ":0") {
		t.Error("should omit line number when 0")
	}
}

// makeTestRankedResults creates N test results spread across numRepos repos.
func makeTestRankedResults(n, numRepos int) []search.RankedResult {
	results := make([]search.RankedResult, n)
	for i := 0; i < n; i++ {
		repoIdx := i % numRepos
		results[i] = search.RankedResult{
			FunctionRef: ctxpkg.FunctionRef{
				Function: strings.Repeat("Func", 1) + string(rune('A'+i%26)),
				File:     "pkg/handlers/file.go",
				Line:     10 + i,
				Summary:  "A test function that does something",
				RepoID:   "github.com/org/repo-" + string(rune('a'+repoIdx)),
			},
			RRFScore: 1.0 / float64(60+i+1),
		}
	}
	return results
}
