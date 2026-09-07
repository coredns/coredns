package dnslkg

import (
	"github.com/coredns/coredns/plugin"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// storedResponses counts the answers recorded as last known good.
	storedResponses = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: plugin.Namespace,
		Subsystem: "dnslkg",
		Name:      "stored_total",
		Help:      "The count of upstream answers stored as last known good.",
	}, []string{"server"})
	// servedResponses counts the responses served from the last known good store.
	servedResponses = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: plugin.Namespace,
		Subsystem: "dnslkg",
		Name:      "served_total",
		Help:      "The count of responses served from the last known good store.",
	}, []string{"server"})
)
