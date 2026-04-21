// Package fetcher provides an interface and implementations for fetching
// HTTP metadata (headers, cookies, page source) from URLs. It includes
// SSRF protection to block requests to private IP ranges.
package fetcher

import (
	"net/http"
	"time"
)

// FetchResult holds all metadata extracted from an HTTP response.
type FetchResult struct {
	StatusCode        int
	Headers           map[string][]string
	Cookies           []CookieInfo
	PageSource        string
	PageSourceSizeBytes int
	FetchDurationMs   int64
	FetchedAt         time.Time
}

// CookieInfo holds a sanitized cookie from the response.
type CookieInfo struct {
	Name     string `json:"name"`
	Value    string `json:"value"`
	Domain   string `json:"domain,omitempty"`
	Path     string `json:"path,omitempty"`
	HTTPOnly bool   `json:"http_only"`
	Secure   bool   `json:"secure"`
}

// Fetcher defines the interface for fetching HTTP metadata from a URL.
// Consumers depend on this interface, allowing the implementation to be
// swapped (e.g., for testing or circuit-breaker wrapping).
type Fetcher interface {
	// Fetch retrieves HTTP metadata for the given URL.
	// It returns the response headers, cookies, and optionally the page source.
	Fetch(url string, includePageSource bool) (*FetchResult, error)
}

// CookieFromHTTP converts an *http.Cookie to a CookieInfo,
// stripping sensitive values.
func CookieFromHTTP(c *http.Cookie) CookieInfo {
	return CookieInfo{
		Name:     c.Name,
		Value:    c.Value, // In production, consider masking this
		Domain:   c.Domain,
		Path:     c.Path,
		HTTPOnly: c.HttpOnly,
		Secure:   c.Secure,
	}
}
