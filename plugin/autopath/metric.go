package autopath

import (
	"github.com/coredns/coredns/plugin"
	"github.com/prometheus/client_golang/prometheus"
)

// metrics collection
type metric struct {

	Count *prometheus.CounterVec
}

// newMetrics returns a new metrics collector set
func newMetric() *metric {
	return &metric{
		Count: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: plugin.Namespace,
			Subsystem: "autopath",
			Name:      "success_count_total",
			Help:      "Counter of requests that did autopath.",
		}, []string{"server"}),
	}
}

// Collectors returns a list of all collectors in this set
func (m *metric) Collectors() []prometheus.Collector {
	return []prometheus.Collector{
		m.Count,
	}
}
