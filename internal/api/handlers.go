package api

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/yashpalc/mcp-repo-context/internal/orchestrator"
)

// decodeParam URL-decodes a chi URL parameter.
func decodeParam(r *http.Request, name string) string {
	v := chi.URLParam(r, name)
	decoded, err := url.PathUnescape(v)
	if err != nil {
		return v
	}
	return decoded
}

// --- Repo handlers ---

type analyzeRepoRequest struct {
	RepoURL string `json:"repo_url"`
	Branch  string `json:"branch"`
}

func (s *APIServer) handleAnalyzeRepo(w http.ResponseWriter, r *http.Request) {
	var req analyzeRepoRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_json", "invalid JSON body")
		return
	}
	if req.RepoURL == "" {
		writeError(w, r, http.StatusBadRequest, "missing_field", "repo_url is required")
		return
	}

	// If job queue is available, enqueue async
	if s.jobQueue != nil {
		payload, _ := json.Marshal(req)
		jobID, err := s.jobQueue.Enqueue(r.Context(), "analyze_repo", "", "", string(payload))
		if err != nil {
			writeError(w, r, http.StatusInternalServerError, "queue_error", err.Error())
			return
		}
		writeJSON(w, r, http.StatusAccepted, map[string]string{"job_id": jobID}, nil)
		return
	}

	// Synchronous fallback
	result, err := s.manager.AnalyzeRepo(r.Context(), req.RepoURL, orchestrator.AnalyzeOptions{
		Branch: req.Branch,
		Force:  true,
	})
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "analyze_error", err.Error())
		return
	}
	writeJSON(w, r, http.StatusAccepted, result, nil)
}

func (s *APIServer) handleListRepos(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	limit, offset := parsePagination(r)

	repos, err := s.manager.ListRepos(r.Context())
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}

	total := len(repos)

	// Apply pagination
	if offset >= len(repos) {
		repos = nil
	} else {
		end := offset + limit
		if end > len(repos) {
			end = len(repos)
		}
		repos = repos[offset:end]
	}

	writeJSONTimed(w, r, http.StatusOK, repos, start, total, limit, offset)
}

func (s *APIServer) handleGetRepoContext(w http.ResponseWriter, r *http.Request) {
	repoID := decodeParam(r, "id")
	ctx, err := s.manager.GetContext(r.Context(), repoID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, r, http.StatusNotFound, "not_found", "repo not found: "+repoID)
			return
		}
		writeError(w, r, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	writeJSON(w, r, http.StatusOK, ctx, nil)
}

func (s *APIServer) handleGetArchitecture(w http.ResponseWriter, r *http.Request) {
	repoID := decodeParam(r, "id")
	ctx, err := s.manager.GetContext(r.Context(), repoID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, r, http.StatusNotFound, "not_found", "repo not found: "+repoID)
			return
		}
		writeError(w, r, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	writeJSON(w, r, http.StatusOK, ctx.Architecture, nil)
}

func (s *APIServer) handleDeleteRepo(w http.ResponseWriter, r *http.Request) {
	repoID := decodeParam(r, "id")
	if err := s.manager.DeleteRepoContext(r.Context(), repoID); err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, r, http.StatusNotFound, "not_found", "repo not found: "+repoID)
			return
		}
		writeError(w, r, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	writeJSON(w, r, http.StatusOK, map[string]string{"status": "deleted"}, nil)
}

func (s *APIServer) handleGetFileContext(w http.ResponseWriter, r *http.Request) {
	repoID := decodeParam(r, "id")
	filePath := r.URL.Query().Get("path")
	if filePath == "" {
		writeError(w, r, http.StatusBadRequest, "missing_param", "path query parameter is required")
		return
	}
	fc, err := s.manager.GetFileContext(r.Context(), repoID, filePath)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, r, http.StatusNotFound, "not_found", err.Error())
			return
		}
		writeError(w, r, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	writeJSON(w, r, http.StatusOK, fc, nil)
}

func (s *APIServer) handleSearchFunctions(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	repoID := decodeParam(r, "id")
	query := r.URL.Query().Get("q")
	if query == "" {
		writeError(w, r, http.StatusBadRequest, "missing_param", "q query parameter is required")
		return
	}
	limit, offset := parsePagination(r)

	results, err := s.manager.SearchFunctions(r.Context(), repoID, query)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, r, http.StatusNotFound, "not_found", err.Error())
			return
		}
		writeError(w, r, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}

	total := len(results)
	if offset >= len(results) {
		results = nil
	} else {
		end := offset + limit
		if end > len(results) {
			end = len(results)
		}
		results = results[offset:end]
	}

	writeJSONTimed(w, r, http.StatusOK, results, start, total, limit, offset)
}

func (s *APIServer) handleGetFunctionContext(w http.ResponseWriter, r *http.Request) {
	repoID := decodeParam(r, "id")
	funcName := chi.URLParam(r, "name")
	filePath := r.URL.Query().Get("file")

	fc, err := s.manager.GetFunctionContext(r.Context(), repoID, filePath, funcName)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, r, http.StatusNotFound, "not_found", err.Error())
			return
		}
		writeError(w, r, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	writeJSON(w, r, http.StatusOK, fc, nil)
}

