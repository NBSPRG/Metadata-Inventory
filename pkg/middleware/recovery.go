package middleware

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"runtime/debug"

	"github.com/metadata-inventory/pkg/apperrors"
	"github.com/metadata-inventory/pkg/observability"
)

// Recovery is middleware that catches panics in downstream handlers,
// logs the stack trace with the request ID, and returns a structured
// 500 response. The service never crashes due to a bad request.
func Recovery(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					log := observability.LoggerFromContext(r.Context(), logger)
					log.Error("panic recovered",
						slog.Any("panic", rec),
						slog.String("stack", string(debug.Stack())),
						slog.String("method", r.Method),
						slog.String("path", r.URL.Path),
					)

					requestID := observability.RequestIDFromContext(r.Context())
					errResp := apperrors.ErrorResponse{
						Error:     "INTERNAL_ERROR",
						Message:   "An unexpected error occurred",
						RequestID: requestID,
					}

					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusInternalServerError)
					json.NewEncoder(w).Encode(errResp)
				}
			}()

			next.ServeHTTP(w, r)
		})
	}
}
