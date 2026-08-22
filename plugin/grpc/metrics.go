package grpc

import (
	"github.com/coredns/coredns/plugin"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Variables declared for monitoring.
var (
	RequestCount = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: plugin.Namespace,
		Subsystem: "grpc",
		Name:      "requests_total",
		Help:      "Counter of requests made per upstream.",
	}, []string{"to"})
	RcodeCount = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: plugin.Namespace,
		Subsystem: "grpc",
		Name:      "responses_total",
		Help:      "Counter of requests made per upstream.",
	}, []string{"rcode", "to"})
	RequestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Namespace:                   plugin.Namespace,
		Subsystem:                   "grpc",
		Name:                        "request_duration_seconds",
		Buckets:                     plugin.TimeBuckets,
		NativeHistogramBucketFactor: plugin.NativeHistogramBucketFactor,
		Help:                        "Histogram of the time each request took.",
	}, []string{"to"})

	// PoolHitsCount is the counter of connection pool cache hits.
	PoolHitsCount = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: plugin.Namespace,
		Subsystem: "grpc",
		Name:      "pool_hits_total",
		Help:      "Counter of connection pool cache hits.",
	}, []string{"to"})
	// PoolMissesCount is the counter of connection pool cache misses.
	PoolMissesCount = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: plugin.Namespace,
		Subsystem: "grpc",
		Name:      "pool_misses_total",
		Help:      "Counter of connection pool cache misses.",
	}, []string{"to"})
	// PoolSizeGauge is the gauge of current number of connections in pool per upstream.
	PoolSizeGauge = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: plugin.Namespace,
		Subsystem: "grpc",
		Name:      "pool_size",
		Help:      "Current number of connections in pool per upstream.",
	}, []string{"to"})

	// HealthcheckFailureCount is the counter of health check failures per upstream.
	HealthcheckFailureCount = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: plugin.Namespace,
		Subsystem: "grpc",
		Name:      "healthcheck_failures_total",
		Help:      "Counter of health check failures per upstream.",
	}, []string{"to"})
)
