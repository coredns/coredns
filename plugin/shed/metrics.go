package shed

import (
	"github.com/coredns/coredns/plugin"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var droppedTotal = promauto.NewCounterVec(prometheus.CounterOpts{
	Namespace: plugin.Namespace,
	Subsystem: "shed",
	Name:      "dropped_total",
	Help:      "Counter of queries and responses dropped, per server, by reason.",
}, []string{"server", "reason"})
