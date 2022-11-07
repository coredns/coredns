package local

import (
	"github.com/coredns/coredns/plugin"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// LocalhostCount report the number of times we've seen a localhost.<domain> query.
var LocalhostCount = promauto.NewCounter(prometheus.CounterOpts{
	Namespace: plugin.Namespace,
	Subsystem: "local",
	Name:      "localhost_requests_total",
	Help:      "Counter of localhost.<domain> requests.",
})
