package dnssec

import (
	"github.com/coredns/coredns/plugin"
	"github.com/prometheus/client_golang/prometheus"
)

// metrics collection
type metric struct {
	Size *prometheus.GaugeVec
	Hits *prometheus.CounterVec
	Misses *prometheus.CounterVec
}

// newMetrics returns a new metrics collector set
func newMetric() *metric {
	return &metric{
		Size: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: plugin.Namespace,
			Subsystem: "dnssec",
			Name:      "cache_size",
			Help:      "The number of elements in the dnssec cache.",
		}, []string{"server", "type"}),
		Hits: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: plugin.Namespace,
			Subsystem: "dnssec",
			Name:      "cache_hits_total",
			Help:      "The count of cache hits.",
		}, []string{"server"}),
		Misses: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: plugin.Namespace,
			Subsystem: "dnssec",
			Name:      "cache_misses_total",
			Help:      "The count of cache misses.",
		}, []string{"server"}),
	}
}

// Collectors returns a list of all collectors in this set
func (m *metric) Collectors() []prometheus.Collector {
	return []prometheus.Collector{
		m.Size,
		m.Hits,
		m.Misses,
	}
}