package log

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

const (
	debugLabel   = "debug"
	infoLabel    = "info"
	warningLabel = "warning"
	errorLabel   = "error"
	fatalLabel   = "fatal"
)

// Variables declared for monitoring.
var (
	LogMessagesTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "coredns", // using plugin.Namespace would cause import loop here
		Name:      "log_messages_total",
		Help:      "Counter of logged messages.",
	}, []string{"level"})
)
