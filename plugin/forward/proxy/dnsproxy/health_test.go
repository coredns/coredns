package dnsproxy

import (
	"crypto/tls"
	"testing"
	"time"

	"github.com/coredns/coredns/plugin/forward/metrics"
	"github.com/coredns/coredns/plugin/pkg/transport"
)

func TestNewHealthChecker(t *testing.T) {
	h := newHealthChecker(transport.DNS, nil)
	if h == nil {
		t.Errorf("Expected new DNS type HealthChecker")
	}

	h = newHealthChecker(transport.GRPC, nil)
	if h != nil {
		t.Errorf("Expected an error GRPC transport is not a valid transport")
	}
}

func TestCheck(t *testing.T) {
	const defaultExpire = 10 * time.Second
	var m = metrics.New()
	opts := &DNSOpts{
		TLSConfig: new(tls.Config),
		Expire:    defaultExpire,
		ForceTCP:  false,
		PreferUDP: false,
	}
	p := New("8.8.8.8:53", opts, m)
	h := newHealthChecker(transport.DNS, m)
	err := h.check(p)
	if err != nil {
		t.Errorf("Expected no error but got: %s", err.Error())
	}
}
