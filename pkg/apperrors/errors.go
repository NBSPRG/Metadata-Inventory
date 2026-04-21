// Package apperrors defines sentinel errors and a structured error response
// type for the metadata-inventory service. Handlers use errors.Is() to map
// domain errors to HTTP status codes. Business logic never returns HTTP
// status codes directly.
package apperrors

import "errors"

// Sentinel errors for the service domain. Use errors.Is() to match these
// in handlers and map to appropriate HTTP status codes.
var (
	ErrNotFound       = errors.New("not found")
	ErrInvalidURL     = errors.New("invalid url")
	ErrFetchFailed    = errors.New("fetch failed")
	ErrAlreadyPending = errors.New("already pending")
	ErrDatabaseError  = errors.New("database error")
	ErrKafkaError     = errors.New("kafka error")
	ErrValidation     = errors.New("validation error")
)

// ErrorResponse is the standard error response format for all API errors.
// Every error returned to clients follows this schema for consistency.
type ErrorResponse struct {
	Error     string `json:"error"`
	Message   string `json:"message"`
	RequestID string `json:"request_id,omitempty"`
	Details   string `json:"details,omitempty"`
}
