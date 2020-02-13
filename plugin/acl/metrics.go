package acl

import (
	"github.com/coredns/coredns/plugin"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	// RequestBlockCount is the number of DNS requests being blocked.
	RequestBlockCount = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: plugin.Namespace,
		Subsystem: "dns",
		Name:      "acl_blocked_requests_total",
		Help:      "Counter of DNS requests being blocked.",
	}, []string{"server", "zone"})
	// RequestAllowCount is the number of DNS requests being Allowed.
	RequestAllowCount = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: plugin.Namespace,
		Subsystem: "dns",
		Name:      "acl_allowed_requests_total",
		Help:      "Counter of DNS requests being allowed.",
	}, []string{"server"})
)
