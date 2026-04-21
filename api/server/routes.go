package server

import (
	"github.com/go-chi/chi/v5"
	"github.com/metadata-inventory/api/handlers"
	httpSwagger "github.com/swaggo/http-swagger/v2"
)

// RegisterRoutes mounts all API routes onto the chi router.
// Routes are versioned under /v1/ for backward compatibility.
func RegisterRoutes(
	r *chi.Mux,
	postHandler *handlers.MetadataPostHandler,
	getHandler *handlers.MetadataGetHandler,
	healthHandler *handlers.HealthHandler,
	readyHandler *handlers.ReadyHandler,
) {
	// --- Health / Readiness (unversioned) ---
	r.Method("GET", "/health", healthHandler)
	r.Method("GET", "/ready", readyHandler)
	r.Get("/swagger/*", httpSwagger.Handler())

	// --- API v1 ---
	r.Route("/v1", func(r chi.Router) {
		r.Method("POST", "/metadata", postHandler)
		r.Method("GET", "/metadata", getHandler)
	})
}
