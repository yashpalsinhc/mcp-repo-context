package nlp

import "testing"

func TestStemBasicWords(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"routing", "rout"},
		{"handlers", "handler"},
		{"structures", "structur"},
		{"validation", "valid"},
		{"caresses", "caress"},
		{"ponies", "poni"},
		{"cats", "cat"},
	}
	for _, tt := range tests {
		got := Stem(tt.input)
		if got != tt.want {
			t.Errorf("Stem(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestStemEdgeCases(t *testing.T) {
	if got := Stem(""); got != "" {
		t.Errorf("Stem(\"\") = %q, want \"\"", got)
	}
	if got := Stem("x"); got != "x" {
		t.Errorf("Stem(\"x\") = %q, want \"x\"", got)
	}
	if got := Stem("RUNNING"); got != Stem("running") {
		t.Errorf("Stem should lowercase: Stem(\"RUNNING\") = %q, Stem(\"running\") = %q", Stem("RUNNING"), Stem("running"))
	}
	// Unicode passes through
	if got := Stem("日本語"); got != "日本語" {
		t.Errorf("Stem(unicode) = %q, want pass-through", got)
	}
}

func TestSplitCamelCase(t *testing.T) {
	tests := []struct {
		input string
		want  []string
	}{
		{"ServeHTTP", []string{"Serve", "HTTP"}},
		{"getURL", []string{"get", "URL"}},
		{"simple", []string{"simple"}},
		{"CompressHandler", []string{"Compress", "Handler"}},
		{"", nil},
	}
	for _, tt := range tests {
		got := SplitCamelCase(tt.input)
		if len(got) != len(tt.want) {
			t.Errorf("SplitCamelCase(%q) = %v, want %v", tt.input, got, tt.want)
			continue
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Errorf("SplitCamelCase(%q)[%d] = %q, want %q", tt.input, i, got[i], tt.want[i])
			}
		}
	}
}

func TestStemCamelCase(t *testing.T) {
	parts := StemIdentifier("ServeHTTP")
	if len(parts) != 2 {
		t.Fatalf("StemIdentifier(\"ServeHTTP\") = %v, want 2 parts", parts)
	}
	parts = StemIdentifier("CompressHandler")
	if len(parts) != 2 {
		t.Fatalf("StemIdentifier(\"CompressHandler\") = %v, want 2 parts", parts)
	}
}
