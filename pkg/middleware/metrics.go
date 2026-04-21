package middleware

import (
	"net/http"
	"strconv"

	"github.com/metadata-inventory/pkg/observability"
)

// MetricsMiddleware returns middleware that records Prometheus metrics
// for every HTTP request (request count and duration).
func MetricsMiddleware(metrics *observability.Metrics) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Skip metrics endpoint itself to avoid recursion
			if r.URL.Path == "/metrics" {
				next.ServeHTTP(w, r)
				return
			}

			wrapped := newResponseWriter(w)
			timer := observability.NewTimer()

			next.ServeHTTP(wrapped, r)

			duration := timer.Elapsed()
			statusCode := strconv.Itoa(wrapped.statusCode)
			path := normalizePath(r.URL.Path)

			metrics.HTTPRequestsTotal.WithLabelValues(r.Method, path, statusCode).Inc()
			metrics.HTTPRequestDuration.WithLabelValues(r.Method, path).Observe(duration)
		})
	}
}

// normalizePath collapses path parameters to prevent high-cardinality labels.
func normalizePath(path string) string {
	// For now, return the path as-is since our routes are fixed.
	// In a real app with dynamic IDs, replace them with :id placeholders.
	return path
}
