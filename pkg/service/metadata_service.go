// Package service implements the core business logic for the metadata-inventory
// service. It orchestrates the repository, Kafka producer, and fetcher to
// handle metadata requests. Handlers call this layer — they never touch
// infrastructure directly.
package service

import (
	"context"

	"github.com/metadata-inventory/pkg/db"
)

// MetadataService defines the business operations for metadata management.
// Handlers depend on this interface for all business logic.
type MetadataService interface {
	// SubmitURL processes a metadata request for the given URL.
	// If the URL already has metadata (status=ready), it returns the existing record.
	// Otherwise, it fetches the URL and stores the result.
	// Returns the record and a boolean indicating if it was newly created.
	SubmitURL(ctx context.Context, url string) (*db.MetadataRecord, bool, error)

	// GetMetadata retrieves metadata for a URL if it exists.
	// If cached (status=ready), returns the record.
	// If not cached, dispatches an async fetch via Kafka and returns nil
	// with a flag indicating the request was queued.
	GetMetadata(ctx context.Context, url string) (*db.MetadataRecord, bool, error)
}
