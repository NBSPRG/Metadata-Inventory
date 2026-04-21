// Package middleware provides HTTP middleware for the metadata-inventory API.
// All middleware is compatible with net/http and chi router.
package middleware

import (
	"net/http"

	"github.com/google/uuid"
	"github.com/metadata-inventory/pkg/observability"
)

const (
	// HeaderRequestID is the HTTP header for request ID propagation.
	HeaderRequestID = "X-Request-ID"
)

// RequestID is a middleware that ensures every request has a unique
// X-Request-ID. If the client provides one, it is reused; otherwise
// a new UUID is generated. The ID is stored in the request context
// and included in the response headers.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := r.Header.Get(HeaderRequestID)
		if requestID == "" {
			requestID = uuid.New().String()
		}

		// Set on response header
		w.Header().Set(HeaderRequestID, requestID)

		// Store in context for downstream use (logging, error responses)
		ctx := observability.ContextWithRequestID(r.Context(), requestID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
