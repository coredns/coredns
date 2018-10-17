package health

import (
	"github.com/coredns/coredns/plugin"
	"github.com/prometheus/client_golang/prometheus"
)

// metrics collection
type metric struct {
	Duration prometheus.Histogram
}

// newMetrics returns a new metrics collector set
func newMetric() *metric {
	return &metric{
		Duration: prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace: plugin.Namespace,
			Subsystem: "health",
			Name:      "request_duration_seconds",
			Buckets:   plugin.TimeBuckets,
			Help:      "Histogram of the time (in seconds) each request took.",
		}),
	}
}

// Collectors returns a list of all collectors in this set
func (m *metric) Collectors() []prometheus.Collector {
	return []prometheus.Collector{
		m.Duration,
	}
}