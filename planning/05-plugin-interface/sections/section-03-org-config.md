# Section 3: OrgConfig Extension

## Overview

Add AnalyzerName and EmbedderName fields to OrgConfig for per-org plugin selection. Update MergeConfigs and copyConfig to handle the new fields.

## Dependencies

None — parallel with Sections 1 and 2.

## Tests First

### File: `internal/org/config_test.go` (extend existing)

```
Test: OrgConfig AnalyzerName JSON round-trip
- config := OrgConfig{AnalyzerName: "python", EmbedderName: "voyage"}
- Marshal to JSON
- Unmarshal back
- Assert AnalyzerName == "python"
- Assert EmbedderName == "voyage"

Test: OrgConfig omitempty for new fields
- config := OrgConfig{MaxFileSize: 1000}
- Marshal to JSON
- Assert JSON does NOT contain "analyzer_name"
- Assert JSON does NOT contain "embedder_name"

Test: MergeConfigs preserves AnalyzerName from org
- orgConfig := &OrgConfig{AnalyzerName: "python"}
- repoOverride := &OrgConfig{}
- merged := MergeConfigs(orgConfig, repoOverride)
- Assert merged.AnalyzerName == "python"

Test: MergeConfigs repo override takes precedence
- orgConfig := &OrgConfig{AnalyzerName: "python"}
- repoOverride := &OrgConfig{AnalyzerName: "typescript"}
- merged := MergeConfigs(orgConfig, repoOverride)
- Assert merged.AnalyzerName == "typescript"

Test: MergeConfigs preserves EmbedderName from org
- orgConfig := &OrgConfig{EmbedderName: "local"}
- repoOverride := &OrgConfig{}
- merged := MergeConfigs(orgConfig, repoOverride)
- Assert merged.EmbedderName == "local"

Test: MergeConfigs EmbedderName override
- orgConfig := &OrgConfig{EmbedderName: "local"}
- repoOverride := &OrgConfig{EmbedderName: "voyage"}
- merged := MergeConfigs(orgConfig, repoOverride)
- Assert merged.EmbedderName == "voyage"

Test: copyConfig copies all fields including new ones
- src := OrgConfig{
    ExcludePatterns: []string{"*.tmp"},
    MaxFileSize: 1000,
    AnalyzerName: "go",
    EmbedderName: "local",
  }
- dst := copyConfig(&src)
- Assert dst.AnalyzerName == "go"
- Assert dst.EmbedderName == "local"
- Assert dst.MaxFileSize == 1000
- Assert dst.ExcludePatterns has "*.tmp"

Test: MergeConfigs nil orgConfig preserves override fields
- merged := MergeConfigs(nil, &OrgConfig{AnalyzerName: "python"})
- Assert merged.AnalyzerName == "python"

Test: MergeConfigs nil repoOverride preserves org fields
- merged := MergeConfigs(&OrgConfig{EmbedderName: "local"}, nil)
- Assert merged.EmbedderName == "local"
```

## Implementation Details

### 1. Add Fields to OrgConfig

**File: `internal/org/types.go`**

Add two fields to OrgConfig struct:

```go
type OrgConfig struct {
    ExcludePatterns []string `json:"exclude_patterns,omitempty"`
    MaxFileSize     int64    `json:"max_file_size,omitempty"`
    AnalyzerName    string   `json:"analyzer_name,omitempty"`
    EmbedderName    string   `json:"embedder_name,omitempty"`
}
```

Both use `omitempty` — empty string means "use default".

### 2. Update copyConfig

**File: `internal/org/config.go`**

The `copyConfig` helper must copy the new fields. Currently it copies ExcludePatterns and MaxFileSize. Add:

```go
func copyConfig(src *OrgConfig) OrgConfig {
    dst := OrgConfig{
        MaxFileSize:  src.MaxFileSize,
        AnalyzerName: src.AnalyzerName,
        EmbedderName: src.EmbedderName,
    }
    if src.ExcludePatterns != nil {
        dst.ExcludePatterns = make([]string, len(src.ExcludePatterns))
        copy(dst.ExcludePatterns, src.ExcludePatterns)
    }
    return dst
}
```

### 3. Update MergeConfigs

MergeConfigs already handles the override pattern for existing fields. For the new string fields, the merge logic is:
- If repoOverride has non-empty AnalyzerName, use it
- Else, use orgConfig's AnalyzerName
- Same for EmbedderName

This follows the same precedence pattern as MaxFileSize.

### 4. No Validation at Save Time

Validation of analyzer/embedder names against the registry happens at use time (in the MCP server layer), not at config save time. This is because:
- The org store doesn't have access to the analyzer/embedder registries
- Names may be valid in one server configuration but not another
- It's better to surface warnings at analysis time when the user can see them

## Error Handling

- No errors introduced by this section — fields are strings with no constraints
- Empty string means "use default" — always valid

## File Summary

| File | Action |
|------|--------|
| `internal/org/types.go` | Modify: add AnalyzerName, EmbedderName to OrgConfig |
| `internal/org/config.go` | Modify: update copyConfig and MergeConfigs |
| `internal/org/config_test.go` | Modify: add tests for new fields |
