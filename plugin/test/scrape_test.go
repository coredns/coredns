package test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/prometheus/common/expfmt"
	"github.com/prometheus/common/model"
)

const sampleTextMetrics = `# HELP coredns_dns_requests_total Counter of DNS requests made per zone, protocol and family.
# TYPE coredns_dns_requests_total counter
coredns_dns_requests_total{family="1",proto="udp",server="dns://:53",type="A",zone="example.org."} 12
coredns_dns_requests_total{family="1",proto="udp",server="dns://:53",type="AAAA",zone="example.com."} 5
# HELP coredns_build_info A metric with a constant '1' value labeled by version, revision, and branch from which CoreDNS was built.
# TYPE coredns_build_info gauge
coredns_build_info{branch="master",goversion="go1.23.0",revision="abcdef",version="1.12.0"} 1
# HELP coredns_dns_request_duration_seconds Histogram of the time each request took.
# TYPE coredns_dns_request_duration_seconds histogram
coredns_dns_request_duration_seconds_bucket{server="dns://:53",zone="example.org.",le="0.00025"} 2
coredns_dns_request_duration_seconds_bucket{server="dns://:53",zone="example.org.",le="0.0005"} 4
coredns_dns_request_duration_seconds_bucket{server="dns://:53",zone="example.org.",le="+Inf"} 6
coredns_dns_request_duration_seconds_sum{server="dns://:53",zone="example.org."} 0.002
coredns_dns_request_duration_seconds_count{server="dns://:53",zone="example.org."} 6
`

func TestScrapeTextFormat(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", `text/plain; version=0.0.4; charset=utf-8`)
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, sampleTextMetrics)
	}))
	defer ts.Close()

	mfs := Scrape(ts.URL)
	if len(mfs) == 0 {
		t.Fatalf("Scrape(%s) returned 0 metric families, expected parsed metrics", ts.URL)
	}

	// Verify counter metric parsing and label lookup
	val, labels := MetricValue("coredns_dns_requests_total", mfs)
	if val != "12" {
		t.Errorf("MetricValue for coredns_dns_requests_total = %q, want %q", val, "12")
	}
	if labels["zone"] != "example.org." {
		t.Errorf("MetricValue labels[zone] = %q, want %q", labels["zone"], "example.org.")
	}

	// Verify MetricValueLabel lookup by specific label
	valLabel, _ := MetricValueLabel("coredns_dns_requests_total", "example.com.", mfs)
	if valLabel != "5" {
		t.Errorf("MetricValueLabel for example.com. = %q, want %q", valLabel, "5")
	}

	// Verify gauge metric parsing
	buildVal, _ := MetricValue("coredns_build_info", mfs)
	if buildVal != "1" {
		t.Errorf("MetricValue for coredns_build_info = %q, want %q", buildVal, "1")
	}

	// Verify ScrapeMetricAsInt sum
	addr := strings.TrimPrefix(ts.URL, "http://")
	tot := ScrapeMetricAsInt(addr, "coredns_dns_requests_total", "example.org.", 0)
	if tot != 12 {
		t.Errorf("ScrapeMetricAsInt total for example.org. = %d, want %d", tot, 12)
	}

	// Verify histogram parsing
	var foundHistogram bool
	for _, mf := range mfs {
		if mf.Name == "coredns_dns_request_duration_seconds" {
			foundHistogram = true
			if len(mf.Metrics) == 0 {
				t.Errorf("Histogram metric family %q had 0 metrics", mf.Name)
			}
			break
		}
	}
	if !foundHistogram {
		t.Errorf("Histogram coredns_dns_request_duration_seconds not found in scraped metrics")
	}
}

func TestScrapeTextParserDirect(t *testing.T) {
	parser := expfmt.NewTextParser(model.LegacyValidation)
	families, err := parser.TextToMetricFamilies(strings.NewReader(sampleTextMetrics))
	if err != nil {
		t.Fatalf("TextToMetricFamilies failed: %v", err)
	}

	if len(families) != 3 {
		t.Fatalf("expected 3 metric families, got %d", len(families))
	}
	if _, ok := families["coredns_dns_requests_total"]; !ok {
		t.Errorf("expected coredns_dns_requests_total family present")
	}
	if _, ok := families["coredns_build_info"]; !ok {
		t.Errorf("expected coredns_build_info family present")
	}
	if _, ok := families["coredns_dns_request_duration_seconds"]; !ok {
		t.Errorf("expected coredns_dns_request_duration_seconds family present")
	}
}

func TestScrapeErrors(t *testing.T) {
	// 404 response should yield empty slice without panic
	ts404 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts404.Close()

	if mfs := Scrape(ts404.URL); len(mfs) != 0 {
		t.Errorf("expected 0 metric families for 404 response, got %d", len(mfs))
	}

	// Invalid body content should yield empty slice without panic
	tsInvalid := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "this is totally invalid prometheus metric text :::")
	}))
	defer tsInvalid.Close()

	if mfs := Scrape(tsInvalid.URL); len(mfs) != 0 {
		t.Errorf("expected 0 metric families for invalid metric text, got %d", len(mfs))
	}
}
