package template

import (
	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/metrics"

	"github.com/caddyserver/caddy"
	"github.com/prometheus/client_golang/prometheus"
)

var (
	// templateMatchesCount is the counter of template regex matches.
	templateMatchesCount = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: plugin.Namespace,
		Subsystem: pluginName,
		Name:      "matches_total",
		Help:      "Counter of template regex matches.",
	}, []string{"server", "zone", "class", "type"})
	// templateFailureCount is the counter of go template failures.
	templateFailureCount = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: plugin.Namespace,
		Subsystem: pluginName,
		Name:      "template_failures_total",
		Help:      "Counter of go template failures.",
	}, []string{"server", "zone", "class", "type", "section", "template"})
	// templateRRFailureCount is the counter of mis-templated RRs.
	templateRRFailureCount = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: plugin.Namespace,
		Subsystem: pluginName,
		Name:      "rr_failures_total",
		Help:      "Counter of mis-templated RRs.",
	}, []string{"server", "zone", "class", "type", "section", "template"})
)

// OnStartupMetrics sets up the metrics on startup.
func setupMetrics(c *caddy.Controller) error {
	c.OnStartup(func() error {
		metrics.MustRegister(c, templateMatchesCount, templateFailureCount, templateRRFailureCount)
		return nil
	})

	return nil
}
