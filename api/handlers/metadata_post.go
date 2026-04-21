// Package handlers contains the HTTP handlers for the metadata-inventory API.
// Handlers are thin — they parse input, call the service layer, and write
// the response. No business logic lives here.
package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/metadata-inventory/pkg/apperrors"
	"github.com/metadata-inventory/pkg/observability"
	"github.com/metadata-inventory/pkg/service"
)

// MetadataPostHandler handles POST /v1/metadata requests.
type MetadataPostHandler struct {
	svc service.MetadataService
}

// NewMetadataPostHandler creates a new handler for POST /v1/metadata.
func NewMetadataPostHandler(svc service.MetadataService) *MetadataPostHandler {
	return &MetadataPostHandler{svc: svc}
}

// PostRequest is the expected JSON body for POST /v1/metadata.
type PostRequest struct {
	URL string `json:"url"`
}

// ServeHTTP godoc
// @Summary Create or refresh metadata for a URL
// @Description Validates a URL, stores a pending metadata record, and either fetches inline or dispatches async collection depending on feature flags.
// @Tags metadata
// @Accept json
// @Produce json
// @Param request body PostRequest true "URL to inventory"
// @Success 200 {object} db.MetadataRecord
// @Success 201 {object} db.MetadataRecord
// @Success 202 {object} db.MetadataRecord
// @Failure 400 {object} apperrors.ErrorResponse
// @Failure 422 {object} apperrors.ErrorResponse
// @Failure 500 {object} apperrors.ErrorResponse
// @Router /v1/metadata [post]
// ServeHTTP handles the POST /v1/metadata endpoint.
// It validates the URL, calls the service layer, and returns the metadata record.
func (h *MetadataPostHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	requestID := observability.RequestIDFromContext(r.Context())

	// Parse request body
	var req PostRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_BODY", "Request body must be valid JSON", requestID)
		return
	}

	// Validate URL
	if err := validateURL(req.URL); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_URL", err.Error(), requestID)
		return
	}

	// Call service
	record, isNew, err := h.svc.SubmitURL(r.Context(), req.URL)
	if err != nil {
		handleServiceError(w, err, requestID)
		return
	}

	if record != nil && (record.Status == "pending" || record.Status == "fetching") {
		writeJSON(w, http.StatusAccepted, record)
		return
	}

	if isNew {
		writeJSON(w, http.StatusCreated, record)
	} else {
		writeJSON(w, http.StatusOK, record)
	}
}

// validateURL checks that the URL is absolute, uses http(s), and isn't too long.
func validateURL(rawURL string) error {
	if rawURL == "" {
		return errors.New("URL is required")
	}
	if len(rawURL) > 2048 {
		return errors.New("URL must be at most 2048 characters")
	}

	parsed, err := url.ParseRequestURI(rawURL)
	if err != nil {
		return errors.New("URL must be a valid absolute URL")
	}

	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return errors.New("URL must use http or https scheme")
	}

	if parsed.Host == "" {
		return errors.New("URL must include a host")
	}

	return nil
}

// handleServiceError maps service-layer errors to HTTP responses.
func handleServiceError(w http.ResponseWriter, err error, requestID string) {
	switch {
	case errors.Is(err, apperrors.ErrInvalidURL):
		writeError(w, http.StatusBadRequest, "INVALID_URL", err.Error(), requestID)
	case errors.Is(err, apperrors.ErrFetchFailed):
		writeError(w, http.StatusUnprocessableEntity, "FETCH_FAILED", err.Error(), requestID)
	case errors.Is(err, apperrors.ErrAlreadyPending):
		writeError(w, http.StatusConflict, "ALREADY_PENDING", err.Error(), requestID)
	case errors.Is(err, apperrors.ErrDatabaseError):
		writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "A database error occurred", requestID)
	case errors.Is(err, apperrors.ErrKafkaError):
		writeError(w, http.StatusInternalServerError, "QUEUE_ERROR", "Failed to queue request", requestID)
	default:
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "An unexpected error occurred", requestID)
	}
}

// writeJSON writes a JSON response with the given status code.
func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

// writeError writes a structured error response.
func writeError(w http.ResponseWriter, status int, code, message, requestID string) {
	resp := apperrors.ErrorResponse{
		Error:     code,
		Message:   message,
		RequestID: requestID,
	}
	writeJSON(w, status, resp)
}
