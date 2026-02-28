package api

import (
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParsePagination_Defaults(t *testing.T) {
	req := httptest.NewRequest("GET", "/test", nil)
	limit, offset := parsePagination(req)
	assert.Equal(t, 50, limit)
	assert.Equal(t, 0, offset)
}

func TestParsePagination_CapAtMax(t *testing.T) {
	req := httptest.NewRequest("GET", "/test?limit=999", nil)
	limit, _ := parsePagination(req)
	assert.Equal(t, 200, limit)
}

func TestParsePagination_NegativeLimit(t *testing.T) {
	req := httptest.NewRequest("GET", "/test?limit=-1", nil)
	limit, _ := parsePagination(req)
	assert.Equal(t, 50, limit)
}

func TestParsePagination_NegativeOffset(t *testing.T) {
	req := httptest.NewRequest("GET", "/test?offset=-5", nil)
	_, offset := parsePagination(req)
	assert.Equal(t, 0, offset)
}

func TestParsePagination_NonNumeric(t *testing.T) {
	req := httptest.NewRequest("GET", "/test?limit=abc", nil)
	limit, _ := parsePagination(req)
	assert.Equal(t, 50, limit)
}

func TestParsePagination_ValidValues(t *testing.T) {
	req := httptest.NewRequest("GET", "/test?limit=25&offset=10", nil)
	limit, offset := parsePagination(req)
	assert.Equal(t, 25, limit)
	assert.Equal(t, 10, offset)
}
