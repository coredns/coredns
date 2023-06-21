package forward

import (
	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/pkg/proxy"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Variables declared for monitoring.
var (
	ProxyMetrics = proxy.NewMetricsWithSubsystem("forward")

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
