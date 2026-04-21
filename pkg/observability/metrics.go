package observability

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Metrics holds all Prometheus metric instruments for the service.
// All metrics are registered via promauto (auto-registered with the
// default prometheus registry).
type Metrics struct {
	// HTTP metrics
	HTTPRequestsTotal    *prometheus.CounterVec
	HTTPRequestDuration  *prometheus.HistogramVec

	// Metadata cache metrics
	MetadataCacheHits   prometheus.Counter
	MetadataCacheMisses prometheus.Counter

	// Fetcher metrics
	FetchDuration    *prometheus.HistogramVec
	FetchErrorsTotal *prometheus.CounterVec

	// Kafka metrics
	KafkaMessagesProduced *prometheus.CounterVec
	KafkaMessagesConsumed *prometheus.CounterVec

	// MongoDB metrics
	MongoOperationDuration *prometheus.HistogramVec
}

// NewMetrics creates and registers all Prometheus metrics for the service.
func NewMetrics(namespace string) *Metrics {
	return &Metrics{
		HTTPRequestsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "http_requests_total",
				Help:      "Total number of HTTP requests by method, path, and status code.",
			},
			[]string{"method", "path", "status_code"},
		),
		HTTPRequestDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "http_request_duration_seconds",
				Help:      "HTTP request duration in seconds.",
				Buckets:   prometheus.DefBuckets,
			},
			[]string{"method", "path"},
		),

		MetadataCacheHits: promauto.NewCounter(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "metadata_cache_hits_total",
				Help:      "Total number of metadata cache hits (URL already fetched).",
			},
		),
		MetadataCacheMisses: promauto.NewCounter(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "metadata_cache_misses_total",
				Help:      "Total number of metadata cache misses (URL not yet fetched).",
			},
		),

		FetchDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "fetch_duration_seconds",
				Help:      "Duration of HTTP fetch operations in seconds.",
				Buckets:   []float64{0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30},
			},
			[]string{"status"},
		),
		FetchErrorsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "worker_fetch_errors_total",
				Help:      "Total number of fetch errors by error type.",
			},
			[]string{"error_type"},
		),

		KafkaMessagesProduced: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "kafka_messages_produced_total",
				Help:      "Total number of Kafka messages produced by topic.",
			},
			[]string{"topic"},
		),
		KafkaMessagesConsumed: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "kafka_messages_consumed_total",
				Help:      "Total number of Kafka messages consumed by topic and status.",
			},
			[]string{"topic", "status"},
		),

		MongoOperationDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "mongodb_operation_duration_seconds",
				Help:      "Duration of MongoDB operations in seconds.",
				Buckets:   []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1},
			},
			[]string{"operation"},
		),
	}
}
