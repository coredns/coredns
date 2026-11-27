package siit

import (
	"github.com/coredns/coredns/plugin"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// requestsTranslatedCount is the number of DNS requests translated by siit.
	requestsTranslatedCount = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: plugin.Namespace,
		Subsystem: pluginName,
		Name:      "requests_translated_total",
		Help:      "Counter of DNS requests translated by siit.",
	}, []string{"server"})

	// excludedAAAACount is the number of AAAA records removed from responses
	// because their address is part of the SIIT mapping (exclude_aaaa).
	excludedAAAACount = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: plugin.Namespace,
		Subsystem: pluginName,
		Name:      "aaaa_records_excluded_total",
		Help:      "Counter of AAAA records excluded from responses because they are part of the siit mapping.",
	}, []string{"server"})
)
