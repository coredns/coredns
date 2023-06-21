package proxy

import (
	"github.com/coredns/coredns/plugin"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Variables declared for monitoring.
var (
	DefaultMetrics = NewMetrics()
)

type Metrics struct {
	proxyMetrics
	transportMetrics
	healthCheckMetrics
}

type proxyMetrics struct {
	RequestCount    *prometheus.CounterVec
	RcodeCount      *prometheus.CounterVec
	RequestDuration *prometheus.HistogramVec
}

type transportMetrics struct {
	ConnCacheHitsCount   *prometheus.CounterVec
	ConnCacheMissesCount *prometheus.CounterVec
}

type healthCheckMetrics struct {
	HealthcheckFailureCount *prometheus.CounterVec
}

func NewMetrics() *Metrics {
	return NewMetricsWithSubsystem("proxy")
}

func NewMetricsWithSubsystem(subsystem string) *Metrics {
	m := &Metrics{
		proxyMetrics: proxyMetrics{
			RequestCount: promauto.NewCounterVec(prometheus.CounterOpts{
				Namespace: plugin.Namespace,
				Subsystem: subsystem,
				Name:      "requests_total",
				Help:      "Counter of requests made per upstream.",
			}, []string{"to"}),
			RcodeCount: promauto.NewCounterVec(prometheus.CounterOpts{
				Namespace: plugin.Namespace,
				Subsystem: subsystem,
				Name:      "responses_total",
				Help:      "Counter of responses received per upstream.",
			}, []string{"rcode", "to"}),
			RequestDuration: promauto.NewHistogramVec(prometheus.HistogramOpts{
				Namespace: plugin.Namespace,
				Subsystem: subsystem,
				Name:      "request_duration_seconds",
				Buckets:   plugin.TimeBuckets,
				Help:      "Histogram of the time each request took.",
			}, []string{"to", "rcode"}),
		},
		healthCheckMetrics: healthCheckMetrics{
			HealthcheckFailureCount: promauto.NewCounterVec(prometheus.CounterOpts{
				Namespace: plugin.Namespace,
				Subsystem: subsystem,
				Name:      "healthcheck_failures_total",
				Help:      "Counter of the number of failed healthchecks.",
			}, []string{"to"}),
		},
		transportMetrics: transportMetrics{
			ConnCacheHitsCount: promauto.NewCounterVec(prometheus.CounterOpts{
				Namespace: plugin.Namespace,
				Subsystem: subsystem,
				Name:      "conn_cache_hits_total",
				Help:      "Counter of connection cache hits per upstream and protocol.",
			}, []string{"to", "proto"}),
			ConnCacheMissesCount: promauto.NewCounterVec(prometheus.CounterOpts{
				Namespace: plugin.Namespace,
				Subsystem: subsystem,
				Name:      "conn_cache_misses_total",
				Help:      "Counter of connection cache misses per upstream and protocol.",
			}, []string{"to", "proto"}),
		},
	}

	return m
}
