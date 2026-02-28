package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5/middleware"
)

// APIResponse is the standard JSON envelope for all API responses.
type APIResponse struct {
	Data  interface{} `json:"data"`
	Meta  *APIMeta    `json:"meta"`
	Error *APIError   `json:"error"`
}

// APIMeta contains metadata about the response.
type APIMeta struct {
	RequestID  string `json:"request_id"`
	DurationMs int64  `json:"duration_ms,omitempty"`
	Total      int    `json:"total,omitempty"`
	Limit      int    `json:"limit,omitempty"`
	Offset     int    `json:"offset,omitempty"`
}

// APIError describes an error response.
type APIError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// writeJSON writes a successful JSON response with the standard envelope.
func writeJSON(w http.ResponseWriter, r *http.Request, status int, data interface{}, meta *APIMeta) {
	if meta == nil {
		meta = &APIMeta{}
	}
	meta.RequestID = middleware.GetReqID(r.Context())

	resp := APIResponse{
		Data:  data,
		Meta:  meta,
		Error: nil,
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(resp)
}

// writeError writes an error JSON response.
func writeError(w http.ResponseWriter, r *http.Request, status int, code, message string) {
	resp := APIResponse{
		Data: nil,
		Meta: &APIMeta{
			RequestID: middleware.GetReqID(r.Context()),
		},
		Error: &APIError{
			Code:    code,
			Message: message,
		},
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(resp)
}

// writeJSONTimed writes a JSON response with timing metadata.
func writeJSONTimed(w http.ResponseWriter, r *http.Request, status int, data interface{}, start time.Time, total, limit, offset int) {
	meta := &APIMeta{
		DurationMs: time.Since(start).Milliseconds(),
		Total:      total,
		Limit:      limit,
		Offset:     offset,
	}
	writeJSON(w, r, status, data, meta)
}
