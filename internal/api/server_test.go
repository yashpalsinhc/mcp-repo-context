package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yashpalc/mcp-repo-context/internal/logging"
)

func newTestServer(t *testing.T) *APIServer {
	t.Helper()
	logger := logging.New(bytes.NewBuffer(nil), logging.INFO, "test")
	config := DefaultAPIConfig()
	config.MaxRequestBodySize = 10 * 1024 * 1024
	return NewAPIServer(nil, nil, config, logger)
}

func TestNewAPIServer_CreatesRoutes(t *testing.T) {
	s := newTestServer(t)
	assert.NotNil(t, s.router)

	// Verify by making a request to a known route
	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code, "should respond to /health")
}

func TestMiddleware_RequestID(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	s.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	// The request_id is in the response body's meta field (set by writeJSON)
	var resp APIResponse
	err := json.NewDecoder(w.Body).Decode(&resp)
	require.NoError(t, err)
	assert.NotNil(t, resp.Meta)
	assert.NotEmpty(t, resp.Meta.RequestID, "response meta should have request_id")
}

func TestMiddleware_PanicRecovery(t *testing.T) {
	logger := logging.New(bytes.NewBuffer(nil), logging.INFO, "test")
	config := DefaultAPIConfig()
	s := NewAPIServer(nil, nil, config, logger)

	// Register a panicking route
	s.router.Get("/panic-test", func(w http.ResponseWriter, r *http.Request) {
		panic("test panic")
	})

	req := httptest.NewRequest(http.MethodGet, "/panic-test", nil)
	w := httptest.NewRecorder()

	// Should not panic
	s.router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestMiddleware_MaxBodySize(t *testing.T) {
	logger := logging.New(bytes.NewBuffer(nil), logging.INFO, "test")
	config := DefaultAPIConfig()
	config.MaxRequestBodySize = 1024 // 1KB limit

	s := NewAPIServer(nil, nil, config, logger)

	// Register a route that reads the body
	s.router.Post("/body-test", func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, 2048)
		_, err := r.Body.Read(buf)
		if err != nil {
			// MaxBytesReader returns a specific error
			http.Error(w, "body too large", http.StatusRequestEntityTooLarge)
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	// Send body > 1KB
	body := strings.Repeat("x", 2048)
	req := httptest.NewRequest(http.MethodPost, "/body-test", strings.NewReader(body))
	w := httptest.NewRecorder()

	s.router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusRequestEntityTooLarge, w.Code)
}

func TestStartAndStop_GracefulShutdown(t *testing.T) {
	logger := logging.New(bytes.NewBuffer(nil), logging.INFO, "test")
	config := DefaultAPIConfig()
	config.ListenAddr = ":0" // Random port

	s := NewAPIServer(nil, nil, config, logger)

	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() {
		errCh <- s.Start(ctx)
	}()

	// Wait for server to start
	time.Sleep(100 * time.Millisecond)

	// Cancel context to trigger shutdown
	cancel()

	select {
	case err := <-errCh:
		// nil or context.Canceled are both acceptable
		if err != nil && err != context.Canceled {
			t.Fatalf("unexpected error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("server did not shut down within 5s")
	}
}

