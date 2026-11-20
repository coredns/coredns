package proxy

import (
	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/request"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// requestDurationMetrics exports one metric family. Each observation goes to
// either the ordinary histogram or the histogram with query labels, never both.
type requestDurationMetrics struct {
	ordinary *prometheus.HistogramVec
	query    *prometheus.HistogramVec
}

func newRequestDurationMetrics() *requestDurationMetrics {
	opts := prometheus.HistogramOpts{
		Namespace:                   plugin.Namespace,
		Subsystem:                   "proxy",
		Name:                        "request_duration_seconds",
		Buckets:                     plugin.TimeBuckets,
		NativeHistogramBucketFactor: plugin.NativeHistogramBucketFactor,
		Help:                        "Histogram of the time each request took.",
	}
	// Register only the combined collector, not its individual vectors.
	factory := promauto.With(nil)
	return &requestDurationMetrics{
		ordinary: factory.NewHistogramVec(opts, []string{"proxy_name", "to", "rcode"}),
		query:    factory.NewHistogramVec(opts, []string{"proxy_name", "to", "rcode", "query_name", "query_type"}),
	}
}

// Describe leaves this collector unchecked because forwarders can independently
// opt in to query labels. Checked collectors require one fixed label schema per
// metric name. Both schemas use standard HistogramVecs and disjoint label sets.
func (*requestDurationMetrics) Describe(chan<- *prometheus.Desc) {}

func (m *requestDurationMetrics) Collect(ch chan<- prometheus.Metric) {
	m.ordinary.Collect(ch)
	m.query.Collect(ch)
}

// SetQueryTypesForMetrics selects the query types that receive query_name and
// query_type metric labels. Call this before starting the proxy; an empty list
// disables query labels. The caller's slice is not retained.
func (p *Proxy) SetQueryTypesForMetrics(types []uint16) {
	p.qtypesForMetrics = nil
	if len(types) == 0 {
		return
	}
	p.qtypesForMetrics = make(map[uint16]struct{}, len(types))
	for _, qtype := range types {
		p.qtypesForMetrics[qtype] = struct{}{}
	}
}

func (p *Proxy) observeRequestDuration(state request.Request, rcode string, elapsed float64) {
	requestDuration.observe(p, state, rcode, elapsed)
}

func (m *requestDurationMetrics) observe(p *Proxy, state request.Request, rcode string, elapsed float64) {
	if len(p.qtypesForMetrics) != 0 && state.Req != nil && len(state.Req.Question) != 0 {
		if _, ok := p.qtypesForMetrics[state.QType()]; ok {
			m.query.WithLabelValues(p.proxyName, p.addr, rcode, state.Name(), state.Type()).Observe(elapsed)
			return
		}
	}
	m.ordinary.WithLabelValues(p.proxyName, p.addr, rcode).Observe(elapsed)
}
