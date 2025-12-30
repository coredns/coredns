package test

import (
	"bytes"
	"context"
	"crypto/tls"
	"io"
	"net/http"
	"testing"
	"time"

	ctls "github.com/coredns/coredns/plugin/pkg/tls"

	"github.com/miekg/dns"
	"github.com/quic-go/quic-go/http3"
)

var https3Corefile = `https3://.:0 {
	tls ../plugin/tls/test_cert.pem ../plugin/tls/test_key.pem ../plugin/tls/test_ca.pem
	whoami
}`

var https3LimitCorefile = `https3://.:0 {
	tls ../plugin/tls/test_cert.pem ../plugin/tls/test_key.pem ../plugin/tls/test_ca.pem
	https3 {
		max_streams 2
	}
	whoami
}`

func generateHTTPS3TLSConfig() *tls.Config {
	tlsConfig, err := ctls.NewTLSConfig(
		"../plugin/tls/test_cert.pem",
		"../plugin/tls/test_key.pem",
		"../plugin/tls/test_ca.pem")

	if err != nil {
		panic(err)
	}

	tlsConfig.InsecureSkipVerify = true

	return tlsConfig
}

func TestHTTPS3(t *testing.T) {
	s, udp, _, err := CoreDNSServerAndPorts(https3Corefile)
	if err != nil {
		t.Fatalf("Could not get CoreDNS serving instance: %s", err)
	}
	defer s.Stop()

	// Create HTTP/3 client
	transport := &http3.Transport{
		TLSClientConfig: generateHTTPS3TLSConfig(),
	}
	defer transport.Close()

	client := &http.Client{
		Transport: transport,
		Timeout:   5 * time.Second,
	}

	// Create DNS query
	m := new(dns.Msg)
	m.SetQuestion("whoami.example.org.", dns.TypeA)
	msg, err := m.Pack()
	if err != nil {
		t.Fatalf("Failed to pack DNS message: %v", err)
	}

	// Make DoH3 request - use UDP address for HTTP/3
	url := "https://" + convertAddress(udp) + "/dns-query"
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, url, bytes.NewReader(msg))
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

// TestHTTPS3WithLimits tests that the server starts and works with configured limits
func TestHTTPS3WithLimits(t *testing.T) {
	s, udp, _, err := CoreDNSServerAndPorts(https3LimitCorefile)
	if err != nil {
		t.Fatalf("Could not get CoreDNS serving instance: %s", err)
	}
	defer s.Stop()

	transport := &http3.Transport{
		TLSClientConfig: generateHTTPS3TLSConfig(),
	}
	defer transport.Close()

	client := &http.Client{
		Transport: transport,
		Timeout:   5 * time.Second,
	}

	m := new(dns.Msg)
	m.SetQuestion("whoami.example.org.", dns.TypeA)
	msg, _ := m.Pack()

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, "https://"+convertAddress(udp)+"/dns-query", bytes.NewReader(msg))
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
