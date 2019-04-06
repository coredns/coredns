package rewrite

import (
	"github.com/coredns/coredns/plugin"

	"github.com/prometheus/client_golang/prometheus"
)

// Metrics for the rewrite plugin
var (
	RequestRewriteNameCount = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: plugin.Namespace,
		Subsystem: "rewrite",
		Name:      "request_rewrite_name_total",
		Help:      "Counter of request name rewrites",
	}, []string{"server", "match_type", "src", "dest"})

	RequestRewriteTypeCount = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: plugin.Namespace,
		Subsystem: "rewrite",
		Name:      "request_rewrite_type_total",
		Help:      "Counter of request type rewrites",
	}, []string{"server", "src", "dest"})

	RequestRewriteClassCount = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: plugin.Namespace,
		Subsystem: "rewrite",
		Name:      "request_rewrite_class_total",
		Help:      "Counter of request type rewrites",
	}, []string{"server", "src", "dest"})

	RequestRewriteEDNS0Count = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: plugin.Namespace,
		Subsystem: "rewrite",
		Name:      "request_rewrite_edns_total",
		Help:      "Counter of request type rewrites",
	}, []string{"server", "type"})

	RequestRewriteTTLCount = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: plugin.Namespace,
		Subsystem: "rewrite",
		Name:      "rewrite_request_ttl_total",
		Help:      "Counter of request ttl rewrites",
	}, []string{"server", "match_type"})

	ResponseRewriteNameCount = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: plugin.Namespace,
		Subsystem: "rewrite",
		Name:      "rewrite_response_name_total",
		Help:      "Counter of response ttl rewrites",
	}, []string{"src", "dest"})

	ResponseRewriteTTLCount = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: plugin.Namespace,
		Subsystem: "rewrite",
		Name:      "rewrite_response_ttl_total",
		Help:      "Counter of request ttl rewrites",
	}, []string{"ttl"})
)
