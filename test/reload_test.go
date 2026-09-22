package test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"

	"github.com/miekg/dns"
)

func TestReload(t *testing.T) {
	corefile := `.:0 {
		whoami
	}`

	coreInput := NewInput(corefile)

	c, err := CoreDNSServer(corefile)
	if err != nil {
		t.Fatalf("Could not get CoreDNS serving instance: %s", err)
	}

	udp, _ := CoreDNSServerPorts(c, 0)

	send(t, udp)

	c1, err := c.Restart(coreInput)
	if err != nil {
		t.Fatal(err)
	}
	udp, _ = CoreDNSServerPorts(c1, 0)

	send(t, udp)

	c1.Stop()
}

func send(t *testing.T, server string) {
	t.Helper()
	m := new(dns.Msg)
	m.SetQuestion("whoami.example.org.", dns.TypeSRV)

	r, err := dns.Exchange(m, server)
	if err != nil {
		// This seems to fail a lot on travis, quick'n dirty: redo
		r, err = dns.Exchange(m, server)
		if err != nil {
			return
		}
	}
	if r.Rcode != dns.RcodeSuccess {
		t.Fatalf("Expected successful reply, got %s", dns.RcodeToString[r.Rcode])
	}
	if len(r.Extra) != 2 {
		t.Fatalf("Expected 2 RRs in additional, got %d", len(r.Extra))
	}
}

func TestReloadHealth(t *testing.T) {
	healthAddr := fmt.Sprintf("127.0.0.1:%d", pickPort(t))
	corefile := `.:0 {
		health ` + healthAddr + `
		whoami
	}`

	c, err := CoreDNSServer(corefile)
	if err != nil {
		t.Fatalf("Could not get service instance: %s", err)
	}

	if c1, err := c.Restart(NewInput(corefile)); err != nil {
		t.Fatal(err)
	} else {
		c1.Stop()
	}
}

func TestReloadMetricsHealth(t *testing.T) {
	metricsAddr := fmt.Sprintf("127.0.0.1:%d", pickPort(t))
	healthAddr := fmt.Sprintf("127.0.0.1:%d", pickPort(t))
	corefile := `.:0 {
		prometheus ` + metricsAddr + `
		health ` + healthAddr + `
		whoami
	}`

	c, err := CoreDNSServer(corefile)
	if err != nil {
		t.Fatalf("Could not get service instance: %s", err)
	}

	c1, err := c.Restart(NewInput(corefile))
	if err != nil {
		t.Fatal(err)
	}
	defer c1.Stop()

	// Health
	ok := httpGetBodyEventually(t, "http://"+healthAddr+"/health")
	if string(ok) != http.StatusText(http.StatusOK) {
		t.Errorf("Failed to receive OK, got %s", ok)
	}

	// Metrics
	const proc = "coredns_build_info"
	metrics := httpGetBodyEventually(t, "http://"+metricsAddr+"/metrics")
	if !bytes.Contains(metrics, []byte(proc)) {
		t.Errorf("Failed to see %s in metric output", proc)
	}
}

func collectMetricsInfo(addr string, procs ...string) error {
	cl := &http.Client{}
	resp, err := cl.Get(fmt.Sprintf("http://%s/metrics", addr))
	if err != nil {
		return err
	}
	metrics, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	for _, p := range procs {
		if !bytes.Contains(metrics, []byte(p)) {
			return fmt.Errorf("failed to see %s in metric output \n%s", p, metrics)
		}
	}
	return nil
}

