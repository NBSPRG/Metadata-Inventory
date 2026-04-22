package db

import (
	"context"
	"sync"
)

// MockRepository is an in-memory implementation of MetadataRepository
// for unit testing. It stores records in a map keyed by URL.
type MockRepository struct {
	mu      sync.RWMutex
	records map[string]*MetadataRecord

	// Hooks for test assertions
	FindByURLCalled    int
	UpsertCalled       int
	UpdateStatusCalled int
	PingCalled         int

	// Configurable error returns for testing error paths
	FindByURLErr    error
	UpsertErr       error
	UpdateStatusErr error
	PingErr         error
}

// NewMockRepository creates a new MockRepository.
func NewMockRepository() *MockRepository {
	return &MockRepository{
		records: make(map[string]*MetadataRecord),
	}
}

// FindByURL retrieves a record by URL from the in-memory store.
func (m *MockRepository) FindByURL(_ context.Context, url string) (*MetadataRecord, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	m.FindByURLCalled++

	if m.FindByURLErr != nil {
		return nil, m.FindByURLErr
	}

	record, ok := m.records[url]
	if !ok {
		return nil, nil
	}
	// Return a copy to prevent mutation
	copied := *record
	return &copied, nil
}

// Upsert stores a record in the in-memory store.
func (m *MockRepository) Upsert(_ context.Context, record *MetadataRecord) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.UpsertCalled++

	if m.UpsertErr != nil {
		return m.UpsertErr
	}

	copied := *record
	m.records[record.URL] = &copied
	return nil
}

// UpdateStatus updates the status of a record in the in-memory store.
func (m *MockRepository) UpdateStatus(_ context.Context, url string, status Status, result *FetchResult) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.UpdateStatusCalled++

	if m.UpdateStatusErr != nil {
		return m.UpdateStatusErr
	}

	record, ok := m.records[url]
	if !ok {
		return nil // Silently ignore missing records (same as MongoDB behavior)
	}

	record.Status = status
	if result != nil {
		record.Headers = result.Headers
		record.Cookies = result.Cookies
		record.PageSource = result.PageSource
		record.PageSourceSizeBytes = result.PageSourceSizeBytes
		record.FetchDurationMs = result.FetchDurationMs
		record.FetchedAt = &result.FetchedAt
	}

	return nil
}

// Ping always returns nil (or the configured error).
func (m *MockRepository) Ping(_ context.Context) error {
	m.PingCalled++
	return m.PingErr
}

// SeedRecord adds a record to the mock store for test setup.
func (m *MockRepository) SeedRecord(record *MetadataRecord) {
	m.mu.Lock()
	defer m.mu.Unlock()
	copied := *record
	m.records[record.URL] = &copied
}

// GetRecord retrieves a record directly for test assertions.
func (m *MockRepository) GetRecord(url string) *MetadataRecord {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if r, ok := m.records[url]; ok {
		copied := *r
		return &copied
	}
	return nil
}
