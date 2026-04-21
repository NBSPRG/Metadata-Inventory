// Package config provides centralized configuration loading and validation
// for the metadata-inventory service. All configuration is read from
// environment variables, with optional .env file support for local development.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

// Config holds all configuration values for the service.
// Every field is populated from environment variables at startup.
type Config struct {
	// Service identity
	ServiceName    string
	ServiceVersion string
	Environment    string // "development", "staging", "production"

	// HTTP server
	HTTPPort          int
	WorkerMetricsPort int
	ReadTimeout       time.Duration
	WriteTimeout      time.Duration
	ShutdownTimeout   time.Duration

	// MongoDB
	MongoURI         string
	MongoDB          string
	MongoMaxPoolSize uint64
	MongoConnTimeout time.Duration

	// Kafka
	KafkaBrokers    []string
	KafkaTopic      string
	KafkaGroupID    string
	KafkaDLTTopic   string
	KafkaMaxRetries int

	// Fetcher
	FetchTimeout      time.Duration
	FetchMaxRedirects int
	DisableSSRF       bool

	// Logging
	LogLevel string // "debug", "info", "warn", "error"

	// Feature Flags
	FFRateLimitEnabled      bool
	FFCircuitBreakerEnabled bool
	FFPageSourceStorage     bool
	FFAsyncFetchOnly        bool
	FFMetricsEnabled        bool
	FFTracingEnabled        bool

	// Tracing (OpenTelemetry)
	OTelEndpoint string
}

// Load reads configuration from environment variables.
// It attempts to load a .env file first (for local development), but does
// not fail if the file is missing. Required values are validated and the
// function returns an error if any are absent.
func Load() (*Config, error) {
	// Best-effort .env loading — ignore error (file may not exist in prod)
	_ = godotenv.Load()

	cfg := &Config{
		ServiceName:    envOrDefault("SERVICE_NAME", "metadata-inventory"),
		ServiceVersion: envOrDefault("SERVICE_VERSION", "0.1.0"),
		Environment:    envOrDefault("ENVIRONMENT", "development"),

		HTTPPort:          envIntOrDefault("HTTP_PORT", 8080),
		WorkerMetricsPort: envIntOrDefault("WORKER_METRICS_PORT", 9091),
		ReadTimeout:       envDurationOrDefault("HTTP_READ_TIMEOUT", 15*time.Second),
		WriteTimeout:      envDurationOrDefault("HTTP_WRITE_TIMEOUT", 15*time.Second),
		ShutdownTimeout:   envDurationOrDefault("HTTP_SHUTDOWN_TIMEOUT", 30*time.Second),

		MongoURI:         envOrDefault("MONGO_URI", "mongodb://localhost:27017"),
		MongoDB:          envOrDefault("MONGO_DB", "metadata_inventory"),
		MongoMaxPoolSize: envUint64OrDefault("MONGO_MAX_POOL_SIZE", 50),
		MongoConnTimeout: envDurationOrDefault("MONGO_CONN_TIMEOUT", 10*time.Second),

		KafkaBrokers:    strings.Split(envOrDefault("KAFKA_BROKERS", "localhost:9092"), ","),
		KafkaTopic:      envOrDefault("KAFKA_TOPIC", "url.fetch.requested"),
		KafkaGroupID:    envOrDefault("KAFKA_GROUP_ID", "metadata-worker-v1"),
		KafkaDLTTopic:   envOrDefault("KAFKA_DLT_TOPIC", "url.fetch.failed"),
		KafkaMaxRetries: envIntOrDefault("KAFKA_MAX_RETRIES", 3),

		FetchTimeout:      envDurationOrDefault("FETCH_TIMEOUT", 30*time.Second),
		FetchMaxRedirects: envIntOrDefault("FETCH_MAX_REDIRECTS", 10),
		DisableSSRF:       envBoolOrDefault("DISABLE_SSRF", false),

		LogLevel: envOrDefault("LOG_LEVEL", "info"),

		FFRateLimitEnabled:      envBoolOrDefault("FF_RATE_LIMIT_ENABLED", false),
		FFCircuitBreakerEnabled: envBoolOrDefault("FF_CIRCUIT_BREAKER_ENABLED", false),
		FFPageSourceStorage:     envBoolOrDefault("FF_PAGE_SOURCE_STORAGE", true),
		FFAsyncFetchOnly:        envBoolOrDefault("FF_ASYNC_FETCH_ONLY", false),
		FFMetricsEnabled:        envBoolOrDefault("FF_METRICS_ENABLED", true),
		FFTracingEnabled:        envBoolOrDefault("FF_TRACING_ENABLED", false),

		OTelEndpoint: envOrDefault("OTEL_ENDPOINT", "localhost:4317"),
	}

	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("config validation: %w", err)
	}

	return cfg, nil
}

// validate checks that all required fields are present and sensible.
func (c *Config) validate() error {
	if c.MongoURI == "" {
		return fmt.Errorf("MONGO_URI is required")
	}
	if c.MongoDB == "" {
		return fmt.Errorf("MONGO_DB is required")
	}
	if len(c.KafkaBrokers) == 0 || c.KafkaBrokers[0] == "" {
		return fmt.Errorf("KAFKA_BROKERS is required")
	}
	if c.HTTPPort <= 0 || c.HTTPPort > 65535 {
		return fmt.Errorf("HTTP_PORT must be between 1 and 65535, got %d", c.HTTPPort)
	}
	if c.WorkerMetricsPort <= 0 || c.WorkerMetricsPort > 65535 {
		return fmt.Errorf("WORKER_METRICS_PORT must be between 1 and 65535, got %d", c.WorkerMetricsPort)
	}
	validLogLevels := map[string]bool{"debug": true, "info": true, "warn": true, "error": true}
	if !validLogLevels[strings.ToLower(c.LogLevel)] {
		return fmt.Errorf("LOG_LEVEL must be one of debug, info, warn, error; got %q", c.LogLevel)
	}
	return nil
}

// --- Helper functions for reading environment variables with defaults ---

func envOrDefault(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

func envIntOrDefault(key string, defaultVal int) int {
	v := os.Getenv(key)
	if v == "" {
		return defaultVal
	}
	i, err := strconv.Atoi(v)
	if err != nil {
		return defaultVal
	}
	return i
}

func envUint64OrDefault(key string, defaultVal uint64) uint64 {
	v := os.Getenv(key)
	if v == "" {
		return defaultVal
	}
	i, err := strconv.ParseUint(v, 10, 64)
	if err != nil {
		return defaultVal
	}
	return i
}

func envBoolOrDefault(key string, defaultVal bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return defaultVal
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return defaultVal
	}
	return b
}

func envDurationOrDefault(key string, defaultVal time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return defaultVal
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return defaultVal
	}
	return d
}
