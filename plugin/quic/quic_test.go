package quic

import (
	"crypto/tls"
	"testing"
)

func TestQUICList(t *testing.T) {
	f := QUIC{
		proxies: []*Proxy{
			newProxy("1.1.1.1:853", new(tls.Config)),
			newProxy("2.2.2.2:853", new(tls.Config)),
			newProxy("3.3.3.3:853", new(tls.Config)),
		},
		p: &roundRobin{},
	}

	expect := []*Proxy{
		newProxy("2.2.2.2:853", new(tls.Config)),
		newProxy("1.1.1.1:853", new(tls.Config)),
		newProxy("3.3.3.3:853", new(tls.Config)),
	}

	got := f.list()

	if len(got) != len(expect) {
		t.Fatalf("Expected: %v results, got: %v", len(expect), len(got))
	}

	for i, p := range got {
		if p.addr != expect[i].addr {
			t.Fatalf("Expected proxy %v to be '%v', got: '%v'", i, expect[i].addr, p.addr)
		}
	}
}
