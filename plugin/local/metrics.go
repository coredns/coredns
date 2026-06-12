package local

import (
	"github.com/coredns/coredns/plugin"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// LocalhostCount reports the number of times we've seen a non-apex localhost special-case query.
	LocalhostCount = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: plugin.Namespace,
		Subsystem: "local",
		Name:      "localhost_requests_total",
		Help:      "Counter of special localhost subdomain requests.",
	})
)
