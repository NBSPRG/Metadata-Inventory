// Package featureflags provides a simple, interface-driven feature flag
// system. The default implementation reads flags from environment variables
// (set at startup via config). The interface allows swapping to LaunchDarkly,
// Unleash, or a DB-backed implementation without changing consumer code.
package featureflags

import "github.com/metadata-inventory/pkg/config"

// Flag represents a named feature flag.
type Flag string

const (
	RateLimitEnabled      Flag = "FF_RATE_LIMIT_ENABLED"
	CircuitBreakerEnabled Flag = "FF_CIRCUIT_BREAKER_ENABLED"
	PageSourceStorage     Flag = "FF_PAGE_SOURCE_STORAGE"
	AsyncFetchOnly        Flag = "FF_ASYNC_FETCH_ONLY"
	MetricsEnabled        Flag = "FF_METRICS_ENABLED"
	TracingEnabled        Flag = "FF_TRACING_ENABLED"
)

// FeatureFlags defines the interface for querying feature flags.
// Consumers depend on this interface, allowing the backing implementation
// to be swapped without code changes.
type FeatureFlags interface {
	IsEnabled(flag Flag) bool
}

// EnvFlags implements FeatureFlags using values loaded from config (env vars).
type EnvFlags struct {
	flags map[Flag]bool
}

// NewEnvFlags creates an EnvFlags instance from the loaded configuration.
func NewEnvFlags(cfg *config.Config) *EnvFlags {
	return &EnvFlags{
		flags: map[Flag]bool{
			RateLimitEnabled:      cfg.FFRateLimitEnabled,
			CircuitBreakerEnabled: cfg.FFCircuitBreakerEnabled,
			PageSourceStorage:     cfg.FFPageSourceStorage,
			AsyncFetchOnly:        cfg.FFAsyncFetchOnly,
			MetricsEnabled:        cfg.FFMetricsEnabled,
			TracingEnabled:        cfg.FFTracingEnabled,
		},
	}
}

// IsEnabled returns true if the given flag is enabled.
// Returns false for unknown flags (safe default).
func (f *EnvFlags) IsEnabled(flag Flag) bool {
	return f.flags[flag]
}
