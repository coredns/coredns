package forwardcrd

import (
	"strings"
	"testing"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/plugin"
)

func TestForwardCRDParse(t *testing.T) {
	c := caddy.NewTestController("dns", `forwardcrd`)
	k, err := parseForwardCRD(c)
	if err != nil {
		t.Fatalf("Expected no errors, but got: %v", err)
	}
	if k.Namespace != "kube-system" {
		t.Errorf("Expected Namespace to be: %s\n but was: %s\n", "kube-system", k.Namespace)
	}

	c = caddy.NewTestController("dns", `forwardcrd {
		endpoint http://localhost:9090
	}`)
	k, err = parseForwardCRD(c)
	if err != nil {
		t.Fatalf("Expected no errors, but got: %v", err)
	}
	if k.APIServerEndpoint != "http://localhost:9090" {
		t.Errorf("Expected APIServerEndpoint to be: %s\n but was: %s\n", "http://localhost:9090", k.APIServerEndpoint)
	}

	c = caddy.NewTestController("dns", `forwardcrd {
		tls cert.crt key.key cacert.crt
	}`)
	k, err = parseForwardCRD(c)
	if err != nil {
		t.Fatalf("Expected no errors, but got: %v", err)
	}
	if k.APIClientCert != "cert.crt" {
		t.Errorf("Expected APIClientCert to be: %s\n but was: %s\n", "cert.crt", k.APIClientCert)
	}
	if k.APIClientKey != "key.key" {
		t.Errorf("Expected APIClientCert to be: %s\n but was: %s\n", "key.key", k.APIClientKey)
	}
	if k.APICertAuth != "cacert.crt" {
		t.Errorf("Expected APICertAuth to be: %s\n but was: %s\n", "cacert.crt", k.APICertAuth)
	}

	c = caddy.NewTestController("dns", `forwardcrd {
		kubeconfig foo.kubeconfig
	}`)
	_, err = parseForwardCRD(c)
	if err != nil {
		t.Fatalf("Expected no errors, but got: %v", err)
	}

	c = caddy.NewTestController("dns", `forwardcrd {
		kubeconfig foo.kubeconfig context
	}`)
	_, err = parseForwardCRD(c)
	if err != nil {
		t.Fatalf("Expected no errors, but got: %v", err)
	}

	c = caddy.NewTestController("dns", `forwardcrd example.org`)
	k, err = parseForwardCRD(c)
	if err != nil {
		t.Fatalf("Expected no errors, but got: %v", err)
	}
	if len(k.Zones) != 1 || k.Zones[0] != "example.org." {
		t.Fatalf("Expected Zones to consist of \"example.org.\" but was %v", k.Zones)
	}

	c = caddy.NewTestController("dns", `forwardcrd`)
	c.ServerBlockKeys = []string{"example.org"}
	k, err = parseForwardCRD(c)
	if err != nil {
		t.Fatalf("Expected no errors, but got: %v", err)
	}
	if len(k.Zones) != 1 || k.Zones[0] != "example.org." {
		t.Fatalf("Expected Zones to consist of \"example.org.\" but was %v", k.Zones)
	}

	c = caddy.NewTestController("dns", `forwardcrd {
		namespace
	}`)
	k, err = parseForwardCRD(c)
	if err != nil {
		t.Fatalf("Expected no errors, but got: %v", err)
	}
	if k.Namespace != "" {
		t.Errorf("Expected Namespace to be: %q\n but was: %q\n", "", k.Namespace)
	}

	c = caddy.NewTestController("dns", `forwardcrd {
		namespace dns-system
	}`)
	k, err = parseForwardCRD(c)
	if err != nil {
		t.Fatalf("Expected no errors, but got: %v", err)
	}
	if k.Namespace != "dns-system" {
		t.Errorf("Expected Namespace to be: %s\n but was: %s\n", "dns-system", k.Namespace)
	}

	// negative

	c = caddy.NewTestController("dns", `forwardcrd {
		endpoint http://localhost:9090 http://foo.bar:1024
	}`)
	_, err = parseForwardCRD(c)
	if err == nil {
		t.Fatalf("Expected errors, but got nil")
	}
	if !strings.Contains(err.Error(), "Wrong argument count") {
		t.Fatalf("Expected error containing \"Wrong argument count\", but got: %v", err.Error())
	}

	c = caddy.NewTestController("dns", `forwardcrd {
		endpoint
	}`)
	_, err = parseForwardCRD(c)
	if err == nil {
		t.Fatalf("Expected errors, but got nil")
	}
	if !strings.Contains(err.Error(), "Wrong argument count") {
		t.Fatalf("Expected error containing \"Wrong argument count\", but got: %v", err.Error())
	}

	c = caddy.NewTestController("dns", `forwardcrd {
		tls foo bar
	}`)
	_, err = parseForwardCRD(c)
	if err == nil {
		t.Fatalf("Expected errors, but got nil")
	}
	if !strings.Contains(err.Error(), "Wrong argument count") {
		t.Fatalf("Expected error containing \"Wrong argument count\", but got: %v", err.Error())
	}

	c = caddy.NewTestController("dns", `forwardcrd {
		kubeconfig
	}`)
	_, err = parseForwardCRD(c)
	if err == nil {
		t.Fatalf("Expected errors, but got nil")
	}
	if !strings.Contains(err.Error(), "Wrong argument count") {
		t.Fatalf("Expected error containing \"Wrong argument count\", but got: %v", err.Error())
	}

	c = caddy.NewTestController("dns", `forwardcrd {
		kubeconfig too many args
	}`)
	_, err = parseForwardCRD(c)
	if err == nil {
		t.Fatalf("Expected errors, but got nil")
	}
	if !strings.Contains(err.Error(), "Wrong argument count") {
		t.Fatalf("Expected error containing \"Wrong argument count\", but got: %v", err.Error())
	}

	c = caddy.NewTestController("dns", `forwardcrd {
		namespace too many args
	}`)
	_, err = parseForwardCRD(c)
	if err == nil {
		t.Fatalf("Expected errors, but got nil")
	}
	if !strings.Contains(err.Error(), "Wrong argument count") {
		t.Fatalf("Expected error containing \"Wrong argument count\", but got: %v", err.Error())
	}

	c = caddy.NewTestController("dns", `forwardcrd {
		invalid
	}`)
	_, err = parseForwardCRD(c)
	if err == nil {
		t.Fatalf("Expected errors, but got nil")
	}
	if !strings.Contains(err.Error(), "unknown property") {
		t.Fatalf("Expected error containing \"unknown property\", but got: %v", err.Error())
	}

	c = caddy.NewTestController("dns", `forwardcrd
forwardcrd`)
	_, err = parseForwardCRD(c)
	if err == nil {
		t.Fatalf("Expected errors, but got nil")
	}
	if !strings.Contains(err.Error(), plugin.ErrOnce.Error()) {
		t.Fatalf("Expected error containing \"%s\", but got: %v", plugin.ErrOnce.Error(), err.Error())
	}
}
