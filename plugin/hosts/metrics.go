package hosts

import (
	"github.com/coredns/coredns/plugin"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// hostsEntries is the combined number of entries in hosts and Corefile.
	hostsEntries = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: plugin.Namespace,
		Subsystem: "hosts",
		Name:      "entries",
		Help:      "The combined number of entries in hosts and Corefile.",
	}, []string{})
	// hostsReloadTime is the timestamp of the last reload of hosts file.
	hostsReloadTime = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: plugin.Namespace,
		Subsystem: "hosts",
		Name:      "reload_timestamp_seconds",
		Help:      "The timestamp of the last reload of hosts file.",
	})
	// hostsHits is counter of requests found in the hosts file
	hostsHits = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: plugin.Namespace,
		Subsystem: "hosts",
		Name:      "hits_total",
		Help:      "The number of requests served from the hosts file",
	})
)
