package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/yashpalc/mcp-repo-context/internal/logging"
)

// maxBodySizeMiddleware limits the request body size.
func maxBodySizeMiddleware(maxSize int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Body != nil {
				r.Body = http.MaxBytesReader(w, r.Body, maxSize)
			}
			next.ServeHTTP(w, r)
		})
	}
}

// corsMiddleware adds CORS headers allowing all origins.
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Request-ID")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// jsonLoggerMiddleware logs each HTTP request as a JSON object.
func jsonLoggerMiddleware(logger *logging.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			next.ServeHTTP(ww, r)

			duration := time.Since(start)
			status := ww.Status()

			level := "info"
			if status >= 500 {
				level = "error"
			}

			entry := map[string]interface{}{
				"timestamp":     time.Now().UTC().Format(time.RFC3339),
				"level":         level,
				"message":       "http_request",
				"method":        r.Method,
				"path":          r.URL.Path,
				"status":        status,
				"duration_ms":   duration.Milliseconds(),
				"request_id":    middleware.GetReqID(r.Context()),
				"remote_addr":   r.RemoteAddr,
				"user_agent":    r.UserAgent(),
				"bytes_written": ww.BytesWritten(),
			}

			data, err := json.Marshal(entry)
			if err != nil {
				logger.Error("failed to marshal request log: %v", err)
				return
			}
			out := logger.Writer()
			if out == nil {
				return
			}
			fmt.Fprintln(out, string(data))
		})
	}
}
