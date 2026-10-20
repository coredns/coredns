package metrics

import (
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

func TestOpenMetricsNegotiation(t *testing.T) {
	m := New("127.0.0.1:0")
	m.Reg = prometheus.NewRegistry()
	h := promauto.With(m.Reg).NewHistogram(prometheus.HistogramOpts{
		Name: "test_duration_seconds",
		Help: "Test duration.",
	})
	h.(prometheus.ExemplarObserver).ObserveWithExemplar(0.001, prometheus.Labels{
		"query_name": "service.cluster.local.",
		"query_type": "A",
	})
	if err := m.OnStartup(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = m.OnFinalShutdown() })
	client := &http.Client{Timeout: 2 * time.Second}
	for _, tc := range []struct {
		name, accept, contentType string
		wantExemplar              bool
	}{
		{"default", "", "text/plain", false},
		{"legacy", "text/plain; version=0.0.4", "text/plain", false},
		{"openmetrics", "application/openmetrics-text; version=1.0.0", "application/openmetrics-text", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://"+m.ln.Addr().String()+"/metrics", nil)
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Accept", tc.accept)
			resp, err := client.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatal(err)
			}
			if resp.StatusCode != http.StatusOK || !strings.HasPrefix(resp.Header.Get("Content-Type"), tc.contentType) {
				t.Fatalf("status = %d, Content-Type = %q", resp.StatusCode, resp.Header.Get("Content-Type"))
			}
			if got := strings.Contains(string(body), `query_name="service.cluster.local."`); got != tc.wantExemplar {
				t.Fatalf("exemplar present = %v, want %v: %s", got, tc.wantExemplar, body)
			}
			if !strings.Contains(string(body), "test_duration_seconds_count 1") {
				t.Fatalf("missing ordinary observation: %s", body)
			}
		})
	}
}
