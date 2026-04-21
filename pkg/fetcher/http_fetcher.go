package fetcher

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"
)

// HTTPFetcher is the production implementation of Fetcher.
// It fetches URL metadata with configurable timeout, redirect limits,
// and SSRF protection against private IP ranges.
type HTTPFetcher struct {
	client       *http.Client
	maxRedirects int
	skipSSRF     bool // Only true in tests — never in production
}

// NewHTTPFetcher creates an HTTPFetcher with the given timeout and redirect limit.
// It configures SSRF protection by blocking connections to private IP ranges.
func NewHTTPFetcher(timeout time.Duration, maxRedirects int, disableSSRF bool) *HTTPFetcher {
	transport := &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: timeout,
		MaxIdleConns:           100,
		MaxIdleConnsPerHost:    10,
	}

	client := &http.Client{
		Timeout:   timeout,
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= maxRedirects {
				return fmt.Errorf("too many redirects (max %d)", maxRedirects)
			}
			return nil
		},
	}

	return &HTTPFetcher{
		client:       client,
		maxRedirects: maxRedirects,
		skipSSRF:     disableSSRF,
	}
}

// Fetch retrieves HTTP metadata for the given URL.
// It performs SSRF validation, executes the request, and extracts headers,
// cookies, and optionally the page source.
func (f *HTTPFetcher) Fetch(url string, includePageSource bool) (*FetchResult, error) {
	// SSRF protection: validate the URL doesn't point to private IPs
	if !f.skipSSRF {
		if err := validateURLSafety(url); err != nil {
			return nil, fmt.Errorf("ssrf protection: %w", err)
		}
	}

	start := time.Now()

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	// Set a realistic User-Agent to avoid being blocked
	req.Header.Set("User-Agent", "MetadataInventory/1.0 (compatible; metadata-fetcher)")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")

	resp, err := f.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http fetch: %w", err)
	}
	defer resp.Body.Close()

	duration := time.Since(start)

	// Extract headers (sanitize sensitive ones)
	headers := sanitizeHeaders(resp.Header)

	// Extract cookies
	cookies := make([]CookieInfo, 0, len(resp.Cookies()))
	for _, c := range resp.Cookies() {
		cookies = append(cookies, CookieFromHTTP(c))
	}

	result := &FetchResult{
		StatusCode:      resp.StatusCode,
		Headers:         headers,
		Cookies:         cookies,
		FetchDurationMs: duration.Milliseconds(),
		FetchedAt:       time.Now().UTC(),
	}

	// Optionally read page source
	if includePageSource {
		body, err := io.ReadAll(io.LimitReader(resp.Body, 5*1024*1024)) // 5MB limit
		if err != nil {
			return nil, fmt.Errorf("read body: %w", err)
		}
		result.PageSource = string(body)
		result.PageSourceSizeBytes = len(body)
	}

	return result, nil
}

// sanitizeHeaders removes sensitive headers that should not be stored.
func sanitizeHeaders(h http.Header) map[string][]string {
	sanitized := make(map[string][]string, len(h))
	sensitiveHeaders := map[string]bool{
		"Set-Cookie":    true,
		"Authorization": true,
	}

	for key, values := range h {
		if sensitiveHeaders[key] {
			sanitized[key] = []string{"[REDACTED]"}
		} else {
			sanitized[key] = values
		}
	}
	return sanitized
}

// validateURLSafety checks that a URL does not target private/internal IP ranges.
// This prevents Server-Side Request Forgery (SSRF) attacks.
func validateURLSafety(rawURL string) error {
	// Parse hostname from URL
	// We extract between :// and the next / or : or end of string
	host := extractHost(rawURL)
	if host == "" {
		return fmt.Errorf("unable to extract host from URL")
	}

	// Resolve hostname to IPs
	ips, err := net.LookupIP(host)
	if err != nil {
		// If DNS resolution fails, let the HTTP client handle it
		return nil
	}

	for _, ip := range ips {
		if isPrivateIP(ip) {
			return fmt.Errorf("URL resolves to private IP %s — blocked", ip.String())
		}
	}

	return nil
}

// extractHost pulls the hostname from a URL string using net/url.Parse
// for correct handling of userinfo, ports, and edge cases.
func extractHost(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Host == "" {
		return ""
	}
	return parsed.Hostname()
}

// isPrivateIP checks if an IP address is in a private/reserved range.
func isPrivateIP(ip net.IP) bool {
	privateRanges := []struct {
		network *net.IPNet
	}{
		{mustParseCIDR("10.0.0.0/8")},
		{mustParseCIDR("172.16.0.0/12")},
		{mustParseCIDR("192.168.0.0/16")},
		{mustParseCIDR("127.0.0.0/8")},
		{mustParseCIDR("169.254.0.0/16")}, // Link-local
		{mustParseCIDR("::1/128")},         // IPv6 loopback
		{mustParseCIDR("fc00::/7")},        // IPv6 private
		{mustParseCIDR("fe80::/10")},       // IPv6 link-local
	}

	for _, r := range privateRanges {
		if r.network.Contains(ip) {
			return true
		}
	}
	return false
}

func mustParseCIDR(cidr string) *net.IPNet {
	_, network, err := net.ParseCIDR(cidr)
	if err != nil {
		panic(fmt.Sprintf("invalid CIDR: %s", cidr))
	}
	return network
}
