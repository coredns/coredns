package test

import (
	"bytes"
	"crypto/tls"
	"encoding/binary"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/coredns/caddy"
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

// TestHTTPSMaxStreamsKeyOrder verifies that max_streams applies regardless of the
// order of keys in a multi-transport server block. Caddy runs the https directive
// setup only for the first key, so without propagation the setting would be stored
// on the first key's config and dropped for the HTTPS key when it is listed second.
func TestHTTPSMaxStreamsKeyOrder(t *testing.T) {
	const maxStreams = 7
	corefiles := map[string]string{
		"https_first": `https://.:0 .:0 {
	bind 127.0.0.1
	tls ../plugin/tls/test_cert.pem ../plugin/tls/test_key.pem ../plugin/tls/test_ca.pem
	https {
		max_streams 7
	}
	whoami
}`,
		"https_second": `.:0 https://.:0 {
	bind 127.0.0.1
	tls ../plugin/tls/test_cert.pem ../plugin/tls/test_key.pem ../plugin/tls/test_ca.pem
	https {
		max_streams 7
	}
	whoami
}`,
	}

	for name, corefile := range corefiles {
		t.Run(name, func(t *testing.T) {
			s, err := CoreDNSServer(corefile)
			if err != nil {
				t.Fatalf("Could not get CoreDNS serving instance: %s", err)
			}
			defer s.Stop()

			tcp := httpsListenerAddr(t, s)
			if got := advertisedMaxConcurrentStreams(t, tcp); got != maxStreams {
				t.Errorf("advertised SETTINGS_MAX_CONCURRENT_STREAMS = %d, want %d", got, maxStreams)
			}
		})
	}
}

// httpsListenerAddr returns the TCP address of the instance's HTTPS listener.
// In a multi-transport block the HTTPS server is not necessarily first; it is
// the TCP-only listener (no packetconn), identified by a nil LocalAddr.
func httpsListenerAddr(t *testing.T, i *caddy.Instance) string {
	t.Helper()
	for _, s := range i.Servers() {
		if s.LocalAddr() == nil && s.Addr() != nil {
			return s.Addr().String()
		}
	}
	t.Fatal("no HTTPS (TCP-only) listener found among started servers")
	return ""
}

// advertisedMaxConcurrentStreams opens a TLS/h2 connection to addr and returns the
// server's advertised SETTINGS_MAX_CONCURRENT_STREAMS. It returns 0 if the server
// does not send the setting.
func advertisedMaxConcurrentStreams(t *testing.T, addr string) uint32 {
	t.Helper()

	conn, err := tls.Dial("tcp", addr, &tls.Config{InsecureSkipVerify: true, NextProtos: []string{"h2"}})
	if err != nil {
		t.Fatalf("TLS dial %s failed: %v", addr, err)
	}
	defer conn.Close()

	if proto := conn.ConnectionState().NegotiatedProtocol; proto != "h2" {
		t.Fatalf("ALPN did not negotiate h2, got %q", proto)
	}

	if _, err := conn.Write([]byte("PRI * HTTP/2.0\r\n\r\nSM\r\n\r\n")); err != nil {
		t.Fatalf("write preface: %v", err)
	}
	if _, err := conn.Write([]byte{0, 0, 0, 0x4, 0, 0, 0, 0, 0}); err != nil {
		t.Fatalf("write client SETTINGS: %v", err)
	}

	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	hdr := make([]byte, 9)
	for {
		if _, err := io.ReadFull(conn, hdr); err != nil {
			t.Fatalf("read frame header: %v", err)
		}
		length := int(hdr[0])<<16 | int(hdr[1])<<8 | int(hdr[2])
		payload := make([]byte, length)
		if length > 0 {
			if _, err := io.ReadFull(conn, payload); err != nil {
				t.Fatalf("read frame payload: %v", err)
			}
		}
		if hdr[3] != 0x4 || hdr[4]&0x1 != 0 { // not a SETTINGS frame, or a SETTINGS ACK
			continue
		}
		for i := 0; i+6 <= len(payload); i += 6 {
			if binary.BigEndian.Uint16(payload[i:i+2]) == 0x3 { // SETTINGS_MAX_CONCURRENT_STREAMS
				return binary.BigEndian.Uint32(payload[i+2 : i+6])
			}
		}
		return 0
	}
}
