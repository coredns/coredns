package reload

import (
	"github.com/coredns/coredns/plugin"

	"github.com/prometheus/client_golang/prometheus"
)

// Metrics for the reload plugin
var (
	FailedCount = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: plugin.Namespace,
		Subsystem: "reload",
		Name:      "failed_total",
		Help:      "Counter of the number of failed reload attempts.",
	})

	reloadInfo = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: plugin.Namespace,
		Subsystem: "reload",
		Name:      "version_info",
		Help:      "A metric with a constant '1' value labeled by hash, and value which type of hash generated.",
	}, []string{"hash", "value"})
)
