package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	workerConsumer "github.com/metadata-inventory/worker/consumer"

	"github.com/metadata-inventory/pkg/config"
	"github.com/metadata-inventory/pkg/db"
	"github.com/metadata-inventory/pkg/featureflags"
	"github.com/metadata-inventory/pkg/fetcher"
	"github.com/metadata-inventory/pkg/kafka"
	"github.com/metadata-inventory/pkg/observability"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "FATAL: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	logger := observability.SetupLogger(cfg.LogLevel, cfg.ServiceName+"-worker", cfg.ServiceVersion)
	logger.Info("starting worker service",
		slog.String("environment", cfg.Environment),
		slog.String("kafka_topic", cfg.KafkaTopic),
		slog.String("kafka_group", cfg.KafkaGroupID),
		slog.Int("metrics_port", cfg.WorkerMetricsPort),
		slog.String("version", cfg.ServiceVersion),
	)

	flags := featureflags.NewEnvFlags(cfg)
	metrics := observability.NewMetrics("metadata_worker")
	metricsServer := startMetricsServer(cfg.WorkerMetricsPort, flags.IsEnabled(featureflags.MetricsEnabled), logger)

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

	httpFetcher := fetcher.NewHTTPFetcher(cfg.FetchTimeout, cfg.FetchMaxRedirects, cfg.DisableSSRF)

	kafkaConsumer := kafka.NewKafkaConsumer(
		cfg.KafkaBrokers,
		cfg.KafkaTopic,
		cfg.KafkaGroupID,
		cfg.KafkaDLTTopic,
		cfg.KafkaMaxRetries,
		logger,
	)

	worker := workerConsumer.NewWorker(repo, httpFetcher, kafkaConsumer, flags, metrics, logger)

	shutdownCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		errCh <- worker.Start(shutdownCtx)
	}()

	logger.Info("worker service running")

	select {
	case err := <-errCh:
		if err != nil {
			return fmt.Errorf("worker error: %w", err)
		}
	case <-shutdownCtx.Done():
		logger.Info("shutdown signal received: draining in-flight work")
	}

	metricsShutdownCtx, metricsCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer metricsCancel()
	if err := metricsServer.Shutdown(metricsShutdownCtx); err != nil {
		logger.Error("worker metrics server shutdown error", slog.String("error", err.Error()))
	}

	if err := worker.Close(); err != nil {
		logger.Error("worker close error", slog.String("error", err.Error()))
	}

	logger.Info("worker service stopped gracefully")
	return nil
}

func startMetricsServer(port int, metricsEnabled bool, logger *slog.Logger) *http.Server {
	mux := http.NewServeMux()
	if metricsEnabled {
		mux.Handle("/metrics", promhttp.Handler())
	}
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	srv := &http.Server{
		Addr:    ":" + strconv.Itoa(port),
		Handler: mux,
	}

	go func() {
		logger.Info("worker metrics server starting", slog.String("addr", srv.Addr))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("worker metrics server error", slog.String("error", err.Error()))
		}
	}()

	return srv
}
