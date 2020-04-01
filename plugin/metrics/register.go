package metrics

import (
	"github.com/coredns/coredns/core/dnsserver"

	"github.com/caddyserver/caddy"
	"github.com/prometheus/client_golang/prometheus"
)

// MustRegister registers the prometheus Collectors when the metrics plugin is used.
func MustRegister(c *caddy.Controller, cs ...prometheus.Collector) {
	m := dnsserver.GetConfig(c).Handler(pluginName)
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
