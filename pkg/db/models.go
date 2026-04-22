// Package db provides the data access layer for the metadata-inventory
// service. It defines the MetadataRecord model, the MetadataRepository
// interface, and a concrete MongoDB implementation.
package db

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// Status represents the processing state of a metadata record.
type Status string

const (
	StatusPending  Status = "pending"
	StatusFetching Status = "fetching"
	StatusReady    Status = "ready"
	StatusFailed   Status = "failed"
)

// MetadataRecord represents a single URL's fetched metadata stored in MongoDB.
// Fields use both BSON tags (for MongoDB) and JSON tags (for API responses).
type MetadataRecord struct {
	ID                  primitive.ObjectID  `bson:"_id,omitempty"  json:"-"`
	URL                 string              `bson:"url"            json:"url"`
	URLHash             string              `bson:"url_hash"       json:"url_hash,omitempty"`
	Status              Status              `bson:"status"         json:"status"`
	SchemaVersion       int                 `bson:"schema_version" json:"schema_version"`
	Headers             map[string][]string `bson:"headers,omitempty"       json:"headers,omitempty"`
	Cookies             []CookieRecord      `bson:"cookies,omitempty"       json:"cookies,omitempty"`
	PageSource          string              `bson:"page_source,omitempty"   json:"page_source,omitempty"`
	PageSourceSizeBytes int                 `bson:"page_source_size_bytes"  json:"page_source_size_bytes,omitempty"`
	Error               string              `bson:"error,omitempty"         json:"error,omitempty"`
	RetryCount          int                 `bson:"retry_count"             json:"retry_count"`
	FetchDurationMs     int64               `bson:"fetch_duration_ms"       json:"fetch_duration_ms,omitempty"`
	FetchedAt           *time.Time          `bson:"fetched_at,omitempty"    json:"fetched_at,omitempty"`
	CreatedAt           time.Time           `bson:"created_at"              json:"created_at"`
	UpdatedAt           time.Time           `bson:"updated_at"              json:"updated_at"`
}

// CookieRecord represents a single HTTP cookie extracted from a response.
type CookieRecord struct {
	Name     string `bson:"name"      json:"name"`
	Value    string `bson:"value"     json:"value"`
	Domain   string `bson:"domain"    json:"domain,omitempty"`
	Path     string `bson:"path"      json:"path,omitempty"`
	HTTPOnly bool   `bson:"http_only" json:"http_only"`
	Secure   bool   `bson:"secure"    json:"secure"`
}

// FetchResult holds the data extracted from an HTTP fetch operation.
// This is used by the worker to update the metadata record after fetching.
type FetchResult struct {
	Headers             map[string][]string
	Cookies             []CookieRecord
	PageSource          string
	PageSourceSizeBytes int
	FetchDurationMs     int64
	FetchedAt           time.Time
}

// CurrentSchemaVersion is the current version of the document schema.
// Increment this when making breaking changes to the document structure.
const CurrentSchemaVersion = 1
