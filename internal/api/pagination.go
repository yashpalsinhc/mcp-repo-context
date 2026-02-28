package api

import (
	"net/http"
	"strconv"
)

const (
	defaultLimit = 50
	maxLimit     = 200
)

// parsePagination extracts limit and offset from query parameters with defaults and bounds.
func parsePagination(r *http.Request) (limit, offset int) {
	limit = defaultLimit
	offset = 0

	if v := r.URL.Query().Get("limit"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil {
			limit = parsed
		}
	}

	if v := r.URL.Query().Get("offset"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil {
			offset = parsed
		}
	}

	// Enforce bounds
	if limit < 1 {
		limit = defaultLimit
	}
	if limit > maxLimit {
		limit = maxLimit
	}
	if offset < 0 {
		offset = 0
	}

	return limit, offset
}
