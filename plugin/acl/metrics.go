package acl

import (
	"github.com/coredns/coredns/plugin"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// requestBlockCount is the number of DNS requests being blocked.
	requestBlockCount = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: plugin.Namespace,
		Subsystem: pluginName,
		Name:      "blocked_requests_total",
		Help:      "Counter of DNS requests being blocked.",
	}, []string{"server", "zone", "view"})
	// requestFilterCount is the number of DNS requests being filtered.
	requestFilterCount = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: plugin.Namespace,
		Subsystem: pluginName,
		Name:      "filtered_requests_total",
		Help:      "Counter of DNS requests being filtered.",
	}, []string{"server", "zone", "view"})
	// requestAllowCount is the number of DNS requests being Allowed.
	requestAllowCount = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: plugin.Namespace,
		Subsystem: pluginName,
		Name:      "allowed_requests_total",
		Help:      "Counter of DNS requests being allowed.",
	}, []string{"server", "view"})
	// requestDropCount is the number of DNS requests being dropped.
	requestDropCount = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: plugin.Namespace,
		Subsystem: pluginName,
		Name:      "dropped_requests_total",
		Help:      "Counter of DNS requests being dropped.",
	}, []string{"server", "zone", "view"})
)
