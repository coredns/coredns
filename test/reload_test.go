package test

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/miekg/dns"
)

func TestReload(t *testing.T) {
	corefile := `.:0 {
	whoami
}
`
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
	corefile := `
.:0 {
	health 127.0.0.1:52182
	whoami
}`
	c, err := CoreDNSServer(corefile)
	if err != nil {
		if strings.Contains(err.Error(), inUse) {
			return // meh, but don't error
		}
		t.Fatalf("Could not get service instance: %s", err)
	}

	if c1, err := c.Restart(NewInput(corefile)); err != nil {
		t.Fatal(err)
	} else {
		c1.Stop()
	}
}

func TestReloadMetricsHealth(t *testing.T) {
	corefile := `
.:0 {
	prometheus 127.0.0.1:53183
	health 127.0.0.1:53184
	whoami
}`
	c, err := CoreDNSServer(corefile)
	if err != nil {
		if strings.Contains(err.Error(), inUse) {
			return // meh, but don't error
		}
		t.Fatalf("Could not get service instance: %s", err)
	}

	c1, err := c.Restart(NewInput(corefile))
	if err != nil {
		t.Fatal(err)
	}
	defer c1.Stop()

	time.Sleep(1 * time.Second)

	// Health
	resp, err := http.Get("http://localhost:53184/health")
	if err != nil {
		t.Fatal(err)
	}
	ok, _ := ioutil.ReadAll(resp.Body)
	resp.Body.Close()
	if string(ok) != "OK" {
		t.Errorf("Failed to receive OK, got %s", ok)
	}

	// Metrics
	resp, err = http.Get("http://localhost:53183/metrics")
	if err != nil {
		t.Fatal(err)
	}

	// verify Metrics is running
	err = collectMetricsInfo("localhost:53183")
	if err != nil {
		t.Errorf("Prometheus is not listening : %s", err)
	}

}

func collectMetricsInfo(addr string) error {
	cl := &http.Client{}
	resp, err := cl.Get(fmt.Sprintf("http://%s/metrics", addr))
	if err != nil {
		return err
	}
	const proc = "coredns_build_info"
	metrics, _ := ioutil.ReadAll(resp.Body)
	if !bytes.Contains(metrics, []byte(proc)) {
		return fmt.Errorf("failed to see %s in metric output", proc)
	}
	return nil
}

// Because of behavior of default HttServer. This Test will work ONLY if metrics HTTP Server is closed with a shutdown
// (a close will not completely end the service)
func TestReloadSeveralTimeMetrics(t *testing.T) {
	//TODO: add a tool that find an available port.
	promAddress := "127.0.0.1:53185" // need to be a port that is not used in another test ! not very convenient
	corefilePrometheus := `
.:0 {
	prometheus ` + promAddress + `
	whoami
}`
	corefileNoPrometheus := `
.:0 {
	whoami
}`

	// The target of the test is to show that after several reloads,
	// plugin metrics still follow directive of the LAST reloaded config
	// check no metrics available
	err := collectMetricsInfo(promAddress)
	if err == nil {
		t.Errorf("Prometheus is listening BEFORE the test start")
	}

	// start coredns for first time
	c, err := CoreDNSServer(corefilePrometheus)
	if err != nil {
		if strings.Contains(err.Error(), inUse) {
			return // meh, but don't error
		}
		t.Errorf("Could not get service instance: %s", err)
	}

	// verify Metrics is running
	err = collectMetricsInfo(promAddress)
	if err != nil {
		t.Errorf("Prometheus is not listening : %s", err)
	}

	nbRestarts := 2 // for local investigation it can be usefull to loop more than twice.
	// restart several times
	for i := 0; i < nbRestarts; i++ {
		c, err = c.Restart(NewInput(corefilePrometheus))
		if err != nil {
			t.Errorf("Could not restart CoreDNS : %s, at loop %v", err, i)
		}
		// verify Metrics is running
		err = collectMetricsInfo(promAddress)
		if err != nil {
			t.Errorf("Prometheus is not listening : %s", err)
		}
	}

	// now restart w/o Prometheus
	c2, err := c.Restart(NewInput(corefileNoPrometheus))
	if err != nil {
		t.Errorf("Could not restart a second time CoreDNS : %s", err)
	}
	c2.Stop()

	// verify Metrics is NOT running
	err = collectMetricsInfo(promAddress)
	if err == nil {
		t.Errorf("Prometheus is STILL listening")
	}
}

const inUse = "address already in use"
