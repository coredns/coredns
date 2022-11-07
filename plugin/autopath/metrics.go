package autopath

import (
	"github.com/coredns/coredns/plugin"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// autoPathCount is counter of successfully autopath-ed queries.
var autoPathCount = promauto.NewCounterVec(prometheus.CounterOpts{
	Namespace: plugin.Namespace,
	Subsystem: "autopath",
	Name:      "success_total",
	Help:      "Counter of requests that did autopath.",
}, []string{"server"})
