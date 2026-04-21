package main

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/metadata-inventory/api/handlers"
	"github.com/metadata-inventory/api/server"
	"github.com/metadata-inventory/pkg/config"
	"github.com/metadata-inventory/pkg/db"
	"github.com/metadata-inventory/pkg/featureflags"
	"github.com/metadata-inventory/pkg/fetcher"
	"github.com/metadata-inventory/pkg/kafka"
	"github.com/metadata-inventory/pkg/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"
)

func newE2EServer(t *testing.T, cfg *config.Config) (*httptest.Server, *db.MockRepository, *kafka.MockProducer, *fetcher.MockFetcher) {
	t.Helper()

	repo := db.NewMockRepository()
	producer := &kafka.MockProducer{}
	mockFetcher := &fetcher.MockFetcher{}
	flags := featureflags.NewEnvFlags(cfg)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	metadataSvc := service.NewMetadataService(repo, producer, mockFetcher, flags, logger)
	postHandler := handlers.NewMetadataPostHandler(metadataSvc)
	getHandler := handlers.NewMetadataGetHandler(metadataSvc)
	healthHandler := handlers.NewHealthHandler()
	readyHandler := handlers.NewReadyHandler(repo, producer)

	router := server.NewRouter(logger, nil, noop.NewTracerProvider().Tracer("test"), flags)
	server.RegisterRoutes(router, postHandler, getHandler, healthHandler, readyHandler)

	return httptest.NewServer(router), repo, producer, mockFetcher
}

func TestE2E_HTTPFlow_SyncPostAndAsyncGetMiss(t *testing.T) {
	t.Parallel()

	ts, _, producer, _ := newE2EServer(t, &config.Config{
		FFPageSourceStorage: true,
		FFAsyncFetchOnly:    false,
		FFMetricsEnabled:    false,
		FFTracingEnabled:    false,
	})
	defer ts.Close()

	healthResp, err := http.Get(ts.URL + "/health")
	require.NoError(t, err)
	defer healthResp.Body.Close()
	require.Equal(t, http.StatusOK, healthResp.StatusCode)

	readyResp, err := http.Get(ts.URL + "/ready")
	require.NoError(t, err)
	defer readyResp.Body.Close()
	require.Equal(t, http.StatusOK, readyResp.StatusCode)

	body := bytes.NewBufferString(`{"url":"https://example.com"}`)
	postResp, err := http.Post(ts.URL+"/v1/metadata", "application/json", body)
	require.NoError(t, err)
	defer postResp.Body.Close()
	require.Equal(t, http.StatusCreated, postResp.StatusCode)

	var created db.MetadataRecord
	require.NoError(t, json.NewDecoder(postResp.Body).Decode(&created))
	assert.Equal(t, "https://example.com", created.URL)
	assert.Equal(t, db.StatusReady, created.Status)

	getHitResp, err := http.Get(ts.URL + "/v1/metadata?url=https://example.com")
	require.NoError(t, err)
	defer getHitResp.Body.Close()
	require.Equal(t, http.StatusOK, getHitResp.StatusCode)

	var cached db.MetadataRecord
	require.NoError(t, json.NewDecoder(getHitResp.Body).Decode(&cached))
	assert.Equal(t, db.StatusReady, cached.Status)

	getMissResp, err := http.Get(ts.URL + "/v1/metadata?url=https://new-example.com")
	require.NoError(t, err)
	defer getMissResp.Body.Close()
	require.Equal(t, http.StatusAccepted, getMissResp.StatusCode)

	var pending handlers.PendingResponse
	require.NoError(t, json.NewDecoder(getMissResp.Body).Decode(&pending))
	assert.Equal(t, "pending", pending.Status)
	assert.Equal(t, "https://new-example.com", pending.URL)
	assert.Equal(t, 5, pending.PollAfterSeconds)

	require.Len(t, producer.Messages, 1)
	assert.Equal(t, "https://new-example.com", producer.Messages[0].URL)
	assert.Equal(t, "api-get", producer.Messages[0].Source)
}

func TestE2E_HTTPFlow_AsyncOnlyPostQueuesKafka(t *testing.T) {
	t.Parallel()

	ts, _, producer, mockFetcher := newE2EServer(t, &config.Config{
		FFPageSourceStorage: true,
		FFAsyncFetchOnly:    true,
		FFMetricsEnabled:    false,
		FFTracingEnabled:    false,
	})
	defer ts.Close()

	body := bytes.NewBufferString(`{"url":"https://queue-only.com"}`)
	postResp, err := http.Post(ts.URL+"/v1/metadata", "application/json", body)
	require.NoError(t, err)
	defer postResp.Body.Close()
	require.Equal(t, http.StatusAccepted, postResp.StatusCode)

	var queued db.MetadataRecord
	require.NoError(t, json.NewDecoder(postResp.Body).Decode(&queued))
	assert.Equal(t, "https://queue-only.com", queued.URL)
	assert.Equal(t, db.StatusPending, queued.Status)
	assert.Equal(t, 0, mockFetcher.Called)

	require.Len(t, producer.Messages, 1)
	assert.Equal(t, "https://queue-only.com", producer.Messages[0].URL)
	assert.Equal(t, "api-post", producer.Messages[0].Source)

	getPendingResp, err := http.Get(ts.URL + "/v1/metadata?url=https://queue-only.com")
	require.NoError(t, err)
	defer getPendingResp.Body.Close()
	require.Equal(t, http.StatusAccepted, getPendingResp.StatusCode)

	require.Len(t, producer.Messages, 1)
}
