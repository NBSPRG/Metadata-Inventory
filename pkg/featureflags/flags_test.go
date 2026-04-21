package featureflags

import (
	"testing"

	"github.com/metadata-inventory/pkg/config"
	"github.com/stretchr/testify/assert"
)

func TestEnvFlags_IsEnabled(t *testing.T) {
	cfg := &config.Config{
		FFRateLimitEnabled:      true,
		FFCircuitBreakerEnabled: false,
		FFPageSourceStorage:     true,
		FFAsyncFetchOnly:        false,
		FFMetricsEnabled:        true,
		FFTracingEnabled:        false,
	}

	flags := NewEnvFlags(cfg)

	assert.True(t, flags.IsEnabled(RateLimitEnabled))
	assert.False(t, flags.IsEnabled(CircuitBreakerEnabled))
	assert.True(t, flags.IsEnabled(PageSourceStorage))
	assert.False(t, flags.IsEnabled(AsyncFetchOnly))
	assert.True(t, flags.IsEnabled(MetricsEnabled))
	assert.False(t, flags.IsEnabled(TracingEnabled))
}

func TestEnvFlags_UnknownFlag(t *testing.T) {
	cfg := &config.Config{}
	flags := NewEnvFlags(cfg)

	// Unknown flags should return false (safe default)
	assert.False(t, flags.IsEnabled(Flag("FF_UNKNOWN")))
}
