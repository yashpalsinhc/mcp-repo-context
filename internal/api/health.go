package api

import (
	"fmt"
	"net/http"
	"time"
)

// handleHealth returns a simple health check response.
func (s *APIServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, r, http.StatusOK, map[string]string{"status": "ok"}, nil)
}

// handleStatus returns operational statistics.
func (s *APIServer) handleStatus(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	ctx := r.Context()

	reposAnalyzed := 0
	orgsCount := 0
	queueDepth := 0
	queueRunning := 0

	// Get repo count
	if repos, err := s.manager.ListRepos(ctx); err == nil {
		reposAnalyzed = len(repos)
	}

	// Get org count
	if orgs, err := s.orgManager.List(ctx); err == nil {
		orgsCount = len(orgs)
	}

	// Get queue stats
	if s.jobQueue != nil {
		pending, running := s.jobQueue.Stats(ctx)
		queueDepth = pending
		queueRunning = running
	}

	data := map[string]interface{}{
		"status":         "ok",
		"repos_analyzed": reposAnalyzed,
		"orgs":           orgsCount,
		"queue_depth":    queueDepth,
		"queue_running":  queueRunning,
		"uptime":         fmt.Sprintf("%s", time.Since(s.startTime).Round(time.Second)),
		"version":        "1.0.0",
	}

	writeJSONTimed(w, r, http.StatusOK, data, start, 0, 0, 0)
}
