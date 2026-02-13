package org

// MergeConfigs merges an organization config with a per-repo override.
// Repo override values take precedence over org values where non-zero.
// Returns a new OrgConfig without mutating inputs.
func MergeConfigs(orgConfig, repoOverride *OrgConfig) *OrgConfig {
	if orgConfig == nil && repoOverride == nil {
		return nil
	}
	if repoOverride == nil {
		return copyConfig(orgConfig)
	}
	if orgConfig == nil {
		return copyConfig(repoOverride)
	}

	merged := &OrgConfig{}

	// ExcludePatterns: union of both, deduplicated
	if orgConfig.ExcludePatterns != nil || repoOverride.ExcludePatterns != nil {
		combined := make([]string, 0, len(orgConfig.ExcludePatterns)+len(repoOverride.ExcludePatterns))
		combined = append(combined, orgConfig.ExcludePatterns...)
		combined = append(combined, repoOverride.ExcludePatterns...)
		merged.ExcludePatterns = deduplicateStrings(combined)
	}

	// MaxFileSize: repo override wins when non-zero
	if repoOverride.MaxFileSize != 0 {
		merged.MaxFileSize = repoOverride.MaxFileSize
	} else {
		merged.MaxFileSize = orgConfig.MaxFileSize
	}

	return merged
}

func copyConfig(c *OrgConfig) *OrgConfig {
	if c == nil {
		return nil
	}
	cp := &OrgConfig{
		MaxFileSize: c.MaxFileSize,
	}
	if c.ExcludePatterns != nil {
		cp.ExcludePatterns = make([]string, len(c.ExcludePatterns))
		copy(cp.ExcludePatterns, c.ExcludePatterns)
	}
	return cp
}

func deduplicateStrings(items []string) []string {
	if items == nil {
		return nil
	}
	seen := make(map[string]struct{}, len(items))
	result := make([]string, 0, len(items))
	for _, item := range items {
		if _, ok := seen[item]; !ok {
			seen[item] = struct{}{}
			result = append(result, item)
		}
	}
	return result
}
