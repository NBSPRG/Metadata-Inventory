package consumer

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/metadata-inventory/pkg/config"
	"github.com/metadata-inventory/pkg/db"
	"github.com/metadata-inventory/pkg/featureflags"
	"github.com/metadata-inventory/pkg/fetcher"
	"github.com/metadata-inventory/pkg/kafka"
	"github.com/metadata-inventory/pkg/observability"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockEventConsumer struct {
	closed bool
}

func (m *mockEventConsumer) Start(_ context.Context, _ kafka.MessageHandler) error {
	return nil
}

func (m *mockEventConsumer) Close() error {
	m.closed = true
	return nil
}

func TestWorkerHandleMessageSuccess(t *testing.T) {
	repo := db.NewMockRepository()
	repo.SeedRecord(&db.MetadataRecord{
		URL:       "https://example.com",
		Status:    db.StatusPending,
		CreatedAt: time.Now().UTC(),
	})

	f := &fetcher.MockFetcher{
		Result: &fetcher.FetchResult{
			StatusCode:          200,
			Headers:             map[string][]string{"Content-Type": {"text/html"}},
			Cookies:             []fetcher.CookieInfo{{Name: "session", Value: "abc"}},
			PageSource:          "<html></html>",
			PageSourceSizeBytes: 13,
			FetchDurationMs:     25,
			FetchedAt:           time.Now().UTC(),
		},
	}

	flags := featureflags.NewEnvFlags(&config.Config{FFPageSourceStorage: true})
	worker := NewWorker(repo, f, &mockEventConsumer{}, flags, nil, observability.SetupLogger("info", "test-worker", "0.0.0"))

	err := worker.handleMessage(context.Background(), kafka.FetchRequestMessage{
		URL:       "https://example.com",
		RequestID: "req-1",
		Source:    "test",
	})

	require.NoError(t, err)
	record := repo.GetRecord("https://example.com")
	require.NotNil(t, record)
	assert.Equal(t, db.StatusReady, record.Status)
	assert.Equal(t, int64(25), record.FetchDurationMs)
	assert.NotNil(t, record.FetchedAt)
}

func TestWorkerHandleMessageFailureMarksRecordFailed(t *testing.T) {
	repo := db.NewMockRepository()
	repo.SeedRecord(&db.MetadataRecord{
		URL:       "https://example.com",
		Status:    db.StatusPending,
		CreatedAt: time.Now().UTC(),
	})

	f := &fetcher.MockFetcher{Err: errors.New("upstream timeout")}
	flags := featureflags.NewEnvFlags(&config.Config{FFPageSourceStorage: true})
	worker := NewWorker(repo, f, &mockEventConsumer{}, flags, nil, observability.SetupLogger("info", "test-worker", "0.0.0"))

	err := worker.handleMessage(context.Background(), kafka.FetchRequestMessage{
		URL:       "https://example.com",
		RequestID: "req-2",
		Source:    "test",
	})

	require.Error(t, err)
	record := repo.GetRecord("https://example.com")
	require.NotNil(t, record)
	assert.Equal(t, db.StatusFailed, record.Status)
}
