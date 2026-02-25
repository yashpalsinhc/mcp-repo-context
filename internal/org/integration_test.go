package org

import (
	"context"
	"testing"
)

// createTestManager creates a Manager with SQLite store and mock orchestrator.
func createTestManager(t *testing.T) (Manager, *mockOrch) {
	t.Helper()
	store := newTestStore(t)
	orch := newMockOrch()
	m := NewManager(store, orch)
	return m, orch
}

func TestIntegration_RegisterGetRoundtrip(t *testing.T) {
	m, _ := createTestManager(t)
	ctx := context.Background()

	config := &OrgConfig{
		ExcludePatterns: []string{"*.log", "vendor/"},
		MaxFileSize:     1048576,
	}
	repos := []string{"repo-1", "repo-2", "repo-3", "repo-4", "repo-5"}

	o, err := m.Register(ctx, "test-org", repos, config)
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if o.ID != "test-org" {
		t.Errorf("ID = %q, want %q", o.ID, "test-org")
	}

	got, err := m.Get(ctx, "test-org")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(got.Repos) != 5 {
		t.Errorf("Repos = %d, want 5", len(got.Repos))
	}
	if len(got.Config.ExcludePatterns) != 2 {
		t.Errorf("ExcludePatterns = %v, want [*.log vendor/]", got.Config.ExcludePatterns)
	}
	if got.Config.MaxFileSize != 1048576 {
		t.Errorf("MaxFileSize = %d, want 1048576", got.Config.MaxFileSize)
	}
}

func TestIntegration_AddRepos(t *testing.T) {
	m, _ := createTestManager(t)
	ctx := context.Background()

	m.Register(ctx, "org-1", []string{"a", "b", "c"}, nil)

	if err := m.AddRepos(ctx, "org-1", []string{"d", "e"}); err != nil {
		t.Fatalf("AddRepos: %v", err)
	}

	got, _ := m.Get(ctx, "org-1")
	if len(got.Repos) != 5 {
		t.Errorf("Repos = %d, want 5", len(got.Repos))
	}

	// Idempotent: adding duplicate should not increase count
	if err := m.AddRepos(ctx, "org-1", []string{"a", "d"}); err != nil {
		t.Fatalf("AddRepos (dup): %v", err)
	}
	got, _ = m.Get(ctx, "org-1")
	if len(got.Repos) != 5 {
		t.Errorf("Repos after dup = %d, want 5", len(got.Repos))
	}
}

func TestIntegration_RemoveRepos(t *testing.T) {
	m, _ := createTestManager(t)
	ctx := context.Background()

	m.Register(ctx, "org-1", []string{"a", "b", "c", "d", "e"}, nil)

	if err := m.RemoveRepos(ctx, "org-1", []string{"b", "d"}); err != nil {
		t.Fatalf("RemoveRepos: %v", err)
	}

	got, _ := m.Get(ctx, "org-1")
	if len(got.Repos) != 3 {
		t.Errorf("Repos = %d, want 3", len(got.Repos))
	}

	// Remove non-existent repo - no error
	if err := m.RemoveRepos(ctx, "org-1", []string{"nonexistent"}); err != nil {
		t.Fatalf("RemoveRepos (nonexistent): %v", err)
	}
	got, _ = m.Get(ctx, "org-1")
	if len(got.Repos) != 3 {
		t.Errorf("Repos after nonexistent remove = %d, want 3", len(got.Repos))
	}
}

func TestIntegration_DeleteDetach(t *testing.T) {
	m, orch := createTestManager(t)
	ctx := context.Background()

	m.Register(ctx, "org-1", []string{"r1", "r2"}, nil)

	if err := m.Delete(ctx, "org-1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	_, err := m.Get(ctx, "org-1")
	if err != ErrNotFound {
		t.Errorf("Get after delete = %v, want ErrNotFound", err)
	}

	// Repos not deleted from orchestrator in detach mode
	if orch.callCounts["r1"] != 0 || orch.callCounts["r2"] != 0 {
		t.Error("Expected no orchestrator calls in detach mode")
	}
}

func TestIntegration_ConfigInheritance(t *testing.T) {
	m, _ := createTestManager(t)
	ctx := context.Background()

	config := &OrgConfig{
		ExcludePatterns: []string{"*.log"},
		MaxFileSize:     1048576,
	}
	m.Register(ctx, "org-1", []string{"r1", "r2"}, config)

	// Set override for r1
	override := &OrgConfig{
		ExcludePatterns: []string{"*.tmp"},
		MaxFileSize:     2097152,
	}
	if err := m.SetRepoConfigOverride(ctx, "org-1", "r1", override); err != nil {
		t.Fatalf("SetRepoConfigOverride: %v", err)
	}

	// Repo with override
	eff, err := m.GetEffectiveConfig(ctx, "org-1", "r1")
	if err != nil {
		t.Fatalf("GetEffectiveConfig (r1): %v", err)
	}
	// MergeConfigs unions ExcludePatterns
	if len(eff.ExcludePatterns) != 2 {
		t.Errorf("r1 ExcludePatterns = %v, want 2 entries", eff.ExcludePatterns)
	}
	if eff.MaxFileSize != 2097152 {
		t.Errorf("r1 MaxFileSize = %d, want 2097152 (override)", eff.MaxFileSize)
	}

	// Repo without override - gets org config
	eff2, err := m.GetEffectiveConfig(ctx, "org-1", "r2")
	if err != nil {
		t.Fatalf("GetEffectiveConfig (r2): %v", err)
	}
	if eff2.MaxFileSize != 1048576 {
		t.Errorf("r2 MaxFileSize = %d, want 1048576 (org default)", eff2.MaxFileSize)
	}
}

func TestIntegration_AnalyzeOrg(t *testing.T) {
	m, orch := createTestManager(t)
	ctx := context.Background()

	repos := []string{"r1", "r2", "r3", "r4", "r5"}
	m.Register(ctx, "org-1", repos, nil)

	// r3 fails once then succeeds (retry)
	orch.failRepos["r3"] = 1

	result, err := m.AnalyzeOrg(ctx, "org-1", false, 2)
	if err != nil {
		t.Fatalf("AnalyzeOrg: %v", err)
	}

	if result.Succeeded != 5 {
		t.Errorf("Succeeded = %d, want 5", result.Succeeded)
	}
	if result.Failed != 0 {
		t.Errorf("Failed = %d, want 0", result.Failed)
	}
	if result.Total != 5 {
		t.Errorf("Total = %d, want 5", result.Total)
	}
}

func TestIntegration_ListMultipleOrgs(t *testing.T) {
	m, _ := createTestManager(t)
	ctx := context.Background()

	m.Register(ctx, "org-a", []string{"r1", "r2"}, nil)
	m.Register(ctx, "org-b", []string{"r1", "r2", "r3", "r4", "r5"}, nil)
	m.Register(ctx, "org-c", make([]string, 10), nil) // 10 empty-string repos get filtered

	orgs, err := m.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(orgs) != 3 {
		t.Errorf("List = %d orgs, want 3", len(orgs))
	}

	// Delete one and re-list
	m.Delete(ctx, "org-b")
	orgs, _ = m.List(ctx)
	if len(orgs) != 2 {
		t.Errorf("List after delete = %d orgs, want 2", len(orgs))
	}
}
