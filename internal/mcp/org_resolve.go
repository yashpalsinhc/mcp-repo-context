package mcp

import (
	"context"

	"github.com/yashpalc/mcp-repo-context/internal/org"
)

// resolveOrgConfig finds the org config for a given repoID by checking all orgs.
// Returns nil if the repo is not in any org or if OrgManager is not configured.
func (s *server) resolveOrgConfig(repoID string) *org.OrgConfig {
	if s.config.OrgManager == nil {
		return nil
	}

	ctx := context.Background()
	orgs, err := s.config.OrgManager.List(ctx)
	if err != nil {
		return nil
	}

	for _, owc := range orgs {
		o, err := s.config.OrgManager.Get(ctx, owc.ID)
		if err != nil {
			continue
		}
		for _, r := range o.Repos {
			if r == repoID {
				// Found the org containing this repo — get effective config
				cfg, err := s.config.OrgManager.GetEffectiveConfig(ctx, owc.ID, repoID)
				if err != nil {
					// Fall back to org-level config
					cfgCopy := o.Config
					return &cfgCopy
				}
				return cfg
			}
		}
	}

	return nil
}

// resolveAnalyzerName resolves the analyzer name from org config and validates it
// against the manager's analyzer registry. Returns the validated name and any warnings.
func (s *server) resolveAnalyzerName(repoID string) (string, []string) {
	orgConfig := s.resolveOrgConfig(repoID)
	if orgConfig == nil || orgConfig.AnalyzerName == "" {
		return "", nil
	}

	return orgConfig.AnalyzerName, nil
}

// resolveEmbedderForRepo resolves the embedder to use for a given repo based on org config.
// Returns the default embedder if the named embedder is not found, along with any warnings.
func (s *server) resolveEmbedderForRepo(repoID string) (string, []string) {
	orgConfig := s.resolveOrgConfig(repoID)
	if orgConfig == nil || orgConfig.EmbedderName == "" {
		return "", nil
	}

	return orgConfig.EmbedderName, nil
}
