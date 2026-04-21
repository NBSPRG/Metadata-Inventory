package handlers

import (
	"context"
	"net/http"
	"time"

	"github.com/metadata-inventory/pkg/db"
	"github.com/metadata-inventory/pkg/kafka"
)

// HealthHandler handles GET /health (liveness probe).
type HealthHandler struct{}

// NewHealthHandler creates a new health check handler.
func NewHealthHandler() *HealthHandler {
	return &HealthHandler{}
}

type healthResponse struct {
	Status    string `json:"status"`
	Timestamp string `json:"timestamp"`
}

// ServeHTTP returns a simple OK response for liveness checks.
func (h *HealthHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, healthResponse{
		Status:    "ok",
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	})
}

// ReadyHandler handles GET /ready (readiness probe).
// It checks that MongoDB and Kafka are reachable.
type ReadyHandler struct {
	repo     db.MetadataRepository
	consumer *kafka.KafkaConsumer // nil for API service (only producer)
}

// NewReadyHandler creates a new readiness check handler.
func NewReadyHandler(repo db.MetadataRepository, consumer *kafka.KafkaConsumer) *ReadyHandler {
	return &ReadyHandler{
		repo:     repo,
		consumer: consumer,
	}
}

type readyResponse struct {
	Status string            `json:"status"`
	Checks map[string]string `json:"checks"`
}

// ServeHTTP checks dependency health and returns readiness status.
func (h *ReadyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	checks := make(map[string]string)
	allOK := true

	// Check MongoDB
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	if err := h.repo.Ping(ctx); err != nil {
		checks["mongodb"] = "error: " + err.Error()
		allOK = false
	} else {
		checks["mongodb"] = "ok"
	}

	// Check Kafka (if consumer available)
	if h.consumer != nil {
		if err := h.consumer.Ping(); err != nil {
			checks["kafka"] = "error: " + err.Error()
			allOK = false
		} else {
			checks["kafka"] = "ok"
		}
	} else {
		checks["kafka"] = "ok" // API service doesn't consume
	}

	resp := readyResponse{
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
