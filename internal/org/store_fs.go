package org

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// FilesystemStore stores orgs in a JSON file.
// Deprecated: Use SQLiteStore instead. Kept for filesystem migration (section 05).
type FilesystemStore struct {
	path string
	mu   sync.RWMutex
}

// NewFilesystemStore creates a store that persists orgs to a JSON file.
func NewFilesystemStore(basePath string) (*FilesystemStore, error) {
	path := filepath.Join(basePath, "_orgs.json")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, fmt.Errorf("failed to create org storage dir: %w", err)
	}
	return &FilesystemStore{path: path}, nil
}

type orgData struct {
	Orgs map[string]*Org `json:"orgs"`
}

func (s *FilesystemStore) load() (*orgData, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return &orgData{Orgs: make(map[string]*Org)}, nil
		}
		return nil, err
	}
	var d orgData
	if err := json.Unmarshal(data, &d); err != nil {
		return nil, fmt.Errorf("failed to decode orgs: %w", err)
	}
	if d.Orgs == nil {
		d.Orgs = make(map[string]*Org)
	}
	return &d, nil
}

func (s *FilesystemStore) save(d *orgData) error {
	data, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0644)
}

func (s *FilesystemStore) SaveOrg(ctx context.Context, o *Org) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, err := s.load()
	if err != nil {
		return err
	}
	d.Orgs[o.ID] = o
	return s.save(d)
}

func (s *FilesystemStore) GetOrg(ctx context.Context, orgID string) (*Org, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	d, err := s.load()
	if err != nil {
		return nil, err
	}
	o, ok := d.Orgs[orgID]
	if !ok {
		return nil, ErrNotFound
	}
	return o, nil
}

func (s *FilesystemStore) ListOrgs(ctx context.Context) ([]OrgWithCount, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	d, err := s.load()
	if err != nil {
		return nil, err
	}
	result := make([]OrgWithCount, 0, len(d.Orgs))
	for _, o := range d.Orgs {
		result = append(result, OrgWithCount{Org: *o, RepoCount: len(o.Repos)})
	}
	return result, nil
}

func (s *FilesystemStore) DeleteOrg(ctx context.Context, orgID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, err := s.load()
	if err != nil {
		return err
	}
	delete(d.Orgs, orgID)
	return s.save(d)
}

func (s *FilesystemStore) AddRepos(ctx context.Context, orgID string, repoIDs []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, err := s.load()
	if err != nil {
		return err
	}
	o, ok := d.Orgs[orgID]
	if !ok {
		return ErrNotFound
	}
	o.Repos = uniqueStrings(append(o.Repos, repoIDs...))
	return s.save(d)
}

func (s *FilesystemStore) RemoveRepos(ctx context.Context, orgID string, repoIDs []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, err := s.load()
	if err != nil {
		return err
	}
	o, ok := d.Orgs[orgID]
	if !ok {
		return ErrNotFound
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
	return s.save(d)
}

func (s *FilesystemStore) GetRepoConfigOverride(_ context.Context, _, _ string) (*OrgConfig, error) {
	return nil, nil // FilesystemStore doesn't support config overrides
}

func (s *FilesystemStore) SetRepoConfigOverride(_ context.Context, _, _ string, _ *OrgConfig) error {
	return fmt.Errorf("config overrides not supported by FilesystemStore")
}

func (s *FilesystemStore) RunMigrations() error {
	return nil // No migrations for filesystem store
}
