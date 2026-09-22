package tls

import (
	"crypto/tls"
	"path/filepath"
	"testing"

	"github.com/coredns/coredns/plugin/test"
)

func getPEMFiles(t *testing.T) (cert, key, ca string) {
	t.Helper()
	tempDir, err := test.WritePEMFiles(t)
	if err != nil {
		t.Fatalf("Could not write PEM files: %s", err)
	}

	cert = filepath.Join(tempDir, "cert.pem")
	key = filepath.Join(tempDir, "key.pem")
	ca = filepath.Join(tempDir, "ca.pem")

	return
}

func assertTLSDefaults(t *testing.T, c *tls.Config) {
	t.Helper()
	if c.MinVersion != tls.VersionTLS12 {
		t.Errorf("MinVersion = %d, want %d", c.MinVersion, tls.VersionTLS12)
	}
	if c.MaxVersion != 0 {
		t.Errorf("MaxVersion = %d, want 0 to use Go defaults", c.MaxVersion)
	}
	if c.CipherSuites != nil {
		t.Errorf("CipherSuites = %v, want nil to use Go defaults", c.CipherSuites)
	}
	if c.CurvePreferences != nil {
		t.Errorf("CurvePreferences = %v, want nil to use Go defaults", c.CurvePreferences)
	}
}

func TestNewTLSConfig(t *testing.T) {
	cert, key, ca := getPEMFiles(t)
	c, err := NewTLSConfig(cert, key, ca)
	if err != nil {
		t.Errorf("Failed to create TLSConfig: %s", err)
	}
	assertTLSDefaults(t, c)
}

func TestNewTLSClientConfig(t *testing.T) {
	_, _, ca := getPEMFiles(t)

	c, err := NewTLSClientConfig(ca)
	if err != nil {
		t.Errorf("Failed to create TLSConfig: %s", err)
	}
	assertTLSDefaults(t, c)
}

func TestNewTLSConfigFromArgs(t *testing.T) {
	cert, key, ca := getPEMFiles(t)

	c, err := NewTLSConfigFromArgs()
	if err != nil {
		t.Errorf("Failed to create TLSConfig: %s", err)
	}
	assertTLSDefaults(t, c)

	c, err = NewTLSConfigFromArgs(ca)
	if err != nil {
		t.Errorf("Failed to create TLSConfig: %s", err)
	}
	assertTLSDefaults(t, c)
	if c.RootCAs == nil {
		t.Error("RootCAs should not be nil when one arg passed")
	}

	c, err = NewTLSConfigFromArgs(cert, key)
	if err != nil {
		t.Errorf("Failed to create TLSConfig: %s", err)
	}
	assertTLSDefaults(t, c)
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
	assertTLSDefaults(t, c)
	if c.RootCAs == nil {
		t.Error("RootCAs should not be nil when three args passed")
	}
	if len(c.Certificates) != 1 {
		t.Error("Certificates should have a single entry when three args passed")
	}
}

func TestNewTLSConfigFromArgsWithRoot(t *testing.T) {
	cert, key, ca := getPEMFiles(t)
	tempDir := t.TempDir()

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
	assertTLSDefaults(t, c)
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
