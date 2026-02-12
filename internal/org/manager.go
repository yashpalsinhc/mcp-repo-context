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
	if err := m.store.Save(ctx, o); err != nil {
		return nil, err
	}
	return o, nil
}

func (m *manager) List(ctx context.Context) ([]OrgWithCount, error) {
	orgs, err := m.store.List(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]OrgWithCount, len(orgs))
	for i, o := range orgs {
		result[i] = OrgWithCount{Org: o, RepoCount: len(o.Repos)}
	}
	return result, nil
}

func (m *manager) Get(ctx context.Context, orgID string) (*Org, error) {
	return m.store.Get(ctx, orgID)
}

func (m *manager) AddRepos(ctx context.Context, orgID string, repoIDs []string) error {
	o, err := m.store.Get(ctx, orgID)
	if err != nil {
		return err
	}
	o.Repos = uniqueStrings(append(o.Repos, repoIDs...))
	return m.store.Save(ctx, o)
}

func (m *manager) RemoveRepos(ctx context.Context, orgID string, repoIDs []string) error {
	o, err := m.store.Get(ctx, orgID)
	if err != nil {
		return err
	}
	removeSet := make(map[string]bool)
	for _, r := range repoIDs {
		removeSet[r] = true
	}
	var kept []string
	for _, r := range o.Repos {
		if !removeSet[r] {
			kept = append(kept, r)
		}
	}
	o.Repos = kept
	return m.store.Save(ctx, o)
}

func (m *manager) Delete(ctx context.Context, orgID string) error {
	return m.store.Delete(ctx, orgID)
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
