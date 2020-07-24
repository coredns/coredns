package dns64

import (
	"github.com/coredns/coredns/plugin"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// RequestsTranslatedCount is the number of DNS requests translated by dns64.
	RequestsTranslatedCount = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: plugin.Namespace,
		Subsystem: pluginName,
		Name:      "requests_translated_total",
		Help:      "Counter of DNS requests translated by dns64.",
	}, []string{"server"})
)
