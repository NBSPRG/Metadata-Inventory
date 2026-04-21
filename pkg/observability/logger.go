// Package observability provides structured logging, metrics, and tracing
// setup for the metadata-inventory service. All observability concerns are
// centralized here and initialized at startup.
package observability

import (
	"context"
	"log/slog"
	"os"
	"strings"
)

// contextKey is an unexported type for context keys in this package.
type contextKey string

const (
	// RequestIDKey is the context key for request IDs.
	RequestIDKey contextKey = "request_id"
	// TraceIDKey is the context key for trace IDs.
	TraceIDKey contextKey = "trace_id"
)

// SetupLogger creates and sets the global slog.Logger as a JSON logger
// writing to stdout with the specified level and default attributes.
func SetupLogger(level, serviceName, version string) *slog.Logger {
	var logLevel slog.Level
	switch strings.ToLower(level) {
	case "debug":
		logLevel = slog.LevelDebug
	case "warn":
		logLevel = slog.LevelWarn
	case "error":
		logLevel = slog.LevelError
	default:
		logLevel = slog.LevelInfo
	}

	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level:     logLevel,
		AddSource: logLevel == slog.LevelDebug,
	})

	logger := slog.New(handler).With(
		slog.String("service", serviceName),
		slog.String("version", version),
	)

	slog.SetDefault(logger)
	return logger
}

// LoggerFromContext returns a logger enriched with request-scoped fields
// (request_id, trace_id) extracted from the context.
func LoggerFromContext(ctx context.Context, base *slog.Logger) *slog.Logger {
	if base == nil {
		base = slog.Default()
	}

	if reqID, ok := ctx.Value(RequestIDKey).(string); ok && reqID != "" {
		base = base.With(slog.String("request_id", reqID))
	}
	if traceID, ok := ctx.Value(TraceIDKey).(string); ok && traceID != "" {
		base = base.With(slog.String("trace_id", traceID))
	}

	return base
}

// ContextWithRequestID returns a new context with the request ID set.
func ContextWithRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, RequestIDKey, requestID)
}

// ContextWithTraceID returns a new context with the trace ID set.
func ContextWithTraceID(ctx context.Context, traceID string) context.Context {
	return context.WithValue(ctx, TraceIDKey, traceID)
}

// RequestIDFromContext extracts the request ID from context, returning
// empty string if not present.
func RequestIDFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(RequestIDKey).(string); ok {
		return v
	}
	return ""
}
