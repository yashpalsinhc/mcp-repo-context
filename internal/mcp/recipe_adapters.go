package mcp

import (
	"context"
	"encoding/json"

	"github.com/yashpalc/mcp-repo-context/internal/org"
	"github.com/yashpalc/mcp-repo-context/internal/orchestrator"
	"github.com/yashpalc/mcp-repo-context/internal/recipes"
	"github.com/yashpalc/mcp-repo-context/internal/vectors"
)

// orchestratorAdapter wraps orchestrator.Manager to satisfy recipes.OrchestratorManager.
type orchestratorAdapter struct {
	mgr orchestrator.Manager
}

func (a *orchestratorAdapter) GetContext(repoID string, scope string) (interface{}, error) {
	ctx := context.Background()
	repoCtx, err := a.mgr.GetContext(ctx, repoID)
	if err != nil {
		return nil, err
	}
	// Convert to map for recipe consumption
	return toInterfaceMap(repoCtx)
}

func (a *orchestratorAdapter) ListRepos() []string {
	repos, err := a.mgr.ListRepos(context.Background())
	if err != nil {
		return nil
	}
	ids := make([]string, len(repos))
	for i, r := range repos {
		ids[i] = r.RepoID
	}
	return ids
}

func (a *orchestratorAdapter) SearchContext(query string, repoID string) ([]interface{}, error) {
	ctx := context.Background()
	results, err := a.mgr.SearchFunctions(ctx, repoID, query)
	if err != nil {
		return nil, err
	}
	items := make([]interface{}, len(results))
	for i, r := range results {
		items[i] = map[string]interface{}{
			"name":      r.Function,
			"file":      r.File,
			"file_path": r.File,
			"line":      r.Line,
			"signature": r.Signature,
			"summary":   r.Summary,
		}
	}
	return items, nil
}

func (a *orchestratorAdapter) GetFunctionContext(repoID string, filePath string, functionName string) (interface{}, error) {
	ctx := context.Background()
	result, err := a.mgr.GetFunctionContext(ctx, repoID, filePath, functionName)
	if err != nil {
		return nil, err
	}
	return toInterfaceMap(result)
}

func (a *orchestratorAdapter) GetCallers(repoID string, functionName string) ([]interface{}, error) {
	ctx := context.Background()
	// Use SearchFunctions to find callers indirectly (the real GetCallers needs a specific file)
	funcCtx, err := a.mgr.GetFunctionContext(ctx, repoID, "", functionName)
	if err != nil {
		return nil, err
	}
	callers := make([]interface{}, len(funcCtx.Callers))
	for i, c := range funcCtx.Callers {
		callers[i] = map[string]interface{}{
			"function": c.Function,
			"file":     c.File,
			"line":     c.Line,
		}
	}
	return callers, nil
}

func (a *orchestratorAdapter) GetPRContext(repoID string, changedFiles []interface{}) (interface{}, error) {
	ctx := context.Background()
	// Convert changedFiles from []interface{} to []orchestrator.ChangedFile
	var cf []orchestrator.ChangedFile
	for _, f := range changedFiles {
		if m, ok := f.(map[string]interface{}); ok {
			path, _ := m["path"].(string)
			changeType, _ := m["change_type"].(string)
			if path != "" {
				cf = append(cf, orchestrator.ChangedFile{
					Path:       path,
					ChangeType: changeType,
				})
			}
		}
	}
	result, err := a.mgr.GetPRContext(ctx, repoID, cf)
	if err != nil {
		return nil, err
	}
	return toInterfaceMap(result)
}

// orgAdapter wraps org.Manager to satisfy recipes.OrgManager.
type orgAdapter struct {
	mgr org.Manager
}

func (a *orgAdapter) GetOrg(orgID string) (interface{}, error) {
	if a.mgr == nil {
		return nil, nil
	}
	ctx := context.Background()
	o, err := a.mgr.Get(ctx, orgID)
	if err != nil {
		return nil, err
	}
	return toInterfaceMap(o)
}

func (a *orgAdapter) ListOrgs() []string {
	if a.mgr == nil {
		return nil
	}
	ctx := context.Background()
	orgs, err := a.mgr.List(ctx)
	if err != nil {
		return nil
	}
	ids := make([]string, len(orgs))
	for i, o := range orgs {
		ids[i] = o.ID
	}
	return ids
}

// vectorAdapter wraps vectors.SemanticSearch to satisfy recipes.VectorSearcher.
type vectorAdapter struct {
	ss *vectors.SemanticSearch
}

func (a *vectorAdapter) SearchFunctions(query string, repoID string, limit int) ([]recipes.SearchResult, error) {
	results, err := a.ss.SearchFunctions(context.Background(), query, repoID, limit)
	if err != nil {
		return nil, err
	}
	out := make([]recipes.SearchResult, len(results))
	for i, r := range results {
		out[i] = recipes.SearchResult{
			Name:     r.Name,
			Type:     "function",
			FilePath: r.FilePath,
			Score:    r.Similarity,
		}
	}
	return out, nil
}

func (a *vectorAdapter) SearchTypes(query string, repoID string, limit int) ([]recipes.SearchResult, error) {
	results, err := a.ss.SearchTypes(context.Background(), query, repoID, limit)
	if err != nil {
		return nil, err
	}
	out := make([]recipes.SearchResult, len(results))
	for i, r := range results {
		out[i] = recipes.SearchResult{
			Name:     r.Name,
			Type:     "type",
			FilePath: r.FilePath,
			Score:    r.Similarity,
		}
	}
	return out, nil
}

// toInterfaceMap converts any struct to map[string]interface{} via JSON round-trip.
func toInterfaceMap(v interface{}) (interface{}, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}
	return result, nil
}

