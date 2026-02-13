package org

import (
	"context"
	"fmt"
	"time"
)

// Manager manages organizations.
type Manager interface {
	Register(ctx context.Context, orgID string, repoIDs []string, config *OrgConfig) (*Org, error)
	List(ctx context.Context) ([]OrgWithCount, error)
	Get(ctx context.Context, orgID string) (*Org, error)
	AddRepos(ctx context.Context, orgID string, repoIDs []string) error
	RemoveRepos(ctx context.Context, orgID string, repoIDs []string) error
	Delete(ctx context.Context, orgID string) error
	GetEffectiveConfig(ctx context.Context, orgID, repoID string) (*OrgConfig, error)
	SetRepoConfigOverride(ctx context.Context, orgID, repoID string, config *OrgConfig) error
}

// manager implements Manager.
type manager struct {
	store Store
}

// NewManager creates a new OrgManager.
func NewManager(store Store) Manager {
	return &manager{store: store}
}

func (m *manager) Register(ctx context.Context, orgID string, repoIDs []string, config *OrgConfig) (*Org, error) {
	if orgID == "" {
		return nil, fmt.Errorf("org_id is required")
	}
	cfg := OrgConfig{}
	if config != nil {
		cfg = *config
	}
	o := &Org{
		ID:      orgID,
		Repos:   uniqueStrings(repoIDs),
		Config:  cfg,
		Created: time.Now(),
	}
	if err := m.store.SaveOrg(ctx, o); err != nil {
		return nil, err
	}
	return o, nil
}

func (m *manager) List(ctx context.Context) ([]OrgWithCount, error) {
	return m.store.ListOrgs(ctx)
}

func (m *manager) Get(ctx context.Context, orgID string) (*Org, error) {
	return m.store.GetOrg(ctx, orgID)
}

func (m *manager) AddRepos(ctx context.Context, orgID string, repoIDs []string) error {
	return m.store.AddRepos(ctx, orgID, repoIDs)
}

func (m *manager) RemoveRepos(ctx context.Context, orgID string, repoIDs []string) error {
	return m.store.RemoveRepos(ctx, orgID, repoIDs)
}

func (m *manager) Delete(ctx context.Context, orgID string) error {
	return m.store.DeleteOrg(ctx, orgID)
}

func (m *manager) GetEffectiveConfig(ctx context.Context, orgID, repoID string) (*OrgConfig, error) {
	o, err := m.store.GetOrg(ctx, orgID)
	if err != nil {
		return nil, err
	}
	override, err := m.store.GetRepoConfigOverride(ctx, orgID, repoID)
	if err != nil {
		return nil, err
	}
	return MergeConfigs(&o.Config, override), nil
}

func (m *manager) SetRepoConfigOverride(ctx context.Context, orgID, repoID string, config *OrgConfig) error {
	return m.store.SetRepoConfigOverride(ctx, orgID, repoID, config)
}

func uniqueStrings(ss []string) []string {
	seen := make(map[string]bool)
	var result []string
	for _, s := range ss {
		if s != "" && !seen[s] {
			seen[s] = true
			result = append(result, s)
		}
	}
	return result
}
