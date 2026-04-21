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

// PendingResponse is returned when a cache miss triggers async dispatch.
type PendingResponse struct {
	Status           string `json:"status"`
	URL              string `json:"url"`
	Message          string `json:"message"`
	PollAfterSeconds int    `json:"poll_after_seconds"`
}

// ServeHTTP godoc
// @Summary Get metadata for a URL
// @Description Returns cached metadata when present. On cache miss, persists a pending record, dispatches async collection, and returns 202 immediately.
// @Tags metadata
// @Produce json
// @Param url query string true "Absolute URL to look up"
// @Success 200 {object} db.MetadataRecord
// @Success 202 {object} PendingResponse
// @Failure 400 {object} apperrors.ErrorResponse
// @Failure 500 {object} apperrors.ErrorResponse
// @Router /v1/metadata [get]
// ServeHTTP handles the GET /v1/metadata?url=... endpoint.
// Returns cached metadata (200), dispatches async fetch on miss (202),
// or returns an error.
func (h *MetadataGetHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	requestID := observability.RequestIDFromContext(r.Context())

	rawURL := r.URL.Query().Get("url")
	if rawURL == "" {
		writeError(w, http.StatusBadRequest, "MISSING_PARAM", "url query parameter is required", requestID)
		return
	}

	if err := validateURL(rawURL); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_URL", err.Error(), requestID)
		return
	}

	record, queued, err := h.svc.GetMetadata(r.Context(), rawURL)
	if err != nil {
		handleServiceError(w, err, requestID)
		return
	}

	if record != nil {
		writeJSON(w, http.StatusOK, record)
		return
	}

	if queued {
		writeJSON(w, http.StatusAccepted, PendingResponse{
			Status:           "pending",
			URL:              rawURL,
			Message:          "Fetch request queued",
			PollAfterSeconds: 5,
		})
		return
	}

	writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "An unexpected error occurred", requestID)
}
