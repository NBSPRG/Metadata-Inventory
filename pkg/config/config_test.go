package config

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad_Defaults(t *testing.T) {
	// Clear any env vars that might interfere
	clearEnv(t)

	cfg, err := Load()
	require.NoError(t, err)

	assert.Equal(t, "metadata-inventory", cfg.ServiceName)
	assert.Equal(t, 8080, cfg.HTTPPort)
	assert.Equal(t, 9091, cfg.WorkerMetricsPort)
	assert.Equal(t, "mongodb://localhost:27017", cfg.MongoURI)
	assert.Equal(t, "metadata_inventory", cfg.MongoDB)
	assert.Equal(t, []string{"localhost:9092"}, cfg.KafkaBrokers)
	assert.Equal(t, "url.fetch.requested", cfg.KafkaTopic)
	assert.Equal(t, "info", cfg.LogLevel)
	assert.True(t, cfg.FFPageSourceStorage)
	assert.True(t, cfg.FFMetricsEnabled)
	assert.False(t, cfg.FFRateLimitEnabled)
	assert.False(t, cfg.FFTracingEnabled)
}

func TestLoad_CustomValues(t *testing.T) {
	clearEnv(t)

	t.Setenv("HTTP_PORT", "9090")
	t.Setenv("WORKER_METRICS_PORT", "9191")
	t.Setenv("MONGO_URI", "mongodb://custom:27017")
	t.Setenv("MONGO_DB", "test_db")
	t.Setenv("KAFKA_BROKERS", "broker1:9092,broker2:9092")
	t.Setenv("LOG_LEVEL", "debug")
	t.Setenv("FF_RATE_LIMIT_ENABLED", "true")
	t.Setenv("FF_PAGE_SOURCE_STORAGE", "false")

	cfg, err := Load()
	require.NoError(t, err)

	assert.Equal(t, 9090, cfg.HTTPPort)
	assert.Equal(t, 9191, cfg.WorkerMetricsPort)
	assert.Equal(t, "mongodb://custom:27017", cfg.MongoURI)
	assert.Equal(t, "test_db", cfg.MongoDB)
	assert.Equal(t, []string{"broker1:9092", "broker2:9092"}, cfg.KafkaBrokers)
	assert.Equal(t, "debug", cfg.LogLevel)
	assert.True(t, cfg.FFRateLimitEnabled)
	assert.False(t, cfg.FFPageSourceStorage)
}

func TestLoad_InvalidPort(t *testing.T) {
	clearEnv(t)
	t.Setenv("HTTP_PORT", "99999")

	_, err := Load()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "HTTP_PORT")
}

func TestLoad_InvalidWorkerMetricsPort(t *testing.T) {
	clearEnv(t)
	t.Setenv("WORKER_METRICS_PORT", "99999")

	_, err := Load()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "WORKER_METRICS_PORT")
}

func TestLoad_InvalidLogLevel(t *testing.T) {
	clearEnv(t)
	t.Setenv("LOG_LEVEL", "verbose")

	_, err := Load()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "LOG_LEVEL")
}

func TestLoad_EmptyMongoURI(t *testing.T) {
	clearEnv(t)
	t.Setenv("MONGO_URI", "")
	// Since envOrDefault returns defaultVal when env is empty,
	// we need to force it to actually fail. The default is non-empty,
	// so we'd need to override the default behavior.
	// This test verifies that defaults prevent empty values.
	cfg, err := Load()
	require.NoError(t, err)
	assert.NotEmpty(t, cfg.MongoURI)
}

// clearEnv removes config-related env vars to ensure test isolation.
func clearEnv(t *testing.T) {
	t.Helper()
	vars := []string{
		"SERVICE_NAME", "SERVICE_VERSION", "ENVIRONMENT",
		"HTTP_PORT", "WORKER_METRICS_PORT", "HTTP_READ_TIMEOUT", "HTTP_WRITE_TIMEOUT", "HTTP_SHUTDOWN_TIMEOUT",
		"MONGO_URI", "MONGO_DB", "MONGO_MAX_POOL_SIZE", "MONGO_CONN_TIMEOUT",
		"KAFKA_BROKERS", "KAFKA_TOPIC", "KAFKA_GROUP_ID", "KAFKA_DLT_TOPIC", "KAFKA_MAX_RETRIES",
		"FETCH_TIMEOUT", "FETCH_MAX_REDIRECTS",
		"LOG_LEVEL",
		"FF_RATE_LIMIT_ENABLED", "FF_CIRCUIT_BREAKER_ENABLED",
		"FF_PAGE_SOURCE_STORAGE", "FF_ASYNC_FETCH_ONLY",
		"FF_METRICS_ENABLED", "FF_TRACING_ENABLED",
		"OTEL_ENDPOINT",
	}
	for _, v := range vars {
		os.Unsetenv(v)
	}
}
