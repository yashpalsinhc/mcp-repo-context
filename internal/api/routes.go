package api

import (
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// setupRoutes registers all route groups on the router.
func (s *APIServer) setupRoutes() {
	r := s.router

	// Health endpoint at root level (no rate limit, no timeout)
	r.Get("/health", s.handleHealth)

	// API v1 group with timeout and rate limiting
	r.Route("/api/v1", func(r chi.Router) {
		r.Use(middleware.Timeout(30 * time.Second))
		r.Use(s.rateLimitMiddleware)

		// Status
		r.Get("/status", s.handleStatus)

		// Repos
		r.Post("/repos/analyze", s.handleAnalyzeRepo)
		r.Post("/repos/compare", s.handleCompareRepos)
		r.Get("/repos", s.handleListRepos)
		r.Get("/repos/{id}", s.handleGetRepoContext)
		r.Delete("/repos/{id}", s.handleDeleteRepo)
		r.Get("/repos/{id}/architecture", s.handleGetArchitecture)
		r.Get("/repos/{id}/files", s.handleGetFileContext)
		r.Get("/repos/{id}/search", s.handleSearchFunctions)
		r.Get("/repos/{id}/functions/{name}", s.handleGetFunctionContext)
		r.Get("/repos/{id}/callers/{name}", s.handleGetCallers)
		r.Get("/repos/{id}/concepts/{name}", s.handleSearchByConcept)
		r.Get("/repos/{id}/side-effects/{type}", s.handleSearchBySideEffect)
		r.Post("/repos/{id}/refresh", s.handleRefreshChanged)
		r.Post("/repos/{id}/refresh-file", s.handleRefreshFile)
		r.Post("/repos/{id}/pr-context", s.handleGetPRContext)

		// Orgs
		r.Post("/orgs", s.handleCreateOrg)
		r.Get("/orgs", s.handleListOrgs)
		r.Get("/orgs/{id}", s.handleGetOrg)
		r.Delete("/orgs/{id}", s.handleDeleteOrg)
		r.Post("/orgs/{id}/repos", s.handleAddRepos)
		r.Delete("/orgs/{id}/repos/{repoId}", s.handleRemoveRepo)
		r.Post("/orgs/{id}/analyze", s.handleAnalyzeOrg)

		// AI
		r.Post("/ask", s.handleAsk)
		r.Post("/search", s.handleSmartQuery)

		// Jobs
		r.Get("/jobs/{id}", s.handleGetJob)
		r.Get("/jobs", s.handleListJobs)

		// Webhooks
		r.Post("/webhooks/github", s.handleGitHubWebhook)
		r.Post("/webhooks/gitlab", s.handleGitLabWebhook)
	})
}
