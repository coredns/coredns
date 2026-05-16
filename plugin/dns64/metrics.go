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

	// RequestsFilteredCount is the number of client A queries suppressed by
	// the filter_a option.
	RequestsFilteredCount = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: plugin.Namespace,
		Subsystem: pluginName,
		Name:      "requests_filtered_total",
		Help:      "Counter of client A queries suppressed by the dns64 filter_a option.",
	}, []string{"server"})
)