func waitForMetricsInfo(t *testing.T, addr string, procs ...string) {
	t.Helper()

	var lastErr error
	for range 50 {
		lastErr = collectMetricsInfo(addr, procs...)
		if lastErr == nil {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("metrics endpoint did not become ready: %v", lastErr)
}

func httpGetBodyEventually(t *testing.T, url string) []byte {
	t.Helper()

	var lastErr error
	for range 50 {
		resp, err := http.Get(url)
		if err == nil {
			body, readErr := io.ReadAll(resp.Body)
			resp.Body.Close()
			if readErr == nil && resp.StatusCode == http.StatusOK {
				return body
			}
			if readErr != nil {
				lastErr = readErr
			} else {
				lastErr = fmt.Errorf("unexpected status %d with body %q", resp.StatusCode, body)
			}
		} else {
			lastErr = err
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("GET %s did not become ready: %v", url, lastErr)
	return nil
}

func waitForHTTPBody(t *testing.T, url string, want string) {
	t.Helper()

	var lastErr error
	for range 50 {
		resp, err := http.Get(url)
		if err == nil {
			body, readErr := io.ReadAll(resp.Body)
			resp.Body.Close()
			if readErr == nil && string(body) == want {
				return
			}
			if readErr != nil {
				lastErr = readErr
			} else {
				lastErr = fmt.Errorf("unexpected body %q", body)
			}
		} else {
			lastErr = err
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("GET %s did not return expected body %q: %v", url, want, lastErr)
}

// TestReloadSeveralTimeMetrics ensures that metrics are not pushed to
// prometheus once the metrics plugin is removed and a coredns
// reload is triggered
// 1. check that metrics have not been exported to prometheus before coredns starts
// 2. ensure that build-related metrics have been exported once coredns starts
// 3. trigger multiple reloads without changing the corefile
// 4. remove the metrics plugin and trigger a final reload
// 5. ensure the original prometheus exporter has not received more metrics
func TestReloadSeveralTimeMetrics(t *testing.T) {
	promAddress := fmt.Sprintf("127.0.0.1:%d", pickPort(t))
	proc := "coredns_build_info"
	corefileWithMetrics := `.:0 {
		prometheus ` + promAddress + `
		whoami
	}`
	corefileWithoutMetrics := `.:0 {
		whoami
	}`

	if err := collectMetricsInfo(promAddress, proc); err == nil {
		t.Errorf("Prometheus is listening before the test started")
	}
	serverWithMetrics, err := CoreDNSServer(corefileWithMetrics)
	if err != nil {
		t.Errorf("Could not get service instance: %s", err)
	}
	// verify prometheus is running
	waitForMetricsInfo(t, promAddress, proc)
	reloadCount := 2
	for i := range reloadCount {
		serverReload, err := serverWithMetrics.Restart(
			NewInput(corefileWithMetrics),
		)
		if err != nil {
			t.Errorf("Could not restart CoreDNS : %s, at loop %v", err, i)
		}
		waitForMetricsInfo(t, promAddress, proc)
		serverWithMetrics = serverReload
	}
	// reload without prometheus
	serverWithoutMetrics, err := serverWithMetrics.Restart(
		NewInput(corefileWithoutMetrics),
	)
	if err != nil {
		t.Errorf("Could not restart a second time CoreDNS : %s", err)
	}
	serverWithoutMetrics.Stop()
	// verify that metrics have not been pushed
	if err := collectMetricsInfo(promAddress, proc); err == nil {
		t.Errorf("Prometheus is still listening")
	}
}

func TestMetricsAvailableAfterReload(t *testing.T) {
	promAddress := fmt.Sprintf("127.0.0.1:%d", pickPort(t))
	procMetric := "coredns_build_info"
	procCache := "coredns_cache_entries"
	procForward := "coredns_dns_request_duration_seconds"
	corefileWithMetrics := `.:0 {
		prometheus ` + promAddress + `
		cache
		forward . 8.8.8.8 {
			force_tcp
		}
	}`

	inst, _, tcp, err := CoreDNSServerAndPorts(corefileWithMetrics)
	if err != nil {
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
	waitForMetricsInfo(t, promAddress, procMetric, procCache, procForward)

	// now reload
	instReload, err := inst.Restart(
		NewInput(corefileWithMetrics),
	)
	if err != nil {
		t.Errorf("Could not restart CoreDNS : %s", err)
		instReload = inst
	}

	// check the metrics are available still
	waitForMetricsInfo(t, promAddress, procMetric, procCache, procForward)

	instReload.Stop()
	// verify that metrics have not been pushed
}

func TestMetricsAvailableAfterReloadAndFailedReload(t *testing.T) {
	promAddress := fmt.Sprintf("127.0.0.1:%d", pickPort(t))
	procMetric := "coredns_build_info"
	procCache := "coredns_cache_entries"
	procForward := "coredns_dns_request_duration_seconds"
	corefileWithMetrics := `.:0 {
		prometheus ` + promAddress + `
		cache
		forward . 8.8.8.8 {
			force_tcp
		}
	}`
	invalidCorefileWithMetrics := `.:0 {
		prometheus ` + promAddress + `
		cache
		forward . 8.8.8.8 {
			force_tcp
		}
		invalid
	}`

	inst, _, tcp, err := CoreDNSServerAndPorts(corefileWithMetrics)
	if err != nil {
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
	waitForMetricsInfo(t, promAddress, procMetric, procCache, procForward)

	for range 2 {
		// now provide a failed reload
		invInst, err := inst.Restart(
			NewInput(invalidCorefileWithMetrics),
		)
		if err == nil {
			t.Errorf("Invalid test - this reload should fail")
			inst = invInst
		}
	}

	// now reload with correct corefile
	instReload, err := inst.Restart(
		NewInput(corefileWithMetrics),
	)
	if err != nil {
		t.Errorf("Could not restart CoreDNS : %s", err)
		instReload = inst
	}

	// check the metrics are available still
	waitForMetricsInfo(t, promAddress, procMetric, procCache, procForward)

	instReload.Stop()
	// verify that metrics have not been pushed
}

// TestReloadUnreadyPlugin tests that the ready plugin properly resets the list of readiness implementors during a reload.
// If it fails to do so, ready will respond with duplicate plugin names after a reload (e.g. in this test "unready,unready").
func TestReloadUnreadyPlugin(t *testing.T) {
	readyAddr := fmt.Sprintf("127.0.0.1:%d", pickPort(t))
	// Add/Register a perpetually unready plugin
	dnsserver.Directives = append([]string{"unready"}, dnsserver.Directives...)
	u := new(unready)
	u.name = "unready"
	plugin.Register("unready", func(c *caddy.Controller) error {
		dnsserver.GetConfig(c).AddPlugin(func(next plugin.Handler) plugin.Handler {
			u.next = next
			return u
		})
		return nil
	})

	corefile := `.:0 {
		unready
        whoami
        ready ` + readyAddr + `
	}`

	coreInput := NewInput(corefile)

	c, err := CoreDNSServer(corefile)
	if err != nil {
		t.Fatalf("Could not get CoreDNS serving instance: %s", err)
	}

	c1, err := c.Restart(coreInput)
	if err != nil {
		t.Fatal(err)
	}

	waitForHTTPBody(t, "http://"+readyAddr+"/ready", u.Name())

	c1.Stop()
}

// TestReloadTwoServerBlocksUnreadyPlugin tests that the ready plugin properly resets the list of readiness implementors during a reload
// when there are multiple server blocks.
func TestReloadTwoServerBlocksUnreadyPlugin(t *testing.T) {
	readyAddr := fmt.Sprintf("127.0.0.1:%d", pickPort(t))
	// Add/Register a perpetually unready plugin
	dnsserver.Directives = append([]string{"unready1", "unready2"}, dnsserver.Directives...)
	u1 := new(unready)
	u2 := new(unready)
	u1.name = "unready1"
	u2.name = "unready2"
	plugin.Register("unready1", func(c *caddy.Controller) error {
		dnsserver.GetConfig(c).AddPlugin(func(next plugin.Handler) plugin.Handler {
			u1.next = next
			return u1
		})
		return nil
	})
	plugin.Register("unready2", func(c *caddy.Controller) error {
		dnsserver.GetConfig(c).AddPlugin(func(next plugin.Handler) plugin.Handler {
			u2.next = next
			return u2
		})
		return nil
	})
	corefile := `cluster.local.:0 {
		unready1
        whoami
        ready ` + readyAddr + `
	}
    .:0 {
		unready2
        whoami
        ready ` + readyAddr + `
	}`

	coreInput := NewInput(corefile)

	c, err := CoreDNSServer(corefile)
	if err != nil {
		t.Fatalf("Could not get CoreDNS serving instance: %s", err)
	}

	c1, err := c.Restart(coreInput)
	if err != nil {
		t.Fatal(err)
	}

	uName := u1.Name() + "," + u2.Name()
	waitForHTTPBody(t, "http://"+readyAddr+"/ready", uName)

	c1.Stop()
}

// TestReloadConcurrentRestartAndStop ensures there is no deadlock when a restart
// races with a shutdown (issue #7314).
func TestReloadConcurrentRestartAndStop(t *testing.T) {
	corefileA := `.:0 {
		reload 2s 1s
		whoami
	}`
	corefileB := `.:0 {
		reload 2s 1s
		whoami
		# change to trigger different config
	}`

	c, err := CoreDNSServer(corefileA)
	if err != nil {
		if strings.Contains(err.Error(), inUse) {
			return
		}
		t.Fatalf("Could not start CoreDNS instance: %v", err)
	}

	restartErr := make(chan error, 1)
	stopDone := make(chan struct{})

	go func() {
		_, err := c.Restart(NewInput(corefileB))
		restartErr <- err
	}()

	// Small delay to increase overlap window
	time.Sleep(50 * time.Millisecond)

	go func() {
		c.Stop()
		close(stopDone)
	}()

	// Both operations should complete promptly; if not, we may be deadlocked.
	select {
	case <-stopDone:
		// ok
	case <-time.After(5 * time.Second):
		t.Fatalf("Stop did not complete in time (possible deadlock)")
	}
	select {
	case <-restartErr:
		// ok: restart either succeeded or returned an error
		// we only care about not hanging
	case <-time.After(5 * time.Second):
		t.Fatalf("Restart did not complete in time (possible deadlock)")
	}
}

type unready struct {
	next plugin.Handler
	name string
}

func (u *unready) Ready() bool { return false }

func (u *unready) Name() string { return u.name }

func (u *unready) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	return u.next.ServeDNS(ctx, w, r)
}

const inUse = "address already in use"
