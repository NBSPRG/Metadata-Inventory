// Package kafka provides typed message structs, a producer interface, and
// a consumer interface for Kafka event-driven communication between the
// API and Worker services.
package kafka

import "time"

// MessageVersion is the current schema version for Kafka messages.
// Worker ignores unknown fields for forward compatibility.
const MessageVersion = "1"

// FetchRequestMessage is the message published to Kafka when a URL
// fetch is requested. It carries enough context for the worker to
// process the request and for tracing to correlate across services.
type FetchRequestMessage struct {
	Version     string    `json:"version"`
	URL         string    `json:"url"`
	RequestedAt time.Time `json:"requested_at"`
	RequestID   string    `json:"request_id"`
	Source      string    `json:"source"` // "api-get" or "api-post"
}
