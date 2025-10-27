package metrics

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/coredns/coredns/plugin/test"

	"github.com/miekg/dns"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

func createTestCertFiles(t *testing.T) (certFile, keyFile, caFile string, cleanup func()) {
	// Generate CA certificate
	caCert, caKey, err := generateCA()
	if err != nil {
		t.Fatalf("Failed to generate CA certificate: %v", err)
	}

	// Generate server certificate signed by CA
	cert, key, err := generateCert(caCert, caKey)
	if err != nil {
		t.Fatalf("Failed to generate server certificate: %v", err)
	}

	// Write certificates to temporary files
	certFile = writeTempFile(t, string(cert))
	keyFile = writeTempFile(t, string(key))
	caFile = writeTempFile(t, string(caCert))

	// Return cleanup function to remove files
	cleanup = func() {
		os.Remove(certFile)
		os.Remove(keyFile)
		os.Remove(caFile)
	}
	return certFile, keyFile, caFile, cleanup
}

func generateCA() ([]byte, []byte, error) {
	ca := &x509.Certificate{
		SerialNumber: big.NewInt(2023),
		Subject: pkix.Name{
			Organization: []string{"Test CA"},
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().AddDate(1, 0, 0),
		IsCA:                  true,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}

	caPrivKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, nil, err
	}

	caBytes, err := x509.CreateCertificate(rand.Reader, ca, ca, &caPrivKey.PublicKey, caPrivKey)
	if err != nil {
		return nil, nil, err
	}

	caPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: caBytes,
	})

	caPrivKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(caPrivKey),
	})

	return caPEM, caPrivKeyPEM, nil
}

func generateCert(caCertPEM, caKeyPEM []byte) ([]byte, []byte, error) {
	caCertBlock, _ := pem.Decode(caCertPEM)
	caCert, err := x509.ParseCertificate(caCertBlock.Bytes)
	if err != nil {
		return nil, nil, err
	}

	caKeyBlock, _ := pem.Decode(caKeyPEM)
	caKey, err := x509.ParsePKCS1PrivateKey(caKeyBlock.Bytes)
	if err != nil {
		return nil, nil, err
	}

	cert := &x509.Certificate{
		SerialNumber: big.NewInt(2023),
		Subject: pkix.Name{
			Organization: []string{"Test Server"},
		},
		DNSNames:    []string{"localhost"},
		NotBefore:   time.Now(),
		NotAfter:    time.Now().AddDate(1, 0, 0),
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		KeyUsage:    x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
	}

	certPrivKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, nil, err
	}

	certBytes, err := x509.CreateCertificate(rand.Reader, cert, caCert, &certPrivKey.PublicKey, caKey)
	if err != nil {
		return nil, nil, err
	}

	certPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certBytes,
	})

	certPrivKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(certPrivKey),
	})

	return certPEM, certPrivKeyPEM, nil
}

func loadTestClientCert(t *testing.T, certFile, keyFile string) tls.Certificate {
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		t.Fatalf("Failed to load client certificate: %v", err)
	}
	return cert
}

func writeTempFile(t *testing.T, content string) string {
	tmpFile, err := os.CreateTemp("", "testcert")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer tmpFile.Close()

	if _, err := tmpFile.WriteString(content); err != nil {
		t.Fatalf("Failed to write to temp file: %v", err)
	}
	return tmpFile.Name()
}

