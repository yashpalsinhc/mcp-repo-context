package workflows

import (
	"context"
	"fmt"
	"time"

	ctxpkg "github.com/yashpalc/mcp-repo-context/internal/context"
)

// mockSearcher implements RepoSearcher for testing.
type mockSearcher struct {
	repos map[string]*ctxpkg.RepoContext
}

func newMockSearcher(repos ...*ctxpkg.RepoContext) *mockSearcher {
	m := &mockSearcher{repos: make(map[string]*ctxpkg.RepoContext)}
	for _, r := range repos {
		m.repos[r.ID] = r
	}
	return m
}

func (m *mockSearcher) GetContext(_ context.Context, repoID string) (*ctxpkg.RepoContext, error) {
	if rc, ok := m.repos[repoID]; ok {
		return rc, nil
	}
	return nil, fmt.Errorf("repo not found: %s", repoID)
}

func (m *mockSearcher) SearchFunctions(_ context.Context, repoID, query string) ([]ctxpkg.FunctionRef, error) {
	rc, ok := m.repos[repoID]
	if !ok {
		return nil, fmt.Errorf("repo not found: %s", repoID)
	}
	var refs []ctxpkg.FunctionRef
	for filePath, fc := range rc.Files {
		for _, fn := range fc.Functions {
			refs = append(refs, ctxpkg.FunctionRef{
				File:     filePath,
				Function: fn.Name,
				Line:     fn.LineStart,
				Summary:  fn.Description,
			})
		}
	}
	return refs, nil
}

func (m *mockSearcher) SearchByConcept(_ context.Context, repoID, concept string) ([]ctxpkg.FunctionRef, error) {
	rc, ok := m.repos[repoID]
	if !ok {
		return nil, fmt.Errorf("repo not found: %s", repoID)
	}
	var refs []ctxpkg.FunctionRef
	for filePath, fc := range rc.Files {
		for _, fn := range fc.Functions {
			refs = append(refs, ctxpkg.FunctionRef{
				File:     filePath,
				Function: fn.Name,
				Line:     fn.LineStart,
				Summary:  fn.Description,
			})
		}
	}
	return refs, nil
}

func (m *mockSearcher) GetCallers(_ context.Context, repoID, funcName string) ([]ctxpkg.FunctionRef, error) {
	rc, ok := m.repos[repoID]
	if !ok {
		return nil, fmt.Errorf("repo not found: %s", repoID)
	}
	var callers []ctxpkg.FunctionRef
	for filePath, fc := range rc.Files {
		for _, fn := range fc.Functions {
			for _, call := range fn.Calls {
				if call.Function == funcName {
					callers = append(callers, ctxpkg.FunctionRef{
						File:     filePath,
						Function: fn.Name,
						Line:     call.Line,
					})
				}
			}
		}
	}
	return callers, nil
}

// mockOrgResolver implements OrgResolver for testing.
type mockOrgResolver struct {
	orgs map[string][]string
}

func newMockOrgResolver(orgID string, repos []string) *mockOrgResolver {
	return &mockOrgResolver{
		orgs: map[string][]string{
			orgID: repos,
		},
	}
}

func (m *mockOrgResolver) GetOrgRepos(_ context.Context, orgID string) ([]string, error) {
	if repos, ok := m.orgs[orgID]; ok {
		return repos, nil
	}
	return nil, fmt.Errorf("org not found: %s", orgID)
}

// Interface compliance checks
var _ RepoSearcher = (*mockSearcher)(nil)
var _ OrgResolver = (*mockOrgResolver)(nil)

// suppress unused import warning for time
var _ = time.Now

// makeTestRepo creates a test repo context with given functions.
func makeTestRepo(id string, funcs map[string][]ctxpkg.FunctionDef) *ctxpkg.RepoContext {
	rc := &ctxpkg.RepoContext{
		ID:    id,
		Files: make(map[string]*ctxpkg.FileContext),
	}
	for file, fns := range funcs {
		rc.Files[file] = &ctxpkg.FileContext{
			Path:      file,
			Language:  "go",
			Functions: fns,
		}
	}
	return rc
}
