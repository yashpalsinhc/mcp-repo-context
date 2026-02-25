package search

import (
	"math"
	"testing"

	ctxpkg "github.com/yashpalc/mcp-repo-context/internal/context"
	"github.com/yashpalc/mcp-repo-context/internal/vectors"
)

func TestMergeRRF_KeywordOnly(t *testing.T) {
	kw := []ctxpkg.FunctionRef{
		{Function: "A", File: "a.go", RepoID: "repo1"},
		{Function: "B", File: "b.go", RepoID: "repo1"},
		{Function: "C", File: "c.go", RepoID: "repo1"},
	}

	results := MergeRRF(kw, nil, DefaultRRFK)
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}

	// A at rank 1: score = 1/(60+1)
	expectScore(t, results[0].RRFScore, 1.0/61.0)
	if results[0].KeywordRank != 1 {
		t.Errorf("expected KeywordRank=1, got %d", results[0].KeywordRank)
	}
	if results[0].SemanticRank != 0 {
		t.Errorf("expected SemanticRank=0, got %d", results[0].SemanticRank)
	}
}

func TestMergeRRF_SemanticOnly(t *testing.T) {
	sem := []vectors.SearchResult{
		{Record: vectors.VectorRecord{Name: "X", FilePath: "x.go", RepoID: "repo1", Type: "function"}, Similarity: 0.9},
		{Record: vectors.VectorRecord{Name: "Y", FilePath: "y.go", RepoID: "repo1", Type: "function"}, Similarity: 0.8},
		{Record: vectors.VectorRecord{Name: "Z", FilePath: "z.go", RepoID: "repo1", Type: "function"}, Similarity: 0.7},
	}

	results := MergeRRF(nil, sem, DefaultRRFK)
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}

	expectScore(t, results[0].RRFScore, 1.0/61.0)
	if results[0].SemanticRank != 1 {
		t.Errorf("expected SemanticRank=1, got %d", results[0].SemanticRank)
	}
	if results[0].KeywordRank != 0 {
		t.Errorf("expected KeywordRank=0, got %d", results[0].KeywordRank)
	}
}

func TestMergeRRF_BoostsOverlap(t *testing.T) {
	kw := []ctxpkg.FunctionRef{
		{Function: "DeleteUser", File: "u.go", RepoID: "repo1"}, // rank 1
		{Function: "ListUser", File: "u.go", RepoID: "repo1"},   // rank 2
		{Function: "GetUser", File: "u.go", RepoID: "repo1"},    // rank 3
	}
	sem := []vectors.SearchResult{
		{Record: vectors.VectorRecord{Name: "Other1", FilePath: "o.go", RepoID: "repo1", Type: "function"}, Similarity: 0.9},
		{Record: vectors.VectorRecord{Name: "Other2", FilePath: "o.go", RepoID: "repo1", Type: "function"}, Similarity: 0.8},
		{Record: vectors.VectorRecord{Name: "Other3", FilePath: "o.go", RepoID: "repo1", Type: "function"}, Similarity: 0.7},
		{Record: vectors.VectorRecord{Name: "Other4", FilePath: "o.go", RepoID: "repo1", Type: "function"}, Similarity: 0.6},
		{Record: vectors.VectorRecord{Name: "GetUser", FilePath: "u.go", RepoID: "repo1", Type: "function"}, Similarity: 0.5}, // rank 5
	}

	results := MergeRRF(kw, sem, DefaultRRFK)

	// Find GetUser and DeleteUser
	var getUserScore, deleteUserScore float64
	for _, r := range results {
		switch r.FunctionRef.Function {
		case "GetUser":
			getUserScore = r.RRFScore
			if r.KeywordRank != 3 || r.SemanticRank != 5 {
				t.Errorf("GetUser: expected kw=3 sem=5, got kw=%d sem=%d", r.KeywordRank, r.SemanticRank)
			}
		case "DeleteUser":
			deleteUserScore = r.RRFScore
		}
	}

	// GetUser should have higher score (overlap boost)
	expectedGetUser := 1.0/63.0 + 1.0/65.0
	expectScore(t, getUserScore, expectedGetUser)
	if getUserScore <= deleteUserScore {
		t.Errorf("GetUser (%.6f) should rank higher than DeleteUser (%.6f) due to overlap", getUserScore, deleteUserScore)
	}
}

