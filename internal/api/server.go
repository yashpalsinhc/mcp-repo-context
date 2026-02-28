package api

import (
	"context"
	"net"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/yashpalc/mcp-repo-context/internal/logging"
	"github.com/yashpalc/mcp-repo-context/internal/orchestrator"
	"github.com/yashpalc/mcp-repo-context/internal/org"
	"github.com/yashpalc/mcp-repo-context/internal/queue"
)

// APIServer is the HTTP server for the REST API.
type APIServer struct {
	router     chi.Router
	manager    orchestrator.Manager
	orgManager org.Manager
	jobQueue   *queue.JobQueue
	config     *APIConfig
	logger     *logging.Logger
	startTime  time.Time
	rateLimiter *rateLimiter
}

// NewAPIServer creates a new API server with the given dependencies.
func NewAPIServer(manager orchestrator.Manager, orgManager org.Manager, config *APIConfig, logger *logging.Logger) *APIServer {
	if config == nil {
		config = DefaultAPIConfig()
	}
	if logger == nil {
		logger = logging.Default()
	}

	s := &APIServer{
		router:     chi.NewRouter(),
		manager:    manager,
		orgManager: orgManager,
		config:     config,
		logger:     logger,
		startTime:  time.Now(),
		rateLimiter: newRateLimiter(config.RateLimitPerMinute),
	}

	// Middleware stack (order matters)
	s.router.Use(middleware.RequestID)
	s.router.Use(middleware.RealIP)
	s.router.Use(jsonLoggerMiddleware(logger))
	s.router.Use(middleware.Recoverer)
	s.router.Use(maxBodySizeMiddleware(config.MaxRequestBodySize))
	s.router.Use(corsMiddleware)

	s.setupRoutes()

	return s
}

// SetJobQueue sets the job queue on the server (called after queue is created).
func (s *APIServer) SetJobQueue(jq *queue.JobQueue) {
	s.jobQueue = jq
}

// Router returns the underlying chi router (useful for testing).
func (s *APIServer) Router() chi.Router {
	return s.router
}

// Start starts the HTTP server and blocks until the context is cancelled.
func (s *APIServer) Start(ctx context.Context) error {
	srv := &http.Server{
		Addr:         s.config.ListenAddr,
		Handler:      s.router,
		ReadTimeout:  s.config.ReadTimeout,
		WriteTimeout: s.config.WriteTimeout,
		IdleTimeout:  s.config.IdleTimeout,
	}

	// Start rate limiter cleanup
	go s.rateLimiter.cleanup(ctx)

	// Start listening
	ln, err := net.Listen("tcp", srv.Addr)
	if err != nil {
		return err
	}

	errCh := make(chan error, 1)
	go func() {
		s.logger.Info("HTTP server listening on %s", ln.Addr().String())
		if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
		close(errCh)
	}()

	// Wait for context cancellation
	select {
	case <-ctx.Done():
		s.logger.Info("Shutting down HTTP server...")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	case err := <-errCh:
		return err
	}
}
