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

// NewMetadataService creates a new MetadataServiceImpl with all dependencies injected.
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
func (s *MetadataServiceImpl) SubmitURL(ctx context.Context, url string) (*db.MetadataRecord, bool, error) {
	log := observability.LoggerFromContext(ctx, s.logger)

	existing, err := s.repo.FindByURL(ctx, url)
	if err != nil {
		return nil, false, fmt.Errorf("%w: %v", apperrors.ErrDatabaseError, err)
	}

	if existing != nil && existing.Status == db.StatusReady {
		log.Info("url already fetched: returning cached",
			slog.String("url", url),
			slog.String("status", string(existing.Status)),
		)
		return existing, false, nil
	}

	if existing != nil && (existing.Status == db.StatusPending || existing.Status == db.StatusFetching) {
		log.Info("url already in progress",
			slog.String("url", url),
			slog.String("status", string(existing.Status)),
		)
		return existing, false, nil
	}

	record := &db.MetadataRecord{
		URL:       url,
		Status:    db.StatusPending,
		CreatedAt: time.Now().UTC(),
	}

	if err := s.repo.Upsert(ctx, record); err != nil {
		return nil, false, fmt.Errorf("%w: %v", apperrors.ErrDatabaseError, err)
	}

	if s.flags.IsEnabled(featureflags.AsyncFetchOnly) {
		if err := s.dispatchAsyncFetch(ctx, url, "api-post"); err != nil {
			return nil, false, err
		}

		log.Info("async-only mode enabled: queued post request",
			slog.String("url", url),
		)

		queuedRecord, err := s.repo.FindByURL(ctx, url)
		if err != nil {
			return nil, false, fmt.Errorf("%w: %v", apperrors.ErrDatabaseError, err)
		}
		return queuedRecord, true, nil
	}

	includePageSource := s.flags.IsEnabled(featureflags.PageSourceStorage)

	log.Info("fetching url", slog.String("url", url))

	if err := s.repo.UpdateStatus(ctx, url, db.StatusFetching, nil); err != nil {
		log.Warn("failed to mark as fetching", slog.String("error", err.Error()))
	}

	result, fetchErr := s.fetcher.Fetch(url, includePageSource)
	if fetchErr != nil {
		if err := s.repo.UpdateStatus(ctx, url, db.StatusFailed, nil); err != nil {
			log.Error("failed to update status to failed", slog.String("error", err.Error()))
		}
		return nil, false, fmt.Errorf("%w: %v", apperrors.ErrFetchFailed, fetchErr)
	}

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
func (s *MetadataServiceImpl) GetMetadata(ctx context.Context, url string) (*db.MetadataRecord, bool, error) {
	log := observability.LoggerFromContext(ctx, s.logger)

	record, err := s.repo.FindByURL(ctx, url)
	if err != nil {
		return nil, false, fmt.Errorf("%w: %v", apperrors.ErrDatabaseError, err)
	}

	if record != nil && record.Status == db.StatusReady {
		log.Info("cache hit", slog.String("url", url))
		return record, false, nil
	}

	if record != nil && (record.Status == db.StatusPending || record.Status == db.StatusFetching) {
		log.Info("url already in progress", slog.String("url", url))
		return nil, true, nil
	}

	pendingRecord := &db.MetadataRecord{
		URL:       url,
		Status:    db.StatusPending,
		CreatedAt: time.Now().UTC(),
	}

	if err := s.repo.Upsert(ctx, pendingRecord); err != nil {
		return nil, false, fmt.Errorf("%w: %v", apperrors.ErrDatabaseError, err)
	}

	if err := s.dispatchAsyncFetch(ctx, url, "api-get"); err != nil {
		return nil, false, err
	}

	return nil, true, nil
}

func (s *MetadataServiceImpl) dispatchAsyncFetch(ctx context.Context, url, source string) error {
	log := observability.LoggerFromContext(ctx, s.logger)

	msg := kafka.FetchRequestMessage{
		Version:     kafka.MessageVersion,
		URL:         url,
		RequestedAt: time.Now().UTC(),
		RequestID:   observability.RequestIDFromContext(ctx),
		Source:      source,
	}

	if err := s.producer.Publish(ctx, msg); err != nil {
		log.Error("failed to publish to kafka",
			slog.String("url", url),
			slog.String("source", source),
			slog.String("error", err.Error()),
		)
		return fmt.Errorf("%w: %v", apperrors.ErrKafkaError, err)
	}

	log.Info("async fetch dispatched",
		slog.String("url", url),
		slog.String("request_id", msg.RequestID),
		slog.String("source", source),
	)

	return nil
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
