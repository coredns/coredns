package cache

import (
	"github.com/coredns/coredns/plugin"
	"github.com/prometheus/client_golang/prometheus"
)

// metrics collection
type metric struct {
	Size *prometheus.GaugeVec
	Hits *prometheus.CounterVec
	Misses *prometheus.CounterVec
	Prefetches *prometheus.CounterVec
	Drops *prometheus.CounterVec
}

// newMetrics returns a new metrics collector set
func newMetric() *metric {
	return &metric{
		Size: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: plugin.Namespace,
			Subsystem: "cache",
			Name:      "size",
			Help:      "The number of elements in the cache.",
		}, []string{"server", "type"}),
		Hits: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: plugin.Namespace,
			Subsystem: "cache",
			Name:      "hits_total",
			Help:      "The count of cache hits.",
		}, []string{"server", "type"}),
		Misses: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: plugin.Namespace,
			Subsystem: "cache",
			Name:      "misses_total",
			Help:      "The count of cache misses.",
		}, []string{"server"}),
		Prefetches: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: plugin.Namespace,
			Subsystem: "cache",
			Name:      "prefetch_total",
			Help:      "The number of time the cache has prefetched a cached item.",
		}, []string{"server"}),
		Drops: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: plugin.Namespace,
			Subsystem: "cache",
			Name:      "drops_total",
			Help:      "The number responses that are not cached, because the reply is malformed.",
		}, []string{"server"}),
	}
}

// Collectors returns a list of all collectors in this set
func (m *metric) Collectors() []prometheus.Collector {
	return []prometheus.Collector{
		m.Size,
		m.Hits,
		m.Misses,
		m.Prefetches,
		m.Drops,
	}
}