func (s *APIServer) handleGetCallers(w http.ResponseWriter, r *http.Request) {
	repoID := decodeParam(r, "id")
	funcName := chi.URLParam(r, "name")

	// Use SearchFunctions to find the function first, then get callers via context
	repoCtx, err := s.manager.GetContext(r.Context(), repoID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, r, http.StatusNotFound, "not_found", "repo not found: "+repoID)
			return
		}
		writeError(w, r, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}

	// Find callers across all files
	var callers []map[string]interface{}
	for path, fileCtx := range repoCtx.Files {
		for _, fn := range fileCtx.Functions {
			for _, call := range fn.Calls {
				if call.Function == funcName {
					callers = append(callers, map[string]interface{}{
						"function": fn.Name,
						"file":     path,
						"line":     call.Line,
					})
				}
			}
		}
	}

	writeJSON(w, r, http.StatusOK, callers, nil)
}

func (s *APIServer) handleSearchByConcept(w http.ResponseWriter, r *http.Request) {
	repoID := decodeParam(r, "id")
	concept := chi.URLParam(r, "name")

	results, err := s.manager.SearchByConcept(r.Context(), repoID, concept)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, r, http.StatusNotFound, "not_found", err.Error())
			return
		}
		writeError(w, r, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	writeJSON(w, r, http.StatusOK, results, nil)
}

func (s *APIServer) handleSearchBySideEffect(w http.ResponseWriter, r *http.Request) {
	repoID := decodeParam(r, "id")
	effectType := chi.URLParam(r, "type")

	results, err := s.manager.SearchBySideEffect(r.Context(), repoID, effectType)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, r, http.StatusNotFound, "not_found", err.Error())
			return
		}
		writeError(w, r, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	writeJSON(w, r, http.StatusOK, results, nil)
}

func (s *APIServer) handleRefreshChanged(w http.ResponseWriter, r *http.Request) {
	repoID := decodeParam(r, "id")
	results, err := s.manager.RefreshChangedFiles(r.Context(), repoID)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	writeJSON(w, r, http.StatusOK, results, nil)
}

func (s *APIServer) handleRefreshFile(w http.ResponseWriter, r *http.Request) {
	repoID := decodeParam(r, "id")
	filePath := r.URL.Query().Get("path")
	if filePath == "" {
		writeError(w, r, http.StatusBadRequest, "missing_param", "path query parameter is required")
		return
	}

	result, err := s.manager.RefreshFile(r.Context(), repoID, filePath, orchestrator.RefreshFileOptions{})
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	writeJSON(w, r, http.StatusOK, result, nil)
}

type compareReposRequest struct {
	RepoIDs []string `json:"repo_ids"`
}

func (s *APIServer) handleCompareRepos(w http.ResponseWriter, r *http.Request) {
	var req compareReposRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_json", "invalid JSON body")
		return
	}
	if len(req.RepoIDs) < 2 {
		writeError(w, r, http.StatusBadRequest, "invalid_input", "at least 2 repo_ids required")
		return
	}
	// Compare not directly on manager, return a placeholder
	writeJSON(w, r, http.StatusOK, map[string]interface{}{
		"repo_ids": req.RepoIDs,
		"status":   "comparison not yet implemented via REST",
	}, nil)
}

type prContextRequest struct {
	ChangedFiles []orchestrator.ChangedFile `json:"changed_files"`
}

func (s *APIServer) handleGetPRContext(w http.ResponseWriter, r *http.Request) {
	repoID := decodeParam(r, "id")
	var req prContextRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_json", "invalid JSON body")
		return
	}

	result, err := s.manager.GetPRContext(r.Context(), repoID, req.ChangedFiles)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, r, http.StatusNotFound, "not_found", err.Error())
			return
		}
		writeError(w, r, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	writeJSON(w, r, http.StatusOK, result, nil)
}

// --- Org handlers ---

type createOrgRequest struct {
	ID    string   `json:"id"`
	Repos []string `json:"repos"`
}

func (s *APIServer) handleCreateOrg(w http.ResponseWriter, r *http.Request) {
	var req createOrgRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_json", "invalid JSON body")
		return
	}
	if req.ID == "" {
		writeError(w, r, http.StatusBadRequest, "missing_field", "id is required")
		return
	}

	org, err := s.orgManager.Register(r.Context(), req.ID, req.Repos, nil)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	writeJSON(w, r, http.StatusCreated, org, nil)
}

func (s *APIServer) handleListOrgs(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	limit, offset := parsePagination(r)

	orgs, err := s.orgManager.List(r.Context())
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}

	total := len(orgs)
	if offset >= len(orgs) {
		orgs = nil
	} else {
		end := offset + limit
		if end > len(orgs) {
			end = len(orgs)
		}
		orgs = orgs[offset:end]
	}

	writeJSONTimed(w, r, http.StatusOK, orgs, start, total, limit, offset)
}

