// Package server provides the HTTP server setup for the metadata-inventory API,
// including route registration, middleware stack, and graceful shutdown.
package server

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel/trace"

	"github.com/metadata-inventory/pkg/featureflags"
	mw "github.com/metadata-inventory/pkg/middleware"
	"github.com/metadata-inventory/pkg/observability"
)

// Server wraps the HTTP server with graceful shutdown support.
type Server struct {
	httpServer *http.Server
	logger     *slog.Logger
}

// Config holds the server configuration.
type Config struct {
	Port            int
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	ShutdownTimeout time.Duration
}

// New creates a new Server with the configured router, middleware, and timeouts.
func New(
	cfg Config,
	router *chi.Mux,
	logger *slog.Logger,
) *Server {
	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Port),
		Handler:      router,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
		IdleTimeout:  60 * time.Second,
	}

	return &Server{
		httpServer: srv,
		logger:     logger,
	}
}

// Start begins listening for HTTP requests. This method blocks until
// the server is shut down.
func (s *Server) Start() error {
	s.logger.Info("http server starting", slog.String("addr", s.httpServer.Addr))
	if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("http server error: %w", err)
	}
	return nil
}

// Shutdown gracefully stops the server, waiting for in-flight requests
// to complete within the given timeout.
func (s *Server) Shutdown(ctx context.Context) error {
	s.logger.Info("http server shutting down")
	return s.httpServer.Shutdown(ctx)
}

// NewRouter creates and configures the chi router with the full middleware
// stack. Route handlers are registered separately via RegisterRoutes.
func NewRouter(
	logger *slog.Logger,
	metrics *observability.Metrics,
	tracer trace.Tracer,
	flags featureflags.FeatureFlags,
) *chi.Mux {
	r := chi.NewRouter()

	// --- Global middleware stack (order matters) ---
	r.Use(mw.RequestID)                    // 1. Request ID first — all subsequent middleware can use it
	r.Use(mw.Recovery(logger))             // 2. Recovery — catch panics before logging
	r.Use(mw.Logging(logger))              // 3. Structured request logging
	r.Use(chimw.RealIP)                    // 4. Extract real IP for rate limiting
	r.Use(mw.Tracing(tracer))              // 5. OTel tracing spans

	// Conditional middleware
	if flags.IsEnabled(featureflags.MetricsEnabled) {
		r.Use(mw.MetricsMiddleware(metrics))
	}

	if flags.IsEnabled(featureflags.RateLimitEnabled) {
		limiter := mw.NewRateLimiter(100, time.Minute) // 100 req/min per IP
		r.Use(limiter.Middleware)
	}

	// Prometheus metrics endpoint
	if flags.IsEnabled(featureflags.MetricsEnabled) {
		r.Handle("/metrics", promhttp.Handler())
	}

	return r
}
