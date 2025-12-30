package test

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/coredns/coredns/plugin/metrics"
	"github.com/coredns/coredns/plugin/metrics/vars"
	"github.com/coredns/coredns/plugin/test"

	"github.com/miekg/dns"
)

// Because we don't properly shutdown the metrics servers we are re-using the metrics between tests, not a superbad issue
// but depending on the ordering of the tests this trips up stuff.

// Start test server that has metrics enabled. Then tear it down again.
func TestMetricsServer(t *testing.T) {
	corefile := `
	example.org:0 {
		chaos CoreDNS-001 miek@miek.nl
		prometheus localhost:0
	}
	example.com:0 {
		log
		prometheus localhost:0
	}`

	srv, err := CoreDNSServer(corefile)
	if err != nil {
		t.Fatalf("Could not get CoreDNS serving instance: %s", err)
	}
	defer srv.Stop()
}

func TestMetricsRefused(t *testing.T) {
	metricName := "coredns_dns_responses_total"
	corefile := `example.org:0 {
		whoami
		prometheus localhost:0
	}`

	srv, udp, _, err := CoreDNSServerAndPorts(corefile)
	if err != nil {
		t.Fatalf("Could not get CoreDNS serving instance: %s", err)
	}
	defer srv.Stop()

	m := new(dns.Msg)
	m.SetQuestion("google.com.", dns.TypeA)

	if _, err = dns.Exchange(m, udp); err != nil {
		t.Fatalf("Could not send message: %s", err)
	}

	data := test.Scrape("http://" + metrics.ListenAddr + "/metrics")
	got, labels := test.MetricValue(metricName, data)

	if got != "1" {
		t.Errorf("Expected value %s for refused, but got %s", "1", got)
	}
	if labels["zone"] != vars.Dropped {
		t.Errorf("Expected zone value %s for refused, but got %s", vars.Dropped, labels["zone"])
	}
	if labels["rcode"] != "REFUSED" {
		t.Errorf("Expected zone value %s for refused, but got %s", "REFUSED", labels["rcode"])
	}
}

// getBucketCount extracts the count for a specific bucket from a metric family
func getBucketCount(mf *test.MetricFamily, bucketLabel string) (int, error) {
	if mf == nil {
		return 0, fmt.Errorf("metric family is nil")
	}
	if len(mf.Metrics) == 0 {
		return 0, fmt.Errorf("metric family %s has no metrics", mf.Name)
	}

	// mf.Metrics[0] is an interface{} containing an unexported 'histogram' struct from plugin/test.
	metricPoint := mf.Metrics[0]
	val := reflect.ValueOf(metricPoint)

	// Check if the underlying type is a struct (as histogram is)
	if val.Kind() != reflect.Struct {
		return 0, fmt.Errorf("metric point for %s is not a struct, but %s", mf.Name, val.Kind())
	}

	// Access the 'Buckets' field, which should be map[string]string
	bucketsField := val.FieldByName("Buckets")
	if !bucketsField.IsValid() {
		return 0, fmt.Errorf("metric point for %s has no 'Buckets' field", mf.Name)
	}

	bucketsMap, ok := bucketsField.Interface().(map[string]string)
	if !ok {
		return 0, fmt.Errorf("'Buckets' field for %s is not a map[string]string", mf.Name)
	}

	countStr, ok := bucketsMap[bucketLabel]
	if !ok {
		// For these tests, we'll treat a missing bucket as 0.
		return 0, nil
	}

	count, err := strconv.Atoi(countStr)
	if err != nil {
		return 0, fmt.Errorf("could not parse bucket count '%s' for %s: %v", countStr, mf.Name, err)
	}
	return count, nil
}

// extractRequestSizeBucketCounts extracts bucket counts from DNS request size metrics
func extractRequestSizeBucketCounts(t *testing.T, metrics []*test.MetricFamily, label string) (int, int, error) {
	t.Helper()
	var countBelow100, countAbove100 int
	var err error

	for _, mf := range metrics {
		if strings.Contains(mf.Name, "coredns_dns_request_size_bytes") {
			t.Logf("  %s: %v", mf.Name, mf.Metrics)
			countBelow100, err = getBucketCount(mf, "100")
			if err != nil {
				return 0, 0, fmt.Errorf("%s: error getting bucket count for 100: %v", label, err)
			}
			countAbove100, err = getBucketCount(mf, "1023")
			if err != nil {
				return 0, 0, fmt.Errorf("%s: error getting bucket count for 1023: %v", label, err)
			}
			return countBelow100, countAbove100, nil
		}
	}

	return 0, 0, fmt.Errorf("%s: could not find coredns_dns_request_size_bytes metric", label)
}

