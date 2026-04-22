package fetcher

import "time"

// MockFetcher is a test double for the Fetcher interface.
// It returns a configurable result or error.
type MockFetcher struct {
	Result  *FetchResult
	Err     error
	Called  int
	LastURL string
}

// Fetch returns the configured result or error, and records the call.
func (m *MockFetcher) Fetch(url string, includePageSource bool) (*FetchResult, error) {
	m.Called++
	m.LastURL = url

	if m.Err != nil {
		return nil, m.Err
	}

	if m.Result != nil {
		return m.Result, nil
	}

	// Default mock result
	return &FetchResult{
		StatusCode: 200,
		Headers: map[string][]string{
			"Content-Type": {"text/html; charset=utf-8"},
			"Server":       {"MockServer/1.0"},
		},
		Cookies: []CookieInfo{
			{Name: "session", Value: "abc123", HTTPOnly: true, Secure: true},
		},
		PageSource:          "<html><body>mock</body></html>",
		PageSourceSizeBytes: 29,
		FetchDurationMs:     42,
		FetchedAt:           time.Now().UTC(),
	}, nil
}
