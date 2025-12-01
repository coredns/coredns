package tls

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/coredns/coredns/plugin/test"
)

func getPEMFiles(t *testing.T) (cert, key, ca string) {
	tempDir, err := test.WritePEMFiles(t)
	if err != nil {
		t.Fatalf("Could not write PEM files: %s", err)
	}

	cert = filepath.Join(tempDir, "cert.pem")
	key = filepath.Join(tempDir, "key.pem")
	ca = filepath.Join(tempDir, "ca.pem")

	return
}

func TestNewTLSConfig(t *testing.T) {
	cert, key, ca := getPEMFiles(t)
	_, err := NewTLSConfig(cert, key, ca)
	if err != nil {
		t.Errorf("Failed to create TLSConfig: %s", err)
	}
}

func TestNewTLSClientConfig(t *testing.T) {
	_, _, ca := getPEMFiles(t)

	_, err := NewTLSClientConfig(ca)
	if err != nil {
		t.Errorf("Failed to create TLSConfig: %s", err)
	}
}

func TestNewTLSConfigFromArgs(t *testing.T) {
	cert, key, ca := getPEMFiles(t)

	_, err := NewTLSConfigFromArgs()
	if err != nil {
		t.Errorf("Failed to create TLSConfig: %s", err)
	}

	c, err := NewTLSConfigFromArgs(ca)
	if err != nil {
		t.Errorf("Failed to create TLSConfig: %s", err)
	}
	if c.RootCAs == nil {
		t.Error("RootCAs should not be nil when one arg passed")
	}

	c, err = NewTLSConfigFromArgs(cert, key)
	if err != nil {
		t.Errorf("Failed to create TLSConfig: %s", err)
	}
	if c.RootCAs != nil {
		t.Error("RootCAs should be nil when two args passed")
	}
	if len(c.Certificates) != 1 {
		t.Error("Certificates should have a single entry when two args passed")
	}
	args := []string{cert, key, ca}
	c, err = NewTLSConfigFromArgs(args...)
	if err != nil {
		t.Errorf("Failed to create TLSConfig: %s", err)
	}
	if c.RootCAs == nil {
		t.Error("RootCAs should not be nil when three args passed")
	}
	if len(c.Certificates) != 1 {
		t.Error("Certificates should have a single entry when three args passed")
	}
}

func TestNewTLSConfigFromArgsWithRoot(t *testing.T) {
	cert, key, ca := getPEMFiles(t)
	tempDir, err := os.MkdirTemp("", "go-test-pemfiles")
	defer func() {
		if err := os.RemoveAll(tempDir); err != nil {
			t.Error("failed to clean up temporary directory", err)
		}
	}()
	if err != nil {
		t.Error("failed to create temporary directory", err)
	}
	root := tempDir
	args := []string{cert, key, ca}
	for i := range args {
		if !filepath.IsAbs(args[i]) && root != "" {
			args[i] = filepath.Join(root, args[i])
		}
	}
	c, err := NewTLSConfigFromArgs(args...)
	if err != nil {
		t.Errorf("Failed to create TLSConfig: %s", err)
	}
	if c.RootCAs == nil {
		t.Error("RootCAs should not be nil when three args passed")
	}
	if len(c.Certificates) != 1 {
		t.Error("Certificates should have a single entry when three args passed")
	}
}

func TestNewHTTPSTransport(t *testing.T) {
	_, _, ca := getPEMFiles(t)

	cc, err := NewTLSClientConfig(ca)
	if err != nil {
		t.Errorf("Failed to create TLSConfig: %s", err)
	}

	tr := NewHTTPSTransport(cc)
	if tr == nil {
		t.Errorf("Failed to create https transport with cc")
	}

	tr = NewHTTPSTransport(nil)
	if tr == nil {
		t.Errorf("Failed to create https transport without cc")
	}
}
