package forward

import (
	"sync"

	"github.com/coredns/coredns/plugin"

	"github.com/prometheus/client_golang/prometheus"
)

// Variables declared for monitoring.
var (
	RequestCount = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: plugin.Namespace,
		Subsystem: "forward",
		Name:      "request_count_total",
		Help:      "Counter of requests made per protocol, family and upstream.",
	}, []string{"proto", "family", "to"})
	SocketCount = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: plugin.Namespace,
		Subsystem: "forward",
		Name:      "socket_count_total",
		Help:      "Counter of open socket per upstream.",
	}, []string{"to"})
	RequestDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: plugin.Namespace,
		Subsystem: "forward",
		Name:      "request_duration_milliseconds",
		Buckets:   append(prometheus.DefBuckets, []float64{15, 20, 25, 30, 40, 50, 100, 200, 500, 1000, 2000, 3000, 4000, 5000, 10000}...),
		Help:      "Histogram of the time (in milliseconds) each request took.",
	}, []string{"proto", "family", "to"})
)

// OnStartupMetrics sets up the metrics on startup.
func OnStartupMetrics() error {
	metricsOnce.Do(func() {
		prometheus.MustRegister(RequestCount)
		prometheus.MustRegister(SocketCount)
		prometheus.MustRegister(RequestDuration)
	})
	return nil
}

// familyToString returns the string form of either 1, or 2. Returns
// empty string is not a known family
func familyToString(f int) string {
	if f == 1 {
		return "1"
	}
	if f == 2 {
		return "2"
	}
	return ""
}

var metricsOnce sync.Once