func TestMetricsRewriteRequestSize(t *testing.T) {
	// number of requests to send
	numRequests := 5

	// First test without rewrite
	corefileWithoutRewrite := `.:0 {
		prometheus localhost:0
		forward . 8.8.8.8
	}`

	srv, udp, _, err := CoreDNSServerAndPorts(corefileWithoutRewrite)
	if err != nil {
		t.Fatalf("Could not get CoreDNS serving instance: %s", err)
	}

	// Create a DNS request with a long name to have a size close to 100 bytes
	m := new(dns.Msg)
	m.SetQuestion("somerequestthathaveasize90.123456789.123456789.123456789.example.com.", dns.TypeA)
	expectedSize := 86
	actualSize := m.Len()
	if actualSize != expectedSize {
		t.Fatalf("Expected request size %d, but got %d", expectedSize, actualSize)
	}

	// Send multiple requests
	for range numRequests {
		if _, err = dns.Exchange(m, udp); err != nil {
			t.Fatalf("Could not send message: %s", err)
		}
	}

	metricsWithoutRewrite := test.Scrape("http://" + metrics.ListenAddr + "/metrics")

	t.Log("Available metrics without rewrite:")
	countBelow100withoutRewrite, countAbove100withoutRewrite, err := extractRequestSizeBucketCounts(t, metricsWithoutRewrite, "without rewrite")
	if err != nil {
		t.Error(err)
	}

	// Stop the first server
	srv.Stop()
	time.Sleep(100 * time.Millisecond) // Give server time to clean up

	// Now test with rewrite plugin
	corefileWithRewrite := `.:0 {
		prometheus localhost:0
		rewrite edns0 local set 0x13 test123456 revert
		forward . 8.8.8.8
	}`

	srv2, udp2, _, err := CoreDNSServerAndPorts(corefileWithRewrite)
	if err != nil {
		t.Fatalf("Could not get CoreDNS serving instance: %s", err)
	}
	defer srv2.Stop()

	// Send the same requests with rewrite
	for range numRequests {
		if _, err = dns.Exchange(m, udp2); err != nil {
			t.Fatalf("Could not send message: %s", err)
		}
	}

	// Scrape metrics again
	metricsWithRewrite := test.Scrape("http://" + metrics.ListenAddr + "/metrics")

	t.Log("Available metrics with rewrite:")
	countBelow100withRewrite, countAbove100withRewrite, err := extractRequestSizeBucketCounts(t, metricsWithRewrite, "with rewrite")
	if err != nil {
		t.Error(err)
	}

	// Both servers should record metrics in the same buckets regardless of the
	// rewrite plugin's modifications. The original request size is 86 bytes,
	// which falls into the le=100 bucket, before and after the rewrite.

	if countBelow100withoutRewrite != countAbove100withoutRewrite &&
		countBelow100withRewrite != countAbove100withRewrite {
		t.Errorf("Expected all requests to go to le=100 bucket")
	}

	// The count in the le=100 bucket should be the same with or without rewrite.
	// Second round of requests should go to le=100 bucket.
	if countBelow100withRewrite != countBelow100withoutRewrite+numRequests {
		t.Errorf("Expected all requests to go to le=100 bucket")
	}
}

func TestMetricsAuto(t *testing.T) {
	tmpdir := t.TempDir()

	corefile := `org:0 {
		auto {
			directory ` + tmpdir + ` db\.(.*) {1}
			reload 0.1s
		}
		prometheus localhost:0
	}`

	i, err := CoreDNSServer(corefile)
	if err != nil {
		t.Fatalf("Could not get CoreDNS serving instance: %s", err)
	}

	udp, _ := CoreDNSServerPorts(i, 0)
	if udp == "" {
		t.Fatalf("Could not get UDP listening port")
	}
	defer i.Stop()

	// Write db.example.org to get example.org.
	if err = os.WriteFile(filepath.Join(tmpdir, "db.example.org"), []byte(zoneContent), 0644); err != nil {
		t.Fatal(err)
	}
	time.Sleep(110 * time.Millisecond) // wait for it to be picked up

	m := new(dns.Msg)
	m.SetQuestion("www.example.org.", dns.TypeA)

	if _, err := dns.Exchange(m, udp); err != nil {
		t.Fatalf("Could not send message: %s", err)
	}

	metricName := "coredns_dns_requests_total" // {zone, proto, family, type}

	data := test.Scrape("http://" + metrics.ListenAddr + "/metrics")
	// Get the value for the metrics where the one of the labels values matches "example.org."
	got, _ := test.MetricValueLabel(metricName, "example.org.", data)

	if got == "0" {
		t.Errorf("Expected value %s for %s, but got %s", "> 1", metricName, got)
	}

	// Remove db.example.org again. And see if the metric stops increasing.
	os.Remove(filepath.Join(tmpdir, "db.example.org"))
	time.Sleep(110 * time.Millisecond) // wait for it to be picked up
	if _, err := dns.Exchange(m, udp); err != nil {
		t.Fatalf("Could not send message: %s", err)
	}

	data = test.Scrape("http://" + metrics.ListenAddr + "/metrics")
	got, _ = test.MetricValueLabel(metricName, "example.org.", data)

	if got == "0" {
		t.Errorf("Expected value %s for %s, but got %s", "> 1", metricName, got)
	}
}

