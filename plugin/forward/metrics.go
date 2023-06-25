package forward

import (
	"strconv"
	"time"

	"github.com/coredns/coredns/plugin"

	"github.com/miekg/dns"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Variables declared for monitoring.
var (
	RequestCount = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: plugin.Namespace,
		Subsystem: "forward",
		Name:      "requests_total",
		Help:      "Counter of requests made per upstream.",
	}, []string{"to"})
	RcodeCount = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: plugin.Namespace,
		Subsystem: "forward",
		Name:      "responses_total",
		Help:      "Counter of responses received per upstream.",
	}, []string{"to", "rcode"})
	RequestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: plugin.Namespace,
		Subsystem: "forward",
		Name:      "request_duration_seconds",
		Buckets:   plugin.TimeBuckets,
		Help:      "Histogram of the time each request took.",
	}, []string{"to", "rcode"})

	HealthcheckBrokenCount = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: plugin.Namespace,
		Subsystem: "forward",
		Name:      "healthcheck_broken_total",
		Help:      "Counter of the number of complete failures of the healthchecks.",
	})
	MaxConcurrentRejectCount = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: plugin.Namespace,
		Subsystem: "forward",
		Name:      "max_concurrent_rejects_total",
		Help:      "Counter of the number of queries rejected because the concurrent queries were at maximum.",
	})
)

func recordReqMetrics(addr string, start time.Time, res *dns.Msg) {
	RequestCount.WithLabelValues(addr).Add(1)

	if res != nil {
		// Record metrics
		rc, ok := dns.RcodeToString[res.Rcode]
		if !ok {
			rc = strconv.Itoa(res.Rcode)
		}
		RcodeCount.WithLabelValues(addr, rc).Add(1)
		RequestDuration.WithLabelValues(addr, rc).Observe(time.Since(start).Seconds())
	}
}
