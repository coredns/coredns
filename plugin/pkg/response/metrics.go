package response

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Variables declared for monitoring.
var (
	negativeMissingSoa = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "coredns",
		Subsystem: "response",
		Name:      "negative_missing_soa",
		Help:      "Counter of the number of negative responses that are missing soa",
	}, []string{})
)
