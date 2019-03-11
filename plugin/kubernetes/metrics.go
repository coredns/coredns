package kubernetes

import (
	"github.com/coredns/coredns/plugin"
	"github.com/prometheus/client_golang/prometheus"
)

const (
	subsystem = "kubernetes"
)

var (
	// DnsProgrammingLatency is defined as the time it took to program a DNS instance - from the time
	// a service or pod has changed to the time the change was propagated and was available to be
	// served by a DNS server.
	// The definition of this SLI can be found at https://github.com/kubernetes/community/blob/master/sig-scalability/slos/dns_programming_latency.md
	// Note that the metrics is partially based on the time exported by the endpoints controller on
	// the master machine. The measurement may be inaccurate if there is a clock drift between the
	// node and master machine.
	// The service_kind label can be one of:
	//   * cluster_ip
	//   * headless_with_selector
	//   * headless_without_selector
	DnsProgrammingLatency = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: plugin.Namespace,
		Subsystem: subsystem,
		Name:      "dns_programming_latency_seconds",
		// From 1 millisecond to ~17 minutes.
		Buckets: prometheus.ExponentialBuckets(0.001, 2, 20),
		Help:      "Histogram of the time (in seconds) it took to program a dns instance.",
	}, []string{"service_kind"})
)

// Register all metrics.
func init() {
	prometheus.MustRegister(DnsProgrammingLatency )
}
