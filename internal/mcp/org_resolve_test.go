package mcp

import (
	"context"
	"testing"

	"github.com/yashpalc/mcp-repo-context/internal/org"
)

// mockOrgManager implements org.Manager for testing org resolution.
type mockOrgManager struct {
	orgs map[string]*org.Org
}

func newMockOrgManager() *mockOrgManager {
	return &mockOrgManager{orgs: make(map[string]*org.Org)}
}

func (m *mockOrgManager) Register(_ context.Context, orgID string, repoIDs []string, config *org.OrgConfig) (*org.Org, error) {
	cfg := org.OrgConfig{}
	if config != nil {
		cfg = *config
	}
	o := &org.Org{
		ID:     orgID,
		Repos:  repoIDs,
		Config: cfg,
	}
	m.orgs[orgID] = o
	return o, nil
}

func (m *mockOrgManager) List(_ context.Context) ([]org.OrgWithCount, error) {
	var result []org.OrgWithCount
	for _, o := range m.orgs {
		result = append(result, org.OrgWithCount{Org: *o, RepoCount: len(o.Repos)})
	}
	return result, nil
}

func (m *mockOrgManager) Get(_ context.Context, orgID string) (*org.Org, error) {
	o, ok := m.orgs[orgID]
	if !ok {
		return nil, org.ErrNotFound
	}
	return o, nil
}

func (m *mockOrgManager) AddRepos(_ context.Context, orgID string, repoIDs []string) error {
	o, ok := m.orgs[orgID]
	if !ok {
		return org.ErrNotFound
	}
	o.Repos = append(o.Repos, repoIDs...)
	return nil
}

func (m *mockOrgManager) RemoveRepos(_ context.Context, _ string, _ []string) error { return nil }
func (m *mockOrgManager) Delete(_ context.Context, _ string) error                  { return nil }
func (m *mockOrgManager) GetEffectiveConfig(_ context.Context, orgID, _ string) (*org.OrgConfig, error) {
	o, ok := m.orgs[orgID]
	if !ok {
		return nil, org.ErrNotFound
	}
	cfg := o.Config
	return &cfg, nil
}
func (m *mockOrgManager) SetRepoConfigOverride(_ context.Context, _, _ string, _ *org.OrgConfig) error {
	return nil
}
func (m *mockOrgManager) AnalyzeOrg(_ context.Context, _ string, _ bool, _ int) (*org.AnalysisResult, error) {
	return nil, nil
}
func (m *mockOrgManager) IndexOrg(_ context.Context, _ string, _ bool, _ int) (*org.IndexOrgResult, error) {
	return nil, nil
}

func TestResolveOrgConfig_RepoInOrg(t *testing.T) {
	mgr := newMockOrgManager()
	mgr.Register(context.Background(), "test-org", []string{"repo-1", "repo-2"},
		&org.OrgConfig{AnalyzerName: "python"})

	s := &server{
		config: &ServerConfig{OrgManager: mgr},
	}

	cfg := s.resolveOrgConfig("repo-1")
	if cfg == nil {
		t.Fatal("expected non-nil config for repo in org")
	}
	if cfg.AnalyzerName != "python" {
		t.Errorf("AnalyzerName = %q, want 'python'", cfg.AnalyzerName)
	}
}

func TestResolveOrgConfig_RepoNotInOrg(t *testing.T) {
	mgr := newMockOrgManager()
	mgr.Register(context.Background(), "test-org", []string{"repo-1"},
		&org.OrgConfig{AnalyzerName: "python"})

	s := &server{
		config: &ServerConfig{OrgManager: mgr},
	}

	cfg := s.resolveOrgConfig("unknown-repo")
	if cfg != nil {
		t.Errorf("expected nil config for repo not in any org, got %+v", cfg)
	}
}

func TestResolveOrgConfig_NoOrgManager(t *testing.T) {
	s := &server{
		config: &ServerConfig{OrgManager: nil},
	}

	cfg := s.resolveOrgConfig("any-repo")
	if cfg != nil {
		t.Errorf("expected nil config when OrgManager is nil, got %+v", cfg)
	}
}

func TestResolveAnalyzerName_FromOrg(t *testing.T) {
	mgr := newMockOrgManager()
	mgr.Register(context.Background(), "test-org", []string{"repo-1"},
		&org.OrgConfig{AnalyzerName: "custom-analyzer"})

	s := &server{
		config: &ServerConfig{OrgManager: mgr},
	}

	name, _ := s.resolveAnalyzerName("repo-1")
	if name != "custom-analyzer" {
		t.Errorf("expected 'custom-analyzer', got %q", name)
	}
}

func TestResolveAnalyzerName_NoOrg(t *testing.T) {
	mgr := newMockOrgManager()

	s := &server{
		config: &ServerConfig{OrgManager: mgr},
	}

	name, _ := s.resolveAnalyzerName("repo-1")
	if name != "" {
		t.Errorf("expected empty analyzer name for repo not in org, got %q", name)
	}
}

func TestResolveEmbedderForRepo_FromOrg(t *testing.T) {
	mgr := newMockOrgManager()
	mgr.Register(context.Background(), "test-org", []string{"repo-1"},
		&org.OrgConfig{EmbedderName: "custom-embedder"})

	s := &server{
		config: &ServerConfig{OrgManager: mgr},
	}

	name, _ := s.resolveEmbedderForRepo("repo-1")
	if name != "custom-embedder" {
		t.Errorf("expected 'custom-embedder', got %q", name)
	}
}

func TestResolveEmbedderForRepo_NoOrg(t *testing.T) {
	mgr := newMockOrgManager()

	s := &server{
		config: &ServerConfig{OrgManager: mgr},
	}

	name, _ := s.resolveEmbedderForRepo("unknown")
	if name != "" {
		t.Errorf("expected empty embedder name, got %q", name)
	}
}