func (s *APIServer) handleGetOrg(w http.ResponseWriter, r *http.Request) {
	orgID := decodeParam(r, "id")
	o, err := s.orgManager.Get(r.Context(), orgID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, r, http.StatusNotFound, "not_found", "org not found: "+orgID)
			return
		}
		writeError(w, r, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	writeJSON(w, r, http.StatusOK, o, nil)
}

func (s *APIServer) handleDeleteOrg(w http.ResponseWriter, r *http.Request) {
	orgID := decodeParam(r, "id")
	if err := s.orgManager.Delete(r.Context(), orgID); err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, r, http.StatusNotFound, "not_found", "org not found: "+orgID)
			return
		}
		writeError(w, r, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	writeJSON(w, r, http.StatusOK, map[string]string{"status": "deleted"}, nil)
}

type addReposRequest struct {
	Repos []string `json:"repos"`
}

func (s *APIServer) handleAddRepos(w http.ResponseWriter, r *http.Request) {
	orgID := decodeParam(r, "id")
	var req addReposRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_json", "invalid JSON body")
		return
	}

	if err := s.orgManager.AddRepos(r.Context(), orgID, req.Repos); err != nil {
		writeError(w, r, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	writeJSON(w, r, http.StatusOK, map[string]string{"status": "added"}, nil)
}

func (s *APIServer) handleRemoveRepo(w http.ResponseWriter, r *http.Request) {
	orgID := decodeParam(r, "id")
	repoID := chi.URLParam(r, "repoId")
	if err := s.orgManager.RemoveRepos(r.Context(), orgID, []string{repoID}); err != nil {
		writeError(w, r, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	writeJSON(w, r, http.StatusOK, map[string]string{"status": "removed"}, nil)
}

func (s *APIServer) handleAnalyzeOrg(w http.ResponseWriter, r *http.Request) {
	orgID := decodeParam(r, "id")

	if s.jobQueue != nil {
		payload, _ := json.Marshal(map[string]string{"org_id": orgID})
		jobID, err := s.jobQueue.Enqueue(r.Context(), "analyze_org", orgID, "", string(payload))
		if err != nil {
			writeError(w, r, http.StatusInternalServerError, "queue_error", err.Error())
			return
		}
		writeJSON(w, r, http.StatusAccepted, map[string]string{"job_id": jobID}, nil)
		return
	}

	// Synchronous fallback
	result, err := s.orgManager.AnalyzeOrg(r.Context(), orgID, false, 2)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	writeJSON(w, r, http.StatusAccepted, result, nil)
}

// --- AI handlers ---

type askRequest struct {
	Query   string   `json:"query"`
	RepoIDs []string `json:"repo_ids"`
}

func (s *APIServer) handleAsk(w http.ResponseWriter, r *http.Request) {
	var req askRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_json", "invalid JSON body")
		return
	}
	if req.Query == "" {
		writeError(w, r, http.StatusBadRequest, "missing_field", "query is required")
		return
	}

	result, err := s.manager.Ask(r.Context(), req.Query, req.RepoIDs)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	writeJSON(w, r, http.StatusOK, result, nil)
}

type smartQueryRequest struct {
	Query     string `json:"query"`
	ProjectID string `json:"project_id"`
}

func (s *APIServer) handleSmartQuery(w http.ResponseWriter, r *http.Request) {
	var req smartQueryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_json", "invalid JSON body")
		return
	}
	if req.Query == "" {
		writeError(w, r, http.StatusBadRequest, "missing_field", "query is required")
		return
	}

	result, err := s.manager.SmartQuery(r.Context(), req.Query, req.ProjectID)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	writeJSON(w, r, http.StatusOK, result, nil)
}

// --- Job handlers ---

func (s *APIServer) handleGetJob(w http.ResponseWriter, r *http.Request) {
	jobID := decodeParam(r, "id")
	if s.jobQueue == nil {
		writeError(w, r, http.StatusServiceUnavailable, "unavailable", "job queue not configured")
		return
	}

	job, err := s.jobQueue.GetJob(r.Context(), jobID)
	if err != nil {
		writeError(w, r, http.StatusNotFound, "not_found", "job not found: "+jobID)
		return
	}
	writeJSON(w, r, http.StatusOK, job, nil)
}

func (s *APIServer) handleListJobs(w http.ResponseWriter, r *http.Request) {
	if s.jobQueue == nil {
		writeError(w, r, http.StatusServiceUnavailable, "unavailable", "job queue not configured")
		return
	}

	start := time.Now()
	status := r.URL.Query().Get("status")
	limit, offset := parsePagination(r)

	jobs, total, err := s.jobQueue.ListJobs(r.Context(), "", status, limit, offset)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}

	writeJSONTimed(w, r, http.StatusOK, jobs, start, total, limit, offset)
}
