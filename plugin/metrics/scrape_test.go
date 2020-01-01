package metrics

import (
	"testing"
)

func TestScrapeMetrics(t *testing.T) {
	met := New("localhost:0")
	if err := met.OnStartup(); err != nil {
		t.Fatalf("Failed to start metrics handler: %s", err)
	}
	defer met.OnFinalShutdown()

	met.AddZone("example.org.")
	ScrapeMetrics("http://" + ListenAddr + "/metrics")
}
