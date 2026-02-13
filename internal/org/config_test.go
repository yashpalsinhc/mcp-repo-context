package org

import (
	"reflect"
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
