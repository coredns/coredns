package file

import (
	"github.com/coredns/coredns/plugin"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// zoneFileEntriesByDomain is the number of entries in zone file grouped by domain name.
	zoneFileEntriesByDomain = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: plugin.Namespace,
		Subsystem: "file",
		Name:      "entries",
		Help:      "The number of entries in zone file grouped by domain name.",
	}, []string{"dns_name"})
)
