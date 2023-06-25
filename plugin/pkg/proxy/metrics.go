package proxy

import (
	"github.com/coredns/coredns/plugin"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Variables declared for monitoring.
var (
	RequestCount = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: plugin.Namespace,
		Subsystem: "proxy",
		Name:      "requests_total",
		Help:      "Counter of requests made per upstream.",
	}, []string{"proxy_name", "to"})

	RcodeCount = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: plugin.Namespace,
		Subsystem: "proxy",
		Name:      "responses_total",
		Help:      "Counter of responses received per upstream.",
	}, []string{"proxy_name", "to", "rcode"})

	RequestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: plugin.Namespace,
		Subsystem: "proxy",
		Name:      "request_duration_seconds",
		Buckets:   plugin.TimeBuckets,
		Help:      "Histogram of the time each request took.",
	}, []string{"proxy_name", "to", "rcode"})

	HealthcheckFailureCount = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: plugin.Namespace,
		Subsystem: "proxy",
		Name:      "healthcheck_failures_total",
		Help:      "Counter of the number of failed healthchecks.",
	}, []string{"proxy_name", "to"})

	ConnCacheHitsCount = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: plugin.Namespace,
		Subsystem: "proxy",
		Name:      "conn_cache_hits_total",
		Help:      "Counter of connection cache hits per upstream and protocol.",
	}, []string{"proxy_name", "to", "proto"})

	ConnCacheMissesCount = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: plugin.Namespace,
		Subsystem: "proxy",
		Name:      "conn_cache_misses_total",
		Help:      "Counter of connection cache misses per upstream and protocol.",
	}, []string{"proxy_name", "to", "proto"})
)
