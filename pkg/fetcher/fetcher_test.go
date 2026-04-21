package fetcher

import (
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestFetcher creates an HTTPFetcher that skips SSRF checks for
// httptest.Server (which binds to 127.0.0.1). In production, SSRF
// protection is always enforced.
func newTestFetcher(timeout time.Duration, maxRedirects int) *HTTPFetcher {
	f := NewHTTPFetcher(timeout, maxRedirects)
	f.skipSSRF = true
	return f
}

func TestHTTPFetcher_BasicFetch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{
			Name:     "test-cookie",
			Value:    "test-value",
			HttpOnly: true,
			Secure:   true,
		})
		w.Header().Set("X-Custom-Header", "custom-value")
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("<html><body>Hello</body></html>"))
	}))
	defer server.Close()

	fetcher := newTestFetcher(10*time.Second, 10)
	result, err := fetcher.Fetch(server.URL, true)

	require.NoError(t, err)
	assert.Equal(t, 200, result.StatusCode)
	assert.Contains(t, result.Headers["X-Custom-Header"], "custom-value")
	assert.Contains(t, result.PageSource, "<html><body>Hello</body></html>")
	assert.Equal(t, len(result.PageSource), result.PageSourceSizeBytes)
	assert.NotEmpty(t, result.Cookies)
	assert.Equal(t, "test-cookie", result.Cookies[0].Name)
	assert.True(t, result.FetchDurationMs >= 0)
}

func TestHTTPFetcher_WithoutPageSource(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("<html><body>Skipped</body></html>"))
	}))
	defer server.Close()

	fetcher := newTestFetcher(10*time.Second, 10)
	result, err := fetcher.Fetch(server.URL, false)

	require.NoError(t, err)
	assert.Empty(t, result.PageSource)
	assert.Equal(t, 0, result.PageSourceSizeBytes)
}

func TestHTTPFetcher_TooManyRedirects(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, r.URL.String()+"x", http.StatusFound)
	}))
	defer server.Close()

	fetcher := newTestFetcher(10*time.Second, 3)
	_, err := fetcher.Fetch(server.URL, false)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "redirect")
}

func TestHTTPFetcher_SensitiveHeadersSanitized(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Authorization", "Bearer secret-token")
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	fetcher := newTestFetcher(10*time.Second, 10)
	result, err := fetcher.Fetch(server.URL, false)

	require.NoError(t, err)
	assert.Equal(t, []string{"[REDACTED]"}, result.Headers["Authorization"])
	assert.Contains(t, result.Headers["Content-Type"], "text/html")
}

func TestHTTPFetcher_SSRFBlocked(t *testing.T) {
	// SSRF should block private IPs in production mode
	fetcher := NewHTTPFetcher(5*time.Second, 10)
	_, err := fetcher.Fetch("http://127.0.0.1:9999/test", false)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "ssrf")
}

func TestIsPrivateIP(t *testing.T) {
	tests := []struct {
		name      string
		ip        string
		isPrivate bool
	}{
		{"loopback v4", "127.0.0.1", true},
		{"private 10.x", "10.0.0.1", true},
		{"private 172.16.x", "172.16.0.1", true},
		{"private 192.168.x", "192.168.1.1", true},
		{"public ip", "8.8.8.8", false},
		{"public ip 2", "142.250.185.14", false},
		{"loopback v6", "::1", true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ip := net.ParseIP(tc.ip)
			require.NotNil(t, ip, "failed to parse IP: %s", tc.ip)
			assert.Equal(t, tc.isPrivate, isPrivateIP(ip), "IP: %s", tc.ip)
		})
	}
}

func TestExtractHost(t *testing.T) {
	tests := []struct {
		url      string
		expected string
	}{
		{"https://example.com/path", "example.com"},
		{"http://example.com:8080/path", "example.com"},
		{"https://user:pass@example.com/path", "example.com"},
		{"invalid-url", ""},
	}

	for _, tc := range tests {
		t.Run(tc.url, func(t *testing.T) {
			assert.Equal(t, tc.expected, extractHost(tc.url))
		})
	}
}
