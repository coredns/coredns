package health

import (
	"net/http"
	"time"

	"github.com/coredns/coredns/plugin"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// overloaded queries the health end point and updates a metrics showing how long it took.
func (h *health) overloaded() {
	timeout := time.Duration(3 * time.Second)
	client := http.Client{
		Timeout: timeout,
	}
	url := "http://" + h.Addr + "/health"
	tick := time.NewTicker(1 * time.Second)
	defer tick.Stop()

	for {
		select {
		case <-tick.C:
			start := time.Now()
			resp, err := client.Get(url)
			if err != nil {
				HealthDuration.Observe(timeout.Seconds())
				log.Warningf("Local health request to %q failed: %s", url, err)
				continue
			}
			resp.Body.Close()
			elapsed := time.Since(start)
			HealthDuration.Observe(elapsed.Seconds())
			if elapsed > time.Second { // 1s is pretty random, but a *local* scrape taking that long isn't good
				log.Warningf("Local health request to %q took more than 1s: %s", url, elapsed)
			}

		case <-h.stop:
			return
		}
	}
}

var (
	// HealthDuration is the metric used for exporting how fast we can retrieve the /health endpoint.
	HealthDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Namespace: plugin.Namespace,
		Subsystem: "health",
		Name:      "request_duration_seconds",
		Buckets:   plugin.TimeBuckets,
		Help:      "Histogram of the time (in seconds) each request took.",
	})
)
