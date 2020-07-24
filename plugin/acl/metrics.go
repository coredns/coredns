package acl

import (
	"github.com/coredns/coredns/plugin"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// RequestBlockCount is the number of DNS requests being blocked.
	RequestBlockCount = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: plugin.Namespace,
		Subsystem: pluginName,
		Name:      "blocked_requests_total",
		Help:      "Counter of DNS requests being blocked.",
	}, []string{"server", "zone"})
	// RequestAllowCount is the number of DNS requests being Allowed.
	RequestAllowCount = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: plugin.Namespace,
		Subsystem: pluginName,
		Name:      "allowed_requests_total",
		Help:      "Counter of DNS requests being allowed.",
	}, []string{"server"})
)
