package org

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/yashpalc/mcp-repo-context/internal/storage"
)

func newBenchStore(b *testing.B) *SQLiteStore {
	b.Helper()
	db, err := sql.Open("sqlite3", ":memory:?_foreign_keys=ON")
	if err != nil {
		b.Fatalf("open db: %v", err)
	}
	b.Cleanup(func() { db.Close() })

	if _, err := storage.NewSQLiteStoreWithDB(db); err != nil {
		b.Fatalf("storage init: %v", err)
	}
	s, err := NewSQLiteStore(db)
	if err != nil {
		b.Fatalf("org store: %v", err)
	}
	return s
}

func seedBenchOrgs(b *testing.B, store *SQLiteStore, count, reposPerOrg int) {
	b.Helper()
	ctx := context.Background()
	for i := 0; i < count; i++ {
		o := &Org{
			ID:    fmt.Sprintf("org-%d", i),
			Config: OrgConfig{ExcludePatterns: []string{"*.log"}},
		}
		for j := 0; j < reposPerOrg; j++ {
			o.Repos = append(o.Repos, fmt.Sprintf("repo-%d-%d", i, j))
		}
		if err := store.SaveOrg(ctx, o); err != nil {
			b.Fatalf("seed org: %v", err)
		}
	}
}

func BenchmarkSaveOrg_1Repo(b *testing.B) {
	store := newBenchStore(b)
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		store.SaveOrg(ctx, &Org{ID: fmt.Sprintf("org-%d", i), Repos: []string{"r1"}})
	}
}

func BenchmarkSaveOrg_10Repos(b *testing.B) {
	store := newBenchStore(b)
	ctx := context.Background()
	repos := make([]string, 10)
	for i := range repos {
		repos[i] = fmt.Sprintf("r%d", i)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		store.SaveOrg(ctx, &Org{ID: fmt.Sprintf("org-%d", i), Repos: repos})
	}
}

func BenchmarkSaveOrg_100Repos(b *testing.B) {
	store := newBenchStore(b)
	ctx := context.Background()
	repos := make([]string, 100)
	for i := range repos {
		repos[i] = fmt.Sprintf("r%d", i)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		store.SaveOrg(ctx, &Org{ID: fmt.Sprintf("org-%d", i), Repos: repos})
	}
}

func BenchmarkListOrgs_10Orgs(b *testing.B) {
	store := newBenchStore(b)
	seedBenchOrgs(b, store, 10, 10)
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		store.ListOrgs(ctx)
	}
}

func BenchmarkListOrgs_50Orgs(b *testing.B) {
	store := newBenchStore(b)
	seedBenchOrgs(b, store, 50, 10)
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		store.ListOrgs(ctx)
	}
}

func BenchmarkGetOrg_SmallOrg(b *testing.B) {
	store := newBenchStore(b)
	seedBenchOrgs(b, store, 1, 5)
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		store.GetOrg(ctx, "org-0")
	}
}

func BenchmarkGetOrg_LargeOrg(b *testing.B) {
	store := newBenchStore(b)
	seedBenchOrgs(b, store, 1, 100)
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		store.GetOrg(ctx, "org-0")
	}
}

func BenchmarkAddRepos_10(b *testing.B) {
	store := newBenchStore(b)
	ctx := context.Background()
	store.SaveOrg(ctx, &Org{ID: "org-0"})
	repos := make([]string, 10)
	for i := range repos {
		repos[i] = fmt.Sprintf("bench-r%d", i)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		store.AddRepos(ctx, "org-0", repos)
	}
}

func BenchmarkAnalyzeOrg_5Repos_Concurrency3(b *testing.B) {
	store := newBenchStore(b)
	orch := newMockOrch()
	orch.latency = 20 * time.Millisecond
	m := NewManager(store, orch)
	ctx := context.Background()

	repos := make([]string, 5)
	for i := range repos {
		repos[i] = fmt.Sprintf("r%d", i)
	}
	m.Register(ctx, "bench-org", repos, nil)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.AnalyzeOrg(ctx, "bench-org", false, 3)
	}
}

func BenchmarkAnalyzeOrg_20Repos_Concurrency5(b *testing.B) {
	store := newBenchStore(b)
	orch := newMockOrch()
	orch.latency = 20 * time.Millisecond
	m := NewManager(store, orch)
	ctx := context.Background()

	repos := make([]string, 20)
	for i := range repos {
		repos[i] = fmt.Sprintf("r%d", i)
	}
	m.Register(ctx, "bench-org", repos, nil)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.AnalyzeOrg(ctx, "bench-org", false, 5)
	}
}
