package test

import (
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"path"
	"strings"
	"testing"
	"time"

	"github.com/coredns/coredns/plugin/pkg/uniq"
	"github.com/mholt/caddy"

	"github.com/coredns/coredns/plugin/cache"
	"github.com/coredns/coredns/plugin/metrics"
	mtest "github.com/coredns/coredns/plugin/metrics/test"
	"github.com/coredns/coredns/plugin/metrics/vars"

	"github.com/miekg/dns"
)

// fail when done in parallel

// Start test server that has metrics enabled. Then tear it down again.
func TestMetricsServer(t *testing.T) {
	corefile := `example.org:0 {
	chaos CoreDNS-001 miek@miek.nl
	prometheus localhost:0
}

example.com:0 {
	proxy . 8.8.4.4:53
	prometheus localhost:0
}
`
	srv, err := CoreDNSServer(corefile)
	if err != nil {
		t.Fatalf("Could not get CoreDNS serving instance: %s", err)
	}
	defer srv.Stop()
}

func TestMetricsRefused(t *testing.T) {
	metricName := "coredns_dns_response_rcode_count_total"

	corefile := `example.org:0 {
	proxy . 8.8.8.8:53
	prometheus localhost:0
}
`
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

	data := mtest.Scrape(t, "http://"+metrics.ListenAddr+"/metrics")
	got, labels := mtest.MetricValue(metricName, data)

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

// TODO(miek): disabled for now - fails in weird ways in travis.
func testMetricsCache(t *testing.T) {
	cacheSizeMetricName := "coredns_cache_size"
	cacheHitMetricName := "coredns_cache_hits_total"

	corefile := `www.example.net:0 {
	proxy . 8.8.8.8:53
	prometheus localhost:0
	cache
}
`
	srv, err := CoreDNSServer(corefile)
	if err != nil {
		t.Fatalf("Could not get CoreDNS serving instance: %s", err)
	}
	defer srv.Stop()

	udp, _ := CoreDNSServerPorts(srv, 0)

	m := new(dns.Msg)
	m.SetQuestion("www.example.net.", dns.TypeA)

	if _, err = dns.Exchange(m, udp); err != nil {
		t.Fatalf("Could not send message: %s", err)
	}

	data := mtest.Scrape(t, "http://"+metrics.ListenAddr+"/metrics")
	// Get the value for the cache size metric where the one of the labels values matches "success".
	got, _ := mtest.MetricValueLabel(cacheSizeMetricName, cache.Success, data)

	if got != "1" {
		t.Errorf("Expected value %s for %s, but got %s", "1", cacheSizeMetricName, got)
	}

	// Second request for the same response to test hit counter.
	if _, err = dns.Exchange(m, udp); err != nil {
		t.Fatalf("Could not send message: %s", err)
	}

	data = mtest.Scrape(t, "http://"+metrics.ListenAddr+"/metrics")
	// Get the value for the cache hit counter where the one of the labels values matches "success".
	got, _ = mtest.MetricValueLabel(cacheHitMetricName, cache.Success, data)

	if got != "2" {
		t.Errorf("Expected value %s for %s, but got %s", "2", cacheHitMetricName, got)
	}
}

func TestMetricsAuto(t *testing.T) {
	tmpdir, err := ioutil.TempDir(os.TempDir(), "coredns")
	if err != nil {
		t.Fatal(err)
	}

	corefile := `org:0 {
		auto {
			directory ` + tmpdir + ` db\.(.*) {1} 1
		}
		prometheus localhost:0
	}
`

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
	if err = ioutil.WriteFile(path.Join(tmpdir, "db.example.org"), []byte(zoneContent), 0644); err != nil {
		t.Fatal(err)
	}
	// TODO(miek): make the auto sleep even less.
	time.Sleep(1100 * time.Millisecond) // wait for it to be picked up

	m := new(dns.Msg)
	m.SetQuestion("www.example.org.", dns.TypeA)

	if _, err := dns.Exchange(m, udp); err != nil {
		t.Fatalf("Could not send message: %s", err)
	}

	metricName := "coredns_dns_request_count_total" //{zone, proto, family}

	data := mtest.Scrape(t, "http://"+metrics.ListenAddr+"/metrics")
	// Get the value for the metrics where the one of the labels values matches "example.org."
	got, _ := mtest.MetricValueLabel(metricName, "example.org.", data)

	if got != "1" {
		t.Errorf("Expected value %s for %s, but got %s", "1", metricName, got)
	}

	// Remove db.example.org again. And see if the metric stops increasing.
	os.Remove(path.Join(tmpdir, "db.example.org"))
	time.Sleep(1100 * time.Millisecond) // wait for it to be picked up
	if _, err := dns.Exchange(m, udp); err != nil {
		t.Fatalf("Could not send message: %s", err)
	}

	data = mtest.Scrape(t, "http://"+metrics.ListenAddr+"/metrics")
	got, _ = mtest.MetricValueLabel(metricName, "example.org.", data)

	if got != "1" {
		t.Errorf("Expected value %s for %s, but got %s", "1", metricName, got)
	}
}

func checkHealthStatus(addr string) (bool, error) {
	resp, err := http.Get(fmt.Sprintf("http://%s/health", addr))
	if err != nil {
		return false, err
	}
	ok, _ := ioutil.ReadAll(resp.Body)
	resp.Body.Close()
	return string(ok) == "OK", err
}

// RepairHealth - a way to relaunch the listeners of Metrics.
// this can be helpful if you compie with Go version < 1.10
// you need to have a kind of monitoring private plugin that would use the healthcheck is coordination with this function
func RepairHealth(inst *caddy.Instance) (bool, error) {
	uniqAddr := inst.Storage["metricsData"].(*uniq.U)
	if uniqAddr == nil {
		return false, fmt.Errorf("cannot retrieve metrics data form instance storage")
	}
	if uniqAddr.HasTodo() {
		uniqAddr.ForEach()
	}
	return !uniqAddr.HasTodo(), nil
}

// verify the implementation of Healther by metrics
// it is good time fo verify that the above function is also working
func TestMetricsHealthAndRepair(t *testing.T) {

	healthAddr := "127.0.0.1:53333"

	// In purpose use a TCP Listener that will fail the listening of metrics
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Could not setup the test : %s", err)
	}

	// Start CoreDNS with Health and metrics
	corefile := fmt.Sprintf(`
.:0 {
	prometheus %s
	health %s
	whoami
}`, ln.Addr(), healthAddr)

	c, err := CoreDNSServer(corefile)
	if err != nil {
		if strings.Contains(err.Error(), inUse) {
			return // meh, but don't error
		}
		t.Fatalf("Could not get service instance: %s", err)
	}
	defer c.Stop()

	// check that healh is not OK.
	ok, err := checkHealthStatus(healthAddr)
	if err != nil {
		t.Fatalf("Unexpected error checking health status : %s", err)
	}
	if ok {
		t.Errorf("Expected value %s for health, but got %v", "false (not OK)", ok)
	}

	// try to repair metrics plugins (all of them as they share the same uniqAddr instance)
	// you should not be able as the listener is still on
	ok, err = RepairHealth(c)
	if err != nil {
		t.Fatalf("Unexpected error whil trying to Repair gealth of metrics : %s", err)
	}
	if ok {
		t.Errorf("Expected value %s for metrics Repair, but got %v", "false (not OK)", ok)
	}

	// Stop the initial listener
	ln.Close()
	time.Sleep(time.Millisecond * 100)

	// try to repair metrics plugins (all of them as they share the same uniqAddr instance)
	// it should be successful as the initial listener is stopped and the port is now available
	ok, err = RepairHealth(c)
	if err != nil {
		t.Fatalf("Unexpected error whil trying to Repair gealth of metrics : %s", err)
	}
	if !ok {
		t.Errorf("Expected value %s for metrics Repair after stoping listener, but got %v", "true (OK)", ok)
	}

	// check that healh is going to OK.
	// it can take up to 1 sec as it is based period for polling health
	ok = false
	for i := 0; i < 11; i++ {
		ok, err = checkHealthStatus(healthAddr)
		if err != nil {
			t.Fatalf("Unexpected error checking health status : %s", err)
		}
		if ok {
			break
		}
		time.Sleep(time.Millisecond * 100)
	}
	if !ok {
		t.Errorf("Expected value %s for health after repair, but got %v", "true (OK)", ok)
	}

}
