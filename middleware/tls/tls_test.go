package tls

import (
        "testing"
        "path/filepath"

	"github.com/miekg/coredns/middleware/test"
)

func TestNewTLSConfig(t *testing.T) {
	tempDir, rmFunc, err := test.WritePEMFiles("")
	if err != nil {
		t.Fatalf("Could not write PEM files: %s", err)
	}
	defer rmFunc()

	cert := filepath.Join(tempDir, "cert.pem")
	key := filepath.Join(tempDir, "key.pem")
	ca := filepath.Join(tempDir, "ca.pem")

	_, err = NewTLSConfig(cert, key, ca)
	if err != nil {
		t.Errorf("Failed to create TLSConfig: %s", err)
	}
}

func TestNewTLSClientConfig(t *testing.T) {
	tempDir, rmFunc, err := test.WritePEMFiles("")
	if err != nil {
		t.Fatalf("Could not write PEM files: %s", err)
	}
	defer rmFunc()

	ca := filepath.Join(tempDir, "ca.pem")

	_, err = NewTLSClientConfig(ca)
	if err != nil {
		t.Errorf("Failed to create TLSConfig: %s", err)
	}
}
