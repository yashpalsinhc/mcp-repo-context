package nlp

import "testing"

func TestLevenshteinDistance(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"kitten", "sitting", 3},
		{"", "abc", 3},
		{"abc", "", 3},
		{"same", "same", 0},
		{"a", "b", 1},
	}
	for _, tt := range tests {
		got := LevenshteinDistance(tt.a, tt.b)
		if got != tt.want {
			t.Errorf("LevenshteinDistance(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestLevenshteinEarlyTermination(t *testing.T) {
	// Very different strings with low max
	got := LevenshteinDistanceMax("abcdef", "uvwxyz", 2)
	if got <= 2 {
		t.Errorf("expected early termination (>2), got %d", got)
	}

	// Same strings should return 0 regardless of max
	got = LevenshteinDistanceMax("same", "same", 1)
	if got != 0 {
		t.Errorf("expected 0 for identical strings, got %d", got)
	}
}

func TestFuzzyMatch(t *testing.T) {
	got := FuzzyMatch("route", []string{"router", "handler", "routing"}, 3)
	if len(got) != 2 {
		t.Errorf("FuzzyMatch = %v, want 2 matches (router, routing)", got)
	}

	got = FuzzyMatch("xyz", []string{"abc", "def"}, 1)
	if len(got) != 0 {
		t.Errorf("FuzzyMatch = %v, want empty", got)
	}

	got = FuzzyMatch("test", nil, 2)
	if len(got) != 0 {
		t.Errorf("FuzzyMatch(nil candidates) = %v, want empty", got)
	}
}
