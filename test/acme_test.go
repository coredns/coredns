package test

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io"
	"log"
	"net"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/letsencrypt/pebble/v2/ca"
	"github.com/letsencrypt/pebble/v2/db"
	"github.com/letsencrypt/pebble/v2/va"
	"github.com/letsencrypt/pebble/v2/wfe"
	"github.com/miekg/dns"
)

func TestACMEDNS01CertificateManagement(t *testing.T) {
	t.Setenv("PEBBLE_VA_NOSLEEP", "1")
	t.Setenv("PEBBLE_WFE_NONCEREJECT", "0")
	t.Setenv("PEBBLE_AUTHZREUSE", "0")

	dnsPort := freeTCPUDPPort(t)
	dotPort := freeTCPPort(t)
	resolver := net.JoinHostPort("127.0.0.1", fmt.Sprint(dnsPort))

	logger := log.New(io.Discard, "", 0)
	store := db.NewMemoryStore()
	pebbleCA := ca.New(logger, store, "", "ecdsa", 0, 1, map[string]ca.Profile{
		"default": {Description: "default", ValidityPeriod: 3600},
	})
	validator := va.New(logger, 5002, 5001, false, resolver, store)
	frontend := wfe.New(logger, store, validator, pebbleCA, []string{"pebble"}, false, false, 0, 0)
	acmeServer := httptest.NewTLSServer(frontend.Handler())
	defer acmeServer.Close()

	tempDir := t.TempDir()
	serverRoot := filepath.Join(tempDir, "acme-server-root.pem")
	serverCert := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: acmeServer.Certificate().Raw})
	if err := os.WriteFile(serverRoot, serverCert, 0600); err != nil {
		t.Fatal(err)
	}

	const domain = "dns.example"
	corefile := fmt.Sprintf(`dns://.:%d {
	bind 127.0.0.1
	whoami
}

tls://.:%d {
	bind 127.0.0.1
	tls {
		acme %s
		ca %s%s
		ca_root %s
		storage %s
		resolver 127.0.0.1:%d
	}
	whoami
}`, dnsPort, dotPort, domain, acmeServer.URL, wfe.DirectoryPath, serverRoot, filepath.Join(tempDir, "certificates"), dnsPort)

	instance, err := CoreDNSServer(corefile)
	if err != nil {
		t.Fatalf("starting CoreDNS: %v", err)
	}
	defer func() {
		instance.ShutdownCallbacks()
		instance.Stop()
	}()

	address := net.JoinHostPort("127.0.0.1", fmt.Sprint(dotPort))
	var peerCertificates []*x509.Certificate
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		conn, err := tls.DialWithDialer(&net.Dialer{Timeout: time.Second}, "tcp", address, &tls.Config{
			ServerName:         domain,
			InsecureSkipVerify: true, // The issued chain is verified explicitly below.
			MinVersion:         tls.VersionTLS12,
		})
		if err == nil {
			peerCertificates = conn.ConnectionState().PeerCertificates
			conn.Close()
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if len(peerCertificates) == 0 {
		t.Fatal("CoreDNS did not obtain and serve an ACME certificate before the deadline")
	}

	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(pebbleCA.GetRootCert(0).PEM()) {
		t.Fatal("could not add Pebble root certificate")
	}
	intermediates := x509.NewCertPool()
	for _, certificate := range peerCertificates[1:] {
		intermediates.AddCert(certificate)
	}
	if _, err := peerCertificates[0].Verify(x509.VerifyOptions{
		DNSName:       domain,
		Roots:         roots,
		Intermediates: intermediates,
	}); err != nil {
		t.Fatalf("verifying managed certificate: %v", err)
	}

	request := new(dns.Msg)
	request.SetQuestion("whoami.example.", dns.TypeA)
	client := &dns.Client{
		Net: "tcp-tls",
		TLSConfig: &tls.Config{
			ServerName: domain,
			RootCAs:    roots,
			MinVersion: tls.VersionTLS12,
		},
		Timeout: 2 * time.Second,
	}
	response, _, err := client.Exchange(request, address)
	if err != nil {
		t.Fatalf("querying CoreDNS with the managed certificate: %v", err)
	}
	if response.Rcode != dns.RcodeSuccess {
		t.Fatalf("DoT response code = %d, want success", response.Rcode)
	}
}

func freeTCPPort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port
}

func freeTCPUDPPort(t *testing.T) int {
	t.Helper()
	for range 10 {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		port := listener.Addr().(*net.TCPAddr).Port
		packet, err := net.ListenPacket("udp", net.JoinHostPort("127.0.0.1", fmt.Sprint(port)))
		if err == nil {
			packet.Close()
			listener.Close()
			return port
		}
		listener.Close()
	}
	t.Fatal("could not find a free TCP/UDP port")
	return 0
}
