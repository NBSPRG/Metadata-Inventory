package service

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/metadata-inventory/pkg/apperrors"
	"github.com/metadata-inventory/pkg/db"
	"github.com/metadata-inventory/pkg/featureflags"
	"github.com/metadata-inventory/pkg/fetcher"
	"github.com/metadata-inventory/pkg/kafka"
	"github.com/metadata-inventory/pkg/observability"
)

// MetadataServiceImpl is the concrete implementation of MetadataService.
// It orchestrates the repository, Kafka producer, and HTTP fetcher.
type MetadataServiceImpl struct {
	repo     db.MetadataRepository
	producer kafka.EventProducer
	fetcher  fetcher.Fetcher
	flags    featureflags.FeatureFlags
	logger   *slog.Logger
}

// NewMetadataService creates a new MetadataServiceImpl with all dependencies
// injected. None of these dependencies are optional.
func NewMetadataService(
	repo db.MetadataRepository,
	producer kafka.EventProducer,
	f fetcher.Fetcher,
	flags featureflags.FeatureFlags,
	logger *slog.Logger,
) *MetadataServiceImpl {
	return &MetadataServiceImpl{
		repo:     repo,
		producer: producer,
		fetcher:  f,
		flags:    flags,
		logger:   logger,
	}
}

// SubmitURL processes a POST /v1/metadata request.
// It is idempotent: if the URL already has status=ready, return existing record.
// Otherwise, fetch the URL, persist metadata, and return the new record.
func (s *MetadataServiceImpl) SubmitURL(ctx context.Context, url string) (*db.MetadataRecord, bool, error) {
	log := observability.LoggerFromContext(ctx, s.logger)

	// Check for existing record (idempotency)
	existing, err := s.repo.FindByURL(ctx, url)
	if err != nil {
		return nil, false, fmt.Errorf("%w: %v", apperrors.ErrDatabaseError, err)
	}

	// If already ready, return existing (idempotent POST)
	if existing != nil && existing.Status == db.StatusReady {
		log.Info("url already fetched — returning cached",
			slog.String("url", url),
			slog.String("status", string(existing.Status)),
		)
		return existing, false, nil
	}

	// If pending/fetching, return the existing record as-is
	if existing != nil && (existing.Status == db.StatusPending || existing.Status == db.StatusFetching) {
		log.Info("url already in progress",
			slog.String("url", url),
			slog.String("status", string(existing.Status)),
		)
		return existing, false, nil
	}

	// Create a pending record first
	record := &db.MetadataRecord{
		URL:       url,
		Status:    db.StatusPending,
		CreatedAt: time.Now().UTC(),
	}

	if err := s.repo.Upsert(ctx, record); err != nil {
		return nil, false, fmt.Errorf("%w: %v", apperrors.ErrDatabaseError, err)
	}

	// Synchronous fetch for POST requests
	includePageSource := s.flags.IsEnabled(featureflags.PageSourceStorage)

	log.Info("fetching url", slog.String("url", url))

	// Mark as fetching
	if err := s.repo.UpdateStatus(ctx, url, db.StatusFetching, nil); err != nil {
		log.Warn("failed to mark as fetching", slog.String("error", err.Error()))
	}

	result, fetchErr := s.fetcher.Fetch(url, includePageSource)
	if fetchErr != nil {
		// Mark as failed
		if err := s.repo.UpdateStatus(ctx, url, db.StatusFailed, nil); err != nil {
			log.Error("failed to update status to failed", slog.String("error", err.Error()))
		}
		return nil, false, fmt.Errorf("%w: %v", apperrors.ErrFetchFailed, fetchErr)
	}

	// Update record with fetch results
	fetchResult := &db.FetchResult{
		Headers:             result.Headers,
		Cookies:             convertCookies(result.Cookies),
		PageSource:          result.PageSource,
		PageSourceSizeBytes: result.PageSourceSizeBytes,
		FetchDurationMs:     result.FetchDurationMs,
		FetchedAt:           result.FetchedAt,
	}

	if err := s.repo.UpdateStatus(ctx, url, db.StatusReady, fetchResult); err != nil {
		return nil, false, fmt.Errorf("%w: %v", apperrors.ErrDatabaseError, err)
	}

	// Re-read the final record
	finalRecord, err := s.repo.FindByURL(ctx, url)
	if err != nil {
		return nil, false, fmt.Errorf("%w: %v", apperrors.ErrDatabaseError, err)
	}

	log.Info("url fetched and stored",
		slog.String("url", url),
		slog.Int64("duration_ms", result.FetchDurationMs),
	)

	return finalRecord, true, nil
}

// GetMetadata processes a GET /v1/metadata request.
// Returns cached data if available, otherwise dispatches async Kafka job.
func (s *MetadataServiceImpl) GetMetadata(ctx context.Context, url string) (*db.MetadataRecord, bool, error) {
	log := observability.LoggerFromContext(ctx, s.logger)

	// Check cache
	record, err := s.repo.FindByURL(ctx, url)
	if err != nil {
		return nil, false, fmt.Errorf("%w: %v", apperrors.ErrDatabaseError, err)
	}

	// Cache hit — return immediately
	if record != nil && record.Status == db.StatusReady {
		log.Info("cache hit", slog.String("url", url))
		return record, false, nil
	}

	// If already pending/fetching, don't re-dispatch
	if record != nil && (record.Status == db.StatusPending || record.Status == db.StatusFetching) {
		log.Info("url already in progress", slog.String("url", url))
		return nil, true, nil // queued = true
	}

	// Cache miss — create pending record and dispatch async
	pendingRecord := &db.MetadataRecord{
		URL:       url,
		Status:    db.StatusPending,
		CreatedAt: time.Now().UTC(),
	}

	if err := s.repo.Upsert(ctx, pendingRecord); err != nil {
		return nil, false, fmt.Errorf("%w: %v", apperrors.ErrDatabaseError, err)
	}

	// Dispatch async fetch via Kafka
	msg := kafka.FetchRequestMessage{
		Version:     kafka.MessageVersion,
		URL:         url,
		RequestedAt: time.Now().UTC(),
		RequestID:   observability.RequestIDFromContext(ctx),
		Source:      "api-get",
	}

	if err := s.producer.Publish(ctx, msg); err != nil {
		log.Error("failed to publish to kafka — falling back",
			slog.String("url", url),
			slog.String("error", err.Error()),
		)
		return nil, false, fmt.Errorf("%w: %v", apperrors.ErrKafkaError, err)
	}

	log.Info("async fetch dispatched",
		slog.String("url", url),
		slog.String("request_id", msg.RequestID),
	)

	return nil, true, nil // queued = true
}

// convertCookies transforms fetcher.CookieInfo to db.CookieRecord.
func convertCookies(cookies []fetcher.CookieInfo) []db.CookieRecord {
	result := make([]db.CookieRecord, len(cookies))
	for i, c := range cookies {
		result[i] = db.CookieRecord{
			Name:     c.Name,
			Value:    c.Value,
			Domain:   c.Domain,
			Path:     c.Path,
			HTTPOnly: c.HTTPOnly,
			Secure:   c.Secure,
		}
	}
	return result
}
