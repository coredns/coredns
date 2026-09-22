package nomad

import (
	"github.com/coredns/coredns/plugin"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// requestSuccessCount is the number of DNS requests handled successfully.
	requestSuccessCount = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: plugin.Namespace,
		Subsystem: pluginName,
		Name:      "success_requests_total",
		Help:      "Counter of DNS requests handled successfully.",
	}, []string{"server", "namespace"})
	// requestFailedCount is the number of DNS requests that failed.
	requestFailedCount = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: plugin.Namespace,
		Subsystem: pluginName,
		Name:      "failed_requests_total",
		Help:      "Counter of DNS requests failed.",
	}, []string{"server", "namespace"})
)
