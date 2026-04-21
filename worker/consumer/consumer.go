// Worker service consumer that processes Kafka messages.
// It receives fetch requests, executes the HTTP fetch pipeline,
// and persists results to MongoDB.
package consumer

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/metadata-inventory/pkg/db"
	"github.com/metadata-inventory/pkg/featureflags"
	"github.com/metadata-inventory/pkg/fetcher"
	"github.com/metadata-inventory/pkg/kafka"
	"github.com/metadata-inventory/pkg/observability"
)

// Worker processes Kafka messages by fetching URLs and storing metadata.
type Worker struct {
	repo     db.MetadataRepository
	fetcher  fetcher.Fetcher
	consumer kafka.EventConsumer
	flags    featureflags.FeatureFlags
	metrics  *observability.Metrics
	logger   *slog.Logger
}

// NewWorker creates a new Worker with all dependencies injected.
func NewWorker(
	repo db.MetadataRepository,
	f fetcher.Fetcher,
	consumer kafka.EventConsumer,
	flags featureflags.FeatureFlags,
	metrics *observability.Metrics,
	logger *slog.Logger,
) *Worker {
	return &Worker{
		repo:     repo,
		fetcher:  f,
		consumer: consumer,
		flags:    flags,
		metrics:  metrics,
		logger:   logger,
	}
}

// Start begins processing Kafka messages. It blocks until the context
// is cancelled (graceful shutdown).
func (w *Worker) Start(ctx context.Context) error {
	w.logger.Info("worker starting — listening for fetch requests")
	return w.consumer.Start(ctx, w.handleMessage)
}

// handleMessage processes a single FetchRequestMessage from Kafka.
// Pipeline: validate → mark fetching → fetch → update DB (ready/failed).
func (w *Worker) handleMessage(ctx context.Context, msg kafka.FetchRequestMessage) error {
	log := w.logger.With(
		slog.String("url", msg.URL),
		slog.String("request_id", msg.RequestID),
		slog.String("source", msg.Source),
	)

	log.Info("processing fetch request")

	// Mark as fetching (prevents duplicate concurrent fetches)
	if err := w.repo.UpdateStatus(ctx, msg.URL, db.StatusFetching, nil); err != nil {
		log.Warn("failed to mark as fetching", slog.String("error", err.Error()))
		// Continue anyway — best effort status update
	}

	// Fetch URL metadata
	includePageSource := w.flags.IsEnabled(featureflags.PageSourceStorage)

	timer := observability.NewTimer()
	result, err := w.fetcher.Fetch(msg.URL, includePageSource)
	fetchDuration := timer.Elapsed()

	if err != nil {
		log.Error("fetch failed",
			slog.String("error", err.Error()),
			slog.Float64("duration_s", fetchDuration),
		)

		// Update DB status to failed
		if dbErr := w.repo.UpdateStatus(ctx, msg.URL, db.StatusFailed, nil); dbErr != nil {
			log.Error("failed to update status to failed", slog.String("error", dbErr.Error()))
		}

		// Record metrics
		if w.metrics != nil {
			w.metrics.FetchDuration.WithLabelValues("failure").Observe(fetchDuration)
			w.metrics.FetchErrorsTotal.WithLabelValues("fetch_error").Inc()
		}

		return fmt.Errorf("fetch url %s: %w", msg.URL, err)
	}

	// Success — update DB with results
	fetchResult := &db.FetchResult{
		Headers:             result.Headers,
		Cookies:             convertCookies(result.Cookies),
		PageSource:          result.PageSource,
		PageSourceSizeBytes: result.PageSourceSizeBytes,
		FetchDurationMs:     result.FetchDurationMs,
		FetchedAt:           result.FetchedAt,
	}

	if err := w.repo.UpdateStatus(ctx, msg.URL, db.StatusReady, fetchResult); err != nil {
		log.Error("failed to update status to ready", slog.String("error", err.Error()))
		return fmt.Errorf("update db for %s: %w", msg.URL, err)
	}

	// Record metrics
	if w.metrics != nil {
		w.metrics.FetchDuration.WithLabelValues("success").Observe(fetchDuration)
	}

	log.Info("fetch completed",
		slog.Int("status_code", result.StatusCode),
		slog.Int64("duration_ms", result.FetchDurationMs),
		slog.Int("page_source_bytes", result.PageSourceSizeBytes),
	)

	return nil
}

// Close shuts down the Kafka consumer gracefully.
func (w *Worker) Close() error {
	return w.consumer.Close()
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
