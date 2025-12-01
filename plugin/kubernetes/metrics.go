package kubernetes

import (
	"context"
	"net/url"
	"time"

	"github.com/coredns/coredns/plugin"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"k8s.io/client-go/tools/metrics"
)

var (
	// requestLatency measures K8s rest client requests latency grouped by verb and host.
	requestLatency = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace:                   plugin.Namespace,
			Subsystem:                   "kubernetes",
			Name:                        "rest_client_request_duration_seconds",
			Help:                        "Request latency in seconds. Broken down by verb and host.",
			Buckets:                     prometheus.DefBuckets,
			NativeHistogramBucketFactor: plugin.NativeHistogramBucketFactor,
		},
		[]string{"verb", "host"},
	)

	// rateLimiterLatency measures K8s rest client rate limiter latency grouped by verb and host.
	rateLimiterLatency = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace:                   plugin.Namespace,
			Subsystem:                   "kubernetes",
			Name:                        "rest_client_rate_limiter_duration_seconds",
			Help:                        "Client side rate limiter latency in seconds. Broken down by verb and host.",
			Buckets:                     prometheus.DefBuckets,
			NativeHistogramBucketFactor: plugin.NativeHistogramBucketFactor,
		},
		[]string{"verb", "host"},
	)

	// requestResult measures K8s rest client request metrics grouped by status code, method & host.
	requestResult = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: plugin.Namespace,
			Subsystem: "kubernetes",
			Name:      "rest_client_requests_total",
			Help:      "Number of HTTP requests, partitioned by status code, method, and host.",
		},
		[]string{"code", "method", "host"},
	)
)

func init() {
	metrics.Register(metrics.RegisterOpts{
		RequestLatency:     &latencyAdapter{m: requestLatency},
		RateLimiterLatency: &latencyAdapter{m: rateLimiterLatency},
		RequestResult:      &resultAdapter{requestResult},
	})
}

type latencyAdapter struct {
	m *prometheus.HistogramVec
}

func (l *latencyAdapter) Observe(_ context.Context, verb string, u url.URL, latency time.Duration) {
	l.m.WithLabelValues(verb, u.Host).Observe(latency.Seconds())
}

type resultAdapter struct {
	m *prometheus.CounterVec
}

func (r *resultAdapter) Increment(_ context.Context, code, method, host string) {
	r.m.WithLabelValues(code, method, host).Inc()
}
