package nlp

import (
	"math"
	"testing"
)

func TestConceptSimilarityBasic(t *testing.T) {
	domain := map[string]bool{"rout": true, "handler": true}

	// All words match
	got := ConceptSimilarity([]string{"routing", "handler"}, domain)
	if got != 1.0 {
		t.Errorf("all match: got %f, want 1.0", got)
	}

	// No words match
	got = ConceptSimilarity([]string{"database", "query"}, domain)
	if got != 0.0 {
		t.Errorf("no match: got %f, want 0.0", got)
	}

	// Half match
	got = ConceptSimilarity([]string{"routing", "database"}, domain)
	if math.Abs(got-0.5) > 0.01 {
		t.Errorf("half match: got %f, want ~0.5", got)
	}
}

func TestConceptSimilarityEdgeCases(t *testing.T) {
	domain := map[string]bool{"test": true}

	if got := ConceptSimilarity(nil, domain); got != 0.0 {
		t.Errorf("nil words: got %f, want 0.0", got)
	}
	if got := ConceptSimilarity([]string{"test"}, nil); got != 0.0 {
		t.Errorf("nil domain: got %f, want 0.0", got)
	}
	if got := ConceptSimilarity([]string{}, domain); got != 0.0 {
		t.Errorf("empty words: got %f, want 0.0", got)
	}
}

func TestConceptSimilarityStopWords(t *testing.T) {
	domain := map[string]bool{"rout": true}

	// Stop words should be excluded
	got := ConceptSimilarity([]string{"Get", "Set", "routing"}, domain)
	if got != 1.0 {
		t.Errorf("stop words excluded: got %f, want 1.0 (only 'routing' counts)", got)
	}

	// Only stop words -> 0.0
	got = ConceptSimilarity([]string{"Get", "Set", "New"}, domain)
	if got != 0.0 {
		t.Errorf("only stop words: got %f, want 0.0", got)
	}
}

func TestConceptSimilarityUseStemmer(t *testing.T) {
	domain := map[string]bool{"rout": true}

	// "routing" should stem to "rout" and match
	got := ConceptSimilarity([]string{"routing"}, domain)
	if got != 1.0 {
		t.Errorf("stemmer matching: got %f, want 1.0", got)
	}
}

func TestBuildDomainProfile(t *testing.T) {
	profile := BuildDomainProfile([]string{"ServeHTTP", "CompressHandler"})
	if !profile["serv"] && !profile["serve"] {
		// Check that some stem of "Serve" exists
		t.Errorf("expected stem of 'Serve' in profile, got %v", profile)
	}
	if !profile["http"] {
		t.Errorf("expected 'http' in profile, got %v", profile)
	}
}

func TestIsStopWord(t *testing.T) {
	if !IsStopWord("Get") {
		t.Error("'Get' should be a stop word")
	}
	if !IsStopWord("get") {
		t.Error("'get' should be a stop word")
	}
	if IsStopWord("routing") {
		t.Error("'routing' should not be a stop word")
	}
}
