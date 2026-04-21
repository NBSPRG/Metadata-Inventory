package db

import "context"

// MetadataRepository defines the data access contract for metadata records.
// All data layer consumers depend on this interface — never on a concrete
// implementation. This enables seamless swapping of the backing store
// (MongoDB → Postgres, in-memory, etc.) without touching business logic.
type MetadataRepository interface {
	// FindByURL retrieves a metadata record by its URL.
	// Returns nil, nil if no record is found.
	FindByURL(ctx context.Context, url string) (*MetadataRecord, error)

	// Upsert inserts a new metadata record or updates an existing one.
	// The URL field is used as the unique key for upsert operations.
	Upsert(ctx context.Context, record *MetadataRecord) error

	// UpdateStatus updates the status of a record identified by URL.
	// If result is non-nil, the fetch result fields are also updated.
	UpdateStatus(ctx context.Context, url string, status Status, result *FetchResult) error

	// Ping verifies that the database connection is alive.
	// Used by the readiness probe.
	Ping(ctx context.Context) error
}
