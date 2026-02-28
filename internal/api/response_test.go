package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriteJSON_ProducesCorrectEnvelope(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/test", nil)

	writeJSON(w, r, http.StatusOK, map[string]string{"key": "value"}, nil)

	var resp APIResponse
	err := json.NewDecoder(w.Body).Decode(&resp)
	require.NoError(t, err)

	assert.NotNil(t, resp.Data)
	assert.NotNil(t, resp.Meta)
	assert.Nil(t, resp.Error)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/json; charset=utf-8", w.Header().Get("Content-Type"))
}

func TestWriteError_ProducesCorrectErrorEnvelope(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/test", nil)

	writeError(w, r, http.StatusNotFound, "not_found", "repo not found")

	var resp APIResponse
	err := json.NewDecoder(w.Body).Decode(&resp)
	require.NoError(t, err)

	assert.Nil(t, resp.Data)
	assert.NotNil(t, resp.Meta)
	assert.NotNil(t, resp.Error)
	assert.Equal(t, "not_found", resp.Error.Code)
	assert.Equal(t, "repo not found", resp.Error.Message)
	assert.Equal(t, http.StatusNotFound, w.Code)
}
