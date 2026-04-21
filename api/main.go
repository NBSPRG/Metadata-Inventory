// API service entry point for the HTTP Metadata Inventory.
// This is the only file that imports concrete implementations —
// all dependency wiring happens here via manual injection.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/metadata-inventory/api/handlers"
	"github.com/metadata-inventory/api/server"
	"github.com/metadata-inventory/pkg/config"
	"github.com/metadata-inventory/pkg/db"
	"github.com/metadata-inventory/pkg/featureflags"
	"github.com/metadata-inventory/pkg/fetcher"
	"github.com/metadata-inventory/pkg/kafka"
	"github.com/metadata-inventory/pkg/observability"
	"github.com/metadata-inventory/pkg/service"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "FATAL: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	// --- Load configuration (fail fast on invalid config) ---
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	// --- Initialize observability ---
	logger := observability.SetupLogger(cfg.LogLevel, cfg.ServiceName, cfg.ServiceVersion)
	logger.Info("starting api service",
		slog.String("environment", cfg.Environment),
		slog.Int("port", cfg.HTTPPort),
		slog.String("version", cfg.ServiceVersion),
	)

	// Feature flags
	flags := featureflags.NewEnvFlags(cfg)
	logFeatureFlags(logger, flags)

	// Metrics
	metrics := observability.NewMetrics("metadata")

	// Tracing
	ctx := context.Background()
	tracer, tracerShutdown, err := observability.SetupTracer(
		ctx, flags.IsEnabled(featureflags.TracingEnabled),
		cfg.ServiceName, cfg.ServiceVersion, cfg.OTelEndpoint,
	)
	if err != nil {
		logger.Warn("tracer setup failed — continuing without tracing", slog.String("error", err.Error()))
	}
	defer func() {
		if tracerShutdown != nil {
			tracerShutdown(context.Background())
		}
	}()

	// --- Connect to MongoDB ---
	mongoDB, err := db.ConnectMongo(ctx, cfg.MongoURI, cfg.MongoDB, cfg.MongoMaxPoolSize, cfg.MongoConnTimeout, logger)
	if err != nil {
		return fmt.Errorf("connect mongo: %w", err)
	}
	defer db.DisconnectMongo(context.Background(), mongoDB, logger)

	repo, err := db.NewMongoRepository(ctx, mongoDB, logger)
	if err != nil {
		return fmt.Errorf("init repository: %w", err)
	}

	// --- Initialize Kafka producer ---
	producer := kafka.NewKafkaProducer(cfg.KafkaBrokers, cfg.KafkaTopic, logger)
	defer producer.Close()

	// --- Initialize HTTP fetcher ---
	httpFetcher := fetcher.NewHTTPFetcher(cfg.FetchTimeout, cfg.FetchMaxRedirects)

	// --- Build service layer ---
	metadataSvc := service.NewMetadataService(repo, producer, httpFetcher, flags, logger)

	// --- Build handlers ---
	postHandler := handlers.NewMetadataPostHandler(metadataSvc)
	getHandler := handlers.NewMetadataGetHandler(metadataSvc)
	healthHandler := handlers.NewHealthHandler()
	readyHandler := handlers.NewReadyHandler(repo, nil) // No consumer in API service

	// --- Build router and server ---
	router := server.NewRouter(logger, metrics, tracer, flags)
	server.RegisterRoutes(router, postHandler, getHandler, healthHandler, readyHandler)

	srv := server.New(server.Config{
		Port:            cfg.HTTPPort,
		ReadTimeout:     cfg.ReadTimeout,
		WriteTimeout:    cfg.WriteTimeout,
		ShutdownTimeout: cfg.ShutdownTimeout,
	}, router, logger)

	// --- Graceful shutdown setup ---
	shutdownCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Start server in goroutine
	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Start()
	}()

	// Wait for shutdown signal or server error
	select {
	case err := <-errCh:
		return fmt.Errorf("server error: %w", err)
	case <-shutdownCtx.Done():
		logger.Info("shutdown signal received")
	}

	// Graceful shutdown with timeout
	shutdownTimeout, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()

	if err := srv.Shutdown(shutdownTimeout); err != nil {
		return fmt.Errorf("server shutdown: %w", err)
	}

	logger.Info("api service stopped gracefully")
	return nil
}

func logFeatureFlags(logger *slog.Logger, flags featureflags.FeatureFlags) {
	logger.Info("feature flags",
		slog.Bool("rate_limit", flags.IsEnabled(featureflags.RateLimitEnabled)),
		slog.Bool("circuit_breaker", flags.IsEnabled(featureflags.CircuitBreakerEnabled)),
		slog.Bool("page_source_storage", flags.IsEnabled(featureflags.PageSourceStorage)),
		slog.Bool("async_fetch_only", flags.IsEnabled(featureflags.AsyncFetchOnly)),
		slog.Bool("metrics", flags.IsEnabled(featureflags.MetricsEnabled)),
		slog.Bool("tracing", flags.IsEnabled(featureflags.TracingEnabled)),
	)
}
