package org

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeOrgsJSON(t *testing.T, dir string, data *orgData) {
	t.Helper()
	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "_orgs.json"), b, 0644); err != nil {
		t.Fatalf("failed to write fixture: %v", err)
	}
}

func TestMigrateFromFilesystem_NoFile(t *testing.T) {
	store := newTestStore(t)
	dir := t.TempDir()

	err := MigrateFromFilesystem(dir, store)
	if err != nil {
		t.Fatalf("expected nil error for missing file, got: %v", err)
	}
}

func TestMigrateFromFilesystem_SingleOrg(t *testing.T) {
	store := newTestStore(t)
	dir := t.TempDir()

	writeOrgsJSON(t, dir, &orgData{
		Orgs: map[string]*Org{
			"org-1": {
				ID:    "org-1",
				Repos: []string{"repo-a", "repo-b"},
				Config: OrgConfig{
					ExcludePatterns: []string{"*.log"},
					MaxFileSize:     1048576,
				},
				Created: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
			},
		},
	})

	err := MigrateFromFilesystem(dir, store)
	if err != nil {
		t.Fatalf("migration failed: %v", err)
	}

	ctx := context.Background()
	got, err := store.GetOrg(ctx, "org-1")
	if err != nil {
		t.Fatalf("GetOrg failed: %v", err)
	}
	if got.ID != "org-1" {
		t.Errorf("ID = %q, want %q", got.ID, "org-1")
	}
	if len(got.Repos) != 2 {
		t.Fatalf("Repos len = %d, want 2", len(got.Repos))
	}
	if got.Repos[0] != "repo-a" || got.Repos[1] != "repo-b" {
		t.Errorf("Repos = %v, want [repo-a repo-b]", got.Repos)
	}
}

func TestMigrateFromFilesystem_MultipleOrgs(t *testing.T) {
	store := newTestStore(t)
	dir := t.TempDir()

	writeOrgsJSON(t, dir, &orgData{
		Orgs: map[string]*Org{
			"alpha": {
				ID:    "alpha",
				Repos: []string{"r1"},
				Config: OrgConfig{
					ExcludePatterns: []string{"vendor/"},
				},
			},
			"beta": {
				ID:    "beta",
				Repos: []string{"r2", "r3"},
				Config: OrgConfig{
					MaxFileSize: 2097152,
				},
			},
		},
	})

	err := MigrateFromFilesystem(dir, store)
	if err != nil {
		t.Fatalf("migration failed: %v", err)
	}

	ctx := context.Background()
	alpha, err := store.GetOrg(ctx, "alpha")
	if err != nil {
		t.Fatalf("GetOrg alpha: %v", err)
	}
	if len(alpha.Config.ExcludePatterns) != 1 || alpha.Config.ExcludePatterns[0] != "vendor/" {
		t.Errorf("alpha ExcludePatterns = %v, want [vendor/]", alpha.Config.ExcludePatterns)
	}

	beta, err := store.GetOrg(ctx, "beta")
	if err != nil {
		t.Fatalf("GetOrg beta: %v", err)
	}
	if beta.Config.MaxFileSize != 2097152 {
		t.Errorf("beta MaxFileSize = %d, want 2097152", beta.Config.MaxFileSize)
	}
	if len(beta.Repos) != 2 {
		t.Errorf("beta Repos len = %d, want 2", len(beta.Repos))
	}
}

