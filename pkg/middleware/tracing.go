package middleware

import (
	"net/http"

	"go.opentelemetry.io/otel/trace"
	"github.com/metadata-inventory/pkg/observability"
)

// Tracing returns middleware that extracts or creates an OpenTelemetry
// trace span for each request and injects the trace ID into the context
// and response headers. When tracing is disabled (no-op tracer), this
// middleware has zero overhead.
func Tracing(tracer trace.Tracer) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx, span := tracer.Start(r.Context(), r.Method+" "+r.URL.Path)
			defer span.End()

			// Add trace ID to context and response header
			traceID := span.SpanContext().TraceID().String()
			if traceID != "" && traceID != "00000000000000000000000000000000" {
				ctx = observability.ContextWithTraceID(ctx, traceID)
				w.Header().Set("X-Trace-ID", traceID)
			}

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
