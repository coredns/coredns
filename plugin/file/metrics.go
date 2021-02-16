package file

import (
	"github.com/coredns/coredns/plugin"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// hostsEntries is the combined number of entries in hosts and Corefile.
	zoneFileEntriesByDomain = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: plugin.Namespace,
		Subsystem: "file",
		Name:      "entries",
		Help:      "The combined number of entries in zone file and Corefile.",
	}, []string{"dns_name"})
)
