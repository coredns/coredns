package metrics

import (
	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"

	"github.com/prometheus/client_golang/prometheus"
)

// MustRegister registers the prometheus Collectors when the metrics plugin is used.
func MustRegister(c *caddy.Controller, cs ...prometheus.Collector) {
	m := dnsserver.GetConfig(c).Handler("prometheus")
	if m == nil {
		return
	}
	x, ok := m.(*Metrics)
	if !ok {
		return
	}
	for _, c := range cs {
		x.MustRegister(c)
	}
}