func TestMetricsTLS(t *testing.T) {
	certFile, keyFile, caFile, cleanup := createTestCertFiles(t)
	defer cleanup()

	tests := []struct {
		name        string
		tlsConfig   *tlsConfig
		expectError bool
		expectHTTPS bool
	}{
		{
			name:        "No TLS",
			tlsConfig:   nil,
			expectError: false,
			expectHTTPS: false,
		},
		{
			name: "Valid TLS",
			tlsConfig: &tlsConfig{
				enabled:    true,
				certFile:   certFile,
				keyFile:    keyFile,
				minVersion: tls.VersionTLS13,
			},
			expectError: false,
			expectHTTPS: true,
		},
		{
			name: "TLS with client auth",
			tlsConfig: &tlsConfig{
				enabled:        true,
				certFile:       certFile,
				keyFile:        keyFile,
				clientCAFile:   caFile,
				clientAuthType: "RequireAndVerifyClientCert",
				minVersion:     tls.VersionTLS13,
			},
			expectError: false,
			expectHTTPS: true,
		},
		{
			name: "Missing cert file",
			tlsConfig: &tlsConfig{
				enabled:    true,
				certFile:   "missing.pem",
				keyFile:    keyFile,
				minVersion: tls.VersionTLS13,
			},
			expectError: true,
			expectHTTPS: false,
		},
		{
			name: "Missing key file",
			tlsConfig: &tlsConfig{
				enabled:    true,
				certFile:   certFile,
				keyFile:    "missing-key.pem",
				minVersion: tls.VersionTLS13,
			},
			expectError: true,
			expectHTTPS: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			met := New("localhost:0")
			met.tlsConfig = tt.tlsConfig

			// Start server
			err := met.OnStartup()
			if tt.expectError {
				if err == nil {
					t.Error("Expected error but got none")
				}
				return
			}
			if err != nil {
				t.Fatalf("Failed to start metrics handler: %s", err)
			}
			defer met.OnFinalShutdown()

			// Wait for server to be ready
			select {
			case <-time.After(2 * time.Second):
				t.Fatal("timeout waiting for server to start")
			case <-func() chan struct{} {
				ch := make(chan struct{})
				go func() {
					for {
						conn, err := net.DialTimeout("tcp", ListenAddr, 100*time.Millisecond)
						if err == nil {
							conn.Close()
							close(ch)
							return
						}
						time.Sleep(100 * time.Millisecond)
					}
				}()
				return ch
			}():
			}

			// Configure client
			tlsConfig := &tls.Config{
				InsecureSkipVerify: true,
			}
			if tt.tlsConfig != nil && tt.tlsConfig.clientCAFile != "" {
				// Load CA cert for client auth
				caCert, err := os.ReadFile(tt.tlsConfig.clientCAFile)
				if err != nil {
					t.Fatalf("Failed to read CA cert: %v", err)
				}
				caCertPool := x509.NewCertPool()
				if !caCertPool.AppendCertsFromPEM(caCert) {
					t.Fatal("Failed to parse CA cert")
				}
				tlsConfig.RootCAs = caCertPool
				tlsConfig.Certificates = []tls.Certificate{loadTestClientCert(t, certFile, keyFile)}
			}

			client := &http.Client{
				Timeout: 10 * time.Second,
				Transport: &http.Transport{
					TLSClientConfig: tlsConfig,
					// Allow connection reuse
					MaxIdleConns:        10,
					IdleConnTimeout:     30 * time.Second,
					DisableCompression:  true,
					TLSHandshakeTimeout: 10 * time.Second,
				},
			}

			// Determine protocol
			protocol := "http"
			if tt.expectHTTPS {
				protocol = "https"
			}

			// Try multiple times to account for server startup time
			var resp *http.Response
			var err2 error
			for i := 0; i < 10; i++ { // Increased retry count
				url := fmt.Sprintf("%s://%s/metrics", protocol, ListenAddr)
				t.Logf("Attempt %d: Connecting to %s", i+1, url)
				resp, err2 = client.Get(url)
				if err2 == nil {
					t.Logf("Successfully connected to metrics server")
					break
				}
				t.Logf("Connection attempt failed: %v", err2)
				time.Sleep(500 * time.Millisecond) // Increased delay between attempts
			}
			if err2 != nil {
				t.Fatalf("Failed to connect to metrics server: %v", err2)
			}
			if resp != nil {
				defer resp.Body.Close()
			}

			if resp.StatusCode != http.StatusOK {
				t.Errorf("Expected status 200, got %d", resp.StatusCode)
			}
		})
	}
}

func TestMetrics(t *testing.T) {
	met := New("localhost:0")
	if err := met.OnStartup(); err != nil {
		t.Fatalf("Failed to start metrics handler: %s", err)
	}
	defer met.OnFinalShutdown()

	met.AddZone("example.org.")

	tests := []struct {
		next          plugin.Handler
		qname         string
		qtype         uint16
		metric        string
		expectedValue string
	}{
		// This all works because 1 bucket (1 zone, 1 type)
		{
			next:          test.NextHandler(dns.RcodeSuccess, nil),
			qname:         "example.org.",
			metric:        "coredns_dns_requests_total",
			expectedValue: "1",
		},
		{
			next:          test.NextHandler(dns.RcodeSuccess, nil),
			qname:         "example.org.",
			metric:        "coredns_dns_requests_total",
			expectedValue: "2",
		},
		{
			next:          test.NextHandler(dns.RcodeSuccess, nil),
			qname:         "example.org.",
			metric:        "coredns_dns_requests_total",
			expectedValue: "3",
		},
		{
			next:          test.NextHandler(dns.RcodeSuccess, nil),
			qname:         "example.org.",
			metric:        "coredns_dns_responses_total",
			expectedValue: "4",
		},
	}

	ctx := context.TODO()

	for i, tc := range tests {
		req := new(dns.Msg)
		if tc.qtype == 0 {
			tc.qtype = dns.TypeA
		}
		req.SetQuestion(tc.qname, tc.qtype)
		met.Next = tc.next

		rec := dnstest.NewRecorder(&test.ResponseWriter{})
		_, err := met.ServeDNS(ctx, rec, req)
		if err != nil {
			t.Fatalf("Test %d: Expected no error, but got %s", i, err)
		}

		result := test.Scrape("http://" + ListenAddr + "/metrics")

		if tc.expectedValue != "" {
			got, _ := test.MetricValue(tc.metric, result)
			if got != tc.expectedValue {
				t.Errorf("Test %d: Expected value %s for metrics %s, but got %s", i, tc.expectedValue, tc.metric, got)
			}
		}
	}
}

