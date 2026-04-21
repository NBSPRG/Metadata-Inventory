package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/metadata-inventory/pkg/apperrors"
	"github.com/metadata-inventory/pkg/config"
	"github.com/metadata-inventory/pkg/db"
	"github.com/metadata-inventory/pkg/featureflags"
	"github.com/metadata-inventory/pkg/fetcher"
	"github.com/metadata-inventory/pkg/kafka"
	"github.com/metadata-inventory/pkg/observability"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestService(repo *db.MockRepository, prod *kafka.MockProducer, f *fetcher.MockFetcher) *MetadataServiceImpl {
	cfg := &config.Config{FFPageSourceStorage: true, FFMetricsEnabled: true}
	flags := featureflags.NewEnvFlags(cfg)
	logger := observability.SetupLogger("info", "test", "0.0.0")
	return NewMetadataService(repo, prod, f, flags, logger)
}

func newTestServiceWithConfig(repo *db.MockRepository, prod *kafka.MockProducer, f *fetcher.MockFetcher, cfg *config.Config) *MetadataServiceImpl {
	flags := featureflags.NewEnvFlags(cfg)
	logger := observability.SetupLogger("info", "test", "0.0.0")
	return NewMetadataService(repo, prod, f, flags, logger)
}

func TestSubmitURL_NewURL(t *testing.T) {
	repo := db.NewMockRepository()
	prod := &kafka.MockProducer{}
	f := &fetcher.MockFetcher{
		Result: &fetcher.FetchResult{
			StatusCode:      200,
			Headers:         map[string][]string{"Content-Type": {"text/html"}},
			Cookies:         []fetcher.CookieInfo{{Name: "id", Value: "abc"}},
			FetchDurationMs: 100,
			FetchedAt:       time.Now().UTC(),
		},
	}

	svc := newTestService(repo, prod, f)
	record, isNew, err := svc.SubmitURL(context.Background(), "https://example.com")

	require.NoError(t, err)
	assert.True(t, isNew)
	assert.NotNil(t, record)
	assert.Equal(t, db.StatusReady, record.Status)
	assert.Equal(t, 1, f.Called)
}

func TestSubmitURL_ExistingReady(t *testing.T) {
	repo := db.NewMockRepository()
	repo.SeedRecord(&db.MetadataRecord{
		URL:     "https://example.com",
		Status:  db.StatusReady,
		Headers: map[string][]string{"Content-Type": {"text/html"}},
	})
	prod := &kafka.MockProducer{}
	f := &fetcher.MockFetcher{}

	svc := newTestService(repo, prod, f)
	record, isNew, err := svc.SubmitURL(context.Background(), "https://example.com")

	require.NoError(t, err)
	assert.False(t, isNew) // Idempotent — not new
	assert.Equal(t, db.StatusReady, record.Status)
	assert.Equal(t, 0, f.Called) // No fetch happened
}

func TestSubmitURL_FetchFails(t *testing.T) {
	repo := db.NewMockRepository()
	prod := &kafka.MockProducer{}
	f := &fetcher.MockFetcher{Err: errors.New("connection timeout")}

	svc := newTestService(repo, prod, f)
	_, _, err := svc.SubmitURL(context.Background(), "https://failing.com")

	assert.Error(t, err)
	assert.True(t, errors.Is(err, apperrors.ErrFetchFailed))
}

func TestSubmitURL_AsyncFetchOnlyQueuesKafka(t *testing.T) {
	repo := db.NewMockRepository()
	prod := &kafka.MockProducer{}
	f := &fetcher.MockFetcher{}

	svc := newTestServiceWithConfig(repo, prod, f, &config.Config{
		FFPageSourceStorage: true,
		FFMetricsEnabled:    true,
		FFAsyncFetchOnly:    true,
	})

	record, isNew, err := svc.SubmitURL(context.Background(), "https://example.com")

	require.NoError(t, err)
	assert.True(t, isNew)
	require.NotNil(t, record)
	assert.Equal(t, db.StatusPending, record.Status)
	assert.Equal(t, 0, f.Called)
	require.Len(t, prod.Messages, 1)
	assert.Equal(t, "api-post", prod.Messages[0].Source)
}

func TestGetMetadata_CacheHit(t *testing.T) {
	repo := db.NewMockRepository()
	repo.SeedRecord(&db.MetadataRecord{
		URL:    "https://cached.com",
		Status: db.StatusReady,
	})
	prod := &kafka.MockProducer{}
	f := &fetcher.MockFetcher{}

	svc := newTestService(repo, prod, f)
	record, queued, err := svc.GetMetadata(context.Background(), "https://cached.com")

	require.NoError(t, err)
	assert.False(t, queued)
	assert.NotNil(t, record)
	assert.Equal(t, db.StatusReady, record.Status)
	assert.Empty(t, prod.Messages) // No Kafka message on cache hit
}

func TestGetMetadata_CacheMiss_DispatchesKafka(t *testing.T) {
	repo := db.NewMockRepository()
	prod := &kafka.MockProducer{}
	f := &fetcher.MockFetcher{}

	svc := newTestService(repo, prod, f)
	record, queued, err := svc.GetMetadata(context.Background(), "https://new.com")

	require.NoError(t, err)
	assert.True(t, queued)
	assert.Nil(t, record) // No record yet

	// Verify Kafka message was published
	require.Len(t, prod.Messages, 1)
	assert.Equal(t, "https://new.com", prod.Messages[0].URL)
	assert.Equal(t, "api-get", prod.Messages[0].Source)
}

func TestGetMetadata_AlreadyPending(t *testing.T) {
	repo := db.NewMockRepository()
	repo.SeedRecord(&db.MetadataRecord{
		URL:    "https://pending.com",
		Status: db.StatusPending,
	})
	prod := &kafka.MockProducer{}
	f := &fetcher.MockFetcher{}

	svc := newTestService(repo, prod, f)
	record, queued, err := svc.GetMetadata(context.Background(), "https://pending.com")

	require.NoError(t, err)
	assert.True(t, queued)
	assert.Nil(t, record)
	assert.Empty(t, prod.Messages) // No re-dispatch
}
