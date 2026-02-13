# Section 02: Config Inheritance

## Overview

Implement pure config merge logic in `internal/org/config.go`. This section has no database dependency — it's a standalone pure function that merges organization-level config with per-repo overrides.

## Prerequisites

- Understanding of existing `OrgConfig` type in `internal/org/types.go`:
  ```go
  type OrgConfig struct {
      ExcludePatterns []string
      MaxFileSize     int64
  }
  ```

## What to Build

### 1. MergeConfigs Function (config.go)

Create `internal/org/config.go` with a pure function:

```go
func MergeConfigs(orgConfig, repoOverride *OrgConfig) *OrgConfig
```

**Merge rules:**
- If `repoOverride` is nil → return a copy of `orgConfig`
- If `orgConfig` is nil → return a copy of `repoOverride`
- If both nil → return nil (caller uses system defaults)
- `ExcludePatterns`: union of both lists, deduplicated. Repo patterns are added to org patterns.
- `MaxFileSize`: repo value wins if non-zero. If repo value is 0, org value is used.

**Critical constraint:** Must NOT mutate input configs. Always return a new `*OrgConfig`.

### 2. Helper: deduplicateStrings

Internal helper to deduplicate string slices while preserving order:

```go
func deduplicateStrings(items []string) []string
```

## Tests to Write First

**In config_test.go — table-driven:**

```go
// Test: MergeConfigs with nil override returns copy of org config (not same pointer)
// Test: MergeConfigs with nil org config returns copy of override
// Test: MergeConfigs with both nil returns nil
// Test: MergeConfigs ExcludePatterns are unioned — org has ["*.log"], repo has ["*.tmp"] → result has both
// Test: MergeConfigs ExcludePatterns with overlapping entries are deduplicated
// Test: MergeConfigs MaxFileSize — repo override wins when non-zero (repo=200, org=100 → 200)
// Test: MergeConfigs MaxFileSize — org value used when override is zero (repo=0, org=100 → 100)
// Test: MergeConfigs does not mutate input configs — modify result, verify originals unchanged
// Test: MergeConfigs with empty ExcludePatterns on both returns empty slice (not nil)
// Test: MergeConfigs with one empty and one populated ExcludePatterns returns populated
```

**Test structure:**
```go
func TestMergeConfigs(t *testing.T) {
    tests := []struct {
        name     string
        org      *OrgConfig
        override *OrgConfig
        want     *OrgConfig
    }{
        // ... table entries
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // ...
        })
    }
}
```

## Files to Create/Modify

| File | Action | Purpose |
|------|--------|---------|
| `internal/org/config.go` | Create | MergeConfigs function + deduplicateStrings helper |
| `internal/org/config_test.go` | Create | Table-driven tests for merge logic |

## Acceptance Criteria

- [ ] `MergeConfigs` handles all nil/empty combinations correctly
- [ ] ExcludePatterns are unioned and deduplicated
- [ ] MaxFileSize override behavior works (non-zero wins)
- [ ] Input configs are never mutated
- [ ] All tests pass
