package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/metadata-inventory/pkg/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubKafkaPinger struct {
	err error
}

func (s stubKafkaPinger) Ping(_ context.Context) error {
	return s.err
}

func TestReadyHandlerReturnsReadyWhenDependenciesAreHealthy(t *testing.T) {
	repo := db.NewMockRepository()
	handler := NewReadyHandler(repo, stubKafkaPinger{})

	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"status":"ready"`)
	assert.Contains(t, rec.Body.String(), `"mongodb":"ok"`)
	assert.Contains(t, rec.Body.String(), `"kafka":"ok"`)
}

func TestReadyHandlerReturnsUnavailableWhenKafkaPingFails(t *testing.T) {
	repo := db.NewMockRepository()
	handler := NewReadyHandler(repo, stubKafkaPinger{err: assert.AnError})

	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusServiceUnavailable, rec.Code)
	assert.Contains(t, rec.Body.String(), `"status":"degraded"`)
	assert.Contains(t, rec.Body.String(), `"kafka":"error:`)
}
