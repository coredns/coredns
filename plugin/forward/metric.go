package forward

import (
	"github.com/coredns/coredns/plugin"
	"github.com/prometheus/client_golang/prometheus"
)

// metrics collection
type metric struct {

	RCodeCount *prometheus.CounterVec
	RequestCount *prometheus.CounterVec
	RequestDuration *prometheus.HistogramVec
	HealthcheckFailure *prometheus.CounterVec
	HealthcheckBroken prometheus.Counter
	Sockets *prometheus.GaugeVec
}

// newMetrics returns a new metrics collector set
func newMetric() *metric {
	return &metric{
		RCodeCount: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: plugin.Namespace,
			Subsystem: "forward",
			Name:      "response_rcode_count_total",
			Help:      "Counter of requests made per upstream.",
		}, []string{"rcode", "to"}),
		RequestCount: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: plugin.Namespace,
			Subsystem: "forward",
			Name:      "response_rcode_count_total",
			Help:      "Counter of requests made per upstream.",
		}, []string{"rcode", "to"}),
		RequestDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: plugin.Namespace,
			Subsystem: "forward",
			Name:      "request_duration_seconds",
			Buckets:   plugin.TimeBuckets,
			Help:      "Histogram of the time each request took.",
		}, []string{"to"}),
		HealthcheckFailure: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: plugin.Namespace,
			Subsystem: "forward",
			Name:      "healthcheck_failure_count_total",
			Help:      "Counter of the number of failed healtchecks.",
		}, []string{"to"}),
		HealthcheckBroken: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: plugin.Namespace,
			Subsystem: "forward",
			Name:      "healthcheck_broken_count_total",
			Help:      "Counter of the number of complete failures of the healtchecks.",
		}),
		Sockets: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: plugin.Namespace,
			Subsystem: "forward",
			Name:      "sockets_open",
			Help:      "Gauge of open sockets per upstream.",
		}, []string{"to"}),
	}
}

// Collectors returns a list of all collectors in this set
func (m *metric) Collectors() []prometheus.Collector {
	return []prometheus.Collector{
		m.RCodeCount,
		m.RequestCount,
		m.RequestDuration,
		m.HealthcheckFailure,
		m.HealthcheckBroken,
		m.Sockets,
	}
}
