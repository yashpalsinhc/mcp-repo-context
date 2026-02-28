package org

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestMergeConfigs(t *testing.T) {
	tests := []struct {
		name     string
		org      *OrgConfig
		override *OrgConfig
		want     *OrgConfig
	}{
		{
			name:     "both nil returns nil",
			org:      nil,
			override: nil,
			want:     nil,
		},
		{
			name:     "nil override returns copy of org config",
			org:      &OrgConfig{ExcludePatterns: []string{"*.log"}, MaxFileSize: 100},
			override: nil,
			want:     &OrgConfig{ExcludePatterns: []string{"*.log"}, MaxFileSize: 100},
		},
		{
			name:     "nil org config returns copy of override",
			org:      nil,
			override: &OrgConfig{ExcludePatterns: []string{"*.tmp"}, MaxFileSize: 200},
			want:     &OrgConfig{ExcludePatterns: []string{"*.tmp"}, MaxFileSize: 200},
		},
		{
			name:     "ExcludePatterns are unioned",
			org:      &OrgConfig{ExcludePatterns: []string{"*.log"}},
			override: &OrgConfig{ExcludePatterns: []string{"*.tmp"}},
			want:     &OrgConfig{ExcludePatterns: []string{"*.log", "*.tmp"}},
		},
		{
			name:     "ExcludePatterns overlapping entries are deduplicated",
			org:      &OrgConfig{ExcludePatterns: []string{"*.log", "*.tmp"}},
			override: &OrgConfig{ExcludePatterns: []string{"*.tmp", "*.bak"}},
			want:     &OrgConfig{ExcludePatterns: []string{"*.log", "*.tmp", "*.bak"}},
		},
		{
			name:     "MaxFileSize repo override wins when non-zero",
			org:      &OrgConfig{MaxFileSize: 100},
			override: &OrgConfig{MaxFileSize: 200},
			want:     &OrgConfig{MaxFileSize: 200},
		},
		{
			name:     "MaxFileSize org value used when override is zero",
			org:      &OrgConfig{MaxFileSize: 100},
			override: &OrgConfig{MaxFileSize: 0},
			want:     &OrgConfig{MaxFileSize: 100},
		},
		{
			name:     "empty ExcludePatterns on both returns empty slice",
			org:      &OrgConfig{ExcludePatterns: []string{}},
			override: &OrgConfig{ExcludePatterns: []string{}},
			want:     &OrgConfig{ExcludePatterns: []string{}},
		},
		{
			name:     "one empty one populated ExcludePatterns returns populated",
			org:      &OrgConfig{ExcludePatterns: []string{}},
			override: &OrgConfig{ExcludePatterns: []string{"*.tmp"}},
			want:     &OrgConfig{ExcludePatterns: []string{"*.tmp"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MergeConfigs(tt.org, tt.override)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("MergeConfigs() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestMergeConfigs_DoesNotMutateInputs(t *testing.T) {
	org := &OrgConfig{ExcludePatterns: []string{"*.log"}, MaxFileSize: 100}
	override := &OrgConfig{ExcludePatterns: []string{"*.tmp"}, MaxFileSize: 200}

	result := MergeConfigs(org, override)

	// Mutate result elements in-place (stronger than append which may re-allocate)
	result.ExcludePatterns[0] = "MUTATED"
	result.MaxFileSize = 999

	// Originals must be unchanged
	if org.ExcludePatterns[0] != "*.log" {
		t.Error("MergeConfigs mutated org ExcludePatterns element")
	}
	if org.MaxFileSize != 100 {
		t.Error("MergeConfigs mutated org MaxFileSize")
	}
	if override.ExcludePatterns[0] != "*.tmp" {
		t.Error("MergeConfigs mutated override ExcludePatterns element")
	}
	if override.MaxFileSize != 200 {
		t.Error("MergeConfigs mutated override MaxFileSize")
	}
}

func TestMergeConfigs_NilOverrideReturnsDifferentPointer(t *testing.T) {
	org := &OrgConfig{ExcludePatterns: []string{"*.log"}, MaxFileSize: 100}
	result := MergeConfigs(org, nil)

	if result == org {
		t.Error("MergeConfigs with nil override returned same pointer as org")
	}
}

func TestMergeConfigs_NilOrgReturnsDifferentPointer(t *testing.T) {
	override := &OrgConfig{ExcludePatterns: []string{"*.tmp"}, MaxFileSize: 200}
	result := MergeConfigs(nil, override)

	if result == override {
		t.Error("MergeConfigs with nil org returned same pointer as override")
	}
}

// Section 3: Tests for AnalyzerName and EmbedderName fields

func TestOrgConfig_JSON_RoundTrip(t *testing.T) {
	config := OrgConfig{AnalyzerName: "python", EmbedderName: "voyage"}
	data, err := json.Marshal(config)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded OrgConfig
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.AnalyzerName != "python" {
		t.Errorf("AnalyzerName = %q, want 'python'", decoded.AnalyzerName)
	}
	if decoded.EmbedderName != "voyage" {
		t.Errorf("EmbedderName = %q, want 'voyage'", decoded.EmbedderName)
	}
}

func TestOrgConfig_OmitEmpty(t *testing.T) {
	config := OrgConfig{MaxFileSize: 1000}
	data, err := json.Marshal(config)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	s := string(data)
	if strings.Contains(s, "analyzer_name") {
		t.Errorf("JSON should not contain analyzer_name, got %s", s)
	}
	if strings.Contains(s, "embedder_name") {
		t.Errorf("JSON should not contain embedder_name, got %s", s)
	}
}

func TestMergeConfigs_AnalyzerName_PreservesOrg(t *testing.T) {
	orgCfg := &OrgConfig{AnalyzerName: "python"}
	override := &OrgConfig{}
	merged := MergeConfigs(orgCfg, override)
	if merged.AnalyzerName != "python" {
		t.Errorf("AnalyzerName = %q, want 'python'", merged.AnalyzerName)
	}
}

func TestMergeConfigs_AnalyzerName_OverrideTakesPrecedence(t *testing.T) {
	orgCfg := &OrgConfig{AnalyzerName: "python"}
	override := &OrgConfig{AnalyzerName: "typescript"}
	merged := MergeConfigs(orgCfg, override)
	if merged.AnalyzerName != "typescript" {
		t.Errorf("AnalyzerName = %q, want 'typescript'", merged.AnalyzerName)
	}
}

func TestMergeConfigs_EmbedderName_PreservesOrg(t *testing.T) {
	orgCfg := &OrgConfig{EmbedderName: "local"}
	override := &OrgConfig{}
	merged := MergeConfigs(orgCfg, override)
	if merged.EmbedderName != "local" {
		t.Errorf("EmbedderName = %q, want 'local'", merged.EmbedderName)
	}
}

func TestMergeConfigs_EmbedderName_OverrideTakesPrecedence(t *testing.T) {
	orgCfg := &OrgConfig{EmbedderName: "local"}
	override := &OrgConfig{EmbedderName: "voyage"}
	merged := MergeConfigs(orgCfg, override)
	if merged.EmbedderName != "voyage" {
		t.Errorf("EmbedderName = %q, want 'voyage'", merged.EmbedderName)
	}
}

func TestCopyConfig_CopiesNewFields(t *testing.T) {
	src := &OrgConfig{
		ExcludePatterns: []string{"*.tmp"},
		MaxFileSize:     1000,
		AnalyzerName:    "go",
		EmbedderName:    "local",
	}
	dst := copyConfig(src)
	if dst.AnalyzerName != "go" {
		t.Errorf("AnalyzerName = %q, want 'go'", dst.AnalyzerName)
	}
	if dst.EmbedderName != "local" {
		t.Errorf("EmbedderName = %q, want 'local'", dst.EmbedderName)
	}
	if dst.MaxFileSize != 1000 {
		t.Errorf("MaxFileSize = %d, want 1000", dst.MaxFileSize)
	}
	if len(dst.ExcludePatterns) != 1 || dst.ExcludePatterns[0] != "*.tmp" {
		t.Errorf("ExcludePatterns = %v, want [*.tmp]", dst.ExcludePatterns)
	}
}

func TestMergeConfigs_NilOrgPreservesOverrideFields(t *testing.T) {
	merged := MergeConfigs(nil, &OrgConfig{AnalyzerName: "python"})
	if merged.AnalyzerName != "python" {
		t.Errorf("AnalyzerName = %q, want 'python'", merged.AnalyzerName)
	}
}

func TestMergeConfigs_NilOverridePreservesOrgFields(t *testing.T) {
	merged := MergeConfigs(&OrgConfig{EmbedderName: "local"}, nil)
	if merged.EmbedderName != "local" {
		t.Errorf("EmbedderName = %q, want 'local'", merged.EmbedderName)
	}
}

func TestDeduplicateStrings(t *testing.T) {
	tests := []struct {
		name  string
		input []string
		want  []string
	}{
		{"nil input", nil, nil},
		{"empty input", []string{}, []string{}},
		{"no duplicates", []string{"a", "b", "c"}, []string{"a", "b", "c"}},
		{"with duplicates", []string{"a", "b", "a", "c", "b"}, []string{"a", "b", "c"}},
		{"all same", []string{"x", "x", "x"}, []string{"x"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := deduplicateStrings(tt.input)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("deduplicateStrings(%v) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}
