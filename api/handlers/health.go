package handlers

import (
	"context"
	"net/http"
	"time"

	"github.com/metadata-inventory/pkg/db"
)

// HealthHandler handles GET /health (liveness probe).
type HealthHandler struct{}

// NewHealthHandler creates a new health check handler.
func NewHealthHandler() *HealthHandler {
	return &HealthHandler{}
}

// HealthResponse is returned by the liveness endpoint.
type HealthResponse struct {
	Status    string `json:"status"`
	Timestamp string `json:"timestamp"`
}

// ServeHTTP godoc
// @Summary Liveness probe
// @Description Returns process liveness for container orchestration and smoke tests.
// @Tags system
// @Produce json
// @Success 200 {object} HealthResponse
// @Router /health [get]
func (h *HealthHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, HealthResponse{
		Status:    "ok",
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	})
}

// ReadyHandler handles GET /ready (readiness probe).
// It checks that MongoDB and Kafka are reachable.
type ReadyHandler struct {
	repo  db.MetadataRepository
	kafka kafkaPinger
}

type kafkaPinger interface {
	Ping(ctx context.Context) error
}

// NewReadyHandler creates a new readiness check handler.
func NewReadyHandler(repo db.MetadataRepository, dependency kafkaPinger) *ReadyHandler {
	return &ReadyHandler{
		repo:  repo,
		kafka: dependency,
	}
}

// ReadyResponse is returned by the readiness endpoint.
type ReadyResponse struct {
	Status string            `json:"status"`
	Checks map[string]string `json:"checks"`
}

// ServeHTTP godoc
// @Summary Readiness probe
// @Description Verifies that MongoDB and Kafka are reachable by the API service.
// @Tags system
// @Produce json
// @Success 200 {object} ReadyResponse
// @Failure 503 {object} ReadyResponse
// @Router /ready [get]
func (h *ReadyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	checks := make(map[string]string)
	allOK := true

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	if err := h.repo.Ping(ctx); err != nil {
		checks["mongodb"] = "error: " + err.Error()
		allOK = false
	} else {
		checks["mongodb"] = "ok"
	}

	if h.kafka != nil {
		if err := h.kafka.Ping(ctx); err != nil {
			checks["kafka"] = "error: " + err.Error()
			allOK = false
		} else {
			checks["kafka"] = "ok"
		}
	} else {
		checks["kafka"] = "skipped"
	}

	resp := ReadyResponse{
		Checks: checks,
	}

	if allOK {
		resp.Status = "ready"
		writeJSON(w, http.StatusOK, resp)
	} else {
		resp.Status = "degraded"
		writeJSON(w, http.StatusServiceUnavailable, resp)
	}
}