func TestMetricsHTTPTimeout(t *testing.T) {
	met := New("localhost:0")
	if err := met.OnStartup(); err != nil {
		t.Fatalf("Failed to start metrics handler: %s", err)
	}
	defer met.OnFinalShutdown()

	// Use context with timeout to prevent test from hanging indefinitely
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	done := make(chan error, 1)

	go func() {
		conn, err := net.Dial("tcp", ListenAddr)
		if err != nil {
			done <- err
			return
		}
		defer conn.Close()

		// Send partial HTTP request and then stop sending data
		// This will cause the server to wait for more data and hit ReadTimeout
		partialRequest := "GET /metrics HTTP/1.1\r\nHost: " + ListenAddr + "\r\nContent-Length: 100\r\n\r\n"
		_, err = conn.Write([]byte(partialRequest))
		if err != nil {
			done <- err
			return
		}

		// Now just wait - server should timeout trying to read the remaining data
		// If server has no ReadTimeout, this will hang indefinitely
		buffer := make([]byte, 1024)
		_, err = conn.Read(buffer)
		done <- err
	}()

	select {
	case <-done:
		t.Log("HTTP request timed out by server")
	case <-ctx.Done():
		t.Error("HTTP request did not time out")
	}
}

func TestMustRegister_DuplicateOK(t *testing.T) {
	met := New("localhost:0")
	met.Reg = prometheus.NewRegistry()

	g := promauto.NewGaugeVec(prometheus.GaugeOpts{Namespace: "test", Name: "dup"}, []string{"l"})
	met.MustRegister(g)
	// registering the same collector again should yield AlreadyRegisteredError internally and be ignored
	met.MustRegister(g)
}

func TestRemoveZone(t *testing.T) {
	met := New("localhost:0")

	met.AddZone("example.org.")
	met.AddZone("example.net.")
	met.RemoveZone("example.net.")

	zones := met.ZoneNames()
	for _, z := range zones {
		if z == "example.net." {
			t.Fatalf("zone %q still present after RemoveZone", z)
		}
	}
}

func TestOnRestartStopsServer(t *testing.T) {
	met := New("localhost:0")
	if err := met.OnStartup(); err != nil {
		t.Fatalf("startup failed: %v", err)
	}

	// server should respond before restart
	resp, err := http.Get("http://" + ListenAddr + "/metrics")
	if err != nil {
		t.Fatalf("pre-restart GET failed: %v", err)
	}
	if resp != nil {
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}

	if err := met.OnRestart(); err != nil {
		t.Fatalf("restart failed: %v", err)
	}

	// after restart, the listener should be closed and request should fail
	if _, err := http.Get("http://" + ListenAddr + "/metrics"); err == nil {
		t.Fatalf("expected GET to fail after restart, but it succeeded")
	}
}

func TestRegistryGetOrSet(t *testing.T) {
	r := newReg()
	addr := "localhost:12345"
	pr1 := prometheus.NewRegistry()
	got1 := r.getOrSet(addr, pr1)
	if got1 != pr1 {
		t.Fatalf("first getOrSet should return provided registry")
	}

	pr2 := prometheus.NewRegistry()
	got2 := r.getOrSet(addr, pr2)
	if got2 != pr1 {
		t.Fatalf("second getOrSet should return original registry, got different one")
	}
}

func TestOnRestartNoop(t *testing.T) {
	met := New("localhost:0")
	// without OnStartup, OnRestart should be a no-op
	if err := met.OnRestart(); err != nil {
		t.Fatalf("OnRestart returned error on no-op: %v", err)
	}
}

func TestContextHelpersEmpty(t *testing.T) {
	if got := WithServer(context.TODO()); got != "" {
		t.Fatalf("WithServer(nil) = %q, want empty", got)
	}
	if got := WithView(context.TODO()); got != "" {
		t.Fatalf("WithView(nil) = %q, want empty", got)
	}
}
