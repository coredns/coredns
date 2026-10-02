package dso

import (
	"github.com/coredns/coredns/plugin"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// sessionDuration is duration of client sessions.
	sessionDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Namespace:                   plugin.Namespace,
		Subsystem:                   Name,
		Name:                        "session_duration_seconds",
		Buckets:                     []float64{0, 10, 60, 600, 1800, 3600, 86400},
		NativeHistogramBucketFactor: plugin.NativeHistogramBucketFactor,
		Help:                        "Histogram session durations in seconds.",
	}, []string{"server"})

	// pushSubscriptionEntries is number of active DSO Push subscriptions.
	pushSubscriptionEntries = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: plugin.Namespace,
		Subsystem: Name,
		Name:      "push_subscription_entries",
		Help:      "The number of active push subscriptions.",
	}, []string{"server", "rrtype"})
	// pushSubscriptionRequests is counter of all DSO Push subscriptions.
	pushSubscriptionRequests = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: plugin.Namespace,
		Subsystem: Name,
		Name:      "push_subscription_requests_total",
		Help:      "The number of push subscription attempts.",
	}, []string{"server", "rrtype"})
	// pushSubscriptionHits is counter of successful DSO Push subscriptions.
	pushSubscriptionHits = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: plugin.Namespace,
		Subsystem: Name,
		Name:      "push_subscription_hits_total",
		Help:      "The number of successful push subscription attempts.",
	}, []string{"server", "rrtype"})
)
