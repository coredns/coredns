package test

import (
	"bytes"
	"crypto/tls"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/miekg/dns"
)

var httpsCorefile = `https://.:0 {
	tls ../plugin/tls/test_cert.pem ../plugin/tls/test_key.pem ../plugin/tls/test_ca.pem
	whoami
}`

var httpsLimitCorefile = `https://.:0 {
	tls ../plugin/tls/test_cert.pem ../plugin/tls/test_key.pem ../plugin/tls/test_ca.pem
	https {
		max_connections 2
	}
	whoami
}`

func TestHTTPS(t *testing.T) {
	s, _, tcp, err := CoreDNSServerAndPorts(httpsCorefile)
	if err != nil {
		t.Fatalf("Could not get CoreDNS serving instance: %s", err)
	}
	defer s.Stop()

	// Create HTTPS client with TLS config
	tlsConfig := &tls.Config{
		InsecureSkipVerify: true,
	}
	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: tlsConfig,
		},
		Timeout: 5 * time.Second,
	}

	// Create DNS query
	m := new(dns.Msg)
	m.SetQuestion("whoami.example.org.", dns.TypeA)
	msg, err := m.Pack()
	if err != nil {
		t.Fatalf("Failed to pack DNS message: %v", err)
	}

	// Make DoH request
	url := "https://" + tcp + "/dns-query"
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(msg))
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/dns-message")
	req.Header.Set("Accept", "application/dns-message")

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read response: %v", err)
	}

	d := new(dns.Msg)
	err = d.Unpack(body)
	if err != nil {
		t.Fatalf("Failed to unpack response: %v", err)
	}

	if d.Rcode != dns.RcodeSuccess {
		t.Errorf("Expected success but got %d", d.Rcode)
	}

	if len(d.Extra) != 2 {
		t.Errorf("Expected 2 RRs in additional section, but got %d", len(d.Extra))
	}
}

// TestHTTPSWithLimits tests that the server starts and works with configured limits
func TestHTTPSWithLimits(t *testing.T) {
	s, _, tcp, err := CoreDNSServerAndPorts(httpsLimitCorefile)
	if err != nil {
		t.Fatalf("Could not get CoreDNS serving instance: %s", err)
	}
	defer s.Stop()

	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
		Timeout: 5 * time.Second,
	}

	m := new(dns.Msg)
	m.SetQuestion("whoami.example.org.", dns.TypeA)
	msg, _ := m.Pack()

	req, _ := http.NewRequest(http.MethodPost, "https://"+tcp+"/dns-query", bytes.NewReader(msg))
	req.Header.Set("Content-Type", "application/dns-message")

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Request failed: %s", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", resp.StatusCode)
	}
}

// TestHTTPSConnectionLimit tests that connection limits are enforced
func TestHTTPSConnectionLimit(t *testing.T) {
	s, _, tcp, err := CoreDNSServerAndPorts(httpsLimitCorefile)
	if err != nil {
		t.Fatalf("Could not get CoreDNS serving instance: %s", err)
	}
	defer s.Stop()

	const maxConns = 2
	const totalConns = 4

	// Create raw TLS connections to hold them open
	conns := make([]net.Conn, 0, totalConns)
	defer func() {
		for _, c := range conns {
			c.Close()
		}
	}()

	// Open connections up to the limit - these should succeed
	for i := range maxConns {
		conn, err := tls.Dial("tcp", tcp, &tls.Config{InsecureSkipVerify: true})
		if err != nil {
			t.Fatalf("Connection %d failed (should succeed): %v", i+1, err)
		}
		conns = append(conns, conn)
	}

	// Try to open more connections beyond the limit
	// The LimitListener blocks Accept() until a slot is free, so Dial with timeout should fail
	conn, err := tls.DialWithDialer(
		&net.Dialer{Timeout: 100 * time.Millisecond},
		"tcp", tcp,
		&tls.Config{InsecureSkipVerify: true},
	)
	if err == nil {
		conn.Close()
		t.Fatal("Connection beyond limit should have timed out")
	}

	// Close one connection and verify a new one can be established
	conns[0].Close()
	conns = conns[1:]

	time.Sleep(10 * time.Millisecond) // Give the listener time to accept

	conn, err = tls.Dial("tcp", tcp, &tls.Config{InsecureSkipVerify: true})
	if err != nil {
		t.Fatalf("Connection after freeing slot failed: %v", err)
	}
	conns = append(conns, conn)
}
