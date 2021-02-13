package object

import (
	"time"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/pkg/log"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	api "k8s.io/api/core/v1"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	// DNSProgrammingLatency is defined as the time it took to program a DNS instance - from the time
	// a service or pod has changed to the time the change was propagated and was available to be
	// served by a DNS server.
	// The definition of this SLI can be found at https://github.com/kubernetes/community/blob/master/sig-scalability/slos/dns_programming_latency.md
	// Note that the metrics is partially based on the time exported by the endpoints controller on
	// the master machine. The measurement may be inaccurate if there is a clock drift between the
	// node and master machine.
	// The service_kind label can be one of:
	//   * cluster_ip
	//   * headless_with_selector
	//   * headless_without_selector
	DNSProgrammingLatency = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: plugin.Namespace,
		Subsystem: "kubernetes",
		Name:      "dns_programming_duration_seconds",
		// From 1 millisecond to ~17 minutes.
		Buckets: prometheus.ExponentialBuckets(0.001, 2, 20),
		Help:    "Histogram of the time (in seconds) it took to program a dns instance.",
	}, []string{"service_kind"})

	// DurationSinceFunc returns the duration elapsed since the given time.
	// Added as a global variable to allow injection for testing.
	DurationSinceFunc = time.Since
)

// EndpointLatencyRecorder records latency metric for endpoint objects
type EndpointLatencyRecorder struct {
	TT          time.Time
	ServiceFunc func(meta.Object) []*Service
	Services    []*Service
}

func (l *EndpointLatencyRecorder) init(o meta.Object) {
	l.Services = l.ServiceFunc(o)
	l.TT = time.Time{}
	stringVal, ok := o.GetAnnotations()[api.EndpointsLastChangeTriggerTime]
	if ok {
		tt, err := time.Parse(time.RFC3339Nano, stringVal)
		if err != nil {
			log.Warningf("DnsProgrammingLatency cannot be calculated for Endpoints '%s/%s'; invalid %q annotation RFC3339 value of %q",
				o.GetNamespace(), o.GetName(), api.EndpointsLastChangeTriggerTime, stringVal)
			// In case of error val = time.Zero, which is ignored downstream.
		}
		l.TT = tt
	}
}

func (l *EndpointLatencyRecorder) record() {
	// isHeadless indicates whether the endpoints object belongs to a headless
	// service (i.e. clusterIp = None). Note that this can be a  false negatives if the service
	// informer is lagging, i.e. we may not see a recently created service. Given that the services
	// don't change very often (comparing to much more frequent endpoints changes), cases when this method
	// will return wrong answer should be relatively rare. Because of that we intentionally accept this
	// flaw to keep the solution simple.
	isHeadless := len(l.Services) == 1 && l.Services[0].Headless()

	if !isHeadless || l.TT.IsZero() {
		return
	}

	// If we're here it means that the Endpoints object is for a headless service and that
	// the Endpoints object was created by the endpoints-controller (because the
	// LastChangeTriggerTime annotation is set). It means that the corresponding service is a
	// "headless service with selector".
	DNSProgrammingLatency.WithLabelValues("headless_with_selector").
		Observe(DurationSinceFunc(l.TT).Seconds())
}
