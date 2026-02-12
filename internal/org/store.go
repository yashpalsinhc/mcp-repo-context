package org

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// Store manages org persistence.
type Store interface {
	Save(ctx context.Context, o *Org) error
	Get(ctx context.Context, orgID string) (*Org, error)
	List(ctx context.Context) ([]Org, error)
	Delete(ctx context.Context, orgID string) error
}

// FilesystemStore stores orgs in a JSON file.
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

func (s *FilesystemStore) Save(ctx context.Context, o *Org) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, err := s.load()
	if err != nil {
		return err
	}
	d.Orgs[o.ID] = o
	return s.save(d)
}

func (s *FilesystemStore) Get(ctx context.Context, orgID string) (*Org, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	d, err := s.load()
	if err != nil {
		return nil, err
	}
	o, ok := d.Orgs[orgID]
	if !ok {
		return nil, fmt.Errorf("org not found: %s", orgID)
	}
	return o, nil
}

func (s *FilesystemStore) List(ctx context.Context) ([]Org, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	d, err := s.load()
	if err != nil {
		return nil, err
	}
	orgs := make([]Org, 0, len(d.Orgs))
	for _, o := range d.Orgs {
		orgs = append(orgs, *o)
	}
	return orgs, nil
}

func (s *FilesystemStore) Delete(ctx context.Context, orgID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, err := s.load()
	if err != nil {
		return err
	}
	delete(d.Orgs, orgID)
	return s.save(d)
}
