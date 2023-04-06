package health

import (
	"context"
	"net"
	"net/http"
	"time"

	"github.com/coredns/coredns/plugin"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// overloaded queries the health end point and updates a metrics showing how long it took.
func (h *health) overloaded(ctx context.Context) {
	bypassProxy := &http.Transport{
		Proxy: nil,
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
	timeout := 3 * time.Second
	client := http.Client{
		Timeout:   timeout,
		Transport: bypassProxy,
	}

	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, h.healthURI.String(), nil)
	tick := time.NewTicker(1 * time.Second)
	defer tick.Stop()

	for {
		select {
		case <-tick.C:
			start := time.Now()
			resp, err := client.Do(req)
			if err != nil && ctx.Err() == context.Canceled {
				// request was cancelled by parent goroutine
				return
			}
			if err != nil {
				HealthDuration.Observe(time.Since(start).Seconds())
				HealthFailures.Inc()
				log.Warningf("Local health request to %q failed: %s", req.URL.String(), err)
				continue
			}
			resp.Body.Close()
			elapsed := time.Since(start)
			HealthDuration.Observe(elapsed.Seconds())
			if elapsed > time.Second { // 1s is pretty random, but a *local* scrape taking that long isn't good
				log.Warningf("Local health request to %q took more than 1s: %s", req.URL.String(), elapsed)
			}

		case <-ctx.Done():
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
		Buckets:   plugin.SlimTimeBuckets,
		Help:      "Histogram of the time (in seconds) each request took.",
	})
	// HealthFailures is the metric used to count how many times the health request failed
	HealthFailures = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: plugin.Namespace,
		Subsystem: "health",
		Name:      "request_failures_total",
		Help:      "The number of times the health check failed.",
	})
)
