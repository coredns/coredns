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
	requestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: plugin.Namespace,
		Subsystem: "forward",
		Name:      "request_duration_seconds",
		Buckets:   plugin.TimeBuckets,
		Help:      "Histogram of the time each request took.",
	}, []string{"to", "rcode"})

	healthcheckBrokenCount = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: plugin.Namespace,
		Subsystem: "forward",
		Name:      "healthcheck_broken_total",
		Help:      "Counter of the number of complete failures of the healthchecks.",
	})

	maxConcurrentRejectCount = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: plugin.Namespace,
		Subsystem: "forward",
		Name:      "max_concurrent_rejects_total",
		Help:      "Counter of the number of queries rejected because the concurrent queries were at maximum.",
	})
)

func recordReqMetrics(addr string, start time.Time, res *dns.Msg) {
	if res != nil {
		// Record metrics
		rc, ok := dns.RcodeToString[res.Rcode]
		if !ok {
			rc = strconv.Itoa(res.Rcode)
		}
		requestDuration.WithLabelValues(addr, rc).Observe(time.Since(start).Seconds())
	}
}
