package metrics

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/coredns/coredns/plugin/test"

	"github.com/miekg/dns"
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
				clientAuthType: tls.RequireAndVerifyClientCert,
			},
			expectError: false,
			expectHTTPS: true,
		},
		{
			name: "Missing cert file",
			tlsConfig: &tlsConfig{
				enabled:  true,
				certFile: "missing.pem",
				keyFile:  keyFile,
			},
			expectError: true,
			expectHTTPS: false,
		},
		{
			name: "Missing key file",
			tlsConfig: &tlsConfig{
				enabled:  true,
				certFile: certFile,
				keyFile:  "missing-key.pem",
			},
			expectError: true,
			expectHTTPS: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			met := New("localhost:0")
			met.tlsConfig = tt.tlsConfig

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

			// Verify server is running with correct protocol
			client := &http.Client{
				Timeout: time.Second,
				Transport: &http.Transport{
					TLSClientConfig: &tls.Config{
						InsecureSkipVerify: true,
						Certificates:       []tls.Certificate{loadTestClientCert(t, certFile, keyFile)},
					},
				},
			}

			url := "http://" + ListenAddr + "/metrics"
			if tt.expectHTTPS {
				url = "https://" + ListenAddr + "/metrics"
			}

			resp, err := client.Get(url)
			if err != nil {
				t.Fatalf("Failed to connect to metrics server: %v", err)
			}
			defer resp.Body.Close()

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
