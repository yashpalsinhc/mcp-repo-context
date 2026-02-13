diff --git a/internal/org/config.go b/internal/org/config.go
new file mode 100644
index 0000000..0e3e214
--- /dev/null
+++ b/internal/org/config.go
@@ -0,0 +1,64 @@
+package org
+
+// MergeConfigs merges an organization config with a per-repo override.
+// Repo override values take precedence over org values where non-zero.
+// Returns a new OrgConfig without mutating inputs.
+func MergeConfigs(orgConfig, repoOverride *OrgConfig) *OrgConfig {
+	if orgConfig == nil && repoOverride == nil {
+		return nil
+	}
+	if repoOverride == nil {
+		return copyConfig(orgConfig)
+	}
+	if orgConfig == nil {
+		return copyConfig(repoOverride)
+	}
+
+	merged := &OrgConfig{}
+
+	// ExcludePatterns: union of both, deduplicated
+	if orgConfig.ExcludePatterns != nil || repoOverride.ExcludePatterns != nil {
+		combined := make([]string, 0, len(orgConfig.ExcludePatterns)+len(repoOverride.ExcludePatterns))
+		combined = append(combined, orgConfig.ExcludePatterns...)
+		combined = append(combined, repoOverride.ExcludePatterns...)
+		merged.ExcludePatterns = deduplicateStrings(combined)
+	}
+
+	// MaxFileSize: repo override wins when non-zero
+	if repoOverride.MaxFileSize != 0 {
+		merged.MaxFileSize = repoOverride.MaxFileSize
+	} else {
+		merged.MaxFileSize = orgConfig.MaxFileSize
+	}
+
+	return merged
+}
+
+func copyConfig(c *OrgConfig) *OrgConfig {
+	if c == nil {
+		return nil
+	}
+	cp := &OrgConfig{
+		MaxFileSize: c.MaxFileSize,
+	}
+	if c.ExcludePatterns != nil {
+		cp.ExcludePatterns = make([]string, len(c.ExcludePatterns))
+		copy(cp.ExcludePatterns, c.ExcludePatterns)
+	}
+	return cp
+}
+
+func deduplicateStrings(items []string) []string {
+	if items == nil {
+		return nil
+	}
+	seen := make(map[string]struct{}, len(items))
+	result := make([]string, 0, len(items))
+	for _, item := range items {
+		if _, ok := seen[item]; !ok {
+			seen[item] = struct{}{}
+			result = append(result, item)
+		}
+	}
+	return result
+}
diff --git a/internal/org/config_test.go b/internal/org/config_test.go
new file mode 100644
index 0000000..01397a8
--- /dev/null
+++ b/internal/org/config_test.go
@@ -0,0 +1,136 @@
+package org
+
+import (
+	"reflect"
+	"testing"
+)
+
+func TestMergeConfigs(t *testing.T) {
+	tests := []struct {
+		name     string
+		org      *OrgConfig
+		override *OrgConfig
+		want     *OrgConfig
+	}{
+		{
+			name:     "both nil returns nil",
+			org:      nil,
+			override: nil,
+			want:     nil,
+		},
+		{
+			name:     "nil override returns copy of org config",
+			org:      &OrgConfig{ExcludePatterns: []string{"*.log"}, MaxFileSize: 100},
+			override: nil,
+			want:     &OrgConfig{ExcludePatterns: []string{"*.log"}, MaxFileSize: 100},
+		},
+		{
+			name:     "nil org config returns copy of override",
+			org:      nil,
+			override: &OrgConfig{ExcludePatterns: []string{"*.tmp"}, MaxFileSize: 200},
+			want:     &OrgConfig{ExcludePatterns: []string{"*.tmp"}, MaxFileSize: 200},
+		},
+		{
+			name:     "ExcludePatterns are unioned",
+			org:      &OrgConfig{ExcludePatterns: []string{"*.log"}},
+			override: &OrgConfig{ExcludePatterns: []string{"*.tmp"}},
+			want:     &OrgConfig{ExcludePatterns: []string{"*.log", "*.tmp"}},
+		},
+		{
+			name:     "ExcludePatterns overlapping entries are deduplicated",
+			org:      &OrgConfig{ExcludePatterns: []string{"*.log", "*.tmp"}},
+			override: &OrgConfig{ExcludePatterns: []string{"*.tmp", "*.bak"}},
+			want:     &OrgConfig{ExcludePatterns: []string{"*.log", "*.tmp", "*.bak"}},
+		},
+		{
+			name:     "MaxFileSize repo override wins when non-zero",
+			org:      &OrgConfig{MaxFileSize: 100},
+			override: &OrgConfig{MaxFileSize: 200},
+			want:     &OrgConfig{MaxFileSize: 200},
+		},
+		{
+			name:     "MaxFileSize org value used when override is zero",
+			org:      &OrgConfig{MaxFileSize: 100},
+			override: &OrgConfig{MaxFileSize: 0},
+			want:     &OrgConfig{MaxFileSize: 100},
+		},
+		{
+			name:     "empty ExcludePatterns on both returns empty slice",
+			org:      &OrgConfig{ExcludePatterns: []string{}},
+			override: &OrgConfig{ExcludePatterns: []string{}},
+			want:     &OrgConfig{ExcludePatterns: []string{}},
+		},
+		{
+			name:     "one empty one populated ExcludePatterns returns populated",
+			org:      &OrgConfig{ExcludePatterns: []string{}},
+			override: &OrgConfig{ExcludePatterns: []string{"*.tmp"}},
+			want:     &OrgConfig{ExcludePatterns: []string{"*.tmp"}},
+		},
+	}
+
+	for _, tt := range tests {
+		t.Run(tt.name, func(t *testing.T) {
+			got := MergeConfigs(tt.org, tt.override)
+			if !reflect.DeepEqual(got, tt.want) {
+				t.Errorf("MergeConfigs() = %+v, want %+v", got, tt.want)
+			}
+		})
+	}
+}
+
+func TestMergeConfigs_DoesNotMutateInputs(t *testing.T) {
+	org := &OrgConfig{ExcludePatterns: []string{"*.log"}, MaxFileSize: 100}
+	override := &OrgConfig{ExcludePatterns: []string{"*.tmp"}, MaxFileSize: 200}
+
+	result := MergeConfigs(org, override)
+
+	// Modify result
+	result.ExcludePatterns = append(result.ExcludePatterns, "*.bak")
+	result.MaxFileSize = 999
+
+	// Originals must be unchanged
+	if len(org.ExcludePatterns) != 1 || org.ExcludePatterns[0] != "*.log" {
+		t.Error("MergeConfigs mutated org ExcludePatterns")
+	}
+	if org.MaxFileSize != 100 {
+		t.Error("MergeConfigs mutated org MaxFileSize")
+	}
+	if len(override.ExcludePatterns) != 1 || override.ExcludePatterns[0] != "*.tmp" {
+		t.Error("MergeConfigs mutated override ExcludePatterns")
+	}
+	if override.MaxFileSize != 200 {
+		t.Error("MergeConfigs mutated override MaxFileSize")
+	}
+}
+
+func TestMergeConfigs_NilOverrideReturnsDifferentPointer(t *testing.T) {
+	org := &OrgConfig{ExcludePatterns: []string{"*.log"}, MaxFileSize: 100}
+	result := MergeConfigs(org, nil)
+
+	if result == org {
+		t.Error("MergeConfigs with nil override returned same pointer as org")
+	}
+}
+
+func TestDeduplicateStrings(t *testing.T) {
+	tests := []struct {
+		name  string
+		input []string
+		want  []string
+	}{
+		{"nil input", nil, nil},
+		{"empty input", []string{}, []string{}},
+		{"no duplicates", []string{"a", "b", "c"}, []string{"a", "b", "c"}},
+		{"with duplicates", []string{"a", "b", "a", "c", "b"}, []string{"a", "b", "c"}},
+		{"all same", []string{"x", "x", "x"}, []string{"x"}},
+	}
+
+	for _, tt := range tests {
+		t.Run(tt.name, func(t *testing.T) {
+			got := deduplicateStrings(tt.input)
+			if !reflect.DeepEqual(got, tt.want) {
+				t.Errorf("deduplicateStrings(%v) = %v, want %v", tt.input, got, tt.want)
+			}
+		})
+	}
+}
