package handlers

import (
	"net/http"

	"github.com/metadata-inventory/pkg/observability"
	"github.com/metadata-inventory/pkg/service"
)

// MetadataGetHandler handles GET /v1/metadata requests.
type MetadataGetHandler struct {
	svc service.MetadataService
}

// NewMetadataGetHandler creates a new handler for GET /v1/metadata.
func NewMetadataGetHandler(svc service.MetadataService) *MetadataGetHandler {
	return &MetadataGetHandler{svc: svc}
}

// pendingResponse is returned when a cache miss triggers async dispatch.
type pendingResponse struct {
	Status           string `json:"status"`
	URL              string `json:"url"`
	Message          string `json:"message"`
	PollAfterSeconds int    `json:"poll_after_seconds"`
}

// ServeHTTP handles the GET /v1/metadata?url=... endpoint.
// Returns cached metadata (200), dispatches async fetch on miss (202),
// or returns an error.
func (h *MetadataGetHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	requestID := observability.RequestIDFromContext(r.Context())

	// Extract URL query parameter
	rawURL := r.URL.Query().Get("url")
	if rawURL == "" {
		writeError(w, http.StatusBadRequest, "MISSING_PARAM", "url query parameter is required", requestID)
		return
	}

	// Validate URL
	if err := validateURL(rawURL); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_URL", err.Error(), requestID)
		return
	}

	// Call service
	record, queued, err := h.svc.GetMetadata(r.Context(), rawURL)
	if err != nil {
		handleServiceError(w, err, requestID)
		return
	}

	// Cache hit — return metadata
	if record != nil {
		writeJSON(w, http.StatusOK, record)
		return
	}

	// Cache miss — async fetch dispatched
	if queued {
		writeJSON(w, http.StatusAccepted, pendingResponse{
			Status:           "pending",
			URL:              rawURL,
			Message:          "Fetch request queued",
			PollAfterSeconds: 5,
		})
		return
	}

	// Shouldn't reach here, but handle gracefully
	writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "An unexpected error occurred", requestID)
}