// Show that when 2 blocs share the same metric listener (they have a prometheus plugin on the same listening address),
// ALL the metrics of the second bloc in order are declared in prometheus, especially the plugins that are used ONLY in the second bloc
func TestMetricsSeveralBlocs(t *testing.T) {
	cacheSizeMetricName := "coredns_cache_entries"
	addrMetrics := "localhost:9155"
	corefile := `
	example.org:0 {
		prometheus ` + addrMetrics + `
		forward . 8.8.8.8:53 {
			force_tcp
		}
	}
	google.com:0 {
		prometheus ` + addrMetrics + `
		whoami
		cache
	}`

	i, udp, _, err := CoreDNSServerAndPorts(corefile)
	if err != nil {
		t.Fatalf("Could not get CoreDNS serving instance: %s", err)
	}
	defer i.Stop()

	// send an initial query to setup properly the cache size
	m := new(dns.Msg)
	m.SetQuestion("google.com.", dns.TypeA)
	if _, err = dns.Exchange(m, udp); err != nil {
		t.Fatalf("Could not send message: %s", err)
	}

	beginCacheSize := test.ScrapeMetricAsInt(addrMetrics, cacheSizeMetricName, "", 0)

	// send an query, different from initial to ensure we have another add to the cache
	m = new(dns.Msg)
	m.SetQuestion("www.google.com.", dns.TypeA)

	if _, err = dns.Exchange(m, udp); err != nil {
		t.Fatalf("Could not send message: %s", err)
	}

	endCacheSize := test.ScrapeMetricAsInt(addrMetrics, cacheSizeMetricName, "", 0)

	if endCacheSize-beginCacheSize != 1 {
		t.Errorf("Expected metric data retrieved for %s, expected %d, got %d", cacheSizeMetricName, 1, endCacheSize-beginCacheSize)
	}
}

func TestMetricsPluginEnabled(t *testing.T) {
	corefile := `
	example.org:0 {
		chaos CoreDNS-001 miek@miek.nl
		prometheus localhost:0
	}
	example.com:0 {
		whoami
		prometheus localhost:0
	}`

	srv, err := CoreDNSServer(corefile)
	if err != nil {
		t.Fatalf("Could not get CoreDNS serving instance: %s", err)
	}
	defer srv.Stop()

	metricName := "coredns_plugin_enabled" //{server, zone, name}

	data := test.Scrape("http://" + metrics.ListenAddr + "/metrics")

	// Get the value for the metrics where the one of the labels values matches "chaos".
	got, _ := test.MetricValueLabel(metricName, "chaos", data)

	if got != "1" {
		t.Errorf("Expected value %s for %s, but got %s", "1", metricName, got)
	}

	// Get the value for the metrics where the one of the labels values matches "erratic".
	got, _ = test.MetricValueLabel(metricName, "erratic", data) // none of these tests use 'erratic'

	if got != "" {
		t.Errorf("Expected value %s for %s, but got %s", "", metricName, got)
	}
}

func TestMetricsAvailable(t *testing.T) {
	procMetric := "coredns_build_info"
	procCache := "coredns_cache_entries"
	procCacheMiss := "coredns_cache_misses_total"
	procForward := "coredns_dns_request_duration_seconds"
	corefileWithMetrics := `.:0 {
		prometheus localhost:0
		cache
		forward . 8.8.8.8 {
			force_tcp
		}
	}`

	inst, _, tcp, err := CoreDNSServerAndPorts(corefileWithMetrics)
	defer inst.Stop()
	if err != nil {
		if strings.Contains(err.Error(), inUse) {
			return
		}
		t.Errorf("Could not get service instance: %s", err)
	}
	// send a query and check we can scrap corresponding metrics
	cl := dns.Client{Net: "tcp"}
	m := new(dns.Msg)
	m.SetQuestion("www.example.org.", dns.TypeA)

	if _, _, err := cl.Exchange(m, tcp); err != nil {
		t.Fatalf("Could not send message: %s", err)
	}

	// we should have metrics from forward, cache, and metrics itself
	if err := collectMetricsInfo(metrics.ListenAddr, procMetric, procCache, procCacheMiss, procForward); err != nil {
		t.Errorf("Could not scrap one of expected stats : %s", err)
	}
}