func TestMigrateFromFilesystem_PreservesConfig(t *testing.T) {
	store := newTestStore(t)
	dir := t.TempDir()

	writeOrgsJSON(t, dir, &orgData{
		Orgs: map[string]*Org{
			"cfg-org": {
				ID: "cfg-org",
				Config: OrgConfig{
					ExcludePatterns: []string{"*.tmp", "build/"},
					MaxFileSize:     524288,
				},
			},
		},
	})

	err := MigrateFromFilesystem(dir, store)
	if err != nil {
		t.Fatalf("migration failed: %v", err)
	}

	ctx := context.Background()
	got, err := store.GetOrg(ctx, "cfg-org")
	if err != nil {
		t.Fatalf("GetOrg: %v", err)
	}
	if len(got.Config.ExcludePatterns) != 2 {
		t.Fatalf("ExcludePatterns len = %d, want 2", len(got.Config.ExcludePatterns))
	}
	if got.Config.ExcludePatterns[0] != "*.tmp" || got.Config.ExcludePatterns[1] != "build/" {
		t.Errorf("ExcludePatterns = %v, want [*.tmp build/]", got.Config.ExcludePatterns)
	}
	if got.Config.MaxFileSize != 524288 {
		t.Errorf("MaxFileSize = %d, want 524288", got.Config.MaxFileSize)
	}
}

func TestMigrateFromFilesystem_RenamesFile(t *testing.T) {
	store := newTestStore(t)
	dir := t.TempDir()

	writeOrgsJSON(t, dir, &orgData{
		Orgs: map[string]*Org{
			"x": {ID: "x"},
		},
	})

	err := MigrateFromFilesystem(dir, store)
	if err != nil {
		t.Fatalf("migration failed: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, "_orgs.json")); !os.IsNotExist(err) {
		t.Error("_orgs.json should not exist after migration")
	}
	if _, err := os.Stat(filepath.Join(dir, "_orgs.json.migrated")); err != nil {
		t.Error("_orgs.json.migrated should exist after migration")
	}
}

func TestMigrateFromFilesystem_Idempotent(t *testing.T) {
	store := newTestStore(t)
	dir := t.TempDir()

	writeOrgsJSON(t, dir, &orgData{
		Orgs: map[string]*Org{
			"idem": {
				ID:    "idem",
				Repos: []string{"r1"},
			},
		},
	})

	// First run
	if err := MigrateFromFilesystem(dir, store); err != nil {
		t.Fatalf("first migration failed: %v", err)
	}

	// Second run — _orgs.json is gone, .migrated exists, should be no-op
	if err := MigrateFromFilesystem(dir, store); err != nil {
		t.Fatalf("second migration failed: %v", err)
	}

	ctx := context.Background()
	got, err := store.GetOrg(ctx, "idem")
	if err != nil {
		t.Fatalf("GetOrg: %v", err)
	}
	if len(got.Repos) != 1 || got.Repos[0] != "r1" {
		t.Errorf("Repos = %v, want [r1]", got.Repos)
	}
}

func TestMigrateFromFilesystem_CorruptedJSON(t *testing.T) {
	store := newTestStore(t)
	dir := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, "_orgs.json"), []byte("{invalid"), 0644); err != nil {
		t.Fatal(err)
	}

	err := MigrateFromFilesystem(dir, store)
	if err == nil {
		t.Fatal("expected error for corrupted JSON")
	}

	// _orgs.json should NOT be renamed on failure
	if _, err := os.Stat(filepath.Join(dir, "_orgs.json")); err != nil {
		t.Error("_orgs.json should still exist after failed migration")
	}
}

func TestMigrateFromFilesystem_EmptyJSON(t *testing.T) {
	store := newTestStore(t)
	dir := t.TempDir()

	writeOrgsJSON(t, dir, &orgData{
		Orgs: map[string]*Org{},
	})

	err := MigrateFromFilesystem(dir, store)
	if err != nil {
		t.Fatalf("migration of empty JSON failed: %v", err)
	}

	// File should be renamed even with zero orgs
	if _, err := os.Stat(filepath.Join(dir, "_orgs.json.migrated")); err != nil {
		t.Error("_orgs.json.migrated should exist after migration")
	}
}

func TestMigrateFromFilesystem_MigratedAlreadyExists(t *testing.T) {
	store := newTestStore(t)
	dir := t.TempDir()

	// Only .migrated exists, no _orgs.json
	if err := os.WriteFile(filepath.Join(dir, "_orgs.json.migrated"), []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}

	err := MigrateFromFilesystem(dir, store)
	if err != nil {
		t.Fatalf("expected no-op when only .migrated exists, got: %v", err)
	}
}
