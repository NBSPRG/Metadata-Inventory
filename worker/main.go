// Worker service entry point for the HTTP Metadata Inventory.
// Consumes Kafka messages, fetches URL metadata, and persists results.
// All dependency wiring happens here via manual injection.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	workerConsumer "github.com/metadata-inventory/worker/consumer"

	"github.com/metadata-inventory/pkg/config"
	"github.com/metadata-inventory/pkg/db"
	"github.com/metadata-inventory/pkg/featureflags"
	"github.com/metadata-inventory/pkg/fetcher"
	"github.com/metadata-inventory/pkg/kafka"
	"github.com/metadata-inventory/pkg/observability"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "FATAL: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	// --- Load configuration ---
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	// --- Initialize observability ---
	logger := observability.SetupLogger(cfg.LogLevel, cfg.ServiceName+"-worker", cfg.ServiceVersion)
	logger.Info("starting worker service",
		slog.String("environment", cfg.Environment),
		slog.String("kafka_topic", cfg.KafkaTopic),
		slog.String("kafka_group", cfg.KafkaGroupID),
		slog.String("version", cfg.ServiceVersion),
	)

	// Feature flags
	flags := featureflags.NewEnvFlags(cfg)

	// Metrics
	metrics := observability.NewMetrics("metadata_worker")

	// --- Connect to MongoDB ---
	ctx := context.Background()
	mongoDB, err := db.ConnectMongo(ctx, cfg.MongoURI, cfg.MongoDB, cfg.MongoMaxPoolSize, cfg.MongoConnTimeout, logger)
	if err != nil {
		return fmt.Errorf("connect mongo: %w", err)
	}
	defer db.DisconnectMongo(context.Background(), mongoDB, logger)

	repo, err := db.NewMongoRepository(ctx, mongoDB, logger)
	if err != nil {
		return fmt.Errorf("init repository: %w", err)
	}

	// --- Initialize HTTP fetcher ---
	httpFetcher := fetcher.NewHTTPFetcher(cfg.FetchTimeout, cfg.FetchMaxRedirects)

	// --- Initialize Kafka consumer ---
	kafkaConsumer := kafka.NewKafkaConsumer(
		cfg.KafkaBrokers,
		cfg.KafkaTopic,
		cfg.KafkaGroupID,
		cfg.KafkaDLTTopic,
		cfg.KafkaMaxRetries,
		logger,
	)

	// --- Build worker ---
	worker := workerConsumer.NewWorker(repo, httpFetcher, kafkaConsumer, flags, metrics, logger)

	// --- Graceful shutdown setup ---
	shutdownCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Start worker in goroutine
	errCh := make(chan error, 1)
	go func() {
		errCh <- worker.Start(shutdownCtx)
	}()

	logger.Info("worker service running — press Ctrl+C to stop")

	// Wait for shutdown signal or worker error
	select {
	case err := <-errCh:
		if err != nil {
			return fmt.Errorf("worker error: %w", err)
		}
	case <-shutdownCtx.Done():
		logger.Info("shutdown signal received — draining in-flight work")
	}

	// Close worker (closes Kafka consumer)
	if err := worker.Close(); err != nil {
		logger.Error("worker close error", slog.String("error", err.Error()))
	}

	logger.Info("worker service stopped gracefully")
	return nil
}
