package template

import (
	"github.com/coredns/coredns/plugin"
	"github.com/prometheus/client_golang/prometheus"
)

// metrics collection
type metric struct {

	MatchCount *prometheus.CounterVec
	FailureCount *prometheus.CounterVec
	RRFailureCount *prometheus.CounterVec
}

// newMetrics returns a new metrics collector set
func newMetric() *metric {
	return &metric{
		MatchCount: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: plugin.Namespace,
			Subsystem: "template",
			Name:      "matches_total",
			Help:      "Counter of template regex matches.",
		}, []string{"server", "zone", "class", "type"}),
		FailureCount: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: plugin.Namespace,
			Subsystem: "template",
			Name:      "template_failures_total",
			Help:      "Counter of go template failures.",
		}, []string{"server", "zone", "class", "type", "section", "template"}),
		RRFailureCount: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: plugin.Namespace,
			Subsystem: "template",
			Name:      "rr_failures_total",
			Help:      "Counter of mis-templated RRs.",
		}, []string{"server", "zone", "class", "type", "section", "template"}),
	}
}

// Collectors returns a list of all collectors in this set
func (m *metric) Collectors() []prometheus.Collector {
	return []prometheus.Collector{
		m.MatchCount,
		m.FailureCount,
		m.RRFailureCount,
	}
}