func TestMergeRRF_Deduplicates(t *testing.T) {
	kw := []ctxpkg.FunctionRef{
		{Function: "GetUser", File: "pkg/user.go", RepoID: "repo-a"},
	}
	sem := []vectors.SearchResult{
		{Record: vectors.VectorRecord{Name: "GetUser", FilePath: "pkg/user.go", RepoID: "repo-a", Type: "function"}, Similarity: 0.9},
	}

	results := MergeRRF(kw, sem, DefaultRRFK)
	if len(results) != 1 {
		t.Fatalf("expected 1 (deduplicated), got %d", len(results))
	}
	if results[0].KeywordRank != 1 || results[0].SemanticRank != 1 {
		t.Errorf("expected kw=1 sem=1, got kw=%d sem=%d", results[0].KeywordRank, results[0].SemanticRank)
	}
}

func TestMergeRRF_SortedDescending(t *testing.T) {
	kw := []ctxpkg.FunctionRef{
		{Function: "A", File: "a.go", RepoID: "r"},
		{Function: "B", File: "b.go", RepoID: "r"},
		{Function: "C", File: "c.go", RepoID: "r"},
	}
	results := MergeRRF(kw, nil, DefaultRRFK)

	for i := 1; i < len(results); i++ {
		if results[i].RRFScore > results[i-1].RRFScore {
			t.Errorf("results not sorted descending at index %d: %.6f > %.6f", i, results[i].RRFScore, results[i-1].RRFScore)
		}
	}
}

func TestMergeRRF_CustomK(t *testing.T) {
	kw := []ctxpkg.FunctionRef{
		{Function: "A", File: "a.go", RepoID: "r"},
		{Function: "B", File: "b.go", RepoID: "r"},
	}

	results10 := MergeRRF(kw, nil, 10)
	results60 := MergeRRF(kw, nil, 60)

	// With k=10, rank gap should be larger
	gap10 := results10[0].RRFScore - results10[1].RRFScore
	gap60 := results60[0].RRFScore - results60[1].RRFScore

	if gap10 <= gap60 {
		t.Errorf("k=10 gap (%.6f) should be larger than k=60 gap (%.6f)", gap10, gap60)
	}
}

func TestMergeRRF_FiltersNonFunction(t *testing.T) {
	sem := []vectors.SearchResult{
		{Record: vectors.VectorRecord{Name: "UserType", FilePath: "t.go", RepoID: "r", Type: "type"}, Similarity: 0.9},
		{Record: vectors.VectorRecord{Name: "GetUser", FilePath: "u.go", RepoID: "r", Type: "function"}, Similarity: 0.8},
	}

	results := MergeRRF(nil, sem, DefaultRRFK)
	if len(results) != 1 {
		t.Fatalf("expected 1 (type filtered), got %d", len(results))
	}
	if results[0].FunctionRef.Function != "GetUser" {
		t.Errorf("expected GetUser, got %s", results[0].FunctionRef.Function)
	}
}

func TestMergeRRF_EmptyInputs(t *testing.T) {
	results := MergeRRF(nil, nil, DefaultRRFK)
	if len(results) != 0 {
		t.Errorf("expected empty results, got %d", len(results))
	}
}

func TestMergeRRF_KeyGeneration(t *testing.T) {
	kw := []ctxpkg.FunctionRef{
		{Function: "GetUser", File: "pkg/handlers/user.go", RepoID: "github.com/org/user-service"},
	}
	sem := []vectors.SearchResult{
		{Record: vectors.VectorRecord{
			Name: "GetUser", FilePath: "pkg/handlers/user.go", RepoID: "github.com/org/user-service", Type: "function",
		}, Similarity: 0.9},
	}

	results := MergeRRF(kw, sem, DefaultRRFK)
	if len(results) != 1 {
		t.Fatalf("expected 1 (deduped by key), got %d", len(results))
	}
}

func TestMergeRRF_PreservesFunctionRef(t *testing.T) {
	kw := []ctxpkg.FunctionRef{
		{Function: "GetUser", File: "u.go", Line: 42, Signature: "func GetUser() error", Summary: "Gets user", RepoID: "repo-a"},
	}

	results := MergeRRF(kw, nil, DefaultRRFK)
	if len(results) != 1 {
		t.Fatal("expected 1 result")
	}

	ref := results[0].FunctionRef
	if ref.Function != "GetUser" || ref.File != "u.go" || ref.Line != 42 || ref.Signature != "func GetUser() error" || ref.Summary != "Gets user" || ref.RepoID != "repo-a" {
		t.Errorf("FunctionRef fields not preserved: %+v", ref)
	}
}

func expectScore(t *testing.T, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 1e-9 {
		t.Errorf("expected score %.9f, got %.9f", want, got)
	}
